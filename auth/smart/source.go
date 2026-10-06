package smart

import (
	"cmp"
	"context"
	"crypto"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/jwtbearer"
	"github.com/cadasto/openehr-sdk-go/internal/noredirect"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

const (
	// clientAssertionType is the RFC 7523 client-assertion-type for
	// private_key_jwt client authentication.
	clientAssertionType = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
	// methodPrivateKeyJWT and methodClientSecretBasic are the
	// token_endpoint_auth_method names used for the discovery cross-check
	// (RFC 8414 / SMART client-confidential-asymmetric).
	methodPrivateKeyJWT     = "private_key_jwt"
	methodClientSecretBasic = "client_secret_basic"
	methodClientSecretPost  = "client_secret_post"
	// defaultAssertionAlg is the client-assertion algorithm used when
	// WithClientAssertionKey names none: RS384, the HL7 SMART asymmetric
	// baseline.
	defaultAssertionAlg = "RS384"
	// nonceLen is the number of random bytes in an OpenID Connect nonce,
	// the same 256 bits as the state.
	nonceLen = 32
)

// clientAssertionKey holds the asymmetric credential for private_key_jwt
// client authentication on the token endpoint (SMART
// client-confidential-asymmetric profile, REQ-068).
type clientAssertionKey struct {
	signer crypto.Signer
	alg    string
	kid    string
}

// Config carries SMART-on-openEHR OAuth2 settings.
type Config struct {
	HTTPClient       *http.Client
	ClientID         string
	ClientSecret     string
	RedirectURI      string
	Scopes           []string
	Audience         string
	Auth             discovery.AuthEndpoints
	Issuer           string
	RefreshThreshold time.Duration
	JWKS             *JWKS
	// IDTokenTrustedAudiences names the audiences an ID token's aud claim
	// may list besides ClientID when the source verifies it. Empty means
	// only ClientID is accepted. See [WithIDTokenTrustedAudiences].
	IDTokenTrustedAudiences []string

	// clientAssertion carries the asymmetric private_key_jwt credential, set
	// via WithClientAssertionKey. Mutually exclusive with ClientSecret.
	clientAssertion *clientAssertionKey
	// assertionSource is built in FromConfig from clientAssertion once the
	// ClientID and token endpoint are known. When non-nil, postToken emits a
	// signed client_assertion instead of HTTP Basic auth (REQ-068).
	assertionSource jwtbearer.AssertionSource
	// secretAuthMethod records how a configured ClientSecret is presented at
	// the token endpoint — client_secret_basic (HTTP Basic, the default) or
	// client_secret_post (credentials in the form body). Resolved by
	// configureClientAuth from the server's advertised methods (REQ-068).
	secretAuthMethod string
	// tokenChange is the hook set by WithTokenChange; nil means none.
	tokenChange func(context.Context, TokenChange)
}

// TokenChange is what the source reports to the [WithTokenChange] hook when
// its tokens change. [Source.Revoke] reports the zero TokenChange.
type TokenChange struct {
	// Access is the access token the source now holds.
	Access auth.Token
	// RefreshToken is the refresh token the source now holds, which is the
	// response's. When the response carried none, it is empty after a code
	// exchange and, after a refresh, the one the source held before.
	RefreshToken string
	// Response is the token response as [Source.LastTokenResponse] returns
	// it, so a refresh response's left-out launch context is filled in. Its
	// Raw map is the hook's own copy. IDTokenClaims and NeedPatientBanner
	// point at values the source keeps: treat them as read-only.
	Response TokenResponse
}

// Option mutates Config during construction.
type Option func(*Config)

// WithHTTPClient injects the client for token and JWKS calls. The token,
// refresh and revocation requests never follow a redirect, whatever
// CheckRedirect c has, and c is not modified. Those requests use a copy of
// c made when the source is built, so a Transport, Timeout or Jar set on c
// afterwards does not reach them.
func WithHTTPClient(c *http.Client) Option {
	return func(cfg *Config) { cfg.HTTPClient = c }
}

// WithClientSecret enables confidential-client token exchange with a
// symmetric secret: client_secret_basic by default, or client_secret_post
// when the server advertises that method and not client_secret_basic.
// Mutually exclusive with WithClientAssertionKey.
func WithClientSecret(secret string) Option {
	return func(cfg *Config) { cfg.ClientSecret = secret }
}

// WithClientAssertionKey enables confidential-client token exchange using
// private_key_jwt (RFC 7523 / SMART client-confidential-asymmetric).
// The signed client_assertion authenticates the client at the token endpoint
// in place of an HTTP Basic header. Each assertion is built by
// [jwtbearer.NewClientAssertion], so it expires five minutes after issue.
//
// alg is the JOSE algorithm: RS384 when empty (the SMART baseline), or
// ES384, RS256 or ES256. kid is required: the HL7 SMART asymmetric profile
// has the assertion name its key in the JWS "kid" header. Construction fails
// with [auth.ErrInvalidConfig] on an empty kid, a nil signer, a key that does
// not fit alg, or an alg that the server's non-empty
// token_endpoint_auth_signing_alg_values_supported does not list. Mutually
// exclusive with WithClientSecret; configuring both is rejected at
// construction the same way.
func WithClientAssertionKey(signer crypto.Signer, alg, kid string) Option {
	return func(cfg *Config) {
		cfg.clientAssertion = &clientAssertionKey{signer: signer, alg: alg, kid: kid}
	}
}

// WithRedirectURI sets the registered redirect URI.
func WithRedirectURI(uri string) Option {
	return func(cfg *Config) { cfg.RedirectURI = uri }
}

// WithScopes sets the space-separated scope request (slice joined).
func WithScopes(scopes ...string) Option {
	return func(cfg *Config) { cfg.Scopes = scopes }
}

// WithAudience sets the `aud` parameter of the authorization request: the
// resource server the access token is meant for. SMART requires it, so a
// source without an audience is refused at construction. Pass the Platform
// base URL (the `iss` of an embedded launch) or an audience identifier the
// authorization server knows. [NewFromCatalog] defaults it to the catalog's
// Platform base URL; set this option to send something else.
func WithAudience(aud string) Option {
	return func(cfg *Config) { cfg.Audience = aud }
}

// WithAuthEndpoints wires OAuth endpoints from discovery.
func WithAuthEndpoints(a discovery.AuthEndpoints) Option {
	return func(cfg *Config) { cfg.Auth = a }
}

// WithIssuer records the deployment issuer on produced tokens.
func WithIssuer(iss string) Option {
	return func(cfg *Config) { cfg.Issuer = iss }
}

// WithIDTokenTrustedAudiences names the audiences, besides the client ID,
// that an ID token's aud claim may list when the source verifies it at the
// code exchange or on refresh. A token whose aud lists any other audience
// is refused, so without this option only the client ID is accepted. A
// later call replaces an earlier one.
func WithIDTokenTrustedAudiences(aud ...string) Option {
	trusted := slices.Clone(aud)
	return func(cfg *Config) { cfg.IDTokenTrustedAudiences = trusted }
}

// WithRefreshThreshold overrides proactive refresh window (default 30s).
func WithRefreshThreshold(d time.Duration) Option {
	return func(cfg *Config) { cfg.RefreshThreshold = d }
}

// WithTokenChange sets fn as the hook the source calls each time it holds
// new tokens: after every successful code exchange
// ([Source.ExchangeAuthorizationCode], and so
// [Source.CompleteAuthorization]) and every successful refresh. The
// [TokenChange] carries the new access token, the refresh token the source
// now holds and the token response. When [Source.Revoke] clears the
// tokens, fn sees the zero TokenChange once, in its place among the
// changes. Revoke itself reports it only after its revocation request has
// been sent or has failed; another call reporting changes may report it in
// its turn instead, before the request has ended or after Revoke has
// returned. A Revoke that finds no token reports nothing. fn is not called
// for a failed exchange or refresh, or for a refresh whose result the
// source discarded because a code exchange, [Source.SetTokens] or Revoke
// replaced or cleared the tokens meanwhile. SetTokens does not call fn
// either: the application already has its tokens. A nil fn sets no hook.
//
// An application that keeps a session across restarts stores the refresh
// token from here. RFC 6749 §6 has the client discard its old refresh token
// when the server issues a new one, and RFC 9700 §4.14 makes rotating it
// one of the two ways a public client's refresh token is protected.
//
// The source calls fn without holding its lock, so fn may call the
// source's methods. ctx is the context of the call that changed the tokens,
// which may have ended by the time fn runs; a hook that must finish a write
// regardless can use [context.WithoutCancel].
//
// fn sees the changes one at a time, in the order the source installed
// them. Callers that wait on a refresh another call started get the new
// token without waiting for fn, and that refresh is reported once.
// When the tokens change again while fn is running, from another goroutine
// or from fn itself through the source, the change is reported after fn
// returns, by the goroutine already running fn; the call that made the
// change returns without waiting for that. So fn must not block for long:
// later changes wait for it.
//
// If fn panics on the goroutine of a source call (the call that made the
// change, or a call reporting changes that others made), the panic goes up
// through that call. The changes still waiting are then reported from a
// new goroutine the source starts, so none of them waits for a later change
// of tokens. No caller could recover a panic on that goroutine, so there
// the source recovers it, drops the change fn panicked on, and goes on with
// the rest.
func WithTokenChange(fn func(ctx context.Context, change TokenChange)) Option {
	return func(cfg *Config) { cfg.tokenChange = fn }
}

// Source implements auth.TokenSource for SMART authorization-code + PKCE.
type Source struct {
	cfg Config
	// credClient sends the token, refresh and revocation requests, which
	// carry a credential: it never follows a redirect. cfg.HTTPClient,
	// which the caller owns, is left as it was.
	credClient *http.Client

	mu       sync.Mutex
	cur      auth.Token
	refresh  string
	lastTR   TokenResponse
	inflight *tokenExchange
	// idBinding is what a refreshed ID token must repeat from the last ID
	// token the source verified; nil until it has verified one.
	idBinding *idTokenBinding
	// forceRefresh makes the held access token count as stale until new
	// tokens replace it. Reauth sets it; setTokensLocked clears it.
	forceRefresh bool
	// session counts the times a code exchange or SetTokens installed
	// tokens or Revoke cleared them. A refresh records it when it starts
	// and discards its result when it has changed by the time the refresh
	// ends. A refresh does not advance it.
	session uint64
	// changes holds the token changes installed but not yet reported to
	// the token-change hook, oldest first; delivering reports that a
	// goroutine is reporting them. See deliverChanges.
	changes    []func()
	delivering bool
}

// queueChangeLocked records change for the token-change hook, with the
// context of the call that made it. The hook runs without s.mu, so it gets
// its own copy of the response's Raw map. The caller holds s.mu, has
// installed the change under it, and calls deliverChanges once it has
// released s.mu.
func (s *Source) queueChangeLocked(ctx context.Context, change TokenChange) {
	hook := s.cfg.tokenChange
	if hook == nil {
		return
	}
	change.Response.Raw = maps.Clone(change.Response.Raw)
	s.changes = append(s.changes, func() { hook(ctx, change) })
}

// deliverChanges reports the queued token changes to the hook, oldest
// first, without holding s.mu. When another goroutine is already reporting
// them, it returns at once and leaves its changes to that goroutine. So the
// hook never runs in two goroutines at once and sees the changes in the
// order they were installed, including a change the hook itself causes
// through the source, which it sees after it returns.
func (s *Source) deliverChanges() { s.reportChanges(false) }

// reportChanges is deliverChanges. handedOn reports that it runs on a
// goroutine the source started, not on the goroutine of a source call.
//
// On a caller's goroutine a panic of the hook goes on up to that caller, and
// the changes still queued are handed on to a new goroutine. On a handed-on
// goroutine no caller could recover a panic, and an unrecovered one would
// end the program, so a change whose hook panics there is dropped and the
// rest are reported.
func (s *Source) reportChanges(handedOn bool) {
	s.mu.Lock()
	if s.delivering {
		s.mu.Unlock()
		return
	}
	s.delivering = true
	locked := true
	defer func() {
		if locked {
			s.delivering = false
			s.mu.Unlock()
			return
		}
		// Unlocked here only when the hook panicked on a caller's
		// goroutine. The panic goes on up to that caller; the changes still
		// queued are reported from a new goroutine, which ends once the
		// queue is empty or finds another goroutine reporting.
		s.mu.Lock()
		s.delivering = false
		rest := len(s.changes) > 0
		s.mu.Unlock()
		if rest {
			go s.reportChanges(true)
		}
	}()
	for len(s.changes) > 0 {
		report := s.changes[0]
		s.changes[0] = nil
		s.changes = s.changes[1:]
		s.mu.Unlock()
		locked = false
		if handedOn {
			reportRecovering(report)
		} else {
			report()
		}
		s.mu.Lock()
		locked = true
	}
}

// reportRecovering runs report and drops a panic from it. It is for a
// goroutine the source started, where nobody could recover the panic.
func reportRecovering(report func()) {
	defer func() { _ = recover() }()
	report()
}

// setTokensLocked replaces the held tokens. New tokens are not the ones a
// Reauth asked to replace, so it also ends a forced refresh. The caller
// holds s.mu.
func (s *Source) setTokensLocked(access auth.Token, refresh string) {
	s.cur = access
	s.refresh = refresh
	s.forceRefresh = false
}

// withHeldScope returns prev, the session's last token response, with the
// scope of held, the access token the session held when a refresh began,
// standing in for an earlier scope that prev lacks. A source whose tokens
// SetTokens installed has had no token response yet, so the scope of the
// token it holds is the only grant it knows (RFC 6749 §6 reads an omitted
// scope as the original grant).
//
// Lacks is decided as keepSessionMembers decides it: prev has no earlier
// scope when its body had no scope member. A scope member prev carried, even
// as an empty string or null, stays, so the earlier response wins over held.
// A held token without a scope leaves prev as it is. The result has its own
// Raw map, so prev is not changed.
func withHeldScope(prev TokenResponse, held auth.Token) TokenResponse {
	if _, had := prev.Raw["scope"]; had || held.Scope == "" {
		return prev
	}
	raw := make(map[string]any, len(prev.Raw)+1)
	maps.Copy(raw, prev.Raw)
	raw["scope"] = held.Scope
	prev.Raw = raw
	prev.Scope = held.Scope
	return prev
}

// idTokenBinding holds the claims of a verified ID token that a later one
// in the same session must repeat (OpenID Connect Core 1.0 §12.2). The
// issuer is not kept: every ID token the source accepts is checked against
// its one configured issuer, so it cannot change.
type idTokenBinding struct {
	subject string
	// audience is the token's aud as a set: sorted, each value once.
	audience []string
}

// bindingOf returns the binding of verified claims, or nil for none. It
// keeps its own copy of the audience, so a caller changing the claims it
// was handed does not change the binding.
func bindingOf(c *IDTokenClaims) *idTokenBinding {
	if c == nil {
		return nil
	}
	return &idTokenBinding{subject: c.Subject, audience: audienceSet(c.Audience)}
}

// audienceSet returns a sorted copy of aud with each value once. RFC 7519
// §4.1.3 gives aud no order, so two tokens list the same audiences when
// their sets are equal.
func audienceSet(aud []string) []string {
	set := slices.Clone(aud)
	slices.Sort(set)
	return slices.Compact(set)
}

type tokenExchange struct {
	done  chan struct{}
	token auth.Token
	err   error
	// session is the session the refresh started in.
	session uint64
	// superseded reports that a code exchange, SetTokens or Revoke replaced
	// the session while the refresh ran, so its result was discarded and the
	// callers waiting on it try again. It is written before done is closed.
	superseded bool
}

// New constructs a Source from clientID and discovery auth endpoints.
//
// SMART requires the `aud` parameter on every authorization request, and
// New has no Platform base URL to default it from, so pass [WithAudience];
// [NewFromCatalog] fills the audience in from a resolved catalog. New
// applies the checks of [FromConfig], so it fails with
// [auth.ErrInvalidConfig] without an audience, or when the server's
// advertised PKCE methods leave out S256.
func New(clientID string, authEP discovery.AuthEndpoints, opts ...Option) (*Source, error) {
	cfg := Config{
		ClientID:         clientID,
		Auth:             authEP,
		RefreshThreshold: 30 * time.Second,
	}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	return FromConfig(cfg)
}

// FromConfig validates cfg and returns a Source.
//
// It fails with [auth.ErrInvalidConfig] when cfg has no HTTPClient, no
// ClientID, no token or authorization endpoint, or no Audience. SMART
// requires the `aud` parameter on the authorization request, and
// FromConfig does not guess one: set Audience to the Platform base URL or
// to an audience identifier the authorization server knows.
//
// The SDK always uses the S256 PKCE method. When cfg.Auth lists the
// server's supported methods (CodeChallengeMethodsSupported) and S256 is
// not among them, FromConfig fails with [auth.ErrInvalidConfig], since such
// a server cannot check the challenge. An empty list is accepted: a
// hand-built catalog often leaves it out.
//
// FromConfig also fails with [auth.ErrInvalidConfig] on client credentials
// that conflict or do not fit the server:
//   - both a client secret and a client assertion key;
//   - a nil signing key, an empty key ID, an unsupported algorithm, or a
//     key that does not suit the algorithm;
//   - an assertion algorithm that a non-empty
//     TokenEndpointAuthSigningAlgValuesSupported does not list;
//   - a client authentication method that a non-empty
//     TokenEndpointAuthMethodsSupported does not list.
//
// A JWKSURI it cannot build a key-set fetcher from fails the same way.
func FromConfig(cfg Config) (*Source, error) {
	if cfg.HTTPClient == nil {
		return nil, fmt.Errorf("%w: HTTPClient is required (REQ-021)", auth.ErrInvalidConfig)
	}
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("%w: ClientID is required", auth.ErrInvalidConfig)
	}
	if cfg.Auth.TokenEndpoint == nil {
		return nil, fmt.Errorf("%w: TokenEndpoint is required", auth.ErrInvalidConfig)
	}
	if cfg.Auth.AuthorizationEndpoint == nil {
		return nil, fmt.Errorf("%w: AuthorizationEndpoint is required", auth.ErrInvalidConfig)
	}
	if cfg.Audience == "" {
		return nil, fmt.Errorf("%w: Audience is required: SMART requires the aud authorization parameter (set WithAudience, or use NewFromCatalog with a catalog that has a BaseURL)", auth.ErrInvalidConfig)
	}
	// The SDK sends only S256 challenges, so a server that lists its PKCE
	// methods without S256 cannot verify them. An empty list says nothing.
	if advertised := cfg.Auth.CodeChallengeMethodsSupported; len(advertised) > 0 &&
		!slices.Contains(advertised, challengeMethod) {
		return nil, fmt.Errorf("%w: the server's code_challenge_methods_supported %q does not list %s, the only PKCE method the SDK sends",
			auth.ErrInvalidConfig, advertised, challengeMethod)
	}
	if cfg.RefreshThreshold == 0 {
		cfg.RefreshThreshold = 30 * time.Second
	}
	if err := configureClientAuth(&cfg); err != nil {
		return nil, err
	}
	if cfg.Auth.JWKSURI != nil && cfg.JWKS == nil {
		jwks, err := NewJWKS(cfg.HTTPClient, cfg.Auth.JWKSURI.String())
		if err != nil {
			return nil, err
		}
		cfg.JWKS = jwks
	}
	return &Source{cfg: cfg, credClient: noredirect.Client(cfg.HTTPClient)}, nil
}

// configureClientAuth resolves the confidential-client authentication method
// for the token endpoint (REQ-068). It rejects ambiguous configuration (both an
// assertion key and a client secret), builds the private_key_jwt assertion
// with jwtbearer.NewClientAssertion after checking its algorithm against the
// server's token_endpoint_auth_signing_alg_values_supported, and performs the
// G-3 discovery cross-check: when the authorization server advertises
// token_endpoint_auth_methods_supported, the method implied by the configured
// credential MUST be listed. An empty or absent list is not constraining.
func configureClientAuth(cfg *Config) error {
	hasSecret := cfg.ClientSecret != ""
	hasAssertion := cfg.clientAssertion != nil
	if hasSecret && hasAssertion {
		return fmt.Errorf("%w: configure either WithClientSecret or WithClientAssertionKey, not both", auth.ErrInvalidConfig)
	}

	var method string
	switch {
	case hasAssertion:
		method = methodPrivateKeyJWT
		alg := cmp.Or(cfg.clientAssertion.alg, defaultAssertionAlg)
		// The algorithm must be one the server accepts for client
		// assertions, when it says which. An empty list says nothing.
		if advertised := cfg.Auth.TokenEndpointAuthSigningAlgValuesSupported; len(advertised) > 0 &&
			!slices.Contains(advertised, alg) {
			return fmt.Errorf("%w: client assertion algorithm %q is not in the server's advertised token_endpoint_auth_signing_alg_values_supported %q",
				auth.ErrInvalidConfig, alg, advertised)
		}
		signer, err := jwtbearer.NewClientAssertion(cfg.ClientID, cfg.Auth.TokenEndpoint.String(),
			cfg.clientAssertion.signer, alg, cfg.clientAssertion.kid)
		if err != nil {
			return err
		}
		cfg.assertionSource = signer
	case hasSecret:
		// Default to client_secret_basic. When the server advertises methods
		// but not basic, fall back to client_secret_post if it is offered, so
		// a deployment that only accepts post-style credentials still works.
		method = methodClientSecretBasic
		if advertised := cfg.Auth.TokenEndpointAuthMethodsSupported; len(advertised) > 0 &&
			!slices.Contains(advertised, methodClientSecretBasic) &&
			slices.Contains(advertised, methodClientSecretPost) {
			method = methodClientSecretPost
		}
		cfg.secretAuthMethod = method
	default:
		// Public client — no client authentication to cross-check.
		return nil
	}

	// G-3: fail fast only when the server advertises methods and the
	// configured one is absent. Empty/absent list is not constraining.
	if advertised := cfg.Auth.TokenEndpointAuthMethodsSupported; len(advertised) > 0 &&
		!slices.Contains(advertised, method) {
		return fmt.Errorf("%w: configured client auth method %q is not in the server's advertised token_endpoint_auth_methods_supported %v",
			auth.ErrInvalidConfig, method, advertised)
	}
	return nil
}

// NewFromCatalog builds a Source from a resolved ServiceCatalog.
//
// It takes the endpoints from catalog.Auth and records catalog.Issuer, the
// OpenID Connect issuer, on the tokens it produces. SMART requires the
// `aud` authorization parameter, and NewFromCatalog sets it to
// catalog.BaseURL, the Platform base URL, unless opts include
// [WithAudience]. A catalog with an empty BaseURL gives no default, so the
// call then fails with [auth.ErrInvalidConfig] unless the caller sets one.
// A catalog whose code_challenge_methods_supported leaves out S256 fails
// with the same error; that check and the others are those of [FromConfig].
func NewFromCatalog(catalog *discovery.ServiceCatalog, clientID string, opts ...Option) (*Source, error) {
	if catalog == nil {
		return nil, fmt.Errorf("%w: catalog is nil", auth.ErrInvalidConfig)
	}
	// The defaults come first, so an option the caller passes wins.
	all := []Option{
		WithAuthEndpoints(catalog.Auth),
		WithIssuer(catalog.Issuer),
	}
	if catalog.BaseURL != "" {
		all = append(all, WithAudience(catalog.BaseURL))
	}
	all = append(all, opts...)
	return New(clientID, catalog.Auth, all...)
}

// AuthorizationRequest holds the values one launch carries from
// [Source.BeginAuthorization] to the redirect back to the app. Keep it, for
// example in the user's session, and pass it unchanged to
// [Source.AuthorizeURL] and then to [Source.CompleteAuthorization] or
// [Source.ExchangeAuthorizationCode].
type AuthorizationRequest struct {
	State string
	// Launch is the launch value an EHR passed to the app (see
	// [ParseEHRLaunch]), kept here so the request carries the whole
	// launch. BeginAuthorization leaves it empty; [Source.AuthorizeURL]
	// sends it when its own launch argument is empty.
	Launch string
	PKCE   PKCEPair
	// Issuer is the source's configured issuer, the OpenID Connect issuer
	// from discovery. When the redirect names an
	// issuer, [Source.CompleteAuthorization] requires it to equal this one,
	// so a response from another authorization server is refused.
	Issuer string
	// Nonce is the OpenID Connect nonce sent on the authorization request.
	// BeginAuthorization sets it when the configured scopes include openid
	// and leaves it empty otherwise. The ID token returned by the code
	// exchange has to carry the same value.
	Nonce string
}

// BeginAuthorization starts one launch: it generates the PKCE pair and
// records what the redirect back to the app will be checked against.
//
// If state is empty, a cryptographically random state value is generated
// (stateLen bytes of entropy, base64url-encoded) and returned in
// [AuthorizationRequest].State. If state is non-empty it is used
// verbatim, and the caller takes responsibility for its strength and
// session binding.
//
// The request also records the source's issuer, the OpenID Connect issuer
// from discovery, in [AuthorizationRequest].Issuer. When the configured
// scopes include openid, it carries a fresh random nonce (32 random bytes,
// base64url-encoded) in [AuthorizationRequest].Nonce; without openid the
// nonce is empty and none is sent.
//
// Callers must retain the returned [AuthorizationRequest] and pass it
// unchanged to [Source.CompleteAuthorization] (or
// [Source.ExchangeAuthorizationCode]), which compare the redirect against
// it. A Source supports many concurrent launches when each flow keeps its
// own request value.
func (s *Source) BeginAuthorization(state string) (AuthorizationRequest, error) {
	if state == "" {
		var err error
		state, err = randBase64URL(stateLen)
		if err != nil {
			return AuthorizationRequest{}, fmt.Errorf("smart: generate state: %w", err)
		}
	}
	pkce, err := NewPKCEPair()
	if err != nil {
		return AuthorizationRequest{}, err
	}
	req := AuthorizationRequest{State: state, PKCE: pkce, Issuer: s.cfg.Issuer}
	if hasScope(s.cfg.Scopes, auth.ScopeOpenID) {
		// OpenID Connect Core 1.0 §3.1.2.1: the nonce binds the ID token
		// to this launch, so a replayed token is refused.
		req.Nonce, err = randBase64URL(nonceLen)
		if err != nil {
			return AuthorizationRequest{}, fmt.Errorf("smart: generate nonce: %w", err)
		}
	}
	return req, nil
}

// hasScope reports whether scopes contain want. Each configured value may
// itself hold several scopes separated by spaces.
func hasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if slices.Contains(strings.Fields(s), want) {
			return true
		}
	}
	return false
}

// AuthorizeURL builds the SMART authorization redirect URL for req.
//
// launch is the launch value an EHR passed to the app (see
// [ParseEHRLaunch]). When it is empty, req.Launch is used instead; leave
// both empty for a standalone launch. When a launch value is set, the URL
// forwards
// it unchanged and the scope it sends includes launch, added when the
// configured scopes lack it. The URL sends req.Nonce as nonce when the
// request has one and the configured scopes include openid; without openid
// it sends none, even when the caller set req.Nonce.
func (s *Source) AuthorizeURL(req AuthorizationRequest, launch string) (string, error) {
	if req.State == "" || req.PKCE.Verifier == "" {
		return "", fmt.Errorf("%w: call BeginAuthorization first or supply State and PKCE", auth.ErrInvalidConfig)
	}
	u := *s.cfg.Auth.AuthorizationEndpoint
	// Start from the endpoint's own query, which RFC 6749 §3.1 says must be
	// kept, and set each SDK parameter over it so none appears twice.
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", s.cfg.ClientID)
	q.Set("redirect_uri", s.cfg.RedirectURI)
	q.Set("code_challenge", req.PKCE.Challenge)
	q.Set("code_challenge_method", challengeMethod)
	q.Set("state", req.State)
	if launch == "" {
		launch = req.Launch
	}
	scopes := s.cfg.Scopes
	if launch != "" && !hasScope(scopes, auth.ScopeLaunch) {
		// HL7 SMART App Launch: an app launched from an EHR asks for the
		// launch scope. Concat copies, so the configuration stays as it was.
		scopes = slices.Concat(scopes, []string{auth.ScopeLaunch})
	}
	if len(scopes) > 0 {
		q.Set("scope", strings.Join(scopes, " "))
	}
	// FromConfig refuses a source without an audience, so aud is always set.
	q.Set("aud", s.cfg.Audience)
	if launch != "" {
		q.Set("launch", launch)
	}
	if req.Nonce != "" && hasScope(s.cfg.Scopes, auth.ScopeOpenID) {
		q.Set("nonce", req.Nonce)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// ExchangeAuthorizationCode completes the PKCE flow: it trades the
// authorization code for tokens at the token endpoint. Most apps call
// [Source.CompleteAuthorization] instead, which also checks the issuer and
// error parameters of the redirect before calling this.
//
// req must be the [AuthorizationRequest] returned by
// [Source.BeginAuthorization] for this launch. callbackState is the state
// query parameter received at the redirect URI; it is compared against
// req.State and [ErrLaunchInvalidState] is returned on mismatch before any
// network call is made, defending against CSRF.
//
// When the token response carries an ID token, ExchangeAuthorizationCode
// verifies it before returning:
//   - the signature against the source's JWKS, using only the algorithms
//     the server lists in id_token_signing_alg_values_supported when it
//     lists any;
//   - the issuer against the source's issuer;
//   - the audience against the client ID and [WithIDTokenTrustedAudiences];
//   - the nonce against req.Nonce.
//
// Any failure there is an [*auth.ExchangeError] matching
// [auth.ErrTokenExchangeFailed] that also matches its cause: a token that
// fails its checks matches [auth.ErrJWKSValidationFailed], a source without
// a JWKS matches [auth.ErrInvalidConfig], and a key set that cannot be
// fetched keeps its fetch error. An unverified ID token is never returned.
// The verified claims are in [TokenResponse].IDTokenClaims. When the call
// fails, the source keeps the tokens it held before.
//
// A successful exchange starts a new session: the source holds the new
// access token and the response's refresh token, or none when the response
// has none, never a refresh token from an earlier session. A refresh still
// running from the earlier session has its result discarded.
//
// The returned [TokenResponse] also carries the SMART launch parameters
// for smart/.
func (s *Source) ExchangeAuthorizationCode(ctx context.Context, code string, callbackState string, req AuthorizationRequest) (auth.Token, TokenResponse, error) {
	if req.State == "" || req.PKCE.Verifier == "" {
		return auth.Token{}, TokenResponse{}, fmt.Errorf("%w: AuthorizationRequest from BeginAuthorization is required", auth.ErrInvalidConfig)
	}
	if callbackState != req.State {
		return auth.Token{}, TokenResponse{}, ErrLaunchInvalidState
	}
	tok, tr, refresh, err := s.exchangeCode(ctx, code, req.PKCE.Verifier)
	if err != nil {
		return auth.Token{}, TokenResponse{}, err
	}
	if tr.IDToken != "" {
		// OpenID Connect Core 1.0 §3.1.3.7: the ID token is checked before
		// anything from this response is kept or returned.
		claims, err := s.verifyIDToken(ctx, tr.IDToken, req.Nonce)
		if err != nil {
			return auth.Token{}, TokenResponse{}, &auth.ExchangeError{Sentinel: auth.ErrTokenExchangeFailed, Inner: fmt.Errorf("id_token: %w", err)}
		}
		tr.IDTokenClaims = claims
	}
	s.mu.Lock()
	s.session++
	s.setTokensLocked(tok, refresh)
	s.lastTR = tr
	// A new authorization starts a new session: without an ID token there
	// is nothing to bind a refresh to.
	s.idBinding = bindingOf(tr.IDTokenClaims)
	s.queueChangeLocked(ctx, TokenChange{Access: tok, RefreshToken: refresh, Response: tr})
	s.mu.Unlock()
	s.deliverChanges()
	return tok, tr, nil
}

// verifyIDToken checks an ID token from the token endpoint against the
// source's JWKS, issuer, client ID, trusted audiences and the server's
// advertised signing algorithms. nonce is the launch's nonce at
// the code exchange, and empty on refresh, where none is checked.
func (s *Source) verifyIDToken(ctx context.Context, raw, nonce string) (*IDTokenClaims, error) {
	return ValidateIDToken(ctx, raw, s.cfg.JWKS, s.cfg.Issuer, s.cfg.ClientID, nonce, time.Time{},
		s.cfg.Auth.IDTokenSigningAlgValuesSupported, WithTrustedAudiences(s.cfg.IDTokenTrustedAudiences...))
}

// LastTokenResponse returns SMART fields from the most recent successful
// token-endpoint call (authorization_code or refresh_token). After
// [Source.Token] refreshes, callers that need an updated [LaunchContext]
// should re-run smart.LaunchContextFromTokenResponse with this value.
//
// A refresh response without an ID token keeps the session's identity: its
// IDTokenClaims are the verified claims the session had before, so a
// launch context rebuilt from it still names the same user.
//
// Likewise, a refresh response that leaves out the scope or a
// launch-context parameter keeps the value the earlier response had, in its
// typed field (fhirContext has none) and in Raw. The launch-context
// parameters are patient, encounter, ehrId, episodeId, fhirContext, intent,
// need_patient_banner, smart_style_url and tenant. Left out means
// the member is absent from the response body: a member the refresh
// response carries replaces the earlier value, even when it is an empty
// string or null. Raw's other members are the refresh response's own.
//
// After [Source.Revoke] it is the zero value until a code exchange or a
// refresh succeeds.
func (s *Source) LastTokenResponse() TokenResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastTR
}

// SetTokens seeds access and optional refresh tokens (testing / token import).
// The new tokens end a refresh that [Source.Reauth] forced, and a refresh
// already running when SetTokens is called has its result discarded.
//
// SetTokens does not start a new session: it keeps the identity of the last
// ID token the source verified, and [Source.LastTokenResponse]. A later
// refresh whose ID token names another user is then refused, so import
// tokens for a different user into a new Source.
//
// SetTokens does not call the [WithTokenChange] hook: the application
// already has the tokens it passes.
func (s *Source) SetTokens(access auth.Token, refresh string) {
	s.mu.Lock()
	s.session++
	s.setTokensLocked(access, refresh)
	s.mu.Unlock()
}

// Token returns a valid access token, refreshing when near expiry.
//
// A refresh that started before [Source.ExchangeAuthorizationCode] or
// [Source.SetTokens] replaced the held tokens, or before [Source.Revoke]
// cleared them, has its result discarded, success or failure. Token then
// answers from the tokens the source holds now: it returns them, refreshing
// them in turn when they are stale, or, after Revoke, returns
// [auth.ErrReauthRequired].
func (s *Source) Token(ctx context.Context) (auth.Token, error) {
	for {
		tok, retry, err := s.tryToken(ctx)
		if !retry {
			return tok, err
		}
		// A code exchange, SetTokens or Revoke replaced the session the
		// refresh belonged to; read what the source holds now. A further
		// retry needs yet another replacement meanwhile.
	}
}

// tryToken makes one attempt at Token. retry reports that the refresh it led
// or waited on was discarded because the session changed, so the caller
// tries again against the current tokens.
func (s *Source) tryToken(ctx context.Context) (tok auth.Token, retry bool, err error) {
	if err := ctx.Err(); err != nil {
		return auth.Token{}, false, err
	}
	s.mu.Lock()
	if !s.staleLocked() {
		t := s.cur
		s.mu.Unlock()
		return t, false, nil
	}
	if s.inflight != nil {
		ex := s.inflight
		s.mu.Unlock()
		select {
		case <-ex.done:
			if ex.superseded {
				return auth.Token{}, true, nil
			}
			return ex.token, false, ex.err
		case <-ctx.Done():
			return auth.Token{}, false, ctx.Err()
		}
	}
	refreshTok := s.refresh
	cur := s.cur
	if refreshTok == "" && cur.IsZero() {
		// Post-terminal state: both the access token and the refresh token have
		// been cleared by a prior terminal failure (F-L). Return immediately
		// without touching inflight — there is nothing to exchange (REQ-063).
		s.mu.Unlock()
		return auth.Token{}, false, &auth.ExchangeError{Sentinel: auth.ErrReauthRequired, Inner: errors.New("no token or refresh_token")}
	}
	if refreshTok == "" && !cur.IsZero() {
		s.mu.Unlock()
		// No refresh_token, and the cached token is stale (within the proactive
		// refresh threshold). If it is past ExpiresAt it MUST NOT be returned
		// silently (REQ-063) — there is nothing to refresh with, so signal
		// re-authentication. A still-valid token (near expiry but not yet past
		// it) is returned as-is without claiming inflight (REQ-026).
		if !cur.ExpiresAt.IsZero() && time.Until(cur.ExpiresAt) <= 0 {
			return auth.Token{}, false, &auth.ExchangeError{Sentinel: auth.ErrReauthRequired, Inner: errors.New("access token expired and no refresh_token")}
		}
		return cur, false, nil
	}
	ex := &tokenExchange{done: make(chan struct{}), session: s.session}
	// The binding is read with the session it belongs to, not after the
	// network call, when another session's may have replaced it.
	binding := s.idBinding
	s.inflight = ex
	s.mu.Unlock()

	var refreshedTR TokenResponse
	tok, refreshedTR, refreshTok, err = s.refreshGrant(ctx, refreshTok, binding)

	s.mu.Lock()
	if s.session != ex.session {
		// A code exchange, SetTokens or Revoke replaced the session while the
		// refresh ran. Its result, success or failure, belongs to the old
		// session: nothing of it is kept, and no terminal failure clears the
		// new tokens. Every caller on this refresh tries again.
		if s.inflight == ex {
			s.inflight = nil
		}
		s.mu.Unlock()
		ex.superseded = true
		close(ex.done)
		return auth.Token{}, true, nil
	}
	if err == nil {
		if refreshedTR.IDToken == "" {
			// OpenID Connect Core 1.0 §12.2 lets a refresh leave the ID
			// token out; the session keeps the identity it verified.
			refreshedTR.IDTokenClaims = s.lastTR.IDTokenClaims
		}
		// Nor does the session lose the launch context or the scope a
		// refresh response leaves out, and the new access token carries the
		// scope the session keeps. A session with no earlier scope on its
		// last token response keeps the scope of the access token it held
		// when the refresh began.
		s.lastTR = keepSessionMembers(withHeldScope(s.lastTR, cur), refreshedTR)
		tok.Scope = s.lastTR.Scope
		s.setTokensLocked(tok, refreshTok)
		if refreshedTR.IDTokenClaims != nil {
			s.idBinding = bindingOf(refreshedTR.IDTokenClaims)
		}
		s.queueChangeLocked(ctx, TokenChange{Access: tok, RefreshToken: refreshTok, Response: s.lastTR})
	} else {
		// F-L: on a terminal failure clear the refresh token and the cached
		// access token so that subsequent Token() calls deterministically
		// return ErrReauthRequired without issuing another doomed POST.
		// On transient failures (5xx, network, ctx) retain both so callers
		// may retry (REQ-063).
		ex2, ok := errors.AsType[*auth.ExchangeError](err)
		if ok && ex2.Terminal() {
			s.setTokensLocked(auth.Token{}, "")
			err = &auth.ExchangeError{Sentinel: auth.ErrReauthRequired, Inner: err}
		}
	}
	s.inflight = nil
	s.mu.Unlock()
	ex.token = tok
	ex.err = err
	close(ex.done)
	// The waiters have their token; the change is reported after them, in
	// the order it was installed whatever the hook does meanwhile.
	s.deliverChanges()
	return tok, false, err
}

// RefreshIfNeeded refreshes the access token only when it is within the
// configured threshold (i.e. stale) and a refresh token is present. It is a
// no-op returning nil when the current token is still fresh. On failure it
// returns the same error contract as Token.
func (s *Source) RefreshIfNeeded(ctx context.Context) error {
	s.mu.Lock()
	if !s.staleLocked() || s.refresh == "" {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	_, err := s.Token(ctx)
	return err
}

// Reauth forces the cached access token to be treated as stale and drives a
// refresh on the next token acquisition, even when the token has not yet crossed
// the proactive-refresh threshold. Use it to recover from a wire 401. If a
// refresh is already in flight (e.g. concurrent 401 recovery), Reauth coalesces
// onto it rather than issuing a duplicate request. On a
// terminal refresh failure it clears the refresh token and returns
// ErrReauthRequired.
//
// Any other failed refresh (a server or network error, or an ID token the
// source refuses) keeps both the access token and the refresh token, and the
// access token stays stale, so the next [Source.Token] call tries the
// refresh again. New tokens, from a refresh, a code exchange or
// [Source.SetTokens], end the forced refresh.
//
// When no refresh_token is available there is nothing to exchange, so Reauth
// does not discard the cached access token (a wire 401 may be scope-related
// rather than an expiry, and a public client has no other credential to fall
// back on). It returns ErrReauthRequired, clearing the cached token only when
// it is already past ExpiresAt, because an expired token is never returned
// silently.
func (s *Source) Reauth(ctx context.Context) error {
	s.mu.Lock()
	if s.refresh == "" {
		expired := !s.cur.IsZero() && !s.cur.ExpiresAt.IsZero() && time.Until(s.cur.ExpiresAt) <= 0
		if expired {
			s.cur = auth.Token{}
		}
		s.mu.Unlock()
		return &auth.ExchangeError{Sentinel: auth.ErrReauthRequired, Inner: errors.New("no refresh_token; re-authentication required")}
	}
	// A refresh_token is available: mark the current token stale so the next
	// Token() executes the refresh even if it has not yet crossed the
	// proactive-refresh threshold. The token itself is kept, so a failed
	// refresh leaves the source as it was.
	s.forceRefresh = true
	s.mu.Unlock()
	_, err := s.Token(ctx)
	return err
}

func (s *Source) staleLocked() bool {
	if s.forceRefresh || s.cur.IsZero() {
		return true
	}
	if s.cur.ExpiresAt.IsZero() {
		return false
	}
	return time.Until(s.cur.ExpiresAt) <= s.cfg.RefreshThreshold
}

func (s *Source) exchangeCode(ctx context.Context, code, verifier string) (auth.Token, TokenResponse, string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {s.cfg.RedirectURI},
		"code_verifier": {verifier},
	}
	return s.postToken(ctx, form)
}

// refreshGrant redeems refresh at the token endpoint. An ID token in the
// response is verified before anything is returned; a failure is a
// refresh failure that is not terminal, so the caller keeps its tokens.
//
// binding is the identity of the session the refresh belongs to, read when
// the refresh started; nil when that session has verified no ID token.
func (s *Source) refreshGrant(ctx context.Context, refresh string, binding *idTokenBinding) (auth.Token, TokenResponse, string, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refresh},
	}
	tok, tr, next, err := s.postToken(ctx, form)
	if err != nil {
		return auth.Token{}, TokenResponse{}, "", err
	}
	if next == "" {
		// RFC 6749 §6: the server may keep the refresh token it was sent.
		next = refresh
	}
	if tr.IDToken == "" {
		return tok, tr, next, nil
	}
	claims, err := s.verifyRefreshedIDToken(ctx, tr.IDToken, binding)
	if err != nil {
		return auth.Token{}, TokenResponse{}, "", &auth.ExchangeError{Sentinel: auth.ErrRefreshFailed, Inner: fmt.Errorf("id_token: %w", err)}
	}
	tr.IDTokenClaims = claims
	return tok, tr, next, nil
}

// verifyRefreshedIDToken verifies an ID token from a refresh response like
// one from the code exchange, without a nonce, and, when prev holds the
// session's earlier verified ID token, requires the same subject and
// audience (OpenID Connect Core 1.0 §12.2).
func (s *Source) verifyRefreshedIDToken(ctx context.Context, raw string, prev *idTokenBinding) (*IDTokenClaims, error) {
	claims, err := s.verifyIDToken(ctx, raw, "")
	if err != nil {
		return nil, err
	}
	if prev != nil && (claims.Subject != prev.subject || !slices.Equal(audienceSet(claims.Audience), prev.audience)) {
		return nil, fmt.Errorf("%w: the refreshed ID token names another subject or audience than the session's", auth.ErrJWKSValidationFailed)
	}
	return claims, nil
}

// maxResponseBody is how much of a token- or revocation-endpoint response
// the source reads.
const maxResponseBody = 1 << 20

// clientRequest builds the form POST to endpoint, a token or revocation
// endpoint, and adds to it the client authentication both take, so the two
// cannot differ. A failure to sign a client assertion is wrapped; the caller
// gives every error its own sentinel.
func (s *Source) clientRequest(ctx context.Context, endpoint string, form url.Values) (*http.Request, error) {
	// Client authentication is selected deterministically (REQ-068):
	//   - assertion signer configured → private_key_jwt (signed client_assertion)
	//   - else client secret set      → client_secret_basic (HTTP Basic) or
	//     client_secret_post (credentials in the form body), per the method
	//     resolved by configureClientAuth from the server's advertised methods
	//   - else                        → public client (no client auth)
	// Only a public client and client_secret_post put client_id in the form:
	// a confidential client is identified by its credential (REQ-068).
	useBasic := false
	if s.cfg.assertionSource != nil {
		assertion, err := s.cfg.assertionSource.Assertion(ctx)
		if err != nil {
			return nil, fmt.Errorf("client_assertion signing: %w", err)
		}
		form.Set("client_assertion_type", clientAssertionType)
		form.Set("client_assertion", assertion)
	} else if s.cfg.ClientSecret != "" {
		if s.cfg.secretAuthMethod == methodClientSecretPost {
			form.Set("client_id", s.cfg.ClientID)
			form.Set("client_secret", s.cfg.ClientSecret)
		} else {
			useBasic = true
		}
	} else {
		form.Set("client_id", s.cfg.ClientID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if useBasic {
		// RFC 6749 §2.3.1: client_id and client_secret are form-encoded
		// (Appendix B) before use as the Basic username and password.
		req.SetBasicAuth(url.QueryEscape(s.cfg.ClientID), url.QueryEscape(s.cfg.ClientSecret))
	}
	return req, nil
}

func (s *Source) postToken(ctx context.Context, form url.Values) (auth.Token, TokenResponse, string, error) {
	req, err := s.clientRequest(ctx, s.cfg.Auth.TokenEndpoint.String(), form)
	if err != nil {
		return auth.Token{}, TokenResponse{}, "", &auth.ExchangeError{Sentinel: auth.ErrTokenExchangeFailed, Inner: err}
	}
	resp, err := s.credClient.Do(req)
	if err != nil {
		return auth.Token{}, TokenResponse{}, "", &auth.ExchangeError{Sentinel: auth.ErrTokenExchangeFailed, Inner: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return auth.Token{}, TokenResponse{}, "", &auth.ExchangeError{Sentinel: auth.ErrTokenExchangeFailed, StatusCode: resp.StatusCode, Inner: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		sentinel := auth.ErrTokenExchangeFailed
		if form.Get("grant_type") == "refresh_token" {
			sentinel = auth.ErrRefreshFailed
		}
		return auth.Token{}, TokenResponse{}, "", &auth.ExchangeError{
			Sentinel:   sentinel,
			StatusCode: resp.StatusCode,
			OAuth2:     auth.ParseOAuth2Error(body),
			Inner:      fmt.Errorf("token endpoint returned %d", resp.StatusCode),
		}
	}
	parsed, err := ParseTokenResponse(body)
	if err != nil {
		return auth.Token{}, TokenResponse{}, "", &auth.ExchangeError{Sentinel: auth.ErrTokenExchangeFailed, StatusCode: resp.StatusCode, Inner: err}
	}
	if parsed.AccessToken == "" {
		return auth.Token{}, TokenResponse{}, "", &auth.ExchangeError{Sentinel: auth.ErrTokenExchangeFailed, StatusCode: resp.StatusCode, Inner: errors.New("empty access_token")}
	}
	tok := tokenFromResponse(parsed, s.cfg.Issuer)
	return tok, parsed, parsed.RefreshToken, nil
}

// JWKS returns the JWKS helper when configured.
func (s *Source) JWKS() *JWKS { return s.cfg.JWKS }
