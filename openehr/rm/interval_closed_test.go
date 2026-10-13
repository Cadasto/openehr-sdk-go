package rm_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ169ClosedIntervalConstructors pins the closed-interval constructors
// (REQ-169): each returns an interval with both bounds set, both included
// flags true and both unbounded flags false, the reading the Go zero value
// does not give.
func TestREQ169ClosedIntervalConstructors(t *testing.T) {
	t.Run("NewClosedInterval of int", func(t *testing.T) {
		got := rm.NewClosedInterval(1, 9)
		want := rm.Interval[int]{Lower: 1, LowerIncluded: true, Upper: 9, UpperIncluded: true}
		if got != want {
			t.Errorf("NewClosedInterval(1, 9) = %+v, want %+v", got, want)
		}
	})
	t.Run("NewClosedInterval of Integer", func(t *testing.T) {
		got := rm.NewClosedInterval[rm.Integer](0, 0)
		want := rm.Interval[rm.Integer]{LowerIncluded: true, UpperIncluded: true}
		if got != want {
			t.Errorf("NewClosedInterval[Integer](0, 0) = %+v, want %+v", got, want)
		}
	})
	t.Run("NewClosedDVInterval of DV_COUNT", func(t *testing.T) {
		lo, hi := rm.DVCount{Magnitude: 1}, rm.DVCount{Magnitude: 5}
		got := rm.NewClosedDVInterval(lo, hi)
		if got.Lower.Magnitude != 1 || got.Upper.Magnitude != 5 {
			t.Errorf("NewClosedDVInterval bounds = %+v, %+v, want magnitudes 1 and 5", got.Lower, got.Upper)
		}
		if !got.LowerIncluded || !got.UpperIncluded || got.LowerUnbounded || got.UpperUnbounded {
			t.Errorf("NewClosedDVInterval flags = included %v/%v, unbounded %v/%v, want true/true, false/false",
				got.LowerIncluded, got.UpperIncluded, got.LowerUnbounded, got.UpperUnbounded)
		}
	})
	t.Run("NewClosedDVInterval of DV_ORDERED", func(t *testing.T) {
		lo, hi := &rm.DVQuantity{Magnitude: 1, Units: "mm"}, &rm.DVQuantity{Magnitude: 5, Units: "mm"}
		got := rm.NewClosedDVInterval[rm.DVOrdered](lo, hi)
		if got.Lower != rm.DVOrdered(lo) || got.Upper != rm.DVOrdered(hi) {
			t.Errorf("NewClosedDVInterval[DVOrdered] bounds = %v, %v, want %v, %v", got.Lower, got.Upper, lo, hi)
		}
		if !got.LowerIncluded || !got.UpperIncluded || got.LowerUnbounded || got.UpperUnbounded {
			t.Errorf("NewClosedDVInterval[DVOrdered] flags = included %v/%v, unbounded %v/%v, want true/true, false/false",
				got.LowerIncluded, got.UpperIncluded, got.LowerUnbounded, got.UpperUnbounded)
		}
	})
}

func ExampleNewClosedDVInterval() {
	q := rm.DVQuantity{Magnitude: 120, Units: "mm[Hg]"}
	q.NormalRange = new(rm.NewClosedDVInterval(
		rm.DVQuantity{Magnitude: 90, Units: "mm[Hg]"},
		rm.DVQuantity{Magnitude: 140, Units: "mm[Hg]"},
	))
	r := q.NormalRange
	fmt.Println(r.Lower.Magnitude, r.LowerIncluded, r.Upper.Magnitude, r.UpperIncluded)
	// Output: 90 true 140 true
}
