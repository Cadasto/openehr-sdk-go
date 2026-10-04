package discovery

import (
	"context"
	"encoding/json"
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
)

// SpecVersionPin is the SDK's pinned openEHR REST contract version.
// The Resolver checks the spec_version a required service advertises
// against this version, unless the caller sets WithAcceptedSpecVersions,
// whose list then replaces it.
const SpecVersionPin = "1.1.0-development"

// WellKnownPath is the standard SMART configuration path appended to
// the Platform base URL, per SMART App Launch §4.1. Some deployments
// expose it under a different prefix; callers can override it with
// WithWellKnownPath when constructing the Resolver.
const WellKnownPath = "/.well-known/smart-configuration"

// DefaultTTL is applied when the discovery document does not advertise
// an explicit Cache-Control max-age.
const DefaultTTL = 15 * time.Minute

// openIDConfigurationPath is the OpenID Connect discovery path, appended
// to the issuer's own path (OpenID Connect Discovery 1.0 §4).
const openIDConfigurationPath = "/.well-known/openid-configuration"

// maxDocumentBytes caps how much of a discovery document the resolver
// reads.
const maxDocumentBytes = 1 << 20

// Resolver fetches, validates, caches, and refreshes SMART
// configuration documents for one or more Platform base URLs.
//
// A single Resolver instance is safe for concurrent use across many
// goroutines; concurrent Resolve()/Refresh() calls for the same base URL
// coalesce around one in-flight fetch.
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
	httpClient             *http.Client
	requiredServices       []string
	acceptedVersions       map[string]struct{}
	acceptedVersionsLocked bool // true when caller explicitly called WithAcceptedSpecVersions
	defaultTTL             time.Duration
	allowInsecure          bool
	skipOpenIDCheck        bool // true when caller called WithoutOpenIDConfigurationCheck
	logger                 *slog.Logger
	wellKnownPath          string
}

// Option mutates a Resolver during construction.
type Option func(*resolverConfig)

// WithHTTPClient injects the *http.Client used for discovery fetches.
// Required.
func WithHTTPClient(c *http.Client) Option {
	return func(cfg *resolverConfig) { cfg.httpClient = c }
}

// WithRequiredServices configures which service IDs must be present in
// every resolved catalog. Default is ["org.openehr.rest"].
func WithRequiredServices(ids ...string) Option {
	return func(cfg *resolverConfig) {
		cfg.requiredServices = append(cfg.requiredServices[:0], ids...)
	}
}

// WithAcceptedSpecVersions sets the versions the resolver accepts on a
// required service, which are {SpecVersionPin} by default. The list
// replaces the pinned version rather than adding to it, so a caller who
// still accepts SpecVersionPin names it too:
// WithAcceptedSpecVersions(SpecVersionPin, "1.1.0").
//
// Without this option the resolver compares a required service's
// spec_version only when the entry advertises one, and never compares its
// version member, which is usually the Platform's own API version. Calling
// it makes the check strict: the compared value is the entry's
// spec_version, or its version when it advertises no spec_version, and an
// entry that advertises neither is rejected unless "" is in the accepted
// set.
func WithAcceptedSpecVersions(versions ...string) Option {
	return func(cfg *resolverConfig) {
		cfg.acceptedVersions = map[string]struct{}{}
		for _, v := range versions {
			cfg.acceptedVersions[v] = struct{}{}
		}
		cfg.acceptedVersionsLocked = true
	}
}

// WithDefaultTTL overrides the cache TTL applied when the discovery
// document does not advertise one.
func WithDefaultTTL(d time.Duration) Option {
	return func(cfg *resolverConfig) { cfg.defaultTTL = d }
}

// WithAllowInsecure permits http:// base URLs, issuers and auth endpoint
// URLs. Default is to refuse plaintext. Use only for local development.
func WithAllowInsecure() Option {
	return func(cfg *resolverConfig) { cfg.allowInsecure = true }
}

// WithoutOpenIDConfigurationCheck turns off the check the resolver makes
// when a SMART configuration names an issuer other than the base URL it
// was resolved from. By default the resolver then fetches the issuer's
// own OpenID configuration, <issuer>/.well-known/openid-configuration,
// and requires it to name the same issuer and, when both documents give
// one, the same jwks_uri. Turn the check off for a Platform whose identity
// provider publishes no OpenID configuration; the declared issuer is then
// accepted as it stands, provided it is well formed and uses https, or
// http with WithAllowInsecure.
func WithoutOpenIDConfigurationCheck() Option {
	return func(cfg *resolverConfig) { cfg.skipOpenIDCheck = true }
}

// WithLogger sets the slog.Logger that warnings (TLS posture, etc.)
// are emitted to. Default is slog.Default().
func WithLogger(l *slog.Logger) Option {
	return func(cfg *resolverConfig) { cfg.logger = l }
}

// WithWellKnownPath overrides the path appended to the base URL when
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
		if o != nil {
			o(&cfg)
		}
	}
	if cfg.httpClient == nil {
		return nil, fmt.Errorf("discovery: %w", &DiscoveryError{Reason: ReasonFetchFailed, Inner: errors.New("HTTPClient is required (REQ-021)")})
	}
	if cfg.logger == nil {
		cfg.logger = slog.Default()
	}
	cfg.httpClient = refuseDowngrade(cfg.httpClient, cfg.allowInsecure)
	if cache == nil {
		cache = NewMemoryCache()
	}
	return &Resolver{
		cfg:      cfg,
		cache:    cache,
		inflight: map[string]*resolveCall{},
	}, nil
}

// errInsecureRedirect marks a redirect the resolver refused because its
// target is not an https URL.
var errInsecureRedirect = errors.New("redirect to a non-https URL refused; use WithAllowInsecure for development")

// refuseDowngrade returns the client the resolver fetches with. Unless
// allowInsecure, it is a shallow copy of c whose redirect policy refuses a
// redirect to a URL that is not https, and otherwise applies c's own
// CheckRedirect, or net/http's default limit of 10 redirects when c has
// none. c itself is never modified.
func refuseDowngrade(c *http.Client, allowInsecure bool) *http.Client {
	if allowInsecure {
		return c
	}
	cp := *c
	callerPolicy := c.CheckRedirect
	cp.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return errInsecureRedirect
		}
		if callerPolicy != nil {
			return callerPolicy(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &cp
}

// requestFailure classifies a request that returned no usable response: a
// redirect refused for leaving https is ReasonInsecureURL, anything else
// ReasonFetchFailed.
func requestFailure(err error) DiscoveryErrorReason {
	if errors.Is(err, errInsecureRedirect) {
		return ReasonInsecureURL
	}
	return ReasonFetchFailed
}

// Resolve returns the catalog for the Platform at baseURL: the cached one
// when fresh, otherwise a newly fetched one, which it caches under
// baseURL. Concurrent calls coalesce, so exactly one fetch happens per
// base URL while a fetch is in flight.
//
// baseURL is the Platform base URL: the SMART configuration is served at
// <baseURL>/.well-known/smart-configuration, and an embedded SMART launch
// passes the same URL to the app as its "iss" parameter. It is not
// necessarily the OpenID Connect issuer. When the document names another
// issuer, that issuer becomes the catalog's Issuer and baseURL stays its
// BaseURL; unless the Resolver is built with
// WithoutOpenIDConfigurationCheck, the issuer must first be confirmed by
// its own OpenID configuration.
func (r *Resolver) Resolve(ctx context.Context, baseURL string) (*ServiceCatalog, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cat, ok := r.cache.Get(ctx, baseURL); ok && !cat.Stale(time.Now()) {
		return cat, nil
	}
	return r.fetchCoalesced(ctx, baseURL, "")
}

// Refresh invalidates any cached catalog for baseURL and forces a fresh
// fetch, with the same checks as Resolve. baseURL is the Platform base
// URL, as for Resolve.
func (r *Resolver) Refresh(ctx context.Context, baseURL string) (*ServiceCatalog, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var prevETag string
	if cat, ok := r.cache.Get(ctx, baseURL); ok {
		prevETag = cat.ETag
	}
	if err := r.cache.Invalidate(ctx, baseURL); err != nil {
		return nil, err
	}
	return r.fetchCoalesced(ctx, baseURL, prevETag)
}

// fetchCoalesced runs at most one in-flight fetch per base URL; other
// callers for the same base URL wait for its result. The fetch runs under
// the context of the caller that started it: if that context ends, the
// fetch fails with the context's error and every waiter receives that same
// error. A waiter whose own context ends first stops waiting and returns
// its own context's error.
func (r *Resolver) fetchCoalesced(ctx context.Context, baseURL, prevETag string) (*ServiceCatalog, error) {
	r.mu.Lock()
	if call, ok := r.inflight[baseURL]; ok {
		r.mu.Unlock()
		select {
		case <-call.done:
			return call.catalog, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &resolveCall{done: make(chan struct{})}
	r.inflight[baseURL] = call
	r.mu.Unlock()

	cat, err := r.fetch(ctx, baseURL, prevETag)

	r.mu.Lock()
	delete(r.inflight, baseURL)
	r.mu.Unlock()

	if err == nil && cat != nil {
		if perr := r.cache.Put(ctx, baseURL, cat); perr != nil {
			r.cfg.logger.Warn("discovery: cache put failed", "base_url", baseURL, "err", perr)
		}
	}
	call.catalog = cat
	call.err = err
	close(call.done)
	return cat, err
}

func (r *Resolver) fetch(ctx context.Context, baseURL, prevETag string) (*ServiceCatalog, error) {
	docURL, err := joinURL(baseURL, r.cfg.wellKnownPath)
	if err != nil {
		return nil, &DiscoveryError{Issuer: baseURL, Reason: ReasonMalformedURL, Inner: err}
	}
	// Decide on the parsed scheme, which url.Parse lowercases: URL schemes
	// are case-insensitive, so "HTTP://" is as plaintext as "http://".
	if !r.cfg.allowInsecure && docURL.Scheme == "http" {
		return nil, &DiscoveryError{Issuer: baseURL, Reason: ReasonInsecureURL, Inner: errors.New("plaintext base URL rejected; use WithAllowInsecure for development")}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, docURL.String(), nil)
	if err != nil {
		return nil, &DiscoveryError{Issuer: baseURL, Reason: ReasonFetchFailed, Inner: err}
	}
	req.Header.Set("Accept", "application/json")
	if prevETag != "" {
		req.Header.Set("If-None-Match", prevETag)
	}

	resp, err := r.cfg.httpClient.Do(req)
	if err != nil {
		return nil, &DiscoveryError{Issuer: baseURL, Reason: requestFailure(err), Inner: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified {
		// A 304 answers only a conditional request. Without If-None-Match
		// the server sent no document, and asking again would loop.
		if prevETag == "" {
			return nil, &DiscoveryError{Issuer: baseURL, Reason: ReasonFetchFailed, Inner: errors.New("discovery fetch returned 304 to a request without If-None-Match")}
		}
		// Refresh invalidated the cache entry the ETag came from, so the
		// unchanged document is no longer at hand. Ask once more without
		// If-None-Match; a second 304 ends in the branch above.
		return r.fetch(ctx, baseURL, "")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &DiscoveryError{Issuer: baseURL, Reason: ReasonFetchFailed, Inner: fmt.Errorf("discovery fetch returned %d", resp.StatusCode)}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDocumentBytes))
	if err != nil {
		return nil, &DiscoveryError{Issuer: baseURL, Reason: ReasonFetchFailed, Inner: err}
	}
	var wire smartConfigWire
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, &DiscoveryError{Issuer: baseURL, Reason: ReasonParseError, Inner: err}
	}
	cat, err := r.parse(baseURL, &wire)
	if err != nil {
		return nil, err
	}
	cat.ETag = resp.Header.Get("ETag")
	cat.ResolvedAt = time.Now()
	cat.ExpiresAt = computeExpiry(resp.Header, r.cfg.defaultTTL, cat.ResolvedAt)
	if err := r.validate(cat); err != nil {
		return nil, err
	}
	if err := r.checkOpenIDConfiguration(ctx, cat, wire.JWKSURI); err != nil {
		return nil, err
	}
	r.warnInsecure(cat)
	return cat, nil
}

// checkOpenIDConfiguration confirms a declared issuer that differs from the
// base URL against the issuer's own OpenID configuration (OpenID Connect
// Discovery 1.0 §4). That document is fetched from a URL built from the
// issuer, so its "issuer" must equal the declared one exactly (§4.3); when
// both documents give a jwks_uri, the two must be equal as well, so ID tokens
// are checked against the keys the issuer itself publishes. smartJWKSURI is
// the SMART configuration's jwks_uri as written.
func (r *Resolver) checkOpenIDConfiguration(ctx context.Context, cat *ServiceCatalog, smartJWKSURI string) error {
	if r.cfg.skipOpenIDCheck || cat.Issuer == cat.BaseURL {
		return nil
	}
	fail := func(reason DiscoveryErrorReason, err error) error {
		return &DiscoveryError{Issuer: cat.BaseURL, Reason: reason, Inner: err}
	}
	docURL, err := joinURL(cat.Issuer, openIDConfigurationPath)
	if err != nil {
		return fail(ReasonMalformedURL, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, docURL.String(), nil)
	if err != nil {
		return fail(ReasonFetchFailed, err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := r.cfg.httpClient.Do(req)
	if err != nil {
		return fail(requestFailure(err), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fail(ReasonFetchFailed, fmt.Errorf("openid-configuration fetch returned %d", resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDocumentBytes))
	if err != nil {
		return fail(ReasonFetchFailed, err)
	}
	var doc struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return fail(ReasonFetchFailed, fmt.Errorf("openid-configuration: %w", err))
	}
	// Every OpenID configuration names its issuer. Without one the body is
	// something else, such as a gateway's JSON error page, so the fetch
	// failed; it is not a mismatch.
	if doc.Issuer == "" {
		return fail(ReasonFetchFailed, errors.New("openid-configuration has no issuer"))
	}
	if doc.Issuer != cat.Issuer {
		return fail(ReasonIssuerMismatch, fmt.Errorf("openid-configuration issuer %q does not equal the declared issuer %q", doc.Issuer, cat.Issuer))
	}
	if doc.JWKSURI != "" && smartJWKSURI != "" && doc.JWKSURI != smartJWKSURI {
		return fail(ReasonIssuerMismatch, fmt.Errorf("openid-configuration jwks_uri %q does not equal the smart-configuration jwks_uri %q", doc.JWKSURI, smartJWKSURI))
	}
	return nil
}

func joinURL(rawBase, path string) (*url.URL, error) {
	base, err := url.Parse(rawBase)
	if err != nil {
		return nil, err
	}
	// Hostname, not Host: url.Parse keeps a lone port such as ":8443" in
	// Host, which leaves no host name to connect to.
	if base.Scheme == "" || base.Hostname() == "" {
		return nil, fmt.Errorf("%q is not an absolute URL with a host", rawBase)
	}
	ref, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	// Append the configured path to the base's path rather than resolving
	// it as a reference: resolving an absolute path would replace the
	// base's path, and a deployment whose base URL has a path serves the
	// document under that path. The query and fragment still come from the
	// configured path, never from the base.
	doc := base.JoinPath(ref.EscapedPath())
	doc.RawQuery, doc.ForceQuery = ref.RawQuery, ref.ForceQuery
	doc.Fragment, doc.RawFragment = ref.Fragment, ref.RawFragment
	return doc, nil
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
//
// The canonical openEHR SMART spec defines "services" as a JSON object/hash
// map keyed by reverse-domain id, each value carrying camelCase "baseUrl".
// See ADR 0008 for the decision to adopt the canonical map shape and drop
// the non-canonical array form.
type smartConfigWire struct {
	Issuer                                     string                      `json:"issuer"`
	AuthorizationEndpoint                      string                      `json:"authorization_endpoint"`
	TokenEndpoint                              string                      `json:"token_endpoint"`
	JWKSURI                                    string                      `json:"jwks_uri"`
	RegistrationEndpoint                       string                      `json:"registration_endpoint"`
	IntrospectionEndpoint                      string                      `json:"introspection_endpoint"`
	RevocationEndpoint                         string                      `json:"revocation_endpoint"`
	ManagementEndpoint                         string                      `json:"management_endpoint"`
	ScopesSupported                            []string                    `json:"scopes_supported"`
	ResponseTypesSupported                     []string                    `json:"response_types_supported"`
	CodeChallengeMethodsSupported              []string                    `json:"code_challenge_methods_supported"`
	GrantTypesSupported                        []string                    `json:"grant_types_supported"`
	TokenEndpointAuthMethodsSupported          []string                    `json:"token_endpoint_auth_methods_supported"`
	TokenEndpointAuthSigningAlgValuesSupported []string                    `json:"token_endpoint_auth_signing_alg_values_supported"`
	IDTokenSigningAlgValuesSupported           []string                    `json:"id_token_signing_alg_values_supported"`
	Capabilities                               []string                    `json:"capabilities"`
	Services                                   map[string]serviceEntryWire `json:"services"`
}

type serviceEntryWire struct {
	BaseURL       string   `json:"baseUrl"`
	Version       string   `json:"version"`
	SpecVersion   string   `json:"spec_version"` // non-canonical extension; tolerated when present
	Description   string   `json:"description"`
	Documentation string   `json:"documentation"`
	OpenAPI       string   `json:"openapi"`
	Capabilities  []string `json:"capabilities"`
}

func (r *Resolver) parse(baseURL string, wire *smartConfigWire) (*ServiceCatalog, error) {
	auth, err := parseAuthEndpoints(baseURL, wire, r.cfg.allowInsecure)
	if err != nil {
		return nil, err
	}
	services := map[string]ServiceEntry{}
	for id, s := range wire.Services {
		u, err := url.Parse(s.BaseURL)
		if err != nil || u.Scheme == "" || u.Hostname() == "" {
			return nil, &DiscoveryError{Issuer: baseURL, Reason: ReasonMalformedURL, Inner: fmt.Errorf("service %q baseUrl %q invalid", id, s.BaseURL)}
		}
		services[id] = ServiceEntry{
			ID:            id,
			BaseURL:       u,
			Version:       s.Version,
			SpecVersion:   s.SpecVersion,
			Description:   s.Description,
			Documentation: s.Documentation,
			OpenAPI:       s.OpenAPI,
			Capabilities:  append([]string(nil), s.Capabilities...),
		}
	}
	// The document's issuer becomes the catalog's Issuer whether or not it
	// equals the base URL: SMART App Launch does not require the two to
	// match, and a Platform may delegate sign-in to an identity provider
	// with its own URL. The base URL stays the catalog's BaseURL and cache
	// key. fetch confirms a differing issuer against the issuer's OpenID
	// configuration. An absent issuer resolves to the base URL.
	issuer := baseURL
	if wire.Issuer != "" {
		if err := validateIssuer(baseURL, wire.Issuer, r.cfg.allowInsecure); err != nil {
			return nil, err
		}
		issuer = wire.Issuer
	}
	return &ServiceCatalog{
		BaseURL:  baseURL,
		Issuer:   issuer,
		Services: services,
		Auth:     auth,
	}, nil
}

// validateIssuer checks the shape OpenID Connect Core 1.0 §2 gives an
// issuer: an absolute https URL with a host and without a query or
// fragment. An http issuer passes only with allowInsecure.
func validateIssuer(baseURL, raw string, allowInsecure bool) error {
	malformed := func(err error) error {
		return &DiscoveryError{Issuer: baseURL, Reason: ReasonMalformedURL, Inner: err}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return malformed(fmt.Errorf("issuer: %w", err))
	}
	// url.Parse lowercases the scheme, so these comparisons ignore case.
	if u.Scheme != "https" && u.Scheme != "http" {
		return malformed(fmt.Errorf("issuer %q is not an https URL", raw))
	}
	if u.Hostname() == "" {
		return malformed(fmt.Errorf("issuer %q has no host", raw))
	}
	if u.RawQuery != "" || u.ForceQuery {
		return malformed(fmt.Errorf("issuer %q has a query", raw))
	}
	// A "#" can only open a fragment, and url.Parse drops an empty one.
	if strings.Contains(raw, "#") {
		return malformed(fmt.Errorf("issuer %q has a fragment", raw))
	}
	if u.Scheme == "http" && !allowInsecure {
		return &DiscoveryError{Issuer: baseURL, Reason: ReasonInsecureURL, Inner: fmt.Errorf("issuer %q uses http; https required (use WithAllowInsecure for development)", raw)}
	}
	return nil
}

func parseAuthEndpoints(baseURL string, w *smartConfigWire, allowInsecure bool) (AuthEndpoints, error) {
	var out AuthEndpoints
	parse := func(name, raw string) (*url.URL, error) {
		if raw == "" {
			return nil, nil
		}
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return nil, &DiscoveryError{Issuer: baseURL, Reason: ReasonMalformedURL, Inner: fmt.Errorf("%s %q invalid", name, raw)}
		}
		// url.Parse lowercases the scheme. https is always accepted, http
		// only with allowInsecure, and anything else is not an endpoint.
		switch u.Scheme {
		case "https":
		case "http":
			if !allowInsecure {
				return nil, &DiscoveryError{Issuer: baseURL, Reason: ReasonInsecureURL, Inner: fmt.Errorf("%s uses scheme %q; https required (use WithAllowInsecure for development)", name, u.Scheme)}
			}
		default:
			return nil, &DiscoveryError{Issuer: baseURL, Reason: ReasonMalformedURL, Inner: fmt.Errorf("%s uses scheme %q; https required", name, u.Scheme)}
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
	// Optional endpoints: absence (empty string) → nil, no error.
	if out.IntrospectionEndpoint, err = parse("introspection_endpoint", w.IntrospectionEndpoint); err != nil {
		return out, err
	}
	if out.RevocationEndpoint, err = parse("revocation_endpoint", w.RevocationEndpoint); err != nil {
		return out, err
	}
	if out.ManagementEndpoint, err = parse("management_endpoint", w.ManagementEndpoint); err != nil {
		return out, err
	}
	out.ScopesSupported = append([]string(nil), w.ScopesSupported...)
	out.ResponseTypesSupported = append([]string(nil), w.ResponseTypesSupported...)
	out.CodeChallengeMethodsSupported = append([]string(nil), w.CodeChallengeMethodsSupported...)
	out.GrantTypesSupported = append([]string(nil), w.GrantTypesSupported...)
	out.TokenEndpointAuthMethodsSupported = append([]string(nil), w.TokenEndpointAuthMethodsSupported...)
	out.TokenEndpointAuthSigningAlgValuesSupported = append([]string(nil), w.TokenEndpointAuthSigningAlgValuesSupported...)
	out.IDTokenSigningAlgValuesSupported = append([]string(nil), w.IDTokenSigningAlgValuesSupported...)
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
		return &DiscoveryError{Issuer: cat.BaseURL, Reason: ReasonMissingService, MissingServices: missing}
	}
	// 2. Version match per required service (REQ-072, softened per ADR 0008).
	// An advertised spec_version is always compared. Without one, the entry
	// passes unless the caller locked the accepted set via
	// WithAcceptedSpecVersions; then its canonical version member is
	// compared instead, and an entry with neither fails. The version member
	// is never compared by default, because a Platform advertises its own
	// API version there.
	for _, id := range r.cfg.requiredServices {
		e := cat.Services[id]
		got := e.SpecVersion
		if got == "" {
			if !r.cfg.acceptedVersionsLocked {
				continue
			}
			got = e.Version
		}
		if _, ok := r.cfg.acceptedVersions[got]; !ok {
			return &DiscoveryError{
				Issuer:          cat.BaseURL,
				Reason:          ReasonSpecVersionMismatch,
				SpecVersionGot:  got,
				SpecVersionWant: acceptedVersionsString(r.cfg.acceptedVersions),
			}
		}
	}
	// 3. The authorization-server members SMART App Launch 2.2.0 makes
	//    conditional on what the document advertises.
	if err := missingAuthMember(cat.Auth); err != nil {
		return &DiscoveryError{Issuer: cat.BaseURL, Reason: ReasonAuthEndpointsMissing, Inner: err}
	}
	return nil
}

// SMART App Launch capabilities that make an authorization-server member
// required.
const (
	capabilityLaunchEHR        = "launch-ehr"
	capabilityLaunchStandalone = "launch-standalone"
	capabilitySSOOpenIDConnect = "sso-openid-connect"
)

// missingAuthMember names the first authorization-server member the
// document needs but omits, or returns nil. A document that declares none
// of authorization_endpoint, token_endpoint and jwks_uri, and advertises
// none of launch-ehr, launch-standalone and sso-openid-connect, is an
// anonymous-only deployment and needs none of them. Otherwise
// token_endpoint is always needed; authorization_endpoint only for a user
// launch (launch-ehr, launch-standalone), so a backend-only document may
// leave it out; and jwks_uri for sso-openid-connect.
func missingAuthMember(a AuthEndpoints) error {
	declared := a.AuthorizationEndpoint != nil || a.TokenEndpoint != nil || a.JWKSURI != nil
	authCapability := ""
	for _, c := range []string{capabilityLaunchEHR, capabilityLaunchStandalone, capabilitySSOOpenIDConnect} {
		if slices.Contains(a.Capabilities, c) {
			authCapability = c
			break
		}
	}
	if !declared && authCapability == "" {
		return nil
	}
	if a.TokenEndpoint == nil {
		if !declared {
			return fmt.Errorf("token_endpoint is required by capability %q", authCapability)
		}
		return errors.New("token_endpoint is required when the document declares authorization_endpoint or jwks_uri")
	}
	if a.AuthorizationEndpoint == nil {
		for _, c := range []string{capabilityLaunchEHR, capabilityLaunchStandalone} {
			if slices.Contains(a.Capabilities, c) {
				return fmt.Errorf("authorization_endpoint is required by capability %q", c)
			}
		}
	}
	if a.JWKSURI == nil && slices.Contains(a.Capabilities, capabilitySSOOpenIDConnect) {
		return fmt.Errorf("jwks_uri is required by capability %q", capabilitySSOOpenIDConnect)
	}
	return nil
}

func acceptedVersionsString(m map[string]struct{}) string {
	return strings.Join(slices.Sorted(maps.Keys(m)), ",")
}

// warnInsecure emits a logger warning for each catalog URL that uses
// plaintext http: every auth endpoint and every service baseUrl. It only
// runs for catalogs that passed parsing: without WithAllowInsecure a
// plaintext auth endpoint is refused there, so the auth-endpoint warnings
// cover the WithAllowInsecure path, while service baseUrl entries are
// warn-only — the consumer is authoritative on which deployments they
// want to talk to.
func (r *Resolver) warnInsecure(cat *ServiceCatalog) {
	check := func(name string, u *url.URL) {
		if u == nil {
			return
		}
		if u.Scheme == "http" {
			r.cfg.logger.Warn("discovery: plaintext URL in catalog (REQ-092)", "base_url", cat.BaseURL, "field", name, "url", u.Redacted())
		}
	}
	check("authorization_endpoint", cat.Auth.AuthorizationEndpoint)
	check("token_endpoint", cat.Auth.TokenEndpoint)
	check("jwks_uri", cat.Auth.JWKSURI)
	check("registration_endpoint", cat.Auth.RegistrationEndpoint)
	check("introspection_endpoint", cat.Auth.IntrospectionEndpoint)
	check("revocation_endpoint", cat.Auth.RevocationEndpoint)
	check("management_endpoint", cat.Auth.ManagementEndpoint)
	for id, s := range cat.Services {
		check("services["+id+"].baseUrl", s.BaseURL)
	}
}
