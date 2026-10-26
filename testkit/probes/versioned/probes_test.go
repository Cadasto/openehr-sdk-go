package versionedprobes_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/versioned"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const (
	ehrIDFixture    openehrclient.EHRID             = "bf0b2ad8-7b0e-4f4d-9d33-6a8de69f0a64"
	compositionVOID openehrclient.VersionedObjectID = "1234abcd-5678-9012-3456-7890abcdef00"
	initialVUID     openehrclient.VersionUID        = "1234abcd-5678-9012-3456-7890abcdef00::cdr.example::1"
	updatedVUID     openehrclient.VersionUID        = "1234abcd-5678-9012-3456-7890abcdef00::cdr.example::2"
)

func newClient(t *testing.T, b *sandbox.Backend) *transport.Client {
	t.Helper()
	c, err := probe.NewClient("https://sandbox.local/openehr/v1", b.HTTPClient(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestProbe010PutWithoutIfMatch(t *testing.T) {
	// PROBE-010 is a compile-time guard exercise — no network needed.
	// Construct a client against any throwaway backend.
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		t.Error("PROBE-010 must short-circuit before any network call")
	})
	r, err := probes.Probe010PutWithoutIfMatch(context.Background(), newClient(t, b), ehrIDFixture)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-010 status = %q (detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe011PutStaleIfMatch_412(t *testing.T) {
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
		_, _ = w.Write([]byte(`{"message":"stale","code":"PRECONDITION_FAILED"}`))
	})
	r, err := probes.Probe011PutStaleIfMatch(context.Background(), newClient(t, b), ehrIDFixture, compositionVOID, "stale", &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-011 (412) status = %q (detail: %s)", r.Status, r.Detail)
	}
}

// TestProbe011PutStaleIfMatch_409Fails pins PROBE-011 to the ITS-REST pin:
// a stale If-Match on a PUT is a 412, so a server that answers 409 fails
// the probe.
func TestProbe011PutStaleIfMatch_409Fails(t *testing.T) {
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"stale","code":"VERSION_CONFLICT"}`))
	})
	r, err := probes.Probe011PutStaleIfMatch(context.Background(), newClient(t, b), ehrIDFixture, compositionVOID, "stale", &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" {
		t.Errorf("PROBE-011 (409) status = %q (detail: %s), want fail", r.Status, r.Detail)
	}
}

func TestProbe012ETagRoundTrip(t *testing.T) {
	// Phase 1 of the probe: a GET returns the initial VersionUID via
	// Location; phase 2: PUT carries that as If-Match and the server
	// returns a fresh VersionUID. The fake server below alternates
	// between the two phases on a single shared state.
	var phase int
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
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
	})

	r, err := probes.Probe012ETagRoundTrip(context.Background(), newClient(t, b), ehrIDFixture, compositionVOID, &rm.Composition{})
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
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(initialVUID))
		w.Header().Set("ETag", `"`+string(initialVUID)+`"`)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(bareCompositionBody))
	})

	r, err := probes.Probe071CompositionWriteResponseShape(context.Background(), newClient(t, b), ehrIDFixture, "", "", &rm.Composition{})
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
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
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
	})

	r, err := probes.Probe071CompositionWriteResponseShape(context.Background(), newClient(t, b), ehrIDFixture, compositionVOID, string(initialVUID), &rm.Composition{})
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
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(initialVUID))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"_type":"ORIGINAL_VERSION","uid":{"_type":"OBJECT_VERSION_ID","value":"x::y::1"},"data":{"_type":"COMPOSITION","name":{"_type":"DV_TEXT","value":"x"}}}`))
	})

	r, err := probes.Probe071CompositionWriteResponseShape(context.Background(), newClient(t, b), ehrIDFixture, "", "", &rm.Composition{})
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
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
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
	})

	r, err := probes.Probe071CompositionWriteResponseShape(context.Background(), newClient(t, b), ehrIDFixture, compositionVOID, string(initialVUID), &rm.Composition{})
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
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = b
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/contribution/cont-1")
		w.WriteHeader(http.StatusCreated)
	})
	ov := newOriginalVersionFixture()
	sub := &contribution.Submission{
		Audit:    ov.CommitAudit,
		Versions: []contribution.CommitVersion{ov},
	}
	r, err := probes.Probe072ContributionSubmissionShape(context.Background(), newClient(t, b), &capturedBody, ehrIDFixture, sub)
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
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		captured = planted
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/contribution/cont-1")
		w.WriteHeader(http.StatusCreated)
	})
	ov := newOriginalVersionFixture()
	sub := &contribution.Submission{
		Audit:    ov.CommitAudit,
		Versions: []contribution.CommitVersion{ov},
	}
	r, err := probes.Probe072ContributionSubmissionShape(context.Background(), newClient(t, b), &captured, ehrIDFixture, sub)
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
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		captured = planted
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/contribution/cont-1")
		w.WriteHeader(http.StatusCreated)
	})
	ov := newOriginalVersionFixture()
	sub := &contribution.Submission{Audit: ov.CommitAudit, Versions: []contribution.CommitVersion{ov}}
	r, err := probes.Probe072ContributionSubmissionShape(context.Background(), newClient(t, b), &captured, ehrIDFixture, sub)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" || !strings.Contains(r.Detail, "time_committed") {
		t.Errorf("PROBE-072 status=%q detail=%q (want fail mentioning time_committed)", r.Status, r.Detail)
	}
}

// probe072DeletionSubmission builds, through the public builder, a batch of a
// creation (with its payload) and a deletion given no payload — the one
// version REQ-130 § Deletion lets carry no `data` member, and the exception
// PROBE-072 makes to its inline-`data` rule. The batch audit names `unknown`
// (253), a code neither version carries, so the audit shape is the builder's
// own rather than a copy of either version.
func probe072DeletionSubmission(t *testing.T) *contribution.Submission {
	t.Helper()
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	sub, err := contribution.NewBuilder().
		WithCommitterName("probe-072").
		WithChangeType(contribution.ChangeTypeUnknown).
		Add(
			contribution.Creation(&comp),
			contribution.Deletion[rm.Composition]("8849182c-82ad-4088-a07f-48ead4180515::cdr.example::1", nil),
		).
		Build()
	if err != nil {
		t.Fatalf("build the PROBE-072 deletion batch: %v", err)
	}
	return sub
}

// TestProbe072AcceptsDeletionWithoutPayload is the PROBE-072 arm for REQ-130
// § Deletion: a real body, built and sent through the client, whose deletion
// carries no `data` member must pass instead of failing "data missing".
func TestProbe072AcceptsDeletionWithoutPayload(t *testing.T) {
	var captured []byte
	b := contributionCommitBackend(&captured, nil)
	r, err := probes.Probe072ContributionSubmissionShape(context.Background(), newClient(t, b), &captured, ehrIDFixture, probe072DeletionSubmission(t))
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-072 status = %q, want pass for a deletion built without a payload (detail=%q)", r.Status, r.Detail)
	}
	// The pass is only worth anything if the body really is the data-less
	// shape: key presence, since `"data":null` would also read as no payload.
	var body struct {
		Versions []map[string]json.RawMessage `json:"versions"`
	}
	if err := json.Unmarshal(captured, &body); err != nil {
		t.Fatalf("captured body is not JSON: %v", err)
	}
	if len(body.Versions) != 2 {
		t.Fatalf("captured body has %d versions, want 2 (a creation and a deletion)", len(body.Versions))
	}
	if _, has := body.Versions[0]["data"]; !has {
		t.Errorf("the creation lost its inline payload: %s", captured)
	}
	if raw, has := body.Versions[1]["data"]; has {
		t.Errorf("the deletion built without a payload carries data %s, want no data member", raw)
	}
	if !strings.Contains(r.Detail, "without a payload") {
		t.Errorf("PROBE-072 pass detail = %q, want it to name the deletion built without a payload", r.Detail)
	}
}

// probe072Version is one `Contribution_create` version as the builder emits
// it, so a planted variant differs in exactly the member under test: typ is
// its `_type`, code its commit_audit change-type code, data the raw JSON of
// its `data` member (empty leaves the member out), and auditExtra extra
// members spliced into the commit_audit.
func probe072Version(typ, code, data, auditExtra string) string {
	s := `{"_type":"` + typ + `","commit_audit":{"_type":"AUDIT_DETAILS"` + auditExtra +
		`,"change_type":{"_type":"DV_CODED_TEXT","defining_code":{"_type":"CODE_PHRASE","code_string":"` + code + `"}}}`
	if data != "" {
		s += `,"data":` + data
	}
	return s + "}"
}

// probe072Body wraps versions in a conformant batch audit.
func probe072Body(versions ...string) []byte {
	return []byte(`{"audit":{"_type":"AUDIT_DETAILS","change_type":{"_type":"DV_CODED_TEXT","defining_code":{"_type":"CODE_PHRASE","code_string":"253"}}},"versions":[` +
		strings.Join(versions, ",") + `]}`)
}

// probe072PlantDataNull takes the real body the builder sends and rewrites
// versions[i].data to a present-but-null member — what a builder that dropped
// `omitempty` from the payload would emit.
func probe072PlantDataNull(t *testing.T, i int) []byte {
	t.Helper()
	var captured []byte
	b := contributionCommitBackend(&captured, nil)
	if _, _, err := contribution.Commit(context.Background(), newClient(t, b), ehrIDFixture, probe072DeletionSubmission(t)); err != nil {
		t.Fatal(err)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(captured, &body); err != nil {
		t.Fatalf("captured body is not JSON: %v", err)
	}
	var versions []map[string]json.RawMessage
	if err := json.Unmarshal(body["versions"], &versions); err != nil {
		t.Fatalf("captured versions[] is not a list of objects: %v", err)
	}
	versions[i]["data"] = json.RawMessage("null")
	raw, err := json.Marshal(versions)
	if err != nil {
		t.Fatal(err)
	}
	body["versions"] = raw
	planted, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return planted
}

// TestProbe072RejectsDeletionExceptionMisuse plants bodies that each cross
// the edge of the PROBE-072 exception for a deletion built without a payload
// (REQ-130 § Deletion), so a probe that let the exception grow, or stopped
// asserting the rest of the shape on such a version, fails here. Each case
// names the substring its Detail must carry: asserting only `status == "fail"`
// would let a case rot into failing for an unrelated reason.
func TestProbe072RejectsDeletionExceptionMisuse(t *testing.T) {
	const (
		payload       = `{"_type":"COMPOSITION"}`
		missingType   = "data missing or has no _type"
		timeCommitted = `,"time_committed":{"value":"2026-01-01T00:00:00Z"}`
	)
	cases := []struct {
		name       string
		planted    []byte
		wantDetail string
	}{
		{
			// The builder's real deletion, its data member turned into null:
			// REQ-130 asks for the member to be left out, so null is a defect
			// even on the one version allowed to carry no payload.
			name:       "deletion built without a payload emits null data",
			planted:    probe072PlantDataNull(t, 1),
			wantDetail: "versions[1].data is null",
		},
		{
			name:       "creation emits null data",
			planted:    probe072PlantDataNull(t, 0),
			wantDetail: "versions[0].data is null",
		},
		{
			name:       "imported version emits null data",
			planted:    probe072Body(probe072Version("IMPORTED_VERSION", "249", "null", "")),
			wantDetail: "versions[0].data is null",
		},
		{
			// The exception is for a version with NO data; an empty object is
			// a payload that lost its `_type`, which the old rule still catches.
			name:       "deletion carries an untyped data object",
			planted:    probe072Body(probe072Version("ORIGINAL_VERSION", "523", `{}`, "")),
			wantDetail: missingType,
		},
		{
			// The exception is keyed on change type 523: an amendment with no
			// payload keeps failing under the existing rule.
			name:       "amendment lacks data",
			planted:    probe072Body(probe072Version("ORIGINAL_VERSION", "250", "", "")),
			wantDetail: missingType,
		},
		{
			name:       "creation lacks data",
			planted:    probe072Body(probe072Version("ORIGINAL_VERSION", "249", "", "")),
			wantDetail: missingType,
		},
		{
			// The exception is for an ORIGINAL_VERSION only; an imported
			// version without data keeps failing even with change type 523.
			name:       "imported deletion lacks data",
			planted:    probe072Body(probe072Version("IMPORTED_VERSION", "523", "", "")),
			wantDetail: missingType,
		},
		{
			// A version with no commit_audit has no change type to key the
			// exception on, so it cannot claim it.
			name:       "version without commit_audit lacks data",
			planted:    probe072Body(`{"_type":"ORIGINAL_VERSION"}`),
			wantDetail: missingType,
		},
		{
			// The rest of the shape is still asserted on the version that
			// carries no payload: its commit_audit write shape.
			name:       "deletion without data carries time_committed",
			planted:    probe072Body(probe072Version("ORIGINAL_VERSION", "523", "", timeCommitted)),
			wantDetail: "time_committed",
		},
		{
			// And the payload a version does carry is still required to be
			// typed on a creation next to the data-less deletion.
			name:       "creation beside a deletion carries an untyped payload",
			planted:    probe072Body(probe072Version("ORIGINAL_VERSION", "249", `{}`, ""), probe072Version("ORIGINAL_VERSION", "523", "", "")),
			wantDetail: "versions[0]." + missingType,
		},
		{
			// A deletion given its payload still passes through the typed rule.
			name:       "deletion with a payload lacking _type",
			planted:    probe072Body(probe072Version("ORIGINAL_VERSION", "249", payload, ""), probe072Version("ORIGINAL_VERSION", "523", `{"name":"x"}`, "")),
			wantDetail: "versions[1]." + missingType,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured []byte
			b := contributionCommitBackend(&captured, tc.planted)
			r, err := probes.Probe072ContributionSubmissionShape(context.Background(), newClient(t, b), &captured, ehrIDFixture, probe072DeletionSubmission(t))
			if err != nil {
				t.Fatal(err)
			}
			if r.Status != "fail" {
				t.Errorf("PROBE-072 status = %q, want fail (detail=%q)", r.Status, r.Detail)
			}
			if !strings.Contains(r.Detail, tc.wantDetail) {
				t.Errorf("PROBE-072 failed for the wrong reason:\n got: %s\nwant it to mention: %s", r.Detail, tc.wantDetail)
			}
		})
	}
}

func TestProbe013CrossEHRIsolation(t *testing.T) {
	const (
		ehrAID          openehrclient.EHRID      = "ehrA-1111-2222-3333-444444444444"
		ehrBID          openehrclient.EHRID      = "ehrB-aaaa-bbbb-cccc-dddddddddddd"
		versionUIDFromA openehrclient.VersionUID = "9999abcd-5678-9012-3456-7890abcdef00::cdr.example::1"
	)
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
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
	})
	r, err := probes.Probe013CrossEHRIsolation(context.Background(), newClient(t, b), ehrAID, ehrBID, versionUIDFromA)
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
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"_type":"COMPOSITION","name":{"_type":"DV_TEXT","value":"leak"},"archetype_node_id":"openEHR-EHR-COMPOSITION.x.v1","language":{"_type":"CODE_PHRASE","code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}},"territory":{"_type":"CODE_PHRASE","code_string":"GB","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"}},"category":{"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}}}`))
	})
	r, err := probes.Probe013CrossEHRIsolation(context.Background(), newClient(t, b), "ehrA", "ehrB", "vuid")
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

// contributionGetBackend answers 200 with a canonical contribution for
// presentUID and 404 for anything else, recording every request.
func contributionGetBackend(t *testing.T, captured *[]*http.Request, presentUID string) *sandbox.Backend {
	t.Helper()
	return sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		*captured = append(*captured, r.Clone(r.Context()))
		if strings.HasSuffix(r.URL.Path, "/contribution/"+presentUID) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(contributionGetBody))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"no such contribution"}`))
	})
}

// REQ-142: PROBE-092 reads a contribution back by id on the contribution
// endpoint.
func TestProbe092ContributionGetPass(t *testing.T) {
	const presentUID = "0826851c-c4c2-4d61-92b9-410fb8275ff0"
	var captured []*http.Request
	b := contributionGetBackend(t, &captured, presentUID)

	r, err := probes.Probe092ContributionGet(context.Background(), newClient(t, b), &captured, ehrIDFixture, presentUID, "missing-uid")
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
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		planted := r.Clone(r.Context())
		planted.Method = http.MethodPost
		captured = append(captured, planted)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(contributionGetBody))
	})

	r, err := probes.Probe092ContributionGet(context.Background(), newClient(t, b), &captured, ehrIDFixture, presentUID, "missing-uid")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" {
		t.Errorf("PROBE-092 status = %q, want fail on a non-GET method (detail=%q)", r.Status, r.Detail)
	}
}

func TestProbe092ContributionGetRejectsMissingInputs(t *testing.T) {
	var captured []*http.Request
	b := contributionGetBackend(t, &captured, "u")
	if _, err := probes.Probe092ContributionGet(context.Background(), nil, &captured, ehrIDFixture, "u", "m"); err == nil {
		t.Error("nil client: expected an error")
	}
	if _, err := probes.Probe092ContributionGet(context.Background(), newClient(t, b), nil, ehrIDFixture, "u", "m"); err == nil {
		t.Error("nil recorder: expected an error")
	}
}

// The PROBE-109 backend routes on full request paths: newClient's service
// base, then the pinned route.
const (
	probe109Base = "/openehr/v1"
	probe109EHR  = probe109Base + "/ehr/" + string(ehrIDFixture)
	probe109VC   = probe109EHR + "/versioned_composition/" + string(compositionVOID)
	probe109VS   = probe109EHR + "/versioned_ehr_status"

	probe109StatusVersionUID   openehrclient.VersionUID        = "f1e2d3c4-b5a6-4978-89ab-cdef01234567::cdr.example::2"
	probe109ImportedVersionUID openehrclient.VersionUID        = "5d6e7f80-91a2-4b3c-8d4e-5f6071829304::imported.example::1"
	probe109PartyVOID          openehrclient.VersionedObjectID = "8849182c-82ad-4088-a07f-48ead4180515"

	// probe109AtTimeWire is the version_at_time every at-time read must
	// send: the pin's example time carried to nanoseconds, in UTC, every
	// fraction digit kept.
	probe109AtTimeWire = "2015-01-20T18:30:22.765432101Z"
	// probe109DroppedFraction is the example time with its fraction of a
	// second cut off: what a truncating formatter would send.
	probe109DroppedFraction = "2015-01-20T18:30:22Z"
	// probe109Milliseconds is the example time cut to milliseconds: what a
	// formatter that keeps three fraction digits would send.
	probe109Milliseconds = "2015-01-20T18:30:22.765Z"
)

func probe109Inputs() probes.Probe109Inputs {
	return probes.Probe109Inputs{
		EHRID:                 ehrIDFixture,
		CompositionVOID:       compositionVOID,
		CompositionVersionUID: updatedVUID,
		StatusVersionUID:      probe109StatusVersionUID,
		ImportedVersionUID:    probe109ImportedVersionUID,
		PartyVOID:             probe109PartyVOID,
	}
}

const (
	probe109AuditJSON = `{"_type":"AUDIT_DETAILS","system_id":"cdr.example",` +
		`"time_committed":{"_type":"DV_DATE_TIME","value":"2015-01-20T18:30:22.765Z"},` +
		`"change_type":{"_type":"DV_CODED_TEXT","value":"modification","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"},"code_string":"251"}},` +
		`"committer":{"_type":"PARTY_IDENTIFIED","name":"Dr Alice"}}`
	probe109ContributionJSON = `{"_type":"OBJECT_REF","id":{"_type":"HIER_OBJECT_ID","value":"5c1a6d0e-2f3b-4c8d-9e7f-0a1b2c3d4e5f"},"namespace":"local","type":"CONTRIBUTION"}`
)

// probe109VersionedObject is a VERSIONED_COMPOSITION or VERSIONED_EHR_STATUS
// (rmType) whose uid is uid.
func probe109VersionedObject(rmType, uid string) string {
	return `{"_type":"` + rmType + `","uid":{"_type":"HIER_OBJECT_ID","value":"` + uid + `"},` +
		`"owner_id":{"_type":"OBJECT_REF","id":{"_type":"HIER_OBJECT_ID","value":"` + string(ehrIDFixture) + `"},"namespace":"local","type":"EHR"},` +
		`"time_created":{"_type":"DV_DATE_TIME","value":"2015-01-20T18:30:22.765Z"}}`
}

// probe109RevisionHistory is a REVISION_HISTORY with one item per version id.
func probe109RevisionHistory(versionUIDs ...string) string {
	items := make([]string, 0, len(versionUIDs))
	for _, uid := range versionUIDs {
		items = append(items, `{"_type":"REVISION_HISTORY_ITEM","version_id":{"_type":"OBJECT_VERSION_ID","value":"`+uid+`"},"audits":[`+probe109AuditJSON+`]}`)
	}
	return `{"_type":"REVISION_HISTORY","items":[` + strings.Join(items, ",") + `]}`
}

// probe109Version is a complete (code 532) ORIGINAL_VERSION whose uid is uid
// and whose data is the raw JSON data, or that carries no data when data is
// empty. probe109DeletedVersion is the deleted one without data.
func probe109Version(uid, data string) string {
	v := `{"_type":"ORIGINAL_VERSION","uid":{"_type":"OBJECT_VERSION_ID","value":"` + uid + `"},` +
		`"lifecycle_state":{"_type":"DV_CODED_TEXT","value":"complete","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"},"code_string":"532"}},` +
		`"commit_audit":` + probe109AuditJSON + `,"contribution":` + probe109ContributionJSON
	if data != "" {
		v += `,"data":` + data
	}
	return v + `}`
}

// probe109DeletedVersion is the ORIGINAL_VERSION of a logical deletion whose
// uid is uid: its lifecycle state and its change type are both deleted (code
// 523), and it carries no data.
func probe109DeletedVersion(uid string) string {
	const deleted = `{"_type":"DV_CODED_TEXT","value":"deleted","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"},"code_string":"523"}}`
	return `{"_type":"ORIGINAL_VERSION","uid":{"_type":"OBJECT_VERSION_ID","value":"` + uid + `"},` +
		`"lifecycle_state":` + deleted + `,` +
		`"commit_audit":{"_type":"AUDIT_DETAILS","system_id":"cdr.example",` +
		`"time_committed":{"_type":"DV_DATE_TIME","value":"2015-01-20T18:30:22.765Z"},` +
		`"change_type":` + deleted + `,"committer":{"_type":"PARTY_IDENTIFIED","name":"Dr Alice"}},` +
		`"contribution":` + probe109ContributionJSON + `}`
}

// probe109Imported is an IMPORTED_VERSION wrapping the version item: the
// other VERSION the pin's version schemas admit.
func probe109Imported(item string) string {
	return `{"_type":"IMPORTED_VERSION","commit_audit":` + probe109AuditJSON +
		`,"contribution":` + probe109ContributionJSON + `,"item":` + item + `}`
}

// probe109Corpus holds the payloads PROBE-109's backend serves, read from
// the vendored corpus.
type probe109Corpus struct {
	composition, status, folder, partyVersion string
}

func probe109ReadCorpus(t *testing.T) probe109Corpus {
	t.Helper()
	read := func(path string) string {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read PROBE-109 corpus fixture: %v", err)
		}
		return string(b)
	}
	itsREST := filepath.Join(fixtures.CorpusRoot(), "its_rest")
	return probe109Corpus{
		composition:  read(fixtures.CompositionJSON("minimal_evaluation.en.v1")),
		status:       read(filepath.Join(itsREST, "ehr", "ehr_status.json")),
		folder:       read(filepath.Join(itsREST, "ehr", "folder.json")),
		partyVersion: read(filepath.Join(itsREST, "demographic", "original_version.json")),
	}
}

// probe109Routes maps each full request path PROBE-109 reads to the 200 body
// a conformant server answers it with.
func probe109Routes(t *testing.T) map[string]string {
	t.Helper()
	c := probe109ReadCorpus(t)
	compositionVersion := probe109Version(string(updatedVUID), c.composition)
	statusVersion := probe109Version(string(probe109StatusVersionUID), c.status)
	return map[string]string{
		// The composition family's versioned-object reads.
		probe109VC:                                                    probe109VersionedObject("VERSIONED_COMPOSITION", string(compositionVOID)),
		probe109VC + "/revision_history":                              probe109RevisionHistory(string(initialVUID), string(updatedVUID)),
		probe109VC + "/version":                                       compositionVersion,
		probe109VC + "/version/" + string(updatedVUID):                compositionVersion,
		probe109VC + "/version/" + string(probe109ImportedVersionUID): probe109Imported(compositionVersion),

		// The EHR_STATUS family's versioned-object reads.
		probe109VS:                       probe109VersionedObject("VERSIONED_EHR_STATUS", "f1e2d3c4-b5a6-4978-89ab-cdef01234567"),
		probe109VS + "/revision_history": probe109RevisionHistory(string(probe109StatusVersionUID)),
		probe109VS + "/version":          statusVersion,
		probe109VS + "/version/" + string(probe109StatusVersionUID):   statusVersion,
		probe109VS + "/version/" + string(probe109ImportedVersionUID): probe109Imported(statusVersion),

		// The other leaves' at-time reads.
		probe109EHR + "/composition/" + string(compositionVOID): c.composition,
		probe109EHR + "/ehr_status":                             c.status,
		probe109EHR + "/directory":                              c.folder,
		probe109Base + "/demographic/versioned_party/" + string(probe109PartyVOID) + "/version": c.partyVersion,
	}
}

// probe109Match picks the request a plant applies to: its full path, and
// whether it carries version_at_time (the latest-version and version-at-time
// reads share a path).
type probe109Match struct {
	path   string
	atTime bool
}

func (m probe109Match) hits(r *http.Request) bool {
	return m.path != "" && r.URL.Path == m.path && r.URL.Query().Has("version_at_time") == m.atTime
}

// probe109Plant changes what the PROBE-109 backend does for the one request
// on matches: record rewrites the copy the probe reads, body and status
// replace the response, and twice records the request a second time.
type probe109Plant struct {
	on     probe109Match
	record func(*http.Request)
	body   string
	status int
	twice  bool
}

// probe109Backend serves probe109Routes, recording every request in
// captured, with plant applied.
func probe109Backend(t *testing.T, captured *[]*http.Request, plant probe109Plant) *sandbox.Backend {
	t.Helper()
	routes := probe109Routes(t)
	return sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		hit := plant.on.hits(r)
		rec := r.Clone(r.Context())
		if hit && plant.record != nil {
			plant.record(rec)
		}
		*captured = append(*captured, rec)
		if hit && plant.twice {
			*captured = append(*captured, rec)
		}
		body, ok := routes[r.URL.Path]
		if !ok {
			t.Errorf("PROBE-109 backend: no route for %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		status := http.StatusOK
		if hit && plant.status != 0 {
			status = plant.status
		}
		if hit && plant.body != "" {
			body = plant.body
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
}

// TestProbe109VersionedObjectReadsPass runs PROBE-109 (REQ-174) against a
// conformant backend: the ten versioned-object reads, the two IMPORTED_VERSION
// refusals and the four at-time reads on the other leaves all pass, and the
// six at-time reads send the example time with all nine fraction digits.
func TestProbe109VersionedObjectReadsPass(t *testing.T) {
	var captured []*http.Request
	b := probe109Backend(t, &captured, probe109Plant{})
	r, err := probes.Probe109VersionedObjectReads(t.Context(), newClient(t, b), &captured, probe109Inputs())
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Fatalf("PROBE-109 status = %q, want pass (detail=%q)", r.Status, r.Detail)
	}
	if got, want := len(captured), 16; got != want {
		t.Errorf("PROBE-109 issued %d requests, want %d (10 versioned-object reads, 2 refusals, 4 at-time reads)", got, want)
	}
	// The probe's example time must hold more digits than milliseconds, or a
	// leaf that cuts the fraction to three digits would pass it.
	var atTimes []string
	for _, req := range captured {
		atTimes = append(atTimes, req.URL.Query()["version_at_time"]...)
	}
	if want := slices.Repeat([]string{probe109AtTimeWire}, 6); !slices.Equal(atTimes, want) {
		t.Errorf("version_at_time values sent = %q, want %q (the example time to nanoseconds, once per at-time read)", atTimes, want)
	}
}

// TestProbe109OtherLeafRowsJudgeTheRequestOnly pins PROBE-109 (REQ-174): the
// four at-time reads on the other leaves are judged on their request, not on
// the read's result, so a read that fails on a well-formed request passes.
// The plant "an at-time read on another leaf fails and cuts the fraction"
// holds the other half: the request is still judged when the read fails.
func TestProbe109OtherLeafRowsJudgeTheRequestOnly(t *testing.T) {
	notFound := `{"message":"not found"}`
	rows := map[string]probe109Match{
		"composition.Get(LatestAtTime)": {path: probe109EHR + "/composition/" + string(compositionVOID), atTime: true},
		"ehrstatus.GetAtTime":           {path: probe109EHR + "/ehr_status", atTime: true},
		"directory.GetAtTime":           {path: probe109EHR + "/directory", atTime: true},
		"demographic.GetVersionAtTime":  {path: probe109Base + "/demographic/versioned_party/" + string(probe109PartyVOID) + "/version", atTime: true},
	}
	for name, on := range rows {
		t.Run(name, func(t *testing.T) {
			var captured []*http.Request
			b := probe109Backend(t, &captured, probe109Plant{on: on, status: http.StatusNotFound, body: notFound})
			r, err := probes.Probe109VersionedObjectReads(t.Context(), newClient(t, b), &captured, probe109Inputs())
			if err != nil {
				t.Fatal(err)
			}
			if r.Status != "pass" {
				t.Errorf("PROBE-109 with %s answered 404 = %q, want pass: the row judges the request only (detail=%q)", name, r.Status, r.Detail)
			}
			if got, want := len(captured), 16; got != want {
				t.Errorf("PROBE-109 issued %d requests, want %d", got, want)
			}
		})
	}
}

// TestProbe109PassesAVersionWithoutData pins PROBE-109 (REQ-174): a deleted
// version (lifecycle state 523) that carries no data is the RM's logical
// deletion, which a version read returns as no payload and no error, so the
// probe passes it. A version that is not deleted and carries no data still
// fails; see the plants. Each case serves the deletion on one of the six
// version reads; the served count shows the deletion reached the read, so a
// plant that matched nothing cannot pass for it.
func TestProbe109PassesAVersionWithoutData(t *testing.T) {
	compositionDeleted := probe109DeletedVersion(string(updatedVUID))
	statusDeleted := probe109DeletedVersion(string(probe109StatusVersionUID))
	cases := []struct {
		read string
		on   probe109Match
		body string
	}{
		{read: "composition.GetVersion", on: probe109Match{path: probe109VC + "/version"}, body: compositionDeleted},
		{read: "composition.GetVersionAtTime", on: probe109Match{path: probe109VC + "/version", atTime: true}, body: compositionDeleted},
		{read: "composition.GetVersionByID", on: probe109Match{path: probe109VC + "/version/" + string(updatedVUID)}, body: compositionDeleted},
		{read: "ehrstatus.GetVersion", on: probe109Match{path: probe109VS + "/version"}, body: statusDeleted},
		{read: "ehrstatus.GetVersionAtTime", on: probe109Match{path: probe109VS + "/version", atTime: true}, body: statusDeleted},
		{read: "ehrstatus.GetVersionByID", on: probe109Match{path: probe109VS + "/version/" + string(probe109StatusVersionUID)}, body: statusDeleted},
	}
	for _, tc := range cases {
		t.Run(tc.read, func(t *testing.T) {
			var captured []*http.Request
			served := 0
			plant := probe109Plant{on: tc.on, body: tc.body, record: func(*http.Request) { served++ }}
			b := probe109Backend(t, &captured, plant)
			r, err := probes.Probe109VersionedObjectReads(t.Context(), newClient(t, b), &captured, probe109Inputs())
			if err != nil {
				t.Fatal(err)
			}
			if served != 1 {
				t.Fatalf("the deletion was served %d time(s), want 1 (the plant does not match the %s request?)", served, tc.read)
			}
			if r.Status != "pass" {
				t.Errorf("PROBE-109 with %s answered by a version without data = %q, want pass (detail=%q)", tc.read, r.Status, r.Detail)
			}
		})
	}
}

// TestProbe109VersionedObjectReadsFlagsPlants proves each PROBE-109 (REQ-174)
// assertion can fail: every case plants one defect on a recorded request or a
// served body, and the probe must report fail naming it. Asserting only
// `status == "fail"` would let a case rot into failing for an unrelated reason.
func TestProbe109VersionedObjectReadsFlagsPlants(t *testing.T) {
	setMethod := func(m string) func(*http.Request) { return func(r *http.Request) { r.Method = m } }
	setPath := func(p string) func(*http.Request) { return func(r *http.Request) { r.URL.Path = p } }
	setAccept := func(a string) func(*http.Request) { return func(r *http.Request) { r.Header.Set("Accept", a) } }
	setAtTime := func(v string) func(*http.Request) {
		return func(r *http.Request) { r.URL.RawQuery = url.Values{"version_at_time": {v}}.Encode() }
	}
	partyVersion := probe109Base + "/demographic/versioned_party/" + string(probe109PartyVOID) + "/version"

	cases := []struct {
		name       string
		plant      probe109Plant
		wantDetail string
	}{
		{
			name:       "wrong method on the versioned-object read",
			plant:      probe109Plant{on: probe109Match{path: probe109VC}, record: setMethod(http.MethodPost)},
			wantDetail: `composition.GetVersionedObject: method = "POST", want GET`,
		},
		{
			name:       "wrong path on the revision-history read",
			plant:      probe109Plant{on: probe109Match{path: probe109VC + "/revision_history"}, record: setPath(probe109VC + "/revisions")},
			wantDetail: "composition.GetRevisionHistory: path = ",
		},
		{
			name:       "wrong path on the EHR_STATUS version-by-id read",
			plant:      probe109Plant{on: probe109Match{path: probe109VS + "/version/" + string(probe109StatusVersionUID)}, record: setPath(probe109EHR + "/ehr_status/" + string(probe109StatusVersionUID))},
			wantDetail: "ehrstatus.GetVersionByID: path = ",
		},
		{
			name:       "Accept other than canonical JSON",
			plant:      probe109Plant{on: probe109Match{path: probe109VS + "/version/" + string(probe109StatusVersionUID)}, record: setAccept("application/xml")},
			wantDetail: "ehrstatus.GetVersionByID: Accept = ",
		},
		{
			name:       "two requests for one read",
			plant:      probe109Plant{on: probe109Match{path: probe109VS}, twice: true},
			wantDetail: "ehrstatus.GetVersionedObject issued 2 request(s), want exactly 1",
		},
		{
			name:       "composition latest-version read sends version_at_time",
			plant:      probe109Plant{on: probe109Match{path: probe109VC + "/version"}, record: setAtTime(probe109AtTimeWire)},
			wantDetail: "composition.GetVersion: sent version_at_time",
		},
		{
			name:       "EHR_STATUS latest-version read sends version_at_time",
			plant:      probe109Plant{on: probe109Match{path: probe109VS + "/version"}, record: setAtTime(probe109AtTimeWire)},
			wantDetail: "ehrstatus.GetVersion: sent version_at_time",
		},
		{
			name:       "composition version-at-time read drops the fraction",
			plant:      probe109Plant{on: probe109Match{path: probe109VC + "/version", atTime: true}, record: setAtTime(probe109DroppedFraction)},
			wantDetail: "composition.GetVersionAtTime: version_at_time = ",
		},
		{
			name:       "EHR_STATUS version-at-time read drops the fraction",
			plant:      probe109Plant{on: probe109Match{path: probe109VS + "/version", atTime: true}, record: setAtTime(probe109DroppedFraction)},
			wantDetail: "ehrstatus.GetVersionAtTime: version_at_time = ",
		},
		{
			name:       "composition version-at-time read cuts the fraction to milliseconds",
			plant:      probe109Plant{on: probe109Match{path: probe109VC + "/version", atTime: true}, record: setAtTime(probe109Milliseconds)},
			wantDetail: "composition.GetVersionAtTime: version_at_time = ",
		},
		{
			name:       "EHR_STATUS version-at-time read cuts the fraction to milliseconds",
			plant:      probe109Plant{on: probe109Match{path: probe109VS + "/version", atTime: true}, record: setAtTime(probe109Milliseconds)},
			wantDetail: "ehrstatus.GetVersionAtTime: version_at_time = ",
		},
		{
			name:       "composition read by a Ref with a time drops the fraction",
			plant:      probe109Plant{on: probe109Match{path: probe109EHR + "/composition/" + string(compositionVOID), atTime: true}, record: setAtTime(probe109DroppedFraction)},
			wantDetail: "composition.Get(LatestAtTime): version_at_time = ",
		},
		{
			name:       "EHR_STATUS read at a time drops the fraction",
			plant:      probe109Plant{on: probe109Match{path: probe109EHR + "/ehr_status", atTime: true}, record: setAtTime(probe109DroppedFraction)},
			wantDetail: "ehrstatus.GetAtTime: version_at_time = ",
		},
		{
			name:       "directory read at a time drops the fraction",
			plant:      probe109Plant{on: probe109Match{path: probe109EHR + "/directory", atTime: true}, record: setAtTime(probe109DroppedFraction)},
			wantDetail: "directory.GetAtTime: version_at_time = ",
		},
		{
			name:       "VERSIONED_PARTY version-at-time read drops the fraction",
			plant:      probe109Plant{on: probe109Match{path: partyVersion, atTime: true}, record: setAtTime(probe109DroppedFraction)},
			wantDetail: "demographic.GetVersionAtTime: version_at_time = ",
		},
		{
			name: "version-at-time read sends the value twice",
			plant: probe109Plant{on: probe109Match{path: probe109VC + "/version", atTime: true}, record: func(r *http.Request) {
				r.URL.RawQuery = url.Values{"version_at_time": {probe109AtTimeWire, probe109AtTimeWire}}.Encode()
			}},
			wantDetail: "composition.GetVersionAtTime: version_at_time = ",
		},
		{
			name:       "versioned object without a uid",
			plant:      probe109Plant{on: probe109Match{path: probe109VC}, body: probe109VersionedObject("VERSIONED_COMPOSITION", "")},
			wantDetail: "composition.GetVersionedObject: the body did not decode to a VERSIONED_COMPOSITION with a uid",
		},
		{
			name:       "EHR_STATUS versioned object without a uid",
			plant:      probe109Plant{on: probe109Match{path: probe109VS}, body: probe109VersionedObject("VERSIONED_EHR_STATUS", "")},
			wantDetail: "ehrstatus.GetVersionedObject: the body did not decode to a VERSIONED_EHR_STATUS with a uid",
		},
		{
			name:       "revision history without items",
			plant:      probe109Plant{on: probe109Match{path: probe109VS + "/revision_history"}, body: probe109RevisionHistory()},
			wantDetail: "ehrstatus.GetRevisionHistory: the body did not decode to a REVISION_HISTORY with items",
		},
		{
			name:       "composition revision history without items",
			plant:      probe109Plant{on: probe109Match{path: probe109VC + "/revision_history"}, body: probe109RevisionHistory()},
			wantDetail: "composition.GetRevisionHistory: the body did not decode to a REVISION_HISTORY with items",
		},
		{
			name:       "version without a uid",
			plant:      probe109Plant{on: probe109Match{path: probe109VC + "/version/" + string(updatedVUID)}, body: probe109Version("", bareCompositionBody)},
			wantDetail: "composition.GetVersionByID: the body did not decode to an ORIGINAL_VERSION with a uid",
		},
		{
			name:       "EHR_STATUS version without a uid",
			plant:      probe109Plant{on: probe109Match{path: probe109VS + "/version", atTime: true}, body: probe109Version("", probe109ReadCorpus(t).status)},
			wantDetail: "ehrstatus.GetVersionAtTime: the body did not decode to an ORIGINAL_VERSION with a uid",
		},
		{
			// A version that carries data must carry the family's type: the
			// leaf's decode refuses any other, and the probe fails the read.
			name:       "composition version whose data is an EHR_STATUS",
			plant:      probe109Plant{on: probe109Match{path: probe109VC + "/version"}, body: probe109Version(string(updatedVUID), probe109ReadCorpus(t).status)},
			wantDetail: "composition.GetVersion failed: transport: decode GET ",
		},
		{
			name:       "EHR_STATUS version whose data is a COMPOSITION",
			plant:      probe109Plant{on: probe109Match{path: probe109VS + "/version", atTime: true}, body: probe109Version(string(probe109StatusVersionUID), bareCompositionBody)},
			wantDetail: "ehrstatus.GetVersionAtTime failed: transport: decode GET ",
		},
		{
			// Only a deleted version may carry no data: a complete one
			// without data is what a leaf that drops the data it was served
			// returns, so the probe fails it.
			name:       "composition version that is not deleted carries no data",
			plant:      probe109Plant{on: probe109Match{path: probe109VC + "/version"}, body: probe109Version(string(updatedVUID), "")},
			wantDetail: `composition.GetVersion: the version is not deleted (lifecycle "532") but carried no COMPOSITION`,
		},
		{
			name:       "EHR_STATUS version that is not deleted carries no data",
			plant:      probe109Plant{on: probe109Match{path: probe109VS + "/version/" + string(probe109StatusVersionUID)}, body: probe109Version(string(probe109StatusVersionUID), "")},
			wantDetail: `ehrstatus.GetVersionByID: the version is not deleted (lifecycle "532") but carried no EHR_STATUS`,
		},
		{
			name:       "a versioned-object read fails",
			plant:      probe109Plant{on: probe109Match{path: probe109VS + "/revision_history"}, status: http.StatusInternalServerError, body: `{"message":"boom"}`},
			wantDetail: "ehrstatus.GetRevisionHistory failed: ",
		},
		{
			// The row judges the request only, so the read failing does not
			// fail the probe, and does not spare the request either.
			name:       "an at-time read on another leaf fails and cuts the fraction",
			plant:      probe109Plant{on: probe109Match{path: probe109EHR + "/directory", atTime: true}, status: http.StatusNotFound, body: `{"message":"not found"}`, record: setAtTime(probe109Milliseconds)},
			wantDetail: "directory.GetAtTime: version_at_time = ",
		},
		{
			name:       "composition refusal row sends another method",
			plant:      probe109Plant{on: probe109Match{path: probe109VC + "/version/" + string(probe109ImportedVersionUID)}, record: setMethod(http.MethodPost)},
			wantDetail: `composition.GetVersionByID on an IMPORTED_VERSION: method = "POST", want GET`,
		},
		{
			name:       "composition refusal row sends another path",
			plant:      probe109Plant{on: probe109Match{path: probe109VC + "/version/" + string(probe109ImportedVersionUID)}, record: setPath(probe109VC + "/version")},
			wantDetail: "composition.GetVersionByID on an IMPORTED_VERSION: path = ",
		},
		{
			name:       "composition refusal row sends version_at_time",
			plant:      probe109Plant{on: probe109Match{path: probe109VC + "/version/" + string(probe109ImportedVersionUID)}, record: setAtTime("x")},
			wantDetail: "composition.GetVersionByID on an IMPORTED_VERSION: sent version_at_time",
		},
		{
			name:       "EHR_STATUS refusal row asks for XML",
			plant:      probe109Plant{on: probe109Match{path: probe109VS + "/version/" + string(probe109ImportedVersionUID)}, record: setAccept("application/xml")},
			wantDetail: "ehrstatus.GetVersionByID on an IMPORTED_VERSION: Accept = ",
		},
		{
			name:       "EHR_STATUS refusal row issues two requests",
			plant:      probe109Plant{on: probe109Match{path: probe109VS + "/version/" + string(probe109ImportedVersionUID)}, twice: true},
			wantDetail: "ehrstatus.GetVersionByID on an IMPORTED_VERSION issued 2 request(s), want exactly 1",
		},
		{
			name:       "ORIGINAL_VERSION served on the composition refusal row",
			plant:      probe109Plant{on: probe109Match{path: probe109VC + "/version/" + string(probe109ImportedVersionUID)}, body: probe109Version(string(probe109ImportedVersionUID), bareCompositionBody)},
			wantDetail: "composition.GetVersionByID on an IMPORTED_VERSION succeeded, want a typed decode error",
		},
		{
			name:       "ORIGINAL_VERSION served on the EHR_STATUS refusal row",
			plant:      probe109Plant{on: probe109Match{path: probe109VS + "/version/" + string(probe109ImportedVersionUID)}, body: probe109Version(string(probe109ImportedVersionUID), probe109ReadCorpus(t).status)},
			wantDetail: "ehrstatus.GetVersionByID on an IMPORTED_VERSION succeeded, want a typed decode error",
		},
		{
			// A refusal must be the typed decode failure, not any error: a 404
			// errors too, but as a wire error.
			name:       "refusal row fails with an error that is not a decode error",
			plant:      probe109Plant{on: probe109Match{path: probe109VS + "/version/" + string(probe109ImportedVersionUID)}, status: http.StatusNotFound, body: `{"message":"not found"}`},
			wantDetail: "ehrstatus.GetVersionByID on an IMPORTED_VERSION failed with an error that is not a *transport.DecodeError",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured []*http.Request
			b := probe109Backend(t, &captured, tc.plant)
			r, err := probes.Probe109VersionedObjectReads(t.Context(), newClient(t, b), &captured, probe109Inputs())
			if err != nil {
				t.Fatal(err)
			}
			if r.Status != "fail" {
				t.Errorf("PROBE-109 status = %q, want fail (detail=%q)", r.Status, r.Detail)
			}
			if !strings.Contains(r.Detail, tc.wantDetail) {
				t.Errorf("PROBE-109 failed for the wrong reason:\n got: %s\nwant it to mention: %s", r.Detail, tc.wantDetail)
			}
		})
	}
}

// TestProbe109VersionedObjectReadsRejectsMissingInputs: PROBE-109 (REQ-174)
// returns an error, never a result, when it cannot run.
func TestProbe109VersionedObjectReadsRejectsMissingInputs(t *testing.T) {
	var captured []*http.Request
	c := newClient(t, probe109Backend(t, &captured, probe109Plant{}))
	if _, err := probes.Probe109VersionedObjectReads(t.Context(), nil, &captured, probe109Inputs()); err == nil {
		t.Error("nil client: expected an error")
	}
	if _, err := probes.Probe109VersionedObjectReads(t.Context(), c, nil, probe109Inputs()); err == nil {
		t.Error("nil recorder: expected an error")
	}
	blank := map[string]func(*probes.Probe109Inputs){
		"EHRID":                 func(in *probes.Probe109Inputs) { in.EHRID = "" },
		"CompositionVOID":       func(in *probes.Probe109Inputs) { in.CompositionVOID = "" },
		"CompositionVersionUID": func(in *probes.Probe109Inputs) { in.CompositionVersionUID = "" },
		"StatusVersionUID":      func(in *probes.Probe109Inputs) { in.StatusVersionUID = "" },
		"ImportedVersionUID":    func(in *probes.Probe109Inputs) { in.ImportedVersionUID = "" },
		"PartyVOID":             func(in *probes.Probe109Inputs) { in.PartyVOID = "" },
	}
	for field, blankOut := range blank {
		in := probe109Inputs()
		blankOut(&in)
		if _, err := probes.Probe109VersionedObjectReads(t.Context(), c, &captured, in); err == nil {
			t.Errorf("empty %s: expected an error", field)
		}
	}

	// The probe takes the base path from the client's openEHR REST service,
	// so a catalog without that service, or with one that has no base URL,
	// cannot run it.
	noREST, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer:   "https://sandbox.local",
		Services: map[string]discovery.ServiceEntry{},
	})
	if err != nil {
		t.Fatalf("NewStaticCatalog rejected an empty services map (setup change?): %v", err)
	}
	noBaseURL := &discovery.ServiceCatalog{
		Issuer:   "https://sandbox.local",
		Services: map[string]discovery.ServiceEntry{discovery.ServiceIDOpenEHRRest: {ID: discovery.ServiceIDOpenEHRRest}},
	}
	catalogs := map[string]*discovery.ServiceCatalog{
		"catalog without the openEHR REST service": noREST,
		"openEHR REST service without a base URL":  noBaseURL,
	}
	for name, cat := range catalogs {
		bare, err := transport.New(cat, transport.WithHTTPClient(c.HTTPClient()))
		if err != nil {
			t.Fatalf("%s: transport.New: %v", name, err)
		}
		if _, err := probes.Probe109VersionedObjectReads(t.Context(), bare, &captured, probe109Inputs()); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if len(captured) != 0 {
		t.Errorf("PROBE-109 issued %d request(s) on missing inputs, want 0", len(captured))
	}
}

// contributionCommitBackend records the request body and answers a bare 201.
func contributionCommitBackend(captured *[]byte, plant []byte) *sandbox.Backend {
	return sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if plant != nil {
			b = plant
		}
		*captured = b
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/contribution/cont-1")
		w.WriteHeader(http.StatusCreated)
	})
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
	b := contributionCommitBackend(&captured, nil)
	r, err := probes.Probe084BuiltContributionBody(context.Background(), newClient(t, b), &captured, ehrIDFixture, submissionCorpus(t))
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
// `{code, rmType, extra}`; `extra` injects the violation. `lifecycle` is the
// lifecycle_state code the version carries, and when it is empty the version
// carries what the builder would give it: `deleted` (523) for a deletion,
// `complete` (532) otherwise. `noData` leaves out the `data` member, as a
// deletion built without a payload does.
type probe084Planted struct {
	code      string
	rmType    string
	extra     string
	lifecycle string
	noData    bool
}

func probe084Body(batchCode string, versions ...probe084Planted) []byte {
	audit := func(code string) string {
		return `{"_type":"AUDIT_DETAILS","change_type":{"_type":"DV_CODED_TEXT","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"value":"openehr"},"code_string":"` + code + `"}}}`
	}
	out := make([]string, 0, len(versions))
	for _, v := range versions {
		lifecycle := v.lifecycle
		if lifecycle == "" {
			lifecycle = "532"
			if v.code == "523" {
				lifecycle = "523"
			}
		}
		s := `{"_type":"ORIGINAL_VERSION","commit_audit":` + audit(v.code) +
			`,"lifecycle_state":{"_type":"DV_CODED_TEXT","defining_code":{"_type":"CODE_PHRASE","code_string":"` + lifecycle + `"}}`
		if !v.noData {
			s += `,"data":{"_type":"` + v.rmType + `"}`
		}
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
		preceding5 = `"preceding_version_uid":{"value":"8849182c-82ad-4088-a07f-48ead4180515::cdr.example::5"}`
		callerUID  = `"uid":{"value":"8849182c-82ad-4088-a07f-48ead4180515::cdr.example::3"}`
	)
	// The five versions PROBE-084's own batch emits, in order: the last is a
	// deletion built without a payload, so it carries no `data` member.
	conformant := []probe084Planted{
		{code: "249", rmType: "COMPOSITION"},
		{code: "250", rmType: "COMPOSITION", extra: preceding1},
		{code: "251", rmType: "EHR_STATUS", extra: preceding2 + "," + callerUID},
		{code: "523", rmType: "FOLDER", extra: preceding4},
		{code: "523", noData: true, extra: preceding5},
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
			// A deletion built without a payload has no `data` member at all.
			// A present-but-null key type-asserts like a missing one, so this is
			// the arm that keeps absence judged on key presence.
			name:       "data-less deletion emits null data",
			planted:    probe084Body("253", mutate(4, probe084Planted{code: "523", noData: true, extra: preceding5 + `,"data":null`})...),
			wantDetail: "built without a payload",
		},
		{
			// The counter-arm: a deletion that WAS given a payload still sends
			// it, so a missing `data` is a defect there.
			name:       "deletion with a payload lacks data",
			planted:    probe084Body("253", mutate(3, probe084Planted{code: "523", noData: true, extra: preceding4})...),
			wantDetail: "data is missing",
		},
		{
			// The batch names no lifecycle override, so a deletion carries the
			// `deleted` default, not `complete`.
			name:       "deletion carries complete under the default",
			planted:    probe084Body("253", mutate(3, probe084Planted{code: "523", rmType: "FOLDER", lifecycle: "532", extra: preceding4})...),
			wantDetail: `lifecycle_state code = "532", want "523"`,
		},
		{
			// The default is not derived from the change type for anything but
			// a deletion: a creation stays `complete`.
			name:       "creation carries deleted",
			planted:    probe084Body("253", mutate(0, probe084Planted{code: "249", rmType: "COMPOSITION", lifecycle: "523"})...),
			wantDetail: `lifecycle_state code = "523", want "532"`,
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
			b := contributionCommitBackend(&captured, tc.planted)
			r, err := probes.Probe084BuiltContributionBody(context.Background(), newClient(t, b), &captured, ehrIDFixture, submissionCorpus(t))
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
	b := contributionCommitBackend(&captured, nil)
	r, err := probes.Probe084BuiltContributionBody(context.Background(), newClient(t, b), &captured, ehrIDFixture, submissionCorpus(t))
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
	b := contributionCommitBackend(&captured, nil)
	hostile := [][]byte{[]byte(`{"versions":[{"_type":"ORIGINAL_VERSION","invented_field":1}]}`)}
	r, err := probes.Probe084BuiltContributionBody(context.Background(), newClient(t, b), &captured, ehrIDFixture, hostile)
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
	b := contributionCommitBackend(&captured, nil)
	corpus := submissionCorpus(t)
	if _, err := probes.Probe084BuiltContributionBody(context.Background(), nil, &captured, ehrIDFixture, corpus); err == nil {
		t.Error("nil client: expected an error")
	}
	if _, err := probes.Probe084BuiltContributionBody(context.Background(), newClient(t, b), nil, ehrIDFixture, corpus); err == nil {
		t.Error("nil recorder: expected an error")
	}
	if _, err := probes.Probe084BuiltContributionBody(context.Background(), newClient(t, b), &captured, "", corpus); err == nil {
		t.Error("empty ehr id: expected an error")
	}
	if _, err := probes.Probe084BuiltContributionBody(context.Background(), newClient(t, b), &captured, ehrIDFixture, nil); err == nil {
		t.Error("empty corpus: expected an error")
	}
}
