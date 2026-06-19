package system_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/openehr/client/system"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// newCatalog returns a static catalog rooted at srv.URL + "/openehr/v1".
// Mirrors the convention from transport's tests so the request path
// composes identically.
func newCatalog(t *testing.T, srv *httptest.Server) *discovery.ServiceCatalog {
	t.Helper()
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://test.example.com",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func newClient(t *testing.T, srv *httptest.Server) *transport.Client {
	t.Helper()
	c, err := transport.New(newCatalog(t, srv), transport.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// readCassette returns the bytes of a vendored cassette at
// testkit/cassettes/its_rest/<dir>/<name>.
func readCassette(t *testing.T, dir, name string) []byte {
	t.Helper()
	_, src, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(src), "..", "..", "..", "testkit", "cassettes", "its_rest", dir, name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cassette %q: %v", path, err)
	}
	return b
}

func TestCapabilitiesDecodesCassette(t *testing.T) {
	var captured *http.Request
	body := readCassette(t, "system", "capabilities.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := newClient(t, srv)
	caps, meta, err := system.Capabilities(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if meta == nil {
		t.Fatal("expected non-nil metadata")
	}
	if captured.Method != http.MethodOptions {
		t.Errorf("method = %q, want OPTIONS", captured.Method)
	}
	if captured.URL.Path != "/openehr/v1/" {
		t.Errorf("path = %q, want /openehr/v1/", captured.URL.Path)
	}
	if caps.Solution != "Cadasto" {
		t.Errorf("Solution = %q", caps.Solution)
	}
	if caps.SolutionVersion != "2.4.0" {
		t.Errorf("SolutionVersion = %q", caps.SolutionVersion)
	}
	if caps.Vendor != "Cadasto" {
		t.Errorf("Vendor = %q", caps.Vendor)
	}
	if caps.RESTAPISpecsVersion != "1.1.0-development" {
		t.Errorf("RESTAPISpecsVersion = %q", caps.RESTAPISpecsVersion)
	}
	if caps.ConformanceProfile != "default" {
		t.Errorf("ConformanceProfile = %q", caps.ConformanceProfile)
	}
	if !slices.Contains(caps.Endpoints, "/query/aql") {
		t.Errorf("Endpoints missing /query/aql: %v", caps.Endpoints)
	}
}

func TestCapabilitiesPreservesExtras(t *testing.T) {
	body := readCassette(t, "system", "capabilities.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	caps, _, err := system.Capabilities(context.Background(), newClient(t, srv))
	if err != nil {
		t.Fatal(err)
	}
	if caps.Extras == nil {
		t.Fatal("Extras should be populated for deployment-specific fields")
	}
	for _, key := range []string{"support_email", "documentation_url", "supported_formats"} {
		if _, ok := caps.Extras[key]; !ok {
			t.Errorf("Extras missing %q (have keys: %v)", key, extrasKeys(caps.Extras))
		}
	}
	// Spot-check decode of one Extras value.
	var email string
	if err := json.Unmarshal(caps.Extras["support_email"], &email); err != nil {
		t.Fatal(err)
	}
	if email != "support@cadasto.example" {
		t.Errorf("support_email = %q", email)
	}
}

func TestCapabilitiesRoundTripsExtras(t *testing.T) {
	body := readCassette(t, "system", "capabilities.json")
	var sc system.ServiceCapabilities
	if err := json.Unmarshal(body, &sc); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(sc)
	if err != nil {
		t.Fatal(err)
	}
	var roundTripped system.ServiceCapabilities
	if err := json.Unmarshal(out, &roundTripped); err != nil {
		t.Fatal(err)
	}
	if roundTripped.Solution != sc.Solution {
		t.Errorf("Solution drifted: %q vs %q", roundTripped.Solution, sc.Solution)
	}
	if len(roundTripped.Extras) != len(sc.Extras) {
		t.Errorf("Extras count drifted: %d vs %d", len(roundTripped.Extras), len(sc.Extras))
	}
	for k := range sc.Extras {
		if _, ok := roundTripped.Extras[k]; !ok {
			t.Errorf("Extras key %q lost in round-trip", k)
		}
	}
}

func TestVersion(t *testing.T) {
	body := readCassette(t, "system", "capabilities.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	v, err := system.Version(context.Background(), newClient(t, srv))
	if err != nil {
		t.Fatal(err)
	}
	if v != "1.1.0-development" {
		t.Errorf("Version = %q, want 1.1.0-development", v)
	}
}

func TestCapabilitiesSurfacesWireError(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		sentinel error
	}{
		{"401", 401, `{"message":"unauthorized","code":"UNAUTHENTICATED"}`, transport.ErrUnauthorized},
		{"404", 404, `{"message":"not found","code":"NOT_FOUND"}`, transport.ErrNotFound},
		{"500", 500, `{"message":"oops","code":"INTERNAL"}`, transport.ErrServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			_, _, err := system.Capabilities(context.Background(), newClient(t, srv))
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, tc.sentinel) {
				t.Errorf("expected errors.Is %v, got %v", tc.sentinel, err)
			}
			var we *transport.WireError
			if !errors.As(err, &we) || we.OpenEHR == nil {
				t.Errorf("expected WireError with OpenEHR detail, got %v", err)
			}
		})
	}
}

func TestCapabilitiesEmptyBodyIsInvalidShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 200 with no body — server bug.
		w.Header().Set("Content-Type", "application/json")
	}))
	defer srv.Close()
	_, _, err := system.Capabilities(context.Background(), newClient(t, srv))
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Errorf("expected ErrInvalidShape, got %v", err)
	}
}

func TestHealthUp(t *testing.T) {
	body := readCassette(t, "system", "capabilities.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	h, err := system.Health(context.Background(), newClient(t, srv))
	if err != nil {
		t.Fatal(err)
	}
	if !h.IsUp() {
		t.Errorf("IsUp() false; HealthStatus = %+v", h)
	}
	if h.HTTPStatusCode != 200 {
		t.Errorf("HTTPStatusCode = %d", h.HTTPStatusCode)
	}
	if h.CheckedAt.IsZero() {
		t.Error("CheckedAt unset")
	}
}

func TestHealthDownOnWireError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()
	h, err := system.Health(context.Background(), newClient(t, srv))
	if err != nil {
		t.Fatalf("expected wire error to fold into HealthStatus, got err: %v", err)
	}
	if h.IsUp() {
		t.Error("expected IsUp()=false on 503")
	}
	if h.HTTPStatusCode != 503 {
		t.Errorf("HTTPStatusCode = %d, want 503", h.HTTPStatusCode)
	}
}

func TestHealthIsAnonymous(t *testing.T) {
	// Health MUST NOT emit Authorization even when a TokenSource is
	// configured. Monitoring tools commonly run without credentials.
	var seenAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"solution":"x"}`))
	}))
	defer srv.Close()
	c, _ := transport.New(
		newCatalog(t, srv),
		transport.WithHTTPClient(srv.Client()),
		transport.WithTokenSource(auth.StaticTokenSource(auth.Token{Value: "should-not-leak", Type: "Bearer"})),
	)
	if _, err := system.Health(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if seenAuth != "" {
		t.Errorf("Health emitted Authorization = %q (must be anonymous)", seenAuth)
	}
}

func TestHealthDownOnNetworkError(t *testing.T) {
	// Client targets a port that nobody listens on.
	cat, _ := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://x",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL("http://127.0.0.1:1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	c, _ := transport.New(cat, transport.WithHTTPClient(&http.Client{Timeout: 50 * time.Millisecond}))
	h, err := system.Health(context.Background(), c)
	if err == nil {
		t.Fatal("expected network error to surface")
	}
	if h == nil || h.IsUp() {
		t.Errorf("expected HealthStatus with Status=down, got %+v", h)
	}
}

func TestRepositoryMirrorsPackageFunctions(t *testing.T) {
	body := readCassette(t, "system", "capabilities.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	repo := system.NewRepository(newClient(t, srv))

	caps, _, err := repo.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if caps.Solution != "Cadasto" {
		t.Errorf("Repository.Capabilities Solution = %q", caps.Solution)
	}
	v, err := repo.Version(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v != "1.1.0-development" {
		t.Errorf("Repository.Version = %q", v)
	}
	h, err := repo.Health(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !h.IsUp() {
		t.Error("Repository.Health expected up")
	}
}

func extrasKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
