package contribution_test

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
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

// valueOf reads a DV_CODED_TEXT's `value` — the rubric a human reads —
// from a decoded body.
func valueOf(m map[string]any, field string) string {
	ct, ok := m[field].(map[string]any)
	if !ok {
		return ""
	}
	s, _ := ct["value"].(string)
	return s
}

// versionKeys returns the member names of one decoded version in sorted order,
// so a test can pin the whole shape of a version instead of the few members it
// happens to look at.
func versionKeys(v map[string]any) []string {
	return slices.Sorted(maps.Keys(v))
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
	if got := valueOf(v, "lifecycle_state"); got != "complete" {
		t.Errorf("lifecycle_state value = %q, want the pinned rubric %q (default 532) — REQ-034", got, "complete")
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
		name          string
		change        contribution.Change
		wantCode      string
		wantLifecycle string
	}{
		{"amendment", contribution.Amendment(preceding, &comp), "250", "532"},
		{"modification", contribution.Modification(preceding, &comp), "251", "532"},
		{"deletion", contribution.Deletion(preceding, &comp), "523", "523"},
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
			// The rubric beside the code is the pin's, never one typed in
			// this package or this test (REQ-034).
			wantRubric, ok := terminology.AuditChangeType.Rubric(tc.wantCode)
			if !ok {
				t.Fatalf("code %q is not in the pinned audit-change-type group, so this table row is stale", tc.wantCode)
			}
			if got := valueOf(ca, "change_type"); got != wantRubric {
				t.Errorf("change_type value = %q, want the pinned rubric %q for %s", got, wantRubric, tc.wantCode)
			}
			uid, ok := v["preceding_version_uid"].(map[string]any)
			if !ok {
				t.Fatalf("preceding_version_uid missing on a %s: %v", tc.name, v)
			}
			if uid["value"] != preceding {
				t.Errorf("preceding_version_uid = %v, want %q", uid["value"], preceding)
			}
			// An amendment and a modification default to `complete`; the
			// lifecycle state is not derived from their change type. A
			// deletion is the one operation whose default is `deleted`.
			if got := codeOf(v, "lifecycle_state"); got != tc.wantLifecycle {
				t.Errorf("lifecycle_state code = %q, want %q (`complete` unless the operation is a deletion)", got, tc.wantLifecycle)
			}
		})
	}
}

// TestBuilderLifecycleStateOverride proves the per-version override reaches
// the body, and that the code the caller names is the code emitted. The
// deletion row names `complete`, the opposite of a deletion's own default, so
// it shows the override replacing that default. REQ-130.
func TestBuilderLifecycleStateOverride(t *testing.T) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	sub, err := newBuilder().
		Add(contribution.Deletion("1::cdr.example::1", &comp, contribution.WithLifecycleState(ehr.LifecycleStateComplete))).
		Add(contribution.Creation(&comp, contribution.WithLifecycleState(ehr.LifecycleStateIncomplete))).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	_, versions := marshalSubmission(t, sub)
	if got := codeOf(versions[0], "lifecycle_state"); got != "532" {
		t.Errorf("versions[0].lifecycle_state = %q, want 532 (the override of a deletion's default)", got)
	}
	if got := codeOf(versions[1], "lifecycle_state"); got != "553" {
		t.Errorf("versions[1].lifecycle_state = %q, want 553", got)
	}
}

// TestBuilderDeletionWithoutPayload pins REQ-130 § Deletion: a deletion given
// no payload builds, and the version it emits carries no `data` member at all
// (absent, not `"data":null`), change type `deleted` (523) and lifecycle state
// `deleted` (523). A nil payload has no type to infer the instantiation from,
// so each versionable type is named explicitly.
func TestBuilderDeletionWithoutPayload(t *testing.T) {
	const preceding = "8849182c-82ad-4088-a07f-48ead4180515::cdr.example::1"
	cases := []struct {
		name   string
		change contribution.Change
	}{
		{"composition", contribution.Deletion[rm.Composition](preceding, nil)},
		{"ehr status", contribution.Deletion[rm.EHRStatus](preceding, nil)},
		{"folder", contribution.Deletion[rm.Folder](preceding, nil)},
		{"ehr access", contribution.Deletion[rm.EHRAccess](preceding, nil)},
	}
	wantKeys := []string{"_type", "commit_audit", "lifecycle_state", "preceding_version_uid"}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub, err := newBuilder().Add(tc.change).Build()
			if err != nil {
				t.Fatalf("Build refused a deletion with no payload: %v", err)
			}
			if err := sub.Validate(); err != nil {
				t.Fatalf("Validate refused the built deletion with no payload: %v", err)
			}
			_, versions := marshalSubmission(t, sub)
			if len(versions) != 1 {
				t.Fatalf("len(versions) = %d, want 1", len(versions))
			}
			v := versions[0]
			// Presence is judged on the key, not the value: a `"data":null`
			// member decodes to a nil value but is still a member.
			if raw, has := v["data"]; has {
				t.Errorf("version carries a `data` member (%v); a deletion built without a payload must omit it, not send null", raw)
			}
			if got := versionKeys(v); !slices.Equal(got, wantKeys) {
				t.Errorf("version members = %v, want exactly %v", got, wantKeys)
			}
			if v["_type"] != "ORIGINAL_VERSION" {
				t.Errorf("_type = %v, want ORIGINAL_VERSION", v["_type"])
			}
			ca, ok := v["commit_audit"].(map[string]any)
			if !ok {
				t.Fatalf("commit_audit missing: %v", v)
			}
			if got := codeOf(ca, "change_type"); got != "523" {
				t.Errorf("commit_audit.change_type code = %q, want 523", got)
			}
			if got := codeOf(v, "lifecycle_state"); got != "523" {
				t.Errorf("lifecycle_state code = %q, want 523 (a deletion defaults to deleted)", got)
			}
			if got := valueOf(v, "lifecycle_state"); got != "deleted" {
				t.Errorf("lifecycle_state value = %q, want the pinned rubric %q", got, "deleted")
			}
			if got := termOf(v, "lifecycle_state"); got != "openehr" {
				t.Errorf("lifecycle_state terminology = %q, want openehr", got)
			}
			uid, ok := v["preceding_version_uid"].(map[string]any)
			if !ok {
				t.Fatalf("preceding_version_uid missing: %v", v)
			}
			if uid["value"] != preceding {
				t.Errorf("preceding_version_uid = %v, want %q", uid["value"], preceding)
			}
		})
	}
}

// TestBuilderDeletionWithPayload is the counter-arm of
// [TestBuilderDeletionWithoutPayload]: a deletion given a payload still sends
// it inline under `data` with its `_type`, so a caller whose server asks for
// the previous content keeps sending it, and the lifecycle state defaults to
// `deleted` (523) with the payload present too. REQ-130.
func TestBuilderDeletionWithPayload(t *testing.T) {
	const preceding = "8849182c-82ad-4088-a07f-48ead4180515::cdr.example::1"
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	folder := rm.Folder{Name: rm.DVText{Value: "Encounters"}}
	cases := []struct {
		name     string
		change   contribution.Change
		wantType string
	}{
		{"composition", contribution.Deletion(preceding, &comp), "COMPOSITION"},
		{"folder", contribution.Deletion(preceding, &folder), "FOLDER"},
	}
	wantKeys := []string{"_type", "commit_audit", "data", "lifecycle_state", "preceding_version_uid"}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub, err := newBuilder().Add(tc.change).Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			_, versions := marshalSubmission(t, sub)
			v := versions[0]
			if got := versionKeys(v); !slices.Equal(got, wantKeys) {
				t.Errorf("version members = %v, want exactly %v", got, wantKeys)
			}
			data, ok := v["data"].(map[string]any)
			if !ok {
				t.Fatalf("data missing or not an object: %v", v)
			}
			if data["_type"] != tc.wantType {
				t.Errorf("data._type = %v, want %s", data["_type"], tc.wantType)
			}
			ca, ok := v["commit_audit"].(map[string]any)
			if !ok {
				t.Fatalf("commit_audit missing: %v", v)
			}
			if got := codeOf(ca, "change_type"); got != "523" {
				t.Errorf("commit_audit.change_type code = %q, want 523", got)
			}
			if got := codeOf(v, "lifecycle_state"); got != "523" {
				t.Errorf("lifecycle_state code = %q, want 523 (a deletion defaults to deleted)", got)
			}
		})
	}
}

// TestBuilderDeletionLifecycleOverride — a per-version [WithLifecycleState]
// replaces a deletion's `deleted` default, with a payload and without one, and
// the rubric beside the code is the pin's own. The change type stays `deleted`:
// the override names a lifecycle state, not an operation. REQ-130.
func TestBuilderDeletionLifecycleOverride(t *testing.T) {
	const preceding = "8849182c-82ad-4088-a07f-48ead4180515::cdr.example::1"
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	cases := []struct {
		name     string
		change   contribution.Change
		wantCode string
		wantData bool
	}{
		{
			name:     "complete with a payload",
			change:   contribution.Deletion(preceding, &comp, contribution.WithLifecycleState(ehr.LifecycleStateComplete)),
			wantCode: "532",
			wantData: true,
		},
		{
			name:     "complete without a payload",
			change:   contribution.Deletion[rm.Composition](preceding, nil, contribution.WithLifecycleState(ehr.LifecycleStateComplete)),
			wantCode: "532",
		},
		{
			name:     "incomplete without a payload",
			change:   contribution.Deletion[rm.Composition](preceding, nil, contribution.WithLifecycleState(ehr.LifecycleStateIncomplete)),
			wantCode: "553",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub, err := newBuilder().Add(tc.change).Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			_, versions := marshalSubmission(t, sub)
			v := versions[0]
			if got := codeOf(v, "lifecycle_state"); got != tc.wantCode {
				t.Errorf("lifecycle_state code = %q, want %q", got, tc.wantCode)
			}
			wantRubric, ok := terminology.VersionLifecycleState.Rubric(tc.wantCode)
			if !ok {
				t.Fatalf("code %q is not in the pinned version-lifecycle-state group, so this table row is stale", tc.wantCode)
			}
			if got := valueOf(v, "lifecycle_state"); got != wantRubric {
				t.Errorf("lifecycle_state value = %q, want the pinned rubric %q", got, wantRubric)
			}
			if _, has := v["data"]; has != tc.wantData {
				t.Errorf("data member present = %v, want %v", has, tc.wantData)
			}
			ca, ok := v["commit_audit"].(map[string]any)
			if !ok {
				t.Fatalf("commit_audit missing: %v", v)
			}
			if got := codeOf(ca, "change_type"); got != "523" {
				t.Errorf("commit_audit.change_type code = %q, want 523 (the override must not change the operation)", got)
			}
		})
	}
}

// TestBuilderDeletionWithoutPayloadDecodesIntoRM — the marshalled version of
// a deletion built without a payload decodes into an rm.OriginalVersion whose
// `data` is nil, so a reader of the SDK's own output sees a Void payload and
// not an empty COMPOSITION. REQ-130.
func TestBuilderDeletionWithoutPayloadDecodesIntoRM(t *testing.T) {
	const preceding = "8849182c-82ad-4088-a07f-48ead4180515::cdr.example::1"
	sub, err := newBuilder().Add(contribution.Deletion[rm.Composition](preceding, nil)).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := sub.Validate(); err != nil {
		t.Fatalf("Validate refused a version with no data: %v", err)
	}
	b, err := canjson.Marshal(sub)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	var body struct {
		Versions []json.RawMessage `json:"versions"`
	}
	if err := json.Unmarshal(b, &body); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, b)
	}
	if len(body.Versions) != 1 {
		t.Fatalf("len(versions) = %d, want 1", len(body.Versions))
	}
	var ov rm.OriginalVersion[rm.Composition]
	if err := canjson.Unmarshal(body.Versions[0], &ov); err != nil {
		t.Fatalf("decode into rm.OriginalVersion[rm.Composition]: %v\n%s", err, body.Versions[0])
	}
	if ov.Data != nil {
		t.Errorf("decoded Data = %+v, want nil (the deletion carries no data)", ov.Data)
	}
	if ov.PrecedingVersionUID == nil || ov.PrecedingVersionUID.Value != preceding {
		t.Errorf("decoded PrecedingVersionUID = %+v, want %q", ov.PrecedingVersionUID, preceding)
	}
	if got := ov.LifecycleState.DefiningCode.CodeString; got != "523" {
		t.Errorf("decoded lifecycle_state code = %q, want 523", got)
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
	const preceding = "8849182c-82ad-4088-a07f-48ead4180515::cdr.example::1"
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
		// Only a deletion may be given no payload (REQ-130 § Deletion): each
		// of the other three operations is still refused at Build.
		{
			name:    "creation with a nil payload",
			builder: newBuilder().Add(contribution.Creation[rm.Composition](nil)),
			want:    "nil data",
		},
		{
			name:    "amendment with a nil payload",
			builder: newBuilder().Add(contribution.Amendment[rm.Composition](preceding, nil)),
			want:    "nil data",
		},
		{
			name:    "modification with a nil payload",
			builder: newBuilder().Add(contribution.Modification[rm.Composition](preceding, nil)),
			want:    "nil data",
		},
		{
			name:    "zero Change",
			builder: newBuilder().Add(contribution.Change{}),
			want:    "zero Change",
		},
		{
			name:    "unknown batch change_type code",
			builder: newBuilder().WithChangeType(contribution.ChangeType("999")).Add(contribution.Creation(&comp)),
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

// TestBuilderDeletionWithoutPayloadRefusals — a deletion given no payload is
// exempt from the nil-data refusal and from nothing else: it still needs the
// version it deletes, and its lifecycle state must still be a code of the
// openEHR version-lifecycle-state group. Each refusal is for its own reason,
// never for the missing payload. REQ-130.
func TestBuilderDeletionWithoutPayloadRefusals(t *testing.T) {
	const preceding = "8849182c-82ad-4088-a07f-48ead4180515::cdr.example::1"
	cases := []struct {
		name   string
		change contribution.Change
		want   string
	}{
		{
			name:   "no preceding uid",
			change: contribution.Deletion[rm.Composition]("", nil),
			want:   "preceding version uid",
		},
		{
			name:   "unknown lifecycle code",
			change: contribution.Deletion[rm.Composition](preceding, nil, contribution.WithLifecycleState(ehr.LifecycleState("999"))),
			want:   "version-lifecycle-state",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub, err := newBuilder().Add(tc.change).Build()
			if err == nil {
				t.Fatalf("Build accepted the deletion, want an error mentioning %q: %+v", tc.want, sub)
			}
			if sub != nil {
				t.Error("Build returned a submission alongside an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "nil data") {
				t.Errorf("error = %q, a deletion is not refused for having no payload", err)
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

// TestBuilderReportsEveryAuditGapInOnePass — `change_type` and `committer`
// are both required on the pin's write-side audit DTO, so one Build reports
// both. Leaving the committer to Submission.Validate would defer it to a
// second Build, because Validate runs only once this error set is empty.
// REQ-130.
func TestBuilderReportsEveryAuditGapInOnePass(t *testing.T) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	sub, err := contribution.NewBuilder().Add(contribution.Creation(&comp)).Build()
	if err == nil {
		t.Fatalf("Build succeeded with neither change_type nor committer: %+v", sub)
	}
	if sub != nil {
		t.Error("Build returned a submission alongside an error")
	}
	for _, want := range []string{"change_type is required", "committer is nil"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q in the same pass", err, want)
		}
	}
}

// TestBuilderBuildIsIdempotentOnTheErrorPath — the success path is covered
// by [TestBuilderBuildIsIdempotent]; this is its refusal twin. A builder
// carrying both a builder-level error (an unknown batch code) and a
// change-level one (an amendment with no preceding uid) must report the
// same set on every Build, and the set must not grow with each attempt.
//
// What this does NOT pin: the `slices.Clone(b.errs)` in Build. That clone
// is defensive hygiene, not a fix for a reachable defect — appending into
// the spare capacity of b.errs writes past its length, which nothing reads,
// and errors.Join materialises the values into a new error, so a returned
// error never aliases the builder. Reverting the clone keeps this test (and
// its success-path twin) green; verified by mutation.
func TestBuilderBuildIsIdempotentOnTheErrorPath(t *testing.T) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	b := contribution.NewBuilder().
		WithCommitterName("alice").
		WithChangeType(contribution.ChangeType("999")).
		Add(contribution.Amendment("", &comp))
	first, err1 := b.Build()
	second, err2 := b.Build()
	if err1 == nil || err2 == nil {
		t.Fatalf("Build must refuse both times: %v / %v", err1, err2)
	}
	if first != nil || second != nil {
		t.Error("Build returned a submission alongside an error")
	}
	if err1.Error() != err2.Error() {
		t.Errorf("error set differs between Builds:\nfirst:  %v\nsecond: %v", err1, err2)
	}
}

// TestBuilderWithAuditCarriesAGroupMember — a wholesale [Builder.WithAudit] is
// the caller's path for a batch audit the operation constructors do not build
// (a committer with identifiers, a synthesis batch, …). It still ships, as
// long as its change_type is a group member carrying the pinned rubric: here
// 252 "synthesis", which no Creation/Amendment/Modification/Deletion sets.
func TestBuilderWithAuditCarriesAGroupMember(t *testing.T) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	name := "alice"
	sub, err := contribution.NewBuilder().
		WithAudit(contribution.UpdateAudit{
			Committer: &rm.PartyIdentified{Name: &name},
			ChangeType: rm.DVCodedText{
				DVText:       rm.DVText{Value: "synthesis"},
				DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "252"},
			},
		}).
		Add(contribution.Creation(&comp)).
		Build()
	if err != nil {
		t.Fatalf("Build with a wholesale audit carrying group member 252: %v", err)
	}
	audit, versions := marshalSubmission(t, sub)
	if got := codeOf(audit, "change_type"); got != "252" {
		t.Errorf("audit.change_type = %q, want 252", got)
	}
	if got := valueOf(audit, "change_type"); got != "synthesis" {
		t.Errorf("audit.change_type value = %q, want the pinned rubric %q", got, "synthesis")
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

// TestBuilderWithAuditRefusesANonGroupChangeType — the openEHR
// AUDIT_DETAILS.Change_type_valid invariant admits only members of the *audit
// change type* group under the `openehr` terminology, so a wholesale
// [Builder.WithAudit] carrying a non-member code is refused at Build with the
// same force [Builder.WithChangeType] applies: the builder never ships an
// RM-invalid audit, whichever path set the change type (REQ-034; wire.md
// § REQ-130). A caller who genuinely wants an off-spec audit hand-wires a
// Submission instead.
func TestBuilderWithAuditRefusesANonGroupChangeType(t *testing.T) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	name := "alice"
	sub, err := contribution.NewBuilder().
		WithAudit(contribution.UpdateAudit{
			Committer: &rm.PartyIdentified{Name: &name},
			ChangeType: rm.DVCodedText{
				DVText:       rm.DVText{Value: "custom"},
				DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "999"},
			},
		}).
		Add(contribution.Creation(&comp)).
		Build()
	if err == nil {
		t.Fatalf("Build accepted a wholesale audit with non-group change_type 999: %+v", sub)
	}
	if sub != nil {
		t.Error("Build returned a submission alongside the refusal")
	}
	if !strings.Contains(err.Error(), "999") {
		t.Errorf("refusal does not name the offending code 999: %v", err)
	}
}

// TestBuilderWithAuditRefusesAnOffSpecChangeType pins the two Build-time arms
// a non-member code never reaches: a group member carrying a hand-typed
// rubric, and an openEHR code declared under a foreign terminology. Each is an
// AUDIT_DETAILS.Change_type_valid violation the wholesale WithAudit path must
// not ship, and each row's inputs satisfy every other arm, so the refusal it
// asserts can only come from the arm under test — deleting the rubric check or
// the terminology-id check fails exactly one row (REQ-034, REQ-130).
func TestBuilderWithAuditRefusesAnOffSpecChangeType(t *testing.T) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	name := "alice"
	cases := []struct {
		name        string
		terminology string
		code, value string
		wantFacets  []string // substrings the refusal must carry
	}{
		{
			name:        "group member with a hand-typed rubric",
			terminology: "openehr",
			code:        "252",
			value:       "custom",
			wantFacets:  []string{`"252"`, "pinned rubric", `"synthesis"`, `"custom"`},
		},
		{
			name:        "openEHR code under a foreign terminology",
			terminology: "SNOMED-CT",
			code:        "249",
			value:       "creation",
			wantFacets:  []string{`"249"`, "coded in terminology", `"SNOMED-CT"`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub, err := contribution.NewBuilder().
				WithAudit(contribution.UpdateAudit{
					Committer: &rm.PartyIdentified{Name: &name},
					ChangeType: rm.DVCodedText{
						DVText:       rm.DVText{Value: tc.value},
						DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: tc.terminology}, CodeString: tc.code},
					},
				}).
				Add(contribution.Creation(&comp)).
				Build()
			if err == nil {
				t.Fatalf("Build accepted a wholesale audit with change_type %s::%s|%s: %+v", tc.terminology, tc.code, tc.value, sub)
			}
			if sub != nil {
				t.Error("Build returned a submission alongside the refusal")
			}
			for _, want := range tc.wantFacets {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refusal for %s::%s|%s = %q, want it to carry %q", tc.terminology, tc.code, tc.value, err, want)
				}
			}
		})
	}
}

// TestWithChangeTypeAdmitsEveryGroupMemberWithItsRubric — REQ-034: the batch
// audit accepts every member of the pinned openEHR *audit change type* group,
// and the rubric that reaches the wire is the pin's own, never one typed
// beside the code in this SDK.
func TestWithChangeTypeAdmitsEveryGroupMemberWithItsRubric(t *testing.T) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	for c := range terminology.AuditChangeType.All() {
		t.Run(c.Code, func(t *testing.T) {
			sub, err := contribution.NewBuilder().
				WithCommitterName("alice").
				WithChangeType(contribution.ChangeType(c.Code)).
				Add(contribution.Creation(&comp)).
				Build()
			if err != nil {
				t.Fatalf("Build with batch change type %s (%s): %v", c.Code, c.Rubric, err)
			}
			audit, _ := marshalSubmission(t, sub)
			if got := codeOf(audit, "change_type"); got != c.Code {
				t.Errorf("audit.change_type code = %q, want %q", got, c.Code)
			}
			if got := termOf(audit, "change_type"); got != terminology.ID {
				t.Errorf("audit.change_type terminology = %q, want %q", got, terminology.ID)
			}
			if got := valueOf(audit, "change_type"); got != c.Rubric {
				t.Errorf("audit.change_type value = %q, want the pinned rubric %q", got, c.Rubric)
			}
		})
	}
}

// TestVersionLifecycleStateCarriesThePinnedRubric — REQ-034, the lifecycle twin
// of [TestWithChangeTypeAdmitsEveryGroupMemberWithItsRubric]: a version's
// `lifecycle_state` is a DV_CODED_TEXT the SDK builds from a code, so every
// member of the pinned openEHR *version lifecycle state* group must reach the
// wire carrying the pin's own rubric as its `value` — never a string typed
// beside the code in this SDK, and never an empty one.
func TestVersionLifecycleStateCarriesThePinnedRubric(t *testing.T) {
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	for c := range terminology.VersionLifecycleState.All() {
		t.Run(c.Code, func(t *testing.T) {
			sub, err := newBuilder().
				Add(contribution.Creation(&comp, contribution.WithLifecycleState(ehr.LifecycleState(c.Code)))).
				Build()
			if err != nil {
				t.Fatalf("Build with WithLifecycleState(%q): %v", c.Code, err)
			}
			_, versions := marshalSubmission(t, sub)
			if len(versions) != 1 {
				t.Fatalf("WithLifecycleState(%q): len(versions) = %d, want 1", c.Code, len(versions))
			}
			v := versions[0]
			if got := codeOf(v, "lifecycle_state"); got != c.Code {
				t.Errorf("WithLifecycleState(%q): lifecycle_state code = %q, want %q", c.Code, got, c.Code)
			}
			if got := termOf(v, "lifecycle_state"); got != "openehr" {
				t.Errorf("WithLifecycleState(%q): lifecycle_state terminology = %q, want %q", c.Code, got, "openehr")
			}
			if got := valueOf(v, "lifecycle_state"); got != c.Rubric {
				t.Errorf("WithLifecycleState(%q): lifecycle_state value = %q, want the pinned rubric %q", c.Code, got, c.Rubric)
			}
		})
	}
}

// TestChangeTypeConstantsCoverTheGroup — REQ-034: the promoted constants MUST
// be exactly the group's members, so a member the pin carries is always
// nameable and no constant outlives its concept.
func TestChangeTypeConstantsCoverTheGroup(t *testing.T) {
	want := map[contribution.ChangeType]bool{
		contribution.ChangeTypeCreation:         true,
		contribution.ChangeTypeAmendment:        true,
		contribution.ChangeTypeModification:     true,
		contribution.ChangeTypeSynthesis:        true,
		contribution.ChangeTypeDeleted:          true,
		contribution.ChangeTypeAttestation:      true,
		contribution.ChangeTypeRestoration:      true,
		contribution.ChangeTypeFormatConversion: true,
		contribution.ChangeTypeUnknown:          true,
	}
	for c := range terminology.AuditChangeType.All() {
		if !want[contribution.ChangeType(c.Code)] {
			t.Errorf("group member %s (%s) has no ChangeType constant", c.Code, c.Rubric)
		}
	}
	if len(want) != terminology.AuditChangeType.Len() {
		t.Errorf("%d constants, group has %d members", len(want), terminology.AuditChangeType.Len())
	}
}

// TestChangeTypeCodedTextOutsideTheGroupIsZero — a code the pin does not
// carry has no rubric to render, so [contribution.ChangeType.CodedText]
// reports the absence as the zero DV_CODED_TEXT rather than fabricating a
// coded value with an empty rubric (REQ-034).
func TestChangeTypeCodedTextOutsideTheGroupIsZero(t *testing.T) {
	outside := contribution.ChangeType("999")
	if got := outside.CodedText(); !reflect.DeepEqual(got, rm.DVCodedText{}) {
		t.Errorf("ChangeType(%q).CodedText() = %+v, want the zero DVCodedText", outside, got)
	}
	if rubric, ok := outside.Rubric(); ok {
		t.Errorf("ChangeType(%q).Rubric() = %q, true — want absence reported", outside, rubric)
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
