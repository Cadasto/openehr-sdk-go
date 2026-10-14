package discovery_test

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

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
