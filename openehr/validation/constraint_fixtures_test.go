package validation_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-103 — vendored Robot Test_dv_* compositions must not violate OPT
// primitive constraints. Full composition validation may still report
// structural issues (slot_fill, rm_type_mismatch on LOCATABLE.name, …) until those
// codec/validator gaps close; this test pins constraint conformance only.
func TestValidateComposition_ConstraintFixtures_NoPrimitiveViolations(t *testing.T) {
	ids, err := fixtures.ConstraintTemplateIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("no constraint template fixtures discovered")
	}
	// constraintViolatingFixtures are vendored fixtures whose instance
	// genuinely violates its OPT primitive constraints — excluded from the
	// "no violations" assertion because the violation is correct, not a
	// validator gap.
	constraintViolatingFixtures := map[string]string{
		// OPT pins media_type to a closed code_list [application/pdf]
		// (despite the "open_constraint" name) while the instance carries
		// application/dicom. Surfaced once the REQ-110 DV_MULTIMEDIA
		// media_type reader let the constraint run; the genuine violation
		// is asserted positively in
		// TestValidateComposition_ConstraintFixture_MultimediaViolation.
		"Test_dv_multimedia_open_constraint.v0": "DV_MULTIMEDIA media_type outside the closed C_CODE_PHRASE list",
		// OPT pins false_valid=false while the instance carries false.
		// Surfaced once INTEGER/BOOLEAN AOM short-name channels validate
		// through DV wrapper scalar attrs (REQ-110 rmread path).
		"Test_dv_boolean_true_false.v0": "DV_BOOLEAN value the C_BOOLEAN does not allow",
		// OPT pins magnitude range [10..20] while the instance carries 25.
		"Test_dv_count_range_constraint.v0": "DV_COUNT magnitude outside the C_INTEGER range",
		// OPT pins formalism to [text/plain] while the instance carries abc.
		// Surfaced once STRING became an AOM primitive short name (REQ-107).
		// Pinned in TestValidateComposition_ConstraintFixture_ParsableViolation.
		"Test_dv_parsable_open_constraint.v0": "DV_PARSABLE formalism outside the closed C_STRING list",
		// OPT name lists are shorter than the instance's runtime names, and
		// one list entry is itself misspelled. Surfaced with the STRING check.
		// Pinned in TestValidateComposition_ConstraintFixture_ClinicalContentViolations.
		"clinical_content_validation": "name/value not in the OPT's closed list",
	}
	for _, id := range ids {
		if _, skip := constraintViolatingFixtures[id]; skip {
			continue
		}
		t.Run(id, func(t *testing.T) {
			c := mustCompile(t, id)
			raw, err := os.ReadFile(fixtures.CompositionJSON(id))
			if err != nil {
				t.Fatal(err)
			}
			var comp rm.Composition
			if err := canjson.Unmarshal(raw, &comp); err != nil {
				t.Fatalf("decode composition: %v", err)
			}
			r := validation.ValidateComposition(&comp, c)
			var primitive []validation.Issue
			for _, issue := range r.Issues {
				if strings.HasPrefix(issue.Code, "primitive_") {
					primitive = append(primitive, issue)
				}
			}
			if len(primitive) == 0 {
				return
			}
			for _, issue := range primitive {
				t.Logf("primitive issue: %s: %s — %s", issue.Path, issue.Code, issue.Detail)
			}
			t.Fatalf("%d primitive constraint violation(s), want 0", len(primitive))
		})
	}
}

// REQ-103 and REQ-110: the DV_MULTIMEDIA media_type reader lets the OPT's
// C_CODE_PHRASE constraint run instead of skipping the attribute.
// Test_dv_multimedia_open_constraint.v0 pins media_type to the closed code
// list [application/pdf] while its instance carries application/dicom. The
// instance has three events, all at at0002, so the one path is reported
// three times, and the report is exactly those three violations.
func TestValidateComposition_ConstraintFixture_MultimediaViolation(t *testing.T) {
	const id = "Test_dv_multimedia_open_constraint.v0"
	const mediaType = "/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0028]/value/media_type"
	want := []string{
		"primitive_not_in_list " + mediaType,
		"primitive_not_in_list " + mediaType,
		"primitive_not_in_list " + mediaType,
	}
	assertFixtureIssues(t, id, want)
}

// REQ-103 and REQ-110: a BOOLEAN AOM short name on a DV wrapper's scalar
// channel validates against the OPT's C_BOOLEAN constraint.
// Test_dv_boolean_true_false.v0 pins false_valid=false while its instance
// carries false. The instance has three events, all at at0002, so the one
// path is reported three times, and the report is exactly those three
// violations.
func TestValidateComposition_ConstraintFixture_BooleanViolation(t *testing.T) {
	const id = "Test_dv_boolean_true_false.v0"
	const value = "/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0029]/value/value"
	want := []string{
		"primitive_not_in_list " + value,
		"primitive_not_in_list " + value,
		"primitive_not_in_list " + value,
	}
	assertFixtureIssues(t, id, want)
}

// REQ-103 and REQ-110: an INTEGER magnitude on DV_COUNT's scalar channel
// validates against the OPT's C_INTEGER range.
// Test_dv_count_range_constraint.v0 pins magnitude to [10..20] while its
// instance carries 25. The instance has three events, all at at0002, so the
// one path is reported three times, and the report is exactly those three
// violations.
func TestValidateComposition_ConstraintFixture_CountRangeViolation(t *testing.T) {
	const id = "Test_dv_count_range_constraint.v0"
	const magnitude = "/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0042]/value/magnitude"
	want := []string{
		"primitive_out_of_range " + magnitude,
		"primitive_out_of_range " + magnitude,
		"primitive_out_of_range " + magnitude,
	}
	assertFixtureIssues(t, id, want)
}

// REQ-103: Test_dv_parsable_open_constraint.v0 pins DV_PARSABLE.formalism to
// the closed list [text/plain] (the C_STRING sets no list_open), while its
// instance carries abc. The instance has three events, all at at0002, and
// each holds the same element, so the one path is reported three times.
// The report is exactly those three violations.
func TestValidateComposition_ConstraintFixture_ParsableViolation(t *testing.T) {
	const id = "Test_dv_parsable_open_constraint.v0"
	const formalism = "/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0028]/value/formalism"
	want := []string{
		"primitive_not_in_list " + formalism,
		"primitive_not_in_list " + formalism,
		"primitive_not_in_list " + formalism,
	}
	assertFixtureIssues(t, id, want)
}

// REQ-102 and REQ-103: clinical_content_validation's report is exactly these
// eight issues, each a real defect of the instance against its OPT:
//
//   - four EVALUATION names outside the OPT's one-entry closed lists (the
//     C_STRING sets no list_open, and the v1 entry is itself misspelled
//     "evaliation");
//   - ITEM_TABLE.rotated, which the OPT requires (existence 1..1) but which
//     the pinned RM does not define, so no instance can carry it;
//   - three content items whose archetype ids are not among the OPT's
//     content children (the OPT names valiadation_instruction_test,
//     validation_action_test and validation_observation_test).
func TestValidateComposition_ConstraintFixture_ClinicalContentViolations(t *testing.T) {
	const id = "clinical_content_validation"
	want := []string{
		"primitive_not_in_list /content[openEHR-EHR-EVALUATION.validation_evaliation_test.v0]/name/value",
		"primitive_not_in_list /content[openEHR-EHR-EVALUATION.validation_evaliation_test.v2]/name/value",
		"primitive_not_in_list /content[openEHR-EHR-EVALUATION.validation_evaliation_test.v1]/name/value",
		"required /content[openEHR-EHR-EVALUATION.validation_evaliation_test.v3]/data/rotated",
		"primitive_not_in_list /content[openEHR-EHR-EVALUATION.validation_evaliation_test.v3]/name/value",
		"slot_fill /content[openEHR-EHR-INSTRUCTION.instruction_test.v0]",
		"slot_fill /content[openEHR-EHR-ACTION.action_test.v0]",
		"slot_fill /content[openEHR-EHR-OBSERVATION.observation_test.v0]",
	}
	assertFixtureIssues(t, id, want)
}

// assertFixtureIssues validates the vendored composition of template id
// against its OPT and fails unless the issues, each read as "code path",
// are exactly want in any order.
func assertFixtureIssues(t *testing.T, id string, want []string) {
	t.Helper()
	c := mustCompile(t, id)
	raw, err := os.ReadFile(fixtures.CompositionJSON(id))
	if err != nil {
		t.Fatal(err)
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(raw, &comp); err != nil {
		t.Fatalf("decode composition: %v", err)
	}
	r := validation.ValidateComposition(&comp, c)
	got := make([]string, 0, len(r.Issues))
	for _, issue := range r.Issues {
		got = append(got, issue.Code+" "+issue.Path)
	}
	slices.Sort(got)
	want = slices.Sorted(slices.Values(want))
	if !slices.Equal(got, want) {
		t.Errorf("ValidateComposition(%s) issues (code path):\n got  %q\n want %q\nfull issues: %+v", id, got, want, r.Issues)
	}
}
