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
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
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

func TestSaveMinimal(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("ETag", `"`+string(compositionVUID)+`"`)
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(compositionVUID))
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	comp := readComposition(t)
	out, meta, err := composition.Save(context.Background(), newClient(t, srv), ehrIDFixture, comp,
		composition.WithTemplateID("openEHR-EHR-COMPOSITION.body_weight.v1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Errorf("expected nil Composition on default Prefer=minimal, got %+v", out)
	}
	if captured.Method != http.MethodPost {
		t.Errorf("method = %q", captured.Method)
	}
	if captured.URL.Path != "/openehr/v1/ehr/"+string(ehrIDFixture)+"/composition" {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if got := captured.Header.Get("Prefer"); got != "return=minimal" {
		t.Errorf("Prefer = %q (default), want return=minimal", got)
	}
	if got := captured.Header.Get("Openehr-Template-Id"); got != "openEHR-EHR-COMPOSITION.body_weight.v1" {
		t.Errorf("openehr-template-id = %q", got)
	}
	if meta.VersionUID != compositionVUID {
		t.Errorf("VersionUID = %q", meta.VersionUID)
	}
}

// TestSaveRepresentationDecodesBareComposition pins SDK-GAP-09:
// `Prefer: return=representation` on POST returns a bare COMPOSITION
// (not an ORIGINAL_VERSION<COMPOSITION>) per the ITS-REST OpenAPI
// `201_COMPOSITION` schema (oneOf: Composition | Identifier).
func TestSaveRepresentationDecodesBareComposition(t *testing.T) {
	body := readCompositionCassette(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"`+string(compositionVUID)+`"`)
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(compositionVUID))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	comp := readComposition(t)
	out, meta, err := composition.Save(context.Background(), newClient(t, srv), ehrIDFixture, comp,
		composition.WithPrefer(transport.PreferRepresentation),
	)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Fatal("expected decoded *rm.Composition on Prefer=representation, got nil")
	}
	if out.ArchetypeNodeID == "" {
		t.Errorf("decoded Composition missing archetype_node_id (bare-body decode likely wrong)")
	}
	if meta.VersionUID != compositionVUID {
		t.Errorf("VersionUID = %q", meta.VersionUID)
	}
}

// TestSaveRepresentationRejectsOriginalVersionShape pins the strict-
// against-spec posture: if a non-conformant server returns an
// `ORIGINAL_VERSION<COMPOSITION>` envelope on POST, the decode MUST
// surface that as an error rather than silently masking it (the
// `_type` of an OV envelope decodes as a Composition type mismatch).
func TestSaveRepresentationRejectsOriginalVersionShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"_type":"ORIGINAL_VERSION","uid":{"_type":"OBJECT_VERSION_ID","value":"x::y::1"},"data":{"_type":"COMPOSITION","name":{"_type":"DV_TEXT","value":"x"}}}`))
	}))
	defer srv.Close()

	comp := readComposition(t)
	out, _, err := composition.Save(context.Background(), newClient(t, srv), ehrIDFixture, comp,
		composition.WithPrefer(transport.PreferRepresentation),
	)
	if err == nil {
		t.Fatalf("expected decode error on ORIGINAL_VERSION envelope, got out=%+v", out)
	}
}

func TestSaveRejectsNil(t *testing.T) {
	_, _, err := composition.Save(context.Background(), nil, ehrIDFixture, nil)
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestUpdateRequiresIfMatch(t *testing.T) {
	_, _, err := composition.Update(context.Background(), nil, ehrIDFixture, compositionVOID, "", &rm.Composition{})
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig on empty If-Match, got %v", err)
	}
}

func TestUpdateRoundTrip(t *testing.T) {
	var capturedPUT *http.Request
	newVUID := openehrclient.VersionUID("1234abcd-5678-9012-3456-7890abcdef00::cdr.example::2")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPUT = r.Clone(r.Context())
		w.Header().Set("ETag", `"`+string(newVUID)+`"`)
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(newVUID))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	comp := readComposition(t)
	_, meta, err := composition.Update(context.Background(), newClient(t, srv), ehrIDFixture, compositionVOID, string(compositionVUID), comp)
	if err != nil {
		t.Fatal(err)
	}
	if capturedPUT.Method != http.MethodPut {
		t.Errorf("method = %q", capturedPUT.Method)
	}
	if got := capturedPUT.Header.Get("If-Match"); got != `"`+string(compositionVUID)+`"` {
		t.Errorf("If-Match = %q (expected re-quoted)", got)
	}
	if meta.VersionUID != newVUID {
		t.Errorf("new VersionUID = %q", meta.VersionUID)
	}
}

// TestUpdateRepresentationDecodesBareComposition pins SDK-GAP-09 on
// the PUT path: `Prefer: return=representation` on PUT returns a bare
// COMPOSITION per the ITS-REST OpenAPI `200_COMPOSITION_updated`
// schema. Save and Update share `doWrite` but the catalog title for
// PROBE-071 cites both POST and PUT, so the PUT arm is exercised
// explicitly here.
func TestUpdateRepresentationDecodesBareComposition(t *testing.T) {
	body := readCompositionCassette(t)
	newVUID := openehrclient.VersionUID("1234abcd-5678-9012-3456-7890abcdef00::cdr.example::2")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"`+string(newVUID)+`"`)
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(newVUID))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	comp := readComposition(t)
	out, meta, err := composition.Update(context.Background(), newClient(t, srv), ehrIDFixture, compositionVOID, string(compositionVUID), comp,
		composition.WithPrefer(transport.PreferRepresentation),
	)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Fatal("expected decoded *rm.Composition on PUT Prefer=representation, got nil")
	}
	if out.ArchetypeNodeID == "" {
		t.Errorf("decoded Composition missing archetype_node_id (bare-body decode likely wrong)")
	}
	if meta.VersionUID != newVUID {
		t.Errorf("new VersionUID = %q", meta.VersionUID)
	}
}

// TestUpdateRepresentationRejectsOriginalVersionShape mirrors the
// POST-side strict-against-spec test on the PUT path: a non-conformant
// server returning ORIGINAL_VERSION on `200_COMPOSITION_updated` must
// surface as a decode error, not silent acceptance.
func TestUpdateRepresentationRejectsOriginalVersionShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"_type":"ORIGINAL_VERSION","uid":{"_type":"OBJECT_VERSION_ID","value":"x::y::1"},"data":{"_type":"COMPOSITION","name":{"_type":"DV_TEXT","value":"x"}}}`))
	}))
	defer srv.Close()

	comp := readComposition(t)
	out, _, err := composition.Update(context.Background(), newClient(t, srv), ehrIDFixture, compositionVOID, string(compositionVUID), comp,
		composition.WithPrefer(transport.PreferRepresentation),
	)
	if err == nil {
		t.Fatalf("expected decode error on ORIGINAL_VERSION envelope, got out=%+v", out)
	}
}

func TestUpdateMapsPreconditionFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
		_, _ = w.Write([]byte(`{"message":"stale","code":"PRECONDITION_FAILED"}`))
	}))
	defer srv.Close()
	_, _, err := composition.Update(context.Background(), newClient(t, srv), ehrIDFixture, compositionVOID, "stale", &rm.Composition{})
	if !errors.Is(err, transport.ErrPreconditionFailed) {
		t.Errorf("expected ErrPreconditionFailed, got %v", err)
	}
}

func TestDelete(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	_, err := composition.Delete(context.Background(), newClient(t, srv), ehrIDFixture, compositionVUID, string(compositionVUID))
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodDelete {
		t.Errorf("method = %q", captured.Method)
	}
	if captured.URL.Path != "/openehr/v1/ehr/"+string(ehrIDFixture)+"/composition/"+string(compositionVUID) {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if got := captured.Header.Get("If-Match"); got != `"`+string(compositionVUID)+`"` {
		t.Errorf("If-Match = %q", got)
	}
}

func TestDeleteRequiresIfMatch(t *testing.T) {
	_, err := composition.Delete(context.Background(), nil, ehrIDFixture, compositionVUID, "")
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

// readComposition decodes the body_weight cassette into a *rm.Composition
// so write-path tests have a valid payload without hand-constructing one.
func readComposition(t *testing.T) *rm.Composition {
	t.Helper()
	body := readCompositionCassette(t)
	var comp rm.Composition
	if err := canjson.Unmarshal(body, &comp); err != nil {
		t.Fatalf("decode composition cassette: %v", err)
	}
	return &comp
}
