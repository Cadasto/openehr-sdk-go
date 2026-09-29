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
