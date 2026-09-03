package validation_test

// rmfloor_test.go: unit pins for REQ-112 — the template-less
// Reference Model validation floor. Each test exercises
// one cassette in the PROBE-077 matrix:
//
//   - structurally-decodable but RM-invalid roots must surface the
//     invariant violation with a path and a stable code;
//   - structurally-valid roots must report OK;
//   - the typed sugars guard against nil and typed-nil roots without
//     panicking, mirroring REQ-110's nil_* contract.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
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
	if !containsIssue(r.Issues, "/name", "required") {
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
		if i.Code == "rm_invariant" && strings.Contains(i.Detail, "DV_INTERVAL") {
			t.Errorf("unexpected rm_invariant on unbounded DV_INTERVAL: %s", i.Detail)
		}
	}
}

// TestValidateRM_DVIntervalDifferentUnitsSkipped: DV_QUANTITY bounds are only
// strictly comparable at the same unit, so a cross-unit interval (10 mg vs
// 5 kg) has no magnitude ordering and the floor must NOT assert lower>upper on
// the raw magnitudes (10 > 5). The bound-ordering invariant is skipped.
func TestValidateRM_DVIntervalDifferentUnitsSkipped(t *testing.T) {
	iv := rm.DVInterval[rm.DVQuantity]{
		Interval: rm.Interval[rm.DVQuantity]{
			Lower:         rm.DVQuantity{Magnitude: 10, Units: "mg"},
			Upper:         rm.DVQuantity{Magnitude: 5, Units: "kg"},
			LowerIncluded: true,
			UpperIncluded: true,
		},
	}
	r := validation.ValidateRM(&iv)
	for _, i := range r.Issues {
		if i.Code == "rm_invariant" && strings.Contains(i.Detail, "DV_INTERVAL") {
			t.Errorf("cross-unit DV_INTERVAL must not be flagged lower>upper: %s", i.Detail)
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
	if !containsIssue(r.Issues, "/items[0]/type", "rm_invariant") {
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

// TestRMFloorTermMappingMatchValueSet covers the TERM_MAPPING.match value
// set: a match outside {'>', '=', '<', '?'} surfaces `term_mapping_match`
// at the offending mapping's path (REQ-112). Table-driven over three
// out-of-set shapes: an arbitrary letter, the empty string (rm.Character's
// zero value — the most likely bad input in practice, e.g. an
// unpopulated field), and a two-character near-miss.
func TestRMFloorTermMappingMatchValueSet(t *testing.T) {
	cases := []struct {
		name  string
		match rm.Character
	}{
		{name: "letter", match: rm.Character("x")},
		{name: "empty", match: rm.Character("")},
		{name: "two_chars", match: rm.Character("==")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			txt := &rm.DVText{Value: "x", Mappings: []rm.TermMapping{{
				Match:  tc.match, // not in {> = < ?}
				Target: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "1"},
			}}}
			r := validation.ValidateRM(txt)
			if r.OK {
				t.Fatalf("expected an invariant issue for match=%q", tc.match)
			}
			if !containsIssue(r.Issues, "/mappings[0]/match", "term_mapping_match") {
				t.Errorf("expected term_mapping_match at /mappings[0]/match, got %+v", r.Issues)
			}
		})
	}
}

// TestValidateRM_TermMappingValid is the positive-path regression guard
// for the value-set check, pairing TestRMFloorTermMappingMatchValueSet per
// this file's established convention (e.g. TestValidateRM_CodePhraseValid
// pairs TestValidateRM_CodePhraseEmptyCodeString): a DV_TEXT carrying one
// TermMapping per accepted match code (`>`, `=`, `<`, `?`), each with a
// fully-populated Target CODE_PHRASE, exercises every non-default arm of
// checkTermMappings's switch and must report clean — no issues at all
// (REQ-112).
func TestValidateRM_TermMappingValid(t *testing.T) {
	var mappings []rm.TermMapping
	for _, match := range []rm.Character{">", "=", "<", "?"} {
		mappings = append(mappings, rm.TermMapping{
			Match:  match,
			Target: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "73211009"},
		})
	}
	txt := &rm.DVText{Value: "x", Mappings: mappings}
	r := validation.ValidateRM(txt)
	if !r.OK {
		t.Errorf("ValidateRM(DV_TEXT with valid term mappings) want OK; got %+v", r.Issues)
	}
	if len(r.Issues) != 0 {
		t.Errorf("ValidateRM(DV_TEXT with valid term mappings) want no issues at all; got %+v", r.Issues)
	}
}

// TestRMFloorMappingsNotEmptyIfPresent covers Mappings_valid: a
// present-but-empty DV_TEXT.mappings (a Go-value-literal empty slice,
// mirroring a decoded literal `"mappings":[]` on the wire) surfaces
// `mappings_valid` at /mappings (REQ-112).
func TestRMFloorMappingsNotEmptyIfPresent(t *testing.T) {
	txt := &rm.DVText{Value: "x", Mappings: []rm.TermMapping{}} // present, empty -> invalid
	r := validation.ValidateRM(txt)
	if r.OK {
		t.Fatalf("expected a Mappings_valid issue for a present-but-empty mappings")
	}
	if !containsIssue(r.Issues, "/mappings", "mappings_valid") {
		t.Errorf("expected mappings_valid at /mappings, got %+v", r.Issues)
	}
}

// TestRMFloorMappingsAbsentIsValid is the no-false-positive guard: a
// DV_TEXT decoded from JSON with no `mappings` key at all decodes to a
// nil slice (verified on this branch — absent and JSON `null` both
// collapse to nil; only a literal `[]` decodes non-nil-empty), which the
// floor must not flag (REQ-112).
func TestRMFloorMappingsAbsentIsValid(t *testing.T) {
	var txt rm.DVText
	if err := json.Unmarshal([]byte(`{"_type":"DV_TEXT","value":"x"}`), &txt); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	r := validation.ValidateRM(&txt)
	if !r.OK {
		t.Errorf("ValidateRM(DV_TEXT, mappings absent) want OK; got %+v", r.Issues)
	}
	if containsCode(r.Issues, "mappings_valid") {
		t.Errorf("absent mappings must not be flagged mappings_valid; got %+v", r.Issues)
	}
}

// TestRMFloorMappingsEmptyLiteralIsInvalid is the decode-path twin of
// TestRMFloorMappingsNotEmptyIfPresent: a DV_TEXT decoded from a literal
// `"mappings":[]` on the wire decodes to a non-nil empty slice and the
// floor MUST flag it (REQ-112).
func TestRMFloorMappingsEmptyLiteralIsInvalid(t *testing.T) {
	var txt rm.DVText
	if err := json.Unmarshal([]byte(`{"_type":"DV_TEXT","value":"x","mappings":[]}`), &txt); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	r := validation.ValidateRM(&txt)
	if r.OK {
		t.Fatalf("expected a mappings_valid issue for a decoded literal empty mappings array")
	}
	if !containsIssue(r.Issues, "/mappings", "mappings_valid") {
		t.Errorf("expected mappings_valid at /mappings, got %+v", r.Issues)
	}
}

// TestRMFloorDVCodedTextBadMatch covers the inheritance path: DV_CODED_TEXT
// carries `mappings` via its embedded DV_TEXT, and the term_mapping_match
// invariant MUST apply there too (REQ-112).
func TestRMFloorDVCodedTextBadMatch(t *testing.T) {
	badMatch := rm.Character("q") // not in {> = < ?}
	txt := &rm.DVCodedText{
		DVText: rm.DVText{
			Value: "x",
			Mappings: []rm.TermMapping{{
				Match:  badMatch,
				Target: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "1"},
			}},
		},
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "2"},
	}
	r := validation.ValidateRM(txt)
	if r.OK {
		t.Fatalf("expected a term_mapping_match issue for DV_CODED_TEXT with match=%q", badMatch)
	}
	if !containsIssue(r.Issues, "/mappings[0]/match", "term_mapping_match") {
		t.Errorf("expected term_mapping_match at /mappings[0]/match, got %+v", r.Issues)
	}
}

// TestValidateRM_TermMappingRoot pins REQ-112's "walks any RM root":
// TERM_MAPPING is a registered RM concrete, so a stand-alone TERM_MAPPING
// handed to the generic entry MUST be recognised and MUST report
// `term_mapping_match` at /match when its match is outside {> = < ?}.
// Before the node-level evaluator the match check lived only on the
// parent DV_TEXT, so a TERM_MAPPING root validated clean.
func TestValidateRM_TermMappingRoot(t *testing.T) {
	tm := &rm.TermMapping{
		Match:  rm.Character("x"), // not in {> = < ?}
		Target: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "73211009"},
	}
	r := validation.ValidateRM(tm)
	if r.OK {
		t.Fatalf("ValidateRM(TERM_MAPPING with match=%q) should not be OK; issues=%+v", tm.Match, r.Issues)
	}
	if containsCode(r.Issues, "rm_type_unknown") {
		t.Errorf("TERM_MAPPING must be a recognised RM root, got rm_type_unknown: %+v", r.Issues)
	}
	if !containsIssue(r.Issues, "/match", "term_mapping_match") {
		t.Errorf("expected term_mapping_match at /match, got %+v", r.Issues)
	}
}

// TestValidateRM_TermMappingRootValid is the positive twin: a valid
// stand-alone TERM_MAPPING reports clean — no issues at all. Guards the
// new walk into TERM_MAPPING's own attributes against false positives on
// `match` / `target` (and the optional `purpose`).
func TestValidateRM_TermMappingRootValid(t *testing.T) {
	tm := &rm.TermMapping{
		Match:  rm.Character("="),
		Target: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "73211009"},
	}
	r := validation.ValidateRM(tm)
	if !r.OK || len(r.Issues) != 0 {
		t.Errorf("ValidateRM(valid TERM_MAPPING) want OK with no issues; got %+v", r.Issues)
	}
}

// TestValidateRM_TermMappingMissingTarget pins the consequence of walking
// into TERM_MAPPING: `target` is RM-mandatory (CODE_PHRASE), so a mapping
// that omits it surfaces `required` at the mapping's /target — the floor's
// required-set walk now reaches inside each mapping (REQ-112).
func TestValidateRM_TermMappingMissingTarget(t *testing.T) {
	txt := &rm.DVText{Value: "x", Mappings: []rm.TermMapping{{
		Match: rm.Character("="),
		// Target intentionally omitted.
	}}}
	r := validation.ValidateRM(txt)
	if r.OK {
		t.Fatalf("ValidateRM(DV_TEXT with a mapping missing target) should not be OK; issues=%+v", r.Issues)
	}
	if !containsIssue(r.Issues, "/mappings[0]/target", "required") {
		t.Errorf("expected required at /mappings[0]/target, got %+v", r.Issues)
	}
}

// TestValidateRM_NestedPurposeMappingBadMatch pins the nested reach the
// node-level evaluator buys: TERM_MAPPING.purpose is a DV_CODED_TEXT, which
// carries `mappings` of its own. A bad match two levels down MUST surface
// at its full path — the parent-only evaluator never looked there (REQ-112).
func TestValidateRM_NestedPurposeMappingBadMatch(t *testing.T) {
	target := rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "73211009"}
	txt := &rm.DVText{Value: "x", Mappings: []rm.TermMapping{{
		Match: rm.Character("="),
		Purpose: &rm.DVCodedText{
			DVText: rm.DVText{
				Value: "billing",
				Mappings: []rm.TermMapping{{
					Match:  rm.Character("x"), // not in {> = < ?}
					Target: target,
				}},
			},
			DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "532"},
		},
		Target: target,
	}}}
	r := validation.ValidateRM(txt)
	if r.OK {
		t.Fatalf("ValidateRM(DV_TEXT with a bad nested purpose mapping) should not be OK; issues=%+v", r.Issues)
	}
	const want = "/mappings[0]/purpose/mappings[0]/match"
	if !containsIssue(r.Issues, want, "term_mapping_match") {
		t.Errorf("expected term_mapping_match at %s, got %+v", want, r.Issues)
	}
}

// TestValidateRM_NestedPurposeMappingValid is the no-false-positive twin
// of TestValidateRM_NestedPurposeMappingBadMatch: the same nesting with a
// valid nested match reports clean — no issues at all.
func TestValidateRM_NestedPurposeMappingValid(t *testing.T) {
	target := rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "73211009"}
	txt := &rm.DVText{Value: "x", Mappings: []rm.TermMapping{{
		Match: rm.Character("="),
		Purpose: &rm.DVCodedText{
			DVText: rm.DVText{
				Value:    "billing",
				Mappings: []rm.TermMapping{{Match: rm.Character("?"), Target: target}},
			},
			DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "532"},
		},
		Target: target,
	}}}
	r := validation.ValidateRM(txt)
	if !r.OK || len(r.Issues) != 0 {
		t.Errorf("ValidateRM(DV_TEXT with a valid nested purpose mapping) want OK with no issues; got %+v", r.Issues)
	}
}

// TestValidateRM_TermMappingEmptyMatchReportsBoth locks the deliberate
// double report on an empty `match`: since the walk enters TERM_MAPPING,
// the RM-mandatory attribute reads as absent (rm.Character's zero value is
// not a legal Character) — so the floor emits BOTH `required` and
// `term_mapping_match` at the same path. Two distinct codes for two
// distinct statements; consumers dispatch on Code (REQ-112).
func TestValidateRM_TermMappingEmptyMatchReportsBoth(t *testing.T) {
	txt := &rm.DVText{Value: "x", Mappings: []rm.TermMapping{{
		Match:  rm.Character(""), // RM-mandatory, and outside {> = < ?}
		Target: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "73211009"},
	}}}
	r := validation.ValidateRM(txt)
	const path = "/mappings[0]/match"
	if !containsIssue(r.Issues, path, "term_mapping_match") {
		t.Errorf("expected term_mapping_match at %s, got %+v", path, r.Issues)
	}
	if !containsIssue(r.Issues, path, "required") {
		t.Errorf("expected required at %s, got %+v", path, r.Issues)
	}
}

// TestRMFloorMappingsNullIsValid pins the decode fact the Mappings_valid
// invariant rests on: an explicit JSON `null` mappings decodes to a nil
// slice — indistinguishable from an absent key — so the floor must NOT
// report `mappings_valid`. Decoded through canjson (the wire decoder
// consumers use) rather than encoding/json, so the pin covers the path
// the claim in checkTermMappings is about (REQ-112 / REQ-052).
func TestRMFloorMappingsNullIsValid(t *testing.T) {
	var txt rm.DVText
	if err := canjson.Unmarshal([]byte(`{"_type":"DV_TEXT","value":"x","mappings":null}`), &txt); err != nil {
		t.Fatalf("canjson.Unmarshal: %v", err)
	}
	if txt.Mappings != nil {
		t.Errorf("null mappings decoded to a non-nil slice (%#v) — the floor's absent/null reading depends on nil", txt.Mappings)
	}
	r := validation.ValidateRM(&txt)
	if !r.OK {
		t.Errorf("ValidateRM(DV_TEXT, mappings null) want OK; got %+v", r.Issues)
	}
	if containsCode(r.Issues, "mappings_valid") {
		t.Errorf("null mappings must not be flagged mappings_valid; got %+v", r.Issues)
	}
}

// TestRMFloorDVCodedTextMappingsEmptyLiteralIsInvalid is the DV_CODED_TEXT
// half of the Mappings_valid pair: DV_CODED_TEXT inherits `mappings` from
// DV_TEXT, so a literal `"mappings":[]` on a DV_CODED_TEXT body decodes to
// a non-nil empty slice and MUST report `mappings_valid` at /mappings. The
// match value set was already pinned on DV_CODED_TEXT
// (TestRMFloorDVCodedTextBadMatch); mappings_valid was not (REQ-112).
func TestRMFloorDVCodedTextMappingsEmptyLiteralIsInvalid(t *testing.T) {
	const body = `{"_type":"DV_CODED_TEXT","value":"x",` +
		`"defining_code":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"},"code_string":"532"},` +
		`"mappings":[]}`
	var txt rm.DVCodedText
	if err := canjson.Unmarshal([]byte(body), &txt); err != nil {
		t.Fatalf("canjson.Unmarshal: %v", err)
	}
	r := validation.ValidateRM(&txt)
	if r.OK {
		t.Fatalf("expected a mappings_valid issue for a DV_CODED_TEXT with a literal empty mappings array")
	}
	if !containsIssue(r.Issues, "/mappings", "mappings_valid") {
		t.Errorf("expected mappings_valid at /mappings, got %+v", r.Issues)
	}
}
