package contribution_test

import (
	"encoding/json"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// buildOriginalVersionRM returns a minimal rm.OriginalVersion[rm.Composition]
// with a time_committed in the commit_audit — used to verify that
// WrapOriginalVersion drops it.
func buildOriginalVersionRM() *rm.OriginalVersion[rm.Composition] {
	name := "carol"
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	return &rm.OriginalVersion[rm.Composition]{
		Version: rm.Version[rm.Composition]{
			CommitAudit: rm.AuditDetails{
				SystemID:  "cdr.example",
				Committer: &rm.PartyIdentified{Name: &name},
				ChangeType: rm.DVCodedText{
					DVText:       rm.DVText{Value: "creation"},
					DefiningCode: rm.CodePhrase{CodeString: "249"},
				},
				TimeCommitted: rm.DVDateTime{Value: "2026-05-17T12:00:00Z"},
			},
		},
		UID:            rm.ObjectVersionID{Value: "42::cdr.example::1"},
		LifecycleState: rm.DVCodedText{DVText: rm.DVText{Value: "complete"}, DefiningCode: rm.CodePhrase{CodeString: "532"}},
		Data:           &comp,
	}
}

// buildImportedVersionRM returns a minimal rm.ImportedVersion[rm.Composition].
func buildImportedVersionRM() *rm.ImportedVersion[rm.Composition] {
	name := "dave"
	inner := rm.OriginalVersion[any]{
		Version: rm.Version[any]{
			CommitAudit: rm.AuditDetails{
				SystemID:  "cdr.example",
				Committer: &rm.PartyIdentified{Name: &name},
				ChangeType: rm.DVCodedText{
					DVText:       rm.DVText{Value: "import"},
					DefiningCode: rm.CodePhrase{CodeString: "240"},
				},
				TimeCommitted: rm.DVDateTime{Value: "2026-05-18T09:00:00Z"},
			},
		},
		UID:            rm.ObjectVersionID{Value: "7::remote.example::1"},
		LifecycleState: rm.DVCodedText{DVText: rm.DVText{Value: "complete"}, DefiningCode: rm.CodePhrase{CodeString: "532"}},
	}
	return &rm.ImportedVersion[rm.Composition]{
		Version: rm.Version[rm.Composition]{
			CommitAudit: rm.AuditDetails{
				SystemID:  "cdr.example",
				Committer: &rm.PartyIdentified{Name: &name},
				ChangeType: rm.DVCodedText{
					DVText:       rm.DVText{Value: "import"},
					DefiningCode: rm.CodePhrase{CodeString: "240"},
				},
				TimeCommitted: rm.DVDateTime{Value: "2026-05-18T09:00:00Z"},
			},
		},
		Item: inner,
	}
}

// marshalToMap is a test helper that marshals v to JSON and decodes it
// into map[string]any, failing the test on any error.
func marshalToMap(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	return m
}

// TestOriginalVersionBMMName verifies the BMMName marker.
func TestOriginalVersionBMMName(t *testing.T) {
	ov := contribution.WrapOriginalVersion(buildOriginalVersionRM())
	if got := ov.BMMName(); got != "ORIGINAL_VERSION" {
		t.Errorf("BMMName = %q, want ORIGINAL_VERSION", got)
	}
}

// TestImportedVersionBMMName verifies the BMMName marker.
func TestImportedVersionBMMName(t *testing.T) {
	iv := contribution.WrapImportedVersion(buildImportedVersionRM())
	if got := iv.BMMName(); got != "IMPORTED_VERSION" {
		t.Errorf("BMMName = %q, want IMPORTED_VERSION", got)
	}
}

// TestOriginalVersionMarshalType checks that the wrapper emits
// _type:"ORIGINAL_VERSION" at the top level.
func TestOriginalVersionMarshalType(t *testing.T) {
	ov := contribution.WrapOriginalVersion(buildOriginalVersionRM())
	m := marshalToMap(t, ov)
	if got := m["_type"]; got != "ORIGINAL_VERSION" {
		t.Errorf("_type = %v, want ORIGINAL_VERSION", got)
	}
}

// TestImportedVersionMarshalType checks that the wrapper emits
// _type:"IMPORTED_VERSION" at the top level.
func TestImportedVersionMarshalType(t *testing.T) {
	iv := contribution.WrapImportedVersion(buildImportedVersionRM())
	m := marshalToMap(t, iv)
	if got := m["_type"]; got != "IMPORTED_VERSION" {
		t.Errorf("_type = %v, want IMPORTED_VERSION", got)
	}
}

// TestOriginalVersionCommitAuditType verifies that commit_audit._type is
// "AUDIT_DETAILS" (the SDK default for UpdateAudit).
func TestOriginalVersionCommitAuditType(t *testing.T) {
	ov := contribution.WrapOriginalVersion(buildOriginalVersionRM())
	m := marshalToMap(t, ov)
	ca, ok := m["commit_audit"].(map[string]any)
	if !ok {
		t.Fatalf("commit_audit is not an object: %v", m["commit_audit"])
	}
	if got := ca["_type"]; got != "AUDIT_DETAILS" {
		t.Errorf("commit_audit._type = %v, want AUDIT_DETAILS", got)
	}
}

// TestOriginalVersionCommitAuditNoTimeCommitted verifies that
// commit_audit has no time_committed — the key result of wrapping.
func TestOriginalVersionCommitAuditNoTimeCommitted(t *testing.T) {
	ov := contribution.WrapOriginalVersion(buildOriginalVersionRM())
	m := marshalToMap(t, ov)
	ca, ok := m["commit_audit"].(map[string]any)
	if !ok {
		t.Fatalf("commit_audit is not an object: %v", m["commit_audit"])
	}
	if _, has := ca["time_committed"]; has {
		t.Errorf("commit_audit must not contain time_committed (WrapOriginalVersion must drop it), got %v", ca["time_committed"])
	}
}

// TestOriginalVersionCommitAuditChangeTypeDVCodedText guards the
// pointer-marshal fix: change_type must carry _type:"DV_CODED_TEXT".
func TestOriginalVersionCommitAuditChangeTypeDVCodedText(t *testing.T) {
	ov := contribution.WrapOriginalVersion(buildOriginalVersionRM())
	m := marshalToMap(t, ov)
	ca, ok := m["commit_audit"].(map[string]any)
	if !ok {
		t.Fatalf("commit_audit is not an object: %v", m["commit_audit"])
	}
	ct, ok := ca["change_type"].(map[string]any)
	if !ok {
		t.Fatalf("commit_audit.change_type is not an object: %v", ca["change_type"])
	}
	if got := ct["_type"]; got != "DV_CODED_TEXT" {
		t.Errorf("commit_audit.change_type._type = %v, want DV_CODED_TEXT (pointer-marshal regression?)", got)
	}
	if _, has := ct["defining_code"]; !has {
		t.Errorf("commit_audit.change_type missing defining_code — want DV_CODED_TEXT shape")
	}
}

// TestOriginalVersionCommitAuditCommitterType verifies that committer
// emits its _type discriminator when set as a pointer.
func TestOriginalVersionCommitAuditCommitterType(t *testing.T) {
	ov := contribution.WrapOriginalVersion(buildOriginalVersionRM())
	m := marshalToMap(t, ov)
	ca, ok := m["commit_audit"].(map[string]any)
	if !ok {
		t.Fatalf("commit_audit is not an object: %v", m["commit_audit"])
	}
	c, ok := ca["committer"].(map[string]any)
	if !ok {
		t.Fatalf("commit_audit.committer is not an object: %v", ca["committer"])
	}
	if got := c["_type"]; got != "PARTY_IDENTIFIED" {
		t.Errorf("commit_audit.committer._type = %v, want PARTY_IDENTIFIED", got)
	}
}

// TestOriginalVersionDataPresent verifies that the inline data payload
// is marshalled and its _type is present.
func TestOriginalVersionDataPresent(t *testing.T) {
	ov := contribution.WrapOriginalVersion(buildOriginalVersionRM())
	m := marshalToMap(t, ov)
	data, ok := m["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing or not an object: %v", m["data"])
	}
	if got := data["_type"]; got != "COMPOSITION" {
		t.Errorf("data._type = %v, want COMPOSITION", got)
	}
}

// TestOriginalVersionLifecycleStateDefiningCode verifies that
// lifecycle_state is marshalled as a DV_CODED_TEXT (has defining_code).
func TestOriginalVersionLifecycleStateDefiningCode(t *testing.T) {
	ov := contribution.WrapOriginalVersion(buildOriginalVersionRM())
	m := marshalToMap(t, ov)
	ls, ok := m["lifecycle_state"].(map[string]any)
	if !ok {
		t.Fatalf("lifecycle_state is not an object: %v", m["lifecycle_state"])
	}
	if _, has := ls["defining_code"]; !has {
		t.Errorf("lifecycle_state missing defining_code — want DV_CODED_TEXT shape, got %v", ls)
	}
}

// TestWrapOriginalVersionDropsTimeCommitted is the canonical regression
// guard: the source rm.AuditDetails has a time_committed; WrapOriginalVersion
// must drop it from the marshalled commit_audit.
func TestWrapOriginalVersionDropsTimeCommitted(t *testing.T) {
	base := buildOriginalVersionRM()
	// Confirm the base RM struct carries a non-empty time_committed.
	ad, ok := rm.AuditDetailsBase(base.CommitAudit)
	if !ok || ad.TimeCommitted.Value == "" {
		t.Fatal("test precondition: base rm.AuditDetails must have a non-empty TimeCommitted")
	}
	ov := contribution.WrapOriginalVersion(base)
	m := marshalToMap(t, ov)
	ca, ok := m["commit_audit"].(map[string]any)
	if !ok {
		t.Fatalf("commit_audit is not an object: %v", m["commit_audit"])
	}
	if _, has := ca["time_committed"]; has {
		t.Errorf("WrapOriginalVersion must drop time_committed from commit_audit, got %v", ca["time_committed"])
	}
}

// TestImportedVersionCommitAuditNoTimeCommitted mirrors the
// ORIGINAL_VERSION check for the IMPORTED_VERSION wrapper.
func TestImportedVersionCommitAuditNoTimeCommitted(t *testing.T) {
	iv := contribution.WrapImportedVersion(buildImportedVersionRM())
	m := marshalToMap(t, iv)
	ca, ok := m["commit_audit"].(map[string]any)
	if !ok {
		t.Fatalf("commit_audit is not an object: %v", m["commit_audit"])
	}
	if _, has := ca["time_committed"]; has {
		t.Errorf("ImportedVersion commit_audit must not contain time_committed, got %v", ca["time_committed"])
	}
}

// TestWrapOriginalVersionIgnoresLaterVersionAudit pins the documented
// contract: WrapOriginalVersion snapshots the audit at wrap time, so a
// later mutation of the wrapped rm Version.CommitAudit does NOT change the
// emitted commit_audit.
func TestWrapOriginalVersionIgnoresLaterVersionAudit(t *testing.T) {
	base := buildOriginalVersionRM() // change_type code_string "249"
	ov := contribution.WrapOriginalVersion(base)
	// Mutate the underlying rm version's audit after wrapping.
	base.CommitAudit = rm.AuditDetails{
		Committer:  &rm.PartyIdentified{},
		ChangeType: rm.DVCodedText{DVText: rm.DVText{Value: "deleted"}, DefiningCode: rm.CodePhrase{CodeString: "999"}},
	}
	m := marshalToMap(t, ov)
	ca, _ := m["commit_audit"].(map[string]any)
	ct, _ := ca["change_type"].(map[string]any)
	dc, _ := ct["defining_code"].(map[string]any)
	if dc["code_string"] == "999" {
		t.Errorf("commit_audit reflects a post-wrap Version.CommitAudit mutation; want the snapshot taken at Wrap time")
	}
}

// TestImportedVersionItemType guards that the nested imported
// ORIGINAL_VERSION (item) keeps its _type discriminator — item is an
// rm.OriginalVersion[any] value field, so a value-vs-pointer-receiver
// regression would silently drop it.
func TestImportedVersionItemType(t *testing.T) {
	iv := contribution.WrapImportedVersion(buildImportedVersionRM())
	m := marshalToMap(t, iv)
	item, ok := m["item"].(map[string]any)
	if !ok {
		t.Fatalf("item is not an object: %v", m["item"])
	}
	if got := item["_type"]; got != "ORIGINAL_VERSION" {
		t.Errorf("item._type = %v, want ORIGINAL_VERSION", got)
	}
}

// TestSubmissionWithWrappedVersions verifies that a Submission containing
// a wrapped OriginalVersion marshals correctly: top-level keys, audit
// shape, and per-version shape.
func TestSubmissionWithWrappedVersions(t *testing.T) {
	ov := contribution.WrapOriginalVersion(buildOriginalVersionRM())
	sub := &contribution.Submission{
		Audit:    ov.CommitAudit,
		Versions: []contribution.CommitVersion{ov},
	}
	m := marshalToMap(t, sub)

	if _, has := m["_type"]; has {
		t.Errorf("Submission must not emit top-level _type (Contribution_create schema), got %v", m["_type"])
	}
	audit, ok := m["audit"].(map[string]any)
	if !ok {
		t.Fatalf("audit is not an object: %v", m["audit"])
	}
	if got := audit["_type"]; got != "AUDIT_DETAILS" {
		t.Errorf("audit._type = %v, want AUDIT_DETAILS", got)
	}
	if _, has := audit["time_committed"]; has {
		t.Errorf("audit must not contain time_committed, got %v", audit["time_committed"])
	}

	versions, ok := m["versions"].([]any)
	if !ok || len(versions) != 1 {
		t.Fatalf("versions is not a 1-element array: %v", m["versions"])
	}
	v0, ok := versions[0].(map[string]any)
	if !ok {
		t.Fatalf("versions[0] is not an object: %v", versions[0])
	}
	if got := v0["_type"]; got != "ORIGINAL_VERSION" {
		t.Errorf("versions[0]._type = %v, want ORIGINAL_VERSION", got)
	}
	ca0, ok := v0["commit_audit"].(map[string]any)
	if !ok {
		t.Fatalf("versions[0].commit_audit is not an object: %v", v0["commit_audit"])
	}
	if _, has := ca0["time_committed"]; has {
		t.Errorf("versions[0].commit_audit must not contain time_committed, got %v", ca0["time_committed"])
	}
}
