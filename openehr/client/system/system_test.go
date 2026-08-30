package system_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
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
// compactJSON is the fair comparison for a preserved Extras value. Extras
// holds the wire bytes exactly as decoded, but re-encoding runs them
// through encoding/json, which compacts insignificant whitespace — so the
// cassette's `["application/json", "application/xml"]` comes back without
// the space and nothing is lost. Comparing raw bytes would fail on the
// spacing alone.
func compactJSON(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		t.Fatalf("Compact(%s) = %v, want nil error", raw, err)
	}
	return buf.String()
}

// assertEmittedKeys checks that the encoded object carries each named key
// with the given JSON value. It reads the raw object rather than a decoded
// ServiceCapabilities because that is the only way to see two keys
// differing only by case: decoding folds them onto one documented field.
func assertEmittedKeys(t *testing.T, out []byte, want map[string]string) {
	t.Helper()
	if len(want) == 0 {
		return
	}
	var emitted map[string]json.RawMessage
	if err := json.Unmarshal(out, &emitted); err != nil {
		t.Fatalf("Unmarshal(%s) into a raw object = %v, want nil error", out, err)
	}
	for k, w := range want {
		raw, ok := emitted[k]
		if !ok {
			t.Errorf("encode dropped key %q: %s", k, out)
			continue
		}
		if w := compactJSON(t, json.RawMessage(w)); string(raw) != w {
			t.Errorf("encoded[%q] = %s, want %s", k, raw, w)
		}
	}
}

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
	caps, meta, err := system.Capabilities(t.Context(), c)
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

	caps, _, err := system.Capabilities(t.Context(), newClient(t, srv))
	if err != nil {
		t.Fatal(err)
	}
	if caps.Extras == nil {
		t.Fatal("Extras should be populated for deployment-specific fields")
	}
	for _, key := range []string{"support_email", "documentation_url", "supported_formats"} {
		if _, ok := caps.Extras[key]; !ok {
			t.Errorf("Extras missing %q (have keys: %v)", key, slices.Sorted(maps.Keys(caps.Extras)))
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

// TestCapabilitiesRoundTripsExtras pins the capabilities encode contract:
// the documented fields a value carries before encode come back unchanged
// after a Marshal/Unmarshal round trip, only the unknown keys ride along
// in Extras, and an Extras key colliding with a documented field name is
// ignored — the documented field is authoritative.
//
// The collision is always caller-constructed: UnmarshalJSON never routes a
// documented field name into Extras. The two collision rows cover both
// mechanisms — a documented field that emits a key (the overlay wins it
// back) and one that emits none because it is empty and every documented
// field here carries omitempty (only deleting the known key from the
// Extras clone can ignore that one). The last row pins the other edge of
// the rule: collision is decided by exact comparison, so a key differing
// from a documented name only by case is not one.
func TestCapabilitiesRoundTripsExtras(t *testing.T) {
	for _, tc := range []struct {
		label string
		body  []byte
		// inject is applied to the decoded value before re-encoding.
		inject map[string]json.RawMessage
		// caseVariant, when set, is a wire key differing from a documented
		// field name only by case. It must populate the documented field
		// (encoding/json matches field names case-insensitively) AND be kept
		// in Extras (the known-field set is matched exactly).
		caseVariant string
		// wantEmitted names keys the encoded object itself must carry, read
		// from the raw JSON — the only way to see two keys that differ only
		// by case, since decoding folds them onto one field.
		wantEmitted map[string]string
	}{
		{
			label: "cassette",
			body:  readCassette(t, "system", "capabilities.json"),
		},
		{
			label: "colliding extra on an emitted field is ignored",
			body:  []byte(`{"solution":"Cadasto","support_email":"support@cadasto.example"}`),
			inject: map[string]json.RawMessage{
				"solution": json.RawMessage(`"caller-supplied solution"`),
			},
		},
		{
			label: "colliding extra on an omitted field is ignored",
			// No solution and no endpoints on the wire, so both stay empty and
			// omitempty emits no key for either — the case a last-wins merge
			// cannot survive.
			body: []byte(`{"vendor":"Cadasto","support_email":"support@cadasto.example"}`),
			inject: map[string]json.RawMessage{
				"solution":  json.RawMessage(`"caller-supplied solution"`),
				"endpoints": json.RawMessage(`["/caller/supplied"]`),
			},
		},
		{
			label: "case-variant key rides beside the documented field",
			// "Solution" differs from the documented `solution` only by case,
			// so it is not a collision: it populates Solution on decode and is
			// preserved in Extras, and encode emits both keys. Solution is
			// non-empty, so its omitempty key is emitted too.
			body:        []byte(`{"Solution":"Cadasto","support_email":"support@cadasto.example"}`),
			caseVariant: "Solution",
			wantEmitted: map[string]string{"solution": `"Cadasto"`, "Solution": `"Cadasto"`},
		},
	} {
		t.Run(tc.label, func(t *testing.T) {
			var sc system.ServiceCapabilities
			if err := json.Unmarshal(tc.body, &sc); err != nil {
				t.Fatalf("Unmarshal(wire) = %v, want nil error", err)
			}
			// Snapshot the decoded value before injection: it is the want for
			// every documented field and for the Extras key set. want.Extras
			// is cleared so nothing reads it through the alias the copy keeps
			// on the map injection is about to mutate.
			wantExtras := maps.Clone(sc.Extras)
			want := sc
			want.Extras = nil
			if tc.caseVariant != "" {
				if sc.Solution != "Cadasto" {
					t.Errorf("Solution = %q after decoding a body whose only solution key is %q, want Cadasto — encoding/json matches field names case-insensitively",
						sc.Solution, tc.caseVariant)
				}
				if raw := sc.Extras[tc.caseVariant]; string(raw) != `"Cadasto"` {
					t.Errorf(`Extras[%q] = %s, want "Cadasto" preserved verbatim — the known-field set is matched exactly, so a case variant is not a documented name`,
						tc.caseVariant, raw)
				}
			}
			for k, v := range tc.inject {
				if sc.Extras == nil {
					sc.Extras = map[string]json.RawMessage{}
				}
				sc.Extras[k] = v
			}
			out, err := json.Marshal(sc)
			if err != nil {
				t.Fatalf("Marshal = %v, want nil error", err)
			}
			assertEmittedKeys(t, out, tc.wantEmitted)
			var got system.ServiceCapabilities
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("Unmarshal(re-encoded %s) = %v, want nil error", out, err)
			}
			if got.Solution != want.Solution {
				t.Errorf("Solution = %q, want %q (the documented field is authoritative)", got.Solution, want.Solution)
			}
			if got.SolutionVersion != want.SolutionVersion {
				t.Errorf("SolutionVersion = %q, want %q", got.SolutionVersion, want.SolutionVersion)
			}
			if got.Vendor != want.Vendor {
				t.Errorf("Vendor = %q, want %q", got.Vendor, want.Vendor)
			}
			if got.RESTAPISpecsVersion != want.RESTAPISpecsVersion {
				t.Errorf("RESTAPISpecsVersion = %q, want %q", got.RESTAPISpecsVersion, want.RESTAPISpecsVersion)
			}
			if got.ConformanceProfile != want.ConformanceProfile {
				t.Errorf("ConformanceProfile = %q, want %q", got.ConformanceProfile, want.ConformanceProfile)
			}
			if !slices.Equal(got.Endpoints, want.Endpoints) {
				t.Errorf("Endpoints = %v, want %v (the documented field is authoritative)", got.Endpoints, want.Endpoints)
			}
			if len(got.Extras) != len(wantExtras) {
				t.Errorf("Extras = %v, want exactly the %d unknown key(s) %v",
					slices.Sorted(maps.Keys(got.Extras)), len(wantExtras), slices.Sorted(maps.Keys(wantExtras)))
			}
			for k, wantRaw := range wantExtras {
				raw, ok := got.Extras[k]
				if !ok {
					t.Errorf("Extras[%q] dropped on re-encode: %s", k, out)
					continue
				}
				if want := compactJSON(t, wantRaw); string(raw) != want {
					t.Errorf("Extras[%q] = %s, want %s", k, raw, want)
				}
			}
		})
	}
}

func TestVersion(t *testing.T) {
	body := readCassette(t, "system", "capabilities.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	v, err := system.Version(t.Context(), newClient(t, srv))
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
			_, _, err := system.Capabilities(t.Context(), newClient(t, srv))
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, tc.sentinel) {
				t.Errorf("expected errors.Is %v, got %v", tc.sentinel, err)
			}
			we, ok := errors.AsType[*transport.WireError](err)
			if !ok || we.OpenEHR == nil {
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
	_, _, err := system.Capabilities(t.Context(), newClient(t, srv))
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
	h, err := system.Health(t.Context(), newClient(t, srv))
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
	h, err := system.Health(t.Context(), newClient(t, srv))
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
	var seenAuth, seenMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenMethod = r.Method
		_, _ = w.Write([]byte(`{"solution":"x"}`))
	}))
	defer srv.Close()
	c, _ := transport.New(
		newCatalog(t, srv),
		transport.WithHTTPClient(srv.Client()),
		transport.WithTokenSource(auth.StaticTokenSource(auth.Token{Value: "should-not-leak", Type: "Bearer"})),
	)
	if _, err := system.Health(t.Context(), c); err != nil {
		t.Fatal(err)
	}
	if seenAuth != "" {
		t.Errorf("Health emitted Authorization = %q (must be anonymous)", seenAuth)
	}
	if seenMethod != http.MethodOptions {
		t.Errorf("Health method = %q, want OPTIONS", seenMethod)
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
	h, err := system.Health(t.Context(), c)
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

	caps, _, err := repo.Capabilities(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if caps.Solution != "Cadasto" {
		t.Errorf("Repository.Capabilities Solution = %q", caps.Solution)
	}
	v, err := repo.Version(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if v != "1.1.0-development" {
		t.Errorf("Repository.Version = %q", v)
	}
	h, err := repo.Health(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !h.IsUp() {
		t.Error("Repository.Health expected up")
	}
}
