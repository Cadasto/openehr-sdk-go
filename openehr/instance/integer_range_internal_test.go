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
		{
			// A side c leaves open is closed no further than the sampler's
			// window of 100 from the side c bounds, not at the limit.
			name: "no upper bound, past the lower end",
			in:   constraints.CInteger{Range: constraints.NumericRange{Lower: -past, LowerInclusive: true, UpperUnbounded: true}},
			lo:   bottom, hi: bottom + 100,
		},
		{
			name: "no lower bound, past the upper end",
			in:   constraints.CInteger{Range: constraints.NumericRange{Upper: past, UpperInclusive: true, LowerUnbounded: true}},
			lo:   top - 100, hi: top,
		},
		{
			// A list is what a draw takes from, so its members inside the
			// limits all stay, however far from the bound they lie.
			name: "no upper bound, a list reaching past",
			in: constraints.CInteger{
				List:  []int64{7, 50_000_000, 1 << 40},
				Range: constraints.NumericRange{Lower: 0, LowerInclusive: true, UpperUnbounded: true},
			},
			lo: 0, hi: top, list: []int64{7, 50_000_000},
		},
		{
			name: "no lower bound, a list reaching past",
			in: constraints.CInteger{
				List:  []int64{-50_000_000, 7, 1 << 40},
				Range: constraints.NumericRange{Upper: 100, UpperInclusive: true, LowerUnbounded: true},
			},
			lo: bottom, hi: 100, list: []int64{-50_000_000, 7},
		},
		{name: "a list reaching past, whole unbounded", in: constraints.CInteger{List: []int64{7, 1 << 40, -1 << 40, 9}}, lo: bottom, hi: top, list: []int64{7, 9}},
		{name: "a list reaching past, in a range", in: constraints.CInteger{List: []int64{7, 1 << 40}, Range: between(0, past)}, lo: 0, hi: top, list: []int64{7}},
		{name: "above the Integer range", in: constraints.CInteger{Range: between(3_000_000_000, past)}, refused: true},
		{name: "below the Integer range", in: constraints.CInteger{Range: between(-past, -3_000_000_000)}, refused: true},
		{name: "exclusive at the upper end", in: constraints.CInteger{Range: constraints.NumericRange{Lower: top, Upper: past}}, refused: true},
		{
			name: "exclusive at the lower end",
			in:   constraints.CInteger{Range: constraints.NumericRange{Lower: bottom, Upper: past, UpperInclusive: true}},
			lo:   bottom + 1, hi: top,
		},
		{name: "a list with no member inside", in: constraints.CInteger{List: []int64{1 << 40, -1 << 40}}, refused: true},
		{name: "a list with no member inside the range", in: constraints.CInteger{List: []int64{5, 1 << 40}, Range: between(10, past)}, refused: true},
	}
	for _, tc := range cases {
		got, err := narrowInteger(tc.in, integerLimits)
		switch {
		case tc.refused:
			if !errors.Is(err, errNoIntegerValue) {
				t.Errorf("%s: narrowInteger = %+v, %v, want errNoIntegerValue", tc.name, got, err)
			}
			continue
		case err != nil:
			t.Errorf("%s: narrowInteger: %v, want none", tc.name, err)
			continue
		case tc.same:
			if !slices.Equal(got.List, tc.in.List) || got.Range != tc.in.Range {
				t.Errorf("%s: narrowInteger = %+v, want the constraint as it was, %+v", tc.name, got, tc.in)
			}
			continue
		}
		want := constraints.NumericRange{Lower: float64(tc.lo), Upper: float64(tc.hi), LowerInclusive: true, UpperInclusive: true}
		if got.Range != want || !slices.Equal(got.List, tc.list) {
			t.Errorf("%s: narrowInteger = range %v list %v, want range %v list %v", tc.name, got.Range, got.List, want, tc.list)
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

// TestREQ107_NarrowIntegerKeepsAPrecisionFromMinusOne pins the narrowing of
// a C_INTEGER on the precision of a DV_QUANTITY or a DV_PROPORTION
// (§ Precedence 3): the RM admits no precision below -1, which means no
// limit, so a constraint that reaches below it keeps the values from -1 up,
// one that excludes -1 itself keeps the values above it, and one with no
// value from -1 up is refused.
func TestREQ107_NarrowIntegerKeepsAPrecisionFromMinusOne(t *testing.T) {
	between := func(lo, hi float64) constraints.NumericRange {
		return constraints.NumericRange{Lower: lo, Upper: hi, LowerInclusive: true, UpperInclusive: true}
	}
	cases := []struct {
		name    string
		in      constraints.CInteger
		lo, hi  int64
		list    []int64
		same    bool
		refused bool
	}{
		{name: "from 0 to 3", in: constraints.CInteger{Range: between(0, 3)}, same: true},
		{name: "from -1 to 3", in: constraints.CInteger{Range: between(-1, 3)}, same: true},
		{name: "from -10 to 10", in: constraints.CInteger{Range: between(-10, 10)}, lo: -1, hi: 10},
		{name: "at least -5", in: constraints.CInteger{Range: constraints.NumericRange{Lower: -5, LowerInclusive: true, UpperUnbounded: true}}, lo: -1, hi: 99},
		{name: "above -5", in: constraints.CInteger{Range: constraints.NumericRange{Lower: -5, UpperUnbounded: true}}, lo: -1, hi: 99},
		{
			name: "up to past the Integer range",
			in:   constraints.CInteger{Range: constraints.NumericRange{Upper: 5_000_000_000, UpperInclusive: true, LowerUnbounded: true}},
			lo:   math.MaxInt32 - 100, hi: math.MaxInt32,
		},
		{name: "up to 5", in: constraints.CInteger{Range: constraints.NumericRange{Upper: 5, UpperInclusive: true, LowerUnbounded: true}}, lo: -1, hi: 5},
		{name: "a list reaching below", in: constraints.CInteger{List: []int64{-5, 2}}, lo: -1, hi: math.MaxInt32, list: []int64{2}},
		{
			name: "excluding -1, past the Integer range",
			in:   constraints.CInteger{Range: constraints.NumericRange{Lower: -1, Upper: 5_000_000_000, UpperInclusive: true}},
			lo:   0, hi: math.MaxInt32,
		},
		{name: "below -1", in: constraints.CInteger{Range: between(-10, -2)}, refused: true},
		{name: "a list below -1", in: constraints.CInteger{List: []int64{-5, -3}}, refused: true},
	}
	for _, tc := range cases {
		got, err := narrowInteger(tc.in, precisionLimits)
		switch {
		case tc.refused:
			if !errors.Is(err, errNoPrecisionValue) {
				t.Errorf("%s: narrowInteger = %+v, %v, want errNoPrecisionValue", tc.name, got, err)
			}
			continue
		case err != nil:
			t.Errorf("%s: narrowInteger: %v, want none", tc.name, err)
			continue
		case tc.same:
			if !slices.Equal(got.List, tc.in.List) || got.Range != tc.in.Range {
				t.Errorf("%s: narrowInteger = %+v, want the constraint as it was, %+v", tc.name, got, tc.in)
			}
			continue
		}
		want := constraints.NumericRange{Lower: float64(tc.lo), Upper: float64(tc.hi), LowerInclusive: true, UpperInclusive: true}
		if got.Range != want || !slices.Equal(got.List, tc.list) {
			t.Errorf("%s: narrowInteger = range %v list %v, want range %v list %v", tc.name, got.Range, got.List, want, tc.list)
		}
	}
}

// TestREQ107_IntegerExampleStaysOnTheBoundedSide pins the value ExampleFill
// takes from a narrowed C_INTEGER (§ Precedence 3): the narrowed
// constraint's least value, but for a constraint bounded above alone, whose
// own example value is its upper bound, the narrowed upper end, so a
// constraint such as `<= 5000000000` gives the greatest Integer, not the
// least. A constraint the narrowing leaves as it is gives its own example
// value.
func TestREQ107_IntegerExampleStaysOnTheBoundedSide(t *testing.T) {
	const (
		past   = float64(5_000_000_000)
		bottom = math.MinInt32
	)
	upTo := func(hi float64, inclusive bool) constraints.NumericRange {
		return constraints.NumericRange{Upper: hi, UpperInclusive: inclusive, LowerUnbounded: true}
	}
	cases := []struct {
		name   string
		in     constraints.CInteger
		limits intLimits
		want   int64
	}{
		{name: "up to past the Integer range", in: constraints.CInteger{Range: upTo(past, true)}, limits: integerLimits, want: math.MaxInt32},
		{name: "up to just above the least Integer", in: constraints.CInteger{Range: upTo(bottom+20, true)}, limits: integerLimits, want: bottom + 20},
		{
			name: "from below the Integer range", in: constraints.CInteger{Range: constraints.NumericRange{Lower: -past, LowerInclusive: true, UpperUnbounded: true}},
			limits: integerLimits, want: bottom,
		},
		{
			name: "past both ends", in: constraints.CInteger{Range: constraints.NumericRange{Lower: -past, Upper: past, LowerInclusive: true, UpperInclusive: true}},
			limits: integerLimits, want: bottom,
		},
		{name: "a precision up to 5", in: constraints.CInteger{Range: upTo(5, true)}, limits: precisionLimits, want: 5},
		{name: "up to 5, left as it is", in: constraints.CInteger{Range: upTo(5, false)}, limits: integerLimits, want: 4},
		{name: "a list up to past the Integer range", in: constraints.CInteger{List: []int64{7, 1 << 40}, Range: upTo(past, true)}, limits: integerLimits, want: 7},
	}
	for _, tc := range cases {
		narrowed, err := narrowInteger(tc.in, tc.limits)
		if err != nil {
			t.Errorf("%s: narrowInteger: %v, want none", tc.name, err)
			continue
		}
		got := integerExample(tc.in, narrowed)
		if got != any(tc.want) {
			t.Errorf("%s: integerExample = %v, want %d", tc.name, got, tc.want)
		}
		if len(narrowed.Validate(got)) != 0 || len(tc.in.Validate(got)) != 0 {
			t.Errorf("%s: integerExample = %v, which the constraint or its narrowing rejects", tc.name, got)
		}
	}
}

// TestREQ107_NarrowOrdinalKeepsTheIntegerPairs pins the narrowing of a
// C_DV_ORDINAL to the pairs whose value is a 32-bit RM Integer, which a
// DV_ORDINAL's value is (§ Precedence 3): a constraint whose pairs all are
// comes back as it is, one with a pair past that range keeps the others,
// in order, and one with none inside it is refused.
func TestREQ107_NarrowOrdinalKeepsTheIntegerPairs(t *testing.T) {
	pair := func(v int, code string) constraints.OrdinalSymbol {
		return constraints.OrdinalSymbol{Value: v, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: code}}
	}
	inside := constraints.CDvOrdinal{Values: []constraints.OrdinalSymbol{pair(1, "at0001"), pair(2, "at0002")}}
	if got, err := narrowOrdinal(inside); err != nil || !slices.Equal(got.Values, inside.Values) {
		t.Errorf("narrowOrdinal(inside) = %+v, %v, want it as it was", got, err)
	}
	mixed := constraints.CDvOrdinal{Values: []constraints.OrdinalSymbol{pair(5_000_000_000, "at0001"), pair(3, "at0002"), pair(-1, "at0003")}}
	if got, err := narrowOrdinal(mixed); err != nil || !slices.Equal(got.Values, mixed.Values[1:]) {
		t.Errorf("narrowOrdinal(mixed) = %+v, %v, want the pairs 3 and -1", got, err)
	}
	if mixed.Values[0].Value != 5_000_000_000 {
		t.Errorf("narrowOrdinal changed the constraint it was given: %+v", mixed)
	}
	outside := constraints.CDvOrdinal{Values: []constraints.OrdinalSymbol{pair(5_000_000_000, "at0001"), pair(-5_000_000_000, "at0002")}}
	if _, err := narrowOrdinal(outside); !errors.Is(err, errNoIntegerValue) {
		t.Errorf("narrowOrdinal(outside) error = %v, want errNoIntegerValue", err)
	}
}
