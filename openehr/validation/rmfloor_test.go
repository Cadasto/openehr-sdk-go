package validation_test

// rmfloor_test.go: unit pins for REQ-112 — the template-less
// Reference Model validation floor (SDK-GAP-15). Each test exercises
// one cassette in the PROBE-077 matrix:
//
//   - structurally-decodable but RM-invalid roots must surface the
//     invariant violation with a path and a stable code;
//   - structurally-valid roots must report OK;
//   - the typed sugars guard against nil and typed-nil roots without
//     panicking, mirroring REQ-110's nil_* contract.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// TestValidateRMFolder_MissingName covers the dossier's named case:
// a FOLDER whose `name` is absent surfaces `required` at "/name". The
// floor walker emits no compile-time panic on the nil interface field.
func TestValidateRMFolder_MissingName(t *testing.T) {
	folder := &rm.Folder{
		ArchetypeNodeID: "openEHR-EHR-FOLDER.generic.v1",
		// Name intentionally nil
	}
	r := validation.ValidateRMFolder(folder)
	if r.OK {
		t.Fatalf("ValidateRMFolder with missing name should not be OK; issues=%+v", r.Issues)
	}
	if !hasIssue(r.Issues, "/name", "required") {
		t.Errorf("expected required issue at /name, got %+v", r.Issues)
	}
}

// TestValidateRMFolder_Valid covers the positive path: a minimally
// well-formed FOLDER reports OK.
func TestValidateRMFolder_Valid(t *testing.T) {
	folder := &rm.Folder{
		ArchetypeNodeID: "openEHR-EHR-FOLDER.generic.v1",
		Name:            rm.DVText{Value: "root"},
	}
	r := validation.ValidateRMFolder(folder)
	if !r.OK {
		t.Errorf("ValidateRMFolder(valid) want OK; got %+v", r.Issues)
	}
}

// TestValidateRMEHRStatus_MinimallyValid covers a well-formed
// EHR_STATUS: archetype node id, name, subject (PartySelf — no required
// child attributes), is_modifiable/is_queryable (bool defaults are
// legal). Floor walker reports OK without descending into any
// invariant trap.
func TestValidateRMEHRStatus_MinimallyValid(t *testing.T) {
	status := &rm.EHRStatus{
		ArchetypeNodeID: "openEHR-EHR-EHR_STATUS.generic.v1",
		Name:            rm.DVText{Value: "EHR Status"},
		Subject:         rm.PartySelf{},
		IsModifiable:    true,
		IsQueryable:     true,
	}
	r := validation.ValidateRMEHRStatus(status)
	if !r.OK {
		t.Errorf("ValidateRMEHRStatus(valid) want OK; got %+v", r.Issues)
	}
}

// TestValidateRM_CodePhraseEmptyCodeString covers the per-type
// invariant on CODE_PHRASE: an empty code_string surfaces
// `rm_invariant` with the offending path. Surfaced via a FOLDER whose
// name carries a DVCodedText with empty defining_code.code_string —
// the floor recurses into the carried CODE_PHRASE.
func TestValidateRM_CodePhraseEmptyCodeString(t *testing.T) {
	folder := &rm.Folder{
		ArchetypeNodeID: "openEHR-EHR-FOLDER.generic.v1",
		Name: rm.DVCodedText{
			DVText: rm.DVText{Value: "labelled"},
			DefiningCode: rm.CodePhrase{
				TerminologyID: rm.TerminologyID{Value: "openehr"},
				CodeString:    "", // empty — RM floor violation
			},
		},
	}
	r := validation.ValidateRMFolder(folder)
	if r.OK {
		t.Fatalf("ValidateRMFolder with empty code_string should not be OK; issues=%+v", r.Issues)
	}
	if !containsCode(r.Issues, "rm_invariant") {
		t.Errorf("expected rm_invariant issue for empty CODE_PHRASE.code_string, got %+v", r.Issues)
	}
}

// TestValidateRM_DVQuantityPrecision covers the DV_QUANTITY.precision
// floor: precision < -1 is out of range, but -1 ("no limit") and any
// non-negative value are valid per the RM. Surfaced as a stand-alone
// DV_QUANTITY root via the generic ValidateRM entry.
func TestValidateRM_DVQuantityPrecision(t *testing.T) {
	// precision = -2 is out of range → rm_invariant.
	bad := rm.Integer(-2)
	r := validation.ValidateRM(&rm.DVQuantity{Magnitude: 1.0, Units: "mg", Precision: &bad})
	if r.OK {
		t.Fatalf("ValidateRM(DV_QUANTITY precision=-2) should not be OK; issues=%+v", r.Issues)
	}
	if !containsCode(r.Issues, "rm_invariant") {
		t.Errorf("expected rm_invariant for precision=-2, got %+v", r.Issues)
	}

	// precision = -1 means "no limit" and is valid — regression guard for
	// the formerly over-strict `< 0` check.
	noLimit := rm.Integer(-1)
	r = validation.ValidateRM(&rm.DVQuantity{Magnitude: 1.0, Units: "mg", Precision: &noLimit})
	if !r.OK {
		t.Errorf("ValidateRM(DV_QUANTITY precision=-1, no-limit) want OK; got %+v", r.Issues)
	}
}

// TestValidateRM_DVIntervalLowerGreaterThanUpper covers the DV_INTERVAL
// bound-ordering floor on the *typed* numeric instantiation RM data
// actually carries (DVInterval[DVQuantity], e.g. DV_QUANTITY.normal_range):
// lower > upper surfaces rm_invariant. Regression guard for the dispatch
// gap where rmTypeInfo reports "DV_INTERVAL<DV_QUANTITY>" while the
// catalogue only matched the bare "DV_INTERVAL" and the adapter only the
// bare DVInterval[DVOrdered] — so inverted typed intervals validated clean.
func TestValidateRM_DVIntervalLowerGreaterThanUpper(t *testing.T) {
	iv := rm.DVInterval[rm.DVQuantity]{
		Interval: rm.Interval[rm.DVQuantity]{
			Lower:         rm.DVQuantity{Magnitude: 10, Units: "mg"},
			Upper:         rm.DVQuantity{Magnitude: 5, Units: "mg"},
			LowerIncluded: true,
			UpperIncluded: true,
		},
	}
	r := validation.ValidateRM(&iv)
	if r.OK {
		t.Fatalf("ValidateRM(DVInterval[DVQuantity] lower>upper) should not be OK; issues=%+v", r.Issues)
	}
	if !containsCode(r.Issues, "rm_invariant") {
		t.Errorf("expected rm_invariant for DV_INTERVAL lower>upper, got %+v", r.Issues)
	}
}

// TestValidateRM_DVIntervalUnboundedSkipped: an unbounded side means
// the comparison is undefined; the floor walker skips the invariant
// check (it does not falsely emit rm_invariant on a half-open
// interval). Exercised on the typed instantiation.
func TestValidateRM_DVIntervalUnboundedSkipped(t *testing.T) {
	iv := rm.DVInterval[rm.DVQuantity]{
		Interval: rm.Interval[rm.DVQuantity]{
			Lower:          rm.DVQuantity{Magnitude: 10, Units: "mg"},
			UpperUnbounded: true,
		},
	}
	r := validation.ValidateRM(&iv)
	// The interval itself emits no rm_invariant (no comparable bounds).
	// Other required-set issues from descent into DV_QUANTITY are
	// allowed; this test asserts the invariant evaluator did not
	// falsely fire.
	for _, i := range r.Issues {
		if i.Code == "rm_invariant" && containsSubstring(i.Detail, "DV_INTERVAL") {
			t.Errorf("unexpected rm_invariant on unbounded DV_INTERVAL: %s", i.Detail)
		}
	}
}

// TestValidateRM_CodePhraseValid is the regression guard for the
// terminology_id false-positive: a fully-populated CODE_PHRASE validates
// cleanly. The walker formerly recursed into the flattened terminology_id
// string as a TERMINOLOGY_ID node and fabricated a `required` on its value.
func TestValidateRM_CodePhraseValid(t *testing.T) {
	cp := &rm.CodePhrase{
		TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"},
		CodeString:    "73211009",
	}
	r := validation.ValidateRM(cp)
	if !r.OK {
		t.Errorf("ValidateRM(valid CODE_PHRASE) want OK; got %+v", r.Issues)
	}
}

// TestValidateRMFolder_ObjectRefItemValid is the regression guard for the
// OBJECT_REF false-positive: a FOLDER carrying a fully-populated OBJECT_REF
// item validates cleanly. rmread does not model OBJECT_REF, so the walker
// formerly read its id/type/namespace back as absent and emitted three
// spurious `required` issues.
func TestValidateRMFolder_ObjectRefItemValid(t *testing.T) {
	folder := &rm.Folder{
		ArchetypeNodeID: "openEHR-EHR-FOLDER.generic.v1",
		Name:            rm.DVText{Value: "root"},
		Items: []rm.ObjectRefLike{rm.ObjectRef{
			ID:        &rm.HierObjectID{Value: "8849182c-82ad-4088-a07f-48ead4180515"},
			Type:      "COMPOSITION",
			Namespace: "local",
		}},
	}
	r := validation.ValidateRMFolder(folder)
	if !r.OK {
		t.Errorf("ValidateRMFolder with a valid OBJECT_REF item want OK; got %+v", r.Issues)
	}
}

// TestValidateRMFolder_ObjectRefItemMissingType covers the OBJECT_REF
// invariant via the items container: an item missing type/namespace
// surfaces rm_invariant (the evaluator, not a spurious `required`).
func TestValidateRMFolder_ObjectRefItemMissingType(t *testing.T) {
	folder := &rm.Folder{
		ArchetypeNodeID: "openEHR-EHR-FOLDER.generic.v1",
		Name:            rm.DVText{Value: "root"},
		Items: []rm.ObjectRefLike{rm.ObjectRef{
			ID: &rm.HierObjectID{Value: "abc"},
			// Type and Namespace intentionally empty.
		}},
	}
	r := validation.ValidateRMFolder(folder)
	if r.OK {
		t.Fatalf("ValidateRMFolder with OBJECT_REF missing type/namespace should not be OK; issues=%+v", r.Issues)
	}
	if !hasIssue(r.Issues, "/items[0]/type", "rm_invariant") {
		t.Errorf("expected rm_invariant at /items[0]/type, got %+v", r.Issues)
	}
}

// TestValidateRMEHRAccess_Valid is the regression guard for the EHR_ACCESS
// dispatch gap: a non-nil EHR_ACCESS is recognised and walked (returns OK)
// rather than reported as rm_type_unknown.
func TestValidateRMEHRAccess_Valid(t *testing.T) {
	access := &rm.EHRAccess{
		ArchetypeNodeID: "openEHR-EHR-EHR_ACCESS.generic.v1",
		Name:            rm.DVText{Value: "EHR Access"},
	}
	r := validation.ValidateRMEHRAccess(access)
	if !r.OK {
		t.Errorf("ValidateRMEHRAccess(valid) want OK; got %+v", r.Issues)
	}
	if containsCode(r.Issues, "rm_type_unknown") {
		t.Errorf("EHR_ACCESS should be recognised, got rm_type_unknown: %+v", r.Issues)
	}
}

// TestValidateRM_NilRoot: a nil any surfaces nil_root, not a panic.
func TestValidateRM_NilRoot(t *testing.T) {
	r := validation.ValidateRM(nil)
	if r.OK || !containsCode(r.Issues, "nil_root") {
		t.Errorf("ValidateRM(nil) want nil_root, got %+v", r.Issues)
	}
}

// TestValidateRM_TypedNilRoot: a typed-nil pointer surfaces nil_root
// (not a panic from the descent).
func TestValidateRM_TypedNilRoot(t *testing.T) {
	var folder *rm.Folder
	r := validation.ValidateRM(folder)
	if r.OK || !containsCode(r.Issues, "nil_root") {
		t.Errorf("ValidateRM(typed-nil *rm.Folder) want nil_root, got %+v", r.Issues)
	}
}

// TestValidateRM_UnknownType: a Go type outside the v2 closed RM set
// surfaces rm_type_unknown — descent cannot proceed but the walker
// reports cleanly.
func TestValidateRM_UnknownType(t *testing.T) {
	type unknownRoot struct{ X int }
	r := validation.ValidateRM(&unknownRoot{X: 1})
	if r.OK || !containsCode(r.Issues, "rm_type_unknown") {
		t.Errorf("ValidateRM(unknown type) want rm_type_unknown, got %+v", r.Issues)
	}
}

// TestValidateRMFolder_NilGuard mirrors REQ-110's nil-typed-wrapper
// contract: a nil *rm.Folder surfaces nil_folder (not nil_root) so the
// caller can distinguish wrapper-side guards from generic ones.
func TestValidateRMFolder_NilGuard(t *testing.T) {
	r := validation.ValidateRMFolder(nil)
	if r.OK || !containsCode(r.Issues, "nil_folder") {
		t.Errorf("ValidateRMFolder(nil) want nil_folder, got %+v", r.Issues)
	}
}

// TestValidateRMEHRStatus_NilGuard mirrors TestValidateRMFolder_NilGuard
// for EHR_STATUS.
func TestValidateRMEHRStatus_NilGuard(t *testing.T) {
	r := validation.ValidateRMEHRStatus(nil)
	if r.OK || !containsCode(r.Issues, "nil_ehr_status") {
		t.Errorf("ValidateRMEHRStatus(nil) want nil_ehr_status, got %+v", r.Issues)
	}
}

// TestValidateRMEHRAccess_NilGuard mirrors the EHR_ACCESS wrapper.
func TestValidateRMEHRAccess_NilGuard(t *testing.T) {
	r := validation.ValidateRMEHRAccess(nil)
	if r.OK || !containsCode(r.Issues, "nil_ehr_access") {
		t.Errorf("ValidateRMEHRAccess(nil) want nil_ehr_access, got %+v", r.Issues)
	}
}

// TestValidateRMDemographic_NilGuard: nil rm.Party interface surfaces
// nil_party from the demographic typed wrapper.
func TestValidateRMDemographic_NilGuard(t *testing.T) {
	r := validation.ValidateRMDemographic(nil)
	if r.OK || !containsCode(r.Issues, "nil_party") {
		t.Errorf("ValidateRMDemographic(nil) want nil_party, got %+v", r.Issues)
	}
}

// hasIssue is a focused predicate combining path + code matching —
// shorter than (containsCode + hand-loop) and self-documenting at call
// site.
func hasIssue(issues []validation.Issue, path, code string) bool {
	for _, i := range issues {
		if i.Path == path && i.Code == code {
			return true
		}
	}
	return false
}

// containsSubstring is a tiny helper used only by the
// unbounded-skip test where the detail message matters but the code
// alone is non-discriminating.
func containsSubstring(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
