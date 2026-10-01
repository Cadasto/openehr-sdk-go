package instance_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// REQ-107 — every generated ELEMENT carries exactly one of value and
// null_flavour (RM Inv_null_flavour_indicated), under both policies and
// both fills.
func TestREQ107_GeneratedElementsCarryExactlyOneOfValueAndNullFlavour(t *testing.T) {
	templates := []string{"vital_signs", "Demonstration.v1", "BMI", "body_weight"}
	policies := []struct {
		name string
		p    instance.Policy
	}{{"Minimal", instance.Minimal}, {"Example", instance.Example}}
	fills := []struct {
		name string
		f    instance.ValueFill
	}{{"ExampleFill", instance.ExampleFill}, {"RandomFill", instance.RandomFill}}

	for _, tpl := range templates {
		for _, pol := range policies {
			for _, fill := range fills {
				t.Run(tpl+"/"+pol.name+"/"+fill.name, func(t *testing.T) {
					c := compileFixture(t, tpl)
					out, err := instance.Generate(t.Context(), c, instance.Options{
						Policy:    pol.p,
						Language:  "en",
						Territory: "NL",
						Composer:  testComposer(),
						Now:       time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC),
						ValueFill: fill.f,
					})
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					for _, iss := range validation.ValidateRM(out).Issues {
						if iss.Code == "rm_invariant" && strings.Contains(iss.Detail, "Inv_null_flavour_indicated") {
							t.Errorf("ValidateRM %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
						}
					}
				})
			}
		}
	}
}

// REQ-107 — a null flavour the generator adds is a DV_CODED_TEXT whose code is
// in the openEHR "null flavours" group, with that code's rubric as its text,
// and it sits on an ELEMENT that has no value.
func TestREQ107_GeneratedNullFlavourIsFromTheOpenEHRGroup(t *testing.T) {
	c := compileFixture(t, "Demonstration.v1")
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
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	seen := 0
	var walk func(n any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]any:
			if nf, ok := v["null_flavour"].(map[string]any); ok {
				seen++
				checkNullFlavour(t, v, nf)
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(tree)
	if seen == 0 {
		t.Fatal("no ELEMENT with a null_flavour in the generated Demonstration.v1; the template has OPT-silent elements")
	}
}

func checkNullFlavour(t *testing.T, element, nf map[string]any) {
	t.Helper()
	if _, has := element["value"]; has {
		t.Errorf("ELEMENT %v carries both value and null_flavour", element["archetype_node_id"])
	}
	code, _ := nf["defining_code"].(map[string]any)
	id, _ := code["terminology_id"].(map[string]any)
	if id["value"] != terminology.ID {
		t.Errorf("null_flavour terminology = %v, want %q", id["value"], terminology.ID)
	}
	codeString, _ := code["code_string"].(string)
	rubric, ok := terminology.NullFlavours.Rubric(codeString)
	if !ok {
		t.Errorf("null_flavour code %q is not in the openEHR null flavours group", codeString)
		return
	}
	if nf["value"] != rubric {
		t.Errorf("null_flavour value = %v, want rubric %q of code %s", nf["value"], rubric, codeString)
	}
}
