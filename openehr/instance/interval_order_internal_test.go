package instance

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestOrderIntervalBoundsQuantityUnits pins the REQ-107 ordering pass on
// DV_QUANTITY bounds with no OPT constraint on either side: an inverted
// pair in the same units is swapped, and a pair in different units is left
// alone, because magnitudes in different units do not compare.
func TestOrderIntervalBoundsQuantityUnits(t *testing.T) {
	cases := []struct {
		name                 string
		lower, upper         rm.DVQuantity
		wantLower, wantUpper rm.DVQuantity
	}{
		{
			name:      "same units, inverted: swapped",
			lower:     rm.DVQuantity{Magnitude: 10, Units: "kg"},
			upper:     rm.DVQuantity{Magnitude: 5, Units: "kg"},
			wantLower: rm.DVQuantity{Magnitude: 5, Units: "kg"},
			wantUpper: rm.DVQuantity{Magnitude: 10, Units: "kg"},
		},
		{
			name:      "different units: left alone",
			lower:     rm.DVQuantity{Magnitude: 10, Units: "kg"},
			upper:     rm.DVQuantity{Magnitude: 5, Units: "g"},
			wantLower: rm.DVQuantity{Magnitude: 10, Units: "kg"},
			wantUpper: rm.DVQuantity{Magnitude: 5, Units: "g"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			iv := &rm.DVInterval[rm.DVQuantity]{}
			iv.Lower, iv.Upper = tc.lower, tc.upper
			orderIntervalBounds(nil, iv)
			if iv.Lower.Magnitude != tc.wantLower.Magnitude || iv.Lower.Units != tc.wantLower.Units ||
				iv.Upper.Magnitude != tc.wantUpper.Magnitude || iv.Upper.Units != tc.wantUpper.Units {
				t.Errorf("orderIntervalBounds: [%v %s, %v %s], want [%v %s, %v %s]",
					iv.Lower.Magnitude, iv.Lower.Units, iv.Upper.Magnitude, iv.Upper.Units,
					tc.wantLower.Magnitude, tc.wantLower.Units, tc.wantUpper.Magnitude, tc.wantUpper.Units)
			}
		})
	}
}
