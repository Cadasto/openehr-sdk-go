// Package clientcreds implements the OAuth2 Client Credentials grant
// (RFC 6749 § 4.4) as an auth.TokenSource — for service-to-service
// callers (benchmark, seeder, MCP server backend, federator) that do
// not run an interactive user flow.
//
// The provider caches the issued access token until it nears expiry,
// then re-requests. Client Credentials does not produce a refresh
// token; "refresh" here means "request a new access token via the same
// grant". Concurrent Token() calls coalesce around a single in-flight
// request (REQ-026).
//
// HTTP client injection follows REQ-021 — callers MUST inject the
// *http.Client whose timeouts and TLS roots they want to apply to the
// token endpoint. A nil http.Client is rejected at construction.
package clientcreds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// AuthMethod selects how the client authenticates to the token
// endpoint when exchanging the grant.
type AuthMethod int

const (
	// AuthBasic sends client_id and client_secret as HTTP Basic auth
	// per RFC 6749 § 2.3.1 ("client_secret_basic"). This is the
	// default and the form the spec describes as REQUIRED-to-support.
	AuthBasic AuthMethod = iota
	// AuthPost sends client_id and client_secret in the form-encoded
	// request body ("client_secret_post"). Use when the deployment
	// rejects Basic auth (some legacy authorization servers).
	AuthPost
)

// Config carries the constructor inputs. Use New(...Option) for the
// idiomatic call site; Config is exposed for declarative configuration
// (env, YAML).
type Config struct {
	// HTTPClient is the injected client used for token-endpoint calls.
	// Required (REQ-021).
	HTTPClient *http.Client
	// TokenURL is the token endpoint of the authorization server.
	TokenURL string
	// ClientID identifies the registered confidential client.
	ClientID string
	// ClientSecret is the corresponding secret. Required.
	ClientSecret string
	// Scope is the space-separated scope to request; empty omits the
	// scope parameter (deployment-defined default scope applies).
	Scope string
	// Audience optionally sets the `audience` parameter for
	// authorization servers that require explicit audience binding.
	Audience string
	// AuthMethod selects HTTP Basic vs form-body credentialing.
	AuthMethod AuthMethod
	// RefreshThreshold is how long before ExpiresAt the source treats
	// the cached token as stale and triggers a refresh. Default 30s
	// (matches docs/specifications/auth.md REQ-063).
	RefreshThreshold time.Duration
	// Issuer is the issuer URL recorded on the produced Token. Optional;
	// when set it is round-tripped to Token.Issuer for audit.
	Issuer string
}

// Option mutates a Config during construction.
type Option func(*Config)

// WithHTTPClient injects the *http.Client used for token-endpoint
// calls. Required per REQ-021 — there is no default.
func WithHTTPClient(c *http.Client) Option {
	return func(cfg *Config) { cfg.HTTPClient = c }
}

// WithScope sets the scope parameter.
func WithScope(scope string) Option { return func(cfg *Config) { cfg.Scope = scope } }

// WithAudience sets the audience parameter.
func WithAudience(aud string) Option { return func(cfg *Config) { cfg.Audience = aud } }

// WithAuthMethod selects how client credentials are presented.
func WithAuthMethod(m AuthMethod) Option { return func(cfg *Config) { cfg.AuthMethod = m } }

// WithRefreshThreshold overrides the staleness window.
func WithRefreshThreshold(d time.Duration) Option {
	return func(cfg *Config) { cfg.RefreshThreshold = d }
}

// WithIssuer sets the issuer URL recorded on produced tokens.
func WithIssuer(iss string) Option { return func(cfg *Config) { cfg.Issuer = iss } }

// Source is the client_credentials TokenSource. Safe for concurrent
// use; concurrent Token() callers coalesce around one outgoing
// exchange (REQ-026).
type Source struct {
	cfg      Config
	tokenURL *url.URL

	mu       sync.Mutex
	cur      auth.Token
	inflight *exchange
}

type exchange struct {
	done  chan struct{}
	token auth.Token
	err   error
}

// New constructs a Source from clientID, clientSecret, tokenURL plus
// options. Returns auth.ErrInvalidConfig (wrapped) on missing required
// inputs.
func New(clientID, clientSecret, tokenURL string, opts ...Option) (*Source, error) {
	cfg := Config{
		ClientID:         clientID,
		ClientSecret:     clientSecret,
		TokenURL:         tokenURL,
		AuthMethod:       AuthBasic,
		RefreshThreshold: 30 * time.Second,
	}
	for _, o := range opts {
		o(&cfg)
	}
	return FromConfig(cfg)
}

// FromConfig validates cfg and returns a Source. Use this when
// configuration is loaded declaratively.
func FromConfig(cfg Config) (*Source, error) {
	if cfg.HTTPClient == nil {
		return nil, fmt.Errorf("%w: HTTPClient is required (REQ-021)", auth.ErrInvalidConfig)
	}
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("%w: ClientID is required", auth.ErrInvalidConfig)
	}
	if cfg.ClientSecret == "" {
		return nil, fmt.Errorf("%w: ClientSecret is required", auth.ErrInvalidConfig)
	}
	if cfg.TokenURL == "" {
		return nil, fmt.Errorf("%w: TokenURL is required", auth.ErrInvalidConfig)
	}
	u, err := url.Parse(cfg.TokenURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("%w: TokenURL %q is not a valid absolute URL", auth.ErrInvalidConfig, cfg.TokenURL)
	}
	if cfg.RefreshThreshold == 0 {
		cfg.RefreshThreshold = 30 * time.Second
	}
	return &Source{cfg: cfg, tokenURL: u}, nil
}

// Invalidate drops the cached token so the next Token call performs a fresh
// exchange. It satisfies auth.Invalidatable: transport/ calls this after a
// wire 401 on an authenticated request, which recovers the case where the
// authorization server omitted "expires_in" (zero ExpiresAt → never treated
// as stale) yet the token has in fact expired server-side (REQ-063).
func (s *Source) Invalidate() {
	s.mu.Lock()
	s.cur = auth.Token{}
	s.mu.Unlock()
}

// Token returns the current access token, refreshing transparently
// when the cached token is within RefreshThreshold of expiry.
// Concurrent callers share the in-flight exchange.
func (s *Source) Token(ctx context.Context) (auth.Token, error) {
	if err := ctx.Err(); err != nil {
		return auth.Token{}, err
	}
	s.mu.Lock()
	if !s.stale() {
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
	ex := &exchange{done: make(chan struct{})}
	s.inflight = ex
	s.mu.Unlock()

	tok, err := s.fetch(ctx)

	s.mu.Lock()
	if err == nil {
		s.cur = tok
	}
	s.inflight = nil
	s.mu.Unlock()
	ex.token = tok
	ex.err = err
	close(ex.done)

	return tok, err
}

func (s *Source) stale() bool {
	if s.cur.IsZero() {
		return true
	}
	if s.cur.ExpiresAt.IsZero() {
		// No declared expiry — cache until the consumer replaces the
		// TokenSource or a wire 401 surfaces (transport does not
		// auto-refresh; the application must obtain a new token).
		return false
	}
	return time.Until(s.cur.ExpiresAt) <= s.cfg.RefreshThreshold
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
		"grant_type": {"client_credentials"},
	}
	if s.cfg.Scope != "" {
		form.Set("scope", s.cfg.Scope)
	}
	if s.cfg.Audience != "" {
		form.Set("audience", s.cfg.Audience)
	}
	if s.cfg.AuthMethod == AuthPost {
		form.Set("client_id", s.cfg.ClientID)
		form.Set("client_secret", s.cfg.ClientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return auth.Token{}, &auth.ExchangeError{Sentinel: auth.ErrTokenExchangeFailed, Inner: err}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if s.cfg.AuthMethod == AuthBasic {
		// RFC 6749 §2.3.1: client_id and client_secret are form-encoded
		// (Appendix B) before use as the Basic username and password.
		// net/http documents the same requirement for OAuth2.
		req.SetBasicAuth(url.QueryEscape(s.cfg.ClientID), url.QueryEscape(s.cfg.ClientSecret))
	}

	resp, err := s.cfg.HTTPClient.Do(req)
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
	if s := tr.ExpiresIn.String(); s != "" {
		if secs, err := strconv.ParseInt(s, 10, 64); err == nil && secs > 0 {
			expiresAt = time.Now().Add(time.Duration(secs) * time.Second)
		}
	}
	typ := tr.TokenType
	if typ == "" {
		typ = "Bearer"
	}
	scope := tr.Scope
	if scope == "" {
		scope = s.cfg.Scope
	}
	return auth.Token{
		Value:     tr.AccessToken,
		Type:      typ,
		ExpiresAt: expiresAt,
		Scope:     scope,
		Issuer:    s.cfg.Issuer,
	}, nil
}
