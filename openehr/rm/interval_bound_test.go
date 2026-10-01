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
		{"interface-typed: nil", IsEmptyIntervalBound[DVOrdered](nil), true},
		{"interface-typed: typed nil", IsEmptyIntervalBound[DVOrdered](nilQuantity), true},
		{"interface-typed: all-zero value", IsEmptyIntervalBound[DVOrdered](DVQuantity{}), false},
		{"interface-typed: pointer to an all-zero value", IsEmptyIntervalBound[DVOrdered](&DVQuantity{}), false},
		{"interface-typed: real bound", IsEmptyIntervalBound[DVOrdered](&DVCount{Magnitude: 3}), false},
		{"any-typed: nil", IsEmptyIntervalBound[any](nil), true},
		{"any-typed: zero Integer", IsEmptyIntervalBound[any](Integer(0)), false},
		{"DV_QUANTITY: zero", IsEmptyIntervalBound(DVQuantity{}), true},
		{"DV_QUANTITY: units only", IsEmptyIntervalBound(DVQuantity{Units: "mm"}), false},
		{"DV_COUNT: zero", IsEmptyIntervalBound(DVCount{}), true},
		{"DV_COUNT: magnitude only", IsEmptyIntervalBound(DVCount{Magnitude: 1}), false},
		{"DV_DATE: zero", IsEmptyIntervalBound(DVDate{}), true},
		{"DV_DATE: value only", IsEmptyIntervalBound(DVDate{Value: "2026-10-01"}), false},
		{"DV_ORDINAL: zero", IsEmptyIntervalBound(DVOrdinal{}), true},
		{"DV_ORDINAL: nested terminology id only", IsEmptyIntervalBound(coded), false},
		{"pointer-typed: nil", IsEmptyIntervalBound(nilQuantity), true},
		{"pointer-typed: pointer to an all-zero value", IsEmptyIntervalBound(&DVQuantity{}), false},
		{"Integer: zero", IsEmptyIntervalBound(Integer(0)), true},
		{"Integer: one", IsEmptyIntervalBound(Integer(1)), false},
		{"Real: zero", IsEmptyIntervalBound(Real(0)), true},
		{"Character: empty", IsEmptyIntervalBound(Character("")), true},
		{"Character: one", IsEmptyIntervalBound(Character("=")), false},
		{"string: empty", IsEmptyIntervalBound(""), true},
		{"bool: false", IsEmptyIntervalBound(false), true},
		{"int: zero", IsEmptyIntervalBound(0), true},
		{"int: one", IsEmptyIntervalBound(1), false},
		{"int8: zero", IsEmptyIntervalBound(int8(0)), true},
		{"int16: zero", IsEmptyIntervalBound(int16(0)), true},
		{"int32: zero", IsEmptyIntervalBound(int32(0)), true},
		{"int64: zero", IsEmptyIntervalBound(int64(0)), true},
		{"uint: zero", IsEmptyIntervalBound(uint(0)), true},
		{"uint8: zero", IsEmptyIntervalBound(uint8(0)), true},
		{"uint16: zero", IsEmptyIntervalBound(uint16(0)), true},
		{"uint32: zero", IsEmptyIntervalBound(uint32(0)), true},
		{"uint64: zero", IsEmptyIntervalBound(uint64(0)), true},
		{"uint64: one", IsEmptyIntervalBound(uint64(1)), false},
		{"uintptr: zero", IsEmptyIntervalBound(uintptr(0)), true},
		{"float32: zero", IsEmptyIntervalBound(float32(0)), true},
		{"float64: zero", IsEmptyIntervalBound(0.0), true},
		{"float64: one half", IsEmptyIntervalBound(0.5), false},
		{"complex64: zero", IsEmptyIntervalBound(complex64(0)), true},
		{"complex128: zero", IsEmptyIntervalBound(complex128(0)), true},
		{"complex128: i", IsEmptyIntervalBound(1i), false},
		// A type outside the rule is emitted as it stands, even at its zero
		// value: DV_TEXT is no interval class's bound type.
		{"outside the rule: zero DV_TEXT", IsEmptyIntervalBound(DVText{}), false},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("IsEmptyIntervalBound(%s) = %v, want %v", tc.name, tc.got, tc.want)
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
