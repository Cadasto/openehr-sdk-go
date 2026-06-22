package validation_test

import (
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-103 — vendored Robot Test_dv_* compositions must not violate OPT
// primitive constraints. Full REQ-102 validation may still report structural
// issues (slot_fill, rm_type_mismatch on LOCATABLE.name, …) until those
// codec/validator gaps close; this test pins constraint conformance only.
func TestValidateComposition_ConstraintCassettes_NoPrimitiveViolations(t *testing.T) {
	ids, err := fixtures.ConstraintTemplateIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("no constraint template cassettes discovered")
	}
	// constraintViolatingCassettes are vendored cassettes whose instance
	// genuinely violates its OPT primitive constraints — excluded from the
	// "no violations" assertion because the violation is correct, not a
	// validator gap.
	constraintViolatingCassettes := map[string]string{
		// OPT pins media_type to a closed code_list [application/pdf]
		// (despite the "open_constraint" name) while the instance carries
		// application/dicom. Surfaced once the REQ-110 DV_MULTIMEDIA
		// media_type reader let the constraint run; the genuine violation
		// is asserted positively in
		// TestValidateComposition_ConstraintCassette_MultimediaViolation.
		"Test_dv_multimedia_open_constraint.v0": "media_type application/dicom not in closed list [application/pdf]",
		// OPT pins false_valid=false while the instance carries false.
		// Surfaced once INTEGER/BOOLEAN AOM short-name channels validate
		// through DV wrapper scalar attrs (SDK-GAP-12 rmread path).
		"Test_dv_boolean_true_false.v0": "value false not allowed",
		// OPT pins magnitude range [10..20] while the instance carries 25.
		"Test_dv_count_range_constraint.v0": "magnitude 25 outside [10..20]",
	}
	for _, id := range ids {
		if _, skip := constraintViolatingCassettes[id]; skip {
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

// REQ-110 — the DV_MULTIMEDIA media_type reader lets the OPT's CODE_PHRASE
// constraint run. Test_dv_multimedia_open_constraint.v0 pins media_type to
// a closed [application/pdf] list while its instance carries
// application/dicom; the validator must catch the violation rather than
// silently skip the (previously unreadable) media_type attribute.
func TestValidateComposition_ConstraintCassette_MultimediaViolation(t *testing.T) {
	const id = "Test_dv_multimedia_open_constraint.v0"
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
	found := false
	for _, issue := range r.Issues {
		if strings.HasPrefix(issue.Code, "primitive_") && strings.HasSuffix(issue.Path, "/media_type") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a primitive media_type violation, got %+v", r.Issues)
	}
}

// REQ-110 — BOOLEAN AOM short-name on a DV wrapper scalar channel must
// validate against the OPT's C_BOOLEAN constraint. Test_dv_boolean_true_false.v0
// pins false_valid=false while the instance carries false.
func TestValidateComposition_ConstraintCassette_BooleanViolation(t *testing.T) {
	const id = "Test_dv_boolean_true_false.v0"
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
	found := false
	for _, issue := range r.Issues {
		if strings.HasPrefix(issue.Code, "primitive_") && strings.Contains(issue.Detail, "false not allowed") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a primitive boolean violation, got %+v", r.Issues)
	}
}

// REQ-110 — INTEGER magnitude on a DV_COUNT scalar channel must validate
// against the OPT range constraint. Test_dv_count_range_constraint.v0 pins
// magnitude [10..20] while the instance carries 25.
func TestValidateComposition_ConstraintCassette_CountRangeViolation(t *testing.T) {
	const id = "Test_dv_count_range_constraint.v0"
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
	found := false
	for _, issue := range r.Issues {
		if strings.HasPrefix(issue.Code, "primitive_") && strings.Contains(issue.Detail, "outside [10..20]") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a primitive count range violation, got %+v", r.Issues)
	}
}
