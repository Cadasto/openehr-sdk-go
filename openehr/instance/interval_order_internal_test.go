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
	parsed, err := fixtures.ParseOPTBytes([]byte(opt))
	if err != nil {
		t.Fatalf("ParseOPTBytes: %v", err)
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
			iv.Lower.Magnitude, iv.Upper.Magnitude = tc.bounds[0], tc.bounds[1]
			iv.LowerUnbounded = tc.lowerOpen
			orderIntervalBounds(node, iv)
			if got := [2]int64{iv.Lower.Magnitude, iv.Upper.Magnitude}; got != tc.want {
				t.Errorf("orderIntervalBounds(%v) = %v, want %v", tc.bounds, got, tc.want)
			}
		})
	}
}

// TestOrderIntervalBoundsQuantity pins the REQ-107 ordering rule on a
// DV_INTERVAL<DV_QUANTITY>: its units rule and the magnitude ends it reads
// from a C_DV_QUANTITY entry. A case with no entries uses no OPT at all.
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
			name:   "different units: left alone",
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
// DV_DATE, DV_TIME and DV_DATE_TIME. The generator writes fixed-width
// ISO-8601 strings, so the strings themselves are the order. A C_DATE,
// C_TIME or C_DATE_TIME has no numeric end, so an inversion is repaired
// by swapping, and a side the OPT leaves open is left alone.
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
			name:   "different types: left alone",
			bounds: [2]rm.DVProportion{p(2, 1, 0), p(1, 2, 1)},
			want:   [2]rm.DVProportion{p(2, 1, 0), p(1, 2, 1)},
		},
		{
			name:   "zero denominator: left alone",
			bounds: [2]rm.DVProportion{p(1, 0, 0), p(1, 1, 0)},
			want:   [2]rm.DVProportion{p(1, 0, 0), p(1, 1, 0)},
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
			name:   "unsatisfiable: left alone",
			node:   node(RealRange(Closed(10), Closed(20)), RealRange(Closed(0), Closed(5))),
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
