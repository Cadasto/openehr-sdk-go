package instance_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
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

// REQ-107 — an ELEMENT carries a null_reason only while it is null (RM
// Inv_null_reason_valid: null_reason /= Void implies is_null()). The floor does
// not evaluate this invariant, so the test reads the fields. When the OPT
// constrains the value and either null attribute, the value wins and both null
// fields go; with no value the ELEMENT keeps its null_reason.
func TestREQ107_GeneratedElementHasNullReasonOnlyWhenNull(t *testing.T) {
	nullFlavour := optSingle("null_flavour", optNode("DV_CODED_TEXT", ""))
	nullReason := optSingle("null_reason", optNode("DV_TEXT", ""))
	cases := []struct {
		name       string
		opt        string
		wantValue  bool
		wantReason bool
	}{
		{
			name:      "value wins over null_flavour and null_reason",
			opt:       optTemplate("ELEMENT", optSingle("value", optNode("DV_TEXT", "")), nullFlavour, nullReason),
			wantValue: true,
		},
		{
			name:      "value wins over null_reason alone",
			opt:       optTemplate("ELEMENT", optSingle("value", optNode("DV_TEXT", "")), nullReason),
			wantValue: true,
		},
		{
			name:       "no value keeps null_reason",
			opt:        optTemplate("ELEMENT", nullFlavour, nullReason),
			wantReason: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileOPTText(t, tc.opt, true)
			for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
				for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
					t.Run(policy.String()+"/"+fill.String(), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, ValueFill: fill, Now: defaultsNow})
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						el, ok := out.(*rm.Element)
						if !ok {
							t.Fatalf("Generate returned %T, want *rm.Element", out)
						}
						if hasValue := el.Value != nil && !rm.IsTypedNil(el.Value); hasValue != tc.wantValue {
							t.Errorf("ELEMENT.value = %+v, want present: %v", el.Value, tc.wantValue)
						}
						if hasFlavour := el.NullFlavour != nil; hasFlavour == tc.wantValue {
							t.Errorf("ELEMENT.null_flavour = %+v, want present: %v", el.NullFlavour, !tc.wantValue)
						}
						if hasReason := el.NullReason != nil && !rm.IsTypedNil(el.NullReason); hasReason != tc.wantReason {
							t.Errorf("ELEMENT.null_reason = %+v, want present: %v", el.NullReason, tc.wantReason)
						}
					})
				}
			}
		})
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
