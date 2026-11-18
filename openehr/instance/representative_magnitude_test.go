package instance_test

// representative_magnitude_test.go: the REQ-107 Primitive-leaf value fill
// rule on the magnitude of a DV_QUANTITY and of a DV_COUNT. ExampleFill
// gives a bounded range its middle, rounded to the precision, and any other
// range, or none, 10 moved into it; RandomFill never draws a count as 0
// where its range is open.

import (
	"fmt"
	mrand "math/rand/v2"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// generateExample generates c under every policy with ExampleFill and calls
// check with each root.
func generateExample(t *testing.T, opt string, check func(t *testing.T, what string, out any)) {
	t.Helper()
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		for _, policy := range everyPolicy {
			what := fmt.Sprintf("implicit=%t/%v", implicit, policy)
			out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, ValueFill: instance.ExampleFill, Now: defaultsNow})
			if err != nil {
				t.Fatalf("%s: Generate: %v", what, err)
			}
			check(t, what, out)
			checkValid(t, out, c)
		}
	}
}

// onlyQuantity returns the one DV_QUANTITY out holds.
func onlyQuantity(t *testing.T, what string, out any) rm.DVQuantity {
	t.Helper()
	var got []rm.DVQuantity
	visitRM(out, func(_ string, q rm.DVQuantity) { got = append(got, q) })
	if len(got) != 1 {
		t.Fatalf("%s: %d DV_QUANTITY generated, want 1", what, len(got))
	}
	return got[0]
}

// onlyCount returns the one DV_COUNT out holds.
func onlyCount(t *testing.T, what string, out any) rm.DVCount {
	t.Helper()
	var got []rm.DVCount
	visitRM(out, func(_ string, n rm.DVCount) { got = append(got, n) })
	if len(got) != 1 {
		t.Fatalf("%s: %d DV_COUNT generated, want 1", what, len(got))
	}
	return got[0]
}

// optElementValue is an ELEMENT template whose value is node.
func optElementValue(node string) string {
	return optTemplate("ELEMENT", optSingle("value", node))
}

// countOver is a DV_COUNT whose magnitude carries a C_INTEGER with body.
func countOver(body string) string {
	return optNode("DV_COUNT", "", optSingle("magnitude", optPrimitive("INTEGER", "C_INTEGER", body)))
}

// TestREQ107_ExampleQuantityMagnitudeIsRepresentative is the REQ-107
// Primitive-leaf value fill check on the magnitude ExampleFill gives a
// DV_QUANTITY under a C_DV_QUANTITY: the middle of a range with both bounds,
// rounded to the entry's precision and moved inside an excluded bound, and
// otherwise 10 moved into the range, the range's nearest bound where it
// does not admit 10, moved inside an excluded one by one step of the
// precision. Where that gives no value the constraint accepts, the
// constraint's own example stays.
func TestREQ107_ExampleQuantityMagnitudeIsRepresentative(t *testing.T) {
	closed, open, noEnd := instance.Closed, instance.Open, instance.NoEnd
	zero := rangeOf(closed(0), closed(0))
	cases := []struct {
		name  string
		entry string // "" is a C_DV_QUANTITY that lists no entry
		units string
		want  float64
	}{
		{name: "0..1000", entry: quantityEntry("kg", rangeOf(closed(0), closed(1000)), ""), units: "kg", want: 500},
		{name: "0..20", entry: quantityEntry("kg", rangeOf(closed(0), closed(20)), ""), units: "kg", want: 10},
		{name: "0..37 precision 0", entry: quantityEntry("kg", rangeOf(closed(0), closed(37)), zero), units: "kg", want: 19},
		{name: "0..0", entry: quantityEntry("kg", rangeOf(closed(0), closed(0)), ""), units: "kg", want: 0},
		{name: ">=0", entry: quantityEntry("cm", rangeOf(closed(0), noEnd), ""), units: "cm", want: 10},
		{name: ">=30", entry: quantityEntry("cm", rangeOf(closed(30), noEnd), ""), units: "cm", want: 30},
		{name: ">30 precision 0", entry: quantityEntry("cm", rangeOf(open(30), noEnd), zero), units: "cm", want: 31},
		{name: ">30 precision 0..1", entry: quantityEntry("cm", rangeOf(open(30), noEnd), rangeOf(closed(0), closed(1))), units: "cm", want: 30.1},
		{name: "<=5", entry: quantityEntry("cm", rangeOf(noEnd, closed(5)), ""), units: "cm", want: 5},
		{name: "<5 precision 0", entry: quantityEntry("cm", rangeOf(noEnd, open(5)), zero), units: "cm", want: 4},
		{name: "no magnitude range", entry: quantityEntry("cm", "", ""), units: "cm", want: 10},
		{name: "no entry", entry: "", units: "1", want: 10},
		{
			// The middle, 0.5, rounds to 1 and steps back to 0, both
			// excluded: the constraint's own example, the middle, stays.
			name: "(0..1) precision 0", entry: quantityEntry("cm", rangeOf(open(0), open(1)), zero), units: "cm", want: 0.5,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var entries []string
			if tc.entry != "" {
				entries = append(entries, tc.entry)
			}
			generateExample(t, optQuantityValue(entries...), func(t *testing.T, what string, out any) {
				q := onlyQuantity(t, what, out)
				if float64(q.Magnitude) != tc.want || q.Units != tc.units {
					t.Errorf("%s: DV_QUANTITY %v %q, want %v %q", what, q.Magnitude, q.Units, tc.want, tc.units)
				}
			})
		})
	}
}

// TestREQ107_ExampleAttributeQuantityMagnitudeIsRepresentative is the
// REQ-107 Primitive-leaf value fill check on a DV_QUANTITY the template
// builds attribute by attribute, a C_REAL on its magnitude, or no
// constraint at all: ExampleFill gives its magnitude as it gives one under
// a C_DV_QUANTITY, rounded to the precision the walk writes, wherever the
// template puts the precision.
func TestREQ107_ExampleAttributeQuantityMagnitudeIsRepresentative(t *testing.T) {
	r, closed, open, noEnd := instance.RealRange, instance.Closed, instance.Open, instance.NoEnd
	units := optSingle("units", optPrimitive("STRING", "C_STRING", "<list>mm</list>"))
	quantity := func(magnitude string, precision ...string) string {
		attrs := append([]string{optSingle("magnitude", optPrimitive("REAL", "C_REAL", magnitude)), units}, precision...)
		return optElementValue(optNode("DV_QUANTITY", "", attrs...))
	}
	precision0 := optProportionPart("precision", instance.IntList(0))
	cases := []struct {
		name string
		opt  string
		want float64
	}{
		{name: "0..100", opt: quantity(r(closed(0), closed(100))), want: 50},
		{name: ">=0", opt: quantity(r(closed(0), noEnd)), want: 10},
		{name: ">30 precision 0", opt: quantity(r(open(30), noEnd), precision0), want: 31},
		{name: ">30 no precision", opt: quantity(r(open(30), noEnd)), want: 30.01},
		{name: "no magnitude constraint", opt: optElementValue(optNode("DV_QUANTITY", "", units)), want: 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			generateExample(t, tc.opt, func(t *testing.T, what string, out any) {
				if q := onlyQuantity(t, what, out); float64(q.Magnitude) != tc.want {
					t.Errorf("%s: DV_QUANTITY magnitude %v, want %v", what, q.Magnitude, tc.want)
				}
			})
		})
	}
}

// TestREQ107_ExampleCountMagnitudeIsRepresentative is the REQ-107
// Primitive-leaf value fill check on the magnitude ExampleFill gives a
// DV_COUNT: as a quantity's with a precision of 0, under a C_INTEGER on its
// magnitude or with no constraint at all. Where that gives no value the
// C_INTEGER accepts, as a list that holds neither the middle nor 10, its own
// example stays.
func TestREQ107_ExampleCountMagnitudeIsRepresentative(t *testing.T) {
	ir, closed, open, noEnd := instance.IntRange, instance.Closed, instance.Open, instance.NoEnd
	cases := []struct {
		name string
		opt  string
		want int64
	}{
		{name: "0..1000", opt: countOver(ir(closed(0), closed(1000))), want: 500},
		{name: "0..20", opt: countOver(ir(closed(0), closed(20))), want: 10},
		{name: "0..7", opt: countOver(ir(closed(0), closed(7))), want: 4},
		{name: "0..0", opt: countOver(ir(closed(0), closed(0))), want: 0},
		{name: "(0..2)", opt: countOver(ir(open(0), open(2))), want: 1},
		{name: ">=0", opt: countOver(ir(closed(0), noEnd)), want: 10},
		{name: ">=30", opt: countOver(ir(closed(30), noEnd)), want: 30},
		{name: ">30", opt: countOver(ir(open(30), noEnd)), want: 31},
		{name: "<=5", opt: countOver(ir(noEnd, closed(5))), want: 5},
		{name: "<5", opt: countOver(ir(noEnd, open(5))), want: 4},
		{name: "list holding 10", opt: countOver(instance.IntList(3, 10, 20)), want: 10},
		{name: "list without 10", opt: countOver(instance.IntList(3, 7)), want: 3},
		{name: "no magnitude constraint", opt: optNode("DV_COUNT", ""), want: 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			generateExample(t, optElementValue(tc.opt), func(t *testing.T, what string, out any) {
				if n := onlyCount(t, what, out); int64(n.Magnitude) != tc.want {
					t.Errorf("%s: DV_COUNT magnitude %d, want %d", what, n.Magnitude, tc.want)
				}
			})
		})
	}
}

// TestREQ107_RandomCountIsNotZeroWhereItsRangeIsOpen is the REQ-107
// Primitive-leaf value fill check on the magnitude RandomFill draws for a
// DV_COUNT with no constraint, or a C_INTEGER whose range is open or
// bounded on one side: it is drawn as an open quantity range is, inside the
// range away from its bound, or from 1 to 100 where it is open, and is
// never 0. The draws vary between seeds.
func TestREQ107_RandomCountIsNotZeroWhereItsRangeIsOpen(t *testing.T) {
	ir, closed, noEnd := instance.IntRange, instance.Closed, instance.NoEnd
	cases := []struct {
		name   string
		opt    string
		lo, hi int64 // the magnitude lies from lo to hi
	}{
		{name: "no magnitude constraint", opt: optNode("DV_COUNT", ""), lo: 1, hi: 100},
		{name: "open C_INTEGER", opt: countOver(""), lo: 1, hi: 100},
		{name: ">=0", opt: countOver(ir(closed(0), noEnd)), lo: 1, hi: 100},
		{name: "<=0", opt: countOver(ir(noEnd, closed(0))), lo: -100, hi: -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileOPTText(t, optElementValue(tc.opt), true)
			seen := map[int64]bool{}
			eachRandomRun(t, func(opts instance.Options) (any, error) {
				return instance.Generate(t.Context(), c, opts)
			}, func(t *testing.T, what string, out any) {
				m := int64(onlyCount(t, what, out).Magnitude)
				if m == 0 || m < tc.lo || m > tc.hi {
					t.Errorf("%s: DV_COUNT magnitude %d, want one from %d to %d and not 0", what, m, tc.lo, tc.hi)
				}
				seen[m] = true
				checkValid(t, out, c)
			})
			if len(seen) < 2 {
				t.Errorf("DV_COUNT magnitudes %v over %d seeds, want draws that vary", seen, roundingSeeds)
			}
		})
	}
}

// TestREQ107_IntervalNewUnitTakesExampleMagnitude is the REQ-107 Intervals
// check that a side whose C_DV_QUANTITY lists units takes, in a new unit,
// the magnitude ExampleFill gives for the first entry in that unit: the
// middle of the g entry's range, not its lower bound.
func TestREQ107_IntervalNewUnitTakesExampleMagnitude(t *testing.T) {
	closed := instance.Closed
	lower := listedQuantity(
		instance.QuantityEntry("mg", closed(0), closed(1000)),
		instance.QuantityEntry("g", closed(2), closed(10)))
	upper := listedQuantity(
		instance.QuantityEntry("g", closed(0), closed(20)),
		instance.QuantityEntry("kg", closed(0), closed(1)))
	c := compileOPTText(t, optElementValue(ordInterval("DV_QUANTITY", lower, upper)), true)
	for _, policy := range everyPolicy {
		opts := instance.Options{Policy: policy, ValueFill: instance.ExampleFill, ValueSource: mrand.NewPCG(1, 2), Now: defaultsNow}
		iv, out := generateInterval[rm.DVQuantity](t, c, opts)
		if iv.LowerUnbounded || iv.UpperUnbounded {
			t.Fatalf("%v: interval %s, want both bounds", policy, intervalString(iv))
		}
		if iv.Lower.Units != "g" || iv.Lower.Magnitude != 6 || iv.Upper.Units != "g" || iv.Upper.Magnitude != 10 {
			t.Errorf("%v: interval %s, want 6 g..10 g", policy, intervalString(iv))
		}
		checkValid(t, out, c)
	}
}
