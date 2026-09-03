package versionedprobes_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/versioned"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const (
	ehrIDFixture    openehrclient.EHRID             = "bf0b2ad8-7b0e-4f4d-9d33-6a8de69f0a64"
	compositionVOID openehrclient.VersionedObjectID = "1234abcd-5678-9012-3456-7890abcdef00"
	initialVUID     openehrclient.VersionUID        = "1234abcd-5678-9012-3456-7890abcdef00::cdr.example::1"
	updatedVUID     openehrclient.VersionUID        = "1234abcd-5678-9012-3456-7890abcdef00::cdr.example::2"
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

func TestProbe010PutWithoutIfMatch(t *testing.T) {
	// PROBE-010 is a compile-time guard exercise — no network needed.
	// Construct a client against any throwaway server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("PROBE-010 must short-circuit before any network call")
	}))
	defer srv.Close()
	r, err := probes.Probe010PutWithoutIfMatch(context.Background(), newClient(t, srv), ehrIDFixture)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-010 status = %q (detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe011PutStaleIfMatch_412(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
		_, _ = w.Write([]byte(`{"message":"stale","code":"PRECONDITION_FAILED"}`))
	}))
	defer srv.Close()
	r, err := probes.Probe011PutStaleIfMatch(context.Background(), newClient(t, srv), ehrIDFixture, compositionVOID, "stale", &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-011 (412) status = %q (detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe011PutStaleIfMatch_409(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"stale","code":"VERSION_CONFLICT"}`))
	}))
	defer srv.Close()
	r, err := probes.Probe011PutStaleIfMatch(context.Background(), newClient(t, srv), ehrIDFixture, compositionVOID, "stale", &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-011 (409) status = %q (detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe012ETagRoundTrip(t *testing.T) {
	// Phase 1 of the probe: a GET returns the initial VersionUID via
	// Location; phase 2: PUT carries that as If-Match and the server
	// returns a fresh VersionUID. The fake server below alternates
	// between the two phases on a single shared state.
	var phase int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		phase++
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(initialVUID))
			w.Header().Set("ETag", `"`+string(initialVUID)+`"`)
			// Body must be a minimal valid composition for canjson decode.
			_, _ = w.Write([]byte(`{"_type":"COMPOSITION","name":{"_type":"DV_TEXT","value":"x"},"archetype_node_id":"openEHR-EHR-COMPOSITION.x.v1","language":{"_type":"CODE_PHRASE","code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}},"territory":{"_type":"CODE_PHRASE","code_string":"GB","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"}},"category":{"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}}}`))
		case http.MethodPut:
			ifMatch := r.Header.Get("If-Match")
			if ifMatch != `"`+string(initialVUID)+`"` {
				t.Errorf("PUT If-Match = %q, want %q", ifMatch, `"`+string(initialVUID)+`"`)
			}
			w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(updatedVUID))
			w.Header().Set("ETag", `"`+string(updatedVUID)+`"`)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected method %q", r.Method)
		}
	}))
	defer srv.Close()

	r, err := probes.Probe012ETagRoundTrip(context.Background(), newClient(t, srv), ehrIDFixture, compositionVOID, &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-012 status = %q (detail: %s)", r.Status, r.Detail)
	}
	if phase < 2 {
		t.Errorf("expected at least 2 server hits (GET + PUT), got %d", phase)
	}
}

// bareCompositionBody is a minimal canonical-JSON COMPOSITION body
// used by PROBE-071 server fakes — keeps the canjson decode path
// honest while staying inline-readable.
const bareCompositionBody = `{"_type":"COMPOSITION","name":{"_type":"DV_TEXT","value":"x"},"archetype_node_id":"openEHR-EHR-COMPOSITION.x.v1","language":{"_type":"CODE_PHRASE","code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}},"territory":{"_type":"CODE_PHRASE","code_string":"GB","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"}},"category":{"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}}}`

func TestProbe071CompositionWriteResponseShape_BareBody_POSTOnly(t *testing.T) {
	// Happy path, POST-only: caller omits voID/ifMatch so the PUT arm
	// is skipped. The probe still passes on a clean bare-body decode.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(initialVUID))
		w.Header().Set("ETag", `"`+string(initialVUID)+`"`)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(bareCompositionBody))
	}))
	defer srv.Close()

	r, err := probes.Probe071CompositionWriteResponseShape(context.Background(), newClient(t, srv), ehrIDFixture, "", "", &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-071 POST-only status = %q (detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe071CompositionWriteResponseShape_BareBody_POSTPlusPUT(t *testing.T) {
	// Happy path, both arms: caller supplies voID + ifMatch so the
	// PUT arm runs. Server returns a bare COMPOSITION on both verbs.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(initialVUID))
			w.Header().Set("ETag", `"`+string(initialVUID)+`"`)
			w.WriteHeader(http.StatusCreated)
		case http.MethodPut:
			w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(updatedVUID))
			w.Header().Set("ETag", `"`+string(updatedVUID)+`"`)
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected method %q", r.Method)
		}
		_, _ = w.Write([]byte(bareCompositionBody))
	}))
	defer srv.Close()

	r, err := probes.Probe071CompositionWriteResponseShape(context.Background(), newClient(t, srv), ehrIDFixture, compositionVOID, string(initialVUID), &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-071 POST+PUT status = %q (detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe071CompositionWriteResponseShape_RejectsOriginalVersion_POST(t *testing.T) {
	// Non-conformant deployment: server returns ORIGINAL_VERSION on
	// POST. The strict-against-spec SDK MUST decode-fail; the probe
	// reports that as fail status (server side is the bug).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(initialVUID))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"_type":"ORIGINAL_VERSION","uid":{"_type":"OBJECT_VERSION_ID","value":"x::y::1"},"data":{"_type":"COMPOSITION","name":{"_type":"DV_TEXT","value":"x"}}}`))
	}))
	defer srv.Close()

	r, err := probes.Probe071CompositionWriteResponseShape(context.Background(), newClient(t, srv), ehrIDFixture, "", "", &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" {
		t.Errorf("PROBE-071 POST-OV-envelope status = %q (expected fail, detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe071CompositionWriteResponseShape_RejectsOriginalVersion_PUT(t *testing.T) {
	// Non-conformant deployment on the PUT path: POST returns the
	// spec-correct bare body but PUT returns ORIGINAL_VERSION. The
	// probe must fail on the PUT arm and surface the asymmetry.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(initialVUID))
			w.Header().Set("ETag", `"`+string(initialVUID)+`"`)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(bareCompositionBody))
		case http.MethodPut:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"_type":"ORIGINAL_VERSION","uid":{"_type":"OBJECT_VERSION_ID","value":"x::y::1"},"data":{"_type":"COMPOSITION","name":{"_type":"DV_TEXT","value":"x"}}}`))
		default:
			t.Errorf("unexpected method %q", r.Method)
		}
	}))
	defer srv.Close()

	r, err := probes.Probe071CompositionWriteResponseShape(context.Background(), newClient(t, srv), ehrIDFixture, compositionVOID, string(initialVUID), &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" {
		t.Errorf("PROBE-071 PUT-OV-envelope status = %q (expected fail, detail: %s)", r.Status, r.Detail)
	}
}

// newOriginalVersionFixture builds a minimal ORIGINAL_VERSION<COMPOSITION>
// for the PROBE-072 server fakes — the probe doesn't care about clinical
// content, only the wire shape, so ArchetypeNodeID is the only Composition
// field set. Returns the contribution write-side wrapper so commit_audit
// drops the server-assigned time_committed.
func newOriginalVersionFixture() *contribution.OriginalVersion[rm.Composition] {
	name := "alice"
	audit := rm.AuditDetails{
		SystemID:  "cdr.example",
		Committer: &rm.PartyIdentified{Name: &name},
		ChangeType: rm.DVCodedText{
			DVText:       rm.DVText{Value: "creation"},
			DefiningCode: rm.CodePhrase{CodeString: "249"},
		},
		TimeCommitted: rm.DVDateTime{Value: "2026-05-17T10:00:00Z"},
	}
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	return contribution.WrapOriginalVersion(&rm.OriginalVersion[rm.Composition]{
		CommitAudit:    audit,
		UID:            rm.ObjectVersionID{Value: "1::cdr.example::1"},
		LifecycleState: rm.DVCodedText{DVText: rm.DVText{Value: "complete"}, DefiningCode: rm.CodePhrase{CodeString: "532"}},
		Data:           &comp,
	})
}

func TestProbe072ContributionSubmissionShapePass(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = b
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/contribution/cont-1")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	ov := newOriginalVersionFixture()
	sub := &contribution.Submission{
		Audit:    ov.CommitAudit,
		Versions: []contribution.CommitVersion{ov},
	}
	r, err := probes.Probe072ContributionSubmissionShape(context.Background(), newClient(t, srv), &capturedBody, ehrIDFixture, sub)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-072 status = %q detail=%q", r.Status, r.Detail)
	}
}

func TestProbe072ContributionSubmissionShapeRejectsObjectRef(t *testing.T) {
	// Server fake reads (and ignores) the real Commit request body and
	// plants the regression-shape — a persisted rm.Contribution with
	// versions[] of OBJECT_REF — into the captured slot. The probe
	// inspects *capturedBody, so it sees the planted body and MUST
	// flag the REQ-050/095 regression.
	planted := []byte(`{"_type":"CONTRIBUTION","audit":{"_type":"AUDIT_DETAILS","system_id":"x"},"versions":[{"_type":"OBJECT_REF","id":{"_type":"OBJECT_VERSION_ID","value":"1::x::1"}}]}`)
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		captured = planted
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/contribution/cont-1")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	ov := newOriginalVersionFixture()
	sub := &contribution.Submission{
		Audit:    ov.CommitAudit,
		Versions: []contribution.CommitVersion{ov},
	}
	r, err := probes.Probe072ContributionSubmissionShape(context.Background(), newClient(t, srv), &captured, ehrIDFixture, sub)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" {
		t.Errorf("PROBE-072 status = %q (expected fail for OBJECT_REF body, detail=%q)", r.Status, r.Detail)
	}
}

func TestProbe072RejectsTimeCommittedAudit(t *testing.T) {
	// Planted body: a Contribution_create shape whose batch audit carries a
	// server-assigned time_committed — the extended PROBE-072 MUST flag it
	// (SPECITS-95 / ITS-REST PR 131).
	planted := []byte(`{"audit":{"_type":"AUDIT_DETAILS","system_id":"x","change_type":{"_type":"DV_CODED_TEXT","defining_code":{"_type":"CODE_PHRASE","code_string":"249"}},"time_committed":{"value":"2026-01-01T00:00:00Z"}},"versions":[{"_type":"ORIGINAL_VERSION","data":{"_type":"COMPOSITION"},"commit_audit":{"_type":"AUDIT_DETAILS","change_type":{"_type":"DV_CODED_TEXT","defining_code":{"_type":"CODE_PHRASE","code_string":"249"}}}}]}`)
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		captured = planted
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/contribution/cont-1")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	ov := newOriginalVersionFixture()
	sub := &contribution.Submission{Audit: ov.CommitAudit, Versions: []contribution.CommitVersion{ov}}
	r, err := probes.Probe072ContributionSubmissionShape(context.Background(), newClient(t, srv), &captured, ehrIDFixture, sub)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" || !strings.Contains(r.Detail, "time_committed") {
		t.Errorf("PROBE-072 status=%q detail=%q (want fail mentioning time_committed)", r.Status, r.Detail)
	}
}

func TestProbe013CrossEHRIsolation(t *testing.T) {
	const (
		ehrAID          openehrclient.EHRID      = "ehrA-1111-2222-3333-444444444444"
		ehrBID          openehrclient.EHRID      = "ehrB-aaaa-bbbb-cccc-dddddddddddd"
		versionUIDFromA openehrclient.VersionUID = "9999abcd-5678-9012-3456-7890abcdef00::cdr.example::1"
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Tenant-isolated server: any composition GET under ehrBID for a
		// VersionUID that doesn't belong to ehrBID is a hard 404. The
		// probe MUST NOT see EHR A's id or data on this path.
		if got := r.URL.Path; !strings.Contains(got, string(ehrBID)) {
			t.Errorf("expected request path to target ehrBID, got %q", got)
		}
		if strings.Contains(r.URL.Path, string(ehrAID)) {
			t.Errorf("path should NOT contain ehrAID, got %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found","code":"NOT_FOUND"}`))
	}))
	defer srv.Close()
	r, err := probes.Probe013CrossEHRIsolation(context.Background(), newClient(t, srv), ehrAID, ehrBID, versionUIDFromA)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-013 status = %q (detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe013RejectsTenantLeak(t *testing.T) {
	// Negative branch: a server that returns 200 for the cross-EHR
	// read MUST be flagged as a tenant leak by the probe.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"_type":"COMPOSITION","name":{"_type":"DV_TEXT","value":"leak"},"archetype_node_id":"openEHR-EHR-COMPOSITION.x.v1","language":{"_type":"CODE_PHRASE","code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}},"territory":{"_type":"CODE_PHRASE","code_string":"GB","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"}},"category":{"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}}}`))
	}))
	defer srv.Close()
	r, err := probes.Probe013CrossEHRIsolation(context.Background(), newClient(t, srv), "ehrA", "ehrB", "vuid")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" {
		t.Errorf("expected fail on 200 cross-EHR read, got %q", r.Status)
	}
}

// contributionGetBody is a canonical persisted CONTRIBUTION — the
// versions[] of OBJECT_REF shape returned by contribution_get.
const contributionGetBody = `{
  "_type": "CONTRIBUTION",
  "uid": {"_type": "HIER_OBJECT_ID", "value": "0826851c-c4c2-4d61-92b9-410fb8275ff0"},
  "audit": {"_type": "AUDIT_DETAILS", "system_id": "cdr.example",
    "committer": {"_type": "PARTY_IDENTIFIED", "name": "alice"},
    "change_type": {"_type": "DV_CODED_TEXT", "value": "creation",
      "defining_code": {"_type": "CODE_PHRASE", "code_string": "249"}}},
  "versions": [{"_type": "OBJECT_REF", "namespace": "local", "type": "COMPOSITION",
    "id": {"_type": "OBJECT_VERSION_ID", "value": "8849182c-82ad-4088-a07f-48ead4180515::cdr.example::1"}}]
}`

// contributionGetServer answers 200 with a canonical contribution for
// presentUID and 404 for anything else, recording every request.
func contributionGetServer(t *testing.T, captured *[]*http.Request, presentUID string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*captured = append(*captured, r.Clone(r.Context()))
		if strings.HasSuffix(r.URL.Path, "/contribution/"+presentUID) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(contributionGetBody))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"no such contribution"}`))
	}))
}

func TestProbe092ContributionGetPass(t *testing.T) {
	const presentUID = "0826851c-c4c2-4d61-92b9-410fb8275ff0"
	var captured []*http.Request
	srv := contributionGetServer(t, &captured, presentUID)
	defer srv.Close()

	r, err := probes.Probe092ContributionGet(context.Background(), newClient(t, srv), &captured, ehrIDFixture, presentUID, "missing-uid")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-092 status = %q detail=%q", r.Status, r.Detail)
	}
}

// TestProbe092ContributionGetFlagsWrongMethod plants a backend that
// records a mutated request (POST) so the probe's method assertion is
// itself exercised — a probe that cannot fail proves nothing.
func TestProbe092ContributionGetFlagsWrongMethod(t *testing.T) {
	const presentUID = "0826851c-c4c2-4d61-92b9-410fb8275ff0"
	var captured []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		planted := r.Clone(r.Context())
		planted.Method = http.MethodPost
		captured = append(captured, planted)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(contributionGetBody))
	}))
	defer srv.Close()

	r, err := probes.Probe092ContributionGet(context.Background(), newClient(t, srv), &captured, ehrIDFixture, presentUID, "missing-uid")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" {
		t.Errorf("PROBE-092 status = %q, want fail on a non-GET method (detail=%q)", r.Status, r.Detail)
	}
}

func TestProbe092ContributionGetRejectsMissingInputs(t *testing.T) {
	var captured []*http.Request
	srv := contributionGetServer(t, &captured, "u")
	defer srv.Close()
	if _, err := probes.Probe092ContributionGet(context.Background(), nil, &captured, ehrIDFixture, "u", "m"); err == nil {
		t.Error("nil client: expected an error")
	}
	if _, err := probes.Probe092ContributionGet(context.Background(), newClient(t, srv), nil, ehrIDFixture, "u", "m"); err == nil {
		t.Error("nil recorder: expected an error")
	}
}

// contributionCommitServer records the request body and answers a bare 201.
func contributionCommitServer(captured *[]byte, plant []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if plant != nil {
			b = plant
		}
		*captured = b
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/contribution/cont-1")
		w.WriteHeader(http.StatusCreated)
	}))
}

// submissionCorpus loads the vendored submission corpus PROBE-084 uses as
// its shape witness.
func submissionCorpus(t *testing.T) [][]byte {
	t.Helper()
	paths, err := fixtures.ListSubmissionJSON()
	if err != nil {
		t.Fatalf("list submission corpus: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("submission corpus is empty")
	}
	corpus := make([][]byte, 0, len(paths))
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		corpus = append(corpus, raw)
	}
	return corpus
}

func TestProbe084BuiltContributionBodyPass(t *testing.T) {
	var captured []byte
	srv := contributionCommitServer(&captured, nil)
	defer srv.Close()
	r, err := probes.Probe084BuiltContributionBody(context.Background(), newClient(t, srv), &captured, ehrIDFixture, submissionCorpus(t))
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-084 status = %q detail=%q", r.Status, r.Detail)
	}
}

// probe084Planted builds a `Contribution_create` body in the shape
// PROBE-084's own batch produces, so a planted variant differs from the
// real thing in exactly the one clause under test. Each version is
// `{code, rmType, extra}`; `extra` injects the violation.
type probe084Planted struct {
	code   string
	rmType string
	extra  string
}

func probe084Body(batchCode string, versions ...probe084Planted) []byte {
	audit := func(code string) string {
		return `{"_type":"AUDIT_DETAILS","change_type":{"_type":"DV_CODED_TEXT","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"value":"openehr"},"code_string":"` + code + `"}}}`
	}
	out := make([]string, 0, len(versions))
	for _, v := range versions {
		s := `{"_type":"ORIGINAL_VERSION","commit_audit":` + audit(v.code) +
			`,"lifecycle_state":{"_type":"DV_CODED_TEXT","defining_code":{"_type":"CODE_PHRASE","code_string":"532"}}` +
			`,"data":{"_type":"` + v.rmType + `"}`
		if v.extra != "" {
			s += "," + v.extra
		}
		out = append(out, s+"}")
	}
	return []byte(`{"audit":` + audit(batchCode) + `,"versions":[` + strings.Join(out, ",") + `]}`)
}

// TestProbe084BuiltContributionBodyRejects plants bodies that each violate
// one REQ-130 clause, so a probe that stopped asserting a clause would fail
// here rather than keep reporting green. Each case also names the substring
// its Detail must carry: asserting only `status == "fail"` would let a case
// rot into failing for an unrelated reason (a version-count change, say) and
// still look like it was pinning its clause.
func TestProbe084BuiltContributionBodyRejects(t *testing.T) {
	const (
		preceding1 = `"preceding_version_uid":{"value":"8849182c-82ad-4088-a07f-48ead4180515::cdr.example::1"}`
		preceding2 = `"preceding_version_uid":{"value":"8849182c-82ad-4088-a07f-48ead4180515::cdr.example::2"}`
		preceding4 = `"preceding_version_uid":{"value":"8849182c-82ad-4088-a07f-48ead4180515::cdr.example::4"}`
		callerUID  = `"uid":{"value":"8849182c-82ad-4088-a07f-48ead4180515::cdr.example::3"}`
	)
	// The four versions PROBE-084's own batch emits, in order.
	conformant := []probe084Planted{
		{code: "249", rmType: "COMPOSITION"},
		{code: "250", rmType: "COMPOSITION", extra: preceding1},
		{code: "251", rmType: "EHR_STATUS", extra: preceding2 + "," + callerUID},
		{code: "523", rmType: "FOLDER", extra: preceding4},
	}
	// mutate returns the conformant batch with one version replaced.
	mutate := func(i int, v probe084Planted) []probe084Planted {
		out := slices.Clone(conformant)
		out[i] = v
		return out
	}
	cases := []struct {
		name       string
		planted    []byte
		wantDetail string
	}{
		{
			name:       "batch change_type derived from a version",
			planted:    probe084Body("249", conformant...),
			wantDetail: "forbids deriving the batch code",
		},
		{
			name:       "creation carries a preceding version",
			planted:    probe084Body("253", mutate(0, probe084Planted{code: "249", rmType: "COMPOSITION", extra: preceding1})...),
			wantDetail: "a creation follows no version",
		},
		{
			name:       "amendment lacks a preceding version",
			planted:    probe084Body("253", mutate(1, probe084Planted{code: "250", rmType: "COMPOSITION"})...),
			wantDetail: "missing preceding_version_uid",
		},
		{
			name:       "wrong change-type code for the operation",
			planted:    probe084Body("253", mutate(2, probe084Planted{code: "250", rmType: "EHR_STATUS", extra: preceding2 + "," + callerUID})...),
			wantDetail: `change_type code = "250", want "251"`,
		},
		{
			name:       "payload type swapped",
			planted:    probe084Body("253", mutate(2, probe084Planted{code: "251", rmType: "COMPOSITION", extra: preceding2 + "," + callerUID})...),
			wantDetail: "data._type = COMPOSITION, want EHR_STATUS",
		},
		{
			// The counter-arm: a uid the caller DID supply must survive.
			name:       "caller-supplied uid dropped",
			planted:    probe084Body("253", mutate(2, probe084Planted{code: "251", rmType: "EHR_STATUS", extra: preceding2})...),
			wantDetail: "dropped the caller-supplied `uid`",
		},
		{
			name:       "lifecycle_state absent",
			planted:    []byte(strings.Replace(string(probe084Body("253", conformant...)), `"lifecycle_state":{"_type":"DV_CODED_TEXT","defining_code":{"_type":"CODE_PHRASE","code_string":"532"}},`, "", 1)),
			wantDetail: "lifecycle_state is missing",
		},
		{
			name:       "server-assigned contribution emitted",
			planted:    probe084Body("253", mutate(0, probe084Planted{code: "249", rmType: "COMPOSITION", extra: `"contribution":null`})...),
			wantDetail: "emits the server-assigned `contribution`",
		},
		{
			name:       "empty uid emitted",
			planted:    probe084Body("253", mutate(0, probe084Planted{code: "249", rmType: "COMPOSITION", extra: `"uid":{"value":""}`})...),
			wantDetail: "whose caller supplied none",
		},
		{
			// The arm that a "reject only an empty uid" reading would let
			// through: REQ-130 forbids the builder synthesising a uid, so a
			// non-empty one the caller never asked for must fail too.
			name:       "synthesised non-empty uid",
			planted:    probe084Body("253", mutate(0, probe084Planted{code: "249", rmType: "COMPOSITION", extra: `"uid":{"value":"invented::cdr.example::1"}`})...),
			wantDetail: "whose caller supplied none",
		},
		{
			// What dropping `omitempty` from the write-side uid field emits.
			// A present-but-null key type-asserts like a missing one, so this
			// is the arm that keeps absence judged on key presence.
			name:       "null uid emitted",
			planted:    probe084Body("253", mutate(0, probe084Planted{code: "249", rmType: "COMPOSITION", extra: `"uid":null`})...),
			wantDetail: "absent rather than empty or null",
		},
		{
			name:       "top-level CONTRIBUTION envelope",
			planted:    []byte(`{"_type":"CONTRIBUTION",` + strings.TrimPrefix(string(probe084Body("253", conformant...)), "{")),
			wantDetail: "Contribution_create has no class envelope",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured []byte
			srv := contributionCommitServer(&captured, tc.planted)
			defer srv.Close()
			r, err := probes.Probe084BuiltContributionBody(context.Background(), newClient(t, srv), &captured, ehrIDFixture, submissionCorpus(t))
			if err != nil {
				t.Fatal(err)
			}
			if r.Status != "fail" {
				t.Errorf("PROBE-084 status = %q, want fail (detail=%q)", r.Status, r.Detail)
			}
			if !strings.Contains(r.Detail, tc.wantDetail) {
				t.Errorf("PROBE-084 failed for the wrong reason:\n got: %s\nwant it to mention: %s", r.Detail, tc.wantDetail)
			}
		})
	}
}

// updateGolden regenerates the checked-in PROBE-084 body golden.
var updateGolden = flag.Bool("update", false, "update probe golden files")

// TestProbe084BuiltBodyGolden is PROBE-084's byte-level pin: the exact
// `Contribution_create` bytes the builder puts on the wire, field order
// included. The vendored corpus cannot serve as this golden — its records
// carry an envelope and payloads this SDK does not author — so the golden
// is the SDK's own output, and a change to it is a reviewable wire change.
func TestProbe084BuiltBodyGolden(t *testing.T) {
	var captured []byte
	srv := contributionCommitServer(&captured, nil)
	defer srv.Close()
	r, err := probes.Probe084BuiltContributionBody(context.Background(), newClient(t, srv), &captured, ehrIDFixture, submissionCorpus(t))
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Fatalf("PROBE-084 status = %q detail=%q", r.Status, r.Detail)
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, captured, "", "  "); err != nil {
		t.Fatalf("indent: %v", err)
	}
	pretty.WriteByte('\n')

	golden := filepath.Join("testdata", "probe_084_built_body.json")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, pretty.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run with -update): %v", err)
	}
	if !bytes.Equal(pretty.Bytes(), want) {
		t.Errorf("built body != golden %s (run with -update to regenerate)", golden)
	}
}

// TestProbe084BuiltContributionBodyCorpusArm proves the shape-witness arm
// bites: a record carrying a version field this SDK cannot emit fails the
// probe rather than passing unnoticed.
func TestProbe084BuiltContributionBodyCorpusArm(t *testing.T) {
	var captured []byte
	srv := contributionCommitServer(&captured, nil)
	defer srv.Close()
	hostile := [][]byte{[]byte(`{"versions":[{"_type":"ORIGINAL_VERSION","invented_field":1}]}`)}
	r, err := probes.Probe084BuiltContributionBody(context.Background(), newClient(t, srv), &captured, ehrIDFixture, hostile)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" {
		t.Errorf("PROBE-084 status = %q, want fail on an unemittable version field (detail=%q)", r.Status, r.Detail)
	}
}

// TestProbe084BuiltContributionBodyFrameworkMisuse separates "could not
// run" from "the builder is wrong" — a missing input or an empty corpus is
// an error, never a passing result (REQ-082).
func TestProbe084BuiltContributionBodyFrameworkMisuse(t *testing.T) {
	var captured []byte
	srv := contributionCommitServer(&captured, nil)
	defer srv.Close()
	corpus := submissionCorpus(t)
	if _, err := probes.Probe084BuiltContributionBody(context.Background(), nil, &captured, ehrIDFixture, corpus); err == nil {
		t.Error("nil client: expected an error")
	}
	if _, err := probes.Probe084BuiltContributionBody(context.Background(), newClient(t, srv), nil, ehrIDFixture, corpus); err == nil {
		t.Error("nil recorder: expected an error")
	}
	if _, err := probes.Probe084BuiltContributionBody(context.Background(), newClient(t, srv), &captured, "", corpus); err == nil {
		t.Error("empty ehr id: expected an error")
	}
	if _, err := probes.Probe084BuiltContributionBody(context.Background(), newClient(t, srv), &captured, ehrIDFixture, nil); err == nil {
		t.Error("empty corpus: expected an error")
	}
}
