package rmwrite

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

func writeDVIntervalQuantitySingle(iv *rm.DVInterval[rm.DVQuantity], attr string, child any) error {
	return writeIntervalSingle(&iv.Interval, attr, child, "DV_QUANTITY")
}

func writeDVIntervalCountSingle(iv *rm.DVInterval[rm.DVCount], attr string, child any) error {
	return writeIntervalSingle(&iv.Interval, attr, child, "DV_COUNT")
}

func writeDVIntervalDateTimeSingle(iv *rm.DVInterval[rm.DVDateTime], attr string, child any) error {
	return writeIntervalSingle(&iv.Interval, attr, child, "DV_DATE_TIME")
}

func writeDVIntervalDateSingle(iv *rm.DVInterval[rm.DVDate], attr string, child any) error {
	return writeIntervalSingle(&iv.Interval, attr, child, "DV_DATE")
}

func writeDVIntervalTimeSingle(iv *rm.DVInterval[rm.DVTime], attr string, child any) error {
	return writeIntervalSingle(&iv.Interval, attr, child, "DV_TIME")
}

func writeDVIntervalProportionSingle(iv *rm.DVInterval[rm.DVProportion], attr string, child any) error {
	return writeIntervalSingle(&iv.Interval, attr, child, "DV_PROPORTION")
}

func writeDVIntervalDurationSingle(iv *rm.DVInterval[rm.DVDuration], attr string, child any) error {
	return writeIntervalSingle(&iv.Interval, attr, child, "DV_DURATION")
}

func writeDVIntervalOrdinalSingle(iv *rm.DVInterval[rm.DVOrdinal], attr string, child any) error {
	return writeIntervalSingle(&iv.Interval, attr, child, "DV_ORDINAL")
}

func writeDVIntervalScaleSingle(iv *rm.DVInterval[rm.DVScale], attr string, child any) error {
	return writeIntervalSingle(&iv.Interval, attr, child, "DV_SCALE")
}

func writeDVIntervalOrderedSingle(iv *rm.DVInterval[rm.DVOrdered], attr string, child any) error {
	return writeIntervalSingle(&iv.Interval, attr, child, "DV_ORDERED")
}

// writeIntervalSingle sets one attribute of an interval. An open side
// carries no bound, so writing a bound also sets that side's *_unbounded
// flag to match it: a bound that is not Void (see isVoidBound) closes its
// side, and a Void bound opens it. The other side is left as it is.
// Writes apply in order: a *_unbounded flag written after the bound
// replaces what the bound set.
func writeIntervalSingle[T any](iv *rm.Interval[T], attr string, child any, boundRM string) error {
	switch attr {
	case "lower":
		return assignVia(child, func(v T) {
			iv.Lower = v
			iv.LowerUnbounded = isVoidBound(v)
		}, attr, boundRM)
	case "upper":
		return assignVia(child, func(v T) {
			iv.Upper = v
			iv.UpperUnbounded = isVoidBound(v)
		}, attr, boundRM)
	case "lower_unbounded":
		v, ok := child.(bool)
		if !ok {
			return mismatch(attr, child, "bool")
		}
		iv.LowerUnbounded = v
		return nil
	case "upper_unbounded":
		v, ok := child.(bool)
		if !ok {
			return mismatch(attr, child, "bool")
		}
		iv.UpperUnbounded = v
		return nil
	case "lower_included":
		v, ok := child.(bool)
		if !ok {
			return mismatch(attr, child, "bool")
		}
		iv.LowerIncluded = v
		return nil
	case "upper_included":
		v, ok := child.(bool)
		if !ok {
			return mismatch(attr, child, "bool")
		}
		iv.UpperIncluded = v
		return nil
	}
	return fmt.Errorf("%w: *rm.Interval[%s] has no single attr %q", ErrUnknownAttribute, boundRM, attr)
}

// isVoidBound reports whether a bound the writer accepted still means
// "no bound". Only the DV_INTERVAL<DV_ORDERED> instantiation can hold
// one: its bound is an interface, which can be nil or carry a typed-nil
// pointer such as (*rm.DVQuantity)(nil). A concrete bound type is a
// struct value and is never Void, even when all its fields are zero.
func isVoidBound[T any](v T) bool {
	b := any(v)
	return b == nil || rm.IsTypedNil(b)
}
