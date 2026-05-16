package directory_test

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
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/directory"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const (
	ehrIDFixture openehrclient.EHRID      = "bf0b2ad8-7b0e-4f4d-9d33-6a8de69f0a64"
	folderVUID   openehrclient.VersionUID = "0a1b2c3d-4e5f-6789-abcd-ef0123456789::cdr.example::1"
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

func readCassette(t *testing.T) []byte {
	t.Helper()
	_, src, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(src), "..", "..", "..", "..", "testkit", "cassettes", "its_rest", "ehr", "folder.json")
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
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/directory/"+string(folderVUID))
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	got, meta, err := directory.Get(context.Background(), newClient(t, srv), ehrIDFixture)
	if err != nil {
		t.Fatal(err)
	}
	if captured.URL.Path != "/openehr/v1/ehr/"+string(ehrIDFixture)+"/directory" {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if got.Name.Value != "Root Directory" {
		t.Errorf("Name.Value = %q", got.Name.Value)
	}
	if len(got.Folders) != 2 {
		t.Errorf("Folders count = %d, want 2", len(got.Folders))
	}
	if meta.VersionUID != folderVUID {
		t.Errorf("VersionUID = %q", meta.VersionUID)
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
	_, _, err := directory.GetAtTime(context.Background(), newClient(t, srv), ehrIDFixture, at)
	if err != nil {
		t.Fatal(err)
	}
	if got := captured.URL.Query().Get("version_at_time"); got != "2026-05-17T08:00:00Z" {
		t.Errorf("version_at_time = %q", got)
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

	_, _, err := directory.GetVersioned(context.Background(), newClient(t, srv), ehrIDFixture, folderVUID)
	if err != nil {
		t.Fatal(err)
	}
	if captured.URL.Path != "/openehr/v1/ehr/"+string(ehrIDFixture)+"/directory/"+string(folderVUID) {
		t.Errorf("path = %q", captured.URL.Path)
	}
}

func TestGetRejectsEmptyVersionUID(t *testing.T) {
	_, _, err := directory.GetVersioned(context.Background(), nil, ehrIDFixture, "")
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestRepository(t *testing.T) {
	body := readCassette(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	repo := directory.NewRepository(newClient(t, srv))
	if _, _, err := repo.Get(context.Background(), ehrIDFixture); err != nil {
		t.Fatal(err)
	}
}

func TestSaveDirectory(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("ETag", `"`+string(folderVUID)+`"`)
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/directory/"+string(folderVUID))
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	folder := &rm.Folder{
		Name:            rm.DVText{Value: "Root Directory"},
		ArchetypeNodeID: "openEHR-EHR-FOLDER.generic.v1",
	}
	_, meta, err := directory.Save(context.Background(), newClient(t, srv), ehrIDFixture, folder)
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodPost {
		t.Errorf("method = %q", captured.Method)
	}
	if captured.URL.Path != "/openehr/v1/ehr/"+string(ehrIDFixture)+"/directory" {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if meta.VersionUID != folderVUID {
		t.Errorf("VersionUID = %q", meta.VersionUID)
	}
}

func TestUpdateDirectoryRequiresIfMatch(t *testing.T) {
	_, _, err := directory.Update(context.Background(), nil, ehrIDFixture, "", &rm.Folder{})
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestUpdateDirectory(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	folder := &rm.Folder{
		Name:            rm.DVText{Value: "Root"},
		ArchetypeNodeID: "openEHR-EHR-FOLDER.generic.v1",
	}
	if _, _, err := directory.Update(context.Background(), newClient(t, srv), ehrIDFixture, string(folderVUID), folder); err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodPut {
		t.Errorf("method = %q", captured.Method)
	}
	if got := captured.Header.Get("If-Match"); got != `"`+string(folderVUID)+`"` {
		t.Errorf("If-Match = %q", got)
	}
}

func TestDeleteDirectory(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	if _, err := directory.Delete(context.Background(), newClient(t, srv), ehrIDFixture, string(folderVUID)); err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodDelete {
		t.Errorf("method = %q", captured.Method)
	}
	if got := captured.Header.Get("If-Match"); got != `"`+string(folderVUID)+`"` {
		t.Errorf("If-Match = %q", got)
	}
}
