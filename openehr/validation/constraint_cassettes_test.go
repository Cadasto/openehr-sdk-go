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
	for _, id := range ids {
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
