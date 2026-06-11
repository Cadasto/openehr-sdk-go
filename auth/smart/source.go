package smart

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// Config carries SMART-on-openEHR OAuth2 settings (REQ-061–063).
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
}

// Option mutates Config during construction.
type Option func(*Config)

// WithHTTPClient injects the client for token and JWKS calls (REQ-021).
func WithHTTPClient(c *http.Client) Option {
	return func(cfg *Config) { cfg.HTTPClient = c }
}

// WithClientSecret enables confidential-client token exchange.
func WithClientSecret(secret string) Option {
	return func(cfg *Config) { cfg.ClientSecret = secret }
}

// WithRedirectURI sets the registered redirect URI.
func WithRedirectURI(uri string) Option {
	return func(cfg *Config) { cfg.RedirectURI = uri }
}

// WithScopes sets the space-separated scope request (slice joined).
func WithScopes(scopes ...string) Option {
	return func(cfg *Config) { cfg.Scopes = scopes }
}

// WithAudience sets the `aud` authorization parameter.
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
func New(clientID string, authEP discovery.AuthEndpoints, opts ...Option) (*Source, error) {
	cfg := Config{
		ClientID:         clientID,
		Auth:             authEP,
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
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("%w: ClientID is required", auth.ErrInvalidConfig)
	}
	if cfg.Auth.TokenEndpoint == nil {
		return nil, fmt.Errorf("%w: TokenEndpoint is required", auth.ErrInvalidConfig)
	}
	if cfg.Auth.AuthorizationEndpoint == nil {
		return nil, fmt.Errorf("%w: AuthorizationEndpoint is required", auth.ErrInvalidConfig)
	}
	if cfg.RefreshThreshold == 0 {
		cfg.RefreshThreshold = 30 * time.Second
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

// NewFromCatalog builds a Source from a resolved ServiceCatalog.
func NewFromCatalog(catalog *discovery.ServiceCatalog, clientID string, opts ...Option) (*Source, error) {
	if catalog == nil {
		return nil, fmt.Errorf("%w: catalog is nil", auth.ErrInvalidConfig)
	}
	all := append([]Option{
		WithAuthEndpoints(catalog.Auth),
		WithIssuer(catalog.Issuer),
	}, opts...)
	return New(clientID, catalog.Auth, all...)
}

// AuthorizationRequest holds inputs for building an authorization URL.
type AuthorizationRequest struct {
	State  string
	Launch string
	PKCE   PKCEPair
}

// BeginAuthorization generates PKCE material for a single launch.
//
// If state is empty, a cryptographically random state value is generated
// (32 bytes of entropy, base64url-encoded) and returned in
// [AuthorizationRequest].State. The caller MUST persist req.State and
// compare it against the state parameter on the authorization-code
// callback to defend against CSRF. If state is non-empty it is used
// verbatim — the caller takes responsibility for its strength and
// session binding.
//
// Callers MUST retain the returned [AuthorizationRequest] and pass it
// unchanged to [Source.ExchangeAuthorizationCode]; a Source supports
// many concurrent launches when each flow keeps its own request value.
func (s *Source) BeginAuthorization(state string) (AuthorizationRequest, error) {
	if state == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return AuthorizationRequest{}, fmt.Errorf("smart: generate state: %w", err)
		}
		state = base64URLEncode(b)
	}
	pkce, err := NewPKCEPair()
	if err != nil {
		return AuthorizationRequest{}, err
	}
	return AuthorizationRequest{State: state, PKCE: pkce}, nil
}

// AuthorizeURL builds the SMART authorization redirect URL (REQ-061).
func (s *Source) AuthorizeURL(req AuthorizationRequest, launch string) (string, error) {
	if req.State == "" || req.PKCE.Verifier == "" {
		return "", fmt.Errorf("%w: call BeginAuthorization first or supply State and PKCE", auth.ErrInvalidConfig)
	}
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {s.cfg.ClientID},
		"redirect_uri":          {s.cfg.RedirectURI},
		"code_challenge":        {req.PKCE.Challenge},
		"code_challenge_method": {challengeMethod},
		"state":                 {req.State},
	}
	if len(s.cfg.Scopes) > 0 {
		q.Set("scope", strings.Join(s.cfg.Scopes, " "))
	}
	if s.cfg.Audience != "" {
		q.Set("aud", s.cfg.Audience)
	}
	if launch != "" {
		q.Set("launch", launch)
	}
	u := *s.cfg.Auth.AuthorizationEndpoint
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// ExchangeAuthorizationCode completes the PKCE flow (REQ-061). req
// MUST be the [AuthorizationRequest] returned by [Source.BeginAuthorization]
// for this launch (state + PKCE verifier). The returned [TokenResponse]
// carries SMART launch parameters for smart/ (REQ-064).
func (s *Source) ExchangeAuthorizationCode(ctx context.Context, code string, req AuthorizationRequest) (auth.Token, TokenResponse, error) {
	if req.State == "" || req.PKCE.Verifier == "" {
		return auth.Token{}, TokenResponse{}, fmt.Errorf("%w: AuthorizationRequest from BeginAuthorization is required", auth.ErrInvalidConfig)
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

// Token returns a valid access token, refreshing when near expiry (REQ-063).
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
	if refreshTok == "" && !cur.IsZero() {
		// Stale but no refresh_token — return the cached access token
		// without claiming inflight (REQ-026).
		s.mu.Unlock()
		return cur, nil
	}
	ex := &tokenExchange{done: make(chan struct{})}
	s.inflight = ex
	s.mu.Unlock()

	var tok auth.Token
	var err error
	var refreshedTR TokenResponse
	if refreshTok != "" {
		tok, refreshedTR, refreshTok, err = s.refreshGrant(ctx, refreshTok)
	} else {
		err = &auth.ExchangeError{Sentinel: auth.ErrReauthRequired, Inner: fmt.Errorf("no token or refresh_token")}
	}

	s.mu.Lock()
	if err == nil {
		s.cur = tok
		s.refresh = refreshTok
		if refreshedTR.AccessToken != "" {
			s.lastTR = refreshedTR
		}
	}
	s.inflight = nil
	s.mu.Unlock()
	ex.token = tok
	ex.err = err
	close(ex.done)
	return tok, err
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.Auth.TokenEndpoint.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return auth.Token{}, TokenResponse{}, "", &auth.ExchangeError{Sentinel: auth.ErrTokenExchangeFailed, Inner: err}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if s.cfg.ClientSecret != "" {
		req.SetBasicAuth(s.cfg.ClientID, s.cfg.ClientSecret)
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
		return auth.Token{}, TokenResponse{}, "", &auth.ExchangeError{Sentinel: auth.ErrTokenExchangeFailed, StatusCode: resp.StatusCode, Inner: fmt.Errorf("empty access_token")}
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

// JWKS returns the JWKS helper when configured (REQ-062).
func (s *Source) JWKS() *JWKS { return s.cfg.JWKS }
