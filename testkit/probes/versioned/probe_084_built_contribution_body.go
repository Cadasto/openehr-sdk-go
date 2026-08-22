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

// probe084Step is one expected version of the batch: what was asked for, and
// what the wire must therefore carry.
type probe084Step struct {
	// label names the operation in a failure message.
	label string
	// rmType is the `data._type` the payload must keep on the wire — the
	// batch spans three of the four versionable types, so a generic
	// instantiation that lost its discriminator shows up here.
	rmType string
	// precedingUID is the version this one follows; empty for a creation,
	// which must carry no `preceding_version_uid` at all.
	precedingUID string
	// wantCode is the audit change-type code the operation implies.
	wantCode string
	// wantUID is the `uid` the caller supplied. Empty means the caller
	// supplied none, and the version must then carry **no** `uid` key:
	// REQ-130 forbids the builder synthesising one. Asserting "absent"
	// rather than "not empty" is what keeps this arm biting — a builder
	// that invented a uid would satisfy a non-empty check.
	wantUID string
}

// probe084Batch is the batch PROBE-084 builds: one version per operation, so
// the whole change-type table (249 / 250 / 251 / 523) is asserted in one
// pass, across three of the four versionable types.
var probe084Batch = []probe084Step{
	{label: "creation of a COMPOSITION", rmType: "COMPOSITION", wantCode: "249"},
	{label: "amendment of a COMPOSITION", rmType: "COMPOSITION", precedingUID: "8849182c-82ad-4088-a07f-48ead4180515::cdr.example::1", wantCode: "250"},
	// This one names its own uid, so both halves of the server-assigned-field
	// rule are asserted on the wire: three versions must carry no `uid`, and
	// this one must carry exactly the caller's.
	{label: "modification of an EHR_STATUS with a caller-supplied uid", rmType: "EHR_STATUS", precedingUID: "8849182c-82ad-4088-a07f-48ead4180515::cdr.example::2", wantCode: "251", wantUID: "8849182c-82ad-4088-a07f-48ead4180515::cdr.example::3"},
	{label: "deletion of a FOLDER", rmType: "FOLDER", precedingUID: "8849182c-82ad-4088-a07f-48ead4180515::cdr.example::4", wantCode: "523"},
}

// probe084BatchCode is the batch audit's change type — openEHR `unknown`,
// which is deliberately NOT one of the four codes the builder authors per
// operation. Since the versions between them now carry all four, a code
// from outside that set is the only batch value no derivation rule over the
// versions could reproduce, so the non-derivation arm cannot pass by
// coincidence. It reaches the audit through Builder.WithAudit — the
// documented escape hatch for a code outside the authored table — which
// this probe therefore also exercises on the wire.
const probe084BatchCode = "253"

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
		if msg := probe084VersionIssue(i, want, body.Versions[i]); msg != "" {
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
	r.Detail = fmt.Sprintf("built body: %d versions covering 249/250/251/523 over COMPOSITION/EHR_STATUS/FOLDER, batch audit code %s carried as declared; %d corpus records witness %d version fields, all emittable",
		len(body.Versions), probe084BatchCode, len(corpus), corpusFields)
	return r, nil
}

// buildProbe084Submission assembles the batch through the public builder —
// the surface under test. The changes are written out rather than looped
// because each operation instantiates its own payload type, which is the
// point: three of the four versionable types travel in one batch. The
// payloads need only carry their discriminator — this probe asserts version
// metadata, not composition validity (REQ-102 owns that). Their order MUST
// match [probe084Batch], and a mismatch is reported as framework misuse
// rather than as a builder defect.
func buildProbe084Submission() (*contribution.Submission, error) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	status := rm.EHRStatus{ArchetypeNodeID: "openEHR-EHR-EHR_STATUS.generic.v1", IsQueryable: true, IsModifiable: true}
	folder := rm.Folder{Name: rm.DVText{Value: "Encounters"}}
	changes := []contribution.Change{
		contribution.Creation(&comp),
		contribution.Amendment(probe084Batch[1].precedingUID, &comp),
		contribution.Modification(probe084Batch[2].precedingUID, &status, contribution.WithVersionUID(probe084Batch[2].wantUID)),
		contribution.Deletion(probe084Batch[3].precedingUID, &folder),
	}
	if len(changes) != len(probe084Batch) {
		return nil, fmt.Errorf("built %d changes for %d expected steps", len(changes), len(probe084Batch))
	}
	// The batch audit is set wholesale so its change type can be a code
	// outside the authored table (see probe084BatchCode); the committer and
	// system id are then layered on, exercising both entry points.
	return contribution.NewBuilder().
		WithAudit(contribution.UpdateAudit{
			ChangeType: rm.DVCodedText{
				DVText:       rm.DVText{Value: "unknown"},
				DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: probe084BatchCode},
			},
		}).
		WithCommitterName("probe-084").
		WithSystemID("cdr.example").
		Add(changes...).
		Build()
}

// probe084VersionIssue returns a non-empty description when one decoded
// version violates REQ-130, or "" when it conforms.
func probe084VersionIssue(i int, want probe084Step, v map[string]any) string {
	at := fmt.Sprintf("versions[%d] (%s)", i, want.label)
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
	if got := codedCodeString(ca, "change_type"); got != want.wantCode {
		return fmt.Sprintf("%s.commit_audit.change_type code = %q, want %q", at, got, want.wantCode)
	}
	if got := codedCodeString(v, "lifecycle_state"); got == "" {
		return at + ".lifecycle_state is missing or not DV_CODED_TEXT-shaped (required on the pin's UpdateVersion)"
	}
	uid, hasPreceding := v["preceding_version_uid"].(map[string]any)
	switch {
	case want.precedingUID == "" && hasPreceding:
		return fmt.Sprintf("%s carries preceding_version_uid %v — a creation follows no version", at, uid["value"])
	case want.precedingUID != "" && !hasPreceding:
		return at + " is missing preceding_version_uid"
	case want.precedingUID != "" && uid["value"] != want.precedingUID:
		return fmt.Sprintf("%s.preceding_version_uid = %v, want %q", at, uid["value"], want.precedingUID)
	}
	if _, has := v["contribution"]; has {
		return at + " emits the server-assigned `contribution` (not declared on UpdateVersion — REQ-130)"
	}
	if msg := probe084UIDIssue(at, want.wantUID, v); msg != "" {
		return msg
	}
	data, ok := v["data"].(map[string]any)
	if !ok || data["_type"] == nil {
		return at + ".data is missing or carries no _type (Contribution_create requires the payload inline)"
	}
	if data["_type"] != want.rmType {
		return fmt.Sprintf("%s.data._type = %v, want %s", at, data["_type"], want.rmType)
	}
	return ""
}

// probe084UIDIssue holds the version `uid` to exactly what the caller asked
// for. REQ-130 splits this two ways and both halves are asserted: a uid the
// caller supplied MUST reach the wire verbatim, and one the caller did not
// supply MUST be absent — not merely non-empty. Accepting any non-empty uid
// where none was supplied would let a builder that synthesised one pass,
// which is the MUST NOT this arm exists to catch.
//
// Absence is judged on **key presence**, before any type assertion, because
// `"uid":null` is a present key whose value type-asserts like a missing one.
// Dropping `omitempty` from the write-side uid field emits exactly that, and
// asserting the type first read it as absent — the neighbouring golden and
// wrapper tests caught the regression, this arm did not.
func probe084UIDIssue(at, wantUID string, v map[string]any) string {
	raw, present := v["uid"]
	if wantUID == "" {
		if present {
			return fmt.Sprintf("%s emits `uid` %v for a version whose caller supplied none — the server assigns it, so the key must be absent rather than empty or null (REQ-130)", at, raw)
		}
		return ""
	}
	if !present {
		return fmt.Sprintf("%s dropped the caller-supplied `uid` %q (REQ-130 emits it verbatim)", at, wantUID)
	}
	got, ok := raw.(map[string]any)
	if !ok {
		return fmt.Sprintf("%s.uid = %v, want the caller-supplied %q as an OBJECT_VERSION_ID object", at, raw, wantUID)
	}
	if got["value"] != wantUID {
		return fmt.Sprintf("%s.uid = %v, want the caller-supplied %q", at, got["value"], wantUID)
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
