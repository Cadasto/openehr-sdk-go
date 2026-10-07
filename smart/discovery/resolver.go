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
	// ownContextEnded is set when the fetch failed only because the context
	// of the caller that ran it ended. The failure says nothing about the
	// Platform, so a waiter does not take it as its own result. Read only
	// after done is closed.
	ownContextEnded bool
	// abandoned is set when the call ended without a result, because a panic
	// in the fetch or in the Cache call that records its result unwound the
	// caller that ran it. As with ownContextEnded, a waiter does not take the
	// call's empty result as its own. Read only after done is closed.
	abandoned bool
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
// URLs, and redirects to a URL that is not https. By default the resolver
// refuses both. Use only for local development.
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
// baseURL. Concurrent calls for the same base URL share one in-flight
// fetch.
//
// baseURL is the Platform base URL: the SMART configuration is served at
// <baseURL>/.well-known/smart-configuration, and an embedded SMART launch
// passes the same URL to the app as its "iss" parameter. It is not
// necessarily the OpenID Connect issuer. When the document names another
// issuer, that issuer becomes the catalog's Issuer and baseURL stays its
// BaseURL; unless the Resolver is built with
// WithoutOpenIDConfigurationCheck, the issuer must first be confirmed by
// its own OpenID configuration.
//
// When the cached catalog has expired and carries an ETag, the fetch is
// conditional, as for Refresh: a 304 Not Modified renews the cached
// catalog once it passes the same checks as a new document, the issuer
// check included. A failed fetch or check drops the cached catalog, unless
// only the caller's own cancelled or expired context caused the failure,
// which leaves it in place.
func (r *Resolver) Resolve(ctx context.Context, baseURL string) (*ServiceCatalog, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cached, ok := r.cache.Get(ctx, baseURL)
	if ok && !cached.Stale(time.Now()) {
		return cached, nil
	}
	return r.fetchCoalesced(ctx, baseURL, cached)
}

// Refresh fetches the catalog for baseURL again even when the cached one
// is fresh, with the same checks as Resolve. baseURL is the Platform base
// URL, as for Resolve.
//
// When the cached catalog carries an ETag, the request is conditional. A
// 304 Not Modified keeps the cached document, services and auth members,
// and renews the expiry from the response's Cache-Control max-age or the
// default TTL. It does so only once the catalog passes the same checks as
// a new document: this Resolver's validation, and the issuer check against
// the issuer's OpenID configuration. A new document replaces the cached
// catalog. When the refresh fails, the cached catalog is dropped, so the
// next Resolve fetches again and reports the error if that fetch fails
// too. The exception is a refresh that failed only because the caller's own
// context ended: that says nothing about the Platform, so the cached catalog
// stays. Until the refresh completes, Resolve keeps returning the cached
// catalog while it is fresh.
func (r *Resolver) Refresh(ctx context.Context, baseURL string) (*ServiceCatalog, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cached, _ := r.cache.Get(ctx, baseURL)
	return r.fetchCoalesced(ctx, baseURL, cached)
}

// fetchCoalesced runs at most one in-flight fetch per base URL; other
// callers for the same base URL wait for its result. The fetch runs under
// the context of the caller that started it. A waiter whose own context ends
// first stops waiting and returns its own context's error.
//
// When the starter's context ends and that alone fails the fetch, the
// starter returns an error that reports its context's error, and the failure
// says nothing about the Platform. The fetch failed that way when its error
// reports the error of the starter's ended context or the cause that context
// ended with (see endedByContext). Such a failure leaves the cache as it was,
// and a waiter whose own context is still live does not receive it. That
// waiter fetches again, joining the next in-flight fetch for baseURL or, when
// there is none, running one under its own context. It repeats this only
// while its own context is live, and each round is a real fetch, so it ends
// when a fetch succeeds, fails for another reason, or the waiter's context
// ends. A waiter woken by such a failure whose own context has ended too
// returns its own context's error and does not fetch.
//
// Every other failure reaches every waiter as it is. That includes the HTTP
// client's own timeout, whose error reports context.DeadlineExceeded: it
// counts as the starter's own ending only when the starter's context ended
// by its deadline too.
//
// A successful fetch is cached under baseURL; any failure other than the
// starter's own context ending drops whatever was cached there. The cache is
// written before the call leaves the in-flight set, so callers that arrive
// while it is written join the call and get its result, and a later fetch
// for baseURL never has its cache entry overwritten or dropped by an earlier
// one.
//
// A panic in the fetch, or in the Cache call that records its result, is not
// recovered: it goes on to the starter as it was raised. The call still leaves
// the in-flight set, so no caller for baseURL is left waiting on it, and its
// waiters behave as when the starter's context ended: each whose own context
// is live fetches again, and one whose context has ended returns its own
// context's error. After a panic the resolver neither writes nor drops the
// cached entry itself.
//
// cached is the catalog held for baseURL, or nil; fetch uses its ETag for
// a conditional request.
func (r *Resolver) fetchCoalesced(ctx context.Context, baseURL string, cached *ServiceCatalog) (*ServiceCatalog, error) {
	for {
		r.mu.Lock()
		call, joined := r.inflight[baseURL]
		if !joined {
			call = &resolveCall{done: make(chan struct{})}
			r.inflight[baseURL] = call
		}
		r.mu.Unlock()
		if !joined {
			return r.runCall(ctx, baseURL, cached, call)
		}

		select {
		case <-call.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if !call.ownContextEnded && !call.abandoned {
			return call.catalog, call.err
		}
		// The starter's context ended, not this caller's, or a panic ended the
		// call without a result. Never hand the starter's outcome on; give up
		// with this caller's own error when its context ended too, otherwise
		// fetch again.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
}

// runCall runs the fetch for call, which the caller has put in the in-flight
// set, writes the cache, takes call out of the set and publishes its result
// to the waiters.
//
// A panic in the fetch or in the cache write unwinds runCall before the result
// is published. The deferred release still takes call out of the set and
// wakes the waiters, with call marked abandoned so that they fetch again, and
// the panic goes on to the caller as it was raised.
func (r *Resolver) runCall(ctx context.Context, baseURL string, cached *ServiceCatalog, call *resolveCall) (*ServiceCatalog, error) {
	// call stays abandoned unless the fetch and the cache write below return.
	call.abandoned = true
	defer r.release(baseURL, call)

	cat, err := r.fetch(ctx, baseURL, cached)
	ownContextEnded := endedByContext(ctx, err)

	// Write the cache while this call is still in flight. A caller arriving
	// meanwhile joins it, so no later fetch for baseURL can write the cache
	// first and then have this older result overwrite or drop its entry.
	switch {
	case ownContextEnded:
		// The caller gave up; the Platform did not fail. Keep what is cached.
	case err != nil:
		// Drop what was cached, so the next resolution fetches again and
		// reports the failure instead of serving a catalog the Platform no
		// longer vouches for. The caller's context may have ended after the
		// failure, so it must not stop the invalidation.
		if ierr := r.cache.Invalidate(context.WithoutCancel(ctx), baseURL); ierr != nil {
			r.cfg.logger.Warn("discovery: cache invalidate failed", "base_url", baseURL, "err", ierr)
		}
	case cat != nil:
		// The caller may have cancelled after the fetch succeeded. The
		// write still has to land, or the entry from before this fetch
		// stays. Cancellation must not refuse it, as with Invalidate above.
		if perr := r.cache.Put(context.WithoutCancel(ctx), baseURL, cat); perr != nil {
			r.cfg.logger.Warn("discovery: cache put failed", "base_url", baseURL, "err", perr)
		}
	}

	// The fetch's error reports the cause, not ctx's own error, when ctx
	// ended with one. The caller still gets an error that reports its
	// context's error, and the fetch's error with the cause stays inside it.
	if ownContextEnded && !errors.Is(err, ctx.Err()) {
		err = fmt.Errorf("%w: %w", ctx.Err(), err)
	}

	call.catalog = cat
	call.err = err
	call.ownContextEnded = ownContextEnded
	call.abandoned = false
	return cat, err
}

// release takes call out of the in-flight set, then wakes its waiters. runCall
// defers it, so it runs exactly once per call, also when a panic unwinds
// runCall.
func (r *Resolver) release(baseURL string, call *resolveCall) {
	r.mu.Lock()
	delete(r.inflight, baseURL)
	r.mu.Unlock()
	close(call.done)
}

// endedByContext reports whether err means the fetch failed only because ctx
// ended: ctx is done, and err reports ctx's own error or its cause. The cause
// counts because net/http returns context.Cause(ctx) when a request's context
// ends, so a context ended with a cause (context.WithCancelCause,
// context.WithTimeoutCause, errgroup) yields an error that reports the cause
// and not context.Canceled or context.DeadlineExceeded. An error that reports
// some other context error does not count and is a failure of the fetch: the
// HTTP client's own timeout reports context.DeadlineExceeded whether ctx is
// live or was only cancelled.
func endedByContext(ctx context.Context, err error) bool {
	ctxErr := ctx.Err()
	return err != nil && ctxErr != nil &&
		(errors.Is(err, ctxErr) || errors.Is(err, context.Cause(ctx)))
}

// fetch retrieves, validates and confirms the SMART configuration at
// baseURL. When cached carries an ETag, the request is conditional, and a
// 304 Not Modified renews cached instead of building a new catalog; the
// renewed copy is validated and confirmed as a new catalog is.
func (r *Resolver) fetch(ctx context.Context, baseURL string, cached *ServiceCatalog) (*ServiceCatalog, error) {
	var etag string
	if cached != nil {
		etag = cached.ETag
	}
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
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := r.cfg.httpClient.Do(req)
	if err != nil {
		return nil, &DiscoveryError{Issuer: baseURL, Reason: requestFailure(err), Inner: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified {
		// A 304 answers only a conditional request. Without If-None-Match
		// the server sent no document, and asking again could loop.
		if etag == "" {
			return nil, &DiscoveryError{Issuer: baseURL, Reason: ReasonFetchFailed, Inner: errors.New("discovery fetch returned 304 to a request without If-None-Match")}
		}
		// The document is the one cached was built from, but it still goes
		// through this Resolver's checks, as on a 200: the catalog may have
		// been cached by a Resolver with other options, and the issuer's
		// OpenID configuration may have changed since it was confirmed.
		c := renewed(cached, resp.Header, r.cfg.defaultTTL)
		if err := r.refusePlaintext(c); err != nil {
			return nil, err
		}
		if err := r.validate(c); err != nil {
			return nil, err
		}
		smartJWKSURI, parsed := renewedJWKSURI(c)
		if err := r.checkOpenIDConfiguration(ctx, c, smartJWKSURI, parsed); err != nil {
			return nil, err
		}
		r.warnInsecure(c)
		return c, nil
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
	cat.smartJWKSURI = wire.JWKSURI
	cat.ResolvedAt = time.Now()
	cat.ExpiresAt = computeExpiry(resp.Header, r.cfg.defaultTTL, cat.ResolvedAt)
	if err := r.validate(cat); err != nil {
		return nil, err
	}
	if err := r.checkOpenIDConfiguration(ctx, cat, wire.JWKSURI, false); err != nil {
		return nil, err
	}
	r.warnInsecure(cat)
	return cat, nil
}

// renewed returns a copy of cached that is fresh again after a 304 Not
// Modified: the same document, services and auth members, the same
// jwks_uri as written, a new ResolvedAt, and an ExpiresAt from the
// response's Cache-Control max-age, or the default TTL. cached itself is
// not changed, because callers may hold it.
func renewed(cached *ServiceCatalog, h http.Header, ttl time.Duration) *ServiceCatalog {
	c := *cached
	c.ResolvedAt = time.Now()
	c.ExpiresAt = computeExpiry(h, ttl, c.ResolvedAt)
	return &c
}

// renewedJWKSURI returns the SMART configuration's jwks_uri for a catalog
// renewed by a 304 Not Modified: the value as written when the catalog
// kept it, or else the parsed Auth.JWKSURI, because a catalog that came
// back from a cache that keeps exported fields only has lost the former.
// parsed reports the second case, in which the OpenID configuration's
// jwks_uri must be parsed the same way before the two are compared.
func renewedJWKSURI(c *ServiceCatalog) (jwksURI string, parsed bool) {
	if c.smartJWKSURI == "" && c.Auth.JWKSURI != nil {
		return c.Auth.JWKSURI.String(), true
	}
	return c.smartJWKSURI, false
}

// sameJWKSURI reports whether the OpenID configuration's jwks_uri equals the
// SMART configuration's. smart is the value as written, or, when parsed is
// true, a parsed URL's String; then openID is parsed too, so a scheme or host
// that only the parser rewrote does not count as a difference.
func sameJWKSURI(openID, smart string, parsed bool) bool {
	if openID == smart {
		return true
	}
	if !parsed {
		return false
	}
	u, err := url.Parse(openID)
	return err == nil && u.String() == smart
}

// checkOpenIDConfiguration confirms a declared issuer that differs from the
// base URL against the issuer's own OpenID configuration (OpenID Connect
// Discovery 1.0 §4). That document is fetched from a URL built from the
// issuer, so its "issuer" must equal the declared one exactly (§4.3); when
// both documents give a jwks_uri, the two must be equal as well, so ID tokens
// are checked against the keys the issuer itself publishes. smartJWKSURI is
// the SMART configuration's jwks_uri as written, or its parsed form when
// parsed is true (see sameJWKSURI).
func (r *Resolver) checkOpenIDConfiguration(ctx context.Context, cat *ServiceCatalog, smartJWKSURI string, parsed bool) error {
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
	if doc.JWKSURI != "" && smartJWKSURI != "" && !sameJWKSURI(doc.JWKSURI, smartJWKSURI, parsed) {
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
	AuthorizationResponseIssParameterSupported bool                        `json:"authorization_response_iss_parameter_supported"`
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
	out.AuthorizationResponseIssParameterSupported = w.AuthorizationResponseIssParameterSupported
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
	// 3. Authorization-server members (REQ-072). SMART App Launch 2.2.0
	//    requires token_endpoint; the anonymous-only relaxation is the
	//    SDK's own choice.
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
	for _, e := range authURLs(cat.Auth) {
		check(e.name, e.url)
	}
	for id, s := range cat.Services {
		check("services["+id+"].baseUrl", s.BaseURL)
	}
}

// refusePlaintext applies the parser's https rule to a catalog this Resolver
// did not parse: unless it allows insecure URLs, an http issuer or auth
// endpoint is a ReasonInsecureURL. A 304 renewal needs it, because the
// cached catalog may come from a Resolver built with WithAllowInsecure.
func (r *Resolver) refusePlaintext(cat *ServiceCatalog) error {
	if r.cfg.allowInsecure {
		return nil
	}
	refuse := func(name, raw string) error {
		return &DiscoveryError{Issuer: cat.BaseURL, Reason: ReasonInsecureURL, Inner: fmt.Errorf("%s %q uses http; https required (use WithAllowInsecure for development)", name, raw)}
	}
	if u, err := url.Parse(cat.Issuer); err == nil && u.Scheme == "http" {
		return refuse("issuer", cat.Issuer)
	}
	for _, e := range authURLs(cat.Auth) {
		if e.url != nil && e.url.Scheme == "http" {
			return refuse(e.name, e.url.Redacted())
		}
	}
	return nil
}

// authURLs lists the auth endpoints of a, each with the name of its member
// in the SMART configuration document; an absent endpoint is nil.
func authURLs(a AuthEndpoints) []struct {
	name string
	url  *url.URL
} {
	return []struct {
		name string
		url  *url.URL
	}{
		{"authorization_endpoint", a.AuthorizationEndpoint},
		{"token_endpoint", a.TokenEndpoint},
		{"jwks_uri", a.JWKSURI},
		{"registration_endpoint", a.RegistrationEndpoint},
		{"introspection_endpoint", a.IntrospectionEndpoint},
		{"revocation_endpoint", a.RevocationEndpoint},
		{"management_endpoint", a.ManagementEndpoint},
	}
}
