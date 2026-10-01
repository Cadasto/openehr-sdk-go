package instance_test

import (
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
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
