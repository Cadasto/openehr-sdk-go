package validation_test

// primitive_no_item_test.go: REQ-102 § Structural — alternatives and REQ-111
// § What the compile step tolerates. In its default mode the compile step
// reads a C_PRIMITIVE_OBJECT with no item as no constraint, which the AOM 1.4
// BMM's any_allowed function (item = Void) reads as any value of the type
// allowed. The validator reads it the same way: the AOM 1.4 short name the
// node carries still names the type of the value, and nothing more is
// checked. REQ-107 — the example generator's value under such a node
// validates there, under both fills.

import (
	"fmt"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// noItemChild returns a C_PRIMITIVE_OBJECT child that names the AOM 1.4
// short name shortName and carries no item.
func noItemChild(shortName string) string {
	return `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>` + shortName + `</rm_type_name><node_id></node_id></children>`
}

// noItemRows are data values, each of which holds the attribute attr under
// a C_PRIMITIVE_OBJECT that names the short name of its type and carries
// no item, and a value of that host.
func noItemRows() []struct {
	name  string
	child string
	value rm.DataValue
} {
	under := func(host, attr, shortName string) string {
		return complexChild(host, singleAttribute(attr, "1", "1", noItemChild(shortName)))
	}
	return []struct {
		name  string
		child string
		value rm.DataValue
	}{
		{"DV_DURATION value, DURATION", under("DV_DURATION", "value", "DURATION"), rm.DVDuration{Value: "PT5M"}},
		{"DV_DATE value, DATE", under("DV_DATE", "value", "DATE"), &rm.DVDate{Value: "2026-11-18"}},
		{"DV_TIME value, TIME", under("DV_TIME", "value", "TIME"), &rm.DVTime{Value: "10:30:00"}},
		{"DV_DATE_TIME value, DATE_TIME", under("DV_DATE_TIME", "value", "DATE_TIME"), &rm.DVDateTime{Value: "2026-11-18T10:30:00Z"}},
		{"DV_COUNT magnitude, INTEGER", under("DV_COUNT", "magnitude", "INTEGER"), rm.DVCount{Magnitude: 15}},
		{"DV_QUANTITY magnitude, REAL", under("DV_QUANTITY", "magnitude", "REAL"), &rm.DVQuantity{Magnitude: 7.5, Units: "cm"}},
		{"DV_BOOLEAN value, BOOLEAN", under("DV_BOOLEAN", "value", "BOOLEAN"), rm.DVBoolean{Value: true}},
		{"DV_TEXT value, STRING", under("DV_TEXT", "value", "STRING"), rm.DVText{Value: "free text"}},
		// The short name in place of the wrapper it stands for, as before.
		{"ELEMENT value, DURATION", noItemChild("DURATION"), rm.DVDuration{Value: "PT5M"}},
	}
}

// REQ-102, REQ-111 — a value under a C_PRIMITIVE_OBJECT with no item
// validates whatever it holds of the type the short name names: under an
// attribute of its data value, where the value is a Go primitive, and in
// place of the data value, where it is the canonical wrapper. The short
// name still names a type: a duration under INTEGER is refused.
func TestValidate_REQ102_REQ111_PrimitiveWithNoItemAdmitsAnyValue(t *testing.T) {
	t.Parallel()
	for _, tc := range noItemRows() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, elementValueOPT(tc.child))
			if got := issueKeys(validation.Validate(valueElement(tc.value), c).Issues); len(got) != 0 {
				t.Errorf("Validate(%s with no item) issues = %q, want none: no item allows any value", tc.name, got)
			}
		})
	}
	t.Run("DV_DURATION value, INTEGER", func(t *testing.T) {
		t.Parallel()
		c := mustCompileInline(t, elementValueOPT(complexChild("DV_DURATION", singleAttribute("value", "1", "1", noItemChild("INTEGER")))))
		want := []string{"rm_type_mismatch /value/value"}
		if got := issueKeys(validation.Validate(valueElement(rm.DVDuration{Value: "PT5M"}), c).Issues); !slices.Equal(got, want) {
			t.Errorf("Validate(a duration under INTEGER with no item) issues = %q, want %q: the short name still names the type", got, want)
		}
	})
}

// REQ-107, REQ-102 — the example generator writes a value under each
// C_PRIMITIVE_OBJECT with no item, under both fills, that the validator
// admits there: the two read the node alike.
func TestGenerate_REQ107_REQ102_PrimitiveWithNoItemValidates(t *testing.T) {
	t.Parallel()
	for _, tc := range noItemRows() {
		for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
			t.Run(tc.name+", "+fmt.Sprint(fill), func(t *testing.T) {
				t.Parallel()
				c := mustCompileInline(t, elementValueOPT(tc.child))
				out, err := instance.Generate(t.Context(), c, instance.Options{Policy: instance.Large, ValueFill: fill})
				if err != nil {
					t.Fatalf("Generate(%s with no item, fill %v): %v", tc.name, fill, err)
				}
				if got := issueKeys(validation.Validate(out, c).Issues); len(got) != 0 {
					t.Errorf("Validate(generated %s with no item, fill %v) issues = %q, want none", tc.name, fill, got)
				}
			})
		}
	}
}

// REQ-102, REQ-171 — a Go primitive under a short name with no item is a
// leaf, as one under a short name with an item is: nothing is below it, so
// the depth limit does not stop the walk there. At a depth limit that falls
// on the leaf, the walk reports what it reports under a C_DURATION, its stop
// elsewhere.
func TestValidator_REQ102_REQ171_PrimitiveWithNoItemAtTheDepthLimit(t *testing.T) {
	t.Parallel()
	v := validation.New(validation.WithLimits(validation.Limits{Depth: 2}))
	value := valueElement(rm.DVDuration{Value: "PT5M"})
	withItem := mustCompileInline(t, elementValueOPT(complexChild("DV_DURATION", singleAttribute("value", "1", "1", durationChild))))
	noItem := mustCompileInline(t, elementValueOPT(complexChild("DV_DURATION", singleAttribute("value", "1", "1", noItemChild("DURATION")))))
	want := issueKeys(v.Validate(value, withItem).Issues)
	if slices.Contains(want, "max_depth /value/value") {
		t.Fatalf("precondition: Validate(a duration under C_DURATION, depth 2) issues = %q, want no stop at the leaf", want)
	}
	if got := issueKeys(v.Validate(value, noItem).Issues); !slices.Equal(got, want) {
		t.Errorf("Validate(a duration under DURATION with no item, depth 2) issues = %q, want %q, as under C_DURATION", got, want)
	}
}
