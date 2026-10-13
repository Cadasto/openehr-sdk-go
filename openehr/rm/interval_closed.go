package rm

// NewClosedInterval returns the interval from lower to upper that includes
// both bounds: both bounds are set, both included flags are true and both
// unbounded flags are false.
//
// Prefer it to a struct literal that sets only the bounds. The zero value of
// an Interval has every flag false, so such a literal excludes both bounds,
// which is not what a serialised interval that leaves its flags out means:
// the canonical decoders read an omitted included flag as true on a side
// that is not open.
func NewClosedInterval[T any](lower, upper T) Interval[T] {
	return Interval[T]{
		Lower:         lower,
		LowerIncluded: true,
		Upper:         upper,
		UpperIncluded: true,
	}
}

// NewClosedDVInterval returns the DV_INTERVAL from lower to upper that
// includes both bounds: both bounds are set, both included flags are true and
// both unbounded flags are false.
//
// Prefer it to a struct literal that sets only the bounds, for the reason
// [NewClosedInterval] gives. A field that holds a pointer, such as a normal
// range, takes new(NewClosedDVInterval(lower, upper)). For the
// DVInterval[DVOrdered] a polymorphic slot holds, name the type argument:
// NewClosedDVInterval[DVOrdered](&lower, &upper).
func NewClosedDVInterval[T DVOrdered](lower, upper T) DVInterval[T] {
	return DVInterval[T]{Interval: NewClosedInterval(lower, upper)}
}
