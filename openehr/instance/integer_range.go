package instance

import (
	"errors"
	"math"
	"slices"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// errNoIntegerValue reports a C_INTEGER none of whose values is a 32-bit
// RM Integer, on an attribute that holds one.
var errNoIntegerValue = errors.New("no value of the constraint is inside the 32-bit Integer range")

// holdsInteger reports whether attr of rmValue is an attribute of the RM
// type Integer, which is 32 bits wide, such as a DV_QUANTITY's precision.
// An attribute of the type Integer64, such as a DV_COUNT's magnitude, is
// not one, and neither is the value itself (attr "").
func holdsInteger(rmValue any, attr string) bool {
	rmType, ok := rminfo.Default.AttributeRMType(bmmtype.Class(rmTypeOf(rmValue)), attr)
	return ok && rmType == "Integer"
}

// narrowToInteger returns c narrowed to the values a 32-bit RM Integer
// holds, so that an example value or a draw is one the attribute can take.
// It returns c itself when nothing it would give leaves that range, and
// errNoIntegerValue when no value of c is inside it.
func narrowToInteger(c constraints.CInteger) (constraints.CInteger, error) {
	if !leavesInteger(c) {
		return c, nil
	}
	lo, hi, ok := integerBounds(c.Range)
	if !ok {
		return c, errNoIntegerValue
	}
	narrowed := c
	narrowed.Range = constraints.NumericRange{
		Lower: float64(lo), Upper: float64(hi),
		LowerInclusive: true, UpperInclusive: true,
	}
	if len(c.List) > 0 {
		narrowed.List = slices.DeleteFunc(slices.Clone(c.List), func(n int64) bool { return n < lo || n > hi })
		if len(narrowed.List) == 0 {
			return c, errNoIntegerValue
		}
	}
	return narrowed, nil
}

// leavesInteger reports whether the example value or a draw of c can be
// outside the 32-bit range: a list member is, or the window the sampler
// draws from, which covers the example value, reaches outside it.
func leavesInteger(c constraints.CInteger) bool {
	if slices.ContainsFunc(c.List, func(n int64) bool { return !fitsInteger(n) }) {
		return true
	}
	lo, hi, ok := intSampleBounds(c.Range)
	return ok && (!fitsInteger(lo) || !fitsInteger(hi))
}

func fitsInteger(n int64) bool { return n >= math.MinInt32 && n <= math.MaxInt32 }

// integerBounds returns the least and the greatest whole number of r that
// is a 32-bit RM Integer; an unbounded side takes the limit of the Integer
// range. ok is false when r holds no such number.
func integerBounds(r constraints.NumericRange) (lo, hi int64, ok bool) {
	lo, hi = math.MinInt32, math.MaxInt32
	if r.IsBounded() {
		if !r.LowerUnbounded && r.Lower > math.MinInt32 {
			if r.Lower > math.MaxInt32 {
				return 0, 0, false
			}
			lo = int64(math.Ceil(r.Lower))
			if !r.LowerInclusive && float64(lo) == r.Lower {
				lo++
			}
		}
		if !r.UpperUnbounded && r.Upper < math.MaxInt32 {
			if r.Upper < math.MinInt32 {
				return 0, 0, false
			}
			hi = int64(math.Floor(r.Upper))
			if !r.UpperInclusive && float64(hi) == r.Upper {
				hi--
			}
		}
	}
	return lo, hi, lo <= hi
}
