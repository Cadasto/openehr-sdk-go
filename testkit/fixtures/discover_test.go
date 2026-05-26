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
