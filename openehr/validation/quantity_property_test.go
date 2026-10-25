package validation_test

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// propertyQuantityChild is a C_DV_QUANTITY whose property is openehr::code,
// with units as its unit list: none when units is empty.
func propertyQuantityChild(code string, units ...string) string {
	var list strings.Builder
	for _, u := range units {
		list.WriteString(`
        <list><units>` + u + `</units></list>`)
	}
	return `
      <children xsi:type="C_DV_QUANTITY">
        <rm_type_name>DV_QUANTITY</rm_type_name>
        <node_id></node_id>
        <property>
          <terminology_id><value>openehr</value></terminology_id>
          <code_string>` + code + `</code_string>
        </property>` + list.String() + `
      </children>`
}

// REQ-102, REQ-103 — the template validator reports a DV_QUANTITY whose
// units are not among those of the property its C_DV_QUANTITY names, when
// the OPT lists no units: the rows of the specifications-CNF conformance
// schedule's case CONT-DV_QUANTITY-validate_property, property openehr::122
// (length). With a unit list, the list decides, as in the case
// CONT-DV_QUANTITY-validate_property_units, list [cm, m]: km is a length unit
// and is still refused. The NULL rows of both cases are the RM's, not the
// constraint's, and are left out.
func TestValidate_REQ102_REQ103_QuantityUnitsOfTheProperty(t *testing.T) {
	t.Parallel()
	refused := []pinnedIssue{templateFinding("unit_unknown", "/value")}
	tests := []struct {
		name      string
		child     string
		magnitude float64
		units     string
		want      []pinnedIssue
	}{
		{name: "property, 0.0 mg", child: propertyQuantityChild("122"), magnitude: 0, units: "mg", want: refused},
		{name: "property, 0.0 cm", child: propertyQuantityChild("122"), magnitude: 0, units: "cm"},
		{name: "property, 1.0 cm", child: propertyQuantityChild("122"), magnitude: 1, units: "cm"},
		{name: "property, 5.7 cm", child: propertyQuantityChild("122"), magnitude: 5.7, units: "cm"},
		{name: "property, 10.0 cm", child: propertyQuantityChild("122"), magnitude: 10, units: "cm"},
		{name: "property and list, 0.0 mg", child: propertyQuantityChild("122", "cm", "m"), magnitude: 0, units: "mg", want: refused},
		{name: "property and list, 0.0 cm", child: propertyQuantityChild("122", "cm", "m"), magnitude: 0, units: "cm"},
		{name: "property and list, 0.0 km", child: propertyQuantityChild("122", "cm", "m"), magnitude: 0, units: "km", want: refused},
		{name: "property and list, 1.0 cm", child: propertyQuantityChild("122", "cm", "m"), magnitude: 1, units: "cm"},
		{name: "property and list, 5.7 cm", child: propertyQuantityChild("122", "cm", "m"), magnitude: 5.7, units: "cm"},
		{name: "property and list, 10.0 cm", child: propertyQuantityChild("122", "cm", "m"), magnitude: 10, units: "cm"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, elementValueOPT(tc.child))
			value := &rm.DVQuantity{Magnitude: rm.Real(tc.magnitude), Units: tc.units}
			got := pinnedIssues(validation.Validate(valueElement(value), c).Issues)
			if !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%v %s) issues =\n  %+v\nwant\n  %+v", tc.magnitude, tc.units, got, tc.want)
			}
		})
	}
}

// REQ-107, REQ-103 — the generator values a DV_QUANTITY whose C_DV_QUANTITY
// names a property and lists no units in a unit of that property, under both
// value fills, so its output validates: the example takes the primary unit,
// and the random fill falls back to the example for a constraint with no unit
// list.
func TestGenerate_REQ107_REQ103_PropertyOnlyQuantityValidates(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ code, primary string }{
		{"122", "m"},
		{"125", "N/m2"},
	} {
		c := mustCompileInline(t, elementValueOPT(propertyQuantityChild(tc.code)))
		for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
			t.Run(fmt.Sprintf("openehr::%s/%s", tc.code, fill), func(t *testing.T) {
				t.Parallel()
				out, err := instance.Generate(t.Context(), c, instance.Options{
					Policy:      instance.Large,
					ValueFill:   fill,
					ValueSource: rand.NewPCG(7, 1),
				})
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				el, ok := out.(*rm.Element)
				if !ok {
					t.Fatalf("Generate root = %T, want *rm.Element", out)
				}
				q, ok := el.Value.(*rm.DVQuantity)
				if !ok {
					t.Fatalf("Generate built ELEMENT.value %T, want *rm.DVQuantity", el.Value)
				}
				if q.Units != tc.primary {
					t.Errorf("Generate built units %q, want the primary unit %q of openehr::%s", q.Units, tc.primary, tc.code)
				}
				if got := issueKeys(validation.Validate(el, c).Issues); len(got) != 0 {
					t.Errorf("Validate(generated) issues = %q, want none", got)
				}
			})
		}
	}
}
