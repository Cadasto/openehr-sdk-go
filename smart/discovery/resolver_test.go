package discovery

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func cassettePath(t *testing.T, name string) string {
	t.Helper()
	_, src, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(src), "..", "..", "testkit", "cassettes", "its_rest", "discovery", name)
}

func cassetteBytes(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(cassettePath(t, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newCassetteServer(t *testing.T, name string, hdr func(http.Header)) *httptest.Server {
	t.Helper()
	body := cassetteBytes(t, name)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if hdr != nil {
			hdr(w.Header())
		}
		_, _ = w.Write(body)
	}))
}

func mustResolver(t *testing.T, opts ...Option) *Resolver {
	t.Helper()
	r, err := NewResolver(NewMemoryCache(), append([]Option{WithHTTPClient(http.DefaultClient), WithAllowInsecure()}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestResolveCassette(t *testing.T) {
	srv := newCassetteServer(t, "smart-configuration.json", nil)
	defer srv.Close()
	r := mustResolver(t, WithHTTPClient(srv.Client()))
	cat, err := r.Resolve(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if cat.Auth.AuthorizationEndpoint == nil || cat.Auth.AuthorizationEndpoint.Host != "auth.example.com" {
		t.Errorf("authorization_endpoint = %v", cat.Auth.AuthorizationEndpoint)
	}
	if cat.Auth.TokenEndpoint == nil || cat.Auth.TokenEndpoint.Host != "auth.example.com" {
		t.Errorf("token_endpoint = %v", cat.Auth.TokenEndpoint)
	}
	rest, ok := cat.OpenEHRRest()
	if !ok {
		t.Fatal("OpenEHRRest service missing")
	}
	if rest.SpecVersion != "1.1.0-development" {
		t.Errorf("spec_version = %q", rest.SpecVersion)
	}
	if rest.BaseURL.String() != "https://api.example.com/openehr/v1" {
		t.Errorf("base_url = %q", rest.BaseURL.String())
	}
	if !containsString(cat.Auth.ResponseTypesSupported, "code") {
		t.Errorf("response_types_supported = %v", cat.Auth.ResponseTypesSupported)
	}
	if !containsString(cat.Auth.CodeChallengeMethodsSupported, "S256") {
		t.Errorf("code_challenge_methods_supported = %v", cat.Auth.CodeChallengeMethodsSupported)
	}
}

func TestResolveSpecVersionMismatch(t *testing.T) {
	srv := newCassetteServer(t, "smart-configuration-mismatch.json", nil)
	defer srv.Close()
	r := mustResolver(t, WithHTTPClient(srv.Client()))
	_, err := r.Resolve(context.Background(), srv.URL)
	var derr *DiscoveryError
	if !errors.As(err, &derr) || derr.Reason != ReasonSpecVersionMismatch {
		t.Fatalf("expected spec_version_mismatch, got %v", err)
	}
	if derr.SpecVersionGot != "1.0.3" {
		t.Errorf("got = %q", derr.SpecVersionGot)
	}
}

func TestResolveAcceptedVersionsWiden(t *testing.T) {
	srv := newCassetteServer(t, "smart-configuration-mismatch.json", nil)
	defer srv.Close()
	r := mustResolver(t,
		WithHTTPClient(srv.Client()),
		WithAcceptedSpecVersions(SpecVersionPin, "1.0.3"),
	)
	if _, err := r.Resolve(context.Background(), srv.URL); err != nil {
		t.Fatalf("widened accept should succeed: %v", err)
	}
}

func TestResolveCacheHit(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Cache-Control", "max-age=300")
		_, _ = w.Write(cassetteBytes(t, "smart-configuration.json"))
	}))
	defer srv.Close()
	r := mustResolver(t, WithHTTPClient(srv.Client()))
	if _, err := r.Resolve(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("expected 1 fetch (cache hit on 2nd), got %d", got)
	}
}

func TestResolveCacheExpiry(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write(cassetteBytes(t, "smart-configuration.json"))
	}))
	defer srv.Close()
	r := mustResolver(t,
		WithHTTPClient(srv.Client()),
		WithDefaultTTL(1*time.Millisecond),
	)
	if _, err := r.Resolve(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, err := r.Resolve(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("expected refetch after TTL expiry, got %d hits", got)
	}
}

func TestResolveRefreshInvalidates(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = w.Write(cassetteBytes(t, "smart-configuration.json"))
	}))
	defer srv.Close()
	r := mustResolver(t, WithHTTPClient(srv.Client()))
	if _, err := r.Resolve(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Refresh(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("expected refresh to refetch, got %d hits", got)
	}
}

func TestResolveCoalescesConcurrent(t *testing.T) {
	var hits int32
	gate := make(chan struct{})
	body := cassetteBytes(t, "smart-configuration.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		<-gate
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	r := mustResolver(t, WithHTTPClient(srv.Client()))
	var wg sync.WaitGroup
	const N = 8
	wg.Add(N)
	for range N {
		go func() {
			defer wg.Done()
			_, _ = r.Resolve(context.Background(), srv.URL)
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(gate)
	wg.Wait()
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("expected coalesced fetch, got %d", got)
	}
}

func TestResolveMissingServiceRequired(t *testing.T) {
	body := `{
        "issuer":"https://x",
        "authorization_endpoint":"https://x/a",
        "token_endpoint":"https://x/t",
        "response_types_supported":["code"],
        "code_challenge_methods_supported":["S256"]
    }`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	r := mustResolver(t, WithHTTPClient(srv.Client()))
	_, err := r.Resolve(context.Background(), srv.URL)
	var derr *DiscoveryError
	if !errors.As(err, &derr) || derr.Reason != ReasonMissingService {
		t.Fatalf("expected missing_service, got %v", err)
	}
	if len(derr.MissingServices) != 1 || derr.MissingServices[0] != ServiceIDOpenEHRRest {
		t.Errorf("MissingServices = %v", derr.MissingServices)
	}
}

func TestResolveMalformedURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"issuer":"x","services":[{"id":"org.openehr.rest","base_url":"::not a url"}]}`)
	}))
	defer srv.Close()
	r := mustResolver(t, WithHTTPClient(srv.Client()))
	_, err := r.Resolve(context.Background(), srv.URL)
	var derr *DiscoveryError
	if !errors.As(err, &derr) || derr.Reason != ReasonMalformedURL {
		t.Fatalf("expected malformed_url, got %v", err)
	}
}

func TestResolveParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `<<not json>>`)
	}))
	defer srv.Close()
	r := mustResolver(t, WithHTTPClient(srv.Client()))
	_, err := r.Resolve(context.Background(), srv.URL)
	var derr *DiscoveryError
	if !errors.As(err, &derr) || derr.Reason != ReasonParseError {
		t.Fatalf("expected parse_error, got %v", err)
	}
}

func TestResolveFetchFailedNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()
	r := mustResolver(t, WithHTTPClient(srv.Client()))
	_, err := r.Resolve(context.Background(), srv.URL)
	var derr *DiscoveryError
	if !errors.As(err, &derr) || derr.Reason != ReasonFetchFailed {
		t.Fatalf("expected fetch_failed, got %v", err)
	}
}

func TestResolveInsecureIssuerRejected(t *testing.T) {
	r, err := NewResolver(NewMemoryCache(), WithHTTPClient(http.DefaultClient))
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Resolve(context.Background(), "http://insecure.example.com")
	var derr *DiscoveryError
	if !errors.As(err, &derr) || derr.Reason != ReasonInsecureURL {
		t.Fatalf("expected insecure_url, got %v", err)
	}
}

func TestNewStaticCatalog(t *testing.T) {
	base := MustParseURL("https://api.example.com/openehr/v1")
	cat, err := NewStaticCatalog(StaticConfig{
		Issuer: "https://auth.example.com",
		Services: map[string]ServiceEntry{
			ServiceIDOpenEHRRest: {BaseURL: base, SpecVersion: SpecVersionPin},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	e, ok := cat.OpenEHRRest()
	if !ok {
		t.Fatal("OpenEHRRest missing")
	}
	if e.ID != ServiceIDOpenEHRRest {
		t.Errorf("ID = %q", e.ID)
	}
	// Mutating the original entry must not affect the catalog (defensive clone).
	base.Path = "/changed"
	if got, _ := cat.OpenEHRRest(); got.BaseURL.Path == "/changed" {
		t.Error("catalog leaked mutable BaseURL")
	}
}

func TestNewStaticCatalogValidates(t *testing.T) {
	_, err := NewStaticCatalog(StaticConfig{})
	if !asDiscoveryError(err, ReasonParseError) {
		t.Errorf("expected parse_error, got %v", err)
	}
	_, err = NewStaticCatalog(StaticConfig{
		Issuer: "https://x",
		Services: map[string]ServiceEntry{
			"x": {BaseURL: nil},
		},
	})
	if !asDiscoveryError(err, ReasonMalformedURL) {
		t.Errorf("expected malformed_url, got %v", err)
	}
}

func TestStaleCatalog(t *testing.T) {
	cat := &ServiceCatalog{ExpiresAt: time.Now().Add(-time.Second)}
	if !cat.Stale(time.Now()) {
		t.Error("expired catalog should be stale")
	}
	cat2 := &ServiceCatalog{} // zero ExpiresAt
	if cat2.Stale(time.Now()) {
		t.Error("zero-expiry catalog should not be stale")
	}
}

func asDiscoveryError(err error, want DiscoveryErrorReason) bool {
	var derr *DiscoveryError
	if !errors.As(err, &derr) {
		return false
	}
	return derr.Reason == want
}

func containsString(s []string, v string) bool {
	return slices.Contains(s, v)
}
