package validation

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// TestPrimitiveInput_RealMagnitude pins REQ-110: a REAL constraint
// reached on a DV_QUANTITY.magnitude scalar channel receives a named
// rm.Real (rm.DVQuantity.Magnitude's type). primitiveInput must
// normalise it to a bare float64 so CReal.Validate accepts it —
// before the fix the named type fell through CReal's float32/float64
// type-switch and produced a spurious wrong_type violation.
func TestPrimitiveInput_RealMagnitude(t *testing.T) {
	got := primitiveInput(rm.Real(1.5))
	if _, ok := got.(float64); !ok {
		t.Fatalf("primitiveInput(rm.Real) = %T, want float64", got)
	}
	if vs := (constraints.CReal{}).Validate(got); len(vs) != 0 {
		t.Fatalf("CReal.Validate(%v) = %+v, want no violations", got, vs)
	}
}

// TestPrimitiveInput_IntegerMagnitude is the sibling INTEGER pin —
// rm.Integer normalises to int64 so CInteger.Validate accepts it.
func TestPrimitiveInput_IntegerMagnitude(t *testing.T) {
	got := primitiveInput(rm.Integer(7))
	if _, ok := got.(int64); !ok {
		t.Fatalf("primitiveInput(rm.Integer) = %T, want int64", got)
	}
	if vs := (constraints.CInteger{}).Validate(got); len(vs) != 0 {
		t.Fatalf("CInteger.Validate(%v) = %+v, want no violations", got, vs)
	}
}

// TestPrimitiveInput_Integer64Magnitude is the DV_COUNT.magnitude pin
// (REQ-110): rm.Integer64 normalises to int64 so CInteger.Validate accepts
// it, the constraints package being unable to name the RM type.
func TestPrimitiveInput_Integer64Magnitude(t *testing.T) {
	got := primitiveInput(rm.Integer64(7))
	if _, ok := got.(int64); !ok {
		t.Fatalf("primitiveInput(rm.Integer64) = %T, want int64", got)
	}
	if vs := (constraints.CInteger{}).Validate(got); len(vs) != 0 {
		t.Fatalf("CInteger.Validate(%v) = %+v, want no violations", got, vs)
	}
}

// REQ-102: an optional primitive attribute the RM holds by pointer reaches
// the typed primitive validator as the value it points to, normalised as
// that value is, so the constraint judges the value instead of rejecting the
// pointer as wrong_type.
func TestPrimitiveInput_REQ102_PointerHeldPrimitive(t *testing.T) {
	t.Parallel()
	flag, integer, number := true, rm.Integer(2), rm.Real(0.5)
	tests := []struct {
		in   any
		want any
		pc   constraints.PrimitiveConstraint
	}{
		{in: &flag, want: true, pc: constraints.CBoolean{TrueValid: true}},
		{in: &integer, want: int64(2), pc: constraints.CInteger{}},
		{in: &number, want: float64(0.5), pc: constraints.CReal{}},
	}
	for _, tc := range tests {
		got := primitiveInput(tc.in)
		if got != tc.want {
			t.Errorf("primitiveInput(%T) = %#v (%T), want %#v (%T)", tc.in, got, got, tc.want, tc.want)
		}
		if vs := tc.pc.Validate(got); len(vs) != 0 {
			t.Errorf("%T.Validate(primitiveInput(%T)) = %+v, want no violations", tc.pc, tc.in, vs)
		}
	}
}

// REQ-102: a pointer-held primitive that is not set is an absent attribute.
// The walk never hands one to the typed primitive validator, because the
// generated access reports it as absent. Handed one, primitiveInput reads it
// as [rmbind.PrimitiveValue] does: an optional Boolean, Integer or Real is no
// value, which the validator refuses as wrong_type, and never the zero value
// it could be mistaken for. An optional String keeps the older rule, the
// empty string.
func TestPrimitiveInput_REQ102_NilPointerHeldPrimitive(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   any
		want any
	}{
		{in: (*bool)(nil)},
		{in: (*rm.Integer)(nil)},
		{in: (*rm.Real)(nil)},
		{in: (*string)(nil), want: ""},
	}
	for _, tc := range tests {
		if got := primitiveInput(tc.in); got != tc.want {
			t.Errorf("primitiveInput(%T nil) = %#v (%T), want %#v (%T)", tc.in, got, got, tc.want, tc.want)
		}
	}
}
