package constraints_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// REQ-103: with no list entry to offer, CReal.ExampleValue and the
// DvQuantity example magnitude pick the same number from a range: the
// lower bound, moved inside when it is exclusive, else the upper bound,
// moved inside when it is exclusive, else zero.
func TestCReal_ExampleValueFromRange(t *testing.T) {
	cases := []struct {
		name string
		r    constraints.NumericRange
		list []float64
		want float64
	}{
		{name: "zero range", want: 0},
		{name: "both sides unbounded", r: constraints.NumericRange{Lower: 2, Upper: 10, LowerUnbounded: true, UpperUnbounded: true}, want: 0},
		{name: "closed", r: constraints.NumericRange{Lower: 2, Upper: 10, LowerInclusive: true, UpperInclusive: true}, want: 2},
		{name: "open", r: constraints.NumericRange{Lower: 2, Upper: 10}, want: 6},
		{name: "exclusive lower, no upper", r: constraints.NumericRange{Lower: 2, UpperUnbounded: true}, want: 3},
		{name: "inclusive lower, no upper", r: constraints.NumericRange{Lower: 2, LowerInclusive: true, UpperUnbounded: true}, want: 2},
		{name: "no lower, inclusive upper", r: constraints.NumericRange{Upper: 10, LowerUnbounded: true, UpperInclusive: true}, want: 10},
		{name: "no lower, exclusive upper", r: constraints.NumericRange{Upper: 10, LowerUnbounded: true}, want: 9},
		{
			name: "every list entry outside the range",
			r:    constraints.NumericRange{Lower: 2, Upper: 10, LowerInclusive: true, UpperInclusive: true},
			list: []float64{0, 11},
			want: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := (constraints.CReal{Range: tc.r, List: tc.list}).ExampleValue(); got != tc.want {
				t.Errorf("CReal{Range: %s, List: %v}.ExampleValue() = %v, want %v", tc.r, tc.list, got, tc.want)
			}
			q := constraints.DvQuantity{Units: []constraints.QuantityUnit{{Units: "1", Magnitude: tc.r}}}
			if got := q.ExampleValue().(constraints.QuantityValue).Magnitude; got != tc.want {
				t.Errorf("DvQuantity with magnitude %s: ExampleValue().Magnitude = %v, want %v", tc.r, got, tc.want)
			}
		})
	}
}
