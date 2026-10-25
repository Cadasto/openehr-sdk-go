package validation_test

// rmfloor_interval_comparable_test.go: REQ-112 DV_INTERVAL, the strict
// comparability half of RM DV_INTERVAL.Limits_consistent. With neither side
// unbounded and a bound on each, two bounds the floor reads as not strictly
// comparable (bounds of two classes, two DV_QUANTITY in different units, two
// DV_PROPORTION of different types) break the rule whatever their order, and
// draw one issue at the interval with the rule's name and no value. The pairs
// the RM makes strictly comparable are ordered as before
// (rmfloor_interval_order_test.go), and the floor still neither orders nor
// reports two DV_ORDINAL or DV_SCALE coded in different terminologies.

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// Details of the comparability finding. Each names the classes of the bounds
// and never a bound's value, units or type (REQ-168).
const (
	unitsDiffer = "DV_INTERVAL<DV_QUANTITY>: lower and upper are not strictly comparable, their units differ (RM Limits_consistent)"
	typesDiffer = "DV_INTERVAL<DV_PROPORTION>: lower and upper are not strictly comparable, their types differ (RM Limits_consistent)"
)

// classesDiffer is the Detail of the comparability finding on bounds of the
// classes lower and upper.
func classesDiffer(lower, upper string) string {
	return "DV_INTERVAL: lower " + lower + " and upper " + upper + " are not strictly comparable (RM Limits_consistent)"
}

// checkNotComparable validates root and requires exactly one
// DV_INTERVAL.Limits_consistent issue: rm_invariant at path on the RM layer,
// with detail and an empty Value, its Detail free of every string in
// secrets. Coverage.Unevaluated still lists Interval.Limits_comparable, which
// BASE declares on every interval and the floor does not evaluate on a
// Proper_interval or a Point_interval.
func checkNotComparable(t *testing.T, root any, path, detail string, secrets ...string) {
	t.Helper()
	r := validation.ValidateRM(root)
	got := limitsIssues(r)
	if len(got) != 1 {
		t.Fatalf("ValidateRM(%T) reports %d %s issues %+v, want 1: the bounds are not strictly comparable", root, len(got), limitsRule, got)
	}
	i := got[0]
	if i.Code != "rm_invariant" || i.Path != path || i.Layer != validation.LayerRM || i.Detail != detail {
		t.Errorf("ValidateRM(%T) issue = %s at %q, layer %s, Detail %q; want rm_invariant at %q, layer rm, Detail %q", root, i.Code, i.Path, i.Layer, i.Detail, path, detail)
	}
	if v := i.Value.Reveal(); v != nil {
		t.Errorf("ValidateRM(%T) issue Value.Reveal() = %T %v, want an empty Value (REQ-168)", root, v, v)
	}
	for _, s := range secrets {
		if strings.Contains(i.Detail, s) {
			t.Errorf("ValidateRM(%T) issue Detail %q holds the submitted %q (REQ-168)", root, i.Detail, s)
		}
	}
	if !slices.Contains(r.Coverage.Unevaluated, "Interval.Limits_comparable") {
		t.Errorf("ValidateRM(%T) Coverage.Unevaluated = %q, want it to list Interval.Limits_comparable", root, r.Coverage.Unevaluated)
	}
}

// REQ-112 — two DV_QUANTITY bounds in different units are not strictly
// comparable, so they break DV_INTERVAL.Limits_consistent in every interval
// form the floor reads, whichever way their magnitudes lie, and the floor
// does not order them: 5 mg against 3 g draws one issue, not two. The same
// two bounds in one unit draw nothing when they are in order, and the
// ordering issue, with its compared magnitudes, when they are not.
func TestValidateRM_REQ112_IntervalQuantitiesInDifferentUnits(t *testing.T) {
	t.Parallel()
	q := func(m float64, units string) rm.DVQuantity { return rm.DVQuantity{Magnitude: rm.Real(m), Units: units} }
	for _, pair := range []struct {
		name         string
		lower, upper rm.DVQuantity
	}{
		{name: "5 mg .. 3 g", lower: q(5, "mg"), upper: q(3, "g")},
		{name: "3 mg .. 5 g", lower: q(3, "mg"), upper: q(5, "g")},
		{name: "5 mmol/l .. 3 mg/dl", lower: q(5, "mmol/l"), upper: q(3, "mg/dl")},
		{name: "1 mm .. 5 with no units", lower: q(1, "mm"), upper: q(5, "")},
		{name: "5 cm .. 1 m", lower: q(5, "cm"), upper: q(1, "m")},
	} {
		for _, f := range orderForms(pair.lower, pair.upper) {
			t.Run(pair.name+"/"+f.name, func(t *testing.T) {
				checkNotComparable(t, f.root, "/", unitsDiffer, "mmol", "mg/dl")
			})
		}
	}
	checkInOrder(t, "3 mg .. 5 mg", q(3, "mg"), q(5, "mg"))
	checkOrder(t, "5 mg .. 3 mg", "DV_QUANTITY", q(5, "mg"), q(3, "mg"), [2]float64{5, 3})
}

// REQ-112 — two DV_PROPORTION bounds of different types are not strictly
// comparable, in order by ratio or not; two of one type are ordered.
func TestValidateRM_REQ112_IntervalProportionsOfDifferentTypes(t *testing.T) {
	t.Parallel()
	ratio := rm.DVProportion{Numerator: 1, Denominator: 4, Type: 0}
	fraction := rm.DVProportion{Numerator: 3, Denominator: 4, Type: 3}
	for _, pair := range []struct {
		name         string
		lower, upper rm.DVProportion
	}{
		{name: "a ratio of 1/4 .. a fraction of 3/4", lower: ratio, upper: fraction},
		{name: "a fraction of 3/4 .. a ratio of 1/4", lower: fraction, upper: ratio},
		{name: "a ratio of 3/4 .. a fraction of 1/4", lower: rm.DVProportion{Numerator: 3, Denominator: 4, Type: 0}, upper: rm.DVProportion{Numerator: 1, Denominator: 4, Type: 3}},
	} {
		for _, f := range orderForms(pair.lower, pair.upper) {
			t.Run(pair.name+"/"+f.name, func(t *testing.T) {
				checkNotComparable(t, f.root, "/", typesDiffer)
			})
		}
	}
	checkInOrder(t, "two ratios in order", ratio, rm.DVProportion{Numerator: 3, Denominator: 4, Type: 0})
}

// REQ-112 — bounds of two classes are not strictly comparable: each RM
// class defines is_strictly_comparable_to against its own class, and the
// floor reads DV_COUNT's, whose definition sets no condition, the same way.
// Only the bare DV_INTERVAL holds two classes, by value or by pointer. A pair
// in order by magnitude breaks the rule as one out of order does.
func TestValidateRM_REQ112_IntervalBoundsOfTwoClasses(t *testing.T) {
	t.Parallel()
	local := orderSymbol("local", "at3")
	closed := func(lower, upper rm.DVOrdered) rm.DVInterval[rm.DVOrdered] {
		return rm.NewClosedDVInterval(lower, upper)
	}
	cases := []struct {
		name                   string
		iv                     rm.DVInterval[rm.DVOrdered]
		lowerClass, upperClass string
	}{
		{name: "DV_COUNT over a unitless DV_QUANTITY", iv: closed(rm.DVCount{Magnitude: 10}, rm.DVQuantity{Magnitude: 5}), lowerClass: "DV_COUNT", upperClass: "DV_QUANTITY"},
		{name: "unitless DV_QUANTITY over a DV_COUNT", iv: closed(&rm.DVQuantity{Magnitude: 10}, &rm.DVCount{Magnitude: 5}), lowerClass: "DV_QUANTITY", upperClass: "DV_COUNT"},
		{name: "DV_COUNT below a DV_QUANTITY", iv: closed(rm.DVCount{Magnitude: 1}, rm.DVQuantity{Magnitude: 5, Units: "mm"}), lowerClass: "DV_COUNT", upperClass: "DV_QUANTITY"},
		{name: "DV_DATE_TIME over a DV_DATE", iv: closed(rm.DVDateTime{Value: "2025-01-01T00:00:00Z"}, rm.DVDate{Value: "2024-01-01"}), lowerClass: "DV_DATE_TIME", upperClass: "DV_DATE"},
		{name: "DV_ORDINAL over a DV_SCALE in one terminology", iv: closed(rm.DVOrdinal{Value: 3, Symbol: local}, rm.DVScale{Value: 1, Symbol: local}), lowerClass: "DV_ORDINAL", upperClass: "DV_SCALE"},
		{name: "DV_DURATION below a DV_PROPORTION", iv: closed(&rm.DVDuration{Value: "P1D"}, &rm.DVProportion{Numerator: 1, Denominator: 1, Type: 1}), lowerClass: "DV_DURATION", upperClass: "DV_PROPORTION"},
	}
	for _, tc := range cases {
		detail := classesDiffer(tc.lowerClass, tc.upperClass)
		t.Run(tc.name+"/by pointer", func(t *testing.T) { checkNotComparable(t, &tc.iv, "/", detail) })
		t.Run(tc.name+"/by value", func(t *testing.T) { checkNotComparable(t, tc.iv, "/", detail) })
	}
}

// REQ-112 — the mixed-unit pair of the reference template test_intervals
// (not vendored), an ELEMENT whose value is a DV_INTERVAL<DV_QUANTITY> with
// a lower bound in mg and an upper one in g, decoded from canonical JSON as
// a CDR receives it: the floor reports it at the ELEMENT's value.
func TestValidateRM_REQ112_DecodedIntervalInMixedUnits(t *testing.T) {
	t.Parallel()
	body := `{"_type":"ELEMENT","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"Interval of quantity"},` +
		`"value":{"_type":"DV_INTERVAL","lower":{"_type":"DV_QUANTITY","magnitude":5,"units":"mg"},` +
		`"upper":{"_type":"DV_QUANTITY","magnitude":3,"units":"g"},` +
		`"lower_included":true,"upper_included":true,"lower_unbounded":false,"upper_unbounded":false}}`
	v, err := typereg.Default.Decode([]byte(body))
	if err != nil {
		t.Fatalf("typereg.Decode(%s): %v", body, err)
	}
	checkNotComparable(t, v, "/value", unitsDiffer)
	if r := validation.ValidateRM(v); r.OK {
		t.Errorf("ValidateRM(%s).OK = true, want false: the interval breaks DV_INTERVAL.Limits_consistent", body)
	}
}
