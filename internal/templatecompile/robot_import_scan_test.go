package templatecompile_test

import (
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// robotRoot is the tests/robot/_resources/test_data_sets directory of an
// ehrbase/integration-tests clone: ROBOT_ROOT when set, as for the two
// Robot ingest scripts, else that directory of a sibling clone, four
// levels up from internal/templatecompile. Its callers reach it only
// after opting in with SCAN_ROBOT=1, so a missing directory fails the
// test instead of skipping it.
func robotRoot(t *testing.T) string {
	t.Helper()
	root := os.Getenv("ROBOT_ROOT")
	if root == "" {
		root = filepath.Join("..", "..", "..", "..", "ehrbase", "integration-tests", "tests", "robot", "_resources", "test_data_sets")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(abs)
	if err == nil && !info.IsDir() {
		err = errors.New("not a directory")
	}
	if err != nil {
		t.Fatalf("SCAN_ROBOT is set but the robot test data at %s is unusable: %v; set ROBOT_ROOT to the tests/robot/_resources/test_data_sets directory of an ehrbase/integration-tests clone", abs, err)
	}
	return abs
}

type robotPair struct {
	optRel  string
	compRel string
}

// Set SCAN_ROBOT=1 to validate candidate ingest pairs from ehrbase Robot fixtures.
func TestRobotImportPairs(t *testing.T) {
	if os.Getenv("SCAN_ROBOT") == "" {
		t.Skip("set SCAN_ROBOT=1 to run")
	}
	root := robotRoot(t)

	var pairs []robotPair

	// 1 — minimal entry suite
	minimal := []struct {
		opt, xml string
	}{
		{"valid_templates/minimal/minimal_evaluation.opt", "xml_compositions/minimal_evaluation.en.v1.instance_xml_input_1.xml"},
		{"valid_templates/minimal/minimal_observation.opt", "xml_compositions/minimal_observation.en.v1.instance_xml_input_1.xml"},
		{"valid_templates/minimal/minimal_admin.opt", "xml_compositions/minimal_admin.en.v1.instance_xml_input_1.xml"},
		{"valid_templates/minimal/minimal_instruction.opt", "xml_compositions/minimal_instruction.en.v1.instance_xml_input_1.xml"},
		{"valid_templates/minimal/minimal_action_2.opt", "valid_templates/minimal/minimal_action_2.instance.composition.xml"},
		{"valid_templates/minimal/minimal_action_2.opt", "valid_templates/minimal/minimal_action_2.instance.composition.json"},
	}
	for _, m := range minimal {
		pairs = append(pairs, robotPair{m.opt, m.xml})
		jsonRel := "compositions/CANONICAL_JSON/minimal_evaluation.en.v1__.json"
		if strings.Contains(m.opt, "observation") {
			// no dedicated CANONICAL for observation in robot tree — xml only
			continue
		}
		if strings.Contains(m.opt, "instruction") {
			jsonRel = "compositions/CANONICAL_JSON/minimal_instruction_1.composition.json"
		}
		if strings.Contains(m.opt, "evaluation") {
			pairs = append(pairs, robotPair{m.opt, jsonRel})
		}
	}

	// 4 — validation + Test_dv_* (canonical json stem matches template file stem)
	for _, rel := range []string{
		"valid_templates/validation/cardinality_of_section.opt",
		"valid_templates/validation/clinical_content_validation.opt",
		"valid_templates/validation/composition_evaluation_test.opt",
	} {
		stem := strings.TrimSuffix(filepath.Base(rel), ".opt")
		pairs = append(pairs, robotPair{
			rel,
			"compositions/CANONICAL_JSON/" + stem + "__full.json",
		})
	}
	dvDir := filepath.Join(root, "valid_templates/all_types")
	entries, err := os.ReadDir(dvDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "Test_dv_") || !strings.HasSuffix(e.Name(), ".opt") {
			continue
		}
		stem := strings.TrimSuffix(e.Name(), ".opt")
		jsonPath := filepath.Join(root, "compositions/CANONICAL_JSON", stem+".json")
		if _, err := os.Stat(jsonPath); err != nil {
			jsonPath = filepath.Join(root, "compositions/CANONICAL_JSON", stem+"__.json")
		}
		if _, err := os.Stat(jsonPath); err != nil {
			t.Logf("skip %s: no CANONICAL_JSON", stem)
			continue
		}
		rel, _ := filepath.Rel(root, jsonPath)
		pairs = append(pairs, robotPair{
			"valid_templates/all_types/" + e.Name(),
			rel,
		})
	}

	// 5 — persistent_minimal
	pairs = append(pairs, robotPair{
		"valid_templates/minimal_persistent/persistent_minimal.opt",
		"compositions/CANONICAL_JSON/persistent_minimal.en.v1__full.json",
	})
	pairs = append(pairs, robotPair{
		"valid_templates/minimal_persistent/persistent_minimal.opt",
		"valid_templates/minimal_persistent/persistent_minimal.composition.xml",
	})

	for _, p := range pairs {
		t.Run(filepath.Base(p.optRel)+"+"+filepath.Base(p.compRel), func(t *testing.T) {
			tryRobotPair(t, root, p)
		})
	}
}

func tryRobotPair(t *testing.T, root string, p robotPair) {
	t.Helper()
	optPath := filepath.Join(root, p.optRel)
	compPath := filepath.Join(root, p.compRel)
	opt, err := template.ParseFile(optPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := templatecompile.Compile(opt); err != nil {
		t.Fatalf("compile: %v", err)
	}
	raw, err := os.ReadFile(compPath)
	if err != nil {
		t.Fatal(err)
	}
	var c rm.Composition
	switch filepath.Ext(compPath) {
	case ".json":
		if err := canjson.Unmarshal(raw, &c); err != nil {
			t.Fatalf("json decode: %v", err)
		}
	case ".xml":
		if err := canxml.Unmarshal(raw, &c); err != nil {
			t.Fatalf("xml decode: %v", err)
		}
	default:
		t.Fatalf("unknown ext %s", compPath)
	}
	t.Logf("template_id=%q", opt.TemplateID())
}

// Set SCAN_ROBOT=1 to decode the Robot RM-only JSON (EHR_STATUS, FOLDER)
// with canjson.Unmarshal, which reads canonical RM JSON.
//
// The scan reads ehr/valid, directory and directory/update. A file with no
// _type member at its root is EHRbase's older ECIS form, not canonical RM
// JSON, and is skipped. The invalid sets are left out because they are
// invalid on purpose. The contribution sets are left out too. They are
// ITS-REST contribution submission bodies, whose versions hold inline
// ORIGINAL_VERSION objects. rm.Contribution holds its versions as
// OBJECT_REFs, and the SDK has no decoder for a submission body.
func TestRobotRMOnlyJSON(t *testing.T) {
	if os.Getenv("SCAN_ROBOT") == "" {
		t.Skip("set SCAN_ROBOT=1 to run")
	}
	root := robotRoot(t)

	var rels []string
	for _, sub := range []string{"ehr/valid", "directory", "directory/update"} {
		ents, err := os.ReadDir(filepath.Join(root, sub))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range ents {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				rels = append(rels, filepath.Join(sub, e.Name()))
			}
		}
	}

	for _, rel := range rels {
		t.Run(rel, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(root, rel))
			if err != nil {
				t.Fatal(err)
			}
			var head struct {
				Type *string `json:"_type"`
			}
			if err := json.Unmarshal(raw, &head); err != nil {
				t.Fatalf("read the root _type: %v", err)
			}
			if head.Type == nil {
				t.Skip("no _type member at the root: not canonical RM JSON (ECIS form)")
			}
			switch *head.Type {
			case "EHR_STATUS":
				var st rm.EHRStatus
				if err := canjson.Unmarshal(raw, &st); err != nil {
					t.Fatalf("EHR_STATUS: %v", err)
				}
			case "FOLDER":
				var f rm.Folder
				if err := canjson.Unmarshal(raw, &f); err != nil {
					t.Fatalf("FOLDER: %v", err)
				}
			default:
				t.Fatalf("root _type %q is neither EHR_STATUS nor FOLDER", *head.Type)
			}
		})
	}
}
