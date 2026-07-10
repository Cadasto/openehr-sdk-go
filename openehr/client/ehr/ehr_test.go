package ehr_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// newClient is a convenience for tests that target srv as the
// openEHR REST base.
func newClient(t *testing.T, srv *httptest.Server) *transport.Client {
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
	c, err := transport.New(cat, transport.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// readCassette returns the bytes of a cassette under
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

const ehrIDFixture = "bf0b2ad8-7b0e-4f4d-9d33-6a8de69f0a64"

func TestGet(t *testing.T) {
	var captured *http.Request
	body := readCassette(t, "ehr", "ehr.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("ETag", `"`+ehrIDFixture+`"`)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	got, meta, err := openehrclient.Get(t.Context(), newClient(t, srv), ehrIDFixture)
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodGet {
		t.Errorf("method = %q", captured.Method)
	}
	if captured.URL.Path != "/openehr/v1/ehr/"+ehrIDFixture {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if got.EHRID.Value != ehrIDFixture {
		t.Errorf("EHRID.Value = %q, want %q", got.EHRID.Value, ehrIDFixture)
	}
	if got.SystemID.Value != "cdr.example" {
		t.Errorf("SystemID.Value = %q", got.SystemID.Value)
	}
	if meta == nil || meta.ETag != ehrIDFixture {
		t.Errorf("ETag captured = %+v", meta)
	}
}

func TestGetRejectsEmptyID(t *testing.T) {
	_, _, err := openehrclient.Get(t.Context(), nil, "")
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestExistsTrue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method = %q, want HEAD", r.Method)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	exists, err := openehrclient.Exists(t.Context(), newClient(t, srv), ehrIDFixture)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("expected exists=true on 200")
	}
}

func TestExistsFalseOn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()
	exists, err := openehrclient.Exists(t.Context(), newClient(t, srv), ehrIDFixture)
	if err != nil {
		t.Errorf("404 must fold into exists=false, got err: %v", err)
	}
	if exists {
		t.Error("expected exists=false on 404")
	}
}

func TestExistsBubblesNon404Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	}))
	defer srv.Close()
	exists, err := openehrclient.Exists(t.Context(), newClient(t, srv), ehrIDFixture)
	if !errors.Is(err, transport.ErrForbidden) {
		t.Errorf("expected ErrForbidden bubbled, got %v", err)
	}
	if exists {
		t.Error("expected exists=false on error")
	}
}

func TestGetBySubject(t *testing.T) {
	var captured *http.Request
	body := readCassette(t, "ehr", "ehr.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	got, _, err := openehrclient.GetBySubject(t.Context(), newClient(t, srv), "demographic", "patient-123")
	if err != nil {
		t.Fatal(err)
	}
	if captured.URL.Path != "/openehr/v1/ehr" {
		t.Errorf("path = %q", captured.URL.Path)
	}
	q := captured.URL.Query()
	if q.Get("subject_id") != "patient-123" || q.Get("subject_namespace") != "demographic" {
		t.Errorf("query = %v", q)
	}
	if got.EHRID.Value != ehrIDFixture {
		t.Errorf("EHRID.Value = %q", got.EHRID.Value)
	}
}

func TestVersionUIDParsing(t *testing.T) {
	v := openehrclient.VersionUID("aaa::cdr.example::3")
	if got := v.VersionedObjectID(); got != "aaa" {
		t.Errorf("VersionedObjectID = %q", got)
	}
	if got := v.CreatingSystemID(); got != "cdr.example" {
		t.Errorf("CreatingSystemID = %q", got)
	}
	if got := v.VersionNumber(); got != "3" {
		t.Errorf("VersionNumber = %q", got)
	}
	// Malformed.
	v2 := openehrclient.VersionUID("not-a-version")
	if v2.VersionedObjectID() != "" || v2.CreatingSystemID() != "" || v2.VersionNumber() != "" {
		t.Error("malformed VersionUID should return empty segments")
	}
}

func TestCreateServerAssigned(t *testing.T) {
	var captured *http.Request
	body := readCassette(t, "ehr", "ehr.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("Location", "/ehr/"+ehrIDFixture)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	got, meta, err := openehrclient.Create(t.Context(), newClient(t, srv))
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", captured.Method)
	}
	if captured.URL.Path != "/openehr/v1/ehr" {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if got.EHRID.Value != ehrIDFixture {
		t.Errorf("EHRID.Value = %q", got.EHRID.Value)
	}
	if meta == nil || meta.Location == "" {
		t.Errorf("expected Location captured, got %+v", meta)
	}
}

func TestCreateClientSupplied(t *testing.T) {
	var captured *http.Request
	body := readCassette(t, "ehr", "ehr.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	_, _, err := openehrclient.Create(t.Context(), newClient(t, srv),
		openehrclient.WithEHRID(ehrIDFixture),
	)
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodPut {
		t.Errorf("method = %q, want PUT", captured.Method)
	}
	if captured.URL.Path != "/openehr/v1/ehr/"+ehrIDFixture {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if got := captured.Header.Get("Prefer"); got != "return=representation" {
		t.Errorf("Prefer = %q, want return=representation (Create default)", got)
	}
}

func TestRefConstruction(t *testing.T) {
	if r := openehrclient.LatestOf("voID"); r.PathSegment() != "voID" {
		t.Errorf("LatestOf PathSegment = %q", r.PathSegment())
	}
	if r := openehrclient.VersionOf("uid::s::1"); r.PathSegment() != "uid::s::1" {
		t.Errorf("VersionOf PathSegment = %q", r.PathSegment())
	}
	at, err := time.Parse(time.RFC3339, "2026-05-17T10:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	r := openehrclient.LatestAtTime("voID", at)
	k, v := r.Query()
	if k != "version_at_time" {
		t.Errorf("LatestAtTime Query key = %q, want version_at_time", k)
	}
	if v != "2026-05-17T10:00:00Z" {
		t.Errorf("LatestAtTime Query value = %q", v)
	}
}
