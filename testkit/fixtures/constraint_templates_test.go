package fixtures_test

import (
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
		if strings.HasPrefix(id, "Test_dv_interval_") {
			t.Errorf("interval template should be excluded: %s", id)
		}
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
