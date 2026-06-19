package rmread

import "github.com/cadasto/openehr-sdk-go/openehr/rm"

func readDVCountSingle(c *rm.DVCount, attr string) (any, bool) {
	switch attr {
	case "magnitude":
		return c.Magnitude, true
	case "normal_status":
		return ptrPresent(c.NormalStatus)
	case "normal_range":
		return intervalPresent(c.NormalRange)
	}
	return nil, false
}

func readDVQuantitySingle(q *rm.DVQuantity, attr string) (any, bool) {
	switch attr {
	case "magnitude":
		return q.Magnitude, true
	case "units":
		return strPresent(q.Units)
	case "normal_status":
		return ptrPresent(q.NormalStatus)
	case "normal_range":
		return intervalPresent(q.NormalRange)
	}
	return nil, false
}

func readDVProportionSingle(p *rm.DVProportion, attr string) (any, bool) {
	switch attr {
	case "numerator":
		return p.Numerator, true
	case "denominator":
		return p.Denominator, true
	case "type":
		return p.Type, true
	case "precision":
		if p.Precision == nil {
			return p.Precision, false
		}
		return *p.Precision, true
	case "normal_status":
		return ptrPresent(p.NormalStatus)
	case "normal_range":
		return intervalPresent(p.NormalRange)
	}
	return nil, false
}

func readDVURISingle(u *rm.DVURI, attr string) (any, bool) {
	if attr == "value" {
		return strPresent(u.Value)
	}
	return nil, false
}

func readDVEHRURISingle(u *rm.DVEHRURI, attr string) (any, bool) {
	if attr == "value" {
		return strPresent(u.Value)
	}
	return nil, false
}

func readDVParsableSingle(p *rm.DVParsable, attr string) (any, bool) {
	switch attr {
	case "value":
		return strPresent(p.Value)
	case "formalism":
		return strPresent(p.Formalism)
	}
	return nil, false
}

func readDVIntervalQuantitySingle(iv *rm.DVInterval[rm.DVQuantity], attr string) (any, bool) {
	return readIntervalSingle(&iv.Interval, attr)
}

func readDVIntervalCountSingle(iv *rm.DVInterval[rm.DVCount], attr string) (any, bool) {
	return readIntervalSingle(&iv.Interval, attr)
}

func readDVIntervalDateTimeSingle(iv *rm.DVInterval[rm.DVDateTime], attr string) (any, bool) {
	return readIntervalSingle(&iv.Interval, attr)
}

func readDVIntervalDateSingle(iv *rm.DVInterval[rm.DVDate], attr string) (any, bool) {
	return readIntervalSingle(&iv.Interval, attr)
}

func readDVIntervalTimeSingle(iv *rm.DVInterval[rm.DVTime], attr string) (any, bool) {
	return readIntervalSingle(&iv.Interval, attr)
}

func readDVIntervalProportionSingle(iv *rm.DVInterval[rm.DVProportion], attr string) (any, bool) {
	return readIntervalSingle(&iv.Interval, attr)
}

func readDVIntervalOrderedSingle(iv *rm.DVInterval[rm.DVOrdered], attr string) (any, bool) {
	return readIntervalSingle(&iv.Interval, attr)
}

func readIntervalSingle[T any](iv *rm.Interval[T], attr string) (any, bool) {
	switch attr {
	case "lower":
		return iv.Lower, true
	case "upper":
		return iv.Upper, true
	case "lower_unbounded":
		return iv.LowerUnbounded, true
	case "upper_unbounded":
		return iv.UpperUnbounded, true
	case "lower_included":
		return iv.LowerIncluded, true
	case "upper_included":
		return iv.UpperIncluded, true
	}
	return nil, false
}

func intervalPresent[T rm.DVOrdered](iv *rm.DVInterval[T]) (any, bool) {
	if iv == nil {
		return nil, false
	}
	return iv, true
}
