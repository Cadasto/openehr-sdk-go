package instance_test

// interval_expanded_bounds_test.go: the REQ-107 order of an interval's
// bounds where the OPT builds a DV_QUANTITY or a DV_ORDINAL bound as a
// C_COMPLEX_OBJECT with a constraint on each attribute, not as the one
// C_DV_QUANTITY or C_DV_ORDINAL that constrains the whole bound.

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// intList and realList are a C_INTEGER and a C_REAL admitting exactly
// values.
func intList(values ...int) string {
	return optPrimitive("INTEGER", "C_INTEGER", instance.IntList(values...))
}

func realList(values ...float64) string {
	return optPrimitive("REAL", "C_REAL", instance.RealList(values...))
}

// intUpTo is a C_INTEGER admitting every value up to hi.
func intUpTo(hi float64) string {
	return optPrimitive("INTEGER", "C_INTEGER", instance.IntRange(instance.NoEnd, instance.Closed(hi)))
}

// realUpTo is a C_REAL admitting every value up to hi.
func realUpTo(hi float64) string {
	return optPrimitive("REAL", "C_REAL", instance.RealRange(instance.NoEnd, instance.Closed(hi)))
}

// expandedQuantity is a DV_QUANTITY in mm whose magnitude the OPT
// constrains with magnitude, a C_REAL, on a node of its own.
func expandedQuantity(magnitude string) string {
	return optNode("DV_QUANTITY", "", optSingle("magnitude", magnitude), optSingle("units", guardString("mm")))
}

// expandedOrdinal is a DV_ORDINAL whose value the OPT constrains with
// value, a C_INTEGER, and whose symbol it constrains to the local codes,
// each on a node of its own. Unlike a C_DV_ORDINAL it lists no pair of
// value and symbol.
func expandedOrdinal(value string, codes ...string) string {
	return optNode("DV_ORDINAL", "", optSingle("value", value), optSingle("symbol", optCodedText("local", codes...)))
}

// expandedPair is one DV_INTERVAL whose two bounds are the OPT nodes lower
// and upper.
type expandedPair struct {
	name         string
	lower, upper string
	// want is the interval as intervalString renders it under ExampleFill,
	// and wantRandom under RandomFill where that differs.
	want, wantRandom string
	// ordered is set where an ordered pair fits both sides, so every
	// RandomFill draw keeps the bounds in order. It is not set where the
	// two sides admit none, which leaves the pair as generated.
	ordered bool
	// unordered is set where the two sides admit no ordered pair, so the
	// bounds stay as generated and break DV_INTERVAL.Limits_consistent
	// (REQ-107 § Exceptions and known gaps), which is the one error either
	// validator reports.
	unordered bool
}

// runExpandedIntervals generates a DV_INTERVAL<boundRM> from each case,
// under every policy, both value fills, and with and without the implicit
// attributes, checks the whole interval against the case and the root
// against both validators, and, for an ordered case, sweeps RandomFill
// seeds for the order. T is the Go type of a bound and position gives the
// value the order reads from one.
func runExpandedIntervals[T rm.DVOrdered](t *testing.T, boundRM string, cases []expandedPair, position func(T) float64) {
	t.Helper()
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", ordInterval(boundRM, tc.lower, tc.upper))), implicit)
			unordered := tc.unordered
			generate := func(t *testing.T, opts instance.Options) *rm.DVInterval[T] {
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
					t.Fatalf("ELEMENT.value is %T, want *rm.DVInterval[%s]", el.Value, boundRM)
				}
				if unordered {
					checkValidBut(t, out, c, ruleBroken("DV_INTERVAL.Limits_consistent"))
				} else {
					checkValid(t, out, c)
				}
				return iv
			}
			for _, opts := range optionsFor(nil) {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					want := tc.want
					if opts.ValueFill == instance.RandomFill && tc.wantRandom != "" {
						want = tc.wantRandom
					}
					if got := intervalString(generate(t, opts)); got != want {
						t.Errorf("ELEMENT.value:\n got %s\nwant %s", got, want)
					}
				})
			}
			if !tc.ordered {
				continue
			}
			t.Run(fmt.Sprintf("%s/implicit=%t/RandomFill seeds", tc.name, implicit), func(t *testing.T) {
				for seed := range uint64(orderSeeds) {
					opts := instance.Options{Policy: instance.Large, ValueFill: instance.RandomFill, ValueSource: rand.NewPCG(seed, 1), Now: defaultsNow}
					iv := generate(t, opts)
					if lo, hi := position(iv.Lower), position(iv.Upper); lo > hi {
						t.Errorf("seed %d: %s, lower above upper", seed, intervalString(iv))
					}
				}
			})
		}
	}
}

// TestREQ107_IntervalOfExpandedQuantitiesStaysInEachSidesConstraint is the
// REQ-107 Intervals and Output check that a DV_INTERVAL<DV_QUANTITY> whose
// bounds the OPT builds as a C_COMPLEX_OBJECT, with a C_REAL on each
// magnitude, is put in order only by a pair each side's own constraint
// accepts: the swapped pair where it fits, else the lowest magnitude of the
// lower side with the highest of the upper side, a side with no end taking
// the other's bound. Where no such pair is in order the bounds stay as
// generated, which no validator reports. It holds under every policy, both
// value fills, and with and without the implicit attributes, and the root
// passes both validators.
func TestREQ107_IntervalOfExpandedQuantitiesStaysInEachSidesConstraint(t *testing.T) {
	runExpandedIntervals(t, "DV_QUANTITY", []expandedPair{
		{
			// Examples 9 and 1: the swapped pair fits.
			name:  "lists [9, 1] and [1, 9], swapped",
			lower: expandedQuantity(realList(9, 1)), upper: expandedQuantity(realList(1, 9)),
			want:    "[DV_QUANTITY 1 mm .. DV_QUANTITY 9 mm]",
			ordered: true,
		},
		{
			// Examples 5 and 0; 0 is outside the lower range, so no swap.
			name:  "ranges 5..10 and 0..20, lowest with highest",
			lower: expandedQuantity(realFrom(5, 10)), upper: expandedQuantity(realFrom(0, 20)),
			want:       "[DV_QUANTITY 5 mm .. DV_QUANTITY 20 mm]",
			wantRandom: "[DV_QUANTITY 8.382278298339125 mm .. DV_QUANTITY 10.170947953520528 mm]",
			ordered:    true,
		},
		{
			// The lower side has no lowest magnitude, so the upper side's
			// 0 stands in; the upper side's highest is 10.
			name:  "a lower range with no end, the upper bound stands in",
			lower: expandedQuantity(realUpTo(20)), upper: expandedQuantity(realFrom(0, 10)),
			want:       "[DV_QUANTITY 0 mm .. DV_QUANTITY 10 mm]",
			wantRandom: "[DV_QUANTITY -12.354434033217501 mm .. DV_QUANTITY 5.085473976760264 mm]",
			ordered:    true,
		},
		{
			// Every lower magnitude is above every upper one.
			name:  "lists [9] and [1], no pair in order",
			lower: expandedQuantity(realList(9)), upper: expandedQuantity(realList(1)),
			want:      "[DV_QUANTITY 9 mm .. DV_QUANTITY 1 mm]",
			unordered: true,
		},
	}, func(q rm.DVQuantity) float64 { return float64(q.Magnitude) })
}

// TestREQ107_IntervalOfExpandedOrdinalsStaysInEachSidesConstraint is the
// REQ-107 Intervals and Output check that a DV_INTERVAL<DV_ORDINAL> whose
// bounds the OPT builds as a C_COMPLEX_OBJECT, with a C_INTEGER on each
// value and a C_CODE_PHRASE on each symbol, is put in order only by a pair
// each side's own constraint accepts: a swap never hands a side a value or
// a symbol code its constraint rejects. The lowest value of the lower side
// with the highest of the upper side keep each side's own symbol. Where no
// such pair is in order the bounds stay as generated. It holds under every
// policy, both value fills, and with and without the implicit attributes,
// and the root passes both validators.
func TestREQ107_IntervalOfExpandedOrdinalsStaysInEachSidesConstraint(t *testing.T) {
	runExpandedIntervals(t, "DV_ORDINAL", []expandedPair{
		{
			// Examples 9 and 1, both with at0001: the swapped pair fits.
			name:  "value lists [9, 1] and [1, 9], one symbol, swapped",
			lower: expandedOrdinal(intList(9, 1), "at0001"), upper: expandedOrdinal(intList(1, 9), "at0001"),
			want:    "[DV_ORDINAL 1 local::at0001 .. DV_ORDINAL 9 local::at0001]",
			ordered: true,
		},
		{
			// A swap would give the lower side 0 and at0003, neither of
			// which it admits.
			name:  "ranges 5..10 and 0..20, own symbols, lowest with highest",
			lower: expandedOrdinal(integerFrom(5, 10), "at0001"), upper: expandedOrdinal(integerFrom(0, 20), "at0003"),
			want:       "[DV_ORDINAL 5 local::at0001 .. DV_ORDINAL 20 local::at0003]",
			wantRandom: "[DV_ORDINAL 9 local::at0001 .. DV_ORDINAL 16 local::at0003]",
			ordered:    true,
		},
		{
			// Examples 9 and 1: the values fit when swapped, but the symbols
			// do not, since at0002 is not a symbol the lower side admits.
			name:  "values swap, symbols cannot, lowest with highest",
			lower: expandedOrdinal(intList(9, 1), "at0001"), upper: expandedOrdinal(intList(1, 9), "at0002"),
			want:    "[DV_ORDINAL 1 local::at0001 .. DV_ORDINAL 9 local::at0002]",
			ordered: true,
		},
		{
			// The lower side has no lowest value, so the upper side's 0
			// stands in, with the lower side's own symbol.
			name:  "a lower range with no end, the upper value stands in with the lower symbol",
			lower: expandedOrdinal(intUpTo(20), "at0001"), upper: expandedOrdinal(integerFrom(0, 10), "at0002"),
			want:       "[DV_ORDINAL 0 local::at0001 .. DV_ORDINAL 10 local::at0002]",
			wantRandom: "[DV_ORDINAL -3 local::at0001 .. DV_ORDINAL 8 local::at0002]",
			ordered:    true,
		},
		{
			// Every lower value is above every upper one.
			name:  "value lists [9] and [1], no pair in order",
			lower: expandedOrdinal(intList(9), "at0001"), upper: expandedOrdinal(intList(1), "at0002"),
			want:      "[DV_ORDINAL 9 local::at0001 .. DV_ORDINAL 1 local::at0002]",
			unordered: true,
		},
	}, func(o rm.DVOrdinal) float64 { return float64(o.Value) })
}
