package composition_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const (
	ehrIDFixture    openehrclient.EHRID             = "bf0b2ad8-7b0e-4f4d-9d33-6a8de69f0a64"
	compositionVOID openehrclient.VersionedObjectID = "1234abcd-5678-9012-3456-7890abcdef00"
	compositionVUID openehrclient.VersionUID        = "1234abcd-5678-9012-3456-7890abcdef00::cdr.example::1"
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

// readCompositionCassette returns one of the vendored canonical-JSON
// composition cassettes from testkit/cassettes/canonical_json/.
// Composition reads exercise full RM round-trip through canjson, so
// the same fixtures used by PROBE-030 cover Phase 3 read paths.
func readCompositionCassette(t *testing.T) []byte {
	t.Helper()
	_, src, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(src), "..", "..", "..", "..", "testkit", "cassettes", "canonical_json", "body_weight.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cassette %q: %v", path, err)
	}
	return b
}

func TestGetLatest(t *testing.T) {
	var captured *http.Request
	body := readCompositionCassette(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("ETag", `"`+string(compositionVUID)+`"`)
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(compositionVUID))
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	got, meta, err := composition.Get(context.Background(), newClient(t, srv), ehrIDFixture, openehrclient.LatestOf(compositionVOID))
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("nil Composition")
	}
	if captured.URL.Path != "/openehr/v1/ehr/"+string(ehrIDFixture)+"/composition/"+string(compositionVOID) {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if meta.VersionUID != compositionVUID {
		t.Errorf("VersionUID (from Location) = %q", meta.VersionUID)
	}
	if meta.ETag != string(compositionVUID) {
		t.Errorf("ETag = %q", meta.ETag)
	}
}

func TestGetSpecificVersion(t *testing.T) {
	var captured *http.Request
	body := readCompositionCassette(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	_, _, err := composition.Get(context.Background(), newClient(t, srv), ehrIDFixture, openehrclient.VersionOf(compositionVUID))
	if err != nil {
		t.Fatal(err)
	}
	if captured.URL.Path != "/openehr/v1/ehr/"+string(ehrIDFixture)+"/composition/"+string(compositionVUID) {
		t.Errorf("path = %q", captured.URL.Path)
	}
}

func TestGetAtTime(t *testing.T) {
	var captured *http.Request
	body := readCompositionCassette(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	at, _ := time.Parse(time.RFC3339, "2026-05-17T08:00:00Z")
	_, _, err := composition.Get(context.Background(), newClient(t, srv), ehrIDFixture, openehrclient.LatestAtTime(compositionVOID, at))
	if err != nil {
		t.Fatal(err)
	}
	if got := captured.URL.Query().Get("version_at_time"); got != "2026-05-17T08:00:00Z" {
		t.Errorf("version_at_time = %q", got)
	}
}

func TestGetRejectsNilRef(t *testing.T) {
	_, _, err := composition.Get(context.Background(), nil, ehrIDFixture, nil)
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestGetRejectsEmptyEHRID(t *testing.T) {
	_, _, err := composition.Get(context.Background(), nil, "", openehrclient.LatestOf(compositionVOID))
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestGetSurfacesNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"message":"not found","code":"NOT_FOUND"}`))
	}))
	defer srv.Close()
	_, _, err := composition.Get(context.Background(), newClient(t, srv), ehrIDFixture, openehrclient.LatestOf(compositionVOID))
	if !errors.Is(err, transport.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRepository(t *testing.T) {
	body := readCompositionCassette(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	repo := composition.NewRepository(newClient(t, srv))
	if _, _, err := repo.Get(context.Background(), ehrIDFixture, openehrclient.LatestOf(compositionVOID)); err != nil {
		t.Fatal(err)
	}
}
