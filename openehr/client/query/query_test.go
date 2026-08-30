package query_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/client/query"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func newClient(t *testing.T, srv *httptest.Server) *transport.Client {
	t.Helper()
	cat, _ := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://test.example.com",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	c, err := transport.New(cat, transport.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// newRawErrorBodiesClient is newClient plus transport.WithRawErrorBodies(true)
// — the deployment opt-in that lets an openEHR error envelope's message text
// reach the caller at all (it is suppressed by default because it may carry
// PHI). A test that must prove a classifier does NOT read message text needs
// the text to actually arrive first, or it would pass vacuously.
func newRawErrorBodiesClient(t *testing.T, srv *httptest.Server) *transport.Client {
	t.Helper()
	cat, _ := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://test.example.com",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	c, err := transport.New(cat,
		transport.WithHTTPClient(srv.Client()),
		transport.WithRawErrorBodies(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func readCassette(t *testing.T, name string) []byte {
	t.Helper()
	_, src, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(src), "..", "..", "..", "testkit", "cassettes", "its_rest", "query", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cassette %q: %v", path, err)
	}
	return b
}

func TestExecuteAdhoc(t *testing.T) {
	var captured *http.Request
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readCassette(t, "result_set.json"))
	}))
	defer srv.Close()

	rs, _, err := query.Execute(t.Context(), newClient(t, srv), aql.Query{
		Q: "SELECT e/ehr_id/value FROM EHR e",
		Parameters: map[string]any{
			"ehr_id": "7d44b88c-4199-4bad-97dc-d78268e01398",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodPost {
		t.Errorf("method = %q", captured.Method)
	}
	if captured.URL.Path != "/openehr/v1/query/aql" {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if body["q"] != "SELECT e/ehr_id/value FROM EHR e" {
		t.Errorf("q = %v", body["q"])
	}
	if len(rs.Rows) != 1 {
		t.Fatalf("rows = %d", len(rs.Rows))
	}
}

// TestExecuteWithEHRID pins REQ-055 (verb-aware EHR scoping) on the POST path: EHR
// scoping is emitted via the `openehr-ehr-id` request header (the spec's
// POST mechanism), NOT the `ehr_id` query parameter (the GET mechanism;
// not declared on the canonical POST operations).
func TestExecuteWithEHRID(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readCassette(t, "result_set.json"))
	}))
	defer srv.Close()

	ehrID := "7d44b88c-4199-4bad-97dc-d78268e01398"
	_, _, err := query.Execute(t.Context(), newClient(t, srv), aql.Query{
		Q:     "SELECT e/ehr_id/value FROM EHR e",
		EHRID: ehrID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodPost {
		t.Fatalf("method = %q, want POST", captured.Method)
	}
	if got := captured.Header.Get("openehr-ehr-id"); got != ehrID {
		t.Errorf("openehr-ehr-id header = %q, want %q", got, ehrID)
	}
	if got := captured.URL.Query().Get("ehr_id"); got != "" {
		t.Errorf("ehr_id query param leaked on POST = %q, want empty (REQ-055 — verb-aware EHR scoping)", got)
	}
}

// TestRunStoredWithEHRID mirrors TestExecuteWithEHRID for the stored-query
// POST path (REQ-055 — verb-aware EHR scoping).
func TestRunStoredWithEHRID(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readCassette(t, "result_set.json"))
	}))
	defer srv.Close()

	ehrID := "7d44b88c-4199-4bad-97dc-d78268e01398"
	_, _, err := query.RunStored(
		t.Context(), newClient(t, srv), "org.openehr::compositions", nil,
		query.WithEHRID(ehrID),
	)
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodPost {
		t.Fatalf("method = %q, want POST", captured.Method)
	}
	if got := captured.Header.Get("openehr-ehr-id"); got != ehrID {
		t.Errorf("openehr-ehr-id header = %q, want %q", got, ehrID)
	}
	if got := captured.URL.Query().Get("ehr_id"); got != "" {
		t.Errorf("ehr_id query param leaked on POST = %q, want empty (REQ-055 — verb-aware EHR scoping)", got)
	}
}

// TestExecuteGETWithEHRID pins REQ-055 (verb-aware EHR scoping) on the GET path: EHR
// scoping uses the `ehr_id` query parameter (declared on the canonical
// GET operations) and MUST NOT send the `openehr-ehr-id` header.
func TestExecuteGETWithEHRID(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readCassette(t, "result_set.json"))
	}))
	defer srv.Close()

	ehrID := "7d44b88c-4199-4bad-97dc-d78268e01398"
	_, _, err := query.ExecuteString(t.Context(), newClient(t, srv),
		"SELECT e/ehr_id/value FROM EHR e", nil,
		query.WithGET(), query.WithEHRID(ehrID))
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodGet {
		t.Fatalf("method = %q, want GET", captured.Method)
	}
	if got := captured.URL.Query().Get("ehr_id"); got != ehrID {
		t.Errorf("ehr_id query param = %q, want %q (GET keeps query-param scoping)", got, ehrID)
	}
	if got := captured.Header.Get("openehr-ehr-id"); got != "" {
		t.Errorf("openehr-ehr-id header leaked on GET = %q, want empty (REQ-055 — verb-aware EHR scoping)", got)
	}
}

func TestRunStored(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openehr/v1/query/org.openehr::compositions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readCassette(t, "result_set.json"))
	}))
	defer srv.Close()

	_, _, err := query.RunStored(t.Context(), newClient(t, srv), "org.openehr::compositions", map[string]any{
		"ehr_id": "7d44b88c-4199-4bad-97dc-d78268e01398",
	})
	if err != nil {
		t.Fatal(err)
	}
	// The stored Query schema requires offset + query_parameters; both must
	// be present even with no paging options. fetch is omitted (no spec
	// default — letting the server choose).
	if _, ok := body["offset"]; !ok {
		t.Errorf("stored body missing required offset: %v", body)
	}
	if _, ok := body["query_parameters"]; !ok {
		t.Errorf("stored body missing required query_parameters: %v", body)
	}
	if _, ok := body["fetch"]; ok {
		t.Errorf("stored body should omit fetch when unset, got %v", body["fetch"])
	}
}

func TestExecuteGET(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readCassette(t, "result_set.json"))
	}))
	defer srv.Close()

	_, _, err := query.ExecuteString(t.Context(), newClient(t, srv),
		"SELECT c FROM EHR e CONTAINS COMPOSITION c",
		map[string]any{"systolic_min": 120},
		query.WithGET(), query.WithFetch(10))
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodGet {
		t.Errorf("method = %q, want GET", captured.Method)
	}
	if captured.URL.Path != "/openehr/v1/query/aql" {
		t.Errorf("path = %q", captured.URL.Path)
	}
	q := captured.URL.Query()
	if q.Get("q") != "SELECT c FROM EHR e CONTAINS COMPOSITION c" {
		t.Errorf("q = %q", q.Get("q"))
	}
	if q.Get("fetch") != "10" {
		t.Errorf("fetch = %q, want 10", q.Get("fetch"))
	}
	if q.Get("systolic_min") != "120" {
		t.Errorf("systolic_min (form/explode query param) = %q, want 120", q.Get("systolic_min"))
	}
}

func TestExecuteGETEncodesScalarsLikeJSON(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readCassette(t, "result_set.json"))
	}))
	defer srv.Close()

	_, _, err := query.ExecuteString(t.Context(), newClient(t, srv),
		"SELECT c FROM EHR e",
		map[string]any{"big": float64(1234567), "flag": true},
		query.WithGET())
	if err != nil {
		t.Fatal(err)
	}
	q := captured.URL.Query()
	// Must match the POST/JSON rendering, NOT fmt.Sprint ("1.234567e+06").
	if q.Get("big") != "1234567" {
		t.Errorf("big = %q, want 1234567 (JSON-consistent, not scientific notation)", q.Get("big"))
	}
	if q.Get("flag") != "true" {
		t.Errorf("flag = %q, want true", q.Get("flag"))
	}
}

func TestRunStoredGET(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readCassette(t, "result_set.json"))
	}))
	defer srv.Close()

	_, _, err := query.RunStored(t.Context(), newClient(t, srv), "org.openehr::compositions",
		map[string]any{"ehr_status": "active"}, query.WithGET())
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodGet {
		t.Errorf("method = %q, want GET", captured.Method)
	}
	if captured.URL.Path != "/openehr/v1/query/org.openehr::compositions" {
		t.Errorf("path = %q", captured.URL.Path)
	}
	q := captured.URL.Query()
	// offset is always present (schema default 0); fetch omitted when unset.
	if q.Get("offset") != "0" {
		t.Errorf("offset = %q, want 0 (always present on stored GET)", q.Get("offset"))
	}
	if _, ok := q["fetch"]; ok {
		t.Errorf("fetch should be omitted when unset, got %q", q.Get("fetch"))
	}
	if q.Get("ehr_status") != "active" {
		t.Errorf("ehr_status param = %q, want active", q.Get("ehr_status"))
	}
}

func TestRunStoredPOSTExplicitZeroOffset(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readCassette(t, "result_set.json"))
	}))
	defer srv.Close()

	_, _, err := query.RunStored(t.Context(), newClient(t, srv), "org.openehr::compositions",
		nil, query.WithOffset(0), query.WithFetch(0))
	if err != nil {
		t.Fatal(err)
	}
	// Explicit zero must be representable (the *Set-flag fix), not dropped.
	if body["fetch"] != float64(0) {
		t.Errorf("fetch = %v, want explicit 0", body["fetch"])
	}
	if body["offset"] != float64(0) {
		t.Errorf("offset = %v, want explicit 0", body["offset"])
	}
}

// TestRunStoredRejectsReservedNameAQL pins REQ-057: the stored path is built
// as "/query/" + name, so the name "aql" would address the ad-hoc route
// /query/aql. The SDK refuses it before issuing any request, on both stored
// entry points and both verbs, and after trimming surrounding whitespace.
func TestRunStoredRejectsReservedNameAQL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server received %s %s; the reserved name must be refused before any request", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := newClient(t, srv)

	cases := []struct {
		label string
		call  func() error
	}{
		{"RunStored_POST", func() error {
			_, _, err := query.RunStored(t.Context(), c, "aql", nil)
			return err
		}},
		{"RunStored_GET", func() error {
			_, _, err := query.RunStored(t.Context(), c, "aql", nil, query.WithGET())
			return err
		}},
		{"RunStored_trimmed", func() error {
			_, _, err := query.RunStored(t.Context(), c, " aql ", nil)
			return err
		}},
		{"RunStoredVersion_POST", func() error {
			_, _, err := query.RunStoredVersion(t.Context(), c, "aql", "1.0.0", nil)
			return err
		}},
		{"RunStoredVersion_GET", func() error {
			_, _, err := query.RunStoredVersion(t.Context(), c, "aql", "1.0.0", nil, query.WithGET())
			return err
		}},
		{"RunStoredVersion_trimmed", func() error {
			_, _, err := query.RunStoredVersion(t.Context(), c, "\taql\n", "1.0.0", nil)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			err := tc.call()
			if !errors.Is(err, query.ErrInvalidConfig) {
				t.Fatalf("err = %v, want one wrapping ErrInvalidConfig", err)
			}
			// The diagnostic must name the collision, not leave the caller
			// to decode a server-side "missing q" 400.
			if !strings.Contains(err.Error(), "/query/aql") {
				t.Errorf("err = %q, want it to name the ad-hoc route /query/aql", err)
			}
		})
	}
}

// TestRunStoredReservedNameIsByteExact pins the other half of REQ-057's
// carve-out: only the exact byte sequence "aql" collides, so every other
// name — including case variants, namespaced forms, and names that merely
// contain it — still reaches the wire verbatim.
//
// The namespaced rows are the discriminator between the two sides of
// REQ-057: the store side refuses "ehr::aql" case-insensitively, while the
// execution path must not raise a client-side error for it. Unifying
// the store's reservedQueryName onto this path would fail here.
func TestRunStoredReservedNameIsByteExact(t *testing.T) {
	for _, name := range []string{"AQL", "Aql", "aql.reports", "org.example.aql", "aqlx", "ehr::aql", "ehr::AQL"} {
		t.Run(name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(readCassette(t, "result_set.json"))
			}))
			defer srv.Close()

			_, _, err := query.RunStored(t.Context(), newClient(t, srv), name, nil)
			if err != nil {
				t.Fatalf("RunStored(%q) = %v, want it to pass through", name, err)
			}
			if want := "/openehr/v1/query/" + name; gotPath != want {
				t.Errorf("path = %q, want %q (name passes through verbatim)", gotPath, want)
			}
		})
	}
}

// TestRunStoredVersionPathConstruction positively pins the stored-version
// route (REQ-057): the version is appended as its own path segment on both
// verbs, `/query/{qualified_query_name}/{version}`.
func TestRunStoredVersionPathConstruction(t *testing.T) {
	for _, tc := range []struct {
		label      string
		opt        []query.ExecuteOption
		wantMethod string
	}{
		{"POST", nil, http.MethodPost},
		{"GET", []query.ExecuteOption{query.WithGET()}, http.MethodGet},
	} {
		t.Run(tc.label, func(t *testing.T) {
			var captured *http.Request
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured = r.Clone(r.Context())
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(readCassette(t, "result_set.json"))
			}))
			defer srv.Close()

			_, _, err := query.RunStoredVersion(t.Context(), newClient(t, srv),
				"org.openehr::vitals", "1.2.3", nil, tc.opt...)
			if err != nil {
				t.Fatal(err)
			}
			if captured.Method != tc.wantMethod {
				t.Errorf("method = %q, want %q", captured.Method, tc.wantMethod)
			}
			if want := "/openehr/v1/query/org.openehr::vitals/1.2.3"; captured.URL.Path != want {
				t.Errorf("path = %q, want %q", captured.URL.Path, want)
			}
		})
	}
}

// TestRunStoredDiagnosticsNameTheOperation pins that the shared stored-path
// implementation reports the operation the caller actually invoked, not its
// sibling — the prefix is the caller's first breadcrumb.
func TestRunStoredDiagnosticsNameTheOperation(t *testing.T) {
	_, _, err := query.RunStoredVersion(t.Context(), nil, "   ", "1.0.0", nil)
	if err == nil || !strings.Contains(err.Error(), "query.RunStoredVersion:") {
		t.Errorf("RunStoredVersion err = %v, want a query.RunStoredVersion: prefix", err)
	}
	_, _, err = query.RunStored(t.Context(), nil, "   ", nil)
	if err == nil || !strings.Contains(err.Error(), "query.RunStored:") {
		t.Errorf("RunStored err = %v, want a query.RunStored: prefix", err)
	}
}

// TestRunStoredDiagnosticsNameTheOperationOnParamErrors extends the op-name
// pin past the empty-name arm: a failure raised further inside
// runStoredAtVersion — the GET query-parameter encoder — must still carry
// the caller's own operation name, not its sibling's (REQ-057). No server
// is involved: both arms return before a request is built.
func TestRunStoredDiagnosticsNameTheOperationOnParamErrors(t *testing.T) {
	for _, tc := range []struct {
		label  string
		params map[string]any
		want   string
	}{
		{"reserved_query_key", map[string]any{"fetch": 5}, "collides with reserved GET query key"},
		{"unencodable_value", map[string]any{"since": make(chan int)}, "query parameter \"since\""},
	} {
		t.Run(tc.label, func(t *testing.T) {
			_, _, err := query.RunStoredVersion(t.Context(), nil,
				"org.openehr::vitals", "1.2.3", tc.params, query.WithGET())
			if !errors.Is(err, query.ErrInvalidConfig) {
				t.Fatalf("err = %v, want one wrapping ErrInvalidConfig", err)
			}
			if !strings.Contains(err.Error(), "query.RunStoredVersion:") {
				t.Errorf("err = %q, want a query.RunStoredVersion: prefix", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %q, want it to name %s", err, tc.want)
			}
		})
	}
}

func TestExecuteGETExplicitZeroOffset(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readCassette(t, "result_set.json"))
	}))
	defer srv.Close()

	_, _, err := query.ExecuteString(t.Context(), newClient(t, srv),
		"SELECT c FROM EHR e", nil, query.WithGET(), query.WithOffset(0))
	if err != nil {
		t.Fatal(err)
	}
	// Explicit WithOffset(0) must reach the wire on the ad-hoc GET path.
	if got := captured.URL.Query().Get("offset"); got != "0" {
		t.Errorf("offset = %q, want explicit 0", got)
	}
}

func TestExecuteGETRejectsReservedParamName(t *testing.T) {
	_, _, err := query.ExecuteString(t.Context(), newClient(t, httptest.NewServer(nil)),
		"SELECT c FROM EHR e", map[string]any{"offset": 5}, query.WithGET())
	if !errors.Is(err, query.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for reserved-key collision, got %v", err)
	}
}

func TestExecuteEmptyQuery(t *testing.T) {
	_, _, err := query.Execute(t.Context(), newClient(t, httptest.NewServer(nil)), aql.Query{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestExecuteAQLError verifies that a server-side AQL error is surfaced as an
// *AQLError with the PHI-free error code present in its .Error() string even
// when the default (PHI-suppressed) client is used.
func TestExecuteAQLError(t *testing.T) {
	// Server returns a 400 with an openEHR error envelope containing PHI in
	// the message but a coded, non-PHI error code.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"patient 1234 not found","code":"VALIDATION_FAILED"}`))
	}))
	defer srv.Close()

	t.Run("default_client_suppresses_phi", func(t *testing.T) {
		// Default client: WithRawErrorBodies is false (PHI suppressed).
		c := newClient(t, srv)
		_, _, err := query.Execute(t.Context(), c, aql.Query{
			Q: "SELECT e/ehr_id/value FROM EHR e",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		aqlErr, ok := errors.AsType[*query.AQLError](err)
		if !ok {
			t.Fatalf("expected *query.AQLError, got %T: %v", err, err)
		}

		// Code must be preserved — it is non-PHI.
		if aqlErr.Code != "VALIDATION_FAILED" {
			t.Errorf("Code = %q, want %q", aqlErr.Code, "VALIDATION_FAILED")
		}
		// Message must be suppressed.
		if aqlErr.Message != "" {
			t.Errorf("Message = %q, want empty (PHI suppressed)", aqlErr.Message)
		}
		// Error() must include the code, not just the generic fallback.
		errStr := aqlErr.Error()
		if !strings.Contains(errStr, "VALIDATION_FAILED") {
			t.Errorf("Error() = %q, want it to contain %q", errStr, "VALIDATION_FAILED")
		}
		// Error() must not contain PHI.
		if strings.Contains(errStr, "1234") {
			t.Errorf("Error() = %q leaks PHI (contains %q)", errStr, "1234")
		}
	})

	t.Run("raw_error_bodies_preserves_message", func(t *testing.T) {
		// Build a client with WithRawErrorBodies(true): message must be visible.
		cat, _ := discovery.NewStaticCatalog(discovery.StaticConfig{
			Issuer: "https://test.example.com",
			Services: map[string]discovery.ServiceEntry{
				discovery.ServiceIDOpenEHRRest: {
					BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
					SpecVersion: discovery.SpecVersionPin,
				},
			},
		})
		c, err := transport.New(
			cat,
			transport.WithHTTPClient(srv.Client()),
			transport.WithRawErrorBodies(true),
		)
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = query.Execute(t.Context(), c, aql.Query{
			Q: "SELECT e/ehr_id/value FROM EHR e",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		aqlErr, ok := errors.AsType[*query.AQLError](err)
		if !ok {
			t.Fatalf("expected *query.AQLError, got %T: %v", err, err)
		}

		if aqlErr.Message != "patient 1234 not found" {
			t.Errorf("Message = %q, want %q", aqlErr.Message, "patient 1234 not found")
		}
		if aqlErr.Code != "VALIDATION_FAILED" {
			t.Errorf("Code = %q, want %q", aqlErr.Code, "VALIDATION_FAILED")
		}
		if !strings.Contains(aqlErr.Error(), "patient 1234 not found") {
			t.Errorf("Error() = %q, want it to contain the message", aqlErr.Error())
		}
	})
}

// TestExecutePathResolutionError verifies the PROBE-021 mapping: a backend AQL
// error whose envelope denotes path resolution is surfaced as an *AQLError that
// also satisfies errors.Is(err, aql.ErrPathResolution), so callers can branch
// without inspecting CDR-specific codes. A generic validation error must NOT.
func TestExecutePathResolutionError(t *testing.T) {
	cases := map[string]struct {
		body      string
		wantIsErr bool
	}{
		"path code":               {`{"code":"AQL_PATH_RESOLUTION","message":"x"}`, true},
		"path message":            {`{"code":"BAD_REQUEST","message":"could not resolve path /foo/bar"}`, true},
		"generic non-path":        {`{"code":"VALIDATION_FAILED","message":"bad request"}`, false},
		"url path code (not aql)": {`{"code":"INVALID_PATH_PARAMETER","message":"bad path param"}`, false},
		"resolve non-path msg":    {`{"code":"BAD_REQUEST","message":"could not resolve dependency"}`, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			// WithRawErrorBodies so the message-based classifier sees the text.
			cat, _ := discovery.NewStaticCatalog(discovery.StaticConfig{
				Issuer: "https://test.example.com",
				Services: map[string]discovery.ServiceEntry{
					discovery.ServiceIDOpenEHRRest: {
						BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
						SpecVersion: discovery.SpecVersionPin,
					},
				},
			})
			c, err := transport.New(
				cat,
				transport.WithHTTPClient(srv.Client()),
				transport.WithRawErrorBodies(true),
			)
			if err != nil {
				t.Fatal(err)
			}

			_, _, err = query.Execute(t.Context(), c, aql.NewQuery("SELECT e FROM EHR e"))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if _, ok := errors.AsType[*query.AQLError](err); !ok {
				t.Fatalf("expected *query.AQLError, got %T", err)
			}
			if got := errors.Is(err, aql.ErrPathResolution); got != tc.wantIsErr {
				t.Errorf("errors.Is(err, aql.ErrPathResolution) = %v, want %v", got, tc.wantIsErr)
			}
		})
	}
}

// TestExecutePathResolutionCodeOnly verifies classification works from the
// PHI-free error code alone — the default client suppresses the message, so a
// message-only signal ("could not resolve path…") must NOT classify, while the
// code (AQL_PATH_RESOLUTION) must.
func TestExecutePathResolutionCodeOnly(t *testing.T) {
	cases := map[string]struct {
		code   string
		wantIs bool
	}{
		"path code":    {"AQL_PATH_RESOLUTION", true},
		"generic code": {"VALIDATION_FAILED", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"code":"` + tc.code + `","message":"could not resolve path for patient 1234"}`))
			}))
			defer srv.Close()

			// Default client: message suppressed (PHI), so only the code is seen.
			_, _, err := query.Execute(t.Context(), newClient(t, srv), aql.NewQuery("SELECT e FROM EHR e"))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got := errors.Is(err, aql.ErrPathResolution); got != tc.wantIs {
				t.Errorf("errors.Is = %v, want %v", got, tc.wantIs)
			}
		})
	}
}

// TestExecuteEngineCapabilityError verifies the 501 arm of the PROBE-021
// mapping (REQ-055): a capability gap — valid AQL this deployment does not
// implement — surfaces as *AQLError satisfying
// errors.Is(err, aql.ErrEngineCapability). The HTTP status alone classifies it,
// with or without an openEHR error envelope, and a 501 is never also a
// path-resolution failure — that is bad AQL, and bad AQL arrives as 400.
// The 400/408 rows pin the pre-existing mapping unchanged.
func TestExecuteEngineCapabilityError(t *testing.T) {
	const pathEnvelope = `{"code":"AQL_PATH_RESOLUTION","message":"could not resolve path /foo"}`

	cases := map[string]struct {
		status         int
		body           string
		wantCapability bool
		wantPathRes    bool
		wantCode       string
	}{
		"501 with envelope":             {http.StatusNotImplemented, `{"code":"NOT_IMPLEMENTED","message":"AQL feature unsupported"}`, true, false, "NOT_IMPLEMENTED"},
		"501 bare":                      {http.StatusNotImplemented, "", true, false, ""},
		"501 with path-shaped envelope": {http.StatusNotImplemented, pathEnvelope, true, false, "AQL_PATH_RESOLUTION"},
		"400 with path envelope":        {http.StatusBadRequest, pathEnvelope, false, true, "AQL_PATH_RESOLUTION"},
		"400 bare":                      {http.StatusBadRequest, "", false, false, ""},
		"408 bare":                      {http.StatusRequestTimeout, "", false, false, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.body != "" {
					w.Header().Set("Content-Type", "application/json")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			_, _, err := query.Execute(t.Context(), newClient(t, srv), aql.NewQuery("SELECT e FROM EHR e"))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			aqlErr, ok := errors.AsType[*query.AQLError](err)
			if !ok {
				t.Fatalf("expected *query.AQLError, got %T", err)
			}
			if aqlErr.Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", aqlErr.Code, tc.wantCode)
			}
			if got := errors.Is(err, aql.ErrEngineCapability); got != tc.wantCapability {
				t.Errorf("errors.Is(err, aql.ErrEngineCapability) = %v, want %v", got, tc.wantCapability)
			}
			if got := errors.Is(err, aql.ErrPathResolution); got != tc.wantPathRes {
				t.Errorf("errors.Is(err, aql.ErrPathResolution) = %v, want %v", got, tc.wantPathRes)
			}
			// The wire error stays reachable underneath the classification.
			we, ok := errors.AsType[*transport.WireError](err)
			if !ok {
				t.Fatalf("expected the wrapped *transport.WireError, got %T", errors.Unwrap(err))
			}
			if we.StatusCode != tc.status {
				t.Errorf("WireError.StatusCode = %d, want %d", we.StatusCode, tc.status)
			}
		})
	}
}

// TestEngineCapabilityIsNeverInferredFromMessageText is the inverse of
// TestExecuteEngineCapabilityError, and pins the "never from message text" half
// of REQ-055 (wire.md § AQL executor): the HTTP 501 status ALONE classifies a
// capability gap. A 400 or 408 is a client error however the envelope words
// itself, so wording like "not implemented" / "unsupported" / "NOT_IMPLEMENTED"
// MUST NOT make it satisfy errors.Is(err, aql.ErrEngineCapability) — while it
// still maps to *query.AQLError exactly as before. The rule holds today because
// the classifier is status-only; nothing pinned it, so a future "helpful" text
// match would land silently.
//
// The client surfaces raw error bodies so the wording genuinely reaches the
// mapper rather than being PHI-suppressed on the way in, and each row asserts
// the trigger wording is present in the mapped error — a negative result on a
// message the mapper never saw would prove nothing. The 501 row is the control:
// same wording, and it MUST classify, so the table cannot pass by an
// errors.Is that has stopped matching anything at all.
func TestEngineCapabilityIsNeverInferredFromMessageText(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		// trigger is the capability-flavoured wording that must be visible in
		// the mapped error, lower-cased for the containment check.
		trigger        string
		wantCapability bool
	}{
		"400 code says NOT_IMPLEMENTED": {
			http.StatusBadRequest,
			`{"code":"NOT_IMPLEMENTED","message":"bad AQL"}`,
			"not_implemented", false,
		},
		"400 message says not implemented": {
			http.StatusBadRequest,
			`{"code":"BAD_REQUEST","message":"this AQL function is not implemented"}`,
			"not implemented", false,
		},
		"400 message says unsupported": {
			http.StatusBadRequest,
			`{"code":"VALIDATION_FAILED","message":"unsupported AQL feature"}`,
			"unsupported", false,
		},
		"408 message says not implemented": {
			http.StatusRequestTimeout,
			`{"code":"TIMEOUT","message":"not implemented on this engine"}`,
			"not implemented", false,
		},
		"501 with the same wording (control)": {
			http.StatusNotImplemented,
			`{"code":"NOT_IMPLEMENTED","message":"this AQL function is not implemented"}`,
			"not implemented", true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			_, _, err := query.Execute(t.Context(), newRawErrorBodiesClient(t, srv),
				aql.NewQuery("SELECT e FROM EHR e"))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			aqlErr, ok := errors.AsType[*query.AQLError](err)
			if !ok {
				t.Fatalf("expected *query.AQLError, got %T: %v", err, err)
			}
			// The wording has to have reached the mapper, or the negative
			// assertion below is vacuous.
			seen := strings.ToLower(aqlErr.Message + " " + aqlErr.Code)
			if !strings.Contains(seen, tc.trigger) {
				t.Fatalf("premise gone: mapped error carries %q, want it to contain %q — the classifier never saw the wording", seen, tc.trigger)
			}
			if got := errors.Is(err, aql.ErrEngineCapability); got != tc.wantCapability {
				t.Errorf("errors.Is(err, aql.ErrEngineCapability) = %v, want %v for HTTP %d — only the status classifies a capability gap, never the message text",
					got, tc.wantCapability, tc.status)
			}
		})
	}
}

// TestStoredQueryEngineCapabilityError extends the REQ-055 501 mapping to the
// stored-query entry points. RunStored and RunStoredVersion reach mapQueryError
// through the same doResultSet path as Execute, so the classification ought to
// hold there — but only the ad-hoc route was covered, and a later divergence on
// the stored path (its own decode step, an extra wrapper) would go unnoticed.
//
// Both entry points and both verbs are covered because each is a one-line call,
// mirroring TestRunStoredRejectsReservedNameAQL. The envelope/bare split is not
// repeated here: that is a property of mapQueryError itself and is already
// pinned by TestExecuteEngineCapabilityError. What is new is that the stored
// path reaches mapQueryError at all.
func TestStoredQueryEngineCapabilityError(t *testing.T) {
	const body = `{"code":"NOT_IMPLEMENTED","message":"stored query uses an AQL feature this engine lacks"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	c := newClient(t, srv)

	cases := []struct {
		label string
		call  func() error
	}{
		{"RunStored_POST", func() error {
			_, _, err := query.RunStored(t.Context(), c, "org.openehr::compositions", nil)
			return err
		}},
		{"RunStored_GET", func() error {
			_, _, err := query.RunStored(t.Context(), c, "org.openehr::compositions", nil, query.WithGET())
			return err
		}},
		{"RunStoredVersion_POST", func() error {
			_, _, err := query.RunStoredVersion(t.Context(), c, "org.openehr::compositions", "1.0.0", nil)
			return err
		}},
		{"RunStoredVersion_GET", func() error {
			_, _, err := query.RunStoredVersion(t.Context(), c, "org.openehr::compositions", "1.0.0", nil, query.WithGET())
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			aqlErr, ok := errors.AsType[*query.AQLError](err)
			if !ok {
				t.Fatalf("expected *query.AQLError, got %T: %v", err, err)
			}
			if !errors.Is(err, aql.ErrEngineCapability) {
				t.Errorf("errors.Is(err, aql.ErrEngineCapability) = false, want true — a 501 is a capability gap on the stored path too")
			}
			if errors.Is(err, aql.ErrPathResolution) {
				t.Error("errors.Is(err, aql.ErrPathResolution) = true, want false — the two classes are disjoint (REQ-055)")
			}
			// The PHI-free code is carried through; the message is suppressed
			// by the default client.
			if aqlErr.Code != "NOT_IMPLEMENTED" {
				t.Errorf("Code = %q, want %q", aqlErr.Code, "NOT_IMPLEMENTED")
			}
			we, ok := errors.AsType[*transport.WireError](err)
			if !ok {
				t.Fatalf("expected the wrapped *transport.WireError, got %T", errors.Unwrap(err))
			}
			if we.StatusCode != http.StatusNotImplemented {
				t.Errorf("WireError.StatusCode = %d, want %d", we.StatusCode, http.StatusNotImplemented)
			}
		})
	}
}

// TestExecuteBuiltQueryEnvelope verifies a query produced by the aql builder
// reaches the wire body intact: the canonical AQL string, envelope paging
// (Offset/Fetch), and bound parameters.
func TestExecuteBuiltQueryEnvelope(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readCassette(t, "result_set.json"))
	}))
	defer srv.Close()

	built, err := aql.NewBuilder().
		Select(aql.Col("o")).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Archetype("OBSERVATION", "o", "openEHR-EHR-OBSERVATION.body_temperature.v2")).
		Bind("ehr_id", "7d44b88c-4199-4bad-97dc-d78268e01398").
		Offset(10).
		Limit(20).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := query.Execute(t.Context(), newClient(t, srv), built); err != nil {
		t.Fatal(err)
	}
	if body["q"] != built.String() {
		t.Errorf("q = %v, want %q", body["q"], built.String())
	}
	// JSON numbers decode to float64.
	if body["offset"] != float64(10) {
		t.Errorf("offset = %v, want 10", body["offset"])
	}
	if body["fetch"] != float64(20) {
		t.Errorf("fetch = %v, want 20", body["fetch"])
	}
	qp, ok := body["query_parameters"].(map[string]any)
	if !ok || qp["ehr_id"] != "7d44b88c-4199-4bad-97dc-d78268e01398" {
		t.Errorf("query_parameters = %v", body["query_parameters"])
	}
}
