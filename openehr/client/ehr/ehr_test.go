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

// TestGetEmpty2xxBodyStaysErrInvalidShape is the negative pin for "reads are
// untouched" (REQ-094's Create closure MUST NOT reach transport.Decode's
// shared read arm): an empty 2xx body on a read stays the bare
// transport.ErrInvalidShape it always was, never *NoRepresentationError.
func TestGetEmpty2xxBodyStaysErrInvalidShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	_, _, err := openehrclient.Get(t.Context(), newClient(t, srv), ehrIDFixture)
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Fatalf("err = %v, want transport.ErrInvalidShape", err)
	}
	if _, ok := errors.AsType[*openehrclient.NoRepresentationError](err); ok {
		t.Error("ehr.Get must not reclassify an empty body as NoRepresentationError")
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

// TestCreateEmpty2xxBody characterises the four response-body shapes
// Create's 2xx arm can receive (REQ-094). An empty or JSON-null body
// commits but carries no representation and MUST surface as
// *NoRepresentationError, carrying the commit metadata; a present but
// undecodable body keeps REQ-151's *transport.DecodeError typing
// unchanged; a valid body decodes normally. ehr.Get/ehr.Exists are
// proven unchanged separately (TestGet/TestExists*).
func TestCreateEmpty2xxBody(t *testing.T) {
	t.Run("empty_body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "/ehr/"+ehrIDFixture)
			w.WriteHeader(http.StatusCreated)
		}))
		defer srv.Close()
		out, meta, err := openehrclient.Create(t.Context(), newClient(t, srv))
		nre, ok := errors.AsType[*openehrclient.NoRepresentationError](err)
		if !ok {
			t.Fatalf("empty body: err = %v, want *NoRepresentationError", err)
		}
		if !errors.Is(err, transport.ErrInvalidShape) {
			t.Errorf("empty body: err must wrap transport.ErrInvalidShape, got %v", err)
		}
		if out != nil {
			t.Errorf("empty body: out = %+v, want nil", out)
		}
		if meta == nil || nre.Meta == nil {
			t.Fatal("empty body: commit metadata must survive the failed representation")
		}
	})

	t.Run("null_body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "/ehr/"+ehrIDFixture)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("null"))
		}))
		defer srv.Close()
		out, meta, err := openehrclient.Create(t.Context(), newClient(t, srv))
		nre, ok := errors.AsType[*openehrclient.NoRepresentationError](err)
		if !ok {
			t.Fatalf("null body: err = %v, want *NoRepresentationError", err)
		}
		if !errors.Is(err, transport.ErrInvalidShape) {
			t.Errorf("null body: err must wrap transport.ErrInvalidShape, got %v", err)
		}
		if out != nil {
			t.Errorf("null body: out = %+v, want nil", out)
		}
		if meta == nil || nre.Meta == nil {
			t.Fatal("null body: commit metadata must survive the failed representation")
		}
	})

	// REQ-094: a whitespace-only body classifies as empty (ErrInvalidShape),
	// not as a decode failure -- the TrimSpace guard is load-bearing on its
	// own, independent of the JSON null arm (write_test.go pins the same
	// case for WriteResult).
	t.Run("whitespace_body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "/ehr/"+ehrIDFixture)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(" \n\t"))
		}))
		defer srv.Close()
		out, meta, err := openehrclient.Create(t.Context(), newClient(t, srv))
		nre, ok := errors.AsType[*openehrclient.NoRepresentationError](err)
		if !ok {
			t.Fatalf("whitespace body: err = %v, want *NoRepresentationError", err)
		}
		if !errors.Is(err, transport.ErrInvalidShape) {
			t.Errorf("whitespace body: err must wrap transport.ErrInvalidShape, got %v", err)
		}
		if out != nil {
			t.Errorf("whitespace body: out = %+v, want nil", out)
		}
		if meta == nil || nre.Meta == nil {
			t.Fatal("whitespace body: commit metadata must survive the failed representation")
		}
	})

	t.Run("garbage_body", func(t *testing.T) {
		const garbage = `}{ not json`
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(garbage))
		}))
		defer srv.Close()
		_, _, err := openehrclient.Create(t.Context(), newClient(t, srv))
		de, ok := errors.AsType[*transport.DecodeError](err)
		if !ok {
			t.Fatalf("garbage body: err = %v, want *transport.DecodeError (REQ-151, unchanged)", err)
		}
		// Pins the transport.Decode bypass: Create builds *transport.DecodeError
		// by hand (it no longer calls transport.Decode -- see the null-body
		// note above), so its payload must match what transport.Decode itself
		// would have produced, not merely the type.
		if string(de.Body) != garbage {
			t.Errorf("garbage body: de.Body = %q, want %q", de.Body, garbage)
		}
		if de.Method != http.MethodPost {
			t.Errorf("garbage body: de.Method = %q, want %q", de.Method, http.MethodPost)
		}
		if de.Route != "/ehr" {
			t.Errorf("garbage body: de.Route = %q, want %q", de.Route, "/ehr")
		}
		if de.Unwrap() == nil {
			t.Error("garbage body: de.Unwrap() = nil, want the canjson decode cause")
		}
		if _, ok := errors.AsType[*openehrclient.NoRepresentationError](err); ok {
			t.Error("garbage body: decode-failure arm must not be reclassified as NoRepresentationError")
		}
	})

	t.Run("valid_body", func(t *testing.T) {
		body := readCassette(t, "ehr", "ehr.json")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(body)
		}))
		defer srv.Close()
		out, _, err := openehrclient.Create(t.Context(), newClient(t, srv))
		if err != nil {
			t.Fatalf("valid body: unexpected err = %v", err)
		}
		if out == nil || out.EHRID.Value != ehrIDFixture {
			t.Errorf("valid body: out = %+v, want EHRID.Value = %q", out, ehrIDFixture)
		}
	})
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

// TestNilClientLeavesDoNotPanic pins the consequence of the REQ-025
// guard in transport.Do: every exported leaf takes a *transport.Client
// and calls c.Do, so a nil client used to crash inside the transport.
// Each leaf must now surface ErrInvalidConfig instead. These are the
// EHR-package entry points; the guard is central, so one arm per
// distinct call shape is enough to hold it.
func TestNilClientLeavesDoNotPanic(t *testing.T) {
	const id openehrclient.EHRID = "8849182c-82ad-4088-a07f-48ead4180515"
	cases := []struct {
		name string
		call func() error
	}{
		{"Exists", func() error { _, err := openehrclient.Exists(t.Context(), nil, id); return err }},
		{"Get", func() error { _, _, err := openehrclient.Get(t.Context(), nil, id); return err }},
		{"Create", func() error { _, _, err := openehrclient.Create(t.Context(), nil); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s with a nil client panicked: %v", tc.name, r)
				}
			}()
			if err := tc.call(); !errors.Is(err, transport.ErrInvalidConfig) {
				t.Errorf("err = %v; want ErrInvalidConfig", err)
			}
		})
	}
}
