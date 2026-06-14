package ehrstatus_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/ehrstatus"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const (
	ehrIDFixture openehrclient.EHRID      = "bf0b2ad8-7b0e-4f4d-9d33-6a8de69f0a64"
	ehrStatusUID openehrclient.VersionUID = "f1e2d3c4-b5a6-4978-89ab-cdef01234567::cdr.example::1"
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

func TestPutMinimal(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		body, _ := io.ReadAll(r.Body)
		if len(body) == 0 {
			t.Error("expected non-empty request body")
		}
		w.Header().Set("ETag", `"new-version-uid"`)
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/ehr_status/new-version-uid")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	status := &rm.EHRStatus{
		ArchetypeNodeID: "openEHR-EHR-EHR_STATUS.generic.v1",
		Name:            rm.DVText{Value: "EHR Status"},
		IsModifiable:    true,
		IsQueryable:     true,
		Subject:         rm.PartySelf{},
	}
	got, meta, err := ehrstatus.Put(context.Background(), newClient(t, srv), ehrIDFixture, "old-version-uid", status)
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodPut {
		t.Errorf("method = %q, want PUT", captured.Method)
	}
	if captured.Header.Get("If-Match") != `"old-version-uid"` {
		t.Errorf("If-Match = %q", captured.Header.Get("If-Match"))
	}
	if captured.Header.Get("Prefer") != "return=minimal" {
		t.Errorf("Prefer = %q, want return=minimal (default)", captured.Header.Get("Prefer"))
	}
	if got != nil {
		t.Errorf("expected nil EHR_STATUS body on PreferMinimal, got %+v", got)
	}
	if meta == nil || meta.ETag != "new-version-uid" {
		t.Errorf("expected ETag captured, got %+v", meta)
	}
	if meta.VersionUID != "new-version-uid" {
		t.Errorf("VersionUID parsed from Location = %q", meta.VersionUID)
	}
}

// TestPutRepresentationEmptyBodyErrors pins REQ-094 on the ehr_status
// leaf (same shared doWrite pattern as composition/directory): an empty
// body under Prefer=return=representation MUST surface
// transport.ErrInvalidShape, not a silent nil EHR_STATUS.
func TestPutRepresentationEmptyBodyErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"`+string(ehrStatusUID)+`"`)
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/ehr_status/"+string(ehrStatusUID))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	status := &rm.EHRStatus{
		ArchetypeNodeID: "openEHR-EHR-EHR_STATUS.generic.v1",
		Name:            rm.DVText{Value: "EHR Status"},
		IsModifiable:    true,
		IsQueryable:     true,
		Subject:         rm.PartySelf{},
	}
	out, meta, err := ehrstatus.Put(context.Background(), newClient(t, srv), ehrIDFixture, "old-version-uid", status,
		ehrstatus.WithPrefer(transport.PreferRepresentation),
	)
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Fatalf("expected ErrInvalidShape, got %v", err)
	}
	if out != nil {
		t.Errorf("expected nil EHR_STATUS on empty representation body, got %+v", out)
	}
	if meta == nil || meta.VersionUID != ehrStatusUID {
		t.Errorf("expected metadata still populated from headers, got %+v", meta)
	}
}

// TestPutIdentifierPopulatesVersionUIDFromBody pins REQ-094 Phase 2 on
// the ehr_status leaf: the ITS-REST Identifier body populates the
// identifier slot when Location is absent.
func TestPutIdentifierPopulatesVersionUIDFromBody(t *testing.T) {
	const idVUID openehrclient.VersionUID = "cccc3333-4444-5555-6666-777788889999::cdr.example::2"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"uid":"` + string(idVUID) + `"}`))
	}))
	defer srv.Close()

	status := &rm.EHRStatus{
		ArchetypeNodeID: "openEHR-EHR-EHR_STATUS.generic.v1",
		Name:            rm.DVText{Value: "EHR Status"},
		IsModifiable:    true,
		IsQueryable:     true,
		Subject:         rm.PartySelf{},
	}
	out, meta, err := ehrstatus.Put(context.Background(), newClient(t, srv), ehrIDFixture, "old-version-uid", status,
		ehrstatus.WithPrefer(transport.PreferIdentifier),
	)
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Errorf("expected nil EHR_STATUS in identifier mode, got %+v", out)
	}
	if meta == nil || meta.VersionUID != idVUID {
		t.Fatalf("expected VersionUID %q from identifier body, got %+v", idVUID, meta)
	}
}

func TestPutRejectsEmptyIfMatch(t *testing.T) {
	_, _, err := ehrstatus.Put(context.Background(), nil, ehrIDFixture, "", &rm.EHRStatus{})
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestPutMapsPreconditionRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Server simulates a 428 even though SDK sent an If-Match
		// (to exercise wire-level mapping; SDK's compile-time guard
		// already prevents empty If-Match).
		w.WriteHeader(http.StatusPreconditionRequired)
		_, _ = w.Write([]byte(`{"message":"PUT requires If-Match","code":"PRECONDITION_REQUIRED"}`))
	}))
	defer srv.Close()
	_, _, err := ehrstatus.Put(context.Background(), newClient(t, srv), ehrIDFixture, "v-1", &rm.EHRStatus{})
	if !errors.Is(err, transport.ErrPreconditionRequired) {
		t.Errorf("expected ErrPreconditionRequired, got %v", err)
	}
}

func TestPutMapsVersionConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"stale If-Match","code":"VERSION_CONFLICT"}`))
	}))
	defer srv.Close()
	_, _, err := ehrstatus.Put(context.Background(), newClient(t, srv), ehrIDFixture, "stale", &rm.EHRStatus{})
	if !errors.Is(err, transport.ErrVersionConflict) {
		t.Errorf("expected ErrVersionConflict, got %v", err)
	}
}

func TestPutWithAuditDetails(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	aliceName := "alice"
	audit := &rm.AuditDetails{
		SystemID:  "cdr.example",
		Committer: rm.PartyIdentified{Name: &aliceName},
		ChangeType: rm.DVCodedText{
			DVText:       rm.DVText{Value: "modification"},
			DefiningCode: rm.CodePhrase{CodeString: "249"},
		},
		TimeCommitted: rm.DVDateTime{Value: "2026-05-17T10:00:00Z"},
	}
	if _, _, err := ehrstatus.Put(context.Background(), newClient(t, srv), ehrIDFixture, "v-1", &rm.EHRStatus{},
		ehrstatus.WithAuditDetails(audit),
	); err != nil {
		t.Fatal(err)
	}
	header := captured.Header.Get("openehr-audit-details")
	if header == "" {
		t.Fatal("openehr-audit-details header not set")
	}
	if !contains(header, `"system_id":"cdr.example"`) {
		t.Errorf("audit header = %q", header)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
