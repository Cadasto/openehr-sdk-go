package instance

import (
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// TestREQ107_NarrowToIntegerKeepsTheValuesAnIntegerHolds pins the narrowing of
// a C_INTEGER to the 32-bit range of an RM Integer (§ Precedence 3): a
// constraint that stays inside it is returned as it is, one that reaches past
// it keeps the values inside it, whichever side or bound it reaches past by,
// and one with no value inside it is refused.
func TestREQ107_NarrowToIntegerKeepsTheValuesAnIntegerHolds(t *testing.T) {
	const (
		past   = float64(5_000_000_000)
		top    = math.MaxInt32
		bottom = math.MinInt32
	)
	between := func(lo, hi float64) constraints.NumericRange {
		return constraints.NumericRange{Lower: lo, Upper: hi, LowerInclusive: true, UpperInclusive: true}
	}
	cases := []struct {
		name string
		in   constraints.CInteger
		// want is the narrowed bounds and list; nil list means no list.
		lo, hi int64
		list   []int64
		// same is set where the constraint comes back as it was.
		same bool
		// refused is set where no value of the constraint is an Integer.
		refused bool
	}{
		{name: "inside the Integer range", in: constraints.CInteger{Range: between(0, 50)}, same: true},
		{name: "unbounded", in: constraints.CInteger{}, same: true},
		{name: "one-sided, inside", in: constraints.CInteger{Range: constraints.NumericRange{Lower: 5, LowerInclusive: true, UpperUnbounded: true}}, same: true},
		{name: "a list inside", in: constraints.CInteger{List: []int64{1, 2}}, same: true},
		{name: "past the upper end", in: constraints.CInteger{Range: between(0, past)}, lo: 0, hi: top},
		{name: "past the lower end", in: constraints.CInteger{Range: between(-past, 7)}, lo: bottom, hi: 7},
		{name: "past both ends", in: constraints.CInteger{Range: between(-past, past)}, lo: bottom, hi: top},
		{
			name: "no upper bound, close to the upper end",
			in:   constraints.CInteger{Range: constraints.NumericRange{Lower: top - 20, LowerInclusive: true, UpperUnbounded: true}},
			lo:   top - 20, hi: top,
		},
		{
			name: "no lower bound, close to the lower end",
			in:   constraints.CInteger{Range: constraints.NumericRange{Upper: bottom + 20, UpperInclusive: true, LowerUnbounded: true}},
			lo:   bottom, hi: bottom + 20,
		},
		{
			name: "exclusive bounds",
			in:   constraints.CInteger{Range: constraints.NumericRange{Lower: top - 5, Upper: past}},
			lo:   top - 4, hi: top,
		},
		{name: "a list reaching past, whole unbounded", in: constraints.CInteger{List: []int64{7, 1 << 40, -1 << 40, 9}}, lo: bottom, hi: top, list: []int64{7, 9}},
		{name: "a list reaching past, in a range", in: constraints.CInteger{List: []int64{7, 1 << 40}, Range: between(0, past)}, lo: 0, hi: top, list: []int64{7}},
		{name: "above the Integer range", in: constraints.CInteger{Range: between(3_000_000_000, past)}, refused: true},
		{name: "below the Integer range", in: constraints.CInteger{Range: between(-past, -3_000_000_000)}, refused: true},
		{name: "exclusive at the upper end", in: constraints.CInteger{Range: constraints.NumericRange{Lower: top, Upper: past}}, refused: true},
		{name: "a list with no member inside", in: constraints.CInteger{List: []int64{1 << 40, -1 << 40}}, refused: true},
		{name: "a list with no member inside the range", in: constraints.CInteger{List: []int64{5, 1 << 40}, Range: between(10, past)}, refused: true},
	}
	for _, tc := range cases {
		got, err := narrowToInteger(tc.in)
		switch {
		case tc.refused:
			if !errors.Is(err, errNoIntegerValue) {
				t.Errorf("%s: narrowToInteger = %+v, %v, want errNoIntegerValue", tc.name, got, err)
			}
			continue
		case err != nil:
			t.Errorf("%s: narrowToInteger: %v, want none", tc.name, err)
			continue
		case tc.same:
			if !slices.Equal(got.List, tc.in.List) || got.Range != tc.in.Range {
				t.Errorf("%s: narrowToInteger = %+v, want the constraint as it was, %+v", tc.name, got, tc.in)
			}
			continue
		}
		want := constraints.NumericRange{Lower: float64(tc.lo), Upper: float64(tc.hi), LowerInclusive: true, UpperInclusive: true}
		if got.Range != want || !slices.Equal(got.List, tc.list) {
			t.Errorf("%s: narrowToInteger = range %v list %v, want range %v list %v", tc.name, got.Range, got.List, want, tc.list)
		}
		if v := got.ExampleValue(); len(got.Validate(v)) != 0 || !fitsInteger(v.(int64)) {
			t.Errorf("%s: the narrowed constraint's example value %v is not a value of it inside the Integer range", tc.name, v)
		}
	}
}

// TestREQ107_HoldsIntegerNamesThe32BitAttributes pins which attributes the
// narrowing applies to: those of the RM type Integer, not Integer64.
func TestREQ107_HoldsIntegerNamesThe32BitAttributes(t *testing.T) {
	cases := []struct {
		value any
		attr  string
		want  bool
	}{
		{&rm.DVQuantity{}, "precision", true},
		{&rm.DVMultimedia{}, "size", true},
		{&rm.DVProportion{}, "precision", true},
		{&rm.IntervalEvent[rm.ItemStructure]{}, "sample_count", true},
		{&rm.DVCount{}, "magnitude", false},
		{&rm.DVQuantity{}, "accuracy", false},
		{&rm.DVQuantity{}, "", false},
		{&rm.DVQuantity{}, "no_such_attribute", false},
		{struct{}{}, "precision", false},
	}
	for _, tc := range cases {
		if got := holdsInteger(tc.value, tc.attr); got != tc.want {
			t.Errorf("holdsInteger(%T, %q) = %t, want %t", tc.value, tc.attr, got, tc.want)
		}
	}
}
