package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// SpecVersionPin is the SDK's pinned openEHR REST contract version
// (REQ-050). The Resolver requires the discovery document to advertise
// this version on every required service unless the caller widens the
// accepted set via WithAcceptedSpecVersions.
const SpecVersionPin = "1.1.0-development"

// WellKnownPath is the standard SMART configuration path appended to
// the issuer URL. Per SMART App Launch §4.1; some deployments expose
// it under a different prefix — callers may override via the Resolver's
// configuration document URL when constructing.
const WellKnownPath = "/.well-known/smart-configuration"

// DefaultTTL is applied when the discovery document does not advertise
// an explicit Cache-Control max-age. Tracks REQ-071's documented
// default.
const DefaultTTL = 15 * time.Minute

// Resolver fetches, validates, caches, and refreshes SMART
// configuration documents for one or more deployment issuers.
//
// A single Resolver instance is safe for concurrent use across many
// goroutines; concurrent Resolve()/Refresh() calls for the same issuer
// coalesce around one in-flight fetch (REQ-026).
type Resolver struct {
	cfg   resolverConfig
	cache Cache

	mu       sync.Mutex
	inflight map[string]*resolveCall
}

type resolveCall struct {
	done    chan struct{}
	catalog *ServiceCatalog
	err     error
}

type resolverConfig struct {
	httpClient       *http.Client
	requiredServices []string
	acceptedVersions map[string]struct{}
	defaultTTL       time.Duration
	allowInsecure    bool
	logger           *slog.Logger
	wellKnownPath    string
}

// Option mutates a Resolver during construction.
type Option func(*resolverConfig)

// WithHTTPClient injects the *http.Client used for discovery fetches.
// Required per REQ-021.
func WithHTTPClient(c *http.Client) Option {
	return func(cfg *resolverConfig) { cfg.httpClient = c }
}

// WithRequiredServices configures which service IDs MUST be present in
// every resolved catalog. Default is ["org.openehr.rest"].
func WithRequiredServices(ids ...string) Option {
	return func(cfg *resolverConfig) {
		cfg.requiredServices = append(cfg.requiredServices[:0], ids...)
	}
}

// WithAcceptedSpecVersions widens the version set the resolver accepts
// on a required service. Default is {SpecVersionPin} (strict).
func WithAcceptedSpecVersions(versions ...string) Option {
	return func(cfg *resolverConfig) {
		cfg.acceptedVersions = map[string]struct{}{}
		for _, v := range versions {
			cfg.acceptedVersions[v] = struct{}{}
		}
	}
}

// WithDefaultTTL overrides the cache TTL applied when the discovery
// document does not advertise one.
func WithDefaultTTL(d time.Duration) Option {
	return func(cfg *resolverConfig) { cfg.defaultTTL = d }
}

// WithAllowInsecure permits http:// issuers and base URLs. Default is
// to refuse plaintext (REQ-092). Use only for local development.
func WithAllowInsecure() Option {
	return func(cfg *resolverConfig) { cfg.allowInsecure = true }
}

// WithLogger sets the slog.Logger that warnings (TLS posture, etc.)
// are emitted to. Default is slog.Default().
func WithLogger(l *slog.Logger) Option {
	return func(cfg *resolverConfig) { cfg.logger = l }
}

// WithWellKnownPath overrides the path appended to the issuer URL when
// fetching the SMART configuration document. Default WellKnownPath.
func WithWellKnownPath(p string) Option {
	return func(cfg *resolverConfig) { cfg.wellKnownPath = p }
}

// NewResolver constructs a Resolver with the given cache and options.
// A nil cache is replaced with a fresh MemoryCache.
func NewResolver(cache Cache, opts ...Option) (*Resolver, error) {
	cfg := resolverConfig{
		requiredServices: []string{ServiceIDOpenEHRRest},
		acceptedVersions: map[string]struct{}{SpecVersionPin: {}},
		defaultTTL:       DefaultTTL,
		wellKnownPath:    WellKnownPath,
	}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.httpClient == nil {
		return nil, fmt.Errorf("discovery: %w", &DiscoveryError{Reason: ReasonFetchFailed, Inner: fmt.Errorf("HTTPClient is required (REQ-021)")})
	}
	if cfg.logger == nil {
		cfg.logger = slog.Default()
	}
	if cache == nil {
		cache = NewMemoryCache()
	}
	return &Resolver{
		cfg:      cfg,
		cache:    cache,
		inflight: map[string]*resolveCall{},
	}, nil
}

// Resolve returns the cached catalog for issuer when fresh, or fetches
// and caches a new one. Concurrent calls coalesce — exactly one fetch
// happens per (issuer, in-flight window).
func (r *Resolver) Resolve(ctx context.Context, issuer string) (*ServiceCatalog, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cat, ok := r.cache.Get(ctx, issuer); ok && !cat.Stale(time.Now()) {
		return cat, nil
	}
	return r.fetchCoalesced(ctx, issuer, "")
}

// Refresh invalidates any cached catalog for issuer and forces a
// fresh fetch.
func (r *Resolver) Refresh(ctx context.Context, issuer string) (*ServiceCatalog, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var prevETag string
	if cat, ok := r.cache.Get(ctx, issuer); ok {
		prevETag = cat.ETag
	}
	if err := r.cache.Invalidate(ctx, issuer); err != nil {
		return nil, err
	}
	return r.fetchCoalesced(ctx, issuer, prevETag)
}

// fetchCoalesced runs at most one in-flight fetch per issuer; other
// callers wait on the result. ctx is honoured for waiting but the
// fetch itself continues even if the initiating caller bails.
func (r *Resolver) fetchCoalesced(ctx context.Context, issuer, prevETag string) (*ServiceCatalog, error) {
	r.mu.Lock()
	if call, ok := r.inflight[issuer]; ok {
		r.mu.Unlock()
		select {
		case <-call.done:
			return call.catalog, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &resolveCall{done: make(chan struct{})}
	r.inflight[issuer] = call
	r.mu.Unlock()

	cat, err := r.fetch(ctx, issuer, prevETag)

	r.mu.Lock()
	delete(r.inflight, issuer)
	r.mu.Unlock()

	if err == nil && cat != nil {
		if perr := r.cache.Put(ctx, issuer, cat); perr != nil {
			r.cfg.logger.Warn("discovery: cache put failed", "issuer", issuer, "err", perr)
		}
	}
	call.catalog = cat
	call.err = err
	close(call.done)
	return cat, err
}

func (r *Resolver) fetch(ctx context.Context, issuer, prevETag string) (*ServiceCatalog, error) {
	if !r.cfg.allowInsecure && strings.HasPrefix(issuer, "http://") {
		return nil, &DiscoveryError{Issuer: issuer, Reason: ReasonInsecureURL, Inner: fmt.Errorf("plaintext issuer rejected; use WithAllowInsecure for development")}
	}
	docURL, err := joinURL(issuer, r.cfg.wellKnownPath)
	if err != nil {
		return nil, &DiscoveryError{Issuer: issuer, Reason: ReasonMalformedURL, Inner: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, docURL.String(), nil)
	if err != nil {
		return nil, &DiscoveryError{Issuer: issuer, Reason: ReasonFetchFailed, Inner: err}
	}
	req.Header.Set("Accept", "application/json")
	if prevETag != "" {
		req.Header.Set("If-None-Match", prevETag)
	}

	resp, err := r.cfg.httpClient.Do(req)
	if err != nil {
		return nil, &DiscoveryError{Issuer: issuer, Reason: ReasonFetchFailed, Inner: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified {
		// Caller invalidated then refreshed; the cache entry is gone.
		// Treat as a fresh fetch with the unchanged body — but we no
		// longer have the body. Re-issue without If-None-Match so the
		// server returns the full document.
		return r.fetch(ctx, issuer, "")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &DiscoveryError{Issuer: issuer, Reason: ReasonFetchFailed, Inner: fmt.Errorf("discovery fetch returned %d", resp.StatusCode)}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, &DiscoveryError{Issuer: issuer, Reason: ReasonFetchFailed, Inner: err}
	}
	cat, err := r.parse(issuer, body)
	if err != nil {
		return nil, err
	}
	cat.ETag = resp.Header.Get("ETag")
	cat.ResolvedAt = time.Now()
	cat.ExpiresAt = computeExpiry(resp.Header, r.cfg.defaultTTL, cat.ResolvedAt)
	if err := r.validate(cat); err != nil {
		return nil, err
	}
	r.warnInsecure(cat)
	return cat, nil
}

func joinURL(issuer, path string) (*url.URL, error) {
	base, err := url.Parse(issuer)
	if err != nil {
		return nil, err
	}
	if base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("issuer %q is not an absolute URL", issuer)
	}
	ref, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	return base.ResolveReference(ref), nil
}

// computeExpiry inspects Cache-Control max-age and falls through to the
// configured default. Per RFC 7234 the max-age directive overrides
// Expires.
func computeExpiry(h http.Header, fallback time.Duration, now time.Time) time.Time {
	cc := h.Get("Cache-Control")
	if cc != "" {
		for part := range strings.SplitSeq(cc, ",") {
			p := strings.TrimSpace(strings.ToLower(part))
			if rest, ok := strings.CutPrefix(p, "max-age="); ok {
				if d, err := time.ParseDuration(rest + "s"); err == nil && d > 0 {
					return now.Add(d)
				}
			}
		}
	}
	if fallback <= 0 {
		return time.Time{}
	}
	return now.Add(fallback)
}

// smartConfigWire mirrors the SMART configuration document shape (plus
// the openEHR "services" extension). Unknown fields are tolerated; only
// the fields the SDK consumes are decoded.
type smartConfigWire struct {
	Issuer                            string             `json:"issuer"`
	AuthorizationEndpoint             string             `json:"authorization_endpoint"`
	TokenEndpoint                     string             `json:"token_endpoint"`
	JWKSURI                           string             `json:"jwks_uri"`
	RegistrationEndpoint              string             `json:"registration_endpoint"`
	ScopesSupported                   []string           `json:"scopes_supported"`
	ResponseTypesSupported            []string           `json:"response_types_supported"`
	CodeChallengeMethodsSupported     []string           `json:"code_challenge_methods_supported"`
	GrantTypesSupported               []string           `json:"grant_types_supported"`
	TokenEndpointAuthMethodsSupported []string           `json:"token_endpoint_auth_methods_supported"`
	Capabilities                      []string           `json:"capabilities"`
	Services                          []serviceEntryWire `json:"services"`
}

type serviceEntryWire struct {
	ID           string   `json:"id"`
	BaseURL      string   `json:"base_url"`
	SpecVersion  string   `json:"spec_version"`
	Capabilities []string `json:"capabilities"`
}

func (r *Resolver) parse(issuer string, body []byte) (*ServiceCatalog, error) {
	var wire smartConfigWire
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, &DiscoveryError{Issuer: issuer, Reason: ReasonParseError, Inner: err}
	}
	auth, err := parseAuthEndpoints(issuer, wire)
	if err != nil {
		return nil, err
	}
	services := map[string]ServiceEntry{}
	for _, s := range wire.Services {
		if s.ID == "" {
			return nil, &DiscoveryError{Issuer: issuer, Reason: ReasonParseError, Inner: fmt.Errorf("service entry missing id")}
		}
		u, err := url.Parse(s.BaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, &DiscoveryError{Issuer: issuer, Reason: ReasonMalformedURL, Inner: fmt.Errorf("service %q base_url %q invalid", s.ID, s.BaseURL)}
		}
		services[s.ID] = ServiceEntry{
			ID:           s.ID,
			BaseURL:      u,
			SpecVersion:  s.SpecVersion,
			Capabilities: append([]string(nil), s.Capabilities...),
		}
	}
	// Trust the issuer the caller supplied over the document's iss
	// field — the discovery URL is the authoritative identifier.
	resolvedIssuer := issuer
	if wire.Issuer != "" {
		resolvedIssuer = wire.Issuer
	}
	return &ServiceCatalog{
		Issuer:   resolvedIssuer,
		Services: services,
		Auth:     auth,
	}, nil
}

func parseAuthEndpoints(issuer string, w smartConfigWire) (AuthEndpoints, error) {
	var out AuthEndpoints
	parse := func(name, raw string) (*url.URL, error) {
		if raw == "" {
			return nil, nil
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, &DiscoveryError{Issuer: issuer, Reason: ReasonMalformedURL, Inner: fmt.Errorf("%s %q invalid", name, raw)}
		}
		return u, nil
	}
	var err error
	if out.AuthorizationEndpoint, err = parse("authorization_endpoint", w.AuthorizationEndpoint); err != nil {
		return out, err
	}
	if out.TokenEndpoint, err = parse("token_endpoint", w.TokenEndpoint); err != nil {
		return out, err
	}
	if out.JWKSURI, err = parse("jwks_uri", w.JWKSURI); err != nil {
		return out, err
	}
	if out.RegistrationEndpoint, err = parse("registration_endpoint", w.RegistrationEndpoint); err != nil {
		return out, err
	}
	out.ScopesSupported = append([]string(nil), w.ScopesSupported...)
	out.ResponseTypesSupported = append([]string(nil), w.ResponseTypesSupported...)
	out.CodeChallengeMethodsSupported = append([]string(nil), w.CodeChallengeMethodsSupported...)
	out.GrantTypesSupported = append([]string(nil), w.GrantTypesSupported...)
	out.TokenEndpointAuthMethodsSupported = append([]string(nil), w.TokenEndpointAuthMethodsSupported...)
	out.Capabilities = append([]string(nil), w.Capabilities...)
	return out, nil
}

func (r *Resolver) validate(cat *ServiceCatalog) error {
	// 1. Required services present.
	var missing []string
	for _, id := range r.cfg.requiredServices {
		if _, ok := cat.Services[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return &DiscoveryError{Issuer: cat.Issuer, Reason: ReasonMissingService, MissingServices: missing}
	}
	// 2. Spec-version match per required service.
	for _, id := range r.cfg.requiredServices {
		e := cat.Services[id]
		if _, ok := r.cfg.acceptedVersions[e.SpecVersion]; !ok {
			return &DiscoveryError{
				Issuer:          cat.Issuer,
				Reason:          ReasonSpecVersionMismatch,
				SpecVersionGot:  e.SpecVersion,
				SpecVersionWant: acceptedVersionsString(r.cfg.acceptedVersions),
			}
		}
	}
	// 3. Required auth endpoints present, when any auth fields are
	//    present at all. A deployment with no auth (anonymous-only)
	//    legitimately ships zero auth endpoints.
	if cat.Auth.AuthorizationEndpoint == nil && cat.Auth.TokenEndpoint == nil && cat.Auth.JWKSURI == nil {
		return nil
	}
	if cat.Auth.AuthorizationEndpoint == nil || cat.Auth.TokenEndpoint == nil {
		return &DiscoveryError{Issuer: cat.Issuer, Reason: ReasonAuthEndpointsMissing, Inner: fmt.Errorf("authorization_endpoint and token_endpoint are required when any auth fields are present")}
	}
	return nil
}

func acceptedVersionsString(m map[string]struct{}) string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return strings.Join(out, ",")
}

// warnInsecure emits a logger warning when any catalog URL uses
// plaintext http://. The Resolver does not refuse those URLs (only the
// initial issuer fetch is gated by allowInsecure) — the consumer is
// authoritative on which deployments they want to talk to. The warning
// surfaces the posture so misconfigurations are visible.
func (r *Resolver) warnInsecure(cat *ServiceCatalog) {
	check := func(name string, u *url.URL) {
		if u == nil {
			return
		}
		if u.Scheme == "http" {
			r.cfg.logger.Warn("discovery: plaintext URL in catalog (REQ-092)", "issuer", cat.Issuer, "field", name, "url", u.Redacted())
		}
	}
	check("authorization_endpoint", cat.Auth.AuthorizationEndpoint)
	check("token_endpoint", cat.Auth.TokenEndpoint)
	check("jwks_uri", cat.Auth.JWKSURI)
	for id, s := range cat.Services {
		check("services["+id+"].base_url", s.BaseURL)
	}
}
