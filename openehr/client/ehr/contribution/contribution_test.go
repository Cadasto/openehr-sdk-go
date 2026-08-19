package contribution_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const ehrIDFixture openehrclient.EHRID = "bf0b2ad8-7b0e-4f4d-9d33-6a8de69f0a64"

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

// newAudit builds the write-side UpdateAudit DTO for Submission.Audit
// (no time_committed; _type AUDIT_DETAILS).
func newAudit() contribution.UpdateAudit {
	name := "alice"
	return contribution.UpdateAudit{
		SystemID:  "cdr.example",
		Committer: &rm.PartyIdentified{Name: &name},
		ChangeType: rm.DVCodedText{
			DVText:       rm.DVText{Value: "creation"},
			DefiningCode: rm.CodePhrase{CodeString: "249"},
		},
	}
}

// newCommitAudit builds the persisted-shaped rm.AuditDetails used as a
// version's commit_audit (the sealed AuditDetailsLike). Distinct from
// newAudit(), which builds the write-side UpdateAudit for Submission.Audit.
func newCommitAudit() rm.AuditDetails {
	name := "alice"
	return rm.AuditDetails{
		SystemID:      "cdr.example",
		Committer:     &rm.PartyIdentified{Name: &name},
		ChangeType:    rm.DVCodedText{DVText: rm.DVText{Value: "creation"}, DefiningCode: rm.CodePhrase{CodeString: "249"}},
		TimeCommitted: rm.DVDateTime{Value: "2026-05-17T10:00:00Z"},
	}
}

// newOriginalVersion builds a minimal ORIGINAL_VERSION<COMPOSITION>
// suitable for use as a Submission.Versions[i] element. The Composition
// fixture only needs ArchetypeNodeID for the round-trip assertion —
// production callers pass real archetype-node Compositions.
func newOriginalVersion() *contribution.OriginalVersion[rm.Composition] {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	return contribution.WrapOriginalVersion(&rm.OriginalVersion[rm.Composition]{
		Version: rm.Version[rm.Composition]{
			CommitAudit: newCommitAudit(),
		},
		UID:            rm.ObjectVersionID{Value: "1::cdr.example::1"},
		LifecycleState: rm.DVCodedText{DVText: rm.DVText{Value: "complete"}, DefiningCode: rm.CodePhrase{CodeString: "532"}},
		Data:           &comp,
	})
}

func TestCommitMinimal(t *testing.T) {
	var captured *http.Request
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		b, _ := io.ReadAll(r.Body)
		capturedBody = b
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/contribution/cont-1")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	batch := &contribution.Submission{
		Audit:    newAudit(),
		Versions: []contribution.CommitVersion{newOriginalVersion()},
	}
	out, meta, err := contribution.Commit(t.Context(), newClient(t, srv), ehrIDFixture, batch)
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Errorf("expected nil Contribution body on PreferMinimal, got %+v", out)
	}
	if captured.URL.Path != "/openehr/v1/ehr/"+string(ehrIDFixture)+"/contribution" {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if captured.Method != http.MethodPost {
		t.Errorf("method = %q", captured.Method)
	}
	if got := captured.Header.Get("Prefer"); got != "return=minimal" {
		t.Errorf("Prefer = %q (default)", got)
	}
	if !strings.Contains(string(capturedBody), `"system_id":"cdr.example"`) {
		t.Errorf("audit not in body: %s", string(capturedBody))
	}
	if meta.Location == "" {
		t.Error("Location not captured")
	}
}

func TestCommitRejectsInputs(t *testing.T) {
	_, _, err := contribution.Commit(t.Context(), nil, "", nil)
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("empty EHRID: expected ErrInvalidConfig, got %v", err)
	}
	_, _, err = contribution.Commit(t.Context(), nil, ehrIDFixture, nil)
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("nil batch: expected ErrInvalidConfig, got %v", err)
	}
}

// TestCommitSubmissionShape pins REQ-050/095 / PROBE-072 — the wire body
// of POST /ehr/{id}/contribution must be the ITS-REST Contribution_create
// shape (versions[i] is ORIGINAL_VERSION<T> with inline data), NOT the
// persisted CONTRIBUTION shape (versions[] of OBJECT_REF). The
// regression we are guarding against is exactly the second shape: the
// pre-REQ-050/095 Commit took *rm.Contribution and emitted
// versions[]: [{"_type":"OBJECT_REF",...}].
func TestCommitSubmissionShape(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = b
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/contribution/cont-1")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	batch := &contribution.Submission{
		Audit:    newAudit(),
		Versions: []contribution.CommitVersion{newOriginalVersion()},
	}
	if _, _, err := contribution.Commit(t.Context(), newClient(t, srv), ehrIDFixture, batch); err != nil {
		t.Fatal(err)
	}

	var body struct {
		Type     string           `json:"_type"`
		Audit    map[string]any   `json:"audit"`
		Versions []map[string]any `json:"versions"`
	}
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("submission body is not valid JSON: %v\n%s", err, capturedBody)
	}
	if body.Type != "" {
		t.Errorf("Contribution_create has no top-level _type envelope, got %q", body.Type)
	}
	if body.Audit["_type"] != "AUDIT_DETAILS" {
		t.Errorf("audit._type = %v (want AUDIT_DETAILS)", body.Audit["_type"])
	}
	if got, want := body.Audit["system_id"], "cdr.example"; got != want {
		t.Errorf("audit.system_id = %v (want %v)", got, want)
	}
	if len(body.Versions) != 1 {
		t.Fatalf("len(versions) = %d (want 1)", len(body.Versions))
	}
	v0 := body.Versions[0]
	switch t0 := v0["_type"]; t0 {
	case "ORIGINAL_VERSION", "IMPORTED_VERSION":
	case "OBJECT_REF":
		t.Errorf("versions[0] is OBJECT_REF — pre-REQ-050/095 persisted shape leaked into the submission body")
	default:
		t.Errorf("versions[0]._type = %v (want ORIGINAL_VERSION or IMPORTED_VERSION)", t0)
	}
	data, ok := v0["data"].(map[string]any)
	if !ok {
		t.Fatalf("versions[0].data missing or not an object: %v", v0["data"])
	}
	if data["_type"] != "COMPOSITION" {
		t.Errorf("versions[0].data._type = %v (want COMPOSITION)", data["_type"])
	}
	if data["archetype_node_id"] != "openEHR-EHR-COMPOSITION.report.v1" {
		t.Errorf("versions[0].data.archetype_node_id = %v (Composition payload not inlined)", data["archetype_node_id"])
	}
	// strings used to disambiguate failures in the raw body if the
	// structural assertions above all pass but the wire shape later
	// regresses for some subtle reason. Belt-and-braces.
	if !strings.Contains(string(capturedBody), `"_type":"ORIGINAL_VERSION"`) {
		t.Errorf("ORIGINAL_VERSION discriminator missing from raw body: %s", capturedBody)
	}
}

func TestCommitMapsVersionConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"batch conflict","code":"VERSION_CONFLICT"}`))
	}))
	defer srv.Close()
	batch := &contribution.Submission{
		Audit:    newAudit(),
		Versions: []contribution.CommitVersion{newOriginalVersion()},
	}
	_, _, err := contribution.Commit(t.Context(), newClient(t, srv), ehrIDFixture, batch)
	if !errors.Is(err, transport.ErrVersionConflict) {
		t.Errorf("expected ErrVersionConflict, got %v", err)
	}
}

// contributionGetFixture is a canonical persisted CONTRIBUTION body —
// the `versions[]` of OBJECT_REF shape (distinct from the
// Contribution_create submission body asserted above).
const contributionGetFixture = `{
  "_type": "CONTRIBUTION",
  "uid": {"_type": "HIER_OBJECT_ID", "value": "0826851c-c4c2-4d61-92b9-410fb8275ff0"},
  "audit": {
    "_type": "AUDIT_DETAILS",
    "system_id": "cdr.example",
    "committer": {"_type": "PARTY_IDENTIFIED", "name": "alice"},
    "change_type": {
      "_type": "DV_CODED_TEXT",
      "value": "creation",
      "defining_code": {"_type": "CODE_PHRASE", "code_string": "249"}
    }
  },
  "versions": [
    {"_type": "OBJECT_REF", "namespace": "local", "type": "COMPOSITION",
     "id": {"_type": "OBJECT_VERSION_ID", "value": "8849182c-82ad-4088-a07f-48ead4180515::cdr.example::1"}}
  ]
}`

// TestGetRejectsInputs pins REQ-142's fail-before-request rule: an empty
// ehr_id or contribution_uid is refused with ErrInvalidConfig and MUST
// NOT issue a request (an empty interpolated segment is what REQ-150's
// validator refuses; the sentinel contract has one home).
func TestGetRejectsInputs(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := newClient(t, srv)

	_, _, err := contribution.Get(t.Context(), c, "", "cont-1")
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("empty EHRID: expected ErrInvalidConfig, got %v", err)
	}
	_, _, err = contribution.Get(t.Context(), c, ehrIDFixture, "")
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("empty contributionUID: expected ErrInvalidConfig, got %v", err)
	}
	if requests != 0 {
		t.Errorf("issued %d request(s); REQ-142 requires failing before any request", requests)
	}
}

// TestGet pins the REQ-142 / PROBE-092 request shape and the 200 decode.
// Per the vendored pin, 200_CONTRIBUTION defines only Content-Type, so
// no version-metadata assertion is made here.
func TestGet(t *testing.T) {
	const uid = "0826851c-c4c2-4d61-92b9-410fb8275ff0"
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(contributionGetFixture))
	}))
	defer srv.Close()

	out, _, err := contribution.Get(t.Context(), newClient(t, srv), ehrIDFixture, uid)
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodGet {
		t.Errorf("method = %q (want GET)", captured.Method)
	}
	if want := "/openehr/v1/ehr/" + string(ehrIDFixture) + "/contribution/" + uid; captured.URL.Path != want {
		t.Errorf("path = %q, want %q", captured.URL.Path, want)
	}
	if out == nil {
		t.Fatal("nil Contribution on 200")
	}
	if out.UID.Value != uid {
		t.Errorf("uid = %q, want %q", out.UID.Value, uid)
	}
	if len(out.Versions) != 1 {
		t.Fatalf("len(versions) = %d (want 1)", len(out.Versions))
	}
	if out.Audit == nil {
		t.Error("audit not decoded")
	}
}

// TestGetMapsNotFound pins REQ-142's 404 -> ErrNotFound mapping.
func TestGetMapsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"no such contribution"}`))
	}))
	defer srv.Close()
	_, _, err := contribution.Get(t.Context(), newClient(t, srv), ehrIDFixture, "missing")
	if !errors.Is(err, transport.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
