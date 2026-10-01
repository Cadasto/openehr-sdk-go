package instance_test

import (
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-107 — generated compositions pass the RM floor on the fields
// phase 4 fills: action time, cluster items, element identity,
// ordinal symbol, and activity archetype id.
func TestREQ107_FloorGapsFilled(t *testing.T) {
	names := []string{
		"flat-conformance/conformance_ehrbase.de.v0",
		"templates/Demonstration.v1",
		"templates/minimal_action_2",
		"templates/minimal_instruction.en.v1",
		"templates/vital_signs",
		"templates/Test_dv_ordinal_open_constraint.v0",
		"templates/Test_dv_identifier_pattern_constraint.v0",
		"templates/nested.en.v1",
		"templates/TestPerson.v2",
		"templates/clinical_content_validation",
	}
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]string{}
	for _, ref := range refs {
		byName[ref.Name] = ref.Path
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			path, ok := byName[name]
			if !ok {
				t.Fatalf("fixture %s not in ListAllOPTs", name)
			}
			opt, err := template.ParseFile(path)
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			c, err := templatecompile.Compile(opt)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			out, err := instance.Generate(t.Context(), c, instance.Options{
				Policy:    instance.Example,
				Language:  "en",
				Territory: "NL",
				Composer:  testComposer(),
				Now:       time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC),
			})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			floor := validation.ValidateRM(out)
			for _, iss := range floor.Issues {
				if iss.Severity != validation.Error {
					continue
				}
				t.Errorf("ValidateRM %s %s", iss.Code, iss.Path)
			}
			var templ validation.Result
			if c.Root().RMTypeName() == "COMPOSITION" {
				comp, err := instance.AsComposition(out)
				if err != nil {
					t.Fatalf("AsComposition: %v", err)
				}
				templ = validation.ValidateComposition(comp, c)
			} else {
				templ = validation.Validate(out, c)
			}
			for _, iss := range templ.Issues {
				if iss.Severity != validation.Error {
					continue
				}
				// ITEM_TABLE.rotated is not an attribute of the pinned RM.
				if iss.Path == "/content[openEHR-EHR-EVALUATION.validation_evaliation_test.v3]/data/rotated" {
					continue
				}
				t.Errorf("template %s %s (%s)", iss.Code, iss.Path, iss.Detail)
			}
		})
	}
}
