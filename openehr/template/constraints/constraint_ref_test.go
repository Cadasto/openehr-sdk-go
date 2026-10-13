package constraints_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// REQ-103: CONSTRAINT_REF maps to ConstraintRef. The value set its Reference
// names is external terminology, so Validate reports no violation for a value
// of the RM type the node constrains, whatever its terminology or code.
func TestREQ103_ConstraintRef_ValidateAdmitsAnyCode(t *testing.T) {
	t.Parallel()
	c := constraints.ConstraintRef{Reference: "ac0001"}
	tests := []struct {
		name  string
		value any
	}{
		{name: "local code", value: constraints.CodedTermRef{Terminology: "local", CodeString: "at0000"}},
		{name: "external code", value: constraints.CodedTermRef{Terminology: "SNOMED-CT", CodeString: "22298006"}},
		{name: "empty coded term", value: constraints.CodedTermRef{}},
		{name: "bare code", value: "22298006"},
		{name: "empty bare code", value: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := c.Validate(tc.value); len(got) != 0 {
				t.Errorf("%#v.Validate(%#v) = %+v, want no violation", c, tc.value, got)
			}
		})
	}
}

// REQ-103: a value whose Go type is outside the set ConstraintRef.Validate
// accepts is a contract failure on the caller side: one CodeWrongType
// violation, as for every other primitive constraint.
func TestREQ103_ConstraintRef_ValidateRefusesOtherTypes(t *testing.T) {
	t.Parallel()
	c := constraints.ConstraintRef{Reference: "ac0001"}
	tests := []struct {
		name  string
		value any
	}{
		{name: "int", value: 42},
		{name: "bool", value: true},
		{name: "quantity", value: constraints.QuantityValue{Magnitude: 1, Units: "kg"}},
		{name: "pointer to coded term", value: &constraints.CodedTermRef{Terminology: "local", CodeString: "at0000"}},
		{name: "nil", value: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := c.Validate(tc.value)
			if len(got) != 1 || got[0].Code != constraints.CodeWrongType {
				t.Errorf("%#v.Validate(%#v) = %+v, want one %s violation", c, tc.value, got, constraints.CodeWrongType)
			}
		})
	}
}

// REQ-107: the generator's placeholder for a code the OPT lists no choice
// for is at0000 in local when the OPT names no terminology, which a
// CONSTRAINT_REF does not. ExampleValue gives that code, whatever the
// reference.
func TestREQ107_ConstraintRefPlaceholder(t *testing.T) {
	t.Parallel()
	want := constraints.CodedTermRef{Terminology: "local", CodeString: "at0000"}
	for _, c := range []constraints.ConstraintRef{{Reference: "ac0001"}, {}} {
		if got := c.ExampleValue(); got != want {
			t.Errorf("%#v.ExampleValue() = %#v, want %#v", c, got, want)
		}
	}
}
