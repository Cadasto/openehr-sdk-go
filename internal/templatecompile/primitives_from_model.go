package templatecompile

// primitives_from_model.go: the primitive constraint of a node, built from
// the primitive classes of the operational template model. The builders read
// the model as the deprecated parser reads an OPT, white space trimmed and
// blank entries left out, and refuse what its strict mode refuses where its
// default mode weakens the constraint instead.

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
// the one its C_PRIMITIVE_OBJECT item, C_CODE_PHRASE, C_DV_QUANTITY,
// C_DV_ORDINAL or CONSTRAINT_REF states, or nil for any other node, and for a
// C_PRIMITIVE_OBJECT with no item, which allows any value. A C_CODE_REFERENCE
// carries none yet.
//
// An error reports a value the constraint cannot carry: a temporal range
// bound that is not ISO 8601 text of its type, a timezone_validity that names
// no VALIDITY_KIND or is mandatory on a C_DATE, a CONSTRAINT_REF with no
// reference, an ordinal symbol that is neither a DV_CODED_TEXT nor a
// CODE_PHRASE, and an item that is no primitive class held by pointer. The
// caller wraps it in ErrInvalidInput with the node's place.
func primitiveFromModel(o aom14.CObject) (constraints.PrimitiveConstraint, error) {
	switch n := o.(type) {
	case *aom14.CPrimitiveObject:
		if n.Item == nil {
			return nil, nil
		}
		return primitiveItemFromModel(n.Item)
	case *aom14.CCodePhrase:
		return codePhraseFromModel(n), nil
	case *aom14.CDVQuantity:
		return dvQuantityFromModel(n), nil
	case *aom14.CDVOrdinal:
		c, err := dvOrdinalFromModel(n)
		if err != nil {
			return nil, err
		}
		return c, nil
	case *aom14.ConstraintRef:
		// A reference is required. The parser's default mode keeps a leaf
		// with an empty one; the compile step refuses it, as strict mode
		// does.
		ref := strings.TrimSpace(n.Reference)
		if ref == "" {
			return nil, fmt.Errorf("CONSTRAINT_REF for %q has no reference", strings.TrimSpace(n.RMTypeName))
		}
		return constraints.ConstraintRef{Reference: ref}, nil
	}
	return nil, nil
}

// primitiveItemFromModel returns the constraint the item of a
// C_PRIMITIVE_OBJECT states.
func primitiveItemFromModel(item aom14.CPrimitive) (constraints.PrimitiveConstraint, error) {
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
		r, tz, err := clockPartsFromModel(p.Range, p.TimezoneValidity, "C_DATE", constraints.CDate{}.Validate)
		if err != nil {
			return nil, err
		}
		return constraints.CDate{Pattern: trimmedOrEmpty(p.Pattern), Range: r, TimezoneValidity: tz}, nil
	case *aom14.CTime:
		r, tz, err := clockPartsFromModel(p.Range, p.TimezoneValidity, "C_TIME", constraints.CTime{}.Validate)
		if err != nil {
			return nil, err
		}
		return constraints.CTime{Pattern: trimmedOrEmpty(p.Pattern), Range: r, TimezoneValidity: tz}, nil
	case *aom14.CDateTime:
		r, tz, err := clockPartsFromModel(p.Range, p.TimezoneValidity, "C_DATE_TIME", constraints.CDateTime{}.Validate)
		if err != nil {
			return nil, err
		}
		return constraints.CDateTime{Pattern: trimmedOrEmpty(p.Pattern), Range: r, TimezoneValidity: tz}, nil
	case *aom14.CDuration:
		r, err := temporalRangeFromModel(p.Range, "C_DURATION", constraints.CDuration{}.Validate)
		if err != nil {
			return nil, err
		}
		return constraints.CDuration{Pattern: trimmedOrEmpty(p.Pattern), Range: r}, nil
	}
	return nil, fmt.Errorf("item %T of C_PRIMITIVE_OBJECT is not a primitive class held by pointer", item)
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
// which admits exactly the ISO 8601 text of the type. A bound it refuses is an
// error: the parser's default mode reads that side as unbounded, and the
// compile step refuses it, as strict mode does.
func temporalRangeFromModel(iv *rm.Interval[string], class string, validate func(any) []constraints.Violation) (constraints.TemporalRange, error) {
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
		if len(validate(lower)) != 0 {
			return constraints.TemporalRange{}, fmt.Errorf("%s range: lower bound %q is not ISO 8601 text of the type", class, lower)
		}
		r.Lower = lower
	}
	if !r.UpperUnbounded {
		upper := strings.TrimSpace(iv.Upper)
		if len(validate(upper)) != 0 {
			return constraints.TemporalRange{}, fmt.Errorf("%s range: upper bound %q is not ISO 8601 text of the type", class, upper)
		}
		r.Upper = upper
	}
	return r, nil
}

// clockPartsFromModel returns the range and the timezone validity of a
// C_DATE, C_TIME or C_DATE_TIME named class.
func clockPartsFromModel(iv *rm.Interval[string], tz *aom14.TimezoneValidityKind, class string, validate func(any) []constraints.Violation) (constraints.TemporalRange, constraints.ValidityKind, error) {
	r, err := temporalRangeFromModel(iv, class, validate)
	if err != nil {
		return r, constraints.ValidityOptional, err
	}
	v, err := timezoneValidityFromModel(tz, class)
	return r, v, err
}

// timezoneValidityFromModel returns the timezone validity of a C_DATE, C_TIME
// or C_DATE_TIME named class: optional when the model gives none. The parser's
// default mode reads a mandatory one on a C_DATE as optional; the compile
// step refuses it, as strict mode does, and refuses a kind that names no
// VALIDITY_KIND, which only a model built in code can hold.
func timezoneValidityFromModel(tz *aom14.TimezoneValidityKind, class string) (constraints.ValidityKind, error) {
	if tz == nil {
		return constraints.ValidityOptional, nil
	}
	switch *tz {
	case aom14.TimezoneValidityKindMandatory:
		if class == "C_DATE" {
			return constraints.ValidityOptional, errors.New("C_DATE timezone_validity is mandatory, which no date meets: a date has no timezone")
		}
		return constraints.ValidityMandatory, nil
	case aom14.TimezoneValidityKindOptional:
		return constraints.ValidityOptional, nil
	case aom14.TimezoneValidityKindDisallowed: // the code of prohibited too
		return constraints.ValidityDisallowed, nil
	}
	return constraints.ValidityOptional, fmt.Errorf("%s timezone_validity %d is no VALIDITY_KIND", class, int32(*tz))
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
