package contribution_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// Robot submission cassettes use the ehrbase CONTRIBUTION envelope (top-level
// _type) while ITS-REST Contribution_create omits it — assert the version
// inline-data shape either way.
func TestRobotSubmissionCassettes_WireShape(t *testing.T) {
	cases := []struct {
		name      string
		stem      string
		wantVers  int
		minVers   bool // when wantVers is 0, assert len>=1 unless wantEmpty
		wantEmpty bool
	}{
		{
			name:     "minimal_evaluation",
			stem:     "contributions_valid_minimal_minimal_evaluation.contribution",
			wantVers: 1,
		},
		{
			name:     "minimal_admin",
			stem:     "contributions_valid_minimal_minimal_admin.contribution",
			wantVers: 1,
		},
		{
			name:     "minimal_observation",
			stem:     "contributions_valid_minimal_minimal_observation.contribution",
			wantVers: 1,
		},
		{
			name:    "composition_and_folder",
			stem:    "contributions_valid_minimal_contribution.create_composition.create_folder",
			minVers: true,
		},
		{
			name:     "folder_creation",
			stem:     "contributions_valid_minimal_folder.contribution.creation",
			wantVers: 1,
		},
		{
			name:      "invalid_empty_versions",
			stem:      "contributions_invalid_no_versions",
			wantEmpty: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := os.ReadFile(fixtures.SubmissionJSON(tc.stem))
			if err != nil {
				t.Fatal(err)
			}
			audit, versions := assertSubmissionVersions(t, raw)
			if tc.wantEmpty {
				if len(versions) != 0 {
					t.Fatalf("len(versions) = %d, want 0", len(versions))
				}
				return
			}
			if tc.minVers {
				if len(versions) < 1 {
					t.Fatalf("len(versions) = %d, want at least 1", len(versions))
				}
			} else if len(versions) != tc.wantVers {
				t.Fatalf("len(versions) = %d, want %d", len(versions), tc.wantVers)
			}
			if audit["_type"] != "AUDIT_DETAILS" {
				t.Errorf("audit._type = %v, want AUDIT_DETAILS", audit["_type"])
			}
		})
	}
}

func assertSubmissionVersions(t *testing.T, raw []byte) (map[string]any, []map[string]any) {
	t.Helper()
	var body struct {
		Type     string           `json:"_type"`
		Audit    map[string]any   `json:"audit"`
		Versions []map[string]any `json:"versions"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if body.Type != "" && body.Type != "CONTRIBUTION" {
		t.Errorf("unexpected top-level _type %q", body.Type)
	}
	if body.Audit == nil {
		t.Fatal("missing audit object")
	}
	for i, v := range body.Versions {
		switch v["_type"] {
		case "ORIGINAL_VERSION", "IMPORTED_VERSION":
		case "OBJECT_REF":
			t.Errorf("versions[%d] is OBJECT_REF — persisted CONTRIBUTION shape, not submission", i)
		default:
			t.Errorf("versions[%d]._type = %v, want ORIGINAL_VERSION or IMPORTED_VERSION", i, v["_type"])
		}
		data, ok := v["data"].(map[string]any)
		if !ok {
			t.Fatalf("versions[%d].data missing or not an object", i)
		}
		if data["_type"] == nil || data["_type"] == "" {
			t.Errorf("versions[%d].data._type missing", i)
		}
	}
	return body.Audit, body.Versions
}
