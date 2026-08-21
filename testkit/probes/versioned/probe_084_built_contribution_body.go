package versionedprobes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// builtVersionFields is the closed set of version-level members a
// write-side body may carry. It is the oracle for the corpus arm below:
// a field a vendored record carries that this SDK can never emit would
// mean the builder cannot express a body the wire is known to accept.
// `contribution` is deliberately absent — REQ-130 § Server-assigned
// fields forbids emitting it — so a corpus record carrying one would
// fail the arm and force the question rather than pass unnoticed.
var builtVersionFields = []string{
	"_type",
	"commit_audit",
	"data",
	"lifecycle_state",
	"preceding_version_uid",
	"uid",
	"signature",
	"attestations",
	"other_input_version_uids",
	"item", // IMPORTED_VERSION's nested historical version
}

// probe084Batch is the batch PROBE-084 builds: one version per operation
// that carries a distinct change-type code, so the code table is asserted
// in one pass. The batch audit deliberately declares `modification` —
// a code no version below shares — because REQ-130 forbids deriving the
// batch code from the versions, and a derived one would silently agree
// with whichever version happened to be first.
var probe084Batch = []struct {
	op           string
	precedingUID string
	wantCode     string
}{
	{op: "creation", wantCode: "249"},
	{op: "amendment", precedingUID: "8849182c-82ad-4088-a07f-48ead4180515::cdr.example::1", wantCode: "250"},
	{op: "deletion", precedingUID: "8849182c-82ad-4088-a07f-48ead4180515::cdr.example::2", wantCode: "523"},
}

const probe084BatchCode = "251"

// Probe084BuiltContributionBody implements PROBE-084: a
// `Contribution_create` body assembled by [contribution.Builder] reaches
// the wire carrying, per version, the requested operation's change-type
// code, a DV_CODED_TEXT lifecycle state, `preceding_version_uid` exactly
// where the operation requires it, and none of the server-assigned fields
// the pin's `UpdateVersion` DTO does not declare.
//
// Pins REQ-130. The body is built and then committed through
// [contribution.Commit], so the assertion reads the bytes the client
// actually sent rather than a hand-marshalled copy — an encode-side
// regression between Build and the wire is in scope.
//
// corpus is the vendored submission corpus (raw
// `testkit/cassettes/submissions/*.json` bodies). It is the shape witness
// for the final arm: every version-level field those records carry must be
// one this SDK can emit ([builtVersionFields]). The comparison is
// structural by construction — the records carry a top-level
// `_type:"CONTRIBUTION"` envelope that `Contribution_create` omits, and RM
// payloads this SDK did not author, so a byte comparison would assert the
// fixture instead of the contract. The corpus is a required input: an arm
// with no data to run against is not a passing arm (REQ-082).
func Probe084BuiltContributionBody(ctx context.Context, c *transport.Client, capturedBody *[]byte, ehrID openehrclient.EHRID, corpus [][]byte) (Result, error) {
	r := Result{Probe: "PROBE-084"}
	if c == nil || ehrID == "" || capturedBody == nil {
		return r, errors.New("PROBE-084: missing required inputs (client/ehr/captured)")
	}
	if len(corpus) == 0 {
		return r, errors.New("PROBE-084: empty corpus — the shape-witness arm has nothing to run against")
	}
	sub, err := buildProbe084Submission()
	if err != nil {
		return r, fmt.Errorf("PROBE-084: %w", err)
	}
	if _, _, err := contribution.Commit(ctx, c, ehrID, sub); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Commit returned error: %v", err)
		return r, nil
	}
	if len(*capturedBody) == 0 {
		r.Status = "fail"
		r.Detail = "captured request body is empty — server fake did not record the request"
		return r, nil
	}
	var body struct {
		Type     string           `json:"_type"`
		Audit    map[string]any   `json:"audit"`
		Versions []map[string]any `json:"versions"`
	}
	if err := json.Unmarshal(*capturedBody, &body); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("request body is not valid JSON: %v", err)
		return r, nil
	}
	if body.Type != "" {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("top-level _type=%q (Contribution_create has no class envelope)", body.Type)
		return r, nil
	}
	if msg := auditWriteShapeIssue("audit", body.Audit); msg != "" {
		r.Status = "fail"
		r.Detail = msg
		return r, nil
	}
	if got := codedCodeString(body.Audit, "change_type"); got != probe084BatchCode {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("audit.change_type code = %q, want the declared %q — REQ-130 forbids deriving the batch code from the versions", got, probe084BatchCode)
		return r, nil
	}
	if len(body.Versions) != len(probe084Batch) {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("versions[] has %d entries, want %d", len(body.Versions), len(probe084Batch))
		return r, nil
	}
	for i, want := range probe084Batch {
		if msg := probe084VersionIssue(i, want.op, want.precedingUID, want.wantCode, body.Versions[i]); msg != "" {
			r.Status = "fail"
			r.Detail = msg
			return r, nil
		}
	}
	corpusFields, msg := probe084CorpusIssue(corpus)
	if msg != "" {
		r.Status = "fail"
		r.Detail = msg
		return r, nil
	}
	r.Status = "pass"
	r.Detail = fmt.Sprintf("built body: %d versions (249/250/523), batch audit code %s carried as declared; %d corpus records witness %d version fields, all emittable",
		len(body.Versions), probe084BatchCode, len(corpus), corpusFields)
	return r, nil
}

// buildProbe084Submission assembles the batch through the public builder —
// the surface under test. The payload only needs to be a COMPOSITION: this
// probe asserts version metadata, not composition validity (REQ-102 owns
// that).
func buildProbe084Submission() (*contribution.Submission, error) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	b := contribution.NewBuilder().
		WithCommitterName("probe-084").
		WithSystemID("cdr.example").
		WithChangeType(contribution.ChangeTypeModification)
	for _, step := range probe084Batch {
		switch step.op {
		case "creation":
			b = b.Add(contribution.Creation(&comp))
		case "amendment":
			b = b.Add(contribution.Amendment(step.precedingUID, &comp))
		case "deletion":
			b = b.Add(contribution.Deletion(step.precedingUID, &comp))
		default:
			return nil, fmt.Errorf("unknown probe step %q", step.op)
		}
	}
	return b.Build()
}

// probe084VersionIssue returns a non-empty description when one decoded
// version violates REQ-130, or "" when it conforms.
func probe084VersionIssue(i int, op, precedingUID, wantCode string, v map[string]any) string {
	at := fmt.Sprintf("versions[%d] (%s)", i, op)
	if v["_type"] != "ORIGINAL_VERSION" {
		return fmt.Sprintf("%s._type = %v, want ORIGINAL_VERSION", at, v["_type"])
	}
	ca, ok := v["commit_audit"].(map[string]any)
	if !ok {
		return at + ".commit_audit is missing"
	}
	if msg := auditWriteShapeIssue(at+".commit_audit", ca); msg != "" {
		return msg
	}
	if got := codedCodeString(ca, "change_type"); got != wantCode {
		return fmt.Sprintf("%s.commit_audit.change_type code = %q, want %q", at, got, wantCode)
	}
	if got := codedCodeString(v, "lifecycle_state"); got == "" {
		return at + ".lifecycle_state is missing or not DV_CODED_TEXT-shaped (required on the pin's UpdateVersion)"
	}
	uid, hasPreceding := v["preceding_version_uid"].(map[string]any)
	switch {
	case precedingUID == "" && hasPreceding:
		return fmt.Sprintf("%s carries preceding_version_uid %v — a creation follows no version", at, uid["value"])
	case precedingUID != "" && !hasPreceding:
		return at + " is missing preceding_version_uid"
	case precedingUID != "" && uid["value"] != precedingUID:
		return fmt.Sprintf("%s.preceding_version_uid = %v, want %q", at, uid["value"], precedingUID)
	}
	if _, has := v["contribution"]; has {
		return at + " emits the server-assigned `contribution` (not declared on UpdateVersion — REQ-130)"
	}
	if uidMap, has := v["uid"].(map[string]any); has {
		val, _ := uidMap["value"].(string)
		if val == "" {
			return at + " emits an empty `uid` (server-assigned — REQ-130)"
		}
	}
	data, ok := v["data"].(map[string]any)
	if !ok || data["_type"] == nil {
		return at + ".data is missing or carries no _type (Contribution_create requires the payload inline)"
	}
	return ""
}

// probe084CorpusIssue checks every vendored submission record's
// version-level field names against [builtVersionFields], returning the
// number of distinct fields witnessed and a non-empty description of the
// first field this SDK could not emit.
func probe084CorpusIssue(corpus [][]byte) (int, string) {
	witnessed := map[string]struct{}{}
	for n, raw := range corpus {
		var body struct {
			Versions []map[string]any `json:"versions"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			return len(witnessed), fmt.Sprintf("corpus record %d is not valid JSON: %v", n, err)
		}
		for i, v := range body.Versions {
			for field := range v {
				witnessed[field] = struct{}{}
				if !slices.Contains(builtVersionFields, field) {
					return len(witnessed), fmt.Sprintf("corpus record %d versions[%d] carries %q, a version field this SDK cannot emit — the builder cannot express a body the wire is known to accept", n, i, field)
				}
			}
		}
	}
	return len(witnessed), ""
}

// codedCodeString reads field's DV_CODED_TEXT defining_code.code_string
// out of a decoded object, returning "" when any hop is absent.
func codedCodeString(m map[string]any, field string) string {
	ct, ok := m[field].(map[string]any)
	if !ok {
		return ""
	}
	dc, ok := ct["defining_code"].(map[string]any)
	if !ok {
		return ""
	}
	s, _ := dc["code_string"].(string)
	return s
}
