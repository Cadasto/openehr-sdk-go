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

func writeDVIntervalOrderedSingle(iv *rm.DVInterval[rm.DVOrdered], attr string, child any) error {
	return writeIntervalSingle(&iv.Interval, attr, child, "DV_ORDERED")
}

func writeIntervalSingle[T any](iv *rm.Interval[T], attr string, child any, boundRM string) error {
	switch attr {
	case "lower":
		v, ok := coerceBound[T](child)
		if !ok {
			return mismatch(attr, child, boundRM)
		}
		iv.Lower = v
		return nil
	case "upper":
		v, ok := coerceBound[T](child)
		if !ok {
			return mismatch(attr, child, boundRM)
		}
		iv.Upper = v
		return nil
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

func coerceBound[T any](child any) (T, bool) {
	var zero T
	switch v := child.(type) {
	case T:
		return v, true
	case *T:
		if v == nil {
			return zero, false
		}
		return *v, true
	}
	return zero, false
}
