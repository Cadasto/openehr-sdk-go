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

// TestValidateRM_DVQuantityNegativePrecision covers the
// DV_QUANTITY.precision floor: a negative precision is invalid.
// Surfaced as a stand-alone DV_QUANTITY root via the generic
// ValidateRM entry — even though DV_QUANTITY is rarely a *root*, the
// floor must catch the invariant when the walker descends into one.
func TestValidateRM_DVQuantityNegativePrecision(t *testing.T) {
	neg := rm.Integer(-1)
	q := &rm.DVQuantity{
		Magnitude: 1.0,
		Units:     "mg",
		Precision: &neg,
	}
	r := validation.ValidateRM(q)
	if r.OK {
		t.Fatalf("ValidateRM(DV_QUANTITY with precision=-1) should not be OK; issues=%+v", r.Issues)
	}
	if !containsCode(r.Issues, "rm_invariant") {
		t.Errorf("expected rm_invariant for negative DV_QUANTITY.precision, got %+v", r.Issues)
	}
}

// TestValidateRM_DVIntervalLowerGreaterThanUpper covers the
// DV_INTERVAL bound-ordering floor for numerically-comparable bound
// types (DV_QUANTITY): lower > upper surfaces rm_invariant.
func TestValidateRM_DVIntervalLowerGreaterThanUpper(t *testing.T) {
	iv := rm.DVInterval[rm.DVOrdered]{
		Interval: rm.Interval[rm.DVOrdered]{
			Lower:         rm.DVQuantity{Magnitude: 10, Units: "mg"},
			Upper:         rm.DVQuantity{Magnitude: 5, Units: "mg"},
			LowerIncluded: true,
			UpperIncluded: true,
		},
	}
	r := validation.ValidateRM(&iv)
	if r.OK {
		t.Fatalf("ValidateRM(DV_INTERVAL with lower>upper) should not be OK; issues=%+v", r.Issues)
	}
	if !containsCode(r.Issues, "rm_invariant") {
		t.Errorf("expected rm_invariant for DV_INTERVAL lower>upper, got %+v", r.Issues)
	}
}

// TestValidateRM_DVIntervalUnboundedSkipped: an unbounded side means
// the comparison is undefined; the floor walker skips the invariant
// check (it does not falsely emit rm_invariant on a half-open
// interval).
func TestValidateRM_DVIntervalUnboundedSkipped(t *testing.T) {
	iv := rm.DVInterval[rm.DVOrdered]{
		Interval: rm.Interval[rm.DVOrdered]{
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
