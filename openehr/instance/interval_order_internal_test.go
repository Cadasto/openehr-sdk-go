package instance

import (
	"fmt"
	"math"
	"strings"
	"testing"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// intervalNode compiles an OPT text and returns its DV_INTERVAL node.
func intervalNode(t *testing.T, opt string) *tcimpl.CompiledNode {
	t.Helper()
	parsed, err := fixtures.ReadOperationalTemplate([]byte(opt))
	if err != nil {
		t.Fatalf("ReadOperationalTemplate: %v", err)
	}
	c, err := templatecompile.Compile(parsed)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	var find func(n *tcimpl.CompiledNode) *tcimpl.CompiledNode
	find = func(n *tcimpl.CompiledNode) *tcimpl.CompiledNode {
		if strings.HasPrefix(n.RMTypeName(), "DV_INTERVAL") {
			return n
		}
		for _, attr := range n.Attributes() {
			for _, child := range attr.Children() {
				if found := find(child); found != nil {
					return found
				}
			}
		}
		return nil
	}
	node := find(c.Root())
	if node == nil {
		t.Fatal("compiled OPT has no DV_INTERVAL node")
	}
	return node
}

// TestOrderIntervalBoundsCount pins each step of the REQ-107 ordering rule
// on a DV_INTERVAL<DV_COUNT>. The bounds are set directly, to values the
// sampler would not pick, against the interval node of an edited copy of
// the vendored DV_COUNT interval OPT.
func TestOrderIntervalBoundsCount(t *testing.T) {
	opt := ReadVendoredOPT(t, CountIntervalOPT)
	cases := []struct {
		name         string
		lower, upper string // the two bounds' C_INTEGER bodies
		lowerOpen    bool
		bounds, want [2]int64
	}{
		{
			name:   "in order: left alone",
			lower:  IntRange(Closed(10), Closed(20)),
			upper:  IntRange(Closed(0), Closed(100)),
			bounds: [2]int64{12, 50}, want: [2]int64{12, 50},
		},
		{
			name:   "the swapped pair fits",
			lower:  IntRange(Closed(0), Closed(100)),
			upper:  IntRange(Closed(0), Closed(100)),
			bounds: [2]int64{70, 30}, want: [2]int64{30, 70},
		},
		{
			// 50 is not in the lower list, so no swap; 55 and 60 are the
			// lowest and highest the two sides accept.
			name:   "the lowest lower with the highest upper",
			lower:  IntList(70, 55),
			upper:  IntRange(Closed(50), Closed(60)),
			bounds: [2]int64{70, 50}, want: [2]int64{55, 60},
		},
		{
			name:   "exclusive ends",
			lower:  IntRange(Open(10), Closed(20)),
			upper:  IntRange(Closed(0), Open(15)),
			bounds: [2]int64{18, 5}, want: [2]int64{11, 14},
		},
		{
			// 15 is above the upper range, so no swap; the lower range has
			// no lowest value, so the upper bound 5 stands in.
			name:   "no lower end: the upper bound stands in",
			lower:  IntRange(NoEnd, Closed(20)),
			upper:  IntRange(Closed(0), Closed(10)),
			bounds: [2]int64{15, 5}, want: [2]int64{5, 10},
		},
		{
			name:   "no upper end: the lower bound stands in",
			lower:  IntRange(Closed(10), Closed(20)),
			upper:  IntRange(Closed(0), NoEnd),
			bounds: [2]int64{15, 5}, want: [2]int64{10, 15},
		},
		{
			// Every lower value is above every upper value.
			name:   "unsatisfiable: left alone",
			lower:  IntRange(Closed(50), Closed(60)),
			upper:  IntRange(Closed(0), Closed(10)),
			bounds: [2]int64{55, 5}, want: [2]int64{55, 5},
		},
		{
			name:      "open side: left alone",
			lower:     IntRange(Closed(0), Closed(100)),
			upper:     IntRange(Closed(0), Closed(100)),
			lowerOpen: true,
			bounds:    [2]int64{70, 30}, want: [2]int64{70, 30},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := intervalNode(t, EditCountBounds(t, opt, tc.lower, tc.upper))
			iv := &rm.DVInterval[rm.DVCount]{}
			iv.Lower.Magnitude, iv.Upper.Magnitude = rm.Integer64(tc.bounds[0]), rm.Integer64(tc.bounds[1])
			iv.LowerUnbounded = tc.lowerOpen
			orderIntervalBounds(node, iv)
			if got := [2]int64{int64(iv.Lower.Magnitude), int64(iv.Upper.Magnitude)}; got != tc.want {
				t.Errorf("orderIntervalBounds(%v) = %v, want %v", tc.bounds, got, tc.want)
			}
		})
	}
}

// TestOrderIntervalBoundsQuantity pins the REQ-107 ordering rule on a
// DV_INTERVAL<DV_QUANTITY>: its units rule and the magnitude ends it reads
// from a C_DV_QUANTITY entry. A case with no entries uses no OPT at all. Two
// bounds in different units are first given one unit both sides admit
// (TestREQ107_ShareComparableQuantity pins how), and are left alone where
// there is none.
func TestOrderIntervalBoundsQuantity(t *testing.T) {
	opt := ReadVendoredOPT(t, QuantityIntervalOPT)
	q := func(m float64, units string) quantityKey { return quantityKey{magnitude: m, units: units} }
	cases := []struct {
		name         string
		lower, upper string // the two bounds' C_DV_QUANTITY entries; "" for no OPT
		lowerOpen    bool
		bounds, want [2]quantityKey
	}{
		{
			// The upper side admits any units, so it takes kg, keeping its
			// magnitude, and the pair is then ordered (shareComparable).
			name:   "different units, no OPT: one unit, then swapped",
			bounds: [2]quantityKey{q(10, "kg"), q(5, "g")}, want: [2]quantityKey{q(5, "kg"), q(10, "kg")},
		},
		{
			name:   "different units, no unit fits both: left alone",
			lower:  QuantityEntry("kg", Closed(0), Closed(100)),
			upper:  QuantityEntry("g", Closed(0), Closed(100)),
			bounds: [2]quantityKey{q(10, "kg"), q(5, "g")}, want: [2]quantityKey{q(10, "kg"), q(5, "g")},
		},
		{
			name:   "same units, no OPT: swapped",
			bounds: [2]quantityKey{q(10, "kg"), q(5, "kg")}, want: [2]quantityKey{q(5, "kg"), q(10, "kg")},
		},
		{
			name:   "the lowest lower with the highest upper",
			lower:  QuantityEntry("Cel", Closed(40), Closed(60)),
			upper:  QuantityEntry("Cel", Closed(0), Closed(50)),
			bounds: [2]quantityKey{q(55, "Cel"), q(45, "Cel")}, want: [2]quantityKey{q(40, "Cel"), q(50, "Cel")},
		},
		{
			name:   "exclusive ends: the nearest float64 inside",
			lower:  QuantityEntry("Cel", Open(40), Closed(60)),
			upper:  QuantityEntry("Cel", Closed(0), Open(50)),
			bounds: [2]quantityKey{q(55, "Cel"), q(45, "Cel")},
			want:   [2]quantityKey{q(math.Nextafter(40, math.Inf(1)), "Cel"), q(math.Nextafter(50, math.Inf(-1)), "Cel")},
		},
		{
			name:   "no lower end: the upper bound stands in",
			lower:  QuantityEntry("Cel", NoEnd, Closed(60)),
			upper:  QuantityEntry("Cel", Closed(0), Closed(50)),
			bounds: [2]quantityKey{q(55, "Cel"), q(45, "Cel")}, want: [2]quantityKey{q(45, "Cel"), q(50, "Cel")},
		},
		{
			name:   "no upper end: the lower bound stands in",
			lower:  QuantityEntry("Cel", Closed(40), Closed(60)),
			upper:  QuantityEntry("Cel", Closed(0), NoEnd),
			bounds: [2]quantityKey{q(55, "Cel"), q(30, "Cel")}, want: [2]quantityKey{q(40, "Cel"), q(55, "Cel")},
		},
		{
			// The lower side admits kg only, so no Cel value fits it.
			name:   "no entry in these units on the lower side: left alone",
			lower:  QuantityEntry("kg", Closed(0), Closed(100)),
			upper:  QuantityEntry("Cel", Closed(0), Closed(100)),
			bounds: [2]quantityKey{q(55, "Cel"), q(45, "Cel")}, want: [2]quantityKey{q(55, "Cel"), q(45, "Cel")},
		},
		{
			name:      "open side: left alone",
			lowerOpen: true,
			bounds:    [2]quantityKey{q(10, "kg"), q(5, "kg")}, want: [2]quantityKey{q(10, "kg"), q(5, "kg")},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var node *tcimpl.CompiledNode
			if tc.lower != "" {
				node = intervalNode(t, EditQuantityBounds(t, opt, tc.lower, tc.upper))
			}
			iv := &rm.DVInterval[rm.DVQuantity]{}
			setQuantityKey(&iv.Lower, tc.bounds[0])
			setQuantityKey(&iv.Upper, tc.bounds[1])
			iv.LowerUnbounded = tc.lowerOpen
			orderIntervalBounds(node, iv)
			if got := [2]quantityKey{quantityKeyOf(iv.Lower), quantityKeyOf(iv.Upper)}; got != tc.want {
				t.Errorf("orderIntervalBounds(%v) = %v, want %v", tc.bounds, got, tc.want)
			}
		})
	}
}

// describeBound names a DV_INTERVAL<DV_ORDERED> bound's form, type and
// compared value, since a DV_COUNT or DV_QUANTITY value does not compare
// with ==.
func describeBound(v rm.DVOrdered) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case *rm.DVCount:
		if x == nil {
			return "(*DV_COUNT)(nil)"
		}
		return fmt.Sprintf("*DV_COUNT %d", x.Magnitude)
	case rm.DVCount:
		return fmt.Sprintf("DV_COUNT %d", x.Magnitude)
	case *rm.DVQuantity:
		if x == nil {
			return "(*DV_QUANTITY)(nil)"
		}
		return fmt.Sprintf("*DV_QUANTITY %v %s", x.Magnitude, x.Units)
	case rm.DVQuantity:
		return fmt.Sprintf("DV_QUANTITY %v %s", x.Magnitude, x.Units)
	}
	return fmt.Sprintf("%T", v)
}

// TestOrderIntervalBoundsOrdered pins the REQ-107 ordering rule on a
// DV_INTERVAL<DV_ORDERED>, whose bounds may be counts or quantities in
// value or pointer form: each form is kept, and a mixed or missing pair is
// left alone.
func TestOrderIntervalBoundsOrdered(t *testing.T) {
	countNode := intervalNode(t, EditCountBounds(t, ReadVendoredOPT(t, CountIntervalOPT),
		IntList(70, 55), IntRange(Closed(50), Closed(60))))
	lowerPtr, upperPtr := &rm.DVCount{Magnitude: 70}, &rm.DVCount{Magnitude: 50}
	var nilCount *rm.DVCount
	cases := []struct {
		name         string
		node         *tcimpl.CompiledNode
		lower, upper rm.DVOrdered
		lowerOpen    bool
		want         [2]string
	}{
		{
			// 50 is not in the lower list, so no swap: the lowest lower and
			// highest upper, written through the two pointers.
			name:  "pointer counts: changed in place",
			node:  countNode,
			lower: lowerPtr, upper: upperPtr,
			want: [2]string{"*DV_COUNT 55", "*DV_COUNT 60"},
		},
		{
			name:  "value counts: swapped",
			lower: rm.DVCount{Magnitude: 70}, upper: rm.DVCount{Magnitude: 30},
			want: [2]string{"DV_COUNT 30", "DV_COUNT 70"},
		},
		{
			name:  "pointer quantities: swapped",
			lower: &rm.DVQuantity{Magnitude: 70, Units: "kg"}, upper: &rm.DVQuantity{Magnitude: 30, Units: "kg"},
			want: [2]string{"*DV_QUANTITY 30 kg", "*DV_QUANTITY 70 kg"},
		},
		{
			name:  "value quantities: swapped",
			lower: rm.DVQuantity{Magnitude: 70, Units: "kg"}, upper: rm.DVQuantity{Magnitude: 30, Units: "kg"},
			want: [2]string{"DV_QUANTITY 30 kg", "DV_QUANTITY 70 kg"},
		},
		{
			name:  "a count and a quantity: left alone",
			lower: rm.DVCount{Magnitude: 70}, upper: rm.DVQuantity{Magnitude: 30, Units: "kg"},
			want: [2]string{"DV_COUNT 70", "DV_QUANTITY 30 kg"},
		},
		{
			// No units, so the units rule alone would not keep the count out.
			name:  "a quantity and a count: left alone",
			lower: rm.DVQuantity{Magnitude: 70}, upper: rm.DVCount{Magnitude: 30},
			want: [2]string{"DV_QUANTITY 70 ", "DV_COUNT 30"},
		},
		{
			name:  "typed-nil lower: left alone",
			lower: nilCount, upper: rm.DVCount{Magnitude: 30},
			want: [2]string{"(*DV_COUNT)(nil)", "DV_COUNT 30"},
		},
		{
			name:  "open side: left alone",
			lower: rm.DVCount{Magnitude: 70}, upper: rm.DVCount{Magnitude: 30},
			lowerOpen: true,
			want:      [2]string{"DV_COUNT 70", "DV_COUNT 30"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			iv := &rm.DVInterval[rm.DVOrdered]{}
			iv.Lower, iv.Upper = tc.lower, tc.upper
			iv.LowerUnbounded = tc.lowerOpen
			orderIntervalBounds(tc.node, iv)
			if got := [2]string{describeBound(iv.Lower), describeBound(iv.Upper)}; got != tc.want {
				t.Errorf("orderIntervalBounds = %q, want %q", got, tc.want)
			}
		})
	}
	if lowerPtr.Magnitude != 55 || upperPtr.Magnitude != 60 {
		t.Errorf("pointer counts after ordering = [%d, %d], want [55, 60] written through the pointers", lowerPtr.Magnitude, upperPtr.Magnitude)
	}
}

// TestOrderIntervalBoundsTemporal pins the REQ-107 ordering rule on
// DV_DATE, DV_TIME and DV_DATE_TIME bounds the generator writes: values in
// one zone, or with none, compare at the precision they share
// (TestREQ107_TemporalBoundsCompareAtSharedPrecision and
// TestREQ107_TemporalBoundsCompareAsInstants pin the rest). Bounds the OPT
// does not constrain are swapped, and a side the OPT leaves open is left
// alone; TestREQ107_TemporalBoundsTakeTheirRangeEnds pins the ends of a
// C_DATE, C_TIME or C_DATE_TIME range.
func TestOrderIntervalBoundsTemporal(t *testing.T) {
	cases := []struct {
		name         string
		apply        func(lo, hi string, open bool) (gotLo, gotHi string)
		lower, upper string
		lowerOpen    bool
		want         [2]string
	}{
		{
			name:  "date: swapped",
			apply: orderDates,
			lower: "2020-06-01", upper: "2020-01-01",
			want: [2]string{"2020-01-01", "2020-06-01"},
		},
		{
			name:  "date: in order, left alone",
			apply: orderDates,
			lower: "2020-01-01", upper: "2020-06-01",
			want: [2]string{"2020-01-01", "2020-06-01"},
		},
		{
			name:  "date: open side, left alone",
			apply: orderDates,
			lower: "2020-06-01", upper: "2020-01-01", lowerOpen: true,
			want: [2]string{"2020-06-01", "2020-01-01"},
		},
		{
			name:  "time: swapped",
			apply: orderTimes,
			lower: "23:00:00", upper: "01:00:00",
			want: [2]string{"01:00:00", "23:00:00"},
		},
		{
			name:  "date-time: swapped",
			apply: orderDateTimes,
			lower: "2020-06-01T00:00:00Z", upper: "2020-01-01T00:00:00Z",
			want: [2]string{"2020-01-01T00:00:00Z", "2020-06-01T00:00:00Z"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotLo, gotHi := tc.apply(tc.lower, tc.upper, tc.lowerOpen)
			if gotLo != tc.want[0] || gotHi != tc.want[1] {
				t.Errorf("orderIntervalBounds(%q, %q) = (%q, %q), want (%q, %q)", tc.lower, tc.upper, gotLo, gotHi, tc.want[0], tc.want[1])
			}
		})
	}

	t.Run("DV_ORDERED pointer dates: swapped in place", func(t *testing.T) {
		lower, upper := &rm.DVDate{Value: "2020-06-01"}, &rm.DVDate{Value: "2020-01-01"}
		iv := &rm.DVInterval[rm.DVOrdered]{}
		iv.Lower, iv.Upper = lower, upper
		orderIntervalBounds(nil, iv)
		if lower.Value != "2020-01-01" || upper.Value != "2020-06-01" {
			t.Errorf("pointer dates after ordering = (%q, %q), want (%q, %q)", lower.Value, upper.Value, "2020-01-01", "2020-06-01")
		}
	})
	t.Run("DV_ORDERED value date-times: swapped", func(t *testing.T) {
		iv := &rm.DVInterval[rm.DVOrdered]{}
		iv.Lower = rm.DVDateTime{Value: "2020-06-01T00:00:00Z"}
		iv.Upper = rm.DVDateTime{Value: "2020-01-01T00:00:00Z"}
		orderIntervalBounds(nil, iv)
		if got := [2]string{orderedString(iv.Lower), orderedString(iv.Upper)}; got != [2]string{"2020-01-01T00:00:00Z", "2020-06-01T00:00:00Z"} {
			t.Errorf("orderIntervalBounds = %q, want swapped date-times", got)
		}
	})
}

func proportionParts(p rm.DVProportion) [3]float64 {
	return [3]float64{float64(p.Numerator), float64(p.Denominator), float64(p.Type)}
}

func ordinalParts(o rm.DVOrdinal) [2]string {
	return [2]string{fmt.Sprintf("%d", o.Value), o.Symbol.DefiningCode.CodeString}
}

func orderDates(lo, hi string, open bool) (string, string) {
	iv := &rm.DVInterval[rm.DVDate]{}
	iv.Lower.Value, iv.Upper.Value = lo, hi
	iv.LowerUnbounded = open
	orderIntervalBounds(nil, iv)
	return iv.Lower.Value, iv.Upper.Value
}

func orderTimes(lo, hi string, open bool) (string, string) {
	iv := &rm.DVInterval[rm.DVTime]{}
	iv.Lower.Value, iv.Upper.Value = lo, hi
	iv.LowerUnbounded = open
	orderIntervalBounds(nil, iv)
	return iv.Lower.Value, iv.Upper.Value
}

func orderDateTimes(lo, hi string, open bool) (string, string) {
	iv := &rm.DVInterval[rm.DVDateTime]{}
	iv.Lower.Value, iv.Upper.Value = lo, hi
	iv.LowerUnbounded = open
	orderIntervalBounds(nil, iv)
	return iv.Lower.Value, iv.Upper.Value
}

func orderedString(v rm.DVOrdered) string {
	switch x := v.(type) {
	case rm.DVDate:
		return x.Value
	case *rm.DVDate:
		return x.Value
	case rm.DVTime:
		return x.Value
	case *rm.DVTime:
		return x.Value
	case rm.DVDateTime:
		return x.Value
	case *rm.DVDateTime:
		return x.Value
	default:
		return fmt.Sprintf("%T", v)
	}
}

// TestOrderIntervalBoundsProportion pins the REQ-107 ordering rule on
// DV_PROPORTION. The compared key is the ratio numerator/denominator, and
// only when both bounds have the same type; DV_PROPORTION.magnitude is not
// implemented on the generated type. A zero denominator does not compare.
// Two bounds are first given one type both sides admit, one in which an
// ordered pair fits where there is one (TestREQ107_ShareComparableProportion
// pins how), and are left alone where there is none.
func TestOrderIntervalBoundsProportion(t *testing.T) {
	p := func(num, den float64, kind int32) rm.DVProportion {
		return rm.DVProportion{Numerator: rm.Real(num), Denominator: rm.Real(den), Type: rm.Integer(kind)}
	}
	opt := ReadVendoredOPT(t, CountIntervalOPT)
	node := func(lowerBody, upperBody string) *tcimpl.CompiledNode {
		t.Helper()
		return intervalNode(t, RetargetInterval(t, opt, "DV_PROPORTION", ProportionBound(lowerBody), ProportionBound(upperBody)))
	}
	cases := []struct {
		name         string
		node         *tcimpl.CompiledNode
		bounds, want [2]rm.DVProportion
	}{
		{
			// Numerators are equal; the ratios 1/2 and 1/4 are not.
			name:   "ratio, not the numerator: swapped",
			bounds: [2]rm.DVProportion{p(1, 2, 0), p(1, 4, 0)},
			want:   [2]rm.DVProportion{p(1, 4, 0), p(1, 2, 0)},
		},
		{
			// Numerators are 2 > 1, but the ratios 2/8 and 1/2 are in order.
			name:   "ratio already in order: left alone",
			bounds: [2]rm.DVProportion{p(2, 8, 0), p(1, 2, 0)},
			want:   [2]rm.DVProportion{p(2, 8, 0), p(1, 2, 0)},
		},
		{
			// The upper side admits a ratio, so it is settled as one, keeping
			// 1/2, and the pair is then ordered (shareComparable).
			name:   "different types, no OPT: one type, then swapped",
			bounds: [2]rm.DVProportion{p(2, 1, 0), p(1, 2, 1)},
			want:   [2]rm.DVProportion{p(1, 2, 0), p(2, 1, 0)},
		},
		{
			name: "different types, no type fits both: left alone",
			node: intervalNode(t, RetargetInterval(t, opt, "DV_PROPORTION",
				ProportionKindBound(RealRange(Closed(0), Closed(10)), IntList(0), ""),
				ProportionKindBound(RealRange(Closed(0), Closed(10)), IntList(1), ""))),
			bounds: [2]rm.DVProportion{p(2, 1, 0), p(1, 2, 1)},
			want:   [2]rm.DVProportion{p(2, 1, 0), p(1, 2, 1)},
		},
		{
			// Both sides admit only ratios, so the pair keeps its type.
			name: "zero denominator: left alone",
			node: intervalNode(t, RetargetInterval(t, opt, "DV_PROPORTION",
				ProportionKindBound(RealRange(Closed(0), Closed(10)), IntList(0), ""),
				ProportionKindBound(RealRange(Closed(0), Closed(10)), IntList(0), ""))),
			bounds: [2]rm.DVProportion{p(1, 0, 0), p(1, 1, 0)},
			want:   [2]rm.DVProportion{p(1, 0, 0), p(1, 1, 0)},
		},
		{
			// No ordered pair is a ratio over the denominator 0, and the sides,
			// which the OPT leaves open, admit a unitary proportion, over 1.
			name:   "zero denominator, no OPT: the first type with an ordered pair",
			bounds: [2]rm.DVProportion{p(1, 0, 0), p(1, 1, 0)},
			want:   [2]rm.DVProportion{p(1, 1, 1), p(1, 1, 1)},
		},
		{
			// 10 is not in the lower list, so no swap; 12 and 15 are the ends.
			name:   "the lowest lower numerator with the highest upper",
			node:   node(RealList(20, 12), RealRange(Closed(0), Closed(15))),
			bounds: [2]rm.DVProportion{p(20, 1, 0), p(10, 1, 0)},
			want:   [2]rm.DVProportion{p(12, 1, 0), p(15, 1, 0)},
		},
		{
			// A negative denominator reverses which numerator end is the low ratio.
			name:   "negative denominator: the high numerator is the low ratio",
			node:   node(RealList(2, 8), RealList(3, 9)),
			bounds: [2]rm.DVProportion{p(2, -1, 0), p(9, -1, 0)},
			want:   [2]rm.DVProportion{p(8, -1, 0), p(3, -1, 0)},
		},
		{
			// No numerator orders the pair over 1, but the lower side's
			// denominator is open: 15/3 meets the upper side's highest, 5/1.
			name:   "numerators out of order over one denominator: the lower denominator meets the upper ratio",
			node:   node(RealRange(Closed(10), Closed(20)), RealRange(Closed(0), Closed(5))),
			bounds: [2]rm.DVProportion{p(15, 1, 0), p(3, 1, 0)},
			want:   [2]rm.DVProportion{p(15, 3, 0), p(5, 1, 0)},
		},
		{
			name: "unsatisfiable: left alone",
			node: intervalNode(t, RetargetInterval(t, opt, "DV_PROPORTION",
				ProportionKindBoundOver(RealRange(Closed(10), Closed(20)), RealList(1), IntList(0), ""),
				ProportionKindBoundOver(RealRange(Closed(0), Closed(5)), RealList(1), IntList(0), ""))),
			bounds: [2]rm.DVProportion{p(15, 1, 0), p(3, 1, 0)},
			want:   [2]rm.DVProportion{p(15, 1, 0), p(3, 1, 0)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			iv := &rm.DVInterval[rm.DVProportion]{}
			iv.Lower, iv.Upper = tc.bounds[0], tc.bounds[1]
			orderIntervalBounds(tc.node, iv)
			if got, want := [2][3]float64{proportionParts(iv.Lower), proportionParts(iv.Upper)}, [2][3]float64{proportionParts(tc.want[0]), proportionParts(tc.want[1])}; got != want {
				t.Errorf("orderIntervalBounds(%v) = %v, want %v", tc.bounds, got, want)
			}
		})
	}
	t.Run("DV_ORDERED pointer proportions: ratio swapped in place", func(t *testing.T) {
		lower := &rm.DVProportion{Numerator: 1, Denominator: 2}
		upper := &rm.DVProportion{Numerator: 1, Denominator: 4}
		iv := &rm.DVInterval[rm.DVOrdered]{}
		iv.Lower, iv.Upper = lower, upper
		orderIntervalBounds(nil, iv)
		if lower.Denominator != 4 || upper.Denominator != 2 {
			t.Errorf("pointer proportions after ordering = (%v, %v), want denominators 4 and 2", lower.Denominator, upper.Denominator)
		}
	})
}

// TestREQ107_ProportionBoundsChangeADenominator pins the REQ-107 Intervals
// rule on two DV_PROPORTION of one type, by ratio, whose only in-order form
// changes a denominator: a side whose C_REAL on the denominator has an end,
// or a list, takes there the denominator that puts its ratio furthest
// toward its own end, as 5/10 lies below 1/1 where 5/1 does not. Where no
// such end orders the pair, a side takes the denominator that puts its
// ratio at the other bound's, or just past it on its own side, as 5/50
// meets 0.1/1 over an open denominator: the lower side first, then the
// upper side. A pair that is in order with each bound's own denominator
// keeps them. A denominator stays where the kind fixes it, 100 for a
// percent, and is a whole number where the kind, a precision of 0 or an
// is_integral pin needs whole numbers (Precedence 4).
func TestREQ107_ProportionBoundsChangeADenominator(t *testing.T) {
	p := func(num, den float64, kind int32) rm.DVProportion {
		return rm.DVProportion{Numerator: rm.Real(num), Denominator: rm.Real(den), Type: rm.Integer(kind)}
	}
	p0 := func(num, den float64, kind int32) rm.DVProportion {
		v := p(num, den, kind)
		v.Precision = new(rm.Integer(0))
		return v
	}
	opt := ReadVendoredOPT(t, CountIntervalOPT)
	node := func(lower, upper string) *tcimpl.CompiledNode {
		t.Helper()
		return intervalNode(t, RetargetInterval(t, opt, "DV_PROPORTION", lower, upper))
	}
	ratio := func(numerator, denominator string) string {
		return ProportionKindBoundOver(numerator, denominator, IntList(0), "")
	}
	cases := []struct {
		name         string
		node         *tcimpl.CompiledNode
		bounds, want [2]rm.DVProportion
	}{
		{
			name:   "the lower denominator moves to its high end",
			node:   node(ratio(RealList(5), RealRange(Closed(1), Closed(10))), ratio(RealList(1), RealList(1))),
			bounds: [2]rm.DVProportion{p(5, 1, 0), p(1, 1, 0)},
			want:   [2]rm.DVProportion{p(5, 10, 0), p(1, 1, 0)},
		},
		{
			name:   "the upper denominator moves to its low end",
			node:   node(ratio(RealList(3), RealList(1)), ratio(RealList(2), RealRange(Closed(0.5), Closed(4)))),
			bounds: [2]rm.DVProportion{p(3, 1, 0), p(2, 4, 0)},
			want:   [2]rm.DVProportion{p(3, 1, 0), p(2, 0.5, 0)},
		},
		{
			// 5/-1 is the lowest ratio over denominators from -10 to -1.
			name:   "a negative denominator moves to the end nearest 0",
			node:   node(ratio(RealList(5), RealRange(Closed(-10), Closed(-1))), ratio(RealList(-1), RealList(1))),
			bounds: [2]rm.DVProportion{p(5, -10, 0), p(-1, 1, 0)},
			want:   [2]rm.DVProportion{p(5, -1, 0), p(-1, 1, 0)},
		},
		{
			// 10/1 with 15/1 is in order, so neither takes 10 as denominator.
			name: "each bound's own denominator kept where that orders the pair",
			node: node(ratio(RealRange(Closed(10), Closed(20)), RealRange(Closed(1), Closed(10))),
				ratio(RealRange(Closed(0), Closed(15)), RealRange(Closed(1), Closed(10)))),
			bounds: [2]rm.DVProportion{p(20, 1, 0), p(10, 1, 0)},
			want:   [2]rm.DVProportion{p(10, 1, 0), p(15, 1, 0)},
		},
		{
			// 500/1000 would lie below 100/100, but a percent is over 100.
			name: "a percent keeps its denominator of 100: left alone",
			node: node(ProportionKindBoundOver(RealList(500), RealRange(Closed(1), Closed(1000)), IntList(2), ""),
				ProportionKindBoundOver(RealList(100), RealRange(Closed(1), Closed(1000)), IntList(2), "")),
			bounds: [2]rm.DVProportion{p(500, 100, 2), p(100, 100, 2)},
			want:   [2]rm.DVProportion{p(500, 100, 2), p(100, 100, 2)},
		},
		{
			// The whole number nearest the high end 10.5 inside it is 10.
			name: "an is_integral side takes a whole denominator",
			node: node(ProportionKindBoundOver(RealList(5), RealRange(Closed(1.5), Closed(10.5)), IntList(0), IntegralPin()),
				ratio(RealList(1), RealList(1))),
			bounds: [2]rm.DVProportion{p(5, 2, 0), p(1, 1, 0)},
			want:   [2]rm.DVProportion{p(5, 10, 0), p(1, 1, 0)},
		},
		{
			name: "a fraction takes a whole denominator",
			node: node(ProportionKindBoundOver(RealList(5), RealRange(Closed(1.5), Closed(10.5)), IntList(3), ""),
				ProportionKindBoundOver(RealList(1), RealList(1), IntList(3), "")),
			bounds: [2]rm.DVProportion{p(5, 2, 3), p(1, 1, 3)},
			want:   [2]rm.DVProportion{p(5, 10, 3), p(1, 1, 3)},
		},
		{
			// The lower denominator is open, so no end of it lists 50.
			name:   "an open lower denominator meets the upper ratio",
			node:   node(ratio(RealList(5), ""), ratio(RealList(0.1), RealList(1))),
			bounds: [2]rm.DVProportion{p(5, 1, 0), p(0.1, 1, 0)},
			want:   [2]rm.DVProportion{p(5, 50, 0), p(0.1, 1, 0)},
		},
		{
			// 5/17.5 is 2/7; the denominator range has no high end.
			name:   "a lower denominator bounded below alone meets the upper ratio",
			node:   node(ratio(RealList(5), RealRange(Closed(1), NoEnd)), ratio(RealList(2), RealList(7))),
			bounds: [2]rm.DVProportion{p(5, 1, 0), p(2, 7, 0)},
			want:   [2]rm.DVProportion{p(5, 17.5, 0), p(2, 7, 0)},
		},
		{
			// 5/17.5 meets 2/7; the whole denominator that keeps 5 over it
			// at or below 2/7 is 18, not 17.
			name: "a fraction meets the other ratio over a whole denominator",
			node: node(ProportionKindBoundOver(RealList(5), RealRange(Closed(1), NoEnd), IntList(3), ""),
				ProportionKindBoundOver(RealList(2), RealList(7), IntList(3), "")),
			bounds: [2]rm.DVProportion{p(5, 1, 3), p(2, 7, 3)},
			want:   [2]rm.DVProportion{p(5, 18, 3), p(2, 7, 3)},
		},
		{
			name: "an is_integral side meets the other ratio over a whole denominator",
			node: node(ProportionKindBoundOver(RealList(5), RealRange(Closed(1), NoEnd), IntList(0), IntegralPin()),
				ratio(RealList(2), RealList(7))),
			bounds: [2]rm.DVProportion{p(5, 1, 0), p(2, 7, 0)},
			want:   [2]rm.DVProportion{p(5, 18, 0), p(2, 7, 0)},
		},
		{
			name: "a bound of precision 0 meets the other ratio over a whole denominator",
			node: node(ProportionKindBoundOver(RealList(5), RealRange(Closed(1), NoEnd), IntList(0), RequiredPrecision(IntList(0))),
				ratio(RealList(2), RealList(7))),
			bounds: [2]rm.DVProportion{p0(5, 1, 0), p(2, 7, 0)},
			want:   [2]rm.DVProportion{p0(5, 18, 0), p(2, 7, 0)},
		},
		{
			// The lower side lists the denominator 1 alone, so the upper side
			// meets 6/1 over 0.5.
			name:   "the upper denominator meets the lower ratio",
			node:   node(ratio(RealList(6), RealList(1)), ratio(RealList(3), "")),
			bounds: [2]rm.DVProportion{p(6, 1, 0), p(3, 1, 0)},
			want:   [2]rm.DVProportion{p(6, 1, 0), p(3, 0.5, 0)},
		},
		{
			// 1000/100 is a percent's only denominator, so no side moves it.
			name: "a percent keeps its denominator of 100 against an open one: left alone",
			node: node(ProportionKindBoundOver(RealList(500), "", IntList(2), ""),
				ProportionKindBoundOver(RealList(100), "", IntList(2), "")),
			bounds: [2]rm.DVProportion{p(500, 100, 2), p(100, 100, 2)},
			want:   [2]rm.DVProportion{p(500, 100, 2), p(100, 100, 2)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			iv := &rm.DVInterval[rm.DVProportion]{}
			iv.Lower, iv.Upper = tc.bounds[0], tc.bounds[1]
			orderIntervalBounds(tc.node, iv)
			in := [2]string{proportionWithPrecision(tc.bounds[0]), proportionWithPrecision(tc.bounds[1])}
			got := [2]string{proportionWithPrecision(iv.Lower), proportionWithPrecision(iv.Upper)}
			want := [2]string{proportionWithPrecision(tc.want[0]), proportionWithPrecision(tc.want[1])}
			if got != want {
				t.Errorf("orderIntervalBounds(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

// TestOrderIntervalBoundsKeepIsIntegral pins the REQ-107 ordering rule on
// DV_PROPORTION bounds whose side pins is_integral to true (Precedence 4):
// a bound the ordering writes on such a side has a whole-number numerator
// and denominator, whatever its kind and its precision, and the precision
// 0 where the bound had it and the side admits it. That holds for the
// lowest and highest ends of each side, for a side that adopts the other
// bound's ratio on its own denominator, and for a swap, which a bound that
// is not whole cannot make onto such a side.
func TestOrderIntervalBoundsKeepIsIntegral(t *testing.T) {
	p := func(num, den float64, precision int32) rm.DVProportion {
		return rm.DVProportion{Numerator: rm.Real(num), Denominator: rm.Real(den), Precision: new(rm.Integer(precision))}
	}
	opt := ReadVendoredOPT(t, CountIntervalOPT)
	node := func(lower, upper string) *tcimpl.CompiledNode {
		t.Helper()
		return intervalNode(t, RetargetInterval(t, opt, "DV_PROPORTION", lower, upper))
	}
	// A ratio side that pins is_integral and requires the precision 2, so
	// no precision of 0 makes its numbers whole.
	integral := func(numerator, denominator string) string {
		return ProportionKindBoundOver(numerator, denominator, IntList(0), IntegralPin()+RequiredPrecision(IntList(2)))
	}
	cases := []struct {
		name         string
		node         *tcimpl.CompiledNode
		bounds, want [2]rm.DVProportion
	}{
		{
			// The swap puts 18 outside the upper range, so each side takes
			// the whole number nearest its end: 11 and 15, not 10.5 and 15.5.
			name: "ends of each side",
			node: node(integral(RealRange(Closed(10.5), Closed(20.5)), ""),
				integral(RealRange(Closed(0.5), Closed(15.5)), "")),
			bounds: [2]rm.DVProportion{p(18, 1, 2), p(12, 1, 2)},
			want:   [2]rm.DVProportion{p(11, 1, 2), p(15, 1, 2)},
		},
		{
			// The upper numerator has no high end, so that side adopts the
			// lower bound 19/3 on its own denominator 2: 12.67 is not whole,
			// so it takes 13.
			name: "a side that adopts the other bound's ratio",
			node: node(integral(RealRange(Closed(10.5), Closed(20.5)), RealList(3)),
				integral(RealRange(Closed(0.5), NoEnd), RealList(2))),
			bounds: [2]rm.DVProportion{p(19, 3, 2), p(4, 2, 2)},
			want:   [2]rm.DVProportion{p(11, 3, 2), p(13, 2, 2)},
		},
		{
			// The lower side pins nothing, so its 12.5 is valid there, but a
			// swap would put it on the upper side, which pins is_integral.
			name: "no swap of a number that is not whole onto such a side",
			node: node(ProportionKindBound(RealRange(Closed(0), Closed(20)), IntList(0), RequiredPrecision(IntList(2))),
				integral(RealRange(Closed(0), Closed(20)), "")),
			bounds: [2]rm.DVProportion{p(12.5, 1, 2), p(3, 1, 2)},
			want:   [2]rm.DVProportion{p(0, 1, 2), p(20, 1, 2)},
		},
		{
			// Both sides admit the precision 0, which each bound has, so the
			// ends keep it.
			name: "the precision 0 kept",
			node: node(ProportionKindBound(RealRange(Closed(10.5), Closed(20.5)), IntList(0), IntegralPin()+RequiredPrecision(IntList(2, 0))),
				ProportionKindBound(RealRange(Closed(0.5), Closed(15.5)), IntList(0), IntegralPin()+RequiredPrecision(IntList(2, 0)))),
			bounds: [2]rm.DVProportion{p(18, 1, 0), p(12, 1, 0)},
			want:   [2]rm.DVProportion{p(11, 1, 0), p(15, 1, 0)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			iv := &rm.DVInterval[rm.DVProportion]{}
			iv.Lower, iv.Upper = tc.bounds[0], tc.bounds[1]
			orderIntervalBounds(tc.node, iv)
			in := [2]string{proportionWithPrecision(tc.bounds[0]), proportionWithPrecision(tc.bounds[1])}
			got := [2]string{proportionWithPrecision(iv.Lower), proportionWithPrecision(iv.Upper)}
			want := [2]string{proportionWithPrecision(tc.want[0]), proportionWithPrecision(tc.want[1])}
			if got != want {
				t.Errorf("orderIntervalBounds(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

// TestREQ107_FractionBoundTakesNoPrecisionOrZero pins the REQ-107 ordering
// rule on the precision a fraction or an integer fraction bound takes where
// the ordering rewrites it (proportionPrecision): the bound's own where it
// is none or 0, else none, else 0, the first its side admits, and never 1,
// -1 or any other, at which the RM floor reads the bound as not is_integral
// and so breaking Fraction_validity (REQ-112). A side that admits neither
// none nor 0 gives no precision. A ratio still keeps its own precision.
func TestREQ107_FractionBoundTakesNoPrecisionOrZero(t *testing.T) {
	opt := ReadVendoredOPT(t, CountIntervalOPT)
	side := func(kind, precisionAttr string) *tcimpl.CompiledNode {
		t.Helper()
		bound := ProportionKindBound(RealRange(Closed(0), Closed(10)), kind, precisionAttr)
		lower, _ := boundNodes(intervalNode(t, RetargetInterval(t, opt, "DV_PROPORTION", bound, bound)))
		return lower
	}
	key := func(kind int64, precision *rm.Integer) proportionKey {
		return proportionKey{numerator: 1, denominator: 3, kind: kind, precision: precision}
	}
	text := func(p *rm.Integer) string {
		if p == nil {
			return "none"
		}
		return fmt.Sprint(*p)
	}
	cases := []struct {
		name string
		side *tcimpl.CompiledNode
		like proportionKey
		// want is the precision taken, nil for none; ok is false where the
		// side admits none.
		want *rm.Integer
		ok   bool
	}{
		{
			name: "a fraction at precision 1 over a side that requires 1 or 0 takes 0",
			side: side(IntList(3), RequiredPrecision(IntList(1, 0))),
			like: key(pkFraction, new(rm.Integer(1))),
			want: new(rm.Integer(0)), ok: true,
		},
		{
			name: "a fraction at precision -1 over a side that admits -1 or 1, or none, takes none",
			side: side(IntList(3), OptionalPrecision(IntList(-1, 1))),
			like: key(pkFraction, new(rm.Integer(-1))),
			want: nil, ok: true,
		},
		{
			name: "an integer fraction with no precision over a side that requires 1, -1 or 0 takes 0",
			side: side(IntList(4), RequiredPrecision(IntList(1, -1, 0))),
			like: key(pkIntegerFraction, nil),
			want: new(rm.Integer(0)), ok: true,
		},
		{
			name: "a fraction at precision 2 over an open precision takes none",
			side: side(IntList(3), ""),
			like: key(pkFraction, new(rm.Integer(2))),
			want: nil, ok: true,
		},
		{
			name: "a fraction keeps its own precision 0",
			side: side(IntList(3), RequiredPrecision(IntList(1, 0))),
			like: key(pkFraction, new(rm.Integer(0))),
			want: new(rm.Integer(0)), ok: true,
		},
		{
			name: "a fraction over a side that requires 1 or -1 takes no precision",
			side: side(IntList(3), RequiredPrecision(IntList(1, -1))),
			like: key(pkFraction, new(rm.Integer(1))),
			ok:   false,
		},
		{
			name: "a ratio keeps its own precision 1",
			side: side(IntList(0), RequiredPrecision(IntList(1, 0))),
			like: key(pkRatio, new(rm.Integer(1))),
			want: new(rm.Integer(1)), ok: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := proportionPrecision(tc.side, tc.like)
			if ok != tc.ok || (ok && text(got) != text(tc.want)) {
				t.Errorf("proportionPrecision(type %d, precision %s) = %s, %v; want %s, %v",
					tc.like.kind, text(tc.like.precision), text(got), ok, text(tc.want), tc.ok)
			}
		})
	}
}

// proportionWithPrecision renders a DV_PROPORTION bound as
// numerator/denominator type t precision p, with "none" for no precision.
func proportionWithPrecision(p rm.DVProportion) string {
	precision := "none"
	if p.Precision != nil {
		precision = fmt.Sprint(*p.Precision)
	}
	return fmt.Sprintf("%v/%v type %d precision %s", p.Numerator, p.Denominator, p.Type, precision)
}

// TestOrderIntervalBoundsOrdinal pins the REQ-107 ordering rule on
// DV_ORDINAL, compared by Value. The symbol travels with the value on a
// swap, and an extreme taken from the OPT carries that entry's symbol.
func TestOrderIntervalBoundsOrdinal(t *testing.T) {
	opt := ReadVendoredOPT(t, CountIntervalOPT)
	node := func(lower, upper []int) *tcimpl.CompiledNode {
		t.Helper()
		return intervalNode(t, RetargetInterval(t, opt, "DV_ORDINAL", OrdinalBound(lower...), OrdinalBound(upper...)))
	}
	ord := func(v int, code string) rm.DVOrdinal {
		return rm.DVOrdinal{
			Value: rm.Integer(v),
			Symbol: rm.DVCodedText{
				DVText:       rm.DVText{Value: code},
				DefiningCode: rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: "local"}},
			},
		}
	}
	cases := []struct {
		name         string
		node         *tcimpl.CompiledNode
		lowerOpen    bool
		bounds, want [2]rm.DVOrdinal
	}{
		{
			name:   "the swapped pair keeps each symbol",
			node:   node([]int{1, 2}, []int{1, 2}),
			bounds: [2]rm.DVOrdinal{ord(2, "at0002"), ord(1, "at0001")},
			want:   [2]rm.DVOrdinal{ord(1, "at0001"), ord(2, "at0002")},
		},
		{
			// 4 is not in the lower list, so no swap. The extremes are 3
			// (code at0002 on the lower list) and 6 (code at0002 on the upper).
			name:   "the lowest lower with the highest upper",
			node:   node([]int{5, 3}, []int{4, 6}),
			bounds: [2]rm.DVOrdinal{ord(5, "at0001"), ord(4, "at0001")},
			want:   [2]rm.DVOrdinal{ord(3, "at0002"), ord(6, "at0002")},
		},
		{
			name:   "unsatisfiable: left alone",
			node:   node([]int{5, 6}, []int{1, 2}),
			bounds: [2]rm.DVOrdinal{ord(6, "at0002"), ord(1, "at0001")},
			want:   [2]rm.DVOrdinal{ord(6, "at0002"), ord(1, "at0001")},
		},
		{
			name:      "open side: left alone",
			lowerOpen: true,
			bounds:    [2]rm.DVOrdinal{ord(2, "at0002"), ord(1, "at0001")},
			want:      [2]rm.DVOrdinal{ord(2, "at0002"), ord(1, "at0001")},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			iv := &rm.DVInterval[rm.DVOrdinal]{}
			iv.Lower, iv.Upper = tc.bounds[0], tc.bounds[1]
			iv.LowerUnbounded = tc.lowerOpen
			orderIntervalBounds(tc.node, iv)
			if got, want := [2][2]string{ordinalParts(iv.Lower), ordinalParts(iv.Upper)}, [2][2]string{ordinalParts(tc.want[0]), ordinalParts(tc.want[1])}; got != want {
				t.Errorf("orderIntervalBounds = %q, want %q", got, want)
			}
		})
	}
	t.Run("DV_ORDERED pointer ordinals: swapped in place", func(t *testing.T) {
		lower, upper := &rm.DVOrdinal{Value: 3}, &rm.DVOrdinal{Value: 1}
		iv := &rm.DVInterval[rm.DVOrdered]{}
		iv.Lower, iv.Upper = lower, upper
		orderIntervalBounds(nil, iv)
		if lower.Value != 1 || upper.Value != 3 {
			t.Errorf("pointer ordinals after ordering = (%d, %d), want (1, 3)", lower.Value, upper.Value)
		}
	})
}
