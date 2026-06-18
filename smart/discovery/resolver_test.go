package discovery

import (
	"context"
	"errors"
	"fmt"
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
	r := mustResolver(
		t,
		WithHTTPClient(srv.Client()),
		WithAcceptedSpecVersions(SpecVersionPin, "1.0.3"),
	)
	if _, err := r.Resolve(context.Background(), srv.URL); err != nil {
		t.Fatalf("widened accept should succeed: %v", err)
	}
}

func TestResolveCacheHit(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
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
	if got := hits.Load(); got != 1 {
		t.Errorf("expected 1 fetch (cache hit on 2nd), got %d", got)
	}
}

func TestResolveCacheExpiry(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write(cassetteBytes(t, "smart-configuration.json"))
	}))
	defer srv.Close()
	r := mustResolver(
		t,
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
	if got := hits.Load(); got != 2 {
		t.Errorf("expected refetch after TTL expiry, got %d hits", got)
	}
}

func TestResolveRefreshInvalidates(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
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
	if got := hits.Load(); got != 2 {
		t.Errorf("expected refresh to refetch, got %d hits", got)
	}
}

func TestResolveCoalescesConcurrent(t *testing.T) {
	var hits atomic.Int32
	gate := make(chan struct{})
	body := cassetteBytes(t, "smart-configuration.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
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
	if got := hits.Load(); got != 1 {
		t.Errorf("expected coalesced fetch, got %d", got)
	}
}

func TestResolveMissingServiceRequired(t *testing.T) {
	body := `{
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
		_, _ = io.WriteString(w, `{"services":{"org.openehr.rest":{"baseUrl":"::not a url"}}}`)
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

func TestResolveIssuerMismatch(t *testing.T) {
	// The document's "issuer" field differs from the URL used to fetch it.
	// Per OIDC Discovery §4.3, Resolve must reject the document and return
	// a *DiscoveryError with ReasonIssuerMismatch.
	body := `{
		"issuer":"https://evil.example.com",
		"authorization_endpoint":"https://evil.example.com/auth",
		"token_endpoint":"https://evil.example.com/token",
		"services":{"org.openehr.rest":{"baseUrl":"https://api.example.com/openehr/v1","spec_version":"1.1.0-development"}}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	r := mustResolver(t, WithHTTPClient(srv.Client()))
	cat, err := r.Resolve(context.Background(), srv.URL)
	if cat != nil {
		t.Error("expected nil catalog on issuer mismatch")
	}
	var derr *DiscoveryError
	if !errors.As(err, &derr) || derr.Reason != ReasonIssuerMismatch {
		t.Fatalf("expected issuer_mismatch DiscoveryError, got %v", err)
	}
}

func TestResolveIssuerMatch(t *testing.T) {
	// Start an unstarted server so we know srv.URL before building the body.
	var srv *httptest.Server
	srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{
			"issuer":"` + srv.URL + `",
			"authorization_endpoint":"https://auth.example.com/auth",
			"token_endpoint":"https://auth.example.com/token",
			"services":{"org.openehr.rest":{"baseUrl":"https://api.example.com/openehr/v1","spec_version":"1.1.0-development"}}
		}`
		_, _ = io.WriteString(w, body)
	}))
	srv.Start()
	defer srv.Close()
	r := mustResolver(t, WithHTTPClient(srv.Client()))
	cat, err := r.Resolve(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("expected success when document issuer matches requested issuer, got: %v", err)
	}
	if cat.Issuer != srv.URL {
		t.Errorf("catalog.Issuer = %q, want %q", cat.Issuer, srv.URL)
	}
}

// TestResolveInsecureEndpointRejectedStrict verifies that a discovery
// document containing a non-https endpoint URL (here jwks_uri) is
// rejected with ReasonInsecureURL when the resolver runs in strict mode
// (no WithAllowInsecure). The issuer itself is served over TLS so the
// issuer-level check does not interfere.
func TestResolveInsecureEndpointRejectedStrict(t *testing.T) {
	body := `{
		"authorization_endpoint":"http://attacker.example/auth",
		"token_endpoint":"http://attacker.example/token",
		"jwks_uri":"http://attacker.example/keys",
		"services":{"org.openehr.rest":{"baseUrl":"https://api.example.com/openehr/v1","spec_version":"1.1.0-development"}}
	}`
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	// Strict resolver: no WithAllowInsecure. Use the TLS server's client so
	// the self-signed cert is trusted.
	r, err := NewResolver(NewMemoryCache(), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	cat, err := r.Resolve(context.Background(), srv.URL)
	if cat != nil {
		t.Error("expected nil catalog when endpoint URLs are non-https in strict mode")
	}
	var derr *DiscoveryError
	if !errors.As(err, &derr) || derr.Reason != ReasonInsecureURL {
		t.Fatalf("expected insecure_url DiscoveryError, got %v", err)
	}
}

// TestResolveInsecureEndpointAllowedWhenAllowInsecure verifies that the
// same non-https endpoint URLs are accepted (with a warning) when the
// resolver is configured with WithAllowInsecure.
func TestResolveInsecureEndpointAllowedWhenAllowInsecure(t *testing.T) {
	body := `{
		"authorization_endpoint":"http://dev.example/auth",
		"token_endpoint":"http://dev.example/token",
		"jwks_uri":"http://dev.example/keys",
		"services":{"org.openehr.rest":{"baseUrl":"https://api.example.com/openehr/v1","spec_version":"1.1.0-development"}}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	// mustResolver includes WithAllowInsecure, so both the issuer fetch
	// and the endpoint-URL scheme check should permit http.
	r := mustResolver(t, WithHTTPClient(srv.Client()))
	cat, err := r.Resolve(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("WithAllowInsecure should permit http endpoint URLs, got: %v", err)
	}
	if cat == nil {
		t.Fatal("expected non-nil catalog")
	}
}

// TestResolveCanonicalServicesMap verifies that the resolver correctly parses
// a SMART configuration document whose "services" field uses the canonical
// JSON object/map shape with camelCase "baseUrl" key (REQ-070).
//
// The SDK previously decoded "services" as an array with snake_case "base_url",
// which is non-canonical and causes discovery to fail against real platforms.
// This test uses the correct canonical shape and must fail before the fix.
func TestResolveCanonicalServicesMap(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{
		  "issuer": %q,
		  "authorization_endpoint": %q,
		  "token_endpoint": %q,
		  "services": {"org.openehr.rest": {"baseUrl": %q}}
		}`, srv.URL, srv.URL+"/authorize", srv.URL+"/token", srv.URL+"/openehr/v1")
	}))
	defer srv.Close()
	res, err := NewResolver(nil, WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	cat, err := res.Resolve(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	e, ok := cat.OpenEHRRest()
	if !ok || e.BaseURL == nil {
		t.Fatalf("org.openehr.rest missing or no BaseURL: %#v", cat.Services)
	}
	if e.BaseURL.String() != srv.URL+"/openehr/v1" {
		t.Errorf("BaseURL = %q, want %q", e.BaseURL.String(), srv.URL+"/openehr/v1")
	}
}

// TestResolveSurfacesAuthMetadata verifies that the resolver surfaces all
// SMART authorization-server metadata fields onto AuthEndpoints (REQ-062,
// REQ-070): the three optional endpoint URLs (introspection, revocation,
// management), the token-endpoint auth signing-alg list, the id-token
// signing-alg list, and the token-endpoint auth-methods list.
//
// This is surface-only: the test asserts the values are parsed and
// propagated — no consuming logic (alg selection, method selection, etc.)
// is tested here.
func TestResolveSurfacesAuthMetadata(t *testing.T) { // REQ-062, REQ-070
	var srv *httptest.Server
	srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{
			"issuer": "` + srv.URL + `",
			"authorization_endpoint": "https://auth.example.com/authorize",
			"token_endpoint": "https://auth.example.com/token",
			"jwks_uri": "https://auth.example.com/jwks",
			"introspection_endpoint": "https://auth.example.com/introspect",
			"revocation_endpoint": "https://auth.example.com/revoke",
			"management_endpoint": "https://auth.example.com/manage",
			"token_endpoint_auth_methods_supported": ["private_key_jwt", "client_secret_basic"],
			"token_endpoint_auth_signing_alg_values_supported": ["RS384", "ES384"],
			"id_token_signing_alg_values_supported": ["RS256", "ES384"],
			"services": {"org.openehr.rest": {"baseUrl": "https://api.example.com/openehr/v1"}}
		}`
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	srv.Start()
	defer srv.Close()

	r := mustResolver(t, WithHTTPClient(srv.Client()))
	cat, err := r.Resolve(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	auth := cat.Auth

	// --- three optional endpoint URLs (full serialized URL, not partial field) ---
	if got := auth.IntrospectionEndpoint.String(); got != "https://auth.example.com/introspect" {
		t.Errorf("IntrospectionEndpoint = %v, want https://auth.example.com/introspect", got)
	}
	if got := auth.RevocationEndpoint.String(); got != "https://auth.example.com/revoke" {
		t.Errorf("RevocationEndpoint = %v, want https://auth.example.com/revoke", got)
	}
	if got := auth.ManagementEndpoint.String(); got != "https://auth.example.com/manage" {
		t.Errorf("ManagementEndpoint = %v, want https://auth.example.com/manage", got)
	}

	// --- auth-methods list (may already be surfaced by Phase 1; verify value) ---
	if !containsString(auth.TokenEndpointAuthMethodsSupported, "private_key_jwt") {
		t.Errorf("TokenEndpointAuthMethodsSupported = %v, want [private_key_jwt ...]", auth.TokenEndpointAuthMethodsSupported)
	}

	// --- token-endpoint signing-alg list (feeds Phase 3b G-3; surface only) ---
	if !containsString(auth.TokenEndpointAuthSigningAlgValuesSupported, "RS384") {
		t.Errorf("TokenEndpointAuthSigningAlgValuesSupported = %v, want [RS384 ES384]", auth.TokenEndpointAuthSigningAlgValuesSupported)
	}
	if !containsString(auth.TokenEndpointAuthSigningAlgValuesSupported, "ES384") {
		t.Errorf("TokenEndpointAuthSigningAlgValuesSupported = %v, want [RS384 ES384]", auth.TokenEndpointAuthSigningAlgValuesSupported)
	}

	// --- id-token signing-alg list (feeds Phase 3e verify allowlist; surface only) ---
	if !containsString(auth.IDTokenSigningAlgValuesSupported, "RS256") {
		t.Errorf("IDTokenSigningAlgValuesSupported = %v, want [RS256 ES384]", auth.IDTokenSigningAlgValuesSupported)
	}
	if !containsString(auth.IDTokenSigningAlgValuesSupported, "ES384") {
		t.Errorf("IDTokenSigningAlgValuesSupported = %v, want [RS256 ES384]", auth.IDTokenSigningAlgValuesSupported)
	}
}

// TestResolveSurfacesAuthMetadata_AbsentEndpointsAreNil verifies that optional
// endpoint fields are nil when the discovery document omits them (REQ-062).
// This solidifies the "Nil when absent" contract for IntrospectionEndpoint,
// RevocationEndpoint, and ManagementEndpoint.
func TestResolveSurfacesAuthMetadata_AbsentEndpointsAreNil(t *testing.T) { // REQ-062
	t.Run("absent endpoints are nil", func(t *testing.T) {
		var srv *httptest.Server
		srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Minimal valid discovery doc — no introspection/revocation/management.
			body := `{
				"issuer": "` + srv.URL + `",
				"authorization_endpoint": "https://auth.example.com/authorize",
				"token_endpoint": "https://auth.example.com/token",
				"jwks_uri": "https://auth.example.com/jwks",
				"services": {"org.openehr.rest": {"baseUrl": "https://api.example.com/openehr/v1"}}
			}`
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, body)
		}))
		srv.Start()
		defer srv.Close()

		r := mustResolver(t, WithHTTPClient(srv.Client()))
		cat, err := r.Resolve(context.Background(), srv.URL)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		auth := cat.Auth

		if auth.IntrospectionEndpoint != nil {
			t.Errorf("IntrospectionEndpoint = %v, want nil when absent from discovery doc", auth.IntrospectionEndpoint)
		}
		if auth.RevocationEndpoint != nil {
			t.Errorf("RevocationEndpoint = %v, want nil when absent from discovery doc", auth.RevocationEndpoint)
		}
		if auth.ManagementEndpoint != nil {
			t.Errorf("ManagementEndpoint = %v, want nil when absent from discovery doc", auth.ManagementEndpoint)
		}
	})
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
