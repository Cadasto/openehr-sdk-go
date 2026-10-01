package rmread

import "github.com/cadasto/openehr-sdk-go/openehr/rm"

// readDVCountSingle, readDVQuantitySingle and readDVProportionSingle also
// read the optional DV_AMOUNT accuracy (a Real), then fall back to the
// DV_ORDERED attributes.
func readDVCountSingle(c *rm.DVCount, attr string) (any, bool) {
	switch attr {
	case "magnitude":
		return c.Magnitude, true
	case "accuracy":
		return ptrPresent(c.Accuracy)
	}
	return readOrderedSingle(c.NormalStatus, c.NormalRange, attr)
}

func readDVQuantitySingle(q *rm.DVQuantity, attr string) (any, bool) {
	switch attr {
	case "magnitude":
		return q.Magnitude, true
	case "units":
		return strPresent(q.Units)
	case "accuracy":
		return ptrPresent(q.Accuracy)
	}
	return readOrderedSingle(q.NormalStatus, q.NormalRange, attr)
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
	case "accuracy":
		return ptrPresent(p.Accuracy)
	}
	return readOrderedSingle(p.NormalStatus, p.NormalRange, attr)
}

// readDVOrdinalSingle serves DV_ORDINAL: the RM-mandatory symbol
// (DV_CODED_TEXT) and value (Integer), then the DV_ORDERED attributes. symbol
// is value-typed and reads as absent when it carries neither a value nor a
// code; value is an Integer, always structurally present, as
// DV_COUNT.magnitude is.
func readDVOrdinalSingle(o *rm.DVOrdinal, attr string) (any, bool) {
	switch attr {
	case "symbol":
		return dvCodedTextPresent(o.Symbol)
	case "value":
		return o.Value, true
	}
	return readOrderedSingle(o.NormalStatus, o.NormalRange, attr)
}

// readDVScaleSingle serves DV_SCALE the same way as DV_ORDINAL; its value is
// a Real.
func readDVScaleSingle(s *rm.DVScale, attr string) (any, bool) {
	switch attr {
	case "symbol":
		return dvCodedTextPresent(s.Symbol)
	case "value":
		return s.Value, true
	}
	return readOrderedSingle(s.NormalStatus, s.NormalRange, attr)
}

// readOrderedSingle serves the two single-valued attributes DV_ORDERED
// declares and every DV_ORDERED concrete inherits: the optional
// normal_status (CODE_PHRASE) and normal_range (DV_INTERVAL). Each concrete
// reader falls back to it for any attribute of its own it does not serve.
func readOrderedSingle[T rm.DVOrdered](normalStatus *rm.CodePhrase, normalRange *rm.DVInterval[T], attr string) (any, bool) {
	switch attr {
	case "normal_status":
		return ptrPresent(normalStatus)
	case "normal_range":
		return intervalPresent(normalRange)
	}
	return nil, false
}

// readOrderedMultiple serves DV_ORDERED's one container,
// other_reference_ranges, boxing each REFERENCE_RANGE as a pointer into the
// backing array so the walk recognises and descends into it.
func readOrderedMultiple[T rm.DVOrdered](ranges []rm.ReferenceRange[T], attr string) ([]any, bool) {
	if attr == "other_reference_ranges" {
		return boxPtrs(ranges), true
	}
	return nil, false
}

// readReferenceRangeSingle serves REFERENCE_RANGE: the RM-mandatory meaning
// (DV_TEXT) and range (DV_INTERVAL). range is value-typed, and an omitted
// range decodes to an interval with no bound and neither side open, so that
// zero interval reads as absent. A range supplied with no bound and every
// flag false reads as absent too, and so is reported as `required`: it cannot
// be told apart from an omitted one, and it breaks the RM's DV_INTERVAL
// Limits_consistent anyway, which needs a comparable lower and upper bound
// when neither side is open. Any other range reads as present, as a pointer
// the walk descends into, so its bounds are checked like any interval's.
func readReferenceRangeSingle[T rm.DVOrdered](r *rm.ReferenceRange[T], attr string) (any, bool) {
	switch attr {
	case "meaning":
		return dvTextPresent(r.Meaning)
	case "range":
		if isZeroInterval(&r.Range) {
			return nil, false
		}
		return &r.Range, true
	}
	return nil, false
}

// isZeroInterval reports whether iv is the Go zero interval: no bound, and
// every flag false.
func isZeroInterval(iv *rm.DVInterval[rm.DVOrdered]) bool {
	return rm.IsEmptyIntervalBound(iv.Lower) && rm.IsEmptyIntervalBound(iv.Upper) &&
		!iv.LowerUnbounded && !iv.UpperUnbounded && !iv.LowerIncluded && !iv.UpperIncluded
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

func readDVIntervalDurationSingle(iv *rm.DVInterval[rm.DVDuration], attr string) (any, bool) {
	return readIntervalSingle(&iv.Interval, attr)
}

func readDVIntervalOrdinalSingle(iv *rm.DVInterval[rm.DVOrdinal], attr string) (any, bool) {
	return readIntervalSingle(&iv.Interval, attr)
}

func readDVIntervalScaleSingle(iv *rm.DVInterval[rm.DVScale], attr string) (any, bool) {
	return readIntervalSingle(&iv.Interval, attr)
}

func readDVIntervalOrderedSingle(iv *rm.DVInterval[rm.DVOrdered], attr string) (any, bool) {
	return readIntervalSingle(&iv.Interval, attr)
}

// readIntervalSingle reads one attribute of an interval. BASE Interval's
// `*_unbounded` flag marks that boundary open, so an open side carries no
// bound value: when the flag is set and the bound is Void
// ([rm.IsEmptyIntervalBound], the test the canonical encoders apply), the
// bound reads as absent. A concrete-typed Go interval cannot hold a nil
// bound and keeps the type's zero value there instead. A bound that is not
// Void reads as present even beside its own flag, and so does a bounded
// side, whatever its value.
func readIntervalSingle[T any](iv *rm.Interval[T], attr string) (any, bool) {
	switch attr {
	case "lower":
		if iv.LowerUnbounded && rm.IsEmptyIntervalBound(iv.Lower) {
			return nil, false
		}
		return iv.Lower, true
	case "upper":
		if iv.UpperUnbounded && rm.IsEmptyIntervalBound(iv.Upper) {
			return nil, false
		}
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
