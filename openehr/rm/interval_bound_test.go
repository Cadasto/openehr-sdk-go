package rm

import "testing"

// TestIsEmptyIntervalBound pins the emptiness test the canonical encoders
// apply to an open interval side's bound (REQ-052, REQ-056), in the reading
// REQ-112 gives the floor: a bound typed by an interface is empty only when
// it is nil or a typed-nil pointer, so an all-zero value behind the interface
// is a bound; a bound of a concrete type is empty when it is that type's zero
// value, down to a field of a nested struct.
func TestIsEmptyIntervalBound(t *testing.T) {
	var nilQuantity *DVQuantity
	coded := DVOrdinal{}
	coded.Symbol.DefiningCode.TerminologyID.Value = "local"

	cases := []struct {
		name string
		got  bool
		want bool
	}{
		{"interface-typed: nil", isEmptyIntervalBound[DVOrdered](nil), true},
		{"interface-typed: typed nil", isEmptyIntervalBound[DVOrdered](nilQuantity), true},
		{"interface-typed: all-zero value", isEmptyIntervalBound[DVOrdered](DVQuantity{}), false},
		{"interface-typed: pointer to an all-zero value", isEmptyIntervalBound[DVOrdered](&DVQuantity{}), false},
		{"interface-typed: real bound", isEmptyIntervalBound[DVOrdered](&DVCount{Magnitude: 3}), false},
		{"any-typed: nil", isEmptyIntervalBound[any](nil), true},
		{"any-typed: zero Integer", isEmptyIntervalBound[any](Integer(0)), false},
		{"DV_QUANTITY: zero", isEmptyIntervalBound(DVQuantity{}), true},
		{"DV_QUANTITY: units only", isEmptyIntervalBound(DVQuantity{Units: "mm"}), false},
		{"DV_COUNT: zero", isEmptyIntervalBound(DVCount{}), true},
		{"DV_COUNT: magnitude only", isEmptyIntervalBound(DVCount{Magnitude: 1}), false},
		{"DV_DATE: zero", isEmptyIntervalBound(DVDate{}), true},
		{"DV_DATE: value only", isEmptyIntervalBound(DVDate{Value: "2026-10-01"}), false},
		{"DV_ORDINAL: zero", isEmptyIntervalBound(DVOrdinal{}), true},
		{"DV_ORDINAL: nested terminology id only", isEmptyIntervalBound(coded), false},
		{"pointer-typed: nil", isEmptyIntervalBound(nilQuantity), true},
		{"pointer-typed: pointer to an all-zero value", isEmptyIntervalBound(&DVQuantity{}), false},
		{"Integer: zero", isEmptyIntervalBound(Integer(0)), true},
		{"Integer: one", isEmptyIntervalBound(Integer(1)), false},
		{"string: empty", isEmptyIntervalBound(""), true},
		{"not a bound type: zero DV_TEXT", isEmptyIntervalBound(DVText{}), false},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("isEmptyIntervalBound(%s) = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

// TestOmitIntervalBoundNeedsOpenSide pins that only an open side loses its
// empty bound (REQ-052): a zero DV_COUNT on a bounded side is a real bound.
func TestOmitIntervalBoundNeedsOpenSide(t *testing.T) {
	if omitIntervalBound(false, DVCount{}) {
		t.Error("omitIntervalBound(bounded, zero DV_COUNT) = true, want false: a bounded side keeps its bound")
	}
	if !omitIntervalBound(true, DVCount{}) {
		t.Error("omitIntervalBound(open, zero DV_COUNT) = false, want true")
	}
	if omitIntervalBound(true, DVCount{Magnitude: 2}) {
		t.Error("omitIntervalBound(open, DV_COUNT 2) = true, want false: a non-empty bound beside its open flag stays")
	}
}
