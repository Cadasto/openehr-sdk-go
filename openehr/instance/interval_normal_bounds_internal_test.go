package instance

import (
	"testing"
)

// TestREQ107_OrderPairNeedsEachSidesNormalRangeRule pins how orderPair takes
// the rule a side's bound keeps (orderSide.narrow) into the pair it returns:
// a value a side's rule narrows is the value the pair gives that side, and a
// side with no value that keeps its rule is part of no pair that fits, so
// the pair is left as generated (§ Contract Intervals, § Precedence 3). A
// DV_COUNT's count scale orders the pairs, with no OPT node.
func TestREQ107_OrderPairNeedsEachSidesNormalRangeRule(t *testing.T) {
	atLeast := func(n int64) func(int64) (int64, bool) {
		return func(k int64) (int64, bool) { return max(k, n), true }
	}
	atMost := func(n int64) func(int64) (int64, bool) {
		return func(k int64) (int64, bool) { return min(k, n), true }
	}
	never := func(int64) (int64, bool) { return 0, false }
	cases := []struct {
		name         string
		lower, upper func(int64) (int64, bool)
		want         [2]int64
		wantOrdered  bool
	}{
		{name: "no rule on either side, swapped", want: [2]int64{2, 9}, wantOrdered: true},
		{name: "the lower side narrows the swapped value", lower: atLeast(5), want: [2]int64{5, 9}, wantOrdered: true},
		{name: "the upper side narrows the swapped value", upper: atMost(3), want: [2]int64{2, 3}, wantOrdered: true},
		{name: "the lower side has no value that keeps its rule", lower: never},
		{name: "the upper side has no value that keeps its rule", upper: never},
		{name: "the narrowed values are out of order", lower: atLeast(5), upper: atMost(3)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lo, hi, ok := orderPair(9, 2, orderSide[int64]{narrow: tc.lower}, orderSide[int64]{narrow: tc.upper}, countScale)
			if ok != tc.wantOrdered {
				t.Fatalf("orderPair(9, 2) ordered = %t (%d, %d), want %t", ok, lo, hi, tc.wantOrdered)
			}
			if ok && [2]int64{lo, hi} != tc.want {
				t.Errorf("orderPair(9, 2) = (%d, %d), want (%d, %d)", lo, hi, tc.want[0], tc.want[1])
			}
		})
	}
}
