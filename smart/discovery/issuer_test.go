package discovery_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

const (
	// platformPath is the Platform base URL's path on the stub server.
	platformPath = "/platform"
	// idpPath is the path of an issuer that differs from the base URL.
	idpPath = "/idp"
	// openIDWellKnownPath is the OpenID Connect discovery path (OIDC
	// Discovery 1.0 §4), appended to the issuer's own path.
	openIDWellKnownPath = "/.well-known/openid-configuration"

	smartDocPath  = platformPath + discovery.WellKnownPath
	openIDDocPath = idpPath + openIDWellKnownPath
)

// documentFunc returns the status and body the stub serves for one
// document. origin is the server's scheme and host, so a document can name
// URLs on the same server.
type documentFunc func(origin string) (status int, body string)

// stubPlatform is one test server that serves the SMART configuration under
// platformPath and the OpenID configuration under idpPath, so the Platform
// base URL and the issuer differ while one certificate is trusted. It records
// the path and Accept header of every request.
type stubPlatform struct {
	srv *httptest.Server

	mu      sync.Mutex
	paths   []string
	accepts []string
}

func startPlatform(t *testing.T, plaintext bool, smart, openID documentFunc) *stubPlatform {
	t.Helper()
	p := &stubPlatform{}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.paths = append(p.paths, r.URL.Path)
		p.accepts = append(p.accepts, r.Header.Get("Accept"))
		p.mu.Unlock()
		var doc documentFunc
		switch r.URL.Path {
		case smartDocPath:
			doc = smart
		case openIDDocPath:
			doc = openID
		}
		if doc == nil {
			http.NotFound(w, r)
			return
		}
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}
		status, body := doc(scheme + "://" + r.Host)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
	if plaintext {
		p.srv = httptest.NewServer(h)
	} else {
		p.srv = httptest.NewTLSServer(h)
	}
	t.Cleanup(p.srv.Close)
	return p
}

// baseURL is the Platform base URL the tests resolve.
func (p *stubPlatform) baseURL() string { return p.srv.URL + platformPath }

// issuer is the OIDC issuer that differs from the base URL.
func (p *stubPlatform) issuer() string { return p.srv.URL + idpPath }

// requests returns the paths and Accept headers received so far.
func (p *stubPlatform) requests() (paths, accepts []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.paths), slices.Clone(p.accepts)
}

func (p *stubPlatform) resolver(t *testing.T, opts ...discovery.Option) *discovery.Resolver {
	t.Helper()
	res, err := discovery.NewResolver(nil, append([]discovery.Option{discovery.WithHTTPClient(p.srv.Client())}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// smartDocument is a SMART configuration document that advertises the
// openEHR REST service and declares issuer and jwks_uri, each left out when
// empty.
func smartDocument(issuer, jwksURI string) string {
	doc := map[string]any{
		"authorization_endpoint": "https://auth.example.com/authorize",
		"token_endpoint":         "https://auth.example.com/token",
		"services": map[string]any{
			discovery.ServiceIDOpenEHRRest: map[string]any{"baseUrl": "https://api.example.com/openehr/v1"},
		},
	}
	if issuer != "" {
		doc["issuer"] = issuer
	}
	if jwksURI != "" {
		doc["jwks_uri"] = jwksURI
	}
	return mustJSON(doc)
}

// openIDDocument is an OpenID configuration document that declares issuer
// and jwks_uri, each left out when empty.
func openIDDocument(issuer, jwksURI string) string {
	doc := map[string]any{"token_endpoint": "https://auth.example.com/token"}
	if issuer != "" {
		doc["issuer"] = issuer
	}
	if jwksURI != "" {
		doc["jwks_uri"] = jwksURI
	}
	return mustJSON(doc)
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// serve answers 200 with the body built from the origin.
func serve(body func(origin string) string) documentFunc {
	return func(origin string) (int, string) { return http.StatusOK, body(origin) }
}

// notFound answers 404, so a resolver that fetches the document fails.
func notFound(string) (int, string) { return http.StatusNotFound, `{"error":"not found"}` }

// TestResolveSeparatesBaseURLFromIssuer pins REQ-070 and REQ-073: the catalog
// carries the base URL the caller resolved and the document's issuer as two
// values, the issuer falling back to the base URL when the document declares
// none. The OpenID configuration is fetched only for an issuer that differs
// from the base URL, and never when the check is turned off.
func TestResolveSeparatesBaseURLFromIssuer(t *testing.T) { // REQ-070, REQ-073
	tests := []struct {
		name       string
		issuer     func(origin string) string // the declared issuer; empty means none
		opts       []discovery.Option
		openID     documentFunc
		wantIssuer func(origin string) string
		wantPaths  []string
	}{
		{
			name:       "no declared issuer",
			issuer:     func(string) string { return "" },
			openID:     notFound,
			wantIssuer: func(o string) string { return o + platformPath },
			wantPaths:  []string{smartDocPath},
		},
		{
			name:       "issuer equal to the base URL",
			issuer:     func(o string) string { return o + platformPath },
			openID:     notFound,
			wantIssuer: func(o string) string { return o + platformPath },
			wantPaths:  []string{smartDocPath},
		},
		{
			name:   "issuer that differs, confirmed by its OpenID configuration",
			issuer: func(o string) string { return o + idpPath },
			openID: serve(func(o string) string {
				return openIDDocument(o+idpPath, "")
			}),
			wantIssuer: func(o string) string { return o + idpPath },
			wantPaths:  []string{smartDocPath, openIDDocPath},
		},
		{
			name:       "issuer that differs, check turned off",
			issuer:     func(o string) string { return o + idpPath },
			opts:       []discovery.Option{discovery.WithoutOpenIDConfigurationCheck()},
			openID:     notFound,
			wantIssuer: func(o string) string { return o + idpPath },
			wantPaths:  []string{smartDocPath},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := startPlatform(t, false, serve(func(o string) string {
				return smartDocument(tc.issuer(o), "")
			}), tc.openID)
			cat, err := p.resolver(t, tc.opts...).Resolve(t.Context(), p.baseURL())
			paths, _ := p.requests()
			if err != nil {
				t.Fatalf("Resolve(%q) error = %v (requested paths %q)", p.baseURL(), err, paths)
			}
			if cat.BaseURL != p.baseURL() {
				t.Errorf("catalog BaseURL = %q, want the resolved base URL %q", cat.BaseURL, p.baseURL())
			}
			if want := tc.wantIssuer(p.srv.URL); cat.Issuer != want {
				t.Errorf("catalog Issuer = %q, want %q", cat.Issuer, want)
			}
			if !slices.Equal(paths, tc.wantPaths) {
				t.Errorf("requested paths %q, want %q", paths, tc.wantPaths)
			}
		})
	}
}

// TestResolveCrossChecksAnotherIssuer pins REQ-073: for a declared issuer
// that differs from the base URL the resolver fetches the issuer's OpenID
// configuration, with Accept: application/json, and requires its issuer to
// equal the declared one exactly and, when both documents declare jwks_uri,
// the two to be equal. A disagreement is ReasonIssuerMismatch; a document
// that cannot be fetched or decoded is ReasonFetchFailed.
func TestResolveCrossChecksAnotherIssuer(t *testing.T) { // REQ-073
	const (
		jwks      = "https://auth.example.com/jwks"
		otherJWKS = "https://other.example.com/jwks"
	)
	tests := []struct {
		name       string
		smartJWKS  string
		openID     documentFunc
		wantReason discovery.DiscoveryErrorReason // empty means success
		wantInner  string                         // a member the error names
	}{
		{
			name:      "issuer and jwks_uri equal",
			smartJWKS: jwks,
			openID:    serve(func(o string) string { return openIDDocument(o+idpPath, jwks) }),
		},
		{
			name:      "jwks_uri only in the SMART configuration",
			smartJWKS: jwks,
			openID:    serve(func(o string) string { return openIDDocument(o+idpPath, "") }),
		},
		{
			name:   "jwks_uri only in the OpenID configuration",
			openID: serve(func(o string) string { return openIDDocument(o+idpPath, jwks) }),
		},
		{
			name:       "OpenID issuer differs",
			openID:     serve(func(string) string { return openIDDocument("https://idp.example.com", "") }),
			wantReason: discovery.ReasonIssuerMismatch,
			wantInner:  "issuer",
		},
		{
			name:       "OpenID issuer differs by a trailing slash",
			openID:     serve(func(o string) string { return openIDDocument(o+idpPath+"/", "") }),
			wantReason: discovery.ReasonIssuerMismatch,
			wantInner:  "issuer",
		},
		{
			// A document without an issuer is not an OpenID configuration, so
			// the fetch failed; a mismatch needs a present, different issuer.
			name:       "OpenID issuer absent",
			openID:     serve(func(string) string { return openIDDocument("", "") }),
			wantReason: discovery.ReasonFetchFailed,
			wantInner:  "no issuer",
		},
		{
			name:       "OpenID configuration an empty object",
			openID:     serve(func(string) string { return `{}` }),
			wantReason: discovery.ReasonFetchFailed,
			wantInner:  "no issuer",
		},
		{
			name:       "JSON error page answered with 200",
			openID:     serve(func(string) string { return `{"error":"upstream unavailable","status":502}` }),
			wantReason: discovery.ReasonFetchFailed,
			wantInner:  "no issuer",
		},
		{
			name:       "jwks_uri differs",
			smartJWKS:  jwks,
			openID:     serve(func(o string) string { return openIDDocument(o+idpPath, otherJWKS) }),
			wantReason: discovery.ReasonIssuerMismatch,
			wantInner:  "jwks_uri",
		},
		{
			name:       "OpenID configuration not found",
			openID:     notFound,
			wantReason: discovery.ReasonFetchFailed,
		},
		{
			// The body would confirm the issuer and its keys, so only the
			// status refuses it.
			name:      "OpenID configuration 404 with a confirming body",
			smartJWKS: jwks,
			openID: func(o string) (int, string) {
				return http.StatusNotFound, openIDDocument(o+idpPath, jwks)
			},
			wantReason: discovery.ReasonFetchFailed,
			wantInner:  "404",
		},
		{
			name:      "OpenID configuration 503 with a confirming body",
			smartJWKS: jwks,
			openID: func(o string) (int, string) {
				return http.StatusServiceUnavailable, openIDDocument(o+idpPath, jwks)
			},
			wantReason: discovery.ReasonFetchFailed,
			wantInner:  "503",
		},
		{
			name:       "OpenID configuration not JSON",
			openID:     serve(func(string) string { return "<html>sign in</html>" }),
			wantReason: discovery.ReasonFetchFailed,
		},
		{
			// Valid JSON whose issuer matches, padded past the 1 MiB body
			// limit: only a resolver that reads at most 1 MiB fails to decode it.
			name: "OpenID configuration over 1 MiB",
			openID: serve(func(o string) string {
				return `{"issuer":"` + o + idpPath + `","padding":"` + strings.Repeat("x", 1<<20) + `"}`
			}),
			wantReason: discovery.ReasonFetchFailed,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := startPlatform(t, false, serve(func(o string) string {
				return smartDocument(o+idpPath, tc.smartJWKS)
			}), tc.openID)
			cat, err := p.resolver(t).Resolve(t.Context(), p.baseURL())

			paths, accepts := p.requests()
			if want := []string{smartDocPath, openIDDocPath}; !slices.Equal(paths, want) {
				t.Errorf("requested paths %q, want %q", paths, want)
			}
			if len(accepts) == 2 && accepts[1] != "application/json" {
				t.Errorf("OpenID configuration request Accept = %q, want %q", accepts[1], "application/json")
			}
			if tc.wantReason == "" {
				if err != nil {
					t.Fatalf("Resolve(%q) error = %v, want success", p.baseURL(), err)
				}
				if cat.Issuer != p.issuer() || cat.BaseURL != p.baseURL() {
					t.Errorf("catalog Issuer, BaseURL = %q, %q, want %q, %q", cat.Issuer, cat.BaseURL, p.issuer(), p.baseURL())
				}
				return
			}
			derr, ok := errors.AsType[*discovery.DiscoveryError](err)
			if !ok || derr.Reason != tc.wantReason {
				t.Fatalf("Resolve(%q) error = %v, want a DiscoveryError with Reason %q", p.baseURL(), err, tc.wantReason)
			}
			if derr.Issuer != p.baseURL() {
				t.Errorf("DiscoveryError.Issuer = %q, want the base URL %q", derr.Issuer, p.baseURL())
			}
			if tc.wantInner != "" && (derr.Inner == nil || !strings.Contains(derr.Inner.Error(), tc.wantInner)) {
				t.Errorf("DiscoveryError.Inner = %v, want it to name %q", derr.Inner, tc.wantInner)
			}
		})
	}
}

// roundTripperFunc adapts a function to http.RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestResolveCrossCheckNetworkFailure pins REQ-073: an OpenID configuration
// that cannot be reached is ReasonFetchFailed, with the network error kept as
// the cause.
func TestResolveCrossCheckNetworkFailure(t *testing.T) { // REQ-073
	p := startPlatform(t, false, serve(func(o string) string {
		return smartDocument(o+idpPath, "")
	}), serve(func(o string) string { return openIDDocument(o+idpPath, "") }))
	errNetwork := errors.New("network unreachable")
	base := p.srv.Client().Transport
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasPrefix(r.URL.Path, idpPath) {
			return nil, errNetwork
		}
		return base.RoundTrip(r)
	})}
	res, err := discovery.NewResolver(nil, discovery.WithHTTPClient(client))
	if err != nil {
		t.Fatal(err)
	}
	_, err = res.Resolve(t.Context(), p.baseURL())
	derr, ok := errors.AsType[*discovery.DiscoveryError](err)
	if !ok || derr.Reason != discovery.ReasonFetchFailed {
		t.Fatalf("Resolve(%q) error = %v, want a DiscoveryError with Reason %q", p.baseURL(), err, discovery.ReasonFetchFailed)
	}
	if !errors.Is(err, errNetwork) {
		t.Errorf("Resolve(%q) error = %v, want it to wrap the network error", p.baseURL(), err)
	}
}

// TestResolveCrossCheckHonoursContext pins REQ-073: the OpenID configuration
// fetch runs under the caller's context, so cancelling it ends the fetch.
func TestResolveCrossCheckHonoursContext(t *testing.T) { // REQ-073
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// The OpenID handler cancels the caller's context, then holds the
	// response until the client gives up. The timeout only bounds a broken run.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issuer := "https://" + r.Host + idpPath
		switch r.URL.Path {
		case smartDocPath:
			_, _ = io.WriteString(w, smartDocument(issuer, ""))
		case openIDDocPath:
			cancel()
			select {
			case <-r.Context().Done():
			case <-time.After(10 * time.Second):
				_, _ = io.WriteString(w, openIDDocument(issuer, ""))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	res, err := discovery.NewResolver(nil, discovery.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	baseURL := srv.URL + platformPath
	_, err = res.Resolve(ctx, baseURL)
	derr, ok := errors.AsType[*discovery.DiscoveryError](err)
	if !ok || derr.Reason != discovery.ReasonFetchFailed {
		t.Fatalf("Resolve(%q) error = %v, want a DiscoveryError with Reason %q", baseURL, err, discovery.ReasonFetchFailed)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Resolve(%q) error = %v, want it to wrap context.Canceled", baseURL, err)
	}
}

// TestResolveCrossCheckOnEveryResolutionAndRefresh pins REQ-071 and REQ-073:
// the catalog is cached under the base URL the caller resolved, never under
// the issuer, and every fetch of the SMART configuration, refresh included,
// repeats the OpenID configuration check.
func TestResolveCrossCheckOnEveryResolutionAndRefresh(t *testing.T) { // REQ-071, REQ-073
	var (
		openIDHits atomic.Int32
		disagree   atomic.Bool
	)
	p := startPlatform(t, false, serve(func(o string) string {
		return smartDocument(o+idpPath, "")
	}), func(o string) (int, string) {
		openIDHits.Add(1)
		if disagree.Load() {
			return http.StatusOK, openIDDocument("https://idp.example.com", "")
		}
		return http.StatusOK, openIDDocument(o+idpPath, "")
	})
	cache := discovery.NewMemoryCache()
	res, err := discovery.NewResolver(cache, discovery.WithHTTPClient(p.srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	if _, err := res.Resolve(ctx, p.baseURL()); err != nil {
		t.Fatalf("first Resolve(%q) error = %v", p.baseURL(), err)
	}
	if _, err := res.Resolve(ctx, p.baseURL()); err != nil {
		t.Fatalf("second Resolve(%q) error = %v", p.baseURL(), err)
	}
	if got := openIDHits.Load(); got != 1 {
		t.Errorf("after a fetch and a cache hit, OpenID configuration fetched %d times, want 1", got)
	}
	if cat, ok := cache.Get(ctx, p.baseURL()); !ok || cat.BaseURL != p.baseURL() {
		t.Errorf("cache.Get(base URL %q) = %v, %t, want the catalog for that base URL", p.baseURL(), cat, ok)
	}
	if _, ok := cache.Get(ctx, p.issuer()); ok {
		t.Errorf("cache.Get(issuer %q) found a catalog, want the cache keyed by base URL only", p.issuer())
	}

	if _, err := res.Refresh(ctx, p.baseURL()); err != nil {
		t.Fatalf("Refresh(%q) error = %v", p.baseURL(), err)
	}
	if got := openIDHits.Load(); got != 2 {
		t.Errorf("after Refresh, OpenID configuration fetched %d times, want 2", got)
	}

	disagree.Store(true)
	_, err = res.Refresh(ctx, p.baseURL())
	if derr, ok := errors.AsType[*discovery.DiscoveryError](err); !ok || derr.Reason != discovery.ReasonIssuerMismatch {
		t.Errorf("Refresh(%q) after the issuer's OpenID configuration changed: error = %v, want Reason %q", p.baseURL(), err, discovery.ReasonIssuerMismatch)
	}
}

// serialisingCache stores each catalog as JSON and keeps the decoded copy,
// as a file-backed or distributed cache does, so a catalog comes back with
// its exported fields only.
type serialisingCache struct{ *discovery.MemoryCache }

func (c serialisingCache) Put(ctx context.Context, baseURL string, cat *discovery.ServiceCatalog) error {
	b, err := json.Marshal(cat)
	if err != nil {
		return err
	}
	var stored discovery.ServiceCatalog
	if err := json.Unmarshal(b, &stored); err != nil {
		return err
	}
	return c.MemoryCache.Put(ctx, baseURL, &stored)
}

// TestRenewalOnNotModifiedRechecksIssuer pins REQ-073: when a conditional
// request for a cached catalog is answered 304 Not Modified, whether Refresh
// sent it or Resolve of the expired catalog did, the resolver fetches the
// issuer's OpenID configuration again. A configuration that now names
// another issuer, or another jwks_uri than the SMART configuration, fails
// the renewal with ReasonIssuerMismatch and drops the cached catalog. The
// jwks_uri is compared as the SMART configuration wrote it, as on a 200.
// A catalog that comes back from a cache that keeps its exported fields only
// has lost that text, so both values are compared parsed: a real change is
// still a mismatch, and a scheme the parser lower-cased is not.
func TestRenewalOnNotModifiedRechecksIssuer(t *testing.T) { // REQ-073
	const (
		jwks      = "https://auth.example.com/jwks"
		otherJWKS = "https://other.example.com/jwks"
	)
	renewals := []struct {
		name  string
		renew func(ctx context.Context, res *discovery.Resolver, baseURL string) (*discovery.ServiceCatalog, error)
	}{
		{name: "Refresh", renew: func(ctx context.Context, res *discovery.Resolver, baseURL string) (*discovery.ServiceCatalog, error) {
			return res.Refresh(ctx, baseURL)
		}},
		{name: "Resolve after expiry", renew: func(ctx context.Context, res *discovery.Resolver, baseURL string) (*discovery.ServiceCatalog, error) {
			synctest.Sleep(61 * time.Second) // past the first response's max-age=60
			return res.Resolve(ctx, baseURL)
		}},
	}
	tests := []struct {
		name       string
		jwksURI    string // declared by both documents until change runs
		serialise  bool   // cache the catalog through serialisingCache
		change     func(p *conditionalPlatform)
		wantReason discovery.DiscoveryErrorReason // empty means the renewal succeeds
		wantInner  string                         // a member the error names
	}{
		{
			name:       "OpenID issuer changes",
			change:     func(p *conditionalPlatform) { p.changeOpenID("https://evil.example.com", "") },
			wantReason: discovery.ReasonIssuerMismatch,
			wantInner:  "issuer",
		},
		{
			name:       "OpenID jwks_uri changes",
			jwksURI:    jwks,
			change:     func(p *conditionalPlatform) { p.changeOpenID("", otherJWKS) },
			wantReason: discovery.ReasonIssuerMismatch,
			wantInner:  "jwks_uri",
		},
		{
			name:       "OpenID jwks_uri changes, catalog from a serialising cache",
			jwksURI:    jwks,
			serialise:  true,
			change:     func(p *conditionalPlatform) { p.changeOpenID("", otherJWKS) },
			wantReason: discovery.ReasonIssuerMismatch,
			wantInner:  "jwks_uri",
		},
		{
			// Both documents write the same text, which the 200 accepted;
			// the parsed URL would spell its scheme in lower case.
			name:    "jwks_uri unchanged, written with an upper-case scheme",
			jwksURI: "HTTPS://auth.example.com/jwks",
		},
		{
			// The written text is lost in the cache; the two documents still
			// agree, so the renewal must not report a mismatch.
			name:      "jwks_uri unchanged, upper-case scheme, catalog from a serialising cache",
			jwksURI:   "HTTPS://auth.example.com/jwks",
			serialise: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, rn := range renewals {
				t.Run(rn.name, func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						p := &conditionalPlatform{jwksURI: tc.jwksURI}
						srv := httptest.NewTestServer(t, p.handler(""))
						var cache discovery.Cache = discovery.NewMemoryCache()
						if tc.serialise {
							cache = serialisingCache{discovery.NewMemoryCache()}
						}
						res, err := discovery.NewResolver(cache, discovery.WithHTTPClient(srv.Client()), discovery.WithAllowInsecure())
						if err != nil {
							t.Fatal(err)
						}
						baseURL := srv.URL + platformPath
						if _, err := res.Resolve(t.Context(), baseURL); err != nil {
							t.Fatalf("Resolve(%q) error = %v", baseURL, err)
						}
						if tc.change != nil {
							tc.change(p)
						}

						_, err = rn.renew(t.Context(), res, baseURL)

						if got, want := p.requests(), []string{"", `"v1"`}; !slices.Equal(got, want) {
							t.Errorf("If-None-Match of the SMART requests = %q, want %q: the renewal is a conditional request answered 304", got, want)
						}
						if got := p.openIDHits.Load(); got != 2 {
							t.Errorf("OpenID configuration fetched %d times, want 2: once when the catalog was built and again on the 304", got)
						}
						if tc.wantReason == "" {
							if err != nil {
								t.Fatalf("%s(%q) error = %v, want the 304 to renew the catalog", rn.name, baseURL, err)
							}
							if _, ok := cache.Get(t.Context(), baseURL); !ok {
								t.Errorf("cache holds no catalog for %q after the renewal", baseURL)
							}
							return
						}
						derr, ok := errors.AsType[*discovery.DiscoveryError](err)
						if !ok || derr.Reason != tc.wantReason {
							t.Fatalf("%s(%q) after the OpenID configuration changed: error = %v, want a DiscoveryError with Reason %q", rn.name, baseURL, err, tc.wantReason)
						}
						if derr.Inner == nil || !strings.Contains(derr.Inner.Error(), tc.wantInner) {
							t.Errorf("DiscoveryError.Inner = %v, want it to name %q", derr.Inner, tc.wantInner)
						}
						if _, ok := cache.Get(t.Context(), baseURL); ok {
							t.Errorf("cache still holds a catalog for %q after the failed renewal", baseURL)
						}
					})
				})
			}
		})
	}
}

// TestResolveValidatesDeclaredIssuer pins REQ-073: a declared issuer is
// accepted whether or not it equals the base URL, provided it is an absolute
// https URL with a host and no query or fragment; the scheme is matched
// without regard to case. A malformed issuer is ReasonMalformedURL and an
// http issuer ReasonInsecureURL unless WithAllowInsecure is set. A refused
// issuer is never contacted.
func TestResolveValidatesDeclaredIssuer(t *testing.T) { // REQ-073
	tests := []struct {
		name       string
		plaintext  bool
		opts       []discovery.Option
		issuer     func(origin string) string
		wantReason discovery.DiscoveryErrorReason // empty means success
	}{
		{
			name:   "https issuer with a path",
			issuer: func(o string) string { return o + idpPath },
		},
		{
			name:   "upper-case HTTPS scheme",
			issuer: func(o string) string { return "HTTPS" + strings.TrimPrefix(o, "https") + idpPath },
		},
		{
			name:      "http issuer with WithAllowInsecure",
			plaintext: true,
			opts:      []discovery.Option{discovery.WithAllowInsecure()},
			issuer:    func(o string) string { return o + idpPath },
		},
		{
			name:       "http issuer",
			issuer:     func(o string) string { return "http" + strings.TrimPrefix(o, "https") + idpPath },
			wantReason: discovery.ReasonInsecureURL,
		},
		{
			name:       "upper-case HTTP issuer",
			issuer:     func(o string) string { return "HTTP" + strings.TrimPrefix(o, "https") + idpPath },
			wantReason: discovery.ReasonInsecureURL,
		},
		{
			name:       "query",
			issuer:     func(o string) string { return o + idpPath + "?tenant=1" },
			wantReason: discovery.ReasonMalformedURL,
		},
		{
			name:       "empty query",
			issuer:     func(o string) string { return o + idpPath + "?" },
			wantReason: discovery.ReasonMalformedURL,
		},
		{
			name:       "fragment",
			issuer:     func(o string) string { return o + idpPath + "#top" },
			wantReason: discovery.ReasonMalformedURL,
		},
		{
			name:       "empty fragment",
			issuer:     func(o string) string { return o + idpPath + "#" },
			wantReason: discovery.ReasonMalformedURL,
		},
		{
			name:       "relative reference",
			issuer:     func(string) string { return idpPath },
			wantReason: discovery.ReasonMalformedURL,
		},
		{
			name:       "no host",
			issuer:     func(string) string { return "https://" + idpPath },
			wantReason: discovery.ReasonMalformedURL,
		},
		{
			name:       "opaque",
			issuer:     func(string) string { return "https:idp.example.com" },
			wantReason: discovery.ReasonMalformedURL,
		},
		{
			name:       "unparseable",
			issuer:     func(o string) string { return o + "/%zz" },
			wantReason: discovery.ReasonMalformedURL,
		},
		{
			name:       "other scheme",
			issuer:     func(o string) string { return "ftp" + strings.TrimPrefix(o, "https") + idpPath },
			wantReason: discovery.ReasonMalformedURL,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantReason == "" {
				p := startPlatform(t, tc.plaintext,
					serve(func(o string) string { return smartDocument(tc.issuer(o), "") }),
					serve(func(o string) string { return openIDDocument(tc.issuer(o), "") }))
				declared := tc.issuer(p.srv.URL)
				cat, err := p.resolver(t, tc.opts...).Resolve(t.Context(), p.baseURL())
				if err != nil {
					t.Fatalf("Resolve with issuer %q: error = %v, want success", declared, err)
				}
				if cat.Issuer != declared {
					t.Errorf("catalog Issuer = %q, want the declared issuer %q verbatim", cat.Issuer, declared)
				}
				return
			}
			// The shape rule holds whether or not the OpenID configuration
			// check runs, so a refusal is pinned with the check on and off.
			for _, check := range []struct {
				name string
				opts []discovery.Option
			}{
				{name: "OpenID check on"},
				{name: "OpenID check off", opts: []discovery.Option{discovery.WithoutOpenIDConfigurationCheck()}},
			} {
				t.Run(check.name, func(t *testing.T) {
					p := startPlatform(t, tc.plaintext,
						serve(func(o string) string { return smartDocument(tc.issuer(o), "") }),
						serve(func(o string) string { return openIDDocument(tc.issuer(o), "") }))
					declared := tc.issuer(p.srv.URL)
					_, err := p.resolver(t, append(slices.Clone(tc.opts), check.opts...)...).Resolve(t.Context(), p.baseURL())
					paths, _ := p.requests()

					derr, ok := errors.AsType[*discovery.DiscoveryError](err)
					if !ok || derr.Reason != tc.wantReason {
						t.Fatalf("Resolve with issuer %q: error = %v, want a DiscoveryError with Reason %q", declared, err, tc.wantReason)
					}
					if derr.Issuer != p.baseURL() {
						t.Errorf("DiscoveryError.Issuer = %q, want the base URL %q", derr.Issuer, p.baseURL())
					}
					if want := []string{smartDocPath}; !slices.Equal(paths, want) {
						t.Errorf("requested paths %q, want only %q: a refused issuer is never contacted", paths, want)
					}
				})
			}
		})
	}
}

// TestNewStaticCatalogBaseURL pins REQ-070: a hand-built catalog carries
// StaticConfig.BaseURL, which defaults to StaticConfig.Issuer when empty.
func TestNewStaticCatalogBaseURL(t *testing.T) { // REQ-070
	tests := []struct {
		name        string
		cfg         discovery.StaticConfig
		wantBaseURL string
		wantIssuer  string
	}{
		{
			name:        "issuer only",
			cfg:         discovery.StaticConfig{Issuer: "https://ehrbase.example/"},
			wantBaseURL: "https://ehrbase.example/",
			wantIssuer:  "https://ehrbase.example/",
		},
		{
			name:        "base URL and issuer",
			cfg:         discovery.StaticConfig{BaseURL: "https://platform.example/gateway", Issuer: "https://idp.example/realms/x"},
			wantBaseURL: "https://platform.example/gateway",
			wantIssuer:  "https://idp.example/realms/x",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cat, err := discovery.NewStaticCatalog(tc.cfg)
			if err != nil {
				t.Fatalf("NewStaticCatalog(BaseURL %q, Issuer %q) error = %v", tc.cfg.BaseURL, tc.cfg.Issuer, err)
			}
			if cat.BaseURL != tc.wantBaseURL || cat.Issuer != tc.wantIssuer {
				t.Errorf("NewStaticCatalog(BaseURL %q, Issuer %q) BaseURL, Issuer = %q, %q, want %q, %q", tc.cfg.BaseURL, tc.cfg.Issuer, cat.BaseURL, cat.Issuer, tc.wantBaseURL, tc.wantIssuer)
			}
		})
	}
}
