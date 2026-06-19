package rmwrite

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

func writeDVCountSingle(c *rm.DVCount, attr string, child any) error {
	switch attr {
	case "magnitude":
		n, ok := coerceInt64(child)
		if !ok {
			return mismatch(attr, child, "Integer")
		}
		c.Magnitude = n
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
	}
	return fmt.Errorf("%w: *rm.DVCount has no single attr %q", ErrUnknownAttribute, attr)
}

func writeDVQuantitySingle(q *rm.DVQuantity, attr string, child any) error {
	switch attr {
	case "magnitude":
		switch v := child.(type) {
		case rm.Real:
			q.Magnitude = v
		case float64:
			q.Magnitude = rm.Real(v)
		case int64:
			q.Magnitude = rm.Real(v)
		case int:
			q.Magnitude = rm.Real(v)
		default:
			return mismatch(attr, child, "Real")
		}
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
	}
	return fmt.Errorf("%w: *rm.DVQuantity has no single attr %q", ErrUnknownAttribute, attr)
}

func coerceInt64(child any) (int64, bool) {
	switch v := child.(type) {
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return int64(v), true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case rm.Integer:
		return int64(v), true
	}
	return 0, false
}
