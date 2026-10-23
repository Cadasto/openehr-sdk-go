package instance

import (
	"cmp"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_NormalStatusSteersPastCandidatesOnTheOtherSide pins that a high
// or low normal_status takes a candidate on its own side of the range
// wherever the OPT admits one there, even where the value's own constraint
// rejects the step past the bound and admits a candidate on the other side
// first in the fixed order (§ Precedence 3). The value is 5 inside [1..9],
// and its own constraint admits 5, -3 and 20 only: a high status takes 20,
// a low one -3, and a status that is neither keeps the fixed order, which
// reaches -3, the lowest value the constraint names, first.
func TestREQ107_NormalStatusSteersPastCandidatesOnTheOtherSide(t *testing.T) {
	admitted := []int64{5, -3, 20}
	cases := []struct {
		name   string
		status *rm.CodePhrase
		want   int64
	}{
		{name: "H", status: normalStatusCode("H"), want: 20},
		{name: "HH", status: normalStatusCode("HH"), want: 20},
		{name: "L", status: normalStatusCode("L"), want: -3},
		{name: "LLL", status: normalStatusCode("LLL"), want: -3},
		{name: "placeholder at0000", status: &rm.CodePhrase{CodeString: "at0000", TerminologyID: rm.TerminologyID{Value: "local"}}, want: -3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := int64(5)
			r := normalRule[int64]{
				bounds:  normalBounds[int64]{lower: 1, upper: 9, lowerIncluded: true, upperIncluded: true},
				value:   5,
				compare: func(a, b int64) (int, bool) { return cmp.Compare(a, b), true },
				steps:   integerSteps,
				own:     []int64{-3, 20, 5, -3, 20},
				fits:    func(k int64) bool { return slices.Contains(admitted, k) },
				set:     func(k int64) { got = k },
			}
			if !r.keep(tc.status, firstCandidate) {
				t.Fatalf("keep(%s) = false, want a candidate that keeps the rule", tc.status.CodeString)
			}
			if got != tc.want {
				t.Errorf("keep(%s) wrote %d, want %d", tc.status.CodeString, got, tc.want)
			}
		})
	}
}
