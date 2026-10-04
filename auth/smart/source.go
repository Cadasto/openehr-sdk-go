package smart

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/jwtbearer"
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
	// nonceLen is the number of random bytes in an OpenID Connect nonce
	// (REQ-061), the same 256 bits as the state.
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
}

// Option mutates Config during construction.
type Option func(*Config)

// WithHTTPClient injects the client for token and JWKS calls.
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
// in place of an HTTP Basic header. alg is the JOSE algorithm (RS384 default
// per SMART; RS256/ES256/ES384 also supported by jwtbearer.ClaimsSigner); kid,
// when set, is emitted as the JWS "kid" header. Mutually exclusive with
// WithClientSecret; configuring both is rejected at construction.
// signer must be non-nil; a nil signer is rejected at construction with
// [auth.ErrInvalidConfig].
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

// WithRefreshThreshold overrides proactive refresh window (default 30s).
func WithRefreshThreshold(d time.Duration) Option {
	return func(cfg *Config) { cfg.RefreshThreshold = d }
}

// Source implements auth.TokenSource for SMART authorization-code + PKCE.
type Source struct {
	cfg Config

	mu       sync.Mutex
	cur      auth.Token
	refresh  string
	lastTR   TokenResponse
	inflight *tokenExchange
}

type tokenExchange struct {
	done  chan struct{}
	token auth.Token
	err   error
}

// New constructs a Source from clientID and discovery auth endpoints.
//
// SMART requires the `aud` parameter on every authorization request, and
// New has no Platform base URL to default it from, so pass [WithAudience];
// without it New fails with [auth.ErrInvalidConfig]. [NewFromCatalog]
// fills the audience in from a resolved catalog. A server whose advertised
// PKCE methods leave out S256 is refused the same way. The other checks are
// those of [FromConfig].
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
// that conflict or do not fit the server: both a client secret and a
// client assertion key; a nil signing key, an unsupported algorithm, or a
// key that does not suit the algorithm; or a client authentication method
// that a non-empty TokenEndpointAuthMethodsSupported does not list. A
// JWKSURI it cannot build a key-set fetcher from fails the same way.
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
	return &Source{cfg: cfg}, nil
}

// configureClientAuth resolves the confidential-client authentication method
// for the token endpoint (REQ-068). It rejects ambiguous configuration (both an
// assertion key and a client secret), builds the jwtbearer.ClaimsSigner for
// private_key_jwt, and performs the G-3 discovery cross-check: when the
// authorization server advertises token_endpoint_auth_methods_supported, the
// method implied by the configured credential MUST be listed; an empty/absent
// list is not constraining (skip).
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
		signer, err := jwtbearer.NewClaimsSigner(
			jwtbearer.ClaimsTemplate{
				Issuer:   cfg.ClientID,
				Subject:  cfg.ClientID,
				Audience: cfg.Auth.TokenEndpoint.String(),
			},
			cfg.clientAssertion.signer,
			jwtbearer.WithAlgorithm(cfg.clientAssertion.alg),
			jwtbearer.WithKeyID(cfg.clientAssertion.kid),
		)
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
// A catalog whose code_challenge_methods_supported leaves out S256 is
// refused with the same error. The other checks are those of [FromConfig].
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
	State  string
	Launch string
	PKCE   PKCEPair
	// Issuer is the issuer the source is bound to: its configured issuer,
	// the OpenID Connect issuer from discovery. When the redirect names an
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
		// to this launch, so a replayed token is refused (REQ-061).
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
// [ParseEHRLaunch]), or empty for a standalone launch. When it is set, the
// URL forwards it unchanged and the scope it sends includes launch, added
// when the configured scopes lack it. The URL sends req.Nonce as nonce when
// the request has one.
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
	if req.Nonce != "" {
		q.Set("nonce", req.Nonce)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// ExchangeAuthorizationCode completes the PKCE flow. req must be
// the [AuthorizationRequest] returned by [Source.BeginAuthorization] for this
// launch. callbackState is the state query parameter received at the redirect
// URI; it is compared against req.State and [ErrLaunchInvalidState] is
// returned on mismatch before any network call is made, defending against
// CSRF. The returned [TokenResponse] carries SMART launch parameters for
// smart/.
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
	s.mu.Lock()
	s.cur = tok
	s.refresh = refresh
	s.lastTR = tr
	s.mu.Unlock()
	return tok, tr, nil
}

// LastTokenResponse returns SMART fields from the most recent successful
// token-endpoint call (authorization_code or refresh_token). After
// [Source.Token] refreshes, callers that need an updated [LaunchContext]
// should re-run smart.LaunchContextFromTokenResponse with this value.
func (s *Source) LastTokenResponse() TokenResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastTR
}

// SetTokens seeds access and optional refresh tokens (testing / token import).
func (s *Source) SetTokens(access auth.Token, refresh string) {
	s.mu.Lock()
	s.cur = access
	s.refresh = refresh
	s.mu.Unlock()
}

// Token returns a valid access token, refreshing when near expiry.
func (s *Source) Token(ctx context.Context) (auth.Token, error) {
	if err := ctx.Err(); err != nil {
		return auth.Token{}, err
	}
	s.mu.Lock()
	if !s.staleLocked() {
		t := s.cur
		s.mu.Unlock()
		return t, nil
	}
	if s.inflight != nil {
		ex := s.inflight
		s.mu.Unlock()
		select {
		case <-ex.done:
			return ex.token, ex.err
		case <-ctx.Done():
			return auth.Token{}, ctx.Err()
		}
	}
	refreshTok := s.refresh
	cur := s.cur
	if refreshTok == "" && cur.IsZero() {
		// Post-terminal state: both the access token and the refresh token have
		// been cleared by a prior terminal failure (F-L). Return immediately
		// without touching inflight — there is nothing to exchange (REQ-063).
		s.mu.Unlock()
		return auth.Token{}, &auth.ExchangeError{Sentinel: auth.ErrReauthRequired, Inner: errors.New("no token or refresh_token")}
	}
	if refreshTok == "" && !cur.IsZero() {
		s.mu.Unlock()
		// No refresh_token, and the cached token is stale (within the proactive
		// refresh threshold). If it is past ExpiresAt it MUST NOT be returned
		// silently (REQ-063) — there is nothing to refresh with, so signal
		// re-authentication. A still-valid token (near expiry but not yet past
		// it) is returned as-is without claiming inflight (REQ-026).
		if !cur.ExpiresAt.IsZero() && time.Until(cur.ExpiresAt) <= 0 {
			return auth.Token{}, &auth.ExchangeError{Sentinel: auth.ErrReauthRequired, Inner: errors.New("access token expired and no refresh_token")}
		}
		return cur, nil
	}
	ex := &tokenExchange{done: make(chan struct{})}
	s.inflight = ex
	s.mu.Unlock()

	var tok auth.Token
	var err error
	var refreshedTR TokenResponse
	tok, refreshedTR, refreshTok, err = s.refreshGrant(ctx, refreshTok)

	s.mu.Lock()
	if err == nil {
		s.cur = tok
		s.refresh = refreshTok
		if refreshedTR.AccessToken != "" {
			s.lastTR = refreshedTR
		}
	} else {
		// F-L: on a terminal failure clear the refresh token and the cached
		// access token so that subsequent Token() calls deterministically
		// return ErrReauthRequired without issuing another doomed POST.
		// On transient failures (5xx, network, ctx) retain both so callers
		// may retry (REQ-063).
		ex2, ok := errors.AsType[*auth.ExchangeError](err)
		if ok && ex2.Terminal() {
			s.refresh = ""
			s.cur = auth.Token{}
			err = &auth.ExchangeError{Sentinel: auth.ErrReauthRequired, Inner: err}
		}
	}
	s.inflight = nil
	s.mu.Unlock()
	ex.token = tok
	ex.err = err
	close(ex.done)
	return tok, err
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
	// proactive-refresh threshold.
	s.cur = auth.Token{}
	s.mu.Unlock()
	_, err := s.Token(ctx)
	return err
}

func (s *Source) staleLocked() bool {
	if s.cur.IsZero() {
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
		"client_id":     {s.cfg.ClientID},
		"code_verifier": {verifier},
	}
	return s.postToken(ctx, form)
}

func (s *Source) refreshGrant(ctx context.Context, refresh string) (auth.Token, TokenResponse, string, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refresh},
		"client_id":     {s.cfg.ClientID},
	}
	return s.postToken(ctx, form)
}

func (s *Source) postToken(ctx context.Context, form url.Values) (auth.Token, TokenResponse, string, error) {
	// Client authentication is selected deterministically (REQ-068):
	//   - assertion signer configured → private_key_jwt (signed client_assertion)
	//   - else client secret set      → client_secret_basic (HTTP Basic) or
	//     client_secret_post (credentials in the form body), per the method
	//     resolved by configureClientAuth from the server's advertised methods
	//   - else                        → public client (no client auth)
	useBasic := false
	if s.cfg.assertionSource != nil {
		assertion, err := s.cfg.assertionSource.Assertion(ctx)
		if err != nil {
			return auth.Token{}, TokenResponse{}, "", &auth.ExchangeError{
				Sentinel: auth.ErrTokenExchangeFailed,
				Inner:    fmt.Errorf("client_assertion signing: %w", err),
			}
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
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.Auth.TokenEndpoint.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return auth.Token{}, TokenResponse{}, "", &auth.ExchangeError{Sentinel: auth.ErrTokenExchangeFailed, Inner: err}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if useBasic {
		// RFC 6749 §2.3.1: client_id and client_secret are form-encoded
		// (Appendix B) before use as the Basic username and password.
		req.SetBasicAuth(url.QueryEscape(s.cfg.ClientID), url.QueryEscape(s.cfg.ClientSecret))
	}
	resp, err := s.cfg.HTTPClient.Do(req)
	if err != nil {
		return auth.Token{}, TokenResponse{}, "", &auth.ExchangeError{Sentinel: auth.ErrTokenExchangeFailed, Inner: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
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
	refresh := parsed.RefreshToken
	if refresh == "" {
		// Keep prior refresh when the server omits a new one.
		s.mu.Lock()
		if s.refresh != "" {
			refresh = s.refresh
		}
		s.mu.Unlock()
	}
	return tok, parsed, refresh, nil
}

// JWKS returns the JWKS helper when configured.
func (s *Source) JWKS() *JWKS { return s.cfg.JWKS }
