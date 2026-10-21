package rmwrite

import (
	"fmt"
	"math"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

func writeDVCountSingle(c *rm.DVCount, attr string, child any) error {
	switch attr {
	case "magnitude":
		n, ok := rm.AsInt64(child)
		if !ok {
			return mismatch(attr, child, "Integer")
		}
		c.Magnitude = rm.Integer64(n)
		return nil
	case "accuracy":
		return writeOptionalReal(&c.Accuracy, attr, child)
	case "accuracy_is_percent":
		return writeOptionalBoolean(&c.AccuracyIsPercent, attr, child)
	case "magnitude_status":
		return writeOptionalString(&c.MagnitudeStatus, attr, child)
	}
	return writeOrderedSingle("*rm.DVCount", &c.NormalStatus, &c.NormalRange, attr, child)
}

func writeDVQuantitySingle(q *rm.DVQuantity, attr string, child any) error {
	switch attr {
	case "magnitude":
		v, ok := rm.AsReal(child)
		if !ok {
			return mismatch(attr, child, "Real")
		}
		q.Magnitude = v
		return nil
	case "units":
		v, ok := child.(string)
		if !ok {
			return mismatch(attr, child, "String")
		}
		q.Units = v
		return nil
	case "accuracy":
		return writeOptionalReal(&q.Accuracy, attr, child)
	case "accuracy_is_percent":
		return writeOptionalBoolean(&q.AccuracyIsPercent, attr, child)
	case "magnitude_status":
		return writeOptionalString(&q.MagnitudeStatus, attr, child)
	case "precision":
		return writeOptionalInteger(&q.Precision, attr, child)
	case "units_system":
		return writeOptionalString(&q.UnitsSystem, attr, child)
	case "units_display_name":
		return writeOptionalString(&q.UnitsDisplayName, attr, child)
	}
	return writeOrderedSingle("*rm.DVQuantity", &q.NormalStatus, &q.NormalRange, attr, child)
}

// writeOptionalReal stores a number on an optional Real attribute, which
// the RM type holds as a pointer.
func writeOptionalReal(dst **rm.Real, attr string, child any) error {
	v, ok := rm.AsReal(child)
	if !ok {
		return mismatch(attr, child, "Real")
	}
	*dst = &v
	return nil
}

// writeOptionalInteger stores a whole number on an optional Integer
// attribute, which the RM type holds as a pointer.
func writeOptionalInteger(dst **rm.Integer, attr string, child any) error {
	v, err := asInteger(attr, child)
	if err != nil {
		return err
	}
	*dst = &v
	return nil
}

// asInteger converts child, a whole number of any Go integer shape or an
// RM Integer held by pointer, to the 32-bit RM Integer attr holds. A child
// that is no whole number is a type mismatch; a whole number outside the
// 32-bit range is one too, and its message says that the range is what
// fails. The message carries the attribute, never the value.
func asInteger(attr string, child any) (rm.Integer, error) {
	if p, ok := child.(*rm.Integer); ok && p != nil {
		child = *p
	}
	n, ok := rm.AsInt64(child)
	if !ok {
		return 0, mismatch(attr, child, "Integer")
	}
	if n < math.MinInt32 || n > math.MaxInt32 {
		return 0, fmt.Errorf("%w: attr %q takes an Integer and the whole number is outside the 32-bit Integer range", ErrTypeMismatch, attr)
	}
	return rm.Integer(n), nil
}

// writeOptionalBoolean stores a bool on an optional Boolean attribute,
// which the RM type holds as a pointer.
func writeOptionalBoolean(dst **bool, attr string, child any) error {
	v, ok := child.(bool)
	if !ok {
		return mismatch(attr, child, "Boolean")
	}
	*dst = &v
	return nil
}
