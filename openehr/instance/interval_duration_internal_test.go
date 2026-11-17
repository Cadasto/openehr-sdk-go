package instance

import (
	"fmt"
	"testing"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_DurationBoundsInOrder pins the REQ-107 Intervals rule on two
// DV_DURATION bounds: they compare by their length in seconds, as the RM
// floor orders them (REQ-112 DV_INTERVAL), not by their text, so P1Y lies
// above PT3H. An inverted pair is swapped where each side's C_DURATION, its
// pattern and its range, accepts the other bound. Where a side refuses the
// swap, each side takes the end of its range nearest the other side,
// written to its pattern and moved inside an excluded end, where that pair
// is in order, and the pair is left as generated where it is not. A pair
// holding a value that does not parse as a duration does not compare and is
// left alone, as is a pair with an open side.
func TestREQ107_DurationBoundsInOrder(t *testing.T) {
	opt := ReadVendoredOPT(t, CountIntervalOPT)
	node := func(lowerBody, upperBody string) *tcimpl.CompiledNode {
		t.Helper()
		bound := func(body string) string { return TemporalBound("DV_DURATION", "DURATION", "C_DURATION", body) }
		return intervalNode(t, RetargetInterval(t, opt, "DV_DURATION", bound(lowerBody), bound(upperBody)))
	}
	ranged := func(pattern, lower string, lowerIncluded bool, upper string, upperIncluded bool) string {
		body := fmt.Sprintf("<range><lower_included>%t</lower_included><upper_included>%t</upper_included>"+
			"<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>"+
			"<lower>%s</lower><upper>%s</upper></range>", lowerIncluded, upperIncluded, lower, upper)
		if pattern == "" {
			return body
		}
		return "<pattern>" + pattern + "</pattern>" + body
	}
	closed := func(pattern, lower, upper string) string { return ranged(pattern, lower, true, upper, true) }
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
			// The lower side's pattern names years alone, so it refuses P10D;
			// its range starts at P0Y, below the upper side's end P30D.
			name:  "a side's pattern refuses the swap: each side's range end",
			node:  node(closed("PY", "P0Y", "P10Y"), closed("PD", "P0D", "P30D")),
			lower: "P5Y", upper: "P10D",
			want: [2]string{"P0Y", "P30D"},
		},
		{
			// The lower side's range starts at P1Y, above the upper side's end.
			name:  "a side's pattern refuses the swap, no ordered pair: left as generated",
			node:  node(closed("PY", "P1Y", "P10Y"), closed("PD", "P0D", "P30D")),
			lower: "P5Y", upper: "P10D",
			want: [2]string{"P5Y", "P10D"},
		},
		{
			// The upper side's range stops at a month, so it refuses P1Y.
			name:  "a side's range refuses the swap: each side's range end",
			node:  node(closed("PYMWD", "P0D", "P1Y"), closed("PYMWD", "P0D", "P1M")),
			lower: "P1Y", upper: "P2D",
			want: [2]string{"P0D", "P1M"},
		},
		{
			// The lower side's range starts at P2M, above the upper side's end.
			name:  "a side's range refuses the swap, no ordered pair: left as generated",
			node:  node(closed("PYMWD", "P2M", "P1Y"), closed("PYMWD", "P0D", "P1M")),
			lower: "P1Y", upper: "P2D",
			want: [2]string{"P1Y", "P2D"},
		},
		{
			// P8Y is above the upper side's range, so the swap fails; P1Y and
			// P2Y, each side's end nearest the other, fit both sides.
			name:  "ranges that overlap: each side's range end",
			node:  node(ranged("", "P1Y", true, "P10Y", true), ranged("", "P0D", true, "P2Y", true)),
			lower: "P8Y", upper: "P2Y",
			want: [2]string{"P1Y", "P2Y"},
		},
		{
			// The lower side excludes P1Y, and its pattern has no time part:
			// the shortest count of one designator past P1Y is 366 days.
			name:  "an excluded end moved inside it, in the side's pattern",
			node:  node(ranged("PYMWD", "P1Y", false, "P10Y", true), ranged("PD", "P0D", true, "P400D", true)),
			lower: "P8Y", upper: "P2D",
			want: [2]string{"P366D", "P400D"},
		},
		{
			// PT0S is the lower side's end, which its pattern refuses: the
			// first designator it admits writes the length 0.
			name:  "a range end written in a designator the pattern refuses",
			node:  node(ranged("PYMWD", "PT0S", true, "P10Y", true), ranged("PD", "P0D", true, "P5D", true)),
			lower: "P8Y", upper: "P2D",
			want: [2]string{"P0Y", "P5D"},
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
