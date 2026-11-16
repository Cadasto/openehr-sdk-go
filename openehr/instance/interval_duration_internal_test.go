package instance

import (
	"testing"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_DurationBoundsInOrder pins the REQ-107 Intervals rule on two
// DV_DURATION bounds: they compare by their length in seconds, as the RM
// floor orders them (REQ-112 DV_INTERVAL), not by their text, so P1Y lies
// above PT3H. An inverted pair is swapped where each side's C_DURATION, its
// pattern and its range, accepts the other bound, and is left as generated
// where a side refuses it. A pair holding a value that does not parse as a
// duration does not compare and is left alone, as is a pair with an open
// side.
func TestREQ107_DurationBoundsInOrder(t *testing.T) {
	opt := ReadVendoredOPT(t, CountIntervalOPT)
	node := func(lowerBody, upperBody string) *tcimpl.CompiledNode {
		t.Helper()
		bound := func(body string) string { return TemporalBound("DV_DURATION", "DURATION", "C_DURATION", body) }
		return intervalNode(t, RetargetInterval(t, opt, "DV_DURATION", bound(lowerBody), bound(upperBody)))
	}
	closed := func(pattern, lower, upper string) string {
		return "<pattern>" + pattern + "</pattern><range><lower_included>true</lower_included><upper_included>true</upper_included>" +
			"<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>" +
			"<lower>" + lower + "</lower><upper>" + upper + "</upper></range>"
	}
	cases := []struct {
		name         string
		node         *tcimpl.CompiledNode
		lower, upper string
		lowerOpen    bool
		want         [2]string
	}{
		{
			// The text P1Y sorts before PT3H; the magnitudes do not.
			name:  "a year above three hours: swapped",
			lower: "P1Y", upper: "PT3H",
			want: [2]string{"PT3H", "P1Y"},
		},
		{
			// The text PT3H sorts after P1D; the magnitudes are in order.
			name:  "three hours below a day: left alone",
			lower: "PT3H", upper: "P1D",
			want: [2]string{"PT3H", "P1D"},
		},
		{
			name:  "eight days above a week: swapped",
			lower: "P8D", upper: "P1W",
			want: [2]string{"P1W", "P8D"},
		},
		{
			name:  "a week and seven days are equal: left alone",
			lower: "P1W", upper: "P7D",
			want: [2]string{"P1W", "P7D"},
		},
		{
			name:  "a value the parse refuses: left alone",
			lower: "P1X", upper: "P0D",
			want: [2]string{"P1X", "P0D"},
		},
		{
			name:  "open side: left alone",
			lower: "P1Y", upper: "PT3H", lowerOpen: true,
			want: [2]string{"P1Y", "PT3H"},
		},
		{
			name:  "both sides accept the swap: swapped",
			node:  node(closed("PYMWD", "P0D", "P1Y"), closed("PYMWD", "P0D", "P10Y")),
			lower: "P1Y", upper: "P2M",
			want: [2]string{"P2M", "P1Y"},
		},
		{
			// The lower side's pattern names years alone, so it refuses P10D.
			name:  "a side refuses the swap: left as generated",
			node:  node(closed("PY", "P0Y", "P10Y"), closed("PD", "P0D", "P30D")),
			lower: "P5Y", upper: "P10D",
			want: [2]string{"P5Y", "P10D"},
		},
		{
			// The upper side's range stops at a month, so it refuses P1Y.
			name:  "a side's range refuses the swap: left as generated",
			node:  node(closed("PYMWD", "P0D", "P1Y"), closed("PYMWD", "P0D", "P1M")),
			lower: "P1Y", upper: "P2D",
			want: [2]string{"P1Y", "P2D"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			iv := &rm.DVInterval[rm.DVDuration]{}
			iv.Lower.Value, iv.Upper.Value = tc.lower, tc.upper
			iv.LowerUnbounded = tc.lowerOpen
			orderIntervalBounds(tc.node, iv)
			if got := [2]string{iv.Lower.Value, iv.Upper.Value}; got != tc.want {
				t.Errorf("orderIntervalBounds(%q, %q) = %q, want %q", tc.lower, tc.upper, got, tc.want)
			}
		})
	}

	t.Run("DV_ORDERED pointer durations: swapped in place", func(t *testing.T) {
		lower, upper := &rm.DVDuration{Value: "P1Y"}, &rm.DVDuration{Value: "PT3H"}
		iv := &rm.DVInterval[rm.DVOrdered]{}
		iv.Lower, iv.Upper = lower, upper
		orderIntervalBounds(nil, iv)
		if got, want := [2]string{lower.Value, upper.Value}, [2]string{"PT3H", "P1Y"}; got != want {
			t.Errorf("pointer durations after ordering = %q, want %q", got, want)
		}
	})
	t.Run("DV_ORDERED value durations: swapped", func(t *testing.T) {
		iv := &rm.DVInterval[rm.DVOrdered]{}
		iv.Lower, iv.Upper = rm.DVDuration{Value: "P8D"}, rm.DVDuration{Value: "P1W"}
		orderIntervalBounds(nil, iv)
		lower, lowerOK := iv.Lower.(rm.DVDuration)
		upper, upperOK := iv.Upper.(rm.DVDuration)
		if !lowerOK || !upperOK {
			t.Fatalf("bounds after ordering are %T and %T, want rm.DVDuration values", iv.Lower, iv.Upper)
		}
		if got, want := [2]string{lower.Value, upper.Value}, [2]string{"P1W", "P8D"}; got != want {
			t.Errorf("value durations after ordering = %q, want %q", got, want)
		}
	})
}
