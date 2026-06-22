package rmwrite

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

func writeDVCountSingle(c *rm.DVCount, attr string, child any) error {
	switch attr {
	case "magnitude":
		n, ok := rm.AsInt64(child)
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
	}
	return fmt.Errorf("%w: *rm.DVQuantity has no single attr %q", ErrUnknownAttribute, attr)
}
