package discovery_test

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// documentWithTokenEndpoint is a SMART configuration that advertises the
// openEHR REST service with https authorization and service URLs, the given
// token_endpoint and, unless it is empty, the given issuer.
func documentWithTokenEndpoint(issuer, tokenEndpoint string) string {
	doc := map[string]any{
		"authorization_endpoint": "https://auth.example.com/authorize",
		"token_endpoint":         tokenEndpoint,
		"services": map[string]any{
			discovery.ServiceIDOpenEHRRest: map[string]any{"baseUrl": "https://api.example.com/openehr/v1"},
		},
	}
	if issuer != "" {
		doc["issuer"] = issuer
	}
	return mustJSON(doc)
}

// TestREQ071_FreshCacheHitPassesTheCallingResolversChecks pins REQ-071 with
// REQ-072 and REQ-073: one Cache serves a lenient Resolver and a strict one.
// The lenient Resolver fetches and caches a catalog the strict one's checks
// refuse; the strict Resolver's Resolve, on the fresh hit, returns the
// DiscoveryError a new document would produce, to the same text, sends no
// request, and leaves the entry cached, so the lenient Resolver still gets
// the cached catalog without a new fetch. A 304 renewal of that catalog by
// the strict Resolver's Refresh yields the same error text as well.
func TestREQ071_FreshCacheHitPassesTheCallingResolversChecks(t *testing.T) { // REQ-071, REQ-072, REQ-073
	t.Parallel()
	const extraService = "org.example.extra"
	quiet := discovery.WithLogger(slog.New(slog.DiscardHandler))
	tests := []struct {
		name string
		// plaintext serves the Platform over http, so the base URL is http.
		plaintext   bool
		doc         string
		lenient     []discovery.Option
		strict      []discovery.Option
		wantReason  discovery.DiscoveryErrorReason
		wantMissing []string
		wantGot     string // SpecVersionGot, for a version mismatch
	}{
		{
			name:        "missing required service",
			doc:         smartDocument("", ""),
			strict:      []discovery.Option{discovery.WithRequiredServices(discovery.ServiceIDOpenEHRRest, extraService)},
			wantReason:  discovery.ReasonMissingService,
			wantMissing: []string{extraService},
		},
		{
			name:       "spec version outside the strict set",
			doc:        serviceDocument(map[string]any{"spec_version": "1.1.1"}),
			lenient:    []discovery.Option{discovery.WithAcceptedSpecVersions(discovery.SpecVersionPin, "1.1.1")},
			wantReason: discovery.ReasonSpecVersionMismatch,
			wantGot:    "1.1.1",
		},
		{
			name:       "http auth endpoint",
			doc:        documentWithTokenEndpoint("", "http://auth.example.com/token"),
			lenient:    []discovery.Option{discovery.WithAllowInsecure()},
			wantReason: discovery.ReasonInsecureURL,
		},
		{
			// A new document is refused for its auth endpoint before its
			// issuer, so the refusal on a hit names the same member.
			name:       "http issuer and http auth endpoint",
			doc:        documentWithTokenEndpoint("http://idp.example.com", "http://auth.example.com/token"),
			lenient:    []discovery.Option{discovery.WithAllowInsecure(), discovery.WithoutOpenIDConfigurationCheck()},
			wantReason: discovery.ReasonInsecureURL,
		},
		{
			name:       "http issuer",
			doc:        smartDocument("http://idp.example.com", ""),
			lenient:    []discovery.Option{discovery.WithAllowInsecure(), discovery.WithoutOpenIDConfigurationCheck()},
			wantReason: discovery.ReasonInsecureURL,
		},
		{
			// The document names no issuer, so the catalog's issuer is the
			// http base URL too.
			name:       "http Platform base URL",
			plaintext:  true,
			doc:        smartDocument("", ""),
			lenient:    []discovery.Option{discovery.WithAllowInsecure()},
			wantReason: discovery.ReasonInsecureURL,
		},
		{
			// The https issuer passes, so only the base URL's own check
			// refuses the catalog.
			name:       "http Platform base URL with an https issuer",
			plaintext:  true,
			doc:        smartDocument("https://idp.example.com", ""),
			lenient:    []discovery.Option{discovery.WithAllowInsecure(), discovery.WithoutOpenIDConfigurationCheck()},
			strict:     []discovery.Option{discovery.WithoutOpenIDConfigurationCheck()},
			wantReason: discovery.ReasonInsecureURL,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var hits, notModified atomic.Int32
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != smartDocPath {
					http.NotFound(w, r)
					return
				}
				hits.Add(1)
				w.Header().Set("ETag", `"v1"`)
				if r.Header.Get("If-None-Match") == `"v1"` {
					notModified.Add(1)
					w.WriteHeader(http.StatusNotModified)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.doc)
			})
			var srv *httptest.Server
			if tc.plaintext {
				srv = httptest.NewServer(h)
			} else {
				srv = httptest.NewTLSServer(h)
			}
			t.Cleanup(srv.Close)
			baseURL := srv.URL + platformPath
			newResolver := func(cache discovery.Cache, opts []discovery.Option) *discovery.Resolver {
				t.Helper()
				res, err := discovery.NewResolver(cache, append([]discovery.Option{discovery.WithHTTPClient(srv.Client()), quiet}, opts...)...)
				if err != nil {
					t.Fatal(err)
				}
				return res
			}
			cache := discovery.NewMemoryCache()
			lenient := newResolver(cache, tc.lenient)
			strict := newResolver(cache, tc.strict)

			first, err := lenient.Resolve(t.Context(), baseURL)
			if err != nil {
				t.Fatalf("lenient Resolve(%q) error = %v, want the catalog cached", baseURL, err)
			}

			got, err := strict.Resolve(t.Context(), baseURL)
			derr, ok := errors.AsType[*discovery.DiscoveryError](err)
			if !ok || derr == nil || derr.Reason != tc.wantReason {
				t.Fatalf("strict Resolve(%q) on a fresh hit = (catalog %p, %v), want a DiscoveryError with Reason %q", baseURL, got, err, tc.wantReason)
			}
			if got != nil {
				t.Errorf("strict Resolve(%q) returned catalog %p with its error, want nil", baseURL, got)
			}
			if !slices.Equal(derr.MissingServices, tc.wantMissing) {
				t.Errorf("strict Resolve(%q) MissingServices = %q, want %q", baseURL, derr.MissingServices, tc.wantMissing)
			}
			if derr.SpecVersionGot != tc.wantGot {
				t.Errorf("strict Resolve(%q) SpecVersionGot = %q, want %q", baseURL, derr.SpecVersionGot, tc.wantGot)
			}
			if n := hits.Load(); n != 1 {
				t.Errorf("server received %d SMART configuration request(s) after the strict Resolve, want 1: a fresh hit sends no request", n)
			}
			if held, ok := cache.Get(t.Context(), baseURL); !ok || held != first {
				t.Errorf("cache.Get(%q) after the refusal = (%p, %v), want the lenient Resolver's catalog %p still cached", baseURL, held, ok, first)
			}

			again, err := lenient.Resolve(t.Context(), baseURL)
			if err != nil || again != first {
				t.Errorf("lenient Resolve(%q) after the refusal = (%p, %v), want the cached catalog %p", baseURL, again, err, first)
			}
			if n := hits.Load(); n != 1 {
				t.Errorf("server received %d SMART configuration request(s) in all, want 1: the lenient Resolver gets the cached catalog without a new fetch", n)
			}

			// The refusal on a hit is the one a new document gets: a strict
			// Resolver with a cache of its own reports the same reason, the
			// same details and the same text, inner error included.
			_, newErr := newResolver(nil, tc.strict).Resolve(t.Context(), baseURL)
			nerr, ok := errors.AsType[*discovery.DiscoveryError](newErr)
			if !ok || nerr == nil {
				t.Fatalf("strict Resolve(%q) of a new document error = %v, want a DiscoveryError", baseURL, newErr)
			}
			if nerr.Reason != derr.Reason || nerr.Issuer != derr.Issuer || !slices.Equal(nerr.MissingServices, derr.MissingServices) ||
				nerr.SpecVersionGot != derr.SpecVersionGot || nerr.SpecVersionWant != derr.SpecVersionWant {
				t.Errorf("fresh-hit DiscoveryError = %+v, want the one a new document gets, %+v", derr, nerr)
			}
			if got, want := derr.Error(), nerr.Error(); got != want {
				t.Errorf("fresh-hit error text = %q, want the text a new document gets, %q", got, want)
			}

			// A 304 renewal of the cached catalog by the strict Resolver runs
			// the same checks and reports the same text. On an http base URL
			// the Refresh is refused before it sends a request.
			_, renewErr := strict.Refresh(t.Context(), baseURL)
			if renewErr == nil {
				t.Fatalf("strict Refresh(%q) of the cached catalog = nil error, want %q", baseURL, nerr.Error())
			}
			if got, want := renewErr.Error(), nerr.Error(); got != want {
				t.Errorf("strict Refresh(%q) error text = %q, want the text a new document gets, %q", baseURL, got, want)
			}
			wantNotModified := int32(1)
			if tc.plaintext {
				wantNotModified = 0
			}
			if n := notModified.Load(); n != wantNotModified {
				t.Errorf("server answered %d conditional request(s) with 304, want %d", n, wantNotModified)
			}
		})
	}
}

// cachedMembers are the URL members of a SMART configuration that a test
// serves as a document and puts in a Cache as a catalog. An empty auth
// endpoint is left out of the document and is nil in the catalog. The issuer
// is built from the test server's origin, so a request for its OpenID
// configuration reaches the test server; a nil issuer is left out of the
// document, and the catalog's Issuer is then the base URL. The openEHR REST
// service's base URL is written as it stands, and an empty one is nil in the
// catalog. The FHIR service is declared only when its base URL is not empty.
type cachedMembers struct {
	issuer                func(origin string) string
	authorizationEndpoint string
	tokenEndpoint         string
	serviceBaseURL        string
	fhirBaseURL           string
}

// document is the SMART configuration that declares m.
func (m cachedMembers) document(origin string) string {
	services := map[string]any{
		discovery.ServiceIDOpenEHRRest: map[string]any{"baseUrl": m.serviceBaseURL},
	}
	if m.fhirBaseURL != "" {
		services[discovery.ServiceIDFHIRRest] = map[string]any{"baseUrl": m.fhirBaseURL}
	}
	doc := map[string]any{"services": services}
	if m.issuer != nil {
		doc["issuer"] = m.issuer(origin)
	}
	if m.authorizationEndpoint != "" {
		doc["authorization_endpoint"] = m.authorizationEndpoint
	}
	if m.tokenEndpoint != "" {
		doc["token_endpoint"] = m.tokenEndpoint
	}
	return mustJSON(doc)
}

// catalog is a fresh catalog for baseURL that carries m, with the ETag the
// test server sends, as a Cache this Resolver's parser did not fill may hold
// it.
func (m cachedMembers) catalog(t *testing.T, origin, baseURL, etag string) *discovery.ServiceCatalog {
	t.Helper()
	parse := func(raw string) *url.URL {
		if raw == "" {
			return nil
		}
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("url.Parse(%q) error = %v", raw, err)
		}
		return u
	}
	issuer := baseURL
	if m.issuer != nil {
		issuer = m.issuer(origin)
	}
	services := map[string]discovery.ServiceEntry{
		discovery.ServiceIDOpenEHRRest: {ID: discovery.ServiceIDOpenEHRRest, BaseURL: parse(m.serviceBaseURL)},
	}
	if m.fhirBaseURL != "" {
		services[discovery.ServiceIDFHIRRest] = discovery.ServiceEntry{ID: discovery.ServiceIDFHIRRest, BaseURL: parse(m.fhirBaseURL)}
	}
	now := time.Now()
	return &discovery.ServiceCatalog{
		BaseURL:  baseURL,
		Issuer:   issuer,
		Services: services,
		Auth: discovery.AuthEndpoints{
			AuthorizationEndpoint: parse(m.authorizationEndpoint),
			TokenEndpoint:         parse(m.tokenEndpoint),
		},
		ResolvedAt: now,
		ExpiresAt:  now.Add(time.Hour),
		ETag:       etag,
	}
}

// TestREQ071_CachedCatalogPassesTheParsersURLChecks pins REQ-071 with REQ-072
// and REQ-073 for a catalog that reached the Cache without this Resolver's
// parser, as from a file-backed or distributed Cache. Each catalog carries a
// URL member a new document is refused for: an auth endpoint with a scheme
// other than http or https or without a host, an issuer with a query or a
// fragment or that is not absolute, a service base URL that is not absolute.
// On a fresh hit, Resolve returns the DiscoveryError the same members get in
// a new document, to the same text, sends no request and leaves the entry
// cached. A Refresh answered 304 returns that error too, before any request
// for the issuer's OpenID configuration. Where a catalog has two refused
// members, each path names the one the parser checks first: the auth
// endpoints in document order, then the service base URLs by id, then the
// issuer. A well-formed catalog is still returned on a fresh hit.
func TestREQ071_CachedCatalogPassesTheParsersURLChecks(t *testing.T) { // REQ-071, REQ-072, REQ-073
	t.Parallel()
	const (
		etag         = `"v1"`
		authorizeURL = "https://auth.example.com/authorize"
		tokenURL     = "https://auth.example.com/token"
		apiURL       = "https://api.example.com/openehr/v1"
		noScheme     = "//api.example.com/openehr/v1"
	)
	issuerWithQuery := func(o string) string { return o + idpPath + "?tenant=a" }
	quiet := discovery.WithLogger(slog.New(slog.DiscardHandler))
	tests := []struct {
		name    string
		members cachedMembers
		// wantReason is the refusal's Reason; empty means the fresh hit
		// returns the cached catalog.
		wantReason discovery.DiscoveryErrorReason
		// wantText is part of the refusal's text: the member it names and
		// why.
		wantText string
	}{
		{
			name: "well-formed members",
			members: cachedMembers{
				issuer:                func(o string) string { return o + idpPath },
				authorizationEndpoint: authorizeURL,
				tokenEndpoint:         tokenURL,
				serviceBaseURL:        apiURL,
			},
		},
		{
			name:       "ftp token endpoint",
			members:    cachedMembers{authorizationEndpoint: authorizeURL, tokenEndpoint: "ftp://auth.example.com/token", serviceBaseURL: apiURL},
			wantReason: discovery.ReasonMalformedURL,
			wantText:   `token_endpoint uses scheme "ftp"`,
		},
		{
			name:       "token endpoint without a host",
			members:    cachedMembers{authorizationEndpoint: authorizeURL, tokenEndpoint: "https:///token", serviceBaseURL: apiURL},
			wantReason: discovery.ReasonMalformedURL,
			wantText:   `token_endpoint "https:///token" invalid`,
		},
		{
			name:       "issuer with a query",
			members:    cachedMembers{issuer: issuerWithQuery, tokenEndpoint: tokenURL, serviceBaseURL: apiURL},
			wantReason: discovery.ReasonMalformedURL,
			wantText:   "has a query",
		},
		{
			name:       "issuer with a fragment",
			members:    cachedMembers{issuer: func(o string) string { return o + idpPath + "#a" }, tokenEndpoint: tokenURL, serviceBaseURL: apiURL},
			wantReason: discovery.ReasonMalformedURL,
			wantText:   "has a fragment",
		},
		{
			name:       "issuer that is not absolute",
			members:    cachedMembers{issuer: func(string) string { return idpPath }, tokenEndpoint: tokenURL, serviceBaseURL: apiURL},
			wantReason: discovery.ReasonMalformedURL,
			wantText:   `issuer "/idp" is not an https URL`,
		},
		{
			name:       "service base URL without a scheme",
			members:    cachedMembers{tokenEndpoint: tokenURL, serviceBaseURL: noScheme},
			wantReason: discovery.ReasonMalformedURL,
			wantText:   `baseUrl "` + noScheme + `" invalid`,
		},
		{
			name:       "service without a base URL",
			members:    cachedMembers{tokenEndpoint: tokenURL},
			wantReason: discovery.ReasonMalformedURL,
			wantText:   `baseUrl "" invalid`,
		},
		{
			// The parser checks authorization_endpoint before token_endpoint.
			name: "two http auth endpoints",
			members: cachedMembers{
				authorizationEndpoint: "http://auth.example.com/authorize",
				tokenEndpoint:         "http://auth.example.com/token",
				serviceBaseURL:        apiURL,
			},
			wantReason: discovery.ReasonInsecureURL,
			wantText:   `authorization_endpoint uses scheme "http"`,
		},
		{
			// The parser checks the service base URLs by id, so
			// "org.fhir.rest" before "org.openehr.rest".
			name:       "two service base URLs without a scheme",
			members:    cachedMembers{tokenEndpoint: tokenURL, serviceBaseURL: noScheme, fhirBaseURL: "//fhir.example.com/r4"},
			wantReason: discovery.ReasonMalformedURL,
			wantText:   `service "org.fhir.rest" baseUrl "//fhir.example.com/r4" invalid`,
		},
		{
			// The parser checks the auth endpoints before the issuer.
			name:       "ftp token endpoint and an issuer with a query",
			members:    cachedMembers{issuer: issuerWithQuery, tokenEndpoint: "ftp://auth.example.com/token", serviceBaseURL: apiURL},
			wantReason: discovery.ReasonMalformedURL,
			wantText:   `token_endpoint uses scheme "ftp"`,
		},
		{
			// The parser checks the service base URLs before the issuer.
			name:       "service base URL without a scheme and an issuer with a query",
			members:    cachedMembers{issuer: issuerWithQuery, tokenEndpoint: tokenURL, serviceBaseURL: noScheme},
			wantReason: discovery.ReasonMalformedURL,
			wantText:   `baseUrl "` + noScheme + `" invalid`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var (
				mu          sync.Mutex
				others      []string // paths requested other than the SMART configuration
				hits        atomic.Int32
				notModified atomic.Int32
			)
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != smartDocPath {
					mu.Lock()
					others = append(others, r.URL.Path)
					mu.Unlock()
					http.NotFound(w, r)
					return
				}
				hits.Add(1)
				w.Header().Set("ETag", etag)
				if r.Header.Get("If-None-Match") == etag {
					notModified.Add(1)
					w.WriteHeader(http.StatusNotModified)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.members.document("https://"+r.Host))
			}))
			t.Cleanup(srv.Close)
			baseURL := srv.URL + platformPath
			newResolver := func(cache discovery.Cache) *discovery.Resolver {
				t.Helper()
				res, err := discovery.NewResolver(cache, discovery.WithHTTPClient(srv.Client()), quiet)
				if err != nil {
					t.Fatal(err)
				}
				return res
			}
			cache := discovery.NewMemoryCache()
			cached := tc.members.catalog(t, srv.URL, baseURL, etag)
			if err := cache.Put(t.Context(), baseURL, cached); err != nil {
				t.Fatalf("cache.Put(%q) error = %v", baseURL, err)
			}
			strict := newResolver(cache)

			got, err := strict.Resolve(t.Context(), baseURL)
			if n := hits.Load(); n != 0 {
				t.Errorf("server received %d SMART configuration request(s) on the fresh hit, want 0", n)
			}
			if tc.wantReason == "" {
				if err != nil || got != cached {
					t.Fatalf("Resolve(%q) on a fresh hit = (%p, %v), want the cached catalog %p", baseURL, got, err, cached)
				}
				return
			}

			// The refusal a new document with the same members gets.
			_, newErr := newResolver(nil).Resolve(t.Context(), baseURL)
			nerr, ok := errors.AsType[*discovery.DiscoveryError](newErr)
			if !ok || nerr == nil || nerr.Reason != tc.wantReason {
				t.Fatalf("Resolve(%q) of a new document error = %v, want a DiscoveryError with Reason %q", baseURL, newErr, tc.wantReason)
			}
			if !strings.Contains(nerr.Error(), tc.wantText) {
				t.Errorf("Resolve(%q) of a new document error text = %q, want it to contain %q", baseURL, nerr.Error(), tc.wantText)
			}

			// The fresh hit gets that refusal, and the entry stays cached.
			if derr, ok := errors.AsType[*discovery.DiscoveryError](err); !ok || derr == nil ||
				derr.Reason != nerr.Reason || derr.Issuer != nerr.Issuer || derr.Error() != nerr.Error() {
				t.Errorf("Resolve(%q) on a fresh hit = (catalog %p, %v), want the error a new document gets, %q", baseURL, got, err, nerr.Error())
			}
			if got != nil {
				t.Errorf("Resolve(%q) on a fresh hit returned catalog %p, want nil", baseURL, got)
			}
			if held, ok := cache.Get(t.Context(), baseURL); !ok || held != cached {
				t.Errorf("cache.Get(%q) after the refusal = (%p, %v), want the catalog %p still cached", baseURL, held, ok, cached)
			}

			// A 304 renewal of the cached catalog gets it too, and the failed
			// renewal drops the entry.
			_, renewErr := strict.Refresh(t.Context(), baseURL)
			if rerr, ok := errors.AsType[*discovery.DiscoveryError](renewErr); !ok || rerr == nil ||
				rerr.Reason != nerr.Reason || rerr.Error() != nerr.Error() {
				t.Errorf("Refresh(%q) answered 304 error = %v, want the error a new document gets, %q", baseURL, renewErr, nerr.Error())
			}
			if n := notModified.Load(); n != 1 {
				t.Errorf("server answered %d conditional request(s) with 304, want 1", n)
			}
			if held, ok := cache.Get(t.Context(), baseURL); ok {
				t.Errorf("cache.Get(%q) after the refused renewal = (%p, true), want the entry dropped", baseURL, held)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(others) != 0 {
				t.Errorf("server received requests for %q, want none: the URL checks refuse the catalog before the issuer's OpenID configuration is fetched", others)
			}
		})
	}
}
