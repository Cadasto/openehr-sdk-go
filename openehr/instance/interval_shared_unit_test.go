package instance_test

// interval_shared_unit_test.go: the REQ-107 Intervals rule that the two
// bounds of a DV_INTERVAL<DV_QUANTITY> take one unit, and the two bounds of a
// DV_INTERVAL<DV_PROPORTION> one type, that both sides admit, before they
// are put in order, so the RM floor reads them as strictly comparable
// (REQ-112 DV_INTERVAL). Where the two sides admit no one unit or type, the
// pair is left as generated and the floor reports it.

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// listedQuantity is a DV_QUANTITY bound constrained by one C_DV_QUANTITY
// that lists entries, each a QuantityEntry.
func listedQuantity(entries ...string) string {
	return `<children xsi:type="C_DV_QUANTITY"><rm_type_name>DV_QUANTITY</rm_type_name><node_id></node_id>` +
		strings.Join(entries, "") + `</children>`
}

// perAttributeQuantity is a DV_QUANTITY bound the OPT builds attribute by
// attribute: a C_REAL from 0 to 10 on its magnitude and a C_STRING listing
// units on its units. With no units, the OPT leaves the units unconstrained.
func perAttributeQuantity(units ...string) string {
	attrs := []string{optSingle("magnitude", realFrom(0, 10))}
	if len(units) > 0 {
		attrs = append(attrs, optSingle("units", optPrimitive("STRING", "C_STRING", "<list>"+strings.Join(units, "</list><list>")+"</list>")))
	}
	return optNode("DV_QUANTITY", "", attrs...)
}

// unitEntry is a C_DV_QUANTITY entry admitting units from 0 to hi.
func unitEntry(units string, hi float64) string {
	return instance.QuantityEntry(units, instance.Closed(0), instance.Closed(hi))
}

// sharedPairCase is one DV_INTERVAL whose two bounds are the OPT nodes lower
// and upper. want is the one unit, or type, both bounds hold once generated;
// empty, the sides admit none, so the bounds keep lowerWant and upperWant,
// as generated, and the floor reports DV_INTERVAL.Limits_consistent.
type sharedPairCase struct {
	name                 string
	lower, upper         string
	want                 string
	lowerWant, upperWant string
}

// runSharedPairs generates a DV_INTERVAL<boundRM> from each case, under
// every policy and both value fills, with and without the implicit
// attributes, and over RandomFill seeds, and checks what each bound holds
// (read) and that the root passes both validators, or, where the case
// expects the pair left as generated, that they report it as breaking
// DV_INTERVAL.Limits_consistent and nothing else.
func runSharedPairs[T rm.DVOrdered](t *testing.T, boundRM string, cases []sharedPairCase, read func(T) string) {
	t.Helper()
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", ordInterval(boundRM, tc.lower, tc.upper))), implicit)
			check := func(t *testing.T, opts instance.Options, what string) {
				t.Helper()
				iv, out := generateInterval[T](t, c, opts)
				lo, hi := read(iv.Lower), read(iv.Upper)
				wantLo, wantHi := tc.want, tc.want
				if tc.want == "" {
					wantLo, wantHi = tc.lowerWant, tc.upperWant
				}
				if lo != wantLo || hi != wantHi {
					t.Errorf("%s: bounds hold %q and %q, want %q and %q (%s)", what, lo, hi, wantLo, wantHi, intervalString(iv))
				}
				if tc.want == "" {
					checkValidBut(t, out, c, ruleBroken("DV_INTERVAL.Limits_consistent"))
				} else {
					checkValid(t, out, c)
				}
			}
			for _, opts := range optionsFor(nil) {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					check(t, opts, "generated")
				})
			}
			t.Run(fmt.Sprintf("%s/implicit=%t/RandomFill seeds", tc.name, implicit), func(t *testing.T) {
				for seed := range uint64(orderSeeds) {
					opts := instance.Options{Policy: instance.Large, ValueFill: instance.RandomFill, ValueSource: rand.NewPCG(seed, 1), Now: defaultsNow}
					check(t, opts, fmt.Sprintf("seed %d", seed))
				}
			})
		}
	}
}

// generateInterval generates c, whose root is an ELEMENT, and returns the
// DV_INTERVAL<T> its value holds, and the root.
func generateInterval[T rm.DVOrdered](t *testing.T, c *templatecompile.Compiled, opts instance.Options) (*rm.DVInterval[T], any) {
	t.Helper()
	out, err := instance.Generate(t.Context(), c, opts)
	if err != nil {
		t.Fatalf("Generate: %v, want a root", err)
	}
	el, ok := out.(*rm.Element)
	if !ok {
		t.Fatalf("generated root is %T, want *rm.Element", out)
	}
	iv, ok := el.Value.(*rm.DVInterval[T])
	if !ok {
		var zero T
		t.Fatalf("ELEMENT.value is %T, want *rm.DVInterval[%T]", el.Value, zero)
	}
	return iv, out
}

// TestREQ107_IntervalQuantityBoundsShareOneUnit is the REQ-107 Intervals
// check that the two bounds of a DV_INTERVAL<DV_QUANTITY> take one unit both
// sides admit before they are ordered, whether the OPT constrains a bound by
// one C_DV_QUANTITY or attribute by attribute, under both value fills: of the
// lower bound's unit, the upper bound's and those the sides list, the first
// both admit in which an ordered pair fits both sides. A side the OPT leaves
// without units takes the other side's. Where the sides admit no one unit,
// the bounds keep their own and the floor reports them as not strictly
// comparable.
func TestREQ107_IntervalQuantityBoundsShareOneUnit(t *testing.T) {
	runSharedPairs(t, "DV_QUANTITY", []sharedPairCase{
		{
			// ExampleFill writes mg below and g above; only g fits both.
			name:  "C_DV_QUANTITY lists [mg, g] and [g, kg]",
			lower: listedQuantity(unitEntry("mg", 1000), unitEntry("g", 10)),
			upper: listedQuantity(unitEntry("g", 10), unitEntry("kg", 1)),
			want:  "g",
		},
		{
			name:  "C_STRING units [mg, g] and [g, kg]",
			lower: perAttributeQuantity("mg", "g"),
			upper: perAttributeQuantity("g", "kg"),
			want:  "g",
		},
		{
			// RandomFill can draw mg below and kg above, which neither side
			// admits of the other; g, the first unit of the lower side's list
			// the upper side admits, fits both.
			name:  "C_STRING units [mg, g] and [kg, g]",
			lower: perAttributeQuantity("mg", "g"),
			upper: perAttributeQuantity("kg", "g"),
			want:  "g",
		},
		{
			name:  "a C_DV_QUANTITY listing [mg, g] below and C_STRING units [g, kg] above",
			lower: listedQuantity(unitEntry("mg", 1000), unitEntry("g", 10)),
			upper: perAttributeQuantity("g", "kg"),
			want:  "g",
		},
		{
			// Both sides admit g and kg, but no ordered pair fits both in g,
			// the lower side starting at 5 g and the upper one ending at 1 g.
			// ExampleFill writes 5 g below and 0 kg above, and RandomFill can
			// draw both bounds in g; in kg an ordered pair fits.
			name:  "C_DV_QUANTITY lists [g from 5, kg] and [kg, g to 1]",
			lower: listedQuantity(instance.QuantityEntry("g", instance.Closed(5), instance.Closed(10)), unitEntry("kg", 1)),
			upper: listedQuantity(unitEntry("kg", 1), unitEntry("g", 1)),
			want:  "kg",
		},
		{
			name:  "a C_DV_QUANTITY listing [mg] below and no units above",
			lower: listedQuantity(unitEntry("mg", 10)),
			upper: perAttributeQuantity(),
			want:  "mg",
		},
		{
			name:      "C_DV_QUANTITY lists [mg] and [kg], no unit fits both",
			lower:     listedQuantity(unitEntry("mg", 10)),
			upper:     listedQuantity(unitEntry("kg", 10)),
			lowerWant: "mg", upperWant: "kg",
		},
	}, func(q rm.DVQuantity) string { return q.Units })
}

// TestREQ107_IntervalProportionBoundsShareOneType is the REQ-107 Intervals
// check that the two bounds of a DV_INTERVAL<DV_PROPORTION> take one type
// both sides admit, settled as that kind inside each side's constraints,
// before they are ordered: of the lower bound's type, the upper bound's and
// every kind in code order, the first both admit in which an ordered pair
// fits both sides. Where the sides admit no one type, the bounds keep their
// own and the floor reports them as not strictly comparable.
func TestREQ107_IntervalProportionBoundsShareOneType(t *testing.T) {
	numerators := instance.RealRange(instance.Closed(0), instance.Closed(100))
	one := instance.RealList(1)
	runSharedPairs(t, "DV_PROPORTION", []sharedPairCase{
		{
			// ExampleFill writes a ratio below and a percent above, and
			// RandomFill can draw a ratio below and a fraction above, which
			// neither side admits of the other.
			name:  "types [ratio, percent] and [percent, fraction]",
			lower: instance.ProportionKindBound(numerators, instance.IntList(0, 2), ""),
			upper: instance.ProportionKindBound(numerators, instance.IntList(2, 3), ""),
			want:  "2",
		},
		{
			// Both sides admit fractions and ratios over 1, but as fractions,
			// which need whole numerators, the lower side starts at 6 and the
			// upper one ends at 5, so no ordered pair fits both. ExampleFill
			// writes a fraction below and a ratio above, and RandomFill can
			// draw two fractions; as ratios an ordered pair fits.
			name:  "types [fraction, ratio] and [ratio, fraction], an ordered pair only as ratios",
			lower: instance.ProportionKindBoundOver(instance.RealRange(instance.Closed(5.2), instance.Closed(6)), one, instance.IntList(3, 0), ""),
			upper: instance.ProportionKindBoundOver(instance.RealRange(instance.Closed(5), instance.Closed(5.5)), one, instance.IntList(0, 3), ""),
			want:  "0",
		},
		{
			name:      "types [ratio] and [fraction], no type fits both",
			lower:     instance.ProportionKindBound(numerators, instance.IntList(0), ""),
			upper:     instance.ProportionKindBound(numerators, instance.IntList(3), ""),
			lowerWant: "0", upperWant: "3",
		},
	}, func(p rm.DVProportion) string { return fmt.Sprint(p.Type) })
}
