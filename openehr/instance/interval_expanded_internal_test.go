package instance

import (
	"fmt"
	"testing"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// expandedQuantityNode compiles a DV_INTERVAL<DV_QUANTITY> whose lower
// bound is a C_COMPLEX_OBJECT with a C_REAL on its magnitude and a C_STRING
// on its units, and returns the lower bound's node.
func expandedQuantityNode(t *testing.T, realBody, units string) *tcimpl.CompiledNode {
	t.Helper()
	bound := ExpandedQuantityBound(realBody, units)
	node := intervalNode(t, RetargetInterval(t, ReadVendoredOPT(t, CountIntervalOPT), "DV_QUANTITY", bound, bound))
	lower, _ := boundNodes(node)
	return lower
}

// expandedOrdinalNode is expandedQuantityNode for a DV_ORDINAL bound with a
// C_INTEGER on its value and the local codes its symbol admits.
func expandedOrdinalNode(t *testing.T, integerBody string, codes ...string) *tcimpl.CompiledNode {
	t.Helper()
	bound := ExpandedOrdinalBound(integerBody, codes...)
	node := intervalNode(t, RetargetInterval(t, ReadVendoredOPT(t, CountIntervalOPT), "DV_ORDINAL", bound, bound))
	lower, _ := boundNodes(node)
	return lower
}

// localCoded is a symbol with a local code and no text of its own.
func localCoded(code string) rm.DVCodedText {
	return rm.DVCodedText{
		Value:        code,
		DefiningCode: rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: "local"}},
	}
}

// TestREQ107_QuantityBoundBuiltPerAttributeFitsItsConstraint pins the
// REQ-107 reading of a DV_QUANTITY bound the OPT builds as a
// C_COMPLEX_OBJECT: a pair fits the side only where the C_REAL on its
// magnitude and the C_STRING on its units accept it, and the side's
// extremes are the ends of that C_REAL, in the one unit its C_STRING
// admits.
func TestREQ107_QuantityBoundBuiltPerAttributeFitsItsConstraint(t *testing.T) {
	node := expandedQuantityNode(t, RealRange(Closed(5), Open(10)), "mm")
	t.Run("fits", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			key  quantityKey
			want bool
		}{
			{"inside the range, in the unit", quantityKey{magnitude: 7, units: "mm"}, true},
			{"at the closed end", quantityKey{magnitude: 5, units: "mm"}, true},
			{"below the range", quantityKey{magnitude: 4, units: "mm"}, false},
			{"at the open end", quantityKey{magnitude: 10, units: "mm"}, false},
			{"in another unit", quantityKey{magnitude: 7, units: "cm"}, false},
		} {
			if got := quantityScale.fits(node, tc.key); got != tc.want {
				t.Errorf("%s: fits(%+v) = %t, want %t", tc.name, tc.key, got, tc.want)
			}
		}
	})
	t.Run("ends", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			units string
			low   bool
			want  quantityKey
			ok    bool
		}{
			{"lowest", "mm", true, quantityKey{magnitude: 5, units: "mm"}, true},
			{"highest, the float64 below the open end", "mm", false, quantityKey{magnitude: 9.999999999999998, units: "mm"}, true},
			{"lowest in another unit", "cm", true, quantityKey{}, false},
		} {
			got, ok := magnitudeEnd(node, tc.units, tc.low)
			if got != tc.want || ok != tc.ok {
				t.Errorf("%s: magnitudeEnd(%q, low=%t) = %+v, %t, want %+v, %t", tc.name, tc.units, tc.low, got, ok, tc.want, tc.ok)
			}
		}
	})
	t.Run("no end", func(t *testing.T) {
		open := expandedQuantityNode(t, RealRange(NoEnd, Closed(10)), "mm")
		if got, ok := magnitudeEnd(open, "mm", true); ok {
			t.Errorf("magnitudeEnd of a range with no lower end = %+v, true, want false", got)
		}
	})
}

// TestREQ107_OrdinalBoundBuiltPerAttributeFitsItsConstraint pins the
// REQ-107 reading of a DV_ORDINAL bound the OPT builds as a
// C_COMPLEX_OBJECT: a pair fits the side only where the C_INTEGER on its
// value, the 32 bits of an RM Integer and the C_CODE_PHRASE on its symbol
// accept it, and the side's extremes are the ends of that C_INTEGER with
// the side's own symbol.
func TestREQ107_OrdinalBoundBuiltPerAttributeFitsItsConstraint(t *testing.T) {
	node := expandedOrdinalNode(t, IntRange(Closed(5), Closed(3_000_000_000)), "at0001", "at0002")
	t.Run("fits", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			key  ordinalKey
			want bool
		}{
			{"value and symbol admitted", ordinalKey{value: 7, symbol: localCoded("at0002")}, true},
			{"value below the range", ordinalKey{value: 4, symbol: localCoded("at0001")}, false},
			{"symbol code not listed", ordinalKey{value: 7, symbol: localCoded("at0003")}, false},
			{"value above an RM Integer", ordinalKey{value: 2_999_999_999, symbol: localCoded("at0001")}, false},
		} {
			if got := ordinalFits(node, tc.key); got != tc.want {
				t.Errorf("%s: ordinalFits(%d, %s) = %t, want %t", tc.name, tc.key.value, tc.key.symbol.DefiningCode.CodeString, got, tc.want)
			}
		}
	})
	t.Run("ends keep the side's own symbol", func(t *testing.T) {
		like := ordinalKey{value: 6, symbol: localCoded("at0002")}
		got, ok := ordinalEnd(node, like, true)
		if want := (ordinalKey{value: 5, symbol: like.symbol}); describeOrdinalKey(got) != describeOrdinalKey(want) || !ok {
			t.Errorf("ordinalEnd(low) = %s, %t, want %s, true", describeOrdinalKey(got), ok, describeOrdinalKey(want))
		}
		if got, ok := ordinalEnd(expandedOrdinalNode(t, intUpToBody(20), "at0001"), like, true); ok {
			t.Errorf("ordinalEnd(low) of a range with no lower end = %s, true, want false", describeOrdinalKey(got))
		}
	})
	t.Run("a side with no end takes the other value and keeps its symbol", func(t *testing.T) {
		like := ordinalKey{value: 20, symbol: localCoded("at0001")}
		other := ordinalKey{value: 3, symbol: localCoded("at0002")}
		open := expandedOrdinalNode(t, intUpToBody(20), "at0001")
		got, ok := ordinalAdopt(open, like, other, true)
		if want := (ordinalKey{value: 3, symbol: like.symbol}); describeOrdinalKey(got) != describeOrdinalKey(want) || !ok {
			t.Errorf("ordinalAdopt = %s, %t, want %s, true", describeOrdinalKey(got), ok, describeOrdinalKey(want))
		}
		if got, _ := ordinalAdopt(nil, like, other, true); describeOrdinalKey(got) != describeOrdinalKey(other) {
			t.Errorf("ordinalAdopt of no node = %s, want the other bound %s", describeOrdinalKey(got), describeOrdinalKey(other))
		}
	})
}

// describeOrdinalKey names an ordinal bound's value and symbol code, since
// a symbol does not compare with ==.
func describeOrdinalKey(k ordinalKey) string {
	return fmt.Sprintf("%d %s", k.value, k.symbol.DefiningCode.CodeString)
}

// intUpToBody is a C_INTEGER body admitting every value up to hi.
func intUpToBody(hi float64) string { return IntRange(NoEnd, Closed(hi)) }
