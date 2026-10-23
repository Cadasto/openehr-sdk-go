package rmwrite

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// writeDVOrdinalSingle writes a DV_ORDINAL's value, its symbol, and the
// attributes it inherits from DV_ORDERED.
func writeDVOrdinalSingle(o *rm.DVOrdinal, attr string, child any) error {
	switch attr {
	case "value":
		v, err := asInteger(attr, child)
		if err != nil {
			return err
		}
		o.Value = v
		return nil
	case "symbol":
		return assignVia(child, func(v rm.DVCodedText) { o.Symbol = v }, attr, "DV_CODED_TEXT")
	}
	return writeOrderedSingle("*rm.DVOrdinal", &o.NormalStatus, &o.NormalRange, attr, child)
}

// writeDVScaleSingle writes a DV_SCALE's value, its symbol, and the
// attributes it inherits from DV_ORDERED.
func writeDVScaleSingle(s *rm.DVScale, attr string, child any) error {
	switch attr {
	case "value":
		v, ok := rm.AsReal(child)
		if !ok {
			return mismatch(attr, child, "Real")
		}
		s.Value = v
		return nil
	case "symbol":
		return assignVia(child, func(v rm.DVCodedText) { s.Symbol = v }, attr, "DV_CODED_TEXT")
	}
	return writeOrderedSingle("*rm.DVScale", &s.NormalStatus, &s.NormalRange, attr, child)
}

// writeDVProportionSingle writes a DV_PROPORTION's numerator, denominator,
// type and precision, the accuracy and magnitude_status it inherits from
// DV_AMOUNT and DV_QUANTIFIED, and the attributes it inherits from
// DV_ORDERED.
func writeDVProportionSingle(p *rm.DVProportion, attr string, child any) error {
	switch attr {
	case "numerator":
		return writeReal(&p.Numerator, attr, child)
	case "denominator":
		return writeReal(&p.Denominator, attr, child)
	case "type":
		v, err := asInteger(attr, child)
		if err != nil {
			return err
		}
		p.Type = v
		return nil
	case "precision":
		return writeOptionalInteger(&p.Precision, attr, child)
	case "accuracy":
		return writeOptionalReal(&p.Accuracy, attr, child)
	case "accuracy_is_percent":
		return writeOptionalBoolean(&p.AccuracyIsPercent, attr, child)
	case "magnitude_status":
		return writeOptionalString(&p.MagnitudeStatus, attr, child)
	}
	return writeOrderedSingle("*rm.DVProportion", &p.NormalStatus, &p.NormalRange, attr, child)
}

// writeReferenceRangeSingle writes a REFERENCE_RANGE's meaning, a DV_TEXT
// or its descendant DV_CODED_TEXT, and its range, a DV_INTERVAL of any
// instantiation, which the field holds as DV_INTERVAL<DV_ORDERED>
// (intervalAs). The type registry builds a REFERENCE_RANGE over
// DV_ORDERED.
func writeReferenceRangeSingle(r *rm.ReferenceRange[rm.DVOrdered], attr string, child any) error {
	switch attr {
	case "meaning":
		return assignDVTextLike(child, func(v rm.DVTextLike) { r.Meaning = v }, attr)
	case "range":
		iv, err := intervalAs[rm.DVOrdered](child, attr, "DV_INTERVAL")
		if err != nil {
			return err
		}
		r.Range = iv
		return nil
	}
	return fmt.Errorf("%w: *rm.ReferenceRange has no single attr %q", ErrUnknownAttribute, attr)
}

// writeOrderedSingle writes the two single attributes every DV_ORDERED
// class declares: normal_status, a CODE_PHRASE given as a value or as a
// pointer, and normal_range, a DV_INTERVAL of any instantiation converted
// into the one the class's field holds, T (intervalAs). class names the Go
// type for the error detail.
func writeOrderedSingle[T rm.DVOrdered](class string, status **rm.CodePhrase, normalRange **rm.DVInterval[T], attr string, child any) error {
	switch attr {
	case "normal_status":
		return assignVia(child, func(v rm.CodePhrase) { *status = &v }, attr, "CODE_PHRASE")
	case "normal_range":
		iv, err := intervalAs[T](child, attr, intervalRMName[T]())
		if err != nil {
			return err
		}
		*normalRange = &iv
		return nil
	}
	return fmt.Errorf("%w: %s has no single attr %q", ErrUnknownAttribute, class, attr)
}

// writeReferenceRanges appends to the other_reference_ranges every
// DV_ORDERED class declares, the one multi-valued attribute it holds. The
// member is a REFERENCE_RANGE of any instantiation, given as a value or as
// a pointer. Every instantiation holds the same two attributes, a DV_TEXT
// meaning and a DV_INTERVAL<DV_ORDERED> range, so the member is copied
// into the instantiation the field holds, T. class names the Go type for
// the error detail.
func writeReferenceRanges[T rm.DVOrdered](class string, list *[]rm.ReferenceRange[T], attr string, child any) error {
	if attr != "other_reference_ranges" {
		return fmt.Errorf("%w: %s has no multiple attr %q", ErrUnknownAttribute, class, attr)
	}
	for _, read := range referenceRangeReaders {
		if read == nil {
			continue
		}
		if meaning, rng, ok := read(child); ok {
			*list = append(*list, rm.ReferenceRange[T]{Meaning: meaning, Range: rng})
			return nil
		}
	}
	return mismatch(attr, child, "REFERENCE_RANGE")
}

// referenceRangeReaders read the meaning and the range of a REFERENCE_RANGE
// of each instantiation the RM types hold.
var referenceRangeReaders = []func(any) (rm.DVTextLike, rm.DVInterval[rm.DVOrdered], bool){
	referenceRangeOf[rm.DVOrdered],
	referenceRangeOf[rm.DVCount],
	referenceRangeOf[rm.DVQuantity],
	referenceRangeOf[rm.DVProportion],
	referenceRangeOf[rm.DVOrdinal],
	referenceRangeOf[rm.DVScale],
	referenceRangeOf[rm.DVDate],
	referenceRangeOf[rm.DVTime],
	referenceRangeOf[rm.DVDateTime],
	referenceRangeOf[rm.DVDuration],
}

// referenceRangeOf returns the meaning and the range of child when it is a
// REFERENCE_RANGE over T, held by value or by a pointer that is not nil.
func referenceRangeOf[T rm.DVOrdered](child any) (rm.DVTextLike, rm.DVInterval[rm.DVOrdered], bool) {
	r, ok := coerceValueOrPtr[rm.ReferenceRange[T]](child)
	return r.Meaning, r.Range, ok
}

// writeReal stores a number on a mandatory Real attribute.
func writeReal(dst *rm.Real, attr string, child any) error {
	v, ok := rm.AsReal(child)
	if !ok {
		return mismatch(attr, child, "Real")
	}
	*dst = v
	return nil
}

// intervalParts is a DV_INTERVAL of any instantiation, read as its four
// flags and its two bounds. A bound is nil where its side is open, and an
// open side is one marked unbounded or one whose bound is Void (see
// isVoidBound), as writeIntervalSingle reads a Void bound. A bound of a
// concrete instantiation is a pointer to a copy of it, as the bare
// DV_INTERVAL the type registry decodes holds a bound; one of the
// DV_ORDERED instantiation is what that interval holds.
type intervalParts struct {
	lower, upper                   any
	lowerUnbounded, upperUnbounded bool
	lowerIncluded, upperIncluded   bool
}

// intervalReaders read the parts of a DV_INTERVAL of each instantiation the
// RM types hold.
var intervalReaders = []func(any) (intervalParts, bool){
	intervalPartsOf[rm.DVOrdered],
	intervalPartsOf[rm.DVCount],
	intervalPartsOf[rm.DVQuantity],
	intervalPartsOf[rm.DVProportion],
	intervalPartsOf[rm.DVOrdinal],
	intervalPartsOf[rm.DVScale],
	intervalPartsOf[rm.DVDate],
	intervalPartsOf[rm.DVTime],
	intervalPartsOf[rm.DVDateTime],
	intervalPartsOf[rm.DVDuration],
}

// intervalPartsOf returns the parts of child when it is a DV_INTERVAL over
// T, held by value or by a pointer that is not nil.
func intervalPartsOf[T rm.DVOrdered](child any) (intervalParts, bool) {
	iv, ok := coerceValueOrPtr[rm.DVInterval[T]](child)
	if !ok {
		return intervalParts{}, false
	}
	p := intervalParts{lowerIncluded: iv.LowerIncluded, upperIncluded: iv.UpperIncluded}
	p.lower, p.lowerUnbounded = boundPart(iv.Lower, iv.LowerUnbounded)
	p.upper, p.upperUnbounded = boundPart(iv.Upper, iv.UpperUnbounded)
	return p, true
}

// boundPart returns the bound b of a side, and whether the side is open:
// marked unbounded, or holding a Void bound. An open side gives a nil
// bound. A bound of a concrete type is returned as a pointer to a copy, so
// the parts share no value with the interval they are read from; the
// bound of a DV_ORDERED side is the pointer, or the value, it holds.
func boundPart[T rm.DVOrdered](b T, unbounded bool) (any, bool) {
	if unbounded || isVoidBound(b) {
		return nil, true
	}
	if held, ordered := any(&b).(*rm.DVOrdered); ordered {
		return *held, false
	}
	return &b, false
}

// intervalAs converts child, a DV_INTERVAL of any instantiation held by
// value or by pointer, into a DV_INTERVAL over T, the instantiation an RM
// field holds: the flags are copied, an open side takes no bound (nil
// where T is DV_ORDERED, the zero value of T otherwise), and each bound
// that is there must be a T. Into DV_ORDERED, a bound is stored as
// intervalParts gives it: by pointer where child is of a concrete
// instantiation. A child that is no DV_INTERVAL, and one with a bound of
// another type, return a mismatch naming rmName.
func intervalAs[T rm.DVOrdered](child any, attr, rmName string) (rm.DVInterval[T], error) {
	var parts intervalParts
	var ok bool
	for _, read := range intervalReaders {
		if read == nil {
			continue
		}
		if parts, ok = read(child); ok {
			break
		}
	}
	if !ok {
		return rm.DVInterval[T]{}, mismatch(attr, child, rmName)
	}
	iv := rm.DVInterval[T]{}
	iv.LowerUnbounded, iv.UpperUnbounded = parts.lowerUnbounded, parts.upperUnbounded
	iv.LowerIncluded, iv.UpperIncluded = parts.lowerIncluded, parts.upperIncluded
	if parts.lower != nil {
		if iv.Lower, ok = coerceValueOrPtr[T](parts.lower); !ok {
			return rm.DVInterval[T]{}, mismatch(attr, child, rmName)
		}
	}
	if parts.upper != nil {
		if iv.Upper, ok = coerceValueOrPtr[T](parts.upper); !ok {
			return rm.DVInterval[T]{}, mismatch(attr, child, rmName)
		}
	}
	return iv, nil
}

// intervalRMName is the RM name of a DV_INTERVAL over T, for the error
// detail: DV_INTERVAL<DV_COUNT> for a concrete T, DV_INTERVAL for
// DV_ORDERED.
func intervalRMName[T rm.DVOrdered]() string {
	var bound T
	if name, ok := rm.RMTypeName(bound); ok {
		return "DV_INTERVAL<" + name + ">"
	}
	return "DV_INTERVAL"
}
