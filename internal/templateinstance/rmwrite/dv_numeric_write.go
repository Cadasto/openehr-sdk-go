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
	case "normal_status":
		v, ok := child.(*rm.CodePhrase)
		if !ok {
			return mismatch(attr, child, "CODE_PHRASE")
		}
		c.NormalStatus = v
		return nil
	case "normal_range":
		v, ok := child.(*rm.DVInterval[rm.DVCount])
		if !ok {
			return mismatch(attr, child, "DV_INTERVAL")
		}
		c.NormalRange = v
		return nil
	case "accuracy":
		return writeOptionalReal(&c.Accuracy, attr, child)
	case "accuracy_is_percent":
		return writeOptionalBoolean(&c.AccuracyIsPercent, attr, child)
	case "magnitude_status":
		return writeOptionalString(&c.MagnitudeStatus, attr, child)
	}
	return fmt.Errorf("%w: *rm.DVCount has no single attr %q", ErrUnknownAttribute, attr)
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
	case "normal_status":
		v, ok := child.(*rm.CodePhrase)
		if !ok {
			return mismatch(attr, child, "CODE_PHRASE")
		}
		q.NormalStatus = v
		return nil
	case "normal_range":
		v, ok := child.(*rm.DVInterval[rm.DVQuantity])
		if !ok {
			return mismatch(attr, child, "DV_INTERVAL")
		}
		q.NormalRange = v
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
	return fmt.Errorf("%w: *rm.DVQuantity has no single attr %q", ErrUnknownAttribute, attr)
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
// attribute, which the RM type holds as a pointer. A number outside the
// 32-bit range an Integer holds is refused.
func writeOptionalInteger(dst **rm.Integer, attr string, child any) error {
	n, ok := rm.AsInt64(child)
	if !ok || n < math.MinInt32 || n > math.MaxInt32 {
		return mismatch(attr, child, "Integer")
	}
	v := rm.Integer(n)
	*dst = &v
	return nil
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
