package instance

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// errNoIntegerValue reports a C_INTEGER none of whose values is a 32-bit
// RM Integer, on an attribute that holds one, and a C_DV_ORDINAL none of
// whose values is.
var errNoIntegerValue = errors.New("no value of the constraint is inside the 32-bit Integer range")

// errNoPrecisionValue reports a C_INTEGER on a DV_QUANTITY's or a
// DV_PROPORTION's precision none of whose values is -1 or more and inside
// the 32-bit Integer range: -1 means no limit, and the RM admits no lower
// precision.
var errNoPrecisionValue = errors.New("no value of the constraint is a precision of -1 or more inside the 32-bit Integer range")

// intLimits are the least and the greatest whole number an attribute
// holds, and the error a constraint on it with no value between them
// reports.
type intLimits struct {
	lo, hi int64
	none   error
}

var (
	// integerLimits is the 32-bit range of an RM Integer.
	integerLimits = intLimits{lo: math.MinInt32, hi: math.MaxInt32, none: errNoIntegerValue}
	// precisionLimits is the part of it a precision takes: from -1, no
	// limit, up.
	precisionLimits = intLimits{lo: -1, hi: math.MaxInt32, none: errNoPrecisionValue}
)

// narrowedLeaf returns pc narrowed by the RM rule on the value it gives
// attr of rmValue, where one narrows what the OPT leaves open (REQ-107
// Precedence 3), and the value ExampleFill takes from it. A C_INTEGER on
// an attribute of the RM type Integer, which is 32 bits wide, is narrowed
// to that range, and on the precision of a DV_QUANTITY or a DV_PROPORTION
// to the part of it from -1 up (narrowInteger). A C_DV_ORDINAL keeps the
// pairs whose value is an Integer (narrowOrdinal). Any other pc comes back
// as it is, with its own example value. Where nothing of pc is left, the
// leaf fails with an error wrapping ErrConstraintUnsatisfiable that names
// the RM type, the attribute and the OPT path, and no value.
func narrowedLeaf(leaf *tcimpl.CompiledNode, rmValue any, attr string, pc constraints.PrimitiveConstraint) (constraints.PrimitiveConstraint, any, error) {
	unsatisfiable := func(attr string, err error) error {
		return fmt.Errorf("%w: %s.%s at %s: %w", ErrConstraintUnsatisfiable, rmTypeOf(rmValue), attr, leafPath(leaf), err)
	}
	switch c := pc.(type) {
	case constraints.CInteger:
		limits, held := integerLimitsOf(rmValue, attr)
		if !held {
			break
		}
		narrowed, err := narrowInteger(c, limits)
		if err != nil {
			return nil, nil, unsatisfiable(attr, err)
		}
		return narrowed, integerExample(c, narrowed), nil
	case constraints.CDvOrdinal:
		// A C_DV_ORDINAL constrains the ordinal itself, and so its value.
		narrowed, err := narrowOrdinal(c)
		if err != nil {
			return nil, nil, unsatisfiable("value", err)
		}
		return narrowed, narrowed.ExampleValue(), nil
	}
	return pc, pc.ExampleValue(), nil
}

// integerLimitsOf returns the limits of attr of rmValue where it is an
// attribute of the RM type Integer (holdsInteger): precisionLimits for the
// precision of a DV_QUANTITY or a DV_PROPORTION (Precision_validity reads
// -1 as no limit), integerLimits for any other. held is false for an
// attribute of any other type.
func integerLimitsOf(rmValue any, attr string) (limits intLimits, held bool) {
	if !holdsInteger(rmValue, attr) {
		return intLimits{}, false
	}
	if attr == "precision" {
		switch rmValue.(type) {
		case *rm.DVQuantity, *rm.DVProportion:
			return precisionLimits, true
		}
	}
	return integerLimits, true
}

// holdsInteger reports whether attr of rmValue is an attribute of the RM
// type Integer, which is 32 bits wide, such as a DV_QUANTITY's precision.
// An attribute of the type Integer64, such as a DV_COUNT's magnitude, is
// not one, and neither is the value itself (attr "").
func holdsInteger(rmValue any, attr string) bool {
	rmType, ok := rminfo.Default.AttributeRMType(bmmtype.Class(rmTypeOf(rmValue)), attr)
	return ok && rmType == "Integer"
}

// narrowInteger returns c narrowed to the whole numbers inside limits, so
// that an example value or a draw is one the attribute can take. It
// returns c itself when nothing it would give leaves limits, and
// limits.none when no value of c is inside them. The narrowed range holds
// the values of c inside limits, both its ends included.
func narrowInteger(c constraints.CInteger, limits intLimits) (constraints.CInteger, error) {
	if !leavesLimits(c, limits) {
		return c, nil
	}
	lo, hi, ok := boundsWithin(c.Range, limits)
	if !ok {
		return c, limits.none
	}
	narrowed := c
	narrowed.Range = constraints.NumericRange{
		Lower: float64(lo), Upper: float64(hi),
		LowerInclusive: true, UpperInclusive: true,
	}
	if len(c.List) > 0 {
		narrowed.List = slices.DeleteFunc(slices.Clone(c.List), func(n int64) bool { return n < lo || n > hi })
		if len(narrowed.List) == 0 {
			return c, limits.none
		}
	}
	return narrowed, nil
}

// integerExample returns the value ExampleFill takes from narrowed, the
// narrowing of c: narrowed's own example value, its least value, unless c
// is bounded above alone and has no list, as `<= 5000000000` is. c's own
// example value is then its upper bound, so the example stays on the side
// c bounds: the upper end of narrowed, not the lower end of the Integer
// range.
func integerExample(c, narrowed constraints.CInteger) any {
	r := c.Range
	if narrowed.Range != r && len(c.List) == 0 && r.IsBounded() && r.LowerUnbounded && !r.UpperUnbounded {
		return int64(narrowed.Range.Upper)
	}
	return narrowed.ExampleValue()
}

// narrowOrdinal returns c with only the pairs whose value is a 32-bit RM
// Integer, which a DV_ORDINAL's value is, and errNoIntegerValue when no
// pair's is. It returns c itself when every pair's value is one.
func narrowOrdinal(c constraints.CDvOrdinal) (constraints.CDvOrdinal, error) {
	outside := func(p constraints.OrdinalSymbol) bool { return !fitsInteger(int64(p.Value)) }
	if !slices.ContainsFunc(c.Values, outside) {
		return c, nil
	}
	kept := slices.DeleteFunc(slices.Clone(c.Values), outside)
	if len(kept) == 0 {
		return c, errNoIntegerValue
	}
	return constraints.CDvOrdinal{Values: kept}, nil
}

// leavesLimits reports whether the example value or a draw of c can be
// outside limits: a list member is, or the window the sampler draws from,
// which covers the example value, reaches outside them.
func leavesLimits(c constraints.CInteger, limits intLimits) bool {
	within := func(n int64) bool { return n >= limits.lo && n <= limits.hi }
	if slices.ContainsFunc(c.List, func(n int64) bool { return !within(n) }) {
		return true
	}
	lo, hi, ok := intSampleBounds(c.Range)
	return ok && (!within(lo) || !within(hi))
}

func fitsInteger(n int64) bool { return n >= math.MinInt32 && n <= math.MaxInt32 }

// boundsWithin returns the least and the greatest whole number of r that
// is inside limits; an unbounded side takes the limit on that side. ok is
// false when r holds no such number.
func boundsWithin(r constraints.NumericRange, limits intLimits) (lo, hi int64, ok bool) {
	lo, hi = limits.lo, limits.hi
	if r.IsBounded() {
		if !r.LowerUnbounded && r.Lower >= float64(limits.lo) {
			if r.Lower > float64(limits.hi) {
				return 0, 0, false
			}
			lo = int64(math.Ceil(r.Lower))
			if !r.LowerInclusive && float64(lo) == r.Lower {
				lo++
			}
		}
		if !r.UpperUnbounded && r.Upper <= float64(limits.hi) {
			if r.Upper < float64(limits.lo) {
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
