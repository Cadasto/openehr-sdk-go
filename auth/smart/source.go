package smart

import (
	"cmp"
	"context"
	"crypto"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
	"github.com/cadasto/openehr-sdk-go/internal/urlredact"
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
	// defaultRefreshThreshold is how long before its expiry the held access
	// token counts as stale when the configuration sets no threshold.
	defaultRefreshThreshold = 30 * time.Second
	// refreshTimeout is how long a refresh request may run. The request does
	// not end with the context of the call that started it, so that a
	// refresh token the server has rotated is installed and reported; this
	// bounds it instead.
	refreshTimeout = 30 * time.Second
	// firstBackoff is how long the source holds back the next refresh after
	// the first transient failure in a row, while it serves the held access
	// token. Each further failure doubles it.
	firstBackoff = time.Second
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
//
// [FromConfig] keeps its own copy of Scopes, IDTokenTrustedAudiences and
// Auth, with the URLs and lists Auth points to, so a change the caller makes
// to them afterwards does not reach the source.
//
// Printed with fmt or logged with log/slog, a Config shows its ClientSecret
// and the private key [WithClientAssertionKey] sets as "[redacted]": see
// [Config.String] and [Config.LogValue].
type Config struct {
	// HTTPClient is the injected client used for token and JWKS calls. The
	// token, refresh and revocation requests never follow a redirect,
	// whatever CheckRedirect the client has, and the client is not modified.
	// Those requests use a copy made when the source is built, so a
	// Transport, Timeout or Jar set on the client afterwards does not reach
	// them. Required.
	HTTPClient   *http.Client
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       []string
	Audience     string
	Auth         discovery.AuthEndpoints
	Issuer       string
	// RefreshThreshold is how long before its expiry the held access token
	// counts as stale, so that [Source.Token] refreshes it ahead of time. 0
	// selects the default of 30 seconds. A negative value counts as 0: the
	// token is refreshed once it has expired, so an expired token is never
	// returned.
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

// String describes c with its ClientSecret replaced by "[redacted]", or left
// empty when it is empty, and the key [WithClientAssertionKey] sets shown by
// its algorithm and key ID only. Auth is shown by its authorization and token
// endpoints, with the password of one that carries one masked. HTTPClient,
// JWKS and the token-change hook are left out; the other fields are shown as
// they are.
func (c Config) String() string {
	return fmt.Sprintf("smart.Config{ClientID: %q, ClientSecret: %q, ClientAssertion: %s, RedirectURI: %q, Scopes: %q, Audience: %q, "+
		"AuthorizationEndpoint: %q, TokenEndpoint: %q, Issuer: %q, RefreshThreshold: %s, IDTokenTrustedAudiences: %q}",
		c.ClientID, redact(c.ClientSecret), c.clientAssertion.text(), c.RedirectURI, c.Scopes, c.Audience,
		redactedURL(c.Auth.AuthorizationEndpoint), redactedURL(c.Auth.TokenEndpoint), c.Issuer, c.RefreshThreshold, c.IDTokenTrustedAudiences)
}

// GoString returns the text of [Config.String], so that printing c in Go
// syntax shows no credential either.
func (c Config) GoString() string { return c.String() }

// Format writes the text of [Config.String] for every fmt verb it is called
// for, whatever the flags and width. fmt answers %T and %p without calling
// it, and %p of a Config value prints its fields, so never print a Config
// with %p.
func (c Config) Format(f fmt.State, _ rune) {
	if f == nil {
		return
	}
	_, _ = io.WriteString(f, c.String())
}

// LogValue lets log/slog log c as a group of the fields [Config.String]
// shows, keyed by field name, with the credentials and endpoints replaced as
// String replaces them.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("ClientID", c.ClientID),
		slog.String("ClientSecret", redact(c.ClientSecret)),
		c.clientAssertion.logAttr(),
		slog.String("RedirectURI", c.RedirectURI),
		slog.Any("Scopes", c.Scopes),
		slog.String("Audience", c.Audience),
		slog.String("AuthorizationEndpoint", redactedURL(c.Auth.AuthorizationEndpoint)),
		slog.String("TokenEndpoint", redactedURL(c.Auth.TokenEndpoint)),
		slog.String("Issuer", c.Issuer),
		slog.Duration("RefreshThreshold", c.RefreshThreshold),
		slog.Any("IDTokenTrustedAudiences", c.IDTokenTrustedAudiences),
	)
}

// text describes k by its algorithm and key ID, with the signer shown as
// "[redacted]". A nil k gives "<nil>".
func (k *clientAssertionKey) text() string {
	if k == nil {
		return "<nil>"
	}
	signer := "<nil>"
	if k.signer != nil {
		signer = redacted
	}
	return fmt.Sprintf("{Signer: %s, Alg: %q, KeyID: %q}", signer, k.alg, k.kid)
}

// logAttr is the ClientAssertion attribute of [Config.LogValue]: a group of
// the values text shows, or nil for a nil k.
func (k *clientAssertionKey) logAttr() slog.Attr {
	if k == nil {
		return slog.Any("ClientAssertion", nil)
	}
	signer := slog.Any("Signer", nil)
	if k.signer != nil {
		signer = slog.String("Signer", redacted)
	}
	return slog.Attr{Key: "ClientAssertion", Value: slog.GroupValue(signer, slog.String("Alg", k.alg), slog.String("KeyID", k.kid))}
}

// redactedURL returns u with the password in its userinfo masked, or "" for a
// nil u.
func redactedURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	return u.Redacted()
}

// TokenChange is what the source reports to the [WithTokenChange] hook when
// its tokens change. [Source.Revoke] reports the zero TokenChange.
//
// Printed with fmt or logged with log/slog, a TokenChange shows its tokens as
// "[redacted]": see [TokenChange.String]. Its JSON encoding is unchanged and
// carries the tokens, so an application can store it.
type TokenChange struct {
	// Access is the access token the source now holds.
	Access auth.Token
	// RefreshToken is the refresh token the source now holds, which is the
	// response's. When the response carried none, it is empty after a code
	// exchange and, after a refresh, the one the source held before.
	RefreshToken string
	// Response is the token response as [Source.LastTokenResponse] returns
	// it, so a refresh response's left-out launch context is filled in. It
	// is the hook's own copy, as deep as LastTokenResponse's: changing it,
	// down to a value nested inside Raw, the IDTokenClaims or
	// NeedPatientBanner, does not reach the source.
	Response TokenResponse
}

// String describes c by the text of [auth.Token.String] for Access and of
// [TokenResponse.String] for Response, with RefreshToken replaced by
// "[redacted]", or left empty when it is empty.
func (c TokenChange) String() string {
	return fmt.Sprintf("smart.TokenChange{Access: %s, RefreshToken: %q, Response: %s}",
		c.Access.String(), redact(c.RefreshToken), c.Response.String())
}

// GoString returns the text of [TokenChange.String], so that printing c in Go
// syntax shows no token either.
func (c TokenChange) GoString() string { return c.String() }

// Format writes the text of [TokenChange.String] for every fmt verb it is
// called for, whatever the flags and width. fmt answers %T and %p without
// calling it, and %p of a TokenChange value prints its fields, so never print
// a TokenChange with %p.
func (c TokenChange) Format(f fmt.State, _ rune) {
	if f == nil {
		return
	}
	_, _ = io.WriteString(f, c.String())
}

// LogValue lets log/slog log c as a group of Access, RefreshToken and
// Response, with the tokens replaced as [TokenChange.String] replaces them.
func (c TokenChange) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Attr{Key: "Access", Value: c.Access.LogValue()},
		slog.String("RefreshToken", redact(c.RefreshToken)),
		slog.Attr{Key: "Response", Value: c.Response.LogValue()},
	)
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

// WithScopes sets the space-separated scope request (slice joined). The
// option keeps its own copy of scopes, so a later change to the caller's
// slice does not reach the source.
func WithScopes(scopes ...string) Option {
	own := slices.Clone(scopes)
	return func(cfg *Config) { cfg.Scopes = own }
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

// WithAuthEndpoints wires OAuth endpoints from discovery. The option keeps
// its own copy of a, with the URLs and lists it points to, so a later change
// to them does not reach the source.
func WithAuthEndpoints(a discovery.AuthEndpoints) Option {
	own := a.Clone()
	return func(cfg *Config) { cfg.Auth = own }
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

// WithRefreshThreshold sets how long before its expiry the held access token
// counts as stale, so that [Source.Token] refreshes it ahead of time. The
// default is 30 seconds, which a d of 0 also selects. A negative d counts as
// 0: the token is refreshed once it has expired, so an expired token is
// never returned.
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
// for a refresh the call that started it, and it may have ended by the time
// fn runs: a refresh goes on after that call gives up, and is then reported
// from the refresh's own goroutine. A hook that must finish a write
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
// of tokens. No caller could recover a panic on that goroutine, or on the
// goroutine of a refresh whose starting call has returned, so there the
// source recovers it, drops the change fn panicked on, and goes on with the
// rest.
func WithTokenChange(fn func(ctx context.Context, change TokenChange)) Option {
	return func(cfg *Config) { cfg.tokenChange = fn }
}

// Source implements auth.TokenSource for SMART authorization-code + PKCE.
//
// The zero value is not usable: create a Source with [New], [FromConfig]
// or [NewFromCatalog].
//
// Printing a Source with any fmt verb shows only its client ID and issuer,
// never its client secret, tokens or token response: see [Source.String].
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
	// retryAt is when Token may send a refresh again after a transient
	// failure left the held access token to be served, never after its
	// ExpiresAt; zero when no backoff holds. backoff is how long the next
	// such failure holds the refresh back. setTokensLocked resets both.
	retryAt time.Time
	backoff time.Duration
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

// String describes s by two values fixed when s was built: its client ID
// and its issuer. It never shows the client secret, a token or a token
// response, and it reads nothing a refresh changes, so it is safe to call
// while other goroutines use s. A nil s gives "<nil>".
func (s *Source) String() string {
	if s == nil {
		return "<nil>"
	}
	return fmt.Sprintf("smart.Source{ClientID: %q, Issuer: %q}", s.cfg.ClientID, s.cfg.Issuer)
}

// GoString returns the text of [Source.String], so that printing s in Go
// syntax shows no credential either.
func (s *Source) GoString() string { return s.String() }

// Format writes the text of [Source.String] for every fmt verb, whatever its
// flags and width. Without it, a verb such as %d or %x would print the
// fields of s, which hold the client secret and the tokens.
func (s *Source) Format(f fmt.State, _ rune) {
	if f == nil {
		return
	}
	_, _ = io.WriteString(f, s.String())
}

// queueChangeLocked records change for the token-change hook, with the
// context of the call that made it. The hook runs without s.mu, so it gets
// its own copy of the response, which shares nothing with the source's. The
// caller holds s.mu, has installed the change under it, and calls
// deliverChanges once it has released s.mu.
func (s *Source) queueChangeLocked(ctx context.Context, change TokenChange) {
	hook := s.cfg.tokenChange
	if hook == nil {
		return
	}
	change.Response = cloneTokenResponse(change.Response)
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
// Reauth asked to replace, so it also ends a forced refresh, and they end
// the backoff of the tokens they replace. The caller holds s.mu.
func (s *Source) setTokensLocked(access auth.Token, refresh string) {
	s.cur = access
	s.refresh = refresh
	s.forceRefresh = false
	s.retryAt = time.Time{}
	s.backoff = 0
}

// backOffLocked records a refresh that failed transiently. While the held
// access token is not yet past its ExpiresAt, and Reauth has not marked it
// for refresh, it holds Token's next refresh back until the backoff ends,
// never later than that ExpiresAt, doubles the backoff for the next failure
// in a row, and reports true: the token is to be served in place of the
// failure. The caller holds s.mu.
func (s *Source) backOffLocked() bool {
	now := time.Now()
	left := s.cur.ExpiresAt.Sub(now)
	if s.forceRefresh || left <= 0 {
		return false
	}
	wait := min(cmp.Or(s.backoff, firstBackoff), left)
	s.retryAt = now.Add(wait)
	s.backoff = 2 * wait
	return true
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
	// held is the access token the refresh's callers get in place of err,
	// except a forced caller (Reauth, RefreshIfNeeded), when serveHeld
	// reports that the refresh failed transiently while held was still
	// valid. Both are written before done is closed.
	held      auth.Token
	serveHeld bool
	// retry reports that the refresh has no result for the callers waiting
	// on it, because it panicked. The refresh does not end with the context
	// of the call that started it, so that call giving up does not set it. A
	// waiter whose own context is live tries again; one whose context has
	// ended returns its own context's error. It is written before done is
	// closed.
	retry bool

	// The fields below are guarded by Source.mu. They decide which of the
	// call that started the refresh and the refresh's own goroutine reports
	// the change to the token-change hook or raises the refresh's panic
	// again, so that exactly one of them does.

	// ended reports that the refresh's goroutine has published its outcome.
	ended bool
	// starterLeft reports that the call that started the refresh returned
	// because its context ended first.
	starterLeft bool
	// raise reports that the refresh panicked while the call that started
	// it was waiting, which then panics again with panicValue.
	raise      bool
	panicValue any
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
	// FromConfig fills in the defaults, such as the refresh threshold.
	cfg := Config{
		ClientID: clientID,
		Auth:     authEP,
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
		// REQ-021: the caller supplies the client.
		return nil, fmt.Errorf("%w: HTTPClient is required", auth.ErrInvalidConfig)
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
	// The source keeps its own copies, so a later change to the caller's
	// slices or URLs does not reach it.
	cfg.Scopes = slices.Clone(cfg.Scopes)
	cfg.IDTokenTrustedAudiences = slices.Clone(cfg.IDTokenTrustedAudiences)
	cfg.Auth = cfg.Auth.Clone()
	// The SDK sends only S256 challenges, so a server that lists its PKCE
	// methods without S256 cannot verify them. An empty list says nothing.
	if advertised := cfg.Auth.CodeChallengeMethodsSupported; len(advertised) > 0 &&
		!slices.Contains(advertised, challengeMethod) {
		return nil, fmt.Errorf("%w: the server's code_challenge_methods_supported %q does not list %s, the only PKCE method the SDK sends",
			auth.ErrInvalidConfig, advertised, challengeMethod)
	}
	cfg.RefreshThreshold = cmp.Or(cfg.RefreshThreshold, defaultRefreshThreshold)
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
// discovery cross-check (auth.md § Method cross-check against discovery): when
// the authorization server advertises token_endpoint_auth_methods_supported,
// the method implied by the configured credential MUST be listed. An empty or
// absent list is not constraining.
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

	// The discovery cross-check: fail fast only when the server advertises
	// methods and the configured one is absent. Empty/absent list is not
	// constraining.
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
	// New takes the endpoints from catalog.Auth. The defaults come first, so
	// an option the caller passes wins.
	all := []Option{WithIssuer(catalog.Issuer)}
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
//
// Printed with fmt or logged with log/slog, an AuthorizationRequest shows its
// PKCE verifier as "[redacted]": see [AuthorizationRequest.String]. Its JSON
// encoding is unchanged and carries the verifier, so it can be kept in a
// session store.
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

// String describes r with its PKCE shown by the text of [PKCEPair.String],
// which replaces the verifier by "[redacted]". The other fields are shown as
// they are.
func (r AuthorizationRequest) String() string {
	return fmt.Sprintf("smart.AuthorizationRequest{State: %q, Launch: %q, PKCE: %s, Issuer: %q, Nonce: %q}",
		r.State, r.Launch, r.PKCE.String(), r.Issuer, r.Nonce)
}

// GoString returns the text of [AuthorizationRequest.String], so that
// printing r in Go syntax shows no verifier either.
func (r AuthorizationRequest) GoString() string { return r.String() }

// Format writes the text of [AuthorizationRequest.String] for every fmt verb
// it is called for, whatever the flags and width. fmt answers %T and %p
// without calling it, and %p of an AuthorizationRequest value prints its
// fields, so never print an AuthorizationRequest with %p.
func (r AuthorizationRequest) Format(f fmt.State, _ rune) {
	if f == nil {
		return
	}
	_, _ = io.WriteString(f, r.String())
}

// LogValue lets log/slog log r as a group of its fields, keyed by field name,
// with the PKCE verifier replaced as [AuthorizationRequest.String] replaces
// it.
func (r AuthorizationRequest) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("State", r.State),
		slog.String("Launch", r.Launch),
		slog.Attr{Key: "PKCE", Value: r.PKCE.LogValue()},
		slog.String("Issuer", r.Issuer),
		slog.String("Nonce", r.Nonce),
	)
}

// BeginAuthorization starts one launch: it generates the PKCE pair and
// records what the redirect back to the app will be checked against.
//
// If state is empty, a cryptographically random state value is generated
// (32 random bytes, base64url-encoded) and returned in
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
		state = randBase64URL(stateLen)
	}
	pkce, err := NewPKCEPair()
	if err != nil {
		return AuthorizationRequest{}, err
	}
	req := AuthorizationRequest{State: state, PKCE: pkce, Issuer: s.cfg.Issuer}
	if hasScope(s.cfg.Scopes, auth.ScopeOpenID) {
		// OpenID Connect Core 1.0 §3.1.2.1: the nonce binds the ID token
		// to this launch, so a replayed token is refused.
		req.Nonce = randBase64URL(nonceLen)
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
// for smart/. It is the caller's own: the source keeps a copy of it that
// shares no map, slice or pointer with it.
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
	// The source keeps its own copy, so the caller may change the tr it is
	// returned.
	s.lastTR = cloneTokenResponse(tr)
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
// When the earlier response has no scope member either, as for a session
// whose tokens [Source.SetTokens] installed, the scope of the access token
// the source held when the refresh began stands in, in Scope and in
// Raw["scope"], although no server sent it.
//
// After [Source.Revoke] it is the zero value until a code exchange or a
// refresh succeeds.
//
// The result is the caller's own copy: it shares no map, slice or pointer
// with the source, down to the values nested inside Raw, the IDTokenClaims
// and NeedPatientBanner, so changing it changes neither the source nor what
// the source returns afterwards.
func (s *Source) LastTokenResponse() TokenResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneTokenResponse(s.lastTR)
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
//
// A later refresh response that leaves out the scope, on a session whose
// last token response has no scope member, gives the refreshed token and
// [Source.LastTokenResponse] the scope of access.
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
//
// Concurrent calls share one refresh. It runs on a goroutine of its own,
// with the values of the context of the call that started it but not its
// cancellation or deadline, and for at most 30 seconds, so that a refresh
// token the server has rotated is installed, and reported to the
// [WithTokenChange] hook, even when that call gives up. A call whose context
// ends first returns that context's error at once; the refresh goes on, and
// the calls waiting on it get its result. A panic in the refresh, from the
// HTTP client's RoundTripper or a client-assertion signer for example, goes
// up through the call that started it, unchanged, while that call waits, and
// the calls waiting on the refresh try again: each whose own context is live
// sends a new refresh or waits on a newer one, and one whose context has
// ended returns its own context's error. When the call that started the
// refresh has already returned, the panic is raised again on the refresh's
// goroutine once the waiting calls are released, so it ends the program as
// any unrecovered panic in a goroutine does. A refresh whose goroutine exits
// through [runtime.Goexit] instead, from a RoundTripper that calls t.FailNow
// for example, got no answer: it ends as a refresh failure that is not
// terminal, as below, with an error matching [auth.ErrRefreshFailed], and
// its callers are not released to try again.
//
// A refresh that fails without being terminal (a server or network error, a
// timeout, an answer the source cannot read, or an ID token it refuses)
// keeps both tokens. While the held access token is not yet past its
// ExpiresAt, Token then returns that token without an error, and sends no
// new refresh until a backoff ends: one second after the first such
// failure, twice as long after each further failure in a row, and never
// later than that ExpiresAt. Until then Token returns the held token without
// a request. A successful refresh, a code exchange, [Source.SetTokens] or
// [Source.Revoke] ends the backoff. Past ExpiresAt the failure is returned,
// matching [auth.ErrRefreshFailed], and each call tries the refresh again. A
// token that [Source.Reauth] marked for refresh is not served in place of a
// failure. The calls waiting on a refresh get the same outcome, the held
// token included; [Source.RefreshIfNeeded] and Reauth get the failure.
func (s *Source) Token(ctx context.Context) (auth.Token, error) {
	return s.token(ctx, false)
}

// token is Token. forced reports a call by Reauth or RefreshIfNeeded, which
// refreshes during a backoff and gets the failure of a refresh that Token
// would answer with the held token.
func (s *Source) token(ctx context.Context, forced bool) (auth.Token, error) {
	for {
		tok, retry, err := s.tryToken(ctx, forced)
		if !retry {
			return tok, err
		}
		// The refresh this call led or waited on left it no result: a code
		// exchange, SetTokens or Revoke replaced the session the refresh
		// belonged to, or the refresh panicked. Read what the source holds
		// now. A further retry needs another such event meanwhile: a new
		// session, or a panic in a newer refresh, which goes up through the
		// call that started that refresh or ends the program. A refresh
		// whose goroutine exits through runtime.Goexit publishes a failure
		// instead of a retry, so the loop does not spin.
	}
}

// tryToken makes one attempt at Token. retry reports that the refresh it led
// or waited on left it no result, because the session changed or because
// the refresh panicked, so the caller tries again against the current
// tokens.
func (s *Source) tryToken(ctx context.Context, forced bool) (tok auth.Token, retry bool, err error) {
	if err := ctx.Err(); err != nil {
		return auth.Token{}, false, err
	}
	s.mu.Lock()
	if !s.staleLocked() || (!forced && !s.forceRefresh && time.Now().Before(s.retryAt)) {
		// The token is fresh, or a transient failure left it to be served
		// until the backoff ends, which is no later than its ExpiresAt.
		t := s.cur
		s.mu.Unlock()
		return t, false, nil
	}
	if s.inflight != nil {
		ex := s.inflight
		s.mu.Unlock()
		select {
		case <-ex.done:
		case <-ctx.Done():
			return auth.Token{}, false, ctx.Err()
		}
		switch {
		case ex.superseded:
			return auth.Token{}, true, nil
		case ex.retry:
			// The refresh panicked: that outcome is not this caller's. Give
			// up with this caller's own error when its context ended,
			// otherwise go round again.
			if err := ctx.Err(); err != nil {
				return auth.Token{}, false, err
			}
			return auth.Token{}, true, nil
		default:
			tok, err := ex.outcome(forced)
			return tok, false, err
		}
	}
	refreshTok := s.refresh
	cur := s.cur
	if refreshTok == "" && cur.IsZero() {
		// The source holds neither an access token nor a refresh token: it is
		// new, Revoke signed it out, or a terminal refresh failure cleared
		// both. Return immediately without touching inflight — there is
		// nothing to exchange (REQ-063).
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

	return s.leadRefresh(ctx, ex, refreshTok, cur, binding, forced)
}

// outcome returns what a caller of the refresh ex stands for gets: the held
// access token in place of a transient failure that left it to be served,
// unless forced reports a call by Reauth or RefreshIfNeeded, and otherwise
// the refresh's own token and error.
func (ex *tokenExchange) outcome(forced bool) (auth.Token, error) {
	if ex.serveHeld && !forced {
		return ex.held, nil
	}
	return ex.token, ex.err
}

// leadRefresh starts the refresh ex stands for on a goroutine of its own and
// waits for it on behalf of the call that started it. refreshTok is the
// refresh token to redeem, and cur and binding are the access token and the
// identity the session held when the refresh began.
//
// When ctx ends first, leadRefresh returns ctx's error at once and leaves the
// refresh to finish on its goroutine, which then reports the change to the
// token-change hook itself. Otherwise the change is reported here, after
// the waiters have their token, so a hook that panics does so in this call.
// A panic in the refresh while this call waits is raised here again.
func (s *Source) leadRefresh(ctx context.Context, ex *tokenExchange, refreshTok string, cur auth.Token, binding *idTokenBinding, forced bool) (auth.Token, bool, error) {
	go s.runRefresh(ctx, ex, refreshTok, cur, binding)
	select {
	case <-ex.done:
	case <-ctx.Done():
		s.mu.Lock()
		ended := ex.ended
		ex.starterLeft = !ended
		s.mu.Unlock()
		if !ended {
			return auth.Token{}, false, ctx.Err()
		}
		// The refresh published its outcome first, so it is this call's to
		// report or raise; done closes right after.
		<-ex.done
	}
	if ex.raise {
		panic(ex.panicValue)
	}
	if ex.superseded || ex.retry {
		return auth.Token{}, true, nil
	}
	// The waiters have their token; the change is reported after them, in
	// the order it was installed whatever the hook does meanwhile.
	s.deliverChanges()
	tok, err := ex.outcome(forced)
	return tok, false, err
}

// runRefresh runs the refresh ex stands for, installs its result and
// publishes it to the callers waiting on it. The request keeps the values of
// ctx, the context of the call that started the refresh, but not its
// cancellation or deadline, and is cut off after refreshTimeout.
//
// A panic in the refresh is recovered here, and every waiter is released to
// try again. The call that started the refresh raises it again when it is
// still waiting; when it has returned, nobody could recover the panic, so it
// is raised again on this goroutine, which ends the program. A refresh
// whose goroutine exits through runtime.Goexit instead ends as a refresh
// failure that is not terminal (see endExitedRefresh).
func (s *Source) runRefresh(ctx context.Context, ex *tokenExchange, refreshTok string, cur auth.Token, binding *idTokenBinding) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), refreshTimeout)
	defer cancel()
	finished := false
	defer func() {
		if finished {
			return
		}
		if v := recover(); v != nil {
			s.endPanickedRefresh(ex, v)
			return
		}
		// Not finished, yet nothing to recover: the goroutine is exiting
		// through runtime.Goexit.
		s.endExitedRefresh(ctx, ex, cur)
	}()

	tok, refreshedTR, next, err := s.refreshGrant(rctx, refreshTok, binding)
	report := s.installRefresh(ctx, ex, cur, tok, refreshedTR, next, err)
	finished = true
	// The result is published before done is closed, so every waiter reads
	// it (auth.md § Concurrency).
	close(ex.done)
	if report {
		// The call that started the refresh has returned, so this goroutine
		// reports the change. No caller could recover a panic of the hook
		// here, so reportChanges drops it.
		s.reportChanges(true)
	}
}

// endPanickedRefresh ends the refresh ex stands for after it panicked with
// v, which is not nil: it takes ex out of flight and releases its waiters
// to try again. It hands v to the call that started the refresh when that
// call is still waiting, and otherwise panics with v itself.
func (s *Source) endPanickedRefresh(ex *tokenExchange, v any) {
	s.mu.Lock()
	if s.inflight == ex {
		s.inflight = nil
	}
	ex.retry = true
	ex.ended = true
	raiseHere := ex.starterLeft
	if !raiseHere {
		ex.raise = true
		ex.panicValue = v
	}
	s.mu.Unlock()
	close(ex.done)
	if raiseHere {
		panic(v)
	}
}

// endExitedRefresh ends the refresh ex stands for when its goroutine exits
// through runtime.Goexit, as one whose RoundTripper calls t.FailNow does.
// The refresh got no answer, so it ends as a refresh failure that is not
// terminal: installRefresh publishes an error matching
// [auth.ErrRefreshFailed], or the held token where such a failure leaves it
// to be served, and the backoff holds the next refresh back. Releasing the
// callers to try again instead would start a new refresh that may exit the
// same way, without end. cur and ctx are as for installRefresh.
func (s *Source) endExitedRefresh(ctx context.Context, ex *tokenExchange, cur auth.Token) {
	err := &auth.ExchangeError{Sentinel: auth.ErrRefreshFailed, Inner: errors.New("the refresh goroutine exited before the token endpoint answered")}
	// A failure queues no token change, so there is nothing to report
	// whether or not the call that started the refresh has returned.
	s.installRefresh(ctx, ex, cur, auth.Token{}, TokenResponse{}, "", err)
	close(ex.done)
}

// installRefresh installs what the refresh ex stands for returned: tok,
// refreshedTR and refreshTok, or err, and publishes on ex the token and error
// the callers of the refresh get. cur is the access token the session held
// when the refresh began, and ctx the context of the call that started the
// refresh, which the token-change hook gets. When a code exchange, SetTokens
// or Revoke replaced the session meanwhile, it installs nothing and marks ex
// superseded. It takes ex out of flight, unless a newer refresh has replaced
// it there, and reports whether the caller is to report the change to the
// hook because the call that started the refresh has returned. The caller
// closes ex.done next. It holds s.mu while it runs.
func (s *Source) installRefresh(ctx context.Context, ex *tokenExchange, cur, tok auth.Token, refreshedTR TokenResponse, refreshTok string, err error) (report bool) {
	s.mu.Lock()
	// Deferred, so that a panic here leaves s.mu free for endPanickedRefresh.
	defer s.mu.Unlock()
	tok, err = s.installLocked(ctx, ex, cur, tok, refreshedTR, refreshTok, err)
	ex.token = tok
	ex.err = err
	if s.inflight == ex {
		s.inflight = nil
	}
	ex.ended = true
	return ex.starterLeft
}

// installLocked is installRefresh's install step. It returns the token and
// error the callers of the refresh get. The caller holds s.mu.
func (s *Source) installLocked(ctx context.Context, ex *tokenExchange, cur, tok auth.Token, refreshedTR TokenResponse, refreshTok string, err error) (auth.Token, error) {
	if s.session != ex.session {
		// A code exchange, SetTokens or Revoke replaced the session while the
		// refresh ran. Its result, success or failure, belongs to the old
		// session: nothing of it is kept, and no terminal failure clears the
		// new tokens. Every caller on this refresh tries again.
		ex.superseded = true
		return auth.Token{}, nil
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
	} else if exErr, ok := errors.AsType[*auth.ExchangeError](err); ok && exErr.Terminal() {
		// On a terminal failure clear the refresh token and the cached access
		// token so that subsequent Token() calls deterministically return
		// ErrReauthRequired without issuing another doomed POST (REQ-063).
		s.setTokensLocked(auth.Token{}, "")
		err = &auth.ExchangeError{Sentinel: auth.ErrReauthRequired, Inner: err}
	} else if s.backOffLocked() {
		// A transient failure (5xx, network, timeout) keeps both tokens. The
		// held access token is still valid, so it is served in place of the
		// failure until the backoff ends (REQ-063).
		ex.held = s.cur
		ex.serveHeld = true
	}
	return tok, err
}

// RefreshIfNeeded refreshes the access token only when it is within the
// configured threshold (i.e. stale) and a refresh token is present. It is a
// no-op returning nil when the current token is still fresh. It refreshes
// even while a backoff after a transient failure holds [Source.Token] back,
// and returns the refresh's failure where Token would return the held token;
// its errors are otherwise those of Token.
func (s *Source) RefreshIfNeeded(ctx context.Context) error {
	s.mu.Lock()
	if !s.staleLocked() || s.refresh == "" {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	_, err := s.token(ctx, true)
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
// source refuses) is returned and keeps both the access token and the
// refresh token. Reauth refreshes even while a backoff after an earlier
// transient failure holds [Source.Token] back. The access token stays stale
// and is not served in place of a failure, so the next Token call tries the
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
	_, err := s.token(ctx, true)
	return err
}

func (s *Source) staleLocked() bool {
	if s.forceRefresh || s.cur.IsZero() {
		return true
	}
	if s.cur.ExpiresAt.IsZero() {
		return false
	}
	// A negative threshold counts as 0, so a token past its expiry is never
	// served (REQ-063).
	return time.Until(s.cur.ExpiresAt) <= max(s.cfg.RefreshThreshold, 0)
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
		// url.Parse's error quotes the endpoint, which may carry a password.
		return nil, urlredact.Error(err, endpoint)
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

// postToken redeems form at the token endpoint. Every failure is an
// *auth.ExchangeError of the grant's own class: auth.ErrRefreshFailed for a
// refresh_token grant, auth.ErrTokenExchangeFailed for any other. That holds
// whether the request cannot be built or sent, or its answer is an error or
// carries no token.
func (s *Source) postToken(ctx context.Context, form url.Values) (auth.Token, TokenResponse, string, error) {
	sentinel := auth.ErrTokenExchangeFailed
	if form.Get("grant_type") == "refresh_token" {
		sentinel = auth.ErrRefreshFailed
	}
	req, err := s.clientRequest(ctx, s.cfg.Auth.TokenEndpoint.String(), form)
	if err != nil {
		return auth.Token{}, TokenResponse{}, "", &auth.ExchangeError{Sentinel: sentinel, Inner: err}
	}
	resp, err := s.credClient.Do(req)
	if err != nil {
		return auth.Token{}, TokenResponse{}, "", &auth.ExchangeError{Sentinel: sentinel, Inner: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return auth.Token{}, TokenResponse{}, "", &auth.ExchangeError{Sentinel: sentinel, StatusCode: resp.StatusCode, Inner: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return auth.Token{}, TokenResponse{}, "", &auth.ExchangeError{
			Sentinel:   sentinel,
			StatusCode: resp.StatusCode,
			OAuth2:     auth.ParseOAuth2Error(body),
			Inner:      fmt.Errorf("token endpoint returned %d", resp.StatusCode),
		}
	}
	parsed, err := ParseTokenResponse(body)
	if err != nil {
		return auth.Token{}, TokenResponse{}, "", &auth.ExchangeError{Sentinel: sentinel, StatusCode: resp.StatusCode, Inner: err}
	}
	if parsed.AccessToken == "" {
		return auth.Token{}, TokenResponse{}, "", &auth.ExchangeError{Sentinel: sentinel, StatusCode: resp.StatusCode, Inner: errors.New("empty access_token")}
	}
	tok := tokenFromResponse(parsed, s.cfg.Issuer)
	return tok, parsed, parsed.RefreshToken, nil
}

// JWKS returns the JWKS helper when configured.
func (s *Source) JWKS() *JWKS { return s.cfg.JWKS }
