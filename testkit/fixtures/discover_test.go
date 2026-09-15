package fixtures_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func TestListCompositionJSON_excludesRobotEHRStatusInvalid(t *testing.T) {
	rels, err := fixtures.ListCompositionJSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range rels {
		if rel.Kind != "rm" {
			continue
		}
		if len(rel.Template) >= len("ehr_status_invalid_") &&
			rel.Template[:len("ehr_status_invalid_")] == "ehr_status_invalid_" {
			t.Errorf("invalid ehr_status fixture must not be in probe list: %s", rel.Rel)
		}
		if rel.Template == "ehr_status_valid_000_ehr_status_ecis" {
			t.Errorf("ECIS alternate-wire fixture must not be in probe list: %s", rel.Rel)
		}
	}
}

// TestListCompositionJSON_includesFormerlyExcludedCompositions pins that the
// seven cassettes once held out on the withdrawn byte-stability rationale
// (ruling R33) are now in the corpus. Dropping any one from ListCompositionJSON
// turns this red.
func TestListCompositionJSON_includesFormerlyExcludedCompositions(t *testing.T) {
	rels, err := fixtures.ListCompositionJSON()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, rel := range rels {
		got[rel.Template] = true
	}
	for _, stem := range []string{
		"Address.v2",
		"Demonstration.v1",
		"TestPerson.v2",
		"Test_dv_interval_dv_count_lower_upper_constraint.v0",
		"Test_dv_interval_dv_count_open_constraint.v0",
		"Test_dv_interval_dv_quantity_lower_upper_constraint.v0",
		"Test_dv_interval_dv_quantity_open_constraint.v0",
	} {
		if !got[stem] {
			t.Errorf("composition %q missing from ListCompositionJSON; it must be in the corpus", stem)
		}
	}
}
