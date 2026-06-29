package admin_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/admin"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

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

func TestDeleteEHRHappyPath(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := newClient(t, srv)

	if err := admin.DeleteEHR(t.Context(), c, ehr.EHRID("abc-123")); err != nil {
		t.Fatalf("DeleteEHR: %v", err)
	}
	if gotMethod != "DELETE" {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/openehr/v1/admin/ehr/abc-123" {
		t.Errorf("path = %q, want /openehr/v1/admin/ehr/abc-123", gotPath)
	}
}

func TestDeleteEHRMissingEHRID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be called for empty EHRID")
	}))
	defer srv.Close()
	c := newClient(t, srv)
	err := admin.DeleteEHR(t.Context(), c, "")
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("err = %v, want wrap of ErrInvalidConfig", err)
	}
}

func TestDeleteEHRSurfaces404AsErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := newClient(t, srv)

	err := admin.DeleteEHR(t.Context(), c, ehr.EHRID("ghost"))
	if !errors.Is(err, transport.ErrNotFound) {
		t.Errorf("err = %v, want wrap of ErrNotFound", err)
	}
}

func TestDeleteAllEHRs(t *testing.T) {
	var hits int
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := newClient(t, srv)
	if err := admin.DeleteAllEHRs(t.Context(), c); err != nil {
		t.Fatalf("DeleteAllEHRs: %v", err)
	}
	if hits != 1 {
		t.Errorf("hits = %d, want 1", hits)
	}
	if gotMethod != "DELETE" || gotPath != "/openehr/v1/admin/ehr/all" {
		t.Errorf("DELETE /admin/ehr/all expected, got %s %s", gotMethod, gotPath)
	}
}

func TestDeleteAllEHRsSubset(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.Query().Encode()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := newClient(t, srv)
	if err := admin.DeleteAllEHRs(t.Context(), c, ehr.EHRID("a"), ehr.EHRID("b")); err != nil {
		t.Fatalf("DeleteAllEHRs subset: %v", err)
	}
	if gotPath != "/openehr/v1/admin/ehr/all" {
		t.Errorf("path = %q, want /openehr/v1/admin/ehr/all", gotPath)
	}
	if gotQuery != "ehr_id=a&ehr_id=b" {
		t.Errorf("query = %q, want ehr_id=a&ehr_id=b", gotQuery)
	}
}

func TestPurgeTemplates(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := newClient(t, srv)
	if err := admin.PurgeTemplates(t.Context(), c); err != nil {
		t.Fatalf("PurgeTemplates: %v", err)
	}
	if gotMethod != "DELETE" || gotPath != "/openehr/v1/admin/template/all" {
		t.Errorf("DELETE /admin/template/all expected, got %s %s", gotMethod, gotPath)
	}
}

func TestRepositoryRoundTrip(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := newClient(t, srv)
	repo := admin.NewRepository(c)
	if err := repo.DeleteEHR(t.Context(), ehr.EHRID("xyz")); err != nil {
		t.Fatal(err)
	}
	if err := repo.PurgeTemplates(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteAllEHRs(t.Context()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"DELETE /openehr/v1/admin/ehr/xyz",
		"DELETE /openehr/v1/admin/template/all",
		"DELETE /openehr/v1/admin/ehr/all",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("call[%d] = %q, want %q", i, calls[i], want[i])
		}
	}
}
