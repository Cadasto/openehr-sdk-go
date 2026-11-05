package fixtures_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func TestConstraintTemplateIDs_includesTestDvAndClinical(t *testing.T) {
	ids, err := fixtures.ConstraintTemplateIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) < 20 {
		t.Fatalf("got %d constraint templates, want at least 20", len(ids))
	}
	var testDv, clinical int
	for _, id := range ids {
		if id == "clinical_content_validation" {
			clinical++
		}
		if strings.HasPrefix(id, "Test_dv_") {
			testDv++
		}
	}
	if clinical != 1 {
		t.Errorf("clinical_content_validation count = %d, want 1", clinical)
	}
	if testDv < 20 {
		t.Errorf("Test_dv_* count = %d, want at least 20", testDv)
	}
}

// The four Test_dv_interval_* templates are part of the constraint-fixture
// axis, so their compositions are validated against their OPT primitive
// constraints.
func TestConstraintTemplateIDs_includesIntervalTemplates(t *testing.T) {
	ids, err := fixtures.ConstraintTemplateIDs()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Test_dv_interval_dv_count_lower_upper_constraint.v0",
		"Test_dv_interval_dv_count_open_constraint.v0",
		"Test_dv_interval_dv_quantity_lower_upper_constraint.v0",
		"Test_dv_interval_dv_quantity_open_constraint.v0",
	} {
		if !slices.Contains(ids, want) {
			t.Errorf("ConstraintTemplateIDs() does not list %s", want)
		}
	}
}
