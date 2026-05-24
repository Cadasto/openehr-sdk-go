package constraints

import (
	"fmt"
	"strings"
)

// NumericRange is the inclusive / exclusive interval shape AOM 1.4
// uses for primitive numeric constraints (C_INTEGER / C_REAL ranges,
// DV_QUANTITY magnitude / precision ranges).
//
// The zero value represents an "any value accepted" range — both
// sides unbounded, which [Contains] returns true for.
type NumericRange struct {
	// Lower / Upper are the numeric bounds. Float64 covers both
	// C_INTEGER (lossless up to 2^53) and C_REAL.
	Lower, Upper float64

	// LowerInclusive / UpperInclusive flip the bounds between closed
	// [a,b] and open (a,b). Default false matches the AOM 1.4
	// "exclusive" reading on each side; AOM XML usually sets both to
	// true.
	LowerInclusive, UpperInclusive bool

	// LowerUnbounded / UpperUnbounded mark the side as "no constraint"
	// — Lower / Upper are then ignored. Mirrors the
	// <lower_unbounded> / <upper_unbounded> wire booleans.
	LowerUnbounded, UpperUnbounded bool
}

// IsBounded reports whether the range carries any constraint at all.
// Returns false for the zero value (no fields set — treated as
// "any value accepted") and for ranges with both sides explicitly
// unbounded. Used by validators to short-circuit the
// no-op-validation case.
func (r NumericRange) IsBounded() bool {
	if r == (NumericRange{}) {
		return false
	}
	return !r.LowerUnbounded || !r.UpperUnbounded
}

// IsValid reports whether the range is internally consistent — the
// lower bound is less than (or equal to, when both sides are
// inclusive) the upper bound. Unbounded sides are skipped.
func (r NumericRange) IsValid() bool {
	if r.LowerUnbounded || r.UpperUnbounded {
		return true
	}
	if r.Lower < r.Upper {
		return true
	}
	return r.Lower == r.Upper && r.LowerInclusive && r.UpperInclusive
}

// Contains reports whether v is inside the range under the configured
// inclusivity. Unbounded sides accept any value on that side.
func (r NumericRange) Contains(v float64) bool {
	if !r.LowerUnbounded {
		if r.LowerInclusive {
			if v < r.Lower {
				return false
			}
		} else {
			if v <= r.Lower {
				return false
			}
		}
	}
	if !r.UpperUnbounded {
		if r.UpperInclusive {
			if v > r.Upper {
				return false
			}
		} else {
			if v >= r.Upper {
				return false
			}
		}
	}
	return true
}

// String renders the range in standard interval notation — `[a..b]`
// for fully-closed, `(a..b)` for open, `[a..*)` for half-unbounded,
// etc. Convenient for Violation.Detail messages.
func (r NumericRange) String() string {
	var sb strings.Builder
	if r.LowerInclusive {
		sb.WriteByte('[')
	} else {
		sb.WriteByte('(')
	}
	if r.LowerUnbounded {
		sb.WriteByte('*')
	} else {
		fmt.Fprintf(&sb, "%v", r.Lower)
	}
	sb.WriteString("..")
	if r.UpperUnbounded {
		sb.WriteByte('*')
	} else {
		fmt.Fprintf(&sb, "%v", r.Upper)
	}
	if r.UpperInclusive {
		sb.WriteByte(']')
	} else {
		sb.WriteByte(')')
	}
	return sb.String()
}
