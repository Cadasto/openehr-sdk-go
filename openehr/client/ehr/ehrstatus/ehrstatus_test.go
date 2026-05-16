package ehrstatus_test

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
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/ehrstatus"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const ehrIDFixture openehrclient.EHRID = "bf0b2ad8-7b0e-4f4d-9d33-6a8de69f0a64"
const ehrStatusUID openehrclient.VersionUID = "f1e2d3c4-b5a6-4978-89ab-cdef01234567::cdr.example::1"

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

func readCassette(t *testing.T) []byte {
	t.Helper()
	_, src, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(src), "..", "..", "..", "..", "testkit", "cassettes", "its_rest", "ehr", "ehr_status.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cassette %q: %v", path, err)
	}
	return b
}

func TestGet(t *testing.T) {
	var captured *http.Request
	body := readCassette(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("ETag", `"`+string(ehrStatusUID)+`"`)
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/ehr_status/"+string(ehrStatusUID))
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	got, meta, err := ehrstatus.Get(context.Background(), newClient(t, srv), ehrIDFixture)
	if err != nil {
		t.Fatal(err)
	}
	if captured.URL.Path != "/openehr/v1/ehr/"+string(ehrIDFixture)+"/ehr_status" {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if !got.IsQueryable || !got.IsModifiable {
		t.Errorf("expected queryable+modifiable, got %+v", got)
	}
	if meta.ETag != string(ehrStatusUID) {
		t.Errorf("ETag = %q", meta.ETag)
	}
	if meta.VersionUID != ehrStatusUID {
		t.Errorf("VersionUID (from Location) = %q, want %q", meta.VersionUID, ehrStatusUID)
	}
}

func TestGetAtTime(t *testing.T) {
	var captured *http.Request
	body := readCassette(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	at, _ := time.Parse(time.RFC3339, "2026-05-17T08:00:00Z")
	_, _, err := ehrstatus.GetAtTime(context.Background(), newClient(t, srv), ehrIDFixture, at)
	if err != nil {
		t.Fatal(err)
	}
	if got := captured.URL.Query().Get("version_at_time"); got != "2026-05-17T08:00:00Z" {
		t.Errorf("version_at_time = %q", got)
	}
}

func TestGetAtTimeRejectsZero(t *testing.T) {
	_, _, err := ehrstatus.GetAtTime(context.Background(), nil, ehrIDFixture, time.Time{})
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig on zero time, got %v", err)
	}
}

func TestGetVersioned(t *testing.T) {
	var captured *http.Request
	body := readCassette(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	_, _, err := ehrstatus.GetVersioned(context.Background(), newClient(t, srv), ehrIDFixture, ehrStatusUID)
	if err != nil {
		t.Fatal(err)
	}
	if captured.URL.Path != "/openehr/v1/ehr/"+string(ehrIDFixture)+"/ehr_status/"+string(ehrStatusUID) {
		t.Errorf("path = %q", captured.URL.Path)
	}
}

func TestErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"message":"not found","code":"NOT_FOUND"}`))
	}))
	defer srv.Close()
	_, _, err := ehrstatus.Get(context.Background(), newClient(t, srv), ehrIDFixture)
	if !errors.Is(err, transport.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRepository(t *testing.T) {
	body := readCassette(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	repo := ehrstatus.NewRepository(newClient(t, srv))
	if _, _, err := repo.Get(context.Background(), ehrIDFixture); err != nil {
		t.Fatal(err)
	}
}
