package templatecompile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// Set SCAN_EHRBASE_TMP=1 to validate candidate pairs under testkit/ehrbase_sdk.tmp.
func TestEhrbaseSDKTmpPairs(t *testing.T) {
	if os.Getenv("SCAN_EHRBASE_TMP") == "" {
		t.Skip("set SCAN_EHRBASE_TMP=1 to run")
	}
	root := filepath.Join("..", "..", "testkit", "ehrbase_sdk.tmp")
	if _, err := os.Stat(root); err != nil {
		t.Skip("testkit/ehrbase_sdk.tmp not present (integrated into testkit/cassettes/)")
	}
	type pair struct {
		opt  string
		comp string
	}
	pairs := []pair{
		{"operational_templates/cluster-slot.ehrbase.org.v0.opt", "composition/cluster-slot.ehrbase.or.v0.json"},
		{"operational_templates/name-test.ehrbase.org.v0.opt", "composition/name-test.json"},
		{"operational_templates/nested.v1.opt", "other-test-data/composition/canonical_json/nested.en.v1.json"},
		{"operational_templates/IDCR - Adverse Reaction List.v1.opt", "composition/IDCR - Adverse Reaction List.v1.xml"},
		{"operational_templates/IDCR Problem List.v1.opt", "composition/IDCR Problem List.v1.xml"},
		{"operational_templates/IDCR-LaboratoryTestReport.opt", "composition/IDCR-LabReportRAW1.xml"},
		{"operational_templates/RIPPLE-ConformanceTest.opt", "composition/RIPPLE-ConformanceTest.xml"},
		{"operational_templates/Test all types.opt", "composition/test_all_types.fixed.v1.xml"},
	}
	for _, p := range pairs {
		t.Run(filepath.Base(p.opt)+"+"+filepath.Base(p.comp), func(t *testing.T) {
			optPath := filepath.Join(root, p.opt)
			compPath := filepath.Join(root, p.comp)
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
				if _, err := canjson.Marshal(&c); err != nil {
					t.Fatalf("json round-trip: %v", err)
				}
			case ".xml":
				if err := canxml.Unmarshal(raw, &c); err != nil {
					t.Fatalf("xml decode: %v", err)
				}
				if _, err := canxml.Marshal(&c); err != nil {
					t.Fatalf("xml round-trip: %v", err)
				}
			default:
				t.Fatalf("unknown ext %s", compPath)
			}
			t.Logf("template_id=%s", opt.TemplateID())
		})
	}
}
