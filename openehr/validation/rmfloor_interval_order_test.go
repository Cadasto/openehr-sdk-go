package validation_test

// rmfloor_interval_order_test.go: REQ-112 DV_INTERVAL. The floor orders the
// two bounds of an interval when neither side is unbounded and the two are
// strictly comparable as the RM defines is_strictly_comparable_to for their
// type, by the RM's less_than for that type, and it orders no other pair. Two
// DV_TIME or DV_DATE_TIME values in different time zones are ordered as
// instants, and one with a zone is not ordered against one without.
// Whether two bounds are comparable is Interval.Limits_comparable, which the
// floor does not evaluate, so Coverage.Unevaluated lists it (REQ-171
// Coverage). PROBE-077 holds a breaking and a conforming interval of each
// bound type as the registry decodes it; the tests here read every interval
// form the floor reads, and the pairs it must not order.

import (
	"math"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// limitsRule is the rule of the bound-order finding.
const limitsRule = "DV_INTERVAL.Limits_consistent"

// limitsIssues returns the issues of r whose rule is limitsRule.
func limitsIssues(r validation.Result) []validation.Issue {
	var out []validation.Issue
	for _, i := range r.Issues {
		if i.Rule == limitsRule {
			out = append(out, i)
		}
	}
	return out
}

// orderSymbol returns a symbol coded as code in the terminology id. An empty
// id gives a symbol coded in no terminology.
func orderSymbol(id, code string) rm.DVCodedText {
	return rm.DVCodedText{
		Value:        code,
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: id}, CodeString: code},
	}
}

// orderForm is an interval in one of the forms the floor reads.
type orderForm struct {
	name string
	root any
}

// orderForms returns the closed interval from lower to upper in each form
// the floor reads: DV_INTERVAL<T> by pointer and by value, as a typed
// normal_range holds it, and the bare DV_INTERVAL, which the registry decodes
// and a REFERENCE_RANGE holds, by pointer and by value, each holding its
// bounds by value and by pointer.
func orderForms[T rm.DVOrdered](lower, upper T) []orderForm {
	lowerRef, upperRef := lower, upper
	boundsByValue := rm.NewClosedDVInterval[rm.DVOrdered](lower, upper)
	boundsByPointer := rm.NewClosedDVInterval(any(&lowerRef).(rm.DVOrdered), any(&upperRef).(rm.DVOrdered))
	return []orderForm{
		{name: "typed, by pointer", root: new(rm.NewClosedDVInterval(lower, upper))},
		{name: "typed, by value", root: rm.NewClosedDVInterval(lower, upper)},
		{name: "bare by pointer, bounds by value", root: &boundsByValue},
		{name: "bare by value, bounds by value", root: boundsByValue},
		{name: "bare by pointer, bounds by pointer", root: &boundsByPointer},
		{name: "bare by value, bounds by pointer", root: boundsByPointer},
	}
}

// checkOrder validates every form of the interval from lower to upper, two
// bounds of class with lower above upper, and of the interval from upper to
// lower, which is in order. The first must draw one Limits_consistent issue,
// at the interval, whose Detail names class and no bound and whose Value
// holds want, the two compared magnitudes, lower first (REQ-168). The second
// must draw none ([checkInOrder]).
func checkOrder[T rm.DVOrdered](t *testing.T, name, class string, lower, upper T, want [2]float64) {
	t.Helper()
	detail := "DV_INTERVAL<" + class + ">: lower must be ≤ upper"
	for _, f := range orderForms(lower, upper) {
		t.Run(name+"/"+f.name+"/lower above upper", func(t *testing.T) {
			got := limitsIssues(validation.ValidateRM(f.root))
			if len(got) != 1 {
				t.Fatalf("ValidateRM(%T) reports %d %s issues %+v, want 1", f.root, len(got), limitsRule, got)
			}
			i := got[0]
			if i.Code != "rm_invariant" || i.Path != "/" || i.Layer != validation.LayerRM || i.Detail != detail {
				t.Errorf("ValidateRM(%T) issue = %s at %q, layer %s, Detail %q; want rm_invariant at \"/\", layer rm, Detail %q", f.root, i.Code, i.Path, i.Layer, i.Detail, detail)
			}
			if v := i.Value.Reveal(); v != want {
				t.Errorf("ValidateRM(%T) issue Value.Reveal() = %T %v, want the compared magnitudes %T %v", f.root, v, v, want, want)
			}
		})
	}
	checkInOrder(t, name, upper, lower)
}

// checkInOrder validates every form of the interval from lower to upper, two
// bounds in order, lower below or equal to upper. None may draw a
// Limits_consistent issue.
func checkInOrder[T rm.DVOrdered](t *testing.T, name string, lower, upper T) {
	t.Helper()
	for _, f := range orderForms(lower, upper) {
		t.Run(name+"/"+f.name+"/in order", func(t *testing.T) {
			if got := limitsIssues(validation.ValidateRM(f.root)); len(got) > 0 {
				t.Errorf("ValidateRM(%T) reports %+v, want no %s issue on bounds in order", f.root, got, limitsRule)
			}
		})
	}
}

// REQ-112 — two strictly comparable bounds with lower above upper break
// Limits_consistent, for each bound type, in each interval form; the same
// bounds in order do not. The order is the RM's less_than for the type:
// magnitude for DV_COUNT, DV_QUANTITY and DV_PROPORTION (numerator over
// denominator), value for DV_ORDINAL and DV_SCALE, and for the temporal
// types the magnitude the REQ-123 parse gives (days since 0001-01-01 for a
// date, seconds for the others). That magnitude reads the clock, so two
// DV_TIME or DV_DATE_TIME values in different time zones are ordered as
// instants instead, and Value holds each one's magnitude in UTC. The floor
// does not compare units_system, so a same-units pair is ordered whatever its
// units_system.
func TestValidateRM_REQ112_IntervalOrderEveryBoundType(t *testing.T) {
	t.Parallel()
	metric, imperial := "metric", "imperial"
	checkOrder(t, "DV_COUNT", "DV_COUNT", rm.DVCount{Magnitude: 5}, rm.DVCount{Magnitude: 1}, [2]float64{5, 1})
	checkOrder(t, "DV_QUANTITY", "DV_QUANTITY",
		rm.DVQuantity{Magnitude: 5.5, Units: "mm"}, rm.DVQuantity{Magnitude: 1.5, Units: "mm"}, [2]float64{5.5, 1.5})
	checkOrder(t, "DV_QUANTITY of one units in two units systems", "DV_QUANTITY",
		rm.DVQuantity{Magnitude: 5.5, Units: "mm", UnitsSystem: &metric}, rm.DVQuantity{Magnitude: 1.5, Units: "mm", UnitsSystem: &imperial}, [2]float64{5.5, 1.5})
	checkOrder(t, "DV_ORDINAL", "DV_ORDINAL",
		rm.DVOrdinal{Value: 3, Symbol: orderSymbol("local", "at3")}, rm.DVOrdinal{Value: 1, Symbol: orderSymbol("local", "at1")}, [2]float64{3, 1})
	checkOrder(t, "DV_SCALE", "DV_SCALE",
		rm.DVScale{Value: 2.5, Symbol: orderSymbol("local", "at3")}, rm.DVScale{Value: 0.5, Symbol: orderSymbol("local", "at1")}, [2]float64{2.5, 0.5})
	checkOrder(t, "DV_PROPORTION", "DV_PROPORTION",
		rm.DVProportion{Numerator: 3, Denominator: 4}, rm.DVProportion{Numerator: 1, Denominator: 4}, [2]float64{0.75, 0.25})
	checkOrder(t, "DV_DATE", "DV_DATE",
		rm.DVDate{Value: "2025-01-01"}, rm.DVDate{Value: "2024-01-01"}, [2]float64{739251, 738885})
	checkOrder(t, "DV_TIME", "DV_TIME",
		rm.DVTime{Value: "10:00:00"}, rm.DVTime{Value: "08:00:00"}, [2]float64{36000, 28800})
	checkOrder(t, "DV_DATE_TIME", "DV_DATE_TIME",
		rm.DVDateTime{Value: "2025-01-01T00:00:00Z"}, rm.DVDateTime{Value: "2024-01-01T00:00:00Z"}, [2]float64{63871286400, 63839664000})
	checkOrder(t, "DV_DURATION", "DV_DURATION",
		rm.DVDuration{Value: "P2D"}, rm.DVDuration{Value: "P1D"}, [2]float64{172800, 86400})

	// In different zones the clocks mislead. 08:00Z is after 10:00+05:00,
	// which is 05:00Z; 2025-01-01T23:00-05:00 is 2025-01-02T04:00Z, after
	// 2025-01-02T01:00Z. So each pair is out of order as instants although
	// its clocks read in order, and the reversed pair is in order although its
	// clocks do not read so.
	checkOrder(t, "DV_TIME in different zones", "DV_TIME",
		rm.DVTime{Value: "08:00:00Z"}, rm.DVTime{Value: "10:00:00+05:00"}, [2]float64{28800, 18000})
	checkOrder(t, "DV_DATE_TIME in different zones", "DV_DATE_TIME",
		rm.DVDateTime{Value: "2025-01-01T23:00:00-05:00"}, rm.DVDateTime{Value: "2025-01-02T01:00:00Z"}, [2]float64{63871387200, 63871376400})
	// Two values in one zone are ordered by their clocks, as two values with
	// no zone are, and Value holds the clock magnitudes.
	checkOrder(t, "DV_TIME in one zone", "DV_TIME",
		rm.DVTime{Value: "10:00:00+05:00"}, rm.DVTime{Value: "08:00:00+05:00"}, [2]float64{36000, 28800})
	checkOrder(t, "DV_DATE_TIME in one zone", "DV_DATE_TIME",
		rm.DVDateTime{Value: "2025-01-01T10:00:00+05:00"}, rm.DVDateTime{Value: "2025-01-01T08:00:00+05:00"}, [2]float64{63871322400, 63871315200})
	// Two values at one instant are in order, lower equal to upper: Z and
	// +00:00 name one offset, and 10:00+05:00 is 05:00Z although its clock
	// reads later.
	checkInOrder(t, "DV_TIME at one instant in Z and +00:00", rm.DVTime{Value: "10:00:00Z"}, rm.DVTime{Value: "10:00:00+00:00"})
	checkInOrder(t, "DV_DATE_TIME at one instant in +00:00 and Z", rm.DVDateTime{Value: "2025-01-01T10:00:00+00:00"}, rm.DVDateTime{Value: "2025-01-01T10:00:00Z"})
	checkInOrder(t, "DV_TIME at one instant in two offsets", rm.DVTime{Value: "10:00:00+05:00"}, rm.DVTime{Value: "05:00:00Z"})
}

// REQ-112, REQ-171 — the floor orders no pair the RM does not make strictly
// comparable, none whose bound has no magnitude to order, and none with an
// unbounded side. Nor does it order a DV_TIME or DV_DATE_TIME with a time
// zone against one without, or two in different zones when either is not an
// instant ToTime can give: a partial value or a leap second. DV_ORDINAL and
// DV_SCALE symbols are comparable only when they are coded in one named
// terminology, so a symbol with an empty terminology id, on either side, is
// not ordered. In each row the lower bound reads above the upper one, so
// ordering the pair would report it; no Limits_consistent issue is reported,
// and Coverage.Unevaluated lists Interval.Limits_comparable, the rule an
// incomparable pair breaks, which the floor does not evaluate.
func TestValidateRM_REQ112_REQ171_IntervalOrderSkipsUnorderedPairs(t *testing.T) {
	t.Parallel()
	closed := func(lower, upper rm.DVOrdered) any { return new(rm.NewClosedDVInterval(lower, upper)) }
	local, snomed, none := orderSymbol("local", "at3"), orderSymbol("SNOMED-CT", "260385009"), orderSymbol("", "at3")
	// An open side carries no bound, so the zero value a typed interval
	// holds there is not compared with the other bound.
	upperOpen := rm.NewClosedDVInterval(rm.DVCount{Magnitude: 5}, rm.DVCount{})
	upperOpen.UpperIncluded, upperOpen.UpperUnbounded = false, true
	lowerOpen := rm.NewClosedDVInterval(rm.DVCount{}, rm.DVCount{Magnitude: -5})
	lowerOpen.LowerIncluded, lowerOpen.LowerUnbounded = false, true
	cases := []struct {
		name string
		root any
	}{
		{name: "DV_COUNT, upper side unbounded", root: &upperOpen},
		{name: "DV_COUNT, lower side unbounded", root: &lowerOpen},
		{name: "DV_QUANTITY, upper magnitude NaN", root: new(rm.NewClosedDVInterval(rm.DVQuantity{Magnitude: 5, Units: "mm"}, rm.DVQuantity{Magnitude: rm.Real(math.NaN()), Units: "mm"}))},
		{name: "DV_QUANTITY in different units", root: new(rm.NewClosedDVInterval(rm.DVQuantity{Magnitude: 5, Units: "cm"}, rm.DVQuantity{Magnitude: 1, Units: "m"}))},
		{name: "DV_ORDINAL symbols in different terminologies", root: new(rm.NewClosedDVInterval(rm.DVOrdinal{Value: 3, Symbol: local}, rm.DVOrdinal{Value: 1, Symbol: snomed}))},
		{name: "DV_ORDINAL symbols coded in no terminology", root: new(rm.NewClosedDVInterval(rm.DVOrdinal{Value: 3, Symbol: none}, rm.DVOrdinal{Value: 1, Symbol: none}))},
		{name: "DV_ORDINAL, lower symbol coded in no terminology", root: new(rm.NewClosedDVInterval(rm.DVOrdinal{Value: 3, Symbol: none}, rm.DVOrdinal{Value: 1, Symbol: local}))},
		{name: "DV_ORDINAL, upper symbol coded in no terminology", root: new(rm.NewClosedDVInterval(rm.DVOrdinal{Value: 3, Symbol: local}, rm.DVOrdinal{Value: 1, Symbol: none}))},
		{name: "DV_SCALE symbols in different terminologies", root: new(rm.NewClosedDVInterval(rm.DVScale{Value: 2.5, Symbol: local}, rm.DVScale{Value: 0.5, Symbol: snomed}))},
		{name: "DV_SCALE symbols coded in no terminology", root: new(rm.NewClosedDVInterval(rm.DVScale{Value: 2.5, Symbol: none}, rm.DVScale{Value: 0.5, Symbol: none}))},
		{name: "DV_SCALE, lower symbol coded in no terminology", root: new(rm.NewClosedDVInterval(rm.DVScale{Value: 2.5, Symbol: none}, rm.DVScale{Value: 0.5, Symbol: local}))},
		{name: "DV_SCALE, upper symbol coded in no terminology", root: new(rm.NewClosedDVInterval(rm.DVScale{Value: 2.5, Symbol: local}, rm.DVScale{Value: 0.5, Symbol: none}))},
		{name: "DV_PROPORTION of different types", root: new(rm.NewClosedDVInterval(rm.DVProportion{Numerator: 3, Denominator: 4, Type: 0}, rm.DVProportion{Numerator: 1, Denominator: 4, Type: 3}))},
		{name: "DV_PROPORTION, lower denominator zero", root: new(rm.NewClosedDVInterval(rm.DVProportion{Numerator: 3, Denominator: 0}, rm.DVProportion{Numerator: 1, Denominator: 4}))},
		{name: "DV_PROPORTION, upper denominator zero", root: new(rm.NewClosedDVInterval(rm.DVProportion{Numerator: 1, Denominator: 4}, rm.DVProportion{Numerator: -1, Denominator: 0}))},
		{name: "DV_DATE, upper not a date", root: new(rm.NewClosedDVInterval(rm.DVDate{Value: "2025-01-01"}, rm.DVDate{Value: "example"}))},
		{name: "DV_DATE, lower not a date", root: new(rm.NewClosedDVInterval(rm.DVDate{Value: "example"}, rm.DVDate{Value: "0000-01-01"}))},
		{name: "DV_TIME, upper not a time", root: new(rm.NewClosedDVInterval(rm.DVTime{Value: "10:00:00"}, rm.DVTime{Value: "example"}))},
		{name: "DV_DATE_TIME, upper not a date-time", root: new(rm.NewClosedDVInterval(rm.DVDateTime{Value: "2025-01-01T00:00:00Z"}, rm.DVDateTime{Value: "example"}))},
		{name: "DV_DATE_TIME, lower not a date-time", root: new(rm.NewClosedDVInterval(rm.DVDateTime{Value: "example"}, rm.DVDateTime{Value: "0000-01-01T00:00:00"}))},
		// Each pair below reads lower above upper by the clocks, and also as
		// instants with a value without a zone taken as UTC (as ToTime takes
		// it) and a partial value's clock moved to UTC, so a floor that
		// ordered the pair either way would report it.
		{name: "DV_TIME with a zone over one without", root: new(rm.NewClosedDVInterval(rm.DVTime{Value: "10:00:00Z"}, rm.DVTime{Value: "08:00:00"}))},
		{name: "DV_TIME without a zone over one with", root: new(rm.NewClosedDVInterval(rm.DVTime{Value: "10:00:00"}, rm.DVTime{Value: "08:00:00+01:00"}))},
		{name: "DV_DATE_TIME with a zone over one without", root: new(rm.NewClosedDVInterval(rm.DVDateTime{Value: "2025-01-01T10:00:00+01:00"}, rm.DVDateTime{Value: "2025-01-01T08:00:00"}))},
		{name: "DV_DATE_TIME without a zone over one with", root: new(rm.NewClosedDVInterval(rm.DVDateTime{Value: "2025-01-01T10:00:00"}, rm.DVDateTime{Value: "2025-01-01T08:00:00Z"}))},
		{name: "DV_TIME in different zones, lower partial", root: new(rm.NewClosedDVInterval(rm.DVTime{Value: "10:00+01:00"}, rm.DVTime{Value: "08:00:00Z"}))},
		{name: "DV_DATE_TIME in different zones, upper partial", root: new(rm.NewClosedDVInterval(rm.DVDateTime{Value: "2025-01-01T10:00:00+01:00"}, rm.DVDateTime{Value: "2025-01-01T08:00Z"}))},
		{name: "DV_DATE_TIME in different zones, upper a leap second", root: new(rm.NewClosedDVInterval(rm.DVDateTime{Value: "2017-01-01T01:30:00+01:00"}, rm.DVDateTime{Value: "2016-12-31T23:59:60Z"}))},
		{name: "DV_DURATION, upper not a duration", root: new(rm.NewClosedDVInterval(rm.DVDuration{Value: "P2D"}, rm.DVDuration{Value: "example"}))},
		{name: "DV_DURATION, lower not a duration", root: new(rm.NewClosedDVInterval(rm.DVDuration{Value: "example"}, rm.DVDuration{Value: "-P1D"}))},
		{name: "DV_COUNT over a unitless DV_QUANTITY", root: closed(rm.DVCount{Magnitude: 10}, rm.DVQuantity{Magnitude: 5})},
		{name: "unitless DV_QUANTITY over a DV_COUNT", root: closed(&rm.DVQuantity{Magnitude: 10}, &rm.DVCount{Magnitude: 5})},
		{name: "DV_DATE_TIME over a DV_DATE", root: closed(rm.DVDateTime{Value: "2025-01-01T00:00:00Z"}, rm.DVDate{Value: "2024-01-01"})},
		{name: "DV_ORDINAL over a DV_SCALE in one terminology", root: closed(rm.DVOrdinal{Value: 3, Symbol: local}, rm.DVScale{Value: 1, Symbol: local})},
		{name: "nil lower bound on a bounded side", root: closed(nil, rm.DVCount{Magnitude: -1})},
		{name: "typed-nil lower bound on a bounded side", root: closed((*rm.DVCount)(nil), &rm.DVCount{Magnitude: -1})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validation.ValidateRM(tc.root)
			if got := limitsIssues(r); len(got) > 0 {
				t.Errorf("ValidateRM(%s) reports %+v, want no %s issue: the floor does not order this pair", tc.name, got, limitsRule)
			}
			if !slices.Contains(r.Coverage.Unevaluated, "Interval.Limits_comparable") {
				t.Errorf("ValidateRM(%s) Coverage.Unevaluated = %q, want it to list Interval.Limits_comparable", tc.name, r.Coverage.Unevaluated)
			}
		})
	}
}
