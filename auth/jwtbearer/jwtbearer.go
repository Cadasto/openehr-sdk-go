package jwtbearer

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
	"strings"
	"sync"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/internal/coalesce"
	"github.com/cadasto/openehr-sdk-go/internal/expiresin"
	"github.com/cadasto/openehr-sdk-go/internal/noredirect"
	"github.com/cadasto/openehr-sdk-go/internal/urlredact"
)

// GrantType is the assertion grant identifier defined by RFC 7523.
const GrantType = "urn:ietf:params:oauth:grant-type:jwt-bearer"

// defaultRefreshThreshold is how long before its expiry the cached access
// token counts as stale when the configuration sets no threshold.
const defaultRefreshThreshold = 30 * time.Second

// firstBackoff is how long the source holds back the next exchange after the
// first transient failure in a row, while it serves the cached access token.
// Each further failure doubles it.
const firstBackoff = time.Second

// Config carries the constructor inputs. Use New(...Option) for the
// idiomatic call site; Config is exposed for declarative configuration.
//
// Printed with fmt or logged with log/slog, a Config shows its Assertion as
// "[redacted]": see [Config.String] and [Config.LogValue]. A [ClaimsSigner]
// assertion holds a private key.
type Config struct {
	// HTTPClient is the injected client used for token-endpoint calls.
	// The token request never follows a redirect, whatever CheckRedirect
	// the client has, and the client is not modified. The request uses a
	// copy made when the source is built, so a Transport, Timeout or Jar set
	// on the client afterwards does not reach it. Required.
	HTTPClient *http.Client
	TokenURL   string
	Assertion  AssertionSource
	Scope      string
	Audience   string
	// RefreshThreshold is how long before its expiry the cached access token
	// counts as stale, so that [Source.Token] exchanges an assertion again
	// ahead of time. 0 selects the default of 30 seconds. A negative value
	// counts as 0: the token is replaced once it has expired, so an expired
	// token is never returned.
	RefreshThreshold time.Duration
	Issuer           string
	// ClientID, when set, is sent in the form body as client_id;
	// some authorization servers require this even with a JWT
	// assertion. Default empty.
	ClientID string
}

// redacted is the text that stands for a credential when a value is printed
// or logged.
const redacted = "[redacted]"

// credentialText returns how a printed value shows a credential that is not a
// string: redacted when set is true, and <nil> when it is not.
func credentialText(set bool) string {
	if set {
		return redacted
	}
	return "<nil>"
}

// credentialAttr returns the log attribute key that stands for a credential
// that is not a string: redacted when set is true, and nil when it is not.
func credentialAttr(key string, set bool) slog.Attr {
	if set {
		return slog.String(key, redacted)
	}
	return slog.Any(key, nil)
}

// String describes c with its Assertion shown as "[redacted]", or as <nil>
// when it has none, and with the password of a TokenURL that carries one
// masked. The other fields are shown as they are, except HTTPClient, which is
// left out.
func (c Config) String() string {
	return fmt.Sprintf("jwtbearer.Config{TokenURL: %q, Assertion: %s, Scope: %q, Audience: %q, RefreshThreshold: %s, Issuer: %q, ClientID: %q}",
		urlredact.String(c.TokenURL), credentialText(c.Assertion != nil), c.Scope, c.Audience, c.RefreshThreshold, c.Issuer, c.ClientID)
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
// shows, keyed by field name, with the Assertion and the TokenURL replaced
// as String replaces them.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("TokenURL", urlredact.String(c.TokenURL)),
		credentialAttr("Assertion", c.Assertion != nil),
		slog.String("Scope", c.Scope),
		slog.String("Audience", c.Audience),
		slog.Duration("RefreshThreshold", c.RefreshThreshold),
		slog.String("Issuer", c.Issuer),
		slog.String("ClientID", c.ClientID),
	)
}

// Option mutates a Config during construction.
type Option func(*Config)

// WithHTTPClient injects the *http.Client used for token-endpoint
// calls. The token request never follows a redirect, whatever
// CheckRedirect c has, and c is not modified. The request uses a copy of c
// made when the source is built, so a Transport, Timeout or Jar set on c
// afterwards does not reach it. Required.
func WithHTTPClient(c *http.Client) Option {
	return func(cfg *Config) { cfg.HTTPClient = c }
}

// WithScope sets the scope parameter.
func WithScope(scope string) Option { return func(cfg *Config) { cfg.Scope = scope } }

// WithAudience sets an additional audience parameter for authorization
// servers that require it alongside the JWT's aud claim.
func WithAudience(aud string) Option { return func(cfg *Config) { cfg.Audience = aud } }

// WithClientID sets the form-body client_id parameter.
func WithClientID(id string) Option { return func(cfg *Config) { cfg.ClientID = id } }

// WithRefreshThreshold sets how long before its expiry the cached access
// token counts as stale, so that [Source.Token] exchanges an assertion again
// ahead of time. The default is 30 seconds, which a d of 0 also selects. A
// negative d counts as 0: the token is replaced once it has expired, so an
// expired token is never returned.
func WithRefreshThreshold(d time.Duration) Option {
	return func(cfg *Config) { cfg.RefreshThreshold = d }
}

// WithIssuer sets the issuer URL recorded on produced tokens.
func WithIssuer(iss string) Option { return func(cfg *Config) { cfg.Issuer = iss } }

// Source is the jwt-bearer TokenSource.
//
// Printing a Source with any fmt verb shows only its token URL, client ID and
// issuer, never its assertion source or the token it holds: see
// [Source.String].
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
// issuer. It never shows the assertion source or a token, and it reads
// nothing an exchange changes, so it is safe to call while other goroutines
// use s. A nil s gives "<nil>".
func (s *Source) String() string {
	if s == nil {
		return "<nil>"
	}
	return fmt.Sprintf("jwtbearer.Source{TokenURL: %q, ClientID: %q, Issuer: %q}",
		urlredact.String(s.cfg.TokenURL), s.cfg.ClientID, s.cfg.Issuer)
}

// GoString returns the text of [Source.String], so that printing s in Go
// syntax shows no credential either.
func (s *Source) GoString() string { return s.String() }

// Format writes the text of [Source.String] for every fmt verb, whatever its
// flags and width. Without it, fmt would print the unexported fields of s
// without calling their methods, the held token included.
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

// New constructs a Source. tokenURL is the authorization server's
// token endpoint; assertion is how the provider obtains a signed JWT
// for each exchange.
func New(tokenURL string, assertion AssertionSource, opts ...Option) (*Source, error) {
	// FromConfig fills in the defaults, such as the refresh threshold.
	cfg := Config{
		TokenURL:  tokenURL,
		Assertion: assertion,
	}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	return FromConfig(cfg)
}

// FromConfig validates cfg and returns a Source.
func FromConfig(cfg Config) (*Source, error) {
	if cfg.HTTPClient == nil {
		// REQ-021: the caller supplies the client.
		return nil, fmt.Errorf("%w: HTTPClient is required", auth.ErrInvalidConfig)
	}
	if cfg.TokenURL == "" {
		return nil, fmt.Errorf("%w: TokenURL is required", auth.ErrInvalidConfig)
	}
	if cfg.Assertion == nil {
		return nil, fmt.Errorf("%w: AssertionSource is required", auth.ErrInvalidConfig)
	}
	u, err := url.Parse(cfg.TokenURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("%w: TokenURL %q is not a valid absolute URL", auth.ErrInvalidConfig, urlredact.String(cfg.TokenURL))
	}
	cfg.RefreshThreshold = cmp.Or(cfg.RefreshThreshold, defaultRefreshThreshold)
	return &Source{cfg: cfg, tokenURL: u, client: noredirect.Client(cfg.HTTPClient)}, nil
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
// a timeout, an answer the source cannot read, or an assertion the
// AssertionSource cannot give) leaves the cached token in place. While that
// token is not yet past its ExpiresAt, Token then returns it without an
// error, and sends no new exchange until a backoff ends: one second after
// the first such failure, twice as long after each further failure in a
// row, and never later than that ExpiresAt. Until then Token returns the
// cached token without a request. A successful exchange ends the backoff.
// Past ExpiresAt the failure is returned, matching
// [auth.ErrTokenExchangeFailed], and each call tries again. The calls
// waiting on an exchange get the same outcome, the cached token included;
// [Source.Reauth] gets the failure.
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
// exchange, even when the cached token is not yet near expiry. The exchange
// asks the AssertionSource for an assertion again; a [ClaimsSigner] signs a
// new one, while a [StaticAssertion] sends the same JWT again. Use Reauth to
// recover from a wire 401, for example through transport.WithReauthOn401.
// The JWT bearer grant issues no refresh token, so a new exchange is the
// only way to a new token.
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
		return false
	}
	// A negative threshold counts as 0, so a token past its expiry is never
	// served (REQ-063).
	return time.Until(s.cur.ExpiresAt) <= max(s.cfg.RefreshThreshold, 0)
}

type tokenResponse struct {
	AccessToken string      `json:"access_token"`
	TokenType   string      `json:"token_type"`
	ExpiresIn   json.Number `json:"expires_in"`
	Scope       string      `json:"scope"`
}

func (s *Source) fetch(ctx context.Context) (auth.Token, error) {
	assertion, err := s.cfg.Assertion.Assertion(ctx)
	if err != nil {
		return auth.Token{}, &auth.ExchangeError{Sentinel: auth.ErrTokenExchangeFailed, Inner: fmt.Errorf("assertion source: %w", err)}
	}
	form := url.Values{
		"grant_type": {GrantType},
		"assertion":  {assertion},
	}
	if s.cfg.Scope != "" {
		form.Set("scope", s.cfg.Scope)
	}
	if s.cfg.Audience != "" {
		form.Set("audience", s.cfg.Audience)
	}
	if s.cfg.ClientID != "" {
		form.Set("client_id", s.cfg.ClientID)
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
