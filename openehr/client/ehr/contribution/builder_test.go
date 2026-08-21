package contribution_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// marshalSubmission renders sub through the canonical-JSON path the wire
// uses and decodes it back into generic maps, so the assertions below read
// the bytes a CDR would receive rather than the Go struct that produced
// them. REQ-130 / PROBE-084.
func marshalSubmission(t *testing.T, sub *contribution.Submission) (audit map[string]any, versions []map[string]any) {
	t.Helper()
	b, err := canjson.Marshal(sub)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	var body struct {
		Audit    map[string]any   `json:"audit"`
		Versions []map[string]any `json:"versions"`
	}
	if err := json.Unmarshal(b, &body); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, b)
	}
	return body.Audit, body.Versions
}

// codeOf reads a DV_CODED_TEXT's defining_code.code_string from a decoded
// body, reporting a t.Errorf-friendly empty string when any hop is absent.
func codeOf(m map[string]any, field string) string {
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

// termOf reads a DV_CODED_TEXT's defining_code.terminology_id.value.
func termOf(m map[string]any, field string) string {
	ct, ok := m[field].(map[string]any)
	if !ok {
		return ""
	}
	dc, ok := ct["defining_code"].(map[string]any)
	if !ok {
		return ""
	}
	tid, ok := dc["terminology_id"].(map[string]any)
	if !ok {
		return ""
	}
	s, _ := tid["value"].(string)
	return s
}

// TestBuilderCreationWireShape pins the REQ-130 creation contract: the
// creation change-type code on the version audit, a defaulted `complete`
// lifecycle state in the version body, no preceding_version_uid, and none
// of the server-assigned fields the pin's UpdateVersion does not declare.
func TestBuilderCreationWireShape(t *testing.T) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	sub, err := contribution.NewBuilder().
		WithCommitterName("alice").
		WithSystemID("cdr.example").
		WithChangeType(contribution.ChangeTypeCreation).
		Add(contribution.Creation(&comp)).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := sub.Validate(); err != nil {
		t.Fatalf("built Submission fails Validate: %v", err)
	}
	audit, versions := marshalSubmission(t, sub)
	if got := codeOf(audit, "change_type"); got != "249" {
		t.Errorf("audit.change_type code = %q, want 249", got)
	}
	if len(versions) != 1 {
		t.Fatalf("len(versions) = %d, want 1", len(versions))
	}
	v := versions[0]
	if v["_type"] != "ORIGINAL_VERSION" {
		t.Errorf("versions[0]._type = %v, want ORIGINAL_VERSION", v["_type"])
	}
	ca, ok := v["commit_audit"].(map[string]any)
	if !ok {
		t.Fatalf("versions[0].commit_audit missing: %v", v)
	}
	if got := codeOf(ca, "change_type"); got != "249" {
		t.Errorf("commit_audit.change_type code = %q, want 249 (creation)", got)
	}
	if got := termOf(ca, "change_type"); got != "openehr" {
		t.Errorf("commit_audit.change_type terminology = %q, want openehr", got)
	}
	if _, has := ca["time_committed"]; has {
		t.Error("commit_audit carries server-assigned time_committed")
	}
	if got := codeOf(v, "lifecycle_state"); got != "532" {
		t.Errorf("lifecycle_state code = %q, want 532 (complete by default)", got)
	}
	if got := termOf(v, "lifecycle_state"); got != "openehr" {
		t.Errorf("lifecycle_state terminology = %q, want openehr", got)
	}
	if _, has := v["preceding_version_uid"]; has {
		t.Error("a creation must not carry preceding_version_uid")
	}
	if _, has := v["contribution"]; has {
		t.Error("server-assigned `contribution` emitted on a create body")
	}
	if _, has := v["uid"]; has {
		t.Error("server-assigned `uid` emitted on a create body")
	}
	data, ok := v["data"].(map[string]any)
	if !ok {
		t.Fatalf("versions[0].data missing or not an object: %v", v)
	}
	if data["_type"] != "COMPOSITION" {
		t.Errorf("data._type = %v, want COMPOSITION", data["_type"])
	}
}

// newBuilder returns a builder with the two audit fields the pin marks
// required already set, so each test below states only what it is about.
func newBuilder() *contribution.Builder {
	return contribution.NewBuilder().
		WithCommitterName("alice").
		WithSystemID("cdr.example").
		WithChangeType(contribution.ChangeTypeCreation)
}

// TestBuilderPrecedingVersionPerOperation pins the change-type code table
// and the preceding-version rule for the three operations that follow an
// existing version. REQ-130.
func TestBuilderPrecedingVersionPerOperation(t *testing.T) {
	const preceding = "8849182c-82ad-4088-a07f-48ead4180515::cdr.example::1"
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	cases := []struct {
		name     string
		change   contribution.Change
		wantCode string
	}{
		{"amendment", contribution.Amendment(preceding, &comp), "250"},
		{"modification", contribution.Modification(preceding, &comp), "251"},
		{"deletion", contribution.Deletion(preceding, &comp), "523"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub, err := newBuilder().Add(tc.change).Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			_, versions := marshalSubmission(t, sub)
			v := versions[0]
			ca, ok := v["commit_audit"].(map[string]any)
			if !ok {
				t.Fatalf("commit_audit missing: %v", v)
			}
			if got := codeOf(ca, "change_type"); got != tc.wantCode {
				t.Errorf("change_type code = %q, want %q", got, tc.wantCode)
			}
			uid, ok := v["preceding_version_uid"].(map[string]any)
			if !ok {
				t.Fatalf("preceding_version_uid missing on a %s: %v", tc.name, v)
			}
			if uid["value"] != preceding {
				t.Errorf("preceding_version_uid = %v, want %q", uid["value"], preceding)
			}
			// The lifecycle state is not derived from the change type —
			// most deletions in the vendored corpus are `complete`.
			if got := codeOf(v, "lifecycle_state"); got != "532" {
				t.Errorf("lifecycle_state code = %q, want 532 (never derived from the change type)", got)
			}
		})
	}
}

// TestBuilderLifecycleStateOverride proves the per-version override reaches
// the body, and that the code the caller names is the code emitted.
func TestBuilderLifecycleStateOverride(t *testing.T) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	sub, err := newBuilder().
		Add(contribution.Deletion("1::cdr.example::1", &comp, contribution.WithLifecycleState(ehr.LifecycleStateDeleted))).
		Add(contribution.Creation(&comp, contribution.WithLifecycleState(ehr.LifecycleStateIncomplete))).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	_, versions := marshalSubmission(t, sub)
	if got := codeOf(versions[0], "lifecycle_state"); got != "523" {
		t.Errorf("versions[0].lifecycle_state = %q, want 523", got)
	}
	if got := codeOf(versions[1], "lifecycle_state"); got != "553" {
		t.Errorf("versions[1].lifecycle_state = %q, want 553", got)
	}
}

// TestBuilderRejectsUnknownLifecycleState — a code outside the openEHR
// version-lifecycle-state group must fail at Build, not reach the wire.
func TestBuilderRejectsUnknownLifecycleState(t *testing.T) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	sub, err := newBuilder().
		Add(contribution.Creation(&comp, contribution.WithLifecycleState(ehr.LifecycleState("999")))).
		Build()
	if err == nil {
		t.Fatalf("Build accepted lifecycle code 999: %+v", sub)
	}
	if sub != nil {
		t.Error("Build returned a submission alongside an error")
	}
}

// TestBuilderDoesNotDeriveBatchChangeType — the batch audit carries what
// the caller declared even when no version shares that code, the spelling
// the vendored corpus records. REQ-130.
func TestBuilderDoesNotDeriveBatchChangeType(t *testing.T) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	sub, err := contribution.NewBuilder().
		WithCommitterName("alice").
		WithChangeType(contribution.ChangeTypeCreation).
		Add(contribution.Modification("1::cdr.example::1", &comp)).
		Add(contribution.Deletion("2::cdr.example::1", &comp)).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	audit, versions := marshalSubmission(t, sub)
	if got := codeOf(audit, "change_type"); got != "249" {
		t.Errorf("audit.change_type = %q, want the declared 249 — never derived from the versions", got)
	}
	for i, want := range []string{"251", "523"} {
		ca, ok := versions[i]["commit_audit"].(map[string]any)
		if !ok {
			t.Fatalf("versions[%d].commit_audit missing", i)
		}
		if got := codeOf(ca, "change_type"); got != want {
			t.Errorf("versions[%d].change_type = %q, want %q", i, got, want)
		}
	}
}

// TestBuilderRefusals covers every accumulation error REQ-130 requires to
// surface at Build with no submission returned.
func TestBuilderRefusals(t *testing.T) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	cases := []struct {
		name    string
		builder *contribution.Builder
		want    string
	}{
		{
			name:    "no versions",
			builder: newBuilder(),
			want:    "no versions added",
		},
		{
			name: "missing batch change_type",
			builder: contribution.NewBuilder().
				WithCommitterName("alice").
				Add(contribution.Creation(&comp)),
			want: "change_type is required",
		},
		{
			name: "missing committer",
			builder: contribution.NewBuilder().
				WithChangeType(contribution.ChangeTypeCreation).
				Add(contribution.Creation(&comp)),
			want: "committer",
		},
		{
			name:    "amendment without a preceding uid",
			builder: newBuilder().Add(contribution.Amendment("", &comp)),
			want:    "preceding version uid",
		},
		{
			name:    "nil payload",
			builder: newBuilder().Add(contribution.Creation[rm.Composition](nil)),
			want:    "nil data",
		},
		{
			name:    "zero Change",
			builder: newBuilder().Add(contribution.Change{}),
			want:    "zero Change",
		},
		{
			name:    "unknown batch change_type code",
			builder: newBuilder().WithChangeType(contribution.ChangeType("253")).Add(contribution.Creation(&comp)),
			want:    "audit-change-type code",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub, err := tc.builder.Build()
			if err == nil {
				t.Fatalf("Build succeeded, want an error mentioning %q", tc.want)
			}
			if sub != nil {
				t.Error("Build returned a submission alongside an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// TestBuilderBuildIsIdempotent — a second Build repeats the first, and
// mutating the builder afterwards leaves an earlier submission untouched.
// REQ-130.
func TestBuilderBuildIsIdempotent(t *testing.T) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	b := newBuilder().Add(contribution.Creation(&comp))
	first, err := b.Build()
	if err != nil {
		t.Fatalf("first Build: %v", err)
	}
	firstBytes, err := canjson.Marshal(first)
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	second, err := b.Build()
	if err != nil {
		t.Fatalf("second Build: %v", err)
	}
	secondBytes, err := canjson.Marshal(second)
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}
	if string(firstBytes) != string(secondBytes) {
		t.Errorf("Build is not idempotent:\n first: %s\nsecond: %s", firstBytes, secondBytes)
	}
	// Mutating the builder must not reach back into `first`.
	b.WithSystemID("other.system").Add(contribution.Creation(&comp))
	afterBytes, err := canjson.Marshal(first)
	if err != nil {
		t.Fatalf("re-marshal first: %v", err)
	}
	if string(afterBytes) != string(firstBytes) {
		t.Errorf("mutating the Builder changed an already-built Submission:\nbefore: %s\n after: %s", firstBytes, afterBytes)
	}
	if len(second.Versions) != 1 {
		t.Errorf("len(second.Versions) = %d, want 1", len(second.Versions))
	}
}

// TestBuilderMixesVersionableTypes — all four members of the closed
// type-set coexist in one batch, each keeping its own `_type`. REQ-130.
func TestBuilderMixesVersionableTypes(t *testing.T) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	status := rm.EHRStatus{ArchetypeNodeID: "openEHR-EHR-EHR_STATUS.generic.v1", IsQueryable: true}
	folder := rm.Folder{Name: rm.DVText{Value: "Encounters"}}
	access := rm.EHRAccess{}
	sub, err := newBuilder().
		Add(contribution.Creation(&comp)).
		Add(contribution.Creation(&status)).
		Add(contribution.Creation(&folder)).
		Add(contribution.Creation(&access)).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	_, versions := marshalSubmission(t, sub)
	want := []string{"COMPOSITION", "EHR_STATUS", "FOLDER", "EHR_ACCESS"}
	if len(versions) != len(want) {
		t.Fatalf("len(versions) = %d, want %d", len(versions), len(want))
	}
	for i, w := range want {
		data, ok := versions[i]["data"].(map[string]any)
		if !ok {
			t.Fatalf("versions[%d].data missing", i)
		}
		if data["_type"] != w {
			t.Errorf("versions[%d].data._type = %v, want %s", i, data["_type"], w)
		}
	}
}

// TestBuilderVersionAuditInheritance — a version inherits the batch
// committer, system id, description, and audit `_type`, and each is
// overridable per version. REQ-130.
func TestBuilderVersionAuditInheritance(t *testing.T) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	bob := "bob"
	sub, err := contribution.NewBuilder().
		WithCommitterName("alice").
		WithSystemID("cdr.example").
		WithDescription("nightly seed").
		WithAuditType(contribution.AuditTypeUpdateAudit).
		WithChangeType(contribution.ChangeTypeCreation).
		Add(contribution.Creation(&comp)).
		Add(contribution.Creation(&comp,
			contribution.WithVersionCommitter(&rm.PartyIdentified{Name: &bob}),
			contribution.WithVersionSystemID("other.system"),
			contribution.WithVersionDescription("correction"))).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	_, versions := marshalSubmission(t, sub)
	inherited, ok := versions[0]["commit_audit"].(map[string]any)
	if !ok {
		t.Fatalf("versions[0].commit_audit missing")
	}
	if inherited["_type"] != "UPDATE_AUDIT" {
		t.Errorf("inherited audit _type = %v, want UPDATE_AUDIT", inherited["_type"])
	}
	if inherited["system_id"] != "cdr.example" {
		t.Errorf("inherited system_id = %v, want cdr.example", inherited["system_id"])
	}
	if committer, _ := inherited["committer"].(map[string]any); committer["name"] != "alice" {
		t.Errorf("inherited committer = %v, want alice", inherited["committer"])
	}
	if desc, _ := inherited["description"].(map[string]any); desc["value"] != "nightly seed" {
		t.Errorf("inherited description = %v, want \"nightly seed\"", inherited["description"])
	}
	overridden, ok := versions[1]["commit_audit"].(map[string]any)
	if !ok {
		t.Fatalf("versions[1].commit_audit missing")
	}
	if overridden["system_id"] != "other.system" {
		t.Errorf("overridden system_id = %v, want other.system", overridden["system_id"])
	}
	if committer, _ := overridden["committer"].(map[string]any); committer["name"] != "bob" {
		t.Errorf("overridden committer = %v, want bob", overridden["committer"])
	}
	if desc, _ := overridden["description"].(map[string]any); desc["value"] != "correction" {
		t.Errorf("overridden description = %v, want \"correction\"", overridden["description"])
	}
}

// TestBuilderEmitsCallerSuppliedUID is the counter-arm of the
// server-assigned-field rule: an empty uid is omitted, but one the caller
// names reaches the wire verbatim. REQ-130.
func TestBuilderEmitsCallerSuppliedUID(t *testing.T) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	const uid = "3d1a9c14-8f6e-4a2c-9c1c-2b8f0f4a1e77::cdr.example::2"
	sub, err := newBuilder().
		Add(contribution.Creation(&comp, contribution.WithVersionUID(uid))).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	_, versions := marshalSubmission(t, sub)
	got, ok := versions[0]["uid"].(map[string]any)
	if !ok {
		t.Fatalf("caller-supplied uid was dropped: %v", versions[0])
	}
	if got["value"] != uid {
		t.Errorf("uid = %v, want %q", got["value"], uid)
	}
}

// TestBuilderWithAuditCarriesAnyChangeType — [Builder.WithChangeType] admits
// only the four codes the SDK authors, so a caller who needs another one
// (`253` unknown, say) supplies the whole audit instead. That path must
// satisfy the required-change_type gate without widening the code set.
func TestBuilderWithAuditCarriesAnyChangeType(t *testing.T) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	name := "alice"
	sub, err := contribution.NewBuilder().
		WithAudit(contribution.UpdateAudit{
			Committer: &rm.PartyIdentified{Name: &name},
			ChangeType: rm.DVCodedText{
				DVText:       rm.DVText{Value: "unknown"},
				DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "253"},
			},
		}).
		Add(contribution.Creation(&comp)).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	audit, versions := marshalSubmission(t, sub)
	if got := codeOf(audit, "change_type"); got != "253" {
		t.Errorf("audit.change_type = %q, want the caller's 253", got)
	}
	// The version audit still carries the operation's own code.
	ca, ok := versions[0]["commit_audit"].(map[string]any)
	if !ok {
		t.Fatalf("versions[0].commit_audit missing")
	}
	if got := codeOf(ca, "change_type"); got != "249" {
		t.Errorf("versions[0].change_type = %q, want 249", got)
	}
}

// TestBuilderNilReceiverNeverPanics — REQ-025: no caller input, including a
// nil Builder, panics the library.
func TestBuilderNilReceiverNeverPanics(t *testing.T) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	var b *contribution.Builder
	sub, err := b.WithCommitterName("alice").
		WithSystemID("s").
		WithDescription("d").
		WithAuditType(contribution.AuditTypeUpdateAudit).
		WithAudit(contribution.UpdateAudit{}).
		WithChangeType(contribution.ChangeTypeCreation).
		Add(contribution.Creation(&comp)).
		Build()
	if err == nil {
		t.Fatalf("nil Builder built a submission: %+v", sub)
	}
	if sub != nil {
		t.Error("nil Builder returned a submission alongside an error")
	}
}
