package clientcreds

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/jwtbearer"
	"github.com/cadasto/openehr-sdk-go/internal/coalesce"
	"github.com/cadasto/openehr-sdk-go/internal/expiresin"
	"github.com/cadasto/openehr-sdk-go/internal/noredirect"
	"github.com/cadasto/openehr-sdk-go/internal/urlredact"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// grantClientCredentials is the grant_type value of this grant, as sent to
// the token endpoint and as listed in a server's grant_types_supported.
const grantClientCredentials = "client_credentials"

// defaultRefreshThreshold is how long before its expiry the cached access
// token counts as stale when the configuration sets no threshold.
const defaultRefreshThreshold = 30 * time.Second

// firstBackoff is how long the source holds back the next exchange after the
// first transient failure in a row, while it serves the cached access token.
// Each further failure doubles it.
const firstBackoff = time.Second

// Client authentication method names, as listed in a server's
// token_endpoint_auth_methods_supported (RFC 8414).
const (
	methodClientSecretBasic = "client_secret_basic"
	methodClientSecretPost  = "client_secret_post"
	methodPrivateKeyJWT     = "private_key_jwt"
)

// AuthMethod selects how the client authenticates to the token
// endpoint when exchanging the grant.
type AuthMethod int

const (
	// AuthBasic sends client_id and client_secret as HTTP Basic auth
	// per RFC 6749 §2.3.1 ("client_secret_basic"). This is the
	// default, and the method RFC 6749 requires authorization servers
	// to support.
	AuthBasic AuthMethod = iota
	// AuthPost sends client_id and client_secret in the form-encoded
	// request body ("client_secret_post"). Use when the deployment
	// rejects Basic auth (some legacy authorization servers).
	AuthPost
	// AuthPrivateKeyJWT uses a signed JWT client_assertion (RFC 7523 /
	// SMART Backend Services). FromConfig sets this automatically when
	// WithClientAssertion is used, so the field reflects the actual
	// auth method in use. Setting AuthPrivateKeyJWT manually without
	// also calling WithClientAssertion is an error: FromConfig will
	// return auth.ErrInvalidConfig.
	AuthPrivateKeyJWT
)

// Config carries the constructor inputs. Use New(...Option) for the
// idiomatic call site; Config is exposed for declarative configuration
// (env, YAML).
//
// Printed with fmt or logged with log/slog, a Config shows its ClientSecret
// and ClientAssertion as "[redacted]": see [Config.String] and
// [Config.LogValue].
type Config struct {
	// HTTPClient is the injected client used for token-endpoint calls.
	// The token request never follows a redirect, whatever CheckRedirect
	// the client has, and the client is not modified. The request uses a
	// copy made when the source is built, so a Transport, Timeout or Jar set
	// on the client afterwards does not reach it. Required.
	HTTPClient *http.Client
	// TokenURL is the token endpoint of the authorization server.
	TokenURL string
	// ClientID identifies the registered confidential client.
	ClientID string
	// ClientSecret is the shared secret. Required unless ClientAssertion is set.
	// Mutually exclusive with ClientAssertion.
	ClientSecret string
	// Scope is the space-separated scope to request; empty omits the
	// scope parameter (deployment-defined default scope applies).
	Scope string
	// Audience optionally sets the `audience` parameter for
	// authorization servers that require explicit audience binding.
	Audience string
	// AuthMethod selects HTTP Basic vs form-body credentialing.
	// Ignored when ClientAssertion is set (AuthPrivateKeyJWT is used instead).
	AuthMethod AuthMethod
	// ClientAssertion, when set, enables SMART Backend Services asymmetric
	// client authentication (RFC 7523). The source is called once per token
	// exchange to produce a freshly signed JWT. Mutually exclusive with
	// ClientSecret. A nil *jwtbearer.ClaimsSigner is refused, as it can
	// never sign.
	ClientAssertion jwtbearer.AssertionSource
	// RefreshThreshold is how long before its expiry the cached access token
	// counts as stale, so that [Source.Token] exchanges the grant again ahead
	// of time. 0 selects the default of 30 seconds. A negative value counts
	// as 0: the token is replaced once it has expired, so an expired token is
	// never returned.
	RefreshThreshold time.Duration
	// Issuer is the issuer URL recorded on the produced Token. Optional;
	// when set it is round-tripped to Token.Issuer for audit.
	Issuer string
}

// redacted is the text that stands for a credential when a value is printed
// or logged.
const redacted = "[redacted]"

// redact returns redacted for a credential s that is set and "" for an empty
// one.
func redact(s string) string {
	if s == "" {
		return ""
	}
	return redacted
}

// String describes c with its ClientSecret shown as "[redacted]", or as ""
// when it is empty, its ClientAssertion shown as "[redacted]", or as <nil>
// when it has none, and the password of a TokenURL that carries one masked.
// The other fields are shown as they are, except HTTPClient, which is left
// out.
func (c Config) String() string {
	assertion := "<nil>"
	if c.ClientAssertion != nil {
		assertion = redacted
	}
	return fmt.Sprintf("clientcreds.Config{TokenURL: %q, ClientID: %q, ClientSecret: %q, ClientAssertion: %s, Scope: %q, Audience: %q, AuthMethod: %d, RefreshThreshold: %s, Issuer: %q}",
		urlredact.String(c.TokenURL), c.ClientID, redact(c.ClientSecret), assertion, c.Scope, c.Audience, c.AuthMethod, c.RefreshThreshold, c.Issuer)
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
// shows, keyed by field name, with the ClientSecret, the ClientAssertion and
// the TokenURL replaced as String replaces them. Without it, the JSON handler
// would encode the client secret, and the private key of a
// [jwtbearer.ClaimsSigner] client assertion.
func (c Config) LogValue() slog.Value {
	assertion := slog.Any("ClientAssertion", nil)
	if c.ClientAssertion != nil {
		assertion = slog.String("ClientAssertion", redacted)
	}
	return slog.GroupValue(
		slog.String("TokenURL", urlredact.String(c.TokenURL)),
		slog.String("ClientID", c.ClientID),
		slog.String("ClientSecret", redact(c.ClientSecret)),
		assertion,
		slog.String("Scope", c.Scope),
		slog.String("Audience", c.Audience),
		slog.Int("AuthMethod", int(c.AuthMethod)),
		slog.Duration("RefreshThreshold", c.RefreshThreshold),
		slog.String("Issuer", c.Issuer),
	)
}

// Option mutates a Config during construction.
type Option func(*Config)

// WithHTTPClient injects the *http.Client used for token-endpoint
// calls. The token request never follows a redirect, whatever
// CheckRedirect c has, and c is not modified. The request uses a copy of c
// made when the source is built, so a Transport, Timeout or Jar set on c
// afterwards does not reach it. It is required; there is no default.
func WithHTTPClient(c *http.Client) Option {
	return func(cfg *Config) { cfg.HTTPClient = c }
}

// WithScope sets the scope parameter.
func WithScope(scope string) Option { return func(cfg *Config) { cfg.Scope = scope } }

// WithAudience sets the audience parameter.
func WithAudience(aud string) Option { return func(cfg *Config) { cfg.Audience = aud } }

// WithAuthMethod selects how client credentials are presented.
func WithAuthMethod(m AuthMethod) Option { return func(cfg *Config) { cfg.AuthMethod = m } }

// WithRefreshThreshold sets how long before its expiry the cached access
// token counts as stale, so that [Source.Token] exchanges the grant again
// ahead of time. The default is 30 seconds, which a d of 0 also selects. A
// negative d counts as 0: the token is replaced once it has expired, so an
// expired token is never returned.
func WithRefreshThreshold(d time.Duration) Option {
	return func(cfg *Config) { cfg.RefreshThreshold = d }
}

// WithIssuer sets the issuer URL recorded on produced tokens.
func WithIssuer(iss string) Option { return func(cfg *Config) { cfg.Issuer = iss } }

// WithClientAssertion enables SMART Backend Services asymmetric client
// authentication (RFC 7523). The provided AssertionSource is called once per
// token exchange to produce a freshly signed JWT bearer assertion. When set,
// ClientSecret must be empty: the two methods are mutually exclusive, and
// FromConfig rejects both being set with auth.ErrInvalidConfig.
func WithClientAssertion(src jwtbearer.AssertionSource) Option {
	return func(cfg *Config) { cfg.ClientAssertion = src }
}

// Source is the client_credentials TokenSource. Safe for concurrent
// use; concurrent Token() callers coalesce around one outgoing
// exchange.
//
// Printing a Source with any fmt verb shows only its token URL, client ID and
// issuer, never its client secret, its client assertion or the token it
// holds: see [Source.String].
type Source struct {
	cfg      Config
	tokenURL *url.URL
	// client sends the token request, which carries a credential: it never
	// follows a redirect. cfg.HTTPClient, which the caller owns, is left as
	// it was.
	client *http.Client

	mu       sync.Mutex
	cur      auth.Token
	inflight *exchange
	// retryAt is when Token may send an exchange again after a transient
	// failure left the cached access token to be served, never after its
	// ExpiresAt; zero when no backoff holds. backoff is how long the next
	// such failure holds the exchange back. setTokenLocked resets both.
	retryAt time.Time
	backoff time.Duration
}

// String describes s by three values fixed when s was built: its token URL,
// with the password of one that carries one masked, its client ID and its
// issuer. It never shows the client secret, the client assertion or a token,
// and it reads nothing an exchange changes, so it is safe to call while
// other goroutines use s. A nil s gives "<nil>".
func (s *Source) String() string {
	if s == nil {
		return "<nil>"
	}
	return fmt.Sprintf("clientcreds.Source{TokenURL: %q, ClientID: %q, Issuer: %q}",
		urlredact.String(s.cfg.TokenURL), s.cfg.ClientID, s.cfg.Issuer)
}

// GoString returns the text of [Source.String], so that printing s in Go
// syntax shows no credential either.
func (s *Source) GoString() string { return s.String() }

// Format writes the text of [Source.String] for every fmt verb, whatever its
// flags and width. Without it, fmt would print the unexported fields of s
// without calling their methods, the client secret and the held token
// included.
func (s *Source) Format(f fmt.State, _ rune) {
	if f == nil {
		return
	}
	_, _ = io.WriteString(f, s.String())
}

type exchange struct {
	done  chan struct{}
	token auth.Token
	err   error
	// held is the access token the exchange's callers get in place of err
	// when serveHeld reports that the exchange failed transiently while held
	// was still valid. Both are written before done is closed.
	held      auth.Token
	serveHeld bool
	// abandoned reports that the exchange has no result for the callers
	// waiting on it: it failed only because the context of the caller
	// running it ended, or a panic unwound that caller before the result was
	// published. That outcome belongs to that caller only. It is written
	// before done is closed.
	abandoned bool
}

// New constructs a Source from clientID, clientSecret, tokenURL plus
// options. Returns auth.ErrInvalidConfig (wrapped) on missing required
// inputs.
func New(clientID, clientSecret, tokenURL string, opts ...Option) (*Source, error) {
	// FromConfig fills in the defaults, such as the refresh threshold.
	cfg := Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     tokenURL,
		AuthMethod:   AuthBasic,
	}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	return FromConfig(cfg)
}

// FromConfig validates cfg and returns a Source. Use this when
// configuration is loaded declaratively.
func FromConfig(cfg Config) (*Source, error) {
	if cfg.HTTPClient == nil {
		// REQ-021: the caller supplies the client.
		return nil, fmt.Errorf("%w: HTTPClient is required", auth.ErrInvalidConfig)
	}
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("%w: ClientID is required", auth.ErrInvalidConfig)
	}
	if signer, ok := cfg.ClientAssertion.(*jwtbearer.ClaimsSigner); ok && signer == nil {
		// A nil signer passes the nil-interface checks below but can never sign.
		return nil, fmt.Errorf("%w: ClientAssertion is a nil *jwtbearer.ClaimsSigner", auth.ErrInvalidConfig)
	}
	if cfg.ClientSecret != "" && cfg.ClientAssertion != nil {
		return nil, fmt.Errorf("%w: ClientSecret and ClientAssertion are mutually exclusive", auth.ErrInvalidConfig)
	}
	if cfg.AuthMethod == AuthPrivateKeyJWT && cfg.ClientAssertion == nil {
		return nil, fmt.Errorf("%w: AuthPrivateKeyJWT requires WithClientAssertion", auth.ErrInvalidConfig)
	}
	if cfg.ClientSecret == "" && cfg.ClientAssertion == nil {
		return nil, fmt.Errorf("%w: ClientSecret is required (or provide ClientAssertion for SMART Backend Services)", auth.ErrInvalidConfig)
	}
	if cfg.ClientAssertion != nil {
		cfg.AuthMethod = AuthPrivateKeyJWT
	}
	if cfg.TokenURL == "" {
		return nil, fmt.Errorf("%w: TokenURL is required", auth.ErrInvalidConfig)
	}
	u, err := url.Parse(cfg.TokenURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("%w: TokenURL %q is not a valid absolute URL", auth.ErrInvalidConfig, urlredact.String(cfg.TokenURL))
	}
	cfg.RefreshThreshold = cmp.Or(cfg.RefreshThreshold, defaultRefreshThreshold)
	return &Source{cfg: cfg, tokenURL: u, client: noredirect.Client(cfg.HTTPClient)}, nil
}

// NewFromCatalog builds a Source for SMART Backend Services from a resolved
// catalog. It sends token requests to catalog.Auth.TokenEndpoint and records
// catalog.Issuer on the tokens it produces, unless opts include
// [WithIssuer]. clientSecret must be empty when opts include
// [WithClientAssertion], as [FromConfig] refuses both together.
//
// A nil catalog, or one without a token endpoint, is refused with
// [auth.ErrInvalidConfig]. So is a configuration the catalog says the server
// does not accept:
//   - grant_types_supported leaves out client_credentials;
//   - token_endpoint_auth_methods_supported leaves out the configured
//     method: private_key_jwt with a client assertion, otherwise
//     client_secret_basic or client_secret_post as [WithAuthMethod] selects;
//   - token_endpoint_auth_signing_alg_values_supported leaves out the
//     algorithm of a [jwtbearer.ClaimsSigner] client assertion.
//
// A list the catalog leaves empty does not constrain anything, and a client
// assertion from a source other than a [jwtbearer.ClaimsSigner] is not
// checked against the algorithm list. The other checks are those of
// [FromConfig].
func NewFromCatalog(catalog *discovery.ServiceCatalog, clientID, clientSecret string, opts ...Option) (*Source, error) {
	if catalog == nil {
		return nil, fmt.Errorf("%w: catalog is nil", auth.ErrInvalidConfig)
	}
	if catalog.Auth.TokenEndpoint == nil {
		return nil, fmt.Errorf("%w: catalog has no token endpoint", auth.ErrInvalidConfig)
	}
	// The issuer default comes first, so an option the caller passes wins.
	all := append([]Option{WithIssuer(catalog.Issuer)}, opts...)
	s, err := New(clientID, clientSecret, catalog.Auth.TokenEndpoint.String(), all...)
	if err != nil {
		return nil, err
	}
	if err := checkAdvertised(catalog.Auth, s.cfg); err != nil {
		return nil, err
	}
	return s, nil
}

// checkAdvertised refuses cfg when a list the server advertises leaves out
// the grant, the client auth method, or the client-assertion algorithm cfg
// would use. An empty list does not constrain. The errors name only public
// metadata: the list and the configured value, never a credential.
func checkAdvertised(a discovery.AuthEndpoints, cfg Config) error {
	if l := a.GrantTypesSupported; len(l) > 0 && !slices.Contains(l, grantClientCredentials) {
		return fmt.Errorf("%w: grant type %q is not in the server's advertised grant_types_supported %v",
			auth.ErrInvalidConfig, grantClientCredentials, l)
	}
	method := authMethodName(cfg)
	if l := a.TokenEndpointAuthMethodsSupported; len(l) > 0 && !slices.Contains(l, method) {
		return fmt.Errorf("%w: configured client auth method %q is not in the server's advertised token_endpoint_auth_methods_supported %v",
			auth.ErrInvalidConfig, method, l)
	}
	// Only the SDK's own signer exposes its algorithm; any other assertion
	// source is opaque and is not checked. FromConfig has already refused a
	// nil signer.
	if signer, ok := cfg.ClientAssertion.(*jwtbearer.ClaimsSigner); ok {
		if l := a.TokenEndpointAuthSigningAlgValuesSupported; len(l) > 0 && !slices.Contains(l, signer.Algorithm) {
			return fmt.Errorf("%w: client assertion algorithm %q is not in the server's advertised token_endpoint_auth_signing_alg_values_supported %v",
				auth.ErrInvalidConfig, signer.Algorithm, l)
		}
	}
	return nil
}

// authMethodName returns the token_endpoint_auth_methods_supported name of
// the client authentication fetch sends for cfg. It follows fetch's own
// choice: a client assertion first, then client_secret_post when selected,
// and HTTP Basic otherwise.
func authMethodName(cfg Config) string {
	switch {
	case cfg.ClientAssertion != nil:
		return methodPrivateKeyJWT
	case cfg.AuthMethod == AuthPost:
		return methodClientSecretPost
	default:
		return methodClientSecretBasic
	}
}

var _ auth.Reauther = (*Source)(nil)

// Token returns the current access token, refreshing transparently
// when the cached token is within RefreshThreshold of expiry.
// Concurrent callers share the in-flight exchange. If the caller running
// that exchange gives up first, and that alone fails the exchange, a caller
// whose own context is still live starts a new exchange rather than
// returning the cancellation of the caller that gave up. Any other failure
// reaches every caller waiting on the exchange. A panic in the exchange,
// from the HTTP client's RoundTripper or the assertion source for example,
// goes up through the caller running it, unchanged, and the callers waiting
// on that exchange start a new one the same way.
//
// An exchange that fails without being terminal (a server or network error,
// a timeout, an answer the source cannot read, or an assertion it cannot
// sign) leaves the cached token in place. While that token is not yet past
// its ExpiresAt, Token then returns it without an error, and sends no new
// exchange until a backoff ends: one second after the first such failure,
// twice as long after each further failure in a row, and never later than
// that ExpiresAt. Until then Token returns the cached token without a
// request. A successful exchange ends the backoff. Past ExpiresAt the
// failure is returned, matching [auth.ErrTokenExchangeFailed], and each call
// tries again. The calls waiting on an exchange get the same outcome, the
// cached token included; [Source.Reauth] gets the failure.
func (s *Source) Token(ctx context.Context) (auth.Token, error) {
	for {
		if err := ctx.Err(); err != nil {
			return auth.Token{}, err
		}
		s.mu.Lock()
		if !s.stale() || time.Now().Before(s.retryAt) {
			// The token is fresh, or a transient failure left it to be served
			// until the backoff ends, which is no later than its ExpiresAt.
			t := s.cur
			s.mu.Unlock()
			return t, nil
		}
		ex := s.inflight
		if ex == nil {
			ex = &exchange{done: make(chan struct{})}
			s.inflight = ex
			s.mu.Unlock()
			return s.lead(ctx, ex)
		}
		s.mu.Unlock()
		select {
		case <-ex.done:
		case <-ctx.Done():
			return auth.Token{}, ctx.Err()
		}
		if !ex.abandoned {
			return ex.outcome()
		}
		// The caller running ex gave up or panicked, and this caller's
		// context is live: go round again to run a new exchange or wait on a
		// newer one. A new round follows only an exchange whose caller gave
		// up or panicked, so the loop never spins.
	}
}

// Reauth drops the cached access token and obtains a new one with a fresh
// exchange, even when the cached token is not yet near expiry. Use it to
// recover from a wire 401, for example through transport.WithReauthOn401.
// The client credentials grant issues no refresh token, so a new exchange is
// the only way to a new token.
//
// When an exchange is already in flight, Reauth waits for that exchange
// instead of starting another, so concurrent Reauth and Token calls send one
// request to the token endpoint. If the caller running that exchange gives up
// first, Reauth starts a new exchange rather than returning that caller's
// cancellation. Reauth returns the exchange's error, which is an
// [*auth.ExchangeError] wrapping [auth.ErrTokenExchangeFailed], or the
// context's error when ctx ends first. It exchanges even while a backoff
// after a transient failure holds [Source.Token] back. A Reauth whose
// context has already ended keeps the cached token. After a failed exchange
// no token is cached, and the next Token call tries again.
func (s *Source) Reauth(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	// Dropping the token also ends its backoff, so the exchange below is
	// sent at once, and no exchange that ends afterwards can serve the
	// token to this call.
	s.setTokenLocked(auth.Token{})
	s.mu.Unlock()
	_, err := s.Token(ctx)
	return err
}

// lead runs the exchange ex on behalf of every caller waiting on it, caches
// the token it obtains, and publishes the result to those callers.
//
// A panic in the exchange unwinds lead before the result is published. The
// deferred release still takes ex out of flight and wakes the waiters, with
// ex marked abandoned so that they start a new exchange, and the panic goes
// on to the caller as it was raised.
func (s *Source) lead(ctx context.Context, ex *exchange) (auth.Token, error) {
	// ex stays abandoned unless the exchange returns and its result is
	// published below.
	ex.abandoned = true
	defer s.release(ex)

	tok, err := s.fetch(ctx)
	abandoned := coalesce.EndedByContext(ctx, err)
	s.mu.Lock()
	switch {
	case err == nil:
		s.setTokenLocked(tok)
	case !abandoned && !terminal(err) && s.backOffLocked():
		// A transient failure, and the cached token is still valid: it is
		// served in place of the failure until the backoff ends (REQ-063).
		ex.held = s.cur
		ex.serveHeld = true
	}
	// Out of flight in the same step, so a Reauth that drops the cached
	// token after this decision sends an exchange of its own instead of
	// joining this one; one that dropped it before has left no token to
	// serve. Either way Reauth gets an exchange's failure.
	if s.inflight == ex {
		s.inflight = nil
	}
	s.mu.Unlock()
	ex.token = tok
	ex.err = err
	ex.abandoned = abandoned
	return ex.outcome()
}

// terminal reports whether err is a permanent token-endpoint failure, as
// [auth.ExchangeError.Terminal] decides it.
func terminal(err error) bool {
	exErr, ok := errors.AsType[*auth.ExchangeError](err)
	return ok && exErr.Terminal()
}

// outcome returns what a caller of the exchange ex stands for gets: the
// cached access token in place of a transient failure that left it to be
// served, and otherwise the exchange's own token and error.
func (ex *exchange) outcome() (auth.Token, error) {
	if ex.serveHeld {
		return ex.held, nil
	}
	return ex.token, ex.err
}

// setTokenLocked caches tok, a new token or none, and ends the backoff of
// the token it replaces. The caller holds s.mu.
func (s *Source) setTokenLocked(tok auth.Token) {
	s.cur = tok
	s.retryAt = time.Time{}
	s.backoff = 0
}

// backOffLocked records an exchange that failed transiently. While the
// cached access token is not yet past its ExpiresAt, it holds Token's next
// exchange back until the backoff ends, never later than that ExpiresAt,
// doubles the backoff for the next failure in a row, and reports true: the
// token is to be served in place of the failure. The caller holds s.mu.
func (s *Source) backOffLocked() bool {
	now := time.Now()
	left := s.cur.ExpiresAt.Sub(now)
	if left <= 0 {
		return false
	}
	wait := min(cmp.Or(s.backoff, firstBackoff), left)
	s.retryAt = now.Add(wait)
	s.backoff = 2 * wait
	return true
}

// release takes ex out of flight, unless a newer exchange has replaced it
// there, then wakes the callers waiting on it. lead defers it, so it runs
// exactly once per exchange, also when a panic unwinds lead.
func (s *Source) release(ex *exchange) {
	s.mu.Lock()
	if s.inflight == ex {
		s.inflight = nil
	}
	s.mu.Unlock()
	close(ex.done)
}

func (s *Source) stale() bool {
	if s.cur.IsZero() {
		return true
	}
	if s.cur.ExpiresAt.IsZero() {
		// No declared expiry: cache until Reauth drops the token, for
		// example after a wire 401 through transport.WithReauthOn401, or
		// until the consumer replaces the TokenSource.
		return false
	}
	// A negative threshold counts as 0, so a token past its expiry is never
	// served (REQ-063).
	return time.Until(s.cur.ExpiresAt) <= max(s.cfg.RefreshThreshold, 0)
}

// tokenResponse mirrors the RFC 6749 § 5.1 success body. Numeric
// "expires_in" is decoded as a json.Number to tolerate authorization
// servers that emit it as a string.
type tokenResponse struct {
	AccessToken string      `json:"access_token"`
	TokenType   string      `json:"token_type"`
	ExpiresIn   json.Number `json:"expires_in"`
	Scope       string      `json:"scope"`
}

func (s *Source) fetch(ctx context.Context) (auth.Token, error) {
	form := url.Values{
		"grant_type": {grantClientCredentials},
	}
	if s.cfg.Scope != "" {
		form.Set("scope", s.cfg.Scope)
	}
	if s.cfg.Audience != "" {
		form.Set("audience", s.cfg.Audience)
	}

	// Credential emission: private_key_jwt (SMART Backend Services) takes
	// priority; otherwise fall back to client_secret_post or client_secret_basic.
	var basicAuth bool
	if s.cfg.ClientAssertion != nil {
		// SMART Backend Services: RFC 7523 asymmetric client authentication.
		// Call Assertion within fetch (outside the mutex) to avoid holding the
		// lock during a potentially slow signing/network operation. This is safe
		// because fetch is only called by Token() after releasing the mutex and
		// setting the in-flight exchange sentinel.
		assertionVal, err := s.cfg.ClientAssertion.Assertion(ctx)
		if err != nil {
			return auth.Token{}, &auth.ExchangeError{
				Sentinel: auth.ErrTokenExchangeFailed,
				Inner:    fmt.Errorf("client_assertion signing: %w", err),
			}
		}
		form.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
		form.Set("client_assertion", assertionVal)
	} else if s.cfg.AuthMethod == AuthPost {
		form.Set("client_id", s.cfg.ClientID)
		form.Set("client_secret", s.cfg.ClientSecret)
	} else {
		// AuthBasic (default): set Basic auth header after building the request.
		basicAuth = true
	}

	tokenURL := s.tokenURL.String()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		// url.Parse can refuse tokenURL even though it accepted
		// cfg.TokenURL, and its error quotes tokenURL with the password.
		return auth.Token{}, &auth.ExchangeError{Sentinel: auth.ErrTokenExchangeFailed, Inner: urlredact.Error(err, tokenURL)}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if basicAuth {
		// RFC 6749 §2.3.1: client_id and client_secret are form-encoded
		// (Appendix B) before use as the Basic username and password.
		// net/http documents the same requirement for OAuth2.
		req.SetBasicAuth(url.QueryEscape(s.cfg.ClientID), url.QueryEscape(s.cfg.ClientSecret))
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return auth.Token{}, &auth.ExchangeError{Sentinel: auth.ErrTokenExchangeFailed, Inner: err}
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return auth.Token{}, &auth.ExchangeError{Sentinel: auth.ErrTokenExchangeFailed, StatusCode: resp.StatusCode, Inner: err}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return auth.Token{}, &auth.ExchangeError{
			Sentinel:   auth.ErrTokenExchangeFailed,
			StatusCode: resp.StatusCode,
			OAuth2:     auth.ParseOAuth2Error(body),
			Inner:      fmt.Errorf("token endpoint returned %d", resp.StatusCode),
		}
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return auth.Token{}, &auth.ExchangeError{
			Sentinel:   auth.ErrTokenExchangeFailed,
			StatusCode: resp.StatusCode,
			Inner:      fmt.Errorf("decode token response: %w", err),
		}
	}
	if tr.AccessToken == "" {
		return auth.Token{}, &auth.ExchangeError{
			Sentinel:   auth.ErrTokenExchangeFailed,
			StatusCode: resp.StatusCode,
			Inner:      errors.New("token endpoint returned no access_token"),
		}
	}

	var expiresAt time.Time
	if secs, ok := expiresin.Seconds(tr.ExpiresIn.String()); ok && secs > 0 {
		expiresAt = time.Now().Add(time.Duration(secs) * time.Second)
	}
	return auth.Token{
		Value:     tr.AccessToken,
		Type:      authScheme(tr.TokenType),
		ExpiresAt: expiresAt,
		Scope:     cmp.Or(tr.Scope, s.cfg.Scope),
		Issuer:    s.cfg.Issuer,
	}, nil
}

// authScheme maps a token endpoint's token_type to the Authorization scheme.
// The value is case insensitive (RFC 6749 §5.1), so any spelling of bearer,
// or no value at all, becomes "Bearer"; any other scheme passes through
// unchanged.
func authScheme(tokenType string) string {
	if tokenType == "" || strings.EqualFold(tokenType, auth.TokenTypeBearer) {
		return auth.TokenTypeBearer
	}
	return tokenType
}
