package instance

import (
	"testing"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// listedUnitsBound is a DV_QUANTITY bound built as a C_COMPLEX_OBJECT: a
// C_REAL from 0 to 10 on its magnitude and itemBody, a C_STRING body, on its
// units.
func listedUnitsBound(itemBody string) string {
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_QUANTITY</rm_type_name>` +
		boundOccurrences + `<node_id></node_id>` +
		boundAttribute("magnitude", "REAL", "C_REAL", RealRange(Closed(0), Closed(10))) +
		boundAttribute("units", "STRING", "C_STRING", itemBody) + `</children>`
}

// TestREQ107_ShareComparableQuantity pins the REQ-107 Intervals rule that
// gives two DV_QUANTITY bounds one unit both sides admit, before they are
// ordered. Of the lower bound's units, the upper bound's, the units the lower
// side lists and then those the upper side lists, it is the first that both
// admit and in which an ordered pair fits both sides, and where no such unit
// has one, the first that both admit: two bounds in one unit take another
// only for an ordered pair. A side whose C_DV_QUANTITY names a property and
// lists no entry lists the property's units. A side whose C_DV_QUANTITY lists
// units takes the magnitude ExampleValue gives for its first entry in the new
// unit; any other side keeps its magnitude. A bound that carries a
// normal_range keeps its units. Where the two sides admit no one unit, the
// pair is left as it is. The bounds are set directly, and only
// shareComparable runs, so no order is put on them.
func TestREQ107_ShareComparableQuantity(t *testing.T) {
	quantityOPT := ReadVendoredOPT(t, QuantityIntervalOPT)
	countOPT := ReadVendoredOPT(t, CountIntervalOPT)
	listed := func(lower, upper string) *tcimpl.CompiledNode {
		t.Helper()
		return intervalNode(t, EditQuantityBounds(t, quantityOPT, lower, upper))
	}
	retarget := func(lower, upper string) *tcimpl.CompiledNode {
		t.Helper()
		return intervalNode(t, RetargetInterval(t, countOPT, "DV_QUANTITY", lower, upper))
	}
	entry := func(units string, lo, hi float64) string { return QuantityEntry(units, Closed(lo), Closed(hi)) }
	q := func(m float64, units string) quantityKey { return quantityKey{magnitude: m, units: units} }
	ranged := &rm.DVInterval[rm.DVQuantity]{}
	cases := []struct {
		name                     string
		node                     *tcimpl.CompiledNode
		lowerRanged, upperRanged bool
		bounds, want             [2]quantityKey
	}{
		{
			name:   "the lower bound's units, which the upper side admits",
			node:   listed(entry("mg", 0, 1000)+entry("g", 0, 10), entry("g", 1, 10)+entry("mg", 0, 1000)),
			bounds: [2]quantityKey{q(5, "mg"), q(3, "g")}, want: [2]quantityKey{q(5, "mg"), q(0, "mg")},
		},
		{
			name:   "else the upper bound's units, which the lower side admits",
			node:   listed(entry("mg", 0, 1000)+entry("g", 2, 10), entry("g", 1, 10)+entry("kg", 0, 1)),
			bounds: [2]quantityKey{q(5, "mg"), q(3, "g")}, want: [2]quantityKey{q(2, "g"), q(3, "g")},
		},
		{
			// Both sides admit g and cg; g comes first in the lower side's
			// list, cg in the upper side's.
			name:   "else the first units the lower side lists that both admit",
			node:   listed(entry("mg", 0, 1000)+entry("g", 2, 10)+entry("cg", 0, 100), entry("kg", 0, 1)+entry("cg", 0, 100)+entry("g", 1, 10)),
			bounds: [2]quantityKey{q(5, "mg"), q(0.5, "kg")}, want: [2]quantityKey{q(2, "g"), q(1, "g")},
		},
		{
			// The lower side's C_STRING admits mg and g by its pattern and
			// lists none, so g comes from the upper side's list. The lower
			// side keeps its magnitude.
			name:   "then the first units the upper side lists that both admit",
			node:   retarget(listedUnitsBound("<pattern>m?g</pattern>"), listedQuantityBound(entry("kg", 0, 1)+entry("g", 1, 10))),
			bounds: [2]quantityKey{q(5, "mg"), q(0.5, "kg")}, want: [2]quantityKey{q(5, "g"), q(1, "g")},
		},
		{
			// No ordered pair fits both sides in g, the lower side starting at
			// 5 g and the upper one ending at 1 g; one fits in kg.
			name:   "the first units both admit in which an ordered pair fits",
			node:   listed(entry("g", 5, 10)+entry("kg", 0, 1), entry("kg", 0, 1)+entry("g", 0, 1)),
			bounds: [2]quantityKey{q(5, "g"), q(0, "kg")}, want: [2]quantityKey{q(0, "kg"), q(0, "kg")},
		},
		{
			name:   "two bounds in one unit with no ordered pair take another",
			node:   listed(entry("g", 5, 10)+entry("kg", 0, 1), entry("kg", 0, 1)+entry("g", 0, 1)),
			bounds: [2]quantityKey{q(5, "g"), q(0.5, "g")}, want: [2]quantityKey{q(0, "kg"), q(0, "kg")},
		},
		{
			// No ordered pair fits both sides in g or in kg.
			name:   "no units both admit hold an ordered pair: the first both admit",
			node:   listed(entry("g", 5, 10)+entry("kg", 2, 3), entry("kg", 0, 1)+entry("g", 0, 1)),
			bounds: [2]quantityKey{q(5, "g"), q(0.5, "kg")}, want: [2]quantityKey{q(5, "g"), q(0, "g")},
		},
		{
			name:   "two bounds in one unit with no ordered pair anywhere: left alone",
			node:   listed(entry("g", 5, 10)+entry("kg", 2, 3), entry("kg", 0, 1)+entry("g", 0, 1)),
			bounds: [2]quantityKey{q(5, "g"), q(0.5, "g")}, want: [2]quantityKey{q(5, "g"), q(0.5, "g")},
		},
		{
			// The lower side admits mass, openehr::124, and lists no entry, so
			// it lists the property's units, among which g comes before mg;
			// without them, mg would come first, from the upper side's list.
			name:   "a side constrained by a property lists the property's units",
			node:   retarget(propertyQuantityBound("124"), listedUnitsBound("<list>m</list><list>mg</list><list>g</list>")),
			bounds: [2]quantityKey{q(0, "kg"), q(5, "m")}, want: [2]quantityKey{q(0, "g"), q(5, "g")},
		},
		{
			name:   "a side built attribute by attribute keeps its magnitude",
			node:   retarget(listedUnitsBound("<list>mg</list><list>g</list>"), listedQuantityBound(entry("g", 1, 10))),
			bounds: [2]quantityKey{q(5, "mg"), q(3, "g")}, want: [2]quantityKey{q(5, "g"), q(3, "g")},
		},
		{
			name:   "no unit both sides admit: left alone",
			node:   listed(entry("mg", 0, 10), entry("kg", 0, 10)),
			bounds: [2]quantityKey{q(5, "mg"), q(1, "kg")}, want: [2]quantityKey{q(5, "mg"), q(1, "kg")},
		},
		{
			name:   "no OPT: the upper side takes the lower bound's units",
			bounds: [2]quantityKey{q(5, "mg"), q(3, "g")}, want: [2]quantityKey{q(5, "mg"), q(3, "mg")},
		},
		{
			// Without its normal_range, the upper bound would take mg.
			name:        "an upper bound with a normal_range keeps its units",
			upperRanged: true,
			bounds:      [2]quantityKey{q(5, "mg"), q(3, "g")}, want: [2]quantityKey{q(5, "g"), q(3, "g")},
		},
		{
			name:        "two bounds with a normal_range each: left alone",
			lowerRanged: true, upperRanged: true,
			bounds: [2]quantityKey{q(5, "mg"), q(3, "g")}, want: [2]quantityKey{q(5, "mg"), q(3, "g")},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			iv := &rm.DVInterval[rm.DVQuantity]{}
			setQuantityKey(&iv.Lower, tc.bounds[0])
			setQuantityKey(&iv.Upper, tc.bounds[1])
			if tc.lowerRanged {
				iv.Lower.NormalRange = ranged
			}
			if tc.upperRanged {
				iv.Upper.NormalRange = ranged
			}
			shareComparable(tc.node, iv)
			if got := [2]quantityKey{quantityKeyOf(iv.Lower), quantityKeyOf(iv.Upper)}; got != tc.want {
				t.Errorf("shareComparable(%v) = %v, want %v", tc.bounds, got, tc.want)
			}
		})
	}
	t.Run("DV_ORDERED bounds keep their form", func(t *testing.T) {
		upper := &rm.DVQuantity{Magnitude: 3, Units: "g"}
		iv := &rm.DVInterval[rm.DVOrdered]{}
		iv.Lower, iv.Upper = rm.DVQuantity{Magnitude: 5, Units: "mg"}, upper
		shareComparable(nil, iv)
		if got := [2]string{describeBound(iv.Lower), describeBound(iv.Upper)}; got != [2]string{"DV_QUANTITY 5 mg", "*DV_QUANTITY 3 mg"} {
			t.Errorf("shareComparable = %q, want the upper pointer bound in mg", got)
		}
		if upper.Units != "mg" {
			t.Errorf("upper bound units after sharing = %q, want mg written through the pointer", upper.Units)
		}
	})
	t.Run("DV_ORDERED bounds of two classes: left alone", func(t *testing.T) {
		iv := &rm.DVInterval[rm.DVOrdered]{}
		iv.Lower, iv.Upper = rm.DVCount{Magnitude: 5}, rm.DVQuantity{Magnitude: 3, Units: "g"}
		shareComparable(nil, iv)
		if got := [2]string{describeBound(iv.Lower), describeBound(iv.Upper)}; got != [2]string{"DV_COUNT 5", "DV_QUANTITY 3 g"} {
			t.Errorf("shareComparable = %q, want the pair left alone", got)
		}
	})
}

// propertyQuantityBound is a DV_QUANTITY bound constrained by one
// C_DV_QUANTITY that names the openehr property code and lists no entry.
func propertyQuantityBound(code string) string {
	return `<children xsi:type="C_DV_QUANTITY"><rm_type_name>DV_QUANTITY</rm_type_name>` +
		boundOccurrences + `<node_id></node_id><property><terminology_id><value>openehr</value></terminology_id>` +
		`<code_string>` + code + `</code_string></property></children>`
}

// listedQuantityBound is a DV_QUANTITY bound constrained by one
// C_DV_QUANTITY that lists entries, each a QuantityEntry.
func listedQuantityBound(entries string) string {
	return `<children xsi:type="C_DV_QUANTITY"><rm_type_name>DV_QUANTITY</rm_type_name>` +
		boundOccurrences + `<node_id></node_id>` + entries + `</children>`
}

// TestREQ107_ShareComparableProportion pins the REQ-107 Intervals rule that
// gives two DV_PROPORTION bounds one type both sides admit, before they are
// ordered. Of the lower bound's type, the upper bound's and every kind in
// code order, it is the first that both admit and in which an ordered pair
// fits both sides, and where no such type has one, the first that both
// admit, a side admitting a kind where its bound can be settled as that kind
// inside its constraints, and is then so settled: two bounds of one type
// take another only for an ordered pair. A bound that carries a normal_range
// keeps its type. Where the two sides admit no one type, the pair is left as
// it is.
func TestREQ107_ShareComparableProportion(t *testing.T) {
	opt := ReadVendoredOPT(t, CountIntervalOPT)
	kinds := func(lower, upper string) *tcimpl.CompiledNode {
		t.Helper()
		numerators := RealRange(Closed(0), Closed(100))
		return intervalNode(t, RetargetInterval(t, opt, "DV_PROPORTION",
			ProportionKindBound(numerators, lower, ""), ProportionKindBound(numerators, upper, "")))
	}
	p := func(num, den float64, kind int32) rm.DVProportion {
		return rm.DVProportion{Numerator: rm.Real(num), Denominator: rm.Real(den), Type: rm.Integer(kind)}
	}
	// whole gives each side the numerators lower and upper allow, the
	// denominator 1 and the types fraction and ratio, the lower side's listed
	// first: as fractions, which need whole numerators, the lower side starts
	// at 6 and the upper one ends at 5, so the two have an ordered pair only
	// as ratios, and only where the upper numerators reach the lowest lower
	// one.
	whole := func(lower, upper string) *tcimpl.CompiledNode {
		t.Helper()
		one := RealList(1)
		return intervalNode(t, RetargetInterval(t, opt, "DV_PROPORTION",
			ProportionKindBoundOver(lower, one, IntList(3, 0), ""), ProportionKindBoundOver(upper, one, IntList(0, 3), "")))
	}
	ordered := whole(RealRange(Closed(5.2), Closed(6)), RealRange(Closed(5), Closed(5.5)))
	unordered := whole(RealRange(Closed(6), Closed(7)), RealRange(Closed(5), Closed(5.5)))
	ranged := &rm.DVInterval[rm.DVProportion]{}
	cases := []struct {
		name         string
		node         *tcimpl.CompiledNode
		upperRanged  bool
		bounds, want [2]rm.DVProportion
	}{
		{
			name:   "the lower bound's type, which the upper side admits",
			node:   kinds(IntList(0, 2), IntList(2, 0)),
			bounds: [2]rm.DVProportion{p(1, 4, 0), p(50, 100, 2)}, want: [2]rm.DVProportion{p(1, 4, 0), p(50, 100, 0)},
		},
		{
			// A percent takes the denominator 100.
			name:   "else the upper bound's type, which the lower side admits",
			node:   kinds(IntList(0, 2), IntList(2, 3)),
			bounds: [2]rm.DVProportion{p(1, 4, 0), p(50, 100, 2)}, want: [2]rm.DVProportion{p(1, 100, 2), p(50, 100, 2)},
		},
		{
			name:   "else the first kind in code order both admit",
			node:   kinds(IntList(0, 3), IntList(2, 3)),
			bounds: [2]rm.DVProportion{p(1, 4, 0), p(50, 100, 2)}, want: [2]rm.DVProportion{p(1, 4, 3), p(50, 100, 3)},
		},
		{
			name:   "the first type both admit in which an ordered pair fits",
			node:   ordered,
			bounds: [2]rm.DVProportion{p(6, 1, 3), p(5, 1, 0)}, want: [2]rm.DVProportion{p(6, 1, 0), p(5, 1, 0)},
		},
		{
			name:   "two bounds of one type with no ordered pair take another",
			node:   ordered,
			bounds: [2]rm.DVProportion{p(6, 1, 3), p(5, 1, 3)}, want: [2]rm.DVProportion{p(6, 1, 0), p(5, 1, 0)},
		},
		{
			name:   "no type both admit holds an ordered pair: the first both admit",
			node:   unordered,
			bounds: [2]rm.DVProportion{p(6, 1, 0), p(5, 1, 3)}, want: [2]rm.DVProportion{p(6, 1, 0), p(5, 1, 0)},
		},
		{
			name:   "two bounds of one type with no ordered pair anywhere: left alone",
			node:   unordered,
			bounds: [2]rm.DVProportion{p(6, 1, 3), p(5, 1, 3)}, want: [2]rm.DVProportion{p(6, 1, 3), p(5, 1, 3)},
		},
		{
			name:   "no type both sides admit: left alone",
			node:   kinds(IntList(0), IntList(1)),
			bounds: [2]rm.DVProportion{p(1, 4, 0), p(1, 1, 1)}, want: [2]rm.DVProportion{p(1, 4, 0), p(1, 1, 1)},
		},
		{
			// Without its normal_range, the upper bound would become a ratio;
			// the lower bound becomes a unitary proportion, of denominator 1.
			name:        "an upper bound with a normal_range keeps its type",
			upperRanged: true,
			bounds:      [2]rm.DVProportion{p(1, 4, 0), p(1, 1, 1)}, want: [2]rm.DVProportion{p(1, 1, 1), p(1, 1, 1)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			iv := &rm.DVInterval[rm.DVProportion]{}
			iv.Lower, iv.Upper = tc.bounds[0], tc.bounds[1]
			if tc.upperRanged {
				iv.Upper.NormalRange = ranged
			}
			shareComparable(tc.node, iv)
			if got, want := [2][3]float64{proportionParts(iv.Lower), proportionParts(iv.Upper)}, [2][3]float64{proportionParts(tc.want[0]), proportionParts(tc.want[1])}; got != want {
				t.Errorf("shareComparable(%v) = %v, want %v", tc.bounds, got, want)
			}
		})
	}
	t.Run("DV_ORDERED pointer bounds: settled in place", func(t *testing.T) {
		lower, upper := &rm.DVProportion{Numerator: 1, Denominator: 4}, &rm.DVProportion{Numerator: 1, Denominator: 2, Type: 1}
		iv := &rm.DVInterval[rm.DVOrdered]{}
		iv.Lower, iv.Upper = lower, upper
		shareComparable(nil, iv)
		if lower.Type != 0 || upper.Type != 0 || upper.Denominator != 2 {
			t.Errorf("pointer proportions after sharing = types %d and %d, upper denominator %v; want two ratios, the upper over 2", lower.Type, upper.Type, upper.Denominator)
		}
	})
}
