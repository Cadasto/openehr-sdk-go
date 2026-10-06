package discovery_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// TestResolveStopsOnUnaskedNotModified pins REQ-071: a 304 Not Modified to a
// request that carried no If-None-Match is a failed fetch, with no second
// request, and the failed fetch leaves nothing cached. The server stops
// answering 304 after 50 requests, so a resolver that loops fails the test
// instead of hanging it.
func TestResolveStopsOnUnaskedNotModified(t *testing.T) { // REQ-071
	const loopCap = 50
	tests := []struct {
		name string
		// warm resolves once against a 200 without an ETag before the server
		// turns to 304, so the second call is a Refresh with nothing to send
		// in If-None-Match.
		warm bool
	}{
		{name: "Resolve with nothing cached"},
		{name: "Refresh of a catalog without an ETag", warm: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				mu     sync.Mutex
				ifNone []string
				warmed = !tc.warm
			)
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				if !warmed {
					warmed = true
					_, _ = io.WriteString(w, smartDocument("", ""))
					return
				}
				ifNone = append(ifNone, r.Header.Get("If-None-Match"))
				if len(ifNone) > loopCap {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				w.WriteHeader(http.StatusNotModified)
			}))
			defer srv.Close()
			cache := discovery.NewMemoryCache()
			res, err := discovery.NewResolver(cache, discovery.WithHTTPClient(srv.Client()))
			if err != nil {
				t.Fatal(err)
			}
			baseURL := srv.URL + platformPath
			if tc.warm {
				if _, err := res.Resolve(t.Context(), baseURL); err != nil {
					t.Fatalf("warm-up Resolve(%q) error = %v", baseURL, err)
				}
				_, err = res.Refresh(t.Context(), baseURL)
			} else {
				_, err = res.Resolve(t.Context(), baseURL)
			}

			derr, ok := errors.AsType[*discovery.DiscoveryError](err)
			if !ok || derr.Reason != discovery.ReasonFetchFailed {
				t.Errorf("error = %v, want a DiscoveryError with Reason %q", err, discovery.ReasonFetchFailed)
			}
			mu.Lock()
			got := slices.Clone(ifNone)
			mu.Unlock()
			if want := []string{""}; !slices.Equal(got, want) {
				t.Errorf("If-None-Match of the requests answered 304 = %q, want %q: one unconditional request", got, want)
			}
			if _, ok := cache.Get(t.Context(), baseURL); ok {
				t.Errorf("cache still holds a catalog for %q after the failed fetch", baseURL)
			}
		})
	}
}

// conditionalPlatform serves a SMART configuration that declares an issuer
// other than the base URL, with ETag "v1" and Cache-Control max-age=60. A
// request whose If-None-Match is "v1" gets a 304 Not Modified carrying
// notModifiedCacheControl; any other later request gets a different
// document (ETag "v2", another service baseUrl), so a full re-fetch shows.
// It records the If-None-Match of every SMART request and counts the
// OpenID configuration fetches.
type conditionalPlatform struct {
	// jwksURI, when not empty, is the jwks_uri that the SMART documents and
	// the OpenID configuration declare. It is set before the server starts.
	jwksURI string

	mu          sync.Mutex
	ifNoneMatch []string
	// openIDIssuer and openIDJWKS, when not empty, replace the issuer and
	// the jwks_uri that the OpenID configuration declares.
	openIDIssuer string
	openIDJWKS   string
	openIDHits   atomic.Int32
}

// changeOpenID makes the later OpenID configuration responses declare
// issuer and jwksURI instead of the values that confirm the SMART
// configuration; an empty value leaves that member as it was.
func (p *conditionalPlatform) changeOpenID(issuer, jwksURI string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.openIDIssuer, p.openIDJWKS = issuer, jwksURI
}

const (
	firstAPIBaseURL  = "https://api.example.com/openehr/v1"
	secondAPIBaseURL = "https://api.example.com/openehr/v2"
)

func (p *conditionalPlatform) handler(notModifiedCacheControl string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}
		issuer := scheme + "://" + r.Host + idpPath
		members := map[string]string{"issuer": issuer, "token_endpoint": "https://auth.example.com/token"}
		if p.jwksURI != "" {
			members["jwks_uri"] = p.jwksURI
		}
		switch r.URL.Path {
		case openIDDocPath:
			p.openIDHits.Add(1)
			p.mu.Lock()
			openIDIssuer, openIDJWKS := issuer, p.jwksURI
			if p.openIDIssuer != "" {
				openIDIssuer = p.openIDIssuer
			}
			if p.openIDJWKS != "" {
				openIDJWKS = p.openIDJWKS
			}
			p.mu.Unlock()
			_, _ = io.WriteString(w, openIDDocument(openIDIssuer, openIDJWKS))
		case smartDocPath:
			p.mu.Lock()
			p.ifNoneMatch = append(p.ifNoneMatch, r.Header.Get("If-None-Match"))
			first := len(p.ifNoneMatch) == 1
			p.mu.Unlock()
			switch {
			case r.Header.Get("If-None-Match") == `"v1"`:
				if notModifiedCacheControl != "" {
					w.Header().Set("Cache-Control", notModifiedCacheControl)
				}
				w.Header().Set("ETag", `"v1"`)
				w.WriteHeader(http.StatusNotModified)
			case first:
				w.Header().Set("ETag", `"v1"`)
				w.Header().Set("Cache-Control", "max-age=60")
				_, _ = io.WriteString(w, documentWith(firstAPIBaseURL, members))
			default:
				w.Header().Set("ETag", `"v2"`)
				_, _ = io.WriteString(w, documentWith(secondAPIBaseURL, members))
			}
		default:
			http.NotFound(w, r)
		}
	})
}

func (p *conditionalPlatform) requests() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.ifNoneMatch)
}

// sameDocument reports how renewed differs from cached in the members the
// document supplied, or "" when they are equal.
func sameDocument(cached, renewed *discovery.ServiceCatalog) string {
	switch {
	case renewed.BaseURL != cached.BaseURL:
		return "BaseURL"
	case renewed.Issuer != cached.Issuer:
		return "Issuer"
	case renewed.ETag != cached.ETag:
		return "ETag"
	case !reflect.DeepEqual(renewed.Services, cached.Services):
		return "Services"
	case !reflect.DeepEqual(renewed.Auth, cached.Auth):
		return "Auth"
	}
	return ""
}

// TestRefreshExtendsOnNotModified pins REQ-071: Refresh of a catalog with
// an ETag sends one conditional request, and a 304 Not Modified keeps the
// cached document, services and auth members while renewing the expiry
// from the 304's Cache-Control, or the default TTL when it has none. The
// issuer's OpenID configuration is fetched again, as on every refresh, and
// the catalog the caller already holds is not changed.
func TestRefreshExtendsOnNotModified(t *testing.T) { // REQ-071
	tests := []struct {
		name         string
		cacheControl string // on the 304
		wantTTL      time.Duration
	}{
		{name: "304 with max-age", cacheControl: "max-age=600", wantTTL: 600 * time.Second},
		{name: "304 without Cache-Control", wantTTL: 42 * time.Minute},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &conditionalPlatform{}
			srv := httptest.NewTLSServer(p.handler(tc.cacheControl))
			defer srv.Close()
			cache := discovery.NewMemoryCache()
			res, err := discovery.NewResolver(cache, discovery.WithHTTPClient(srv.Client()), discovery.WithDefaultTTL(42*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			baseURL := srv.URL + platformPath
			first, err := res.Resolve(t.Context(), baseURL)
			if err != nil {
				t.Fatalf("Resolve(%q) error = %v", baseURL, err)
			}
			firstExpiry := first.ExpiresAt

			renewed, err := res.Refresh(t.Context(), baseURL)
			if err != nil {
				t.Fatalf("Refresh(%q) error = %v, want the 304 to renew the catalog", baseURL, err)
			}

			if got, want := p.requests(), []string{"", `"v1"`}; !slices.Equal(got, want) {
				t.Errorf("If-None-Match of the SMART requests = %q, want %q: one conditional request, no full re-fetch", got, want)
			}
			if diff := sameDocument(first, renewed); diff != "" {
				t.Errorf("renewed catalog %s differs from the cached one, want the same document", diff)
			}
			if e, _ := renewed.OpenEHRRest(); e.BaseURL == nil || e.BaseURL.String() != firstAPIBaseURL {
				t.Errorf("renewed org.openehr.rest baseUrl = %v, want %q from the cached document", e.BaseURL, firstAPIBaseURL)
			}
			if got := renewed.ExpiresAt.Sub(renewed.ResolvedAt); got != tc.wantTTL {
				t.Errorf("renewed ExpiresAt - ResolvedAt = %v, want %v", got, tc.wantTTL)
			}
			if !renewed.ExpiresAt.After(firstExpiry) {
				t.Errorf("renewed ExpiresAt %v is not after the cached %v", renewed.ExpiresAt, firstExpiry)
			}
			if !first.ExpiresAt.Equal(firstExpiry) {
				t.Errorf("the catalog returned by Resolve changed its ExpiresAt to %v, want it left at %v", first.ExpiresAt, firstExpiry)
			}
			if cached, ok := cache.Get(t.Context(), baseURL); !ok || !cached.ExpiresAt.Equal(renewed.ExpiresAt) {
				t.Errorf("cache.Get(%q) = %v, %t, want the renewed catalog", baseURL, cached, ok)
			}
			if got := p.openIDHits.Load(); got != 2 {
				t.Errorf("OpenID configuration fetched %d times, want 2: once for the Resolve and again for the Refresh answered 304", got)
			}
		})
	}
}

// TestResolveExtendsExpiredOnNotModified pins REQ-071: Resolve of an expired
// catalog with an ETag takes the same conditional path as Refresh, so a 304
// renews the cached catalog instead of fetching the document again.
func TestResolveExtendsExpiredOnNotModified(t *testing.T) { // REQ-071
	synctest.Test(t, func(t *testing.T) {
		p := &conditionalPlatform{}
		srv := httptest.NewTestServer(t, p.handler("max-age=600"))
		res, err := discovery.NewResolver(nil, discovery.WithHTTPClient(srv.Client()), discovery.WithAllowInsecure())
		if err != nil {
			t.Fatal(err)
		}
		baseURL := srv.URL + platformPath
		first, err := res.Resolve(t.Context(), baseURL)
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v", baseURL, err)
		}
		synctest.Sleep(61 * time.Second) // past the first response's max-age=60

		renewed, err := res.Resolve(t.Context(), baseURL)
		if err != nil {
			t.Fatalf("Resolve(%q) after expiry error = %v", baseURL, err)
		}
		if got, want := p.requests(), []string{"", `"v1"`}; !slices.Equal(got, want) {
			t.Errorf("If-None-Match of the SMART requests = %q, want %q", got, want)
		}
		if diff := sameDocument(first, renewed); diff != "" {
			t.Errorf("renewed catalog %s differs from the cached one, want the same document", diff)
		}
		if got := renewed.ExpiresAt.Sub(renewed.ResolvedAt); got != 600*time.Second {
			t.Errorf("renewed ExpiresAt - ResolvedAt = %v, want 10m0s", got)
		}
	})
}

// TestRefreshFailureDropsCachedCatalog pins REQ-071: when a refresh fails,
// the cached catalog is dropped, so the next resolution fetches again and
// reports the failure instead of serving the old catalog. A refresh that
// fails only because the caller's own context ended is the exception, which
// TestCancelledFetchKeepsCachedCatalog pins.
func TestRefreshFailureDropsCachedCatalog(t *testing.T) { // REQ-071
	tests := []struct {
		name string
		// later answers the Refresh request, the second one. Every later
		// request gets a 503.
		later func(w http.ResponseWriter)
	}{
		{name: "server error", later: func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}},
		{name: "document missing the required service", later: func(w http.ResponseWriter) {
			_, _ = io.WriteString(w, `{"token_endpoint":"https://auth.example.com/token"}`)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var served atomic.Int32
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				switch served.Add(1) {
				case 1:
					w.Header().Set("ETag", `"v1"`)
					w.Header().Set("Cache-Control", "max-age=3600")
					_, _ = io.WriteString(w, smartDocument("", ""))
				case 2:
					tc.later(w)
				default:
					w.WriteHeader(http.StatusServiceUnavailable)
				}
			}))
			defer srv.Close()
			cache := discovery.NewMemoryCache()
			res, err := discovery.NewResolver(cache, discovery.WithHTTPClient(srv.Client()))
			if err != nil {
				t.Fatal(err)
			}
			baseURL := srv.URL + platformPath
			if _, err := res.Resolve(t.Context(), baseURL); err != nil {
				t.Fatalf("Resolve(%q) error = %v", baseURL, err)
			}
			if _, err := res.Refresh(t.Context(), baseURL); err == nil {
				t.Fatalf("Refresh(%q) succeeded, want the %s to fail it", baseURL, tc.name)
			}
			if _, ok := cache.Get(t.Context(), baseURL); ok {
				t.Errorf("cache still holds the old catalog for %q after the failed refresh", baseURL)
			}
			if _, err := res.Resolve(t.Context(), baseURL); err == nil {
				t.Errorf("Resolve(%q) after the failed refresh succeeded, want it to fetch again and fail", baseURL)
			}
		})
	}
}

// heldCache holds every Put, or every Invalidate, until release is closed,
// as a slow networked cache can.
type heldCache struct {
	*discovery.MemoryCache
	holdPut bool // hold Put; otherwise hold Invalidate
	release chan struct{}
}

func (c *heldCache) Put(ctx context.Context, baseURL string, cat *discovery.ServiceCatalog) error {
	if c.holdPut {
		<-c.release
	}
	return c.MemoryCache.Put(ctx, baseURL, cat)
}

func (c *heldCache) Invalidate(ctx context.Context, baseURL string) error {
	if !c.holdPut {
		<-c.release
	}
	return c.MemoryCache.Invalidate(ctx, baseURL)
}

// TestFetchSettlesCacheBeforeTheNextFetch pins REQ-071: a fetch writes its
// result to the cache, storing a success or dropping the entry on a
// failure, before another fetch for the same base URL can start. A call
// that arrives while the cache is written joins that fetch and gets its
// result, so the cache always agrees with what the last call reported: a
// success stored late does not bring a catalog back after a refresh
// reported a failure, and a failure dropped late does not remove the
// catalog a later call reported.
func TestFetchSettlesCacheBeforeTheNextFetch(t *testing.T) { // REQ-071
	type call func(ctx context.Context, res *discovery.Resolver, baseURL string) (*discovery.ServiceCatalog, error)
	tests := []struct {
		name string
		// firstOK makes the first request succeed and every later one get
		// a 503; otherwise the first gets a 503 and every later one succeeds.
		firstOK bool
		// second arrives while the first fetch writes the cache.
		second call
	}{
		{
			name:    "success being stored, then a Refresh",
			firstOK: true,
			second: func(ctx context.Context, res *discovery.Resolver, baseURL string) (*discovery.ServiceCatalog, error) {
				return res.Refresh(ctx, baseURL)
			},
		},
		{
			name: "failure being dropped, then a Resolve",
			second: func(ctx context.Context, res *discovery.Resolver, baseURL string) (*discovery.ServiceCatalog, error) {
				return res.Resolve(ctx, baseURL)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var hits atomic.Int32
				srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					if (hits.Add(1) == 1) != tc.firstOK {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					w.Header().Set("Cache-Control", "max-age=3600")
					_, _ = io.WriteString(w, smartDocument("", ""))
				}))
				cache := &heldCache{MemoryCache: discovery.NewMemoryCache(), holdPut: tc.firstOK, release: make(chan struct{})}
				res, err := discovery.NewResolver(cache, discovery.WithHTTPClient(srv.Client()), discovery.WithAllowInsecure())
				if err != nil {
					t.Fatal(err)
				}
				baseURL := srv.URL + platformPath

				var (
					wg                  sync.WaitGroup
					firstErr, secondErr error
					secondCat           *discovery.ServiceCatalog
				)
				wg.Go(func() { _, firstErr = res.Resolve(t.Context(), baseURL) })
				synctest.Wait() // the first fetch is answered and its cache write held
				wg.Go(func() { secondCat, secondErr = tc.second(t.Context(), res, baseURL) })
				synctest.Wait() // the second call has returned, or waits for the first
				close(cache.release)
				wg.Wait()

				if (firstErr == nil) != tc.firstOK {
					t.Fatalf("first Resolve(%q) error = %v, want success %t", baseURL, firstErr, tc.firstOK)
				}
				if got := hits.Load(); got != 1 {
					t.Errorf("server answered %d requests, want 1: a call arriving while the first fetch writes the cache joins it", got)
				}
				if (secondErr == nil) != (firstErr == nil) {
					t.Errorf("second call error = %v, want the result of the fetch it joined (error = %v)", secondErr, firstErr)
				}
				cached, ok := cache.Get(t.Context(), baseURL)
				switch {
				case secondErr != nil && ok:
					t.Errorf("cache holds a catalog for %q after the last call reported %v, want it dropped so the next Resolve fetches again", baseURL, secondErr)
				case secondErr == nil && (!ok || cached != secondCat):
					t.Errorf("cache.Get(%q) = %p, %t, want the catalog %p the last call returned", baseURL, cached, ok, secondCat)
				}
			})
		})
	}
}
