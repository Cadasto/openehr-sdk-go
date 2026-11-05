package templatecompile

// primitives_from_model.go: the primitive constraint of a node, built from
// the primitive classes of the operational template model. The builders read
// the model with white space trimmed and blank entries left out. Where a
// value cannot be held as written, they read it in a weaker form, and when
// strict they refuse it.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
)

// primitiveFromModel returns the primitive constraint of the model node o:
// the one its C_PRIMITIVE_OBJECT item, C_CODE_PHRASE, C_CODE_REFERENCE,
// C_DV_QUANTITY, C_DV_ORDINAL or CONSTRAINT_REF states, or nil for any other
// node. A C_CODE_REFERENCE's code phrase is read as a C_CODE_PHRASE's.
//
// Unless strict, it reads a value the constraint cannot carry in a weaker
// form: a C_PRIMITIVE_OBJECT with no item, or with a nil pointer for one, as
// no constraint, which allows any value; a CONSTRAINT_REF with no reference
// as one with an empty reference; a temporal range bound that is not ISO 8601
// text of its type as unbounded on its side; and a timezone_validity that
// names no VALIDITY_KIND, or is mandatory on a C_DATE, as optional. When
// strict, each is an error.
//
// In both modes an error reports an ordinal symbol that is neither a
// DV_CODED_TEXT nor a CODE_PHRASE and an item that is no primitive class held
// by pointer, which only a model built in code can hold. The caller wraps an
// error in ErrInvalidInput with the node's place.
func primitiveFromModel(o aom14.CObject, strict bool) (constraints.PrimitiveConstraint, error) {
	switch n := o.(type) {
	case *aom14.CPrimitiveObject:
		if n.Item == nil || isNilPrimitive(n.Item) {
			if strict {
				return nil, fmt.Errorf("C_PRIMITIVE_OBJECT for %q has no item", strings.TrimSpace(n.RMTypeName))
			}
			return nil, nil
		}
		return primitiveItemFromModel(n.Item, strict)
	case *aom14.CCodePhrase:
		return codePhraseFromModel(n), nil
	case *aom14.CCodeReference:
		return constraints.CodeReference{
			CodePhrase:      codePhraseFromModel(&n.CCodePhrase),
			ReferenceSetURI: strings.TrimSpace(n.ReferenceSetURI),
		}, nil
	case *aom14.CDVQuantity:
		return dvQuantityFromModel(n), nil
	case *aom14.CDVOrdinal:
		c, err := dvOrdinalFromModel(n)
		if err != nil {
			return nil, err
		}
		return c, nil
	case *aom14.ConstraintRef:
		// A reference is required. The default mode keeps a leaf with an
		// empty one, and the strict mode refuses it.
		ref := strings.TrimSpace(n.Reference)
		if ref == "" && strict {
			return nil, fmt.Errorf("CONSTRAINT_REF for %q has no reference", strings.TrimSpace(n.RMTypeName))
		}
		return constraints.ConstraintRef{Reference: ref}, nil
	}
	return nil, nil
}

// primitiveItemFromModel returns the constraint the item of a
// C_PRIMITIVE_OBJECT states. item is neither nil nor a nil pointer, and
// strict is as for [primitiveFromModel].
func primitiveItemFromModel(item aom14.CPrimitive, strict bool) (constraints.PrimitiveConstraint, error) {
	switch p := item.(type) {
	case *aom14.CBoolean:
		c := constraints.CBoolean{TrueValid: p.TrueValid, FalseValid: p.FalseValid}
		if p.AssumedValue != nil {
			c.Default = new(*p.AssumedValue)
		}
		return c, nil
	case *aom14.CInteger:
		c := constraints.CInteger{Range: numericRangeFromModel(p.Range)}
		for _, n := range p.List {
			c.List = append(c.List, int64(n))
		}
		if p.AssumedValue != nil {
			c.Default = new(int64(*p.AssumedValue))
		}
		return c, nil
	case *aom14.CReal:
		c := constraints.CReal{Range: numericRangeFromModel(p.Range)}
		for _, f := range p.List {
			c.List = append(c.List, float64(f))
		}
		if p.AssumedValue != nil {
			c.Default = new(float64(*p.AssumedValue))
		}
		return c, nil
	case *aom14.CString:
		var list []string
		for _, s := range p.List {
			if s = strings.TrimSpace(s); s != "" {
				list = append(list, s)
			}
		}
		c := constraints.NewCString(trimmedOrEmpty(p.Pattern), list, trimmedOrEmpty(p.AssumedValue))
		// The ITS-XML AM 1.4 schema makes list_open optional with no
		// default, so an absent one leaves the list closed.
		c.ListOpen = p.ListOpen != nil && *p.ListOpen
		return c, nil
	case *aom14.CDate:
		r, tz, err := clockPartsFromModel(p.Range, p.TimezoneValidity, "C_DATE", constraints.CDate{}.Validate, strict)
		if err != nil {
			return nil, err
		}
		return constraints.CDate{Pattern: trimmedOrEmpty(p.Pattern), Range: r, TimezoneValidity: tz}, nil
	case *aom14.CTime:
		r, tz, err := clockPartsFromModel(p.Range, p.TimezoneValidity, "C_TIME", constraints.CTime{}.Validate, strict)
		if err != nil {
			return nil, err
		}
		return constraints.CTime{Pattern: trimmedOrEmpty(p.Pattern), Range: r, TimezoneValidity: tz}, nil
	case *aom14.CDateTime:
		r, tz, err := clockPartsFromModel(p.Range, p.TimezoneValidity, "C_DATE_TIME", constraints.CDateTime{}.Validate, strict)
		if err != nil {
			return nil, err
		}
		return constraints.CDateTime{Pattern: trimmedOrEmpty(p.Pattern), Range: r, TimezoneValidity: tz}, nil
	case *aom14.CDuration:
		r, err := temporalRangeFromModel(p.Range, "C_DURATION", constraints.CDuration{}.Validate, strict)
		if err != nil {
			return nil, err
		}
		return constraints.CDuration{Pattern: trimmedOrEmpty(p.Pattern), Range: r}, nil
	}
	return nil, fmt.Errorf("item %T of C_PRIMITIVE_OBJECT is not a primitive class held by pointer", item)
}

// isNilPrimitive reports whether item is a nil pointer to one of the
// primitive classes [primitiveItemFromModel] reads.
func isNilPrimitive(item aom14.CPrimitive) bool {
	switch p := item.(type) {
	case *aom14.CBoolean:
		return p == nil
	case *aom14.CInteger:
		return p == nil
	case *aom14.CReal:
		return p == nil
	case *aom14.CString:
		return p == nil
	case *aom14.CDate:
		return p == nil
	case *aom14.CTime:
		return p == nil
	case *aom14.CDateTime:
		return p == nil
	case *aom14.CDuration:
		return p == nil
	}
	return false
}

// numericRangeFromModel returns the range of a C_INTEGER, a C_REAL or a
// C_DV_QUANTITY item, unbounded on both sides when the model gives none. The
// model's decode has already given each flag the OPT leaves out its default.
// A bound on an unbounded side is not kept.
func numericRangeFromModel[T aom14.Integer | aom14.Real](iv *rm.Interval[T]) constraints.NumericRange {
	if iv == nil {
		return constraints.NumericRange{LowerUnbounded: true, UpperUnbounded: true}
	}
	r := constraints.NumericRange{
		LowerUnbounded: iv.LowerUnbounded,
		UpperUnbounded: iv.UpperUnbounded,
		LowerInclusive: iv.LowerIncluded,
		UpperInclusive: iv.UpperIncluded,
	}
	if !r.LowerUnbounded {
		r.Lower = float64(iv.Lower)
	}
	if !r.UpperUnbounded {
		r.Upper = float64(iv.Upper)
	}
	return r
}

// temporalRangeFromModel returns the range of a C_DATE, C_TIME, C_DATE_TIME
// or C_DURATION named class, or the zero range, which constrains nothing,
// when the model gives none. validate is the zero constraint of the type,
// which admits exactly the ISO 8601 text of the type. A bound it refuses
// leaves its side unbounded, and not inclusive, or, when strict, is an error.
func temporalRangeFromModel(iv *rm.Interval[string], class string, validate func(any) []constraints.Violation, strict bool) (constraints.TemporalRange, error) {
	if iv == nil {
		return constraints.TemporalRange{}, nil
	}
	r := constraints.TemporalRange{
		LowerUnbounded: iv.LowerUnbounded,
		UpperUnbounded: iv.UpperUnbounded,
		LowerInclusive: iv.LowerIncluded,
		UpperInclusive: iv.UpperIncluded,
	}
	if !r.LowerUnbounded {
		lower := strings.TrimSpace(iv.Lower)
		switch {
		case len(validate(lower)) == 0:
			r.Lower = lower
		case strict:
			return constraints.TemporalRange{}, fmt.Errorf("%s range: lower bound %q is not ISO 8601 text of the type", class, lower)
		default:
			r.LowerUnbounded, r.LowerInclusive = true, false
		}
	}
	if !r.UpperUnbounded {
		upper := strings.TrimSpace(iv.Upper)
		switch {
		case len(validate(upper)) == 0:
			r.Upper = upper
		case strict:
			return constraints.TemporalRange{}, fmt.Errorf("%s range: upper bound %q is not ISO 8601 text of the type", class, upper)
		default:
			r.UpperUnbounded, r.UpperInclusive = true, false
		}
	}
	return r, nil
}

// clockPartsFromModel returns the range and the timezone validity of a
// C_DATE, C_TIME or C_DATE_TIME named class, as [temporalRangeFromModel] and
// [timezoneValidityFromModel] read them.
func clockPartsFromModel(iv *rm.Interval[string], tz *aom14.TimezoneValidityKind, class string, validate func(any) []constraints.Violation, strict bool) (constraints.TemporalRange, constraints.ValidityKind, error) {
	r, err := temporalRangeFromModel(iv, class, validate, strict)
	if err != nil {
		return r, constraints.ValidityOptional, err
	}
	v, err := timezoneValidityFromModel(tz, class, strict)
	return r, v, err
}

// timezoneValidityFromModel returns the timezone validity of a C_DATE, C_TIME
// or C_DATE_TIME named class: optional when the model gives none. A mandatory
// one on a C_DATE, which no date meets, and a kind that names no
// VALIDITY_KIND, which only a model built in code can hold, read as optional,
// or, when strict, are an error.
func timezoneValidityFromModel(tz *aom14.TimezoneValidityKind, class string, strict bool) (constraints.ValidityKind, error) {
	if tz == nil {
		return constraints.ValidityOptional, nil
	}
	switch *tz {
	case aom14.TimezoneValidityKindMandatory:
		if class != "C_DATE" {
			return constraints.ValidityMandatory, nil
		}
		if strict {
			return constraints.ValidityOptional, errors.New("C_DATE timezone_validity is mandatory, which no date meets: a date has no timezone")
		}
		return constraints.ValidityOptional, nil
	case aom14.TimezoneValidityKindOptional:
		return constraints.ValidityOptional, nil
	case aom14.TimezoneValidityKindDisallowed: // the code of prohibited too
		return constraints.ValidityDisallowed, nil
	}
	if strict {
		return constraints.ValidityOptional, fmt.Errorf("%s timezone_validity %d is no VALIDITY_KIND", class, int32(*tz))
	}
	return constraints.ValidityOptional, nil
}

// codePhraseFromModel returns the constraint of a C_CODE_PHRASE.
func codePhraseFromModel(p *aom14.CCodePhrase) constraints.CodePhrase {
	var c constraints.CodePhrase
	if p.TerminologyID != nil {
		c.Terminology = strings.TrimSpace(p.TerminologyID.Value)
	}
	if p.CodeList != nil {
		for _, code := range *p.CodeList {
			if s := strings.TrimSpace(code); s != "" {
				c.CodeList = append(c.CodeList, s)
			}
		}
	}
	return c
}

// dvQuantityFromModel returns the constraint of a C_DV_QUANTITY: its property,
// with the UCUM units the property-unit table lists for an openehr property,
// and each item that carries units or a bounded range.
func dvQuantityFromModel(q *aom14.CDVQuantity) constraints.DvQuantity {
	var c constraints.DvQuantity
	if q.Property != nil {
		ref := codedTermRef(*q.Property)
		if ref.Terminology != "" || ref.CodeString != "" {
			c.Property = &ref
		}
		if ref.Terminology == terminology.ID {
			c.PropertyUnits = propertyUnits(ref.CodeString)
		}
	}
	for _, item := range q.List {
		unit := constraints.QuantityUnit{
			Units:     strings.TrimSpace(item.Units),
			Magnitude: numericRangeFromModel(item.Magnitude),
			Precision: numericRangeFromModel(item.Precision),
		}
		if unit.Units == "" && !unit.Magnitude.IsBounded() && !unit.Precision.IsBounded() {
			continue
		}
		c.Units = append(c.Units, unit)
	}
	return c
}

// propertyUnits returns the UCUM units the pinned property-unit table lists
// for the openehr property code, its primary units first and each part in
// source order, or nil when the table lists the code with no units or not at
// all. The slice is the caller's own.
func propertyUnits(code string) []string {
	p, ok := terminology.QuantityPropertyByCode(code)
	if !ok || p.Len() == 0 {
		return nil
	}
	out := make([]string, 0, p.Len())
	for u := range p.All() {
		if u.Primary {
			out = append(out, u.UCUM)
		}
	}
	for u := range p.All() {
		if !u.Primary {
			out = append(out, u.UCUM)
		}
	}
	return out
}

// dvOrdinalFromModel returns the constraint of a C_DV_ORDINAL: each listed
// ordinal that carries a symbol, as its value and its symbol's code.
func dvOrdinalFromModel(o *aom14.CDVOrdinal) (constraints.CDvOrdinal, error) {
	var c constraints.CDvOrdinal
	for i, item := range o.List {
		var code rm.CodePhrase
		switch s := item.Symbol.(type) {
		case nil:
			continue
		case *rm.DVCodedText:
			if s == nil {
				continue
			}
			code = s.DefiningCode
		case rm.DVCodedText:
			code = s.DefiningCode
		case *rm.CodePhrase:
			if s == nil {
				continue
			}
			code = *s
		case rm.CodePhrase:
			code = s
		default:
			return constraints.CDvOrdinal{}, fmt.Errorf("C_DV_ORDINAL list[%d]: symbol %T is neither a DV_CODED_TEXT nor a CODE_PHRASE", i, item.Symbol)
		}
		c.Values = append(c.Values, constraints.OrdinalSymbol{Value: int(item.Value), Symbol: codedTermRef(code)})
	}
	return c, nil
}

// codedTermRef returns the code phrase's terminology and code, trimmed.
func codedTermRef(code rm.CodePhrase) constraints.CodedTermRef {
	return constraints.CodedTermRef{
		Terminology: strings.TrimSpace(code.TerminologyID.Value),
		CodeString:  strings.TrimSpace(code.CodeString),
	}
}

// trimmedOrEmpty returns the text s points to, trimmed, or "" when s is nil.
func trimmedOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}
