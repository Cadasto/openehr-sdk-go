package constraints_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// REQ-103: a DV_QUANTITY constraint may list the same units in more
// than one entry. The value must match one of them on units and lie
// inside that entry's magnitude range, so any same-units entry that
// admits it is enough. When none does, the violations name the first
// same-units entry.
func TestDvQuantity_ValidateSameUnitsInSeveralEntries(t *testing.T) {
	closed := func(lo, hi float64) constraints.NumericRange {
		return constraints.NumericRange{Lower: lo, Upper: hi, LowerInclusive: true, UpperInclusive: true}
	}
	c := constraints.DvQuantity{Units: []constraints.QuantityUnit{
		{Units: "mm", Magnitude: closed(0, 10), Precision: closed(0, 0)},
		{Units: "kg", Magnitude: closed(0, 1000)},
		{Units: "mm", Magnitude: closed(100, 200)},
	}}
	// The refused rows below all carry magnitude 50 or precision 3.
	outsideFirstMagnitude := constraints.Violation{
		Code:   constraints.CodeOutOfRange,
		Detail: `magnitude outside [0..10] for units "mm"`,
		Value:  constraints.Redact(50.0),
	}
	outsideFirstPrecision := constraints.Violation{
		Code:   constraints.CodeOutOfRange,
		Detail: `precision outside [0..0] for units "mm"`,
		Value:  constraints.Redact(3),
	}
	cases := []struct {
		name  string
		value constraints.QuantityValue
		want  []constraints.Violation
	}{
		{name: "inside the first entry", value: constraints.QuantityValue{Magnitude: 5, Units: "mm", Precision: 0}},
		{name: "inside the second mm entry only", value: constraints.QuantityValue{Magnitude: 150, Units: "mm", Precision: -1}},
		{name: "second mm entry, precision only the first refuses", value: constraints.QuantityValue{Magnitude: 150, Units: "mm", Precision: 3}},
		{
			name:  "inside no mm entry",
			value: constraints.QuantityValue{Magnitude: 50, Units: "mm", Precision: -1},
			want:  []constraints.Violation{outsideFirstMagnitude},
		},
		{
			name:  "inside no mm entry, precision outside the first",
			value: constraints.QuantityValue{Magnitude: 50, Units: "mm", Precision: 3},
			want:  []constraints.Violation{outsideFirstMagnitude, outsideFirstPrecision},
		},
		{
			name:  "first entry's magnitude, its precision refused",
			value: constraints.QuantityValue{Magnitude: 5, Units: "mm", Precision: 3},
			want:  []constraints.Violation{outsideFirstPrecision},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.Validate(tc.value)
			if !slices.EqualFunc(got, tc.want, sameViolation) {
				t.Errorf("Validate(%v %s, precision %d) = %v, want %v",
					tc.value.Magnitude, tc.value.Units, tc.value.Precision, revealed(got), revealed(tc.want))
			}
		})
	}
}

func sameViolation(a, b constraints.Violation) bool {
	return a.Code == b.Code && a.Detail == b.Detail && a.Value.Equal(b.Value)
}

// revealed prints violations with their values shown, for a failure
// message in a test.
func revealed(vs []constraints.Violation) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = fmt.Sprintf("{%s %q %v}", v.Code, v.Detail, v.Value.Reveal())
	}
	return out
}
