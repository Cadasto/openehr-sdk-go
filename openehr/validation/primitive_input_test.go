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
