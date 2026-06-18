// Package jwtbearer implements the OAuth2 JWT Bearer (RFC 7523) grant
// as an auth.TokenSource. The provider exchanges a signed JWT
// assertion for an access token at the deployment's token endpoint.
//
// Two signing modes are offered:
//
//   - ClaimsSigner — the SDK signs claims with a held crypto.Signer
//     (default RS384; SMART client-confidential-asymmetric baseline —
//     RS256/ES256/ES384 also supported). Use when the consumer owns the
//     private key, including opaque KMS/HSM adapters.
//   - AssertionFunc / StaticAssertion — the consumer supplies a
//     pre-signed assertion. Use when the assertion is minted by an
//     upstream identity broker.
//
// The Source caches the issued access token until it nears expiry,
// then signs a fresh assertion and re-exchanges. Concurrent Token()
// calls coalesce around a single in-flight exchange (REQ-026).
//
// HTTP client injection follows REQ-021.
package jwtbearer

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

// GrantType is the assertion grant identifier defined by RFC 7523.
const GrantType = "urn:ietf:params:oauth:grant-type:jwt-bearer"

// Config carries the constructor inputs. Use New(...Option) for the
// idiomatic call site; Config is exposed for declarative configuration.
type Config struct {
	HTTPClient       *http.Client
	TokenURL         string
	Assertion        AssertionSource
	Scope            string
	Audience         string
	RefreshThreshold time.Duration
	Issuer           string
	// ClientID, when set, is sent in the form body as client_id —
	// some authorization servers require this even with a JWT
	// assertion. Default empty.
	ClientID string
}

// Option mutates a Config during construction.
type Option func(*Config)

// WithHTTPClient injects the *http.Client used for token-endpoint
// calls. Required per REQ-021.
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

// WithRefreshThreshold overrides the staleness window.
func WithRefreshThreshold(d time.Duration) Option {
	return func(cfg *Config) { cfg.RefreshThreshold = d }
}

// WithIssuer sets the issuer URL recorded on produced tokens.
func WithIssuer(iss string) Option { return func(cfg *Config) { cfg.Issuer = iss } }

// Source is the jwt-bearer TokenSource.
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

// New constructs a Source. tokenURL is the authorization server's
// token endpoint; assertion is how the provider obtains a signed JWT
// for each exchange.
func New(tokenURL string, assertion AssertionSource, opts ...Option) (*Source, error) {
	cfg := Config{
		TokenURL:         tokenURL,
		Assertion:        assertion,
		RefreshThreshold: 30 * time.Second,
	}
	for _, o := range opts {
		o(&cfg)
	}
	return FromConfig(cfg)
}

// FromConfig validates cfg and returns a Source.
func FromConfig(cfg Config) (*Source, error) {
	if cfg.HTTPClient == nil {
		return nil, fmt.Errorf("%w: HTTPClient is required (REQ-021)", auth.ErrInvalidConfig)
	}
	if cfg.TokenURL == "" {
		return nil, fmt.Errorf("%w: TokenURL is required", auth.ErrInvalidConfig)
	}
	if cfg.Assertion == nil {
		return nil, fmt.Errorf("%w: AssertionSource is required", auth.ErrInvalidConfig)
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

// Token returns the current access token, refreshing transparently
// when the cached token is within RefreshThreshold of expiry.
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
		return false
	}
	return time.Until(s.cur.ExpiresAt) <= s.cfg.RefreshThreshold
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return auth.Token{}, &auth.ExchangeError{Sentinel: auth.ErrTokenExchangeFailed, Inner: err}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

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
	if str := tr.ExpiresIn.String(); str != "" {
		if secs, err := strconv.ParseInt(str, 10, 64); err == nil && secs > 0 {
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
