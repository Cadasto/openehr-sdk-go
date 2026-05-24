package template

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// xmlNumericInterval is the AOM 1.4 numeric range shape used by
// C_INTEGER, C_REAL, and the magnitude / precision sub-intervals of
// C_DV_QUANTITY. Distinct from xmlInterval (which models
// existence / occurrences as integer cardinalities) — primitive
// ranges are float-typed and carry inclusivity flags.
type xmlNumericInterval struct {
	LowerIncluded  *bool   `xml:"lower_included"`
	UpperIncluded  *bool   `xml:"upper_included"`
	LowerUnbounded bool    `xml:"lower_unbounded"`
	UpperUnbounded bool    `xml:"upper_unbounded"`
	Lower          float64 `xml:"lower"`
	Upper          float64 `xml:"upper"`
}

// xmlPrimitiveListItem captures one <list> entry inside a primitive
// constraint payload. The same element name carries different
// payloads depending on the parent xsi:type — chardata for
// C_INTEGER / C_REAL / C_STRING, structured children for
// C_DV_QUANTITY / C_DV_ORDINAL. The dispatch reads only the fields
// relevant to the parent.
type xmlPrimitiveListItem struct {
	Text string `xml:",chardata"`

	// C_DV_QUANTITY children
	Magnitude *xmlNumericInterval `xml:"magnitude"`
	Precision *xmlNumericInterval `xml:"precision"`
	Units     string              `xml:"units"`

	// C_DV_ORDINAL children
	Value  *xmlIntValue      `xml:"value"`
	Symbol *xmlCodePhraseRef `xml:"symbol"`
}

// xmlIntValue wraps the simple <value>N</value> integer payload used
// inside C_DV_ORDINAL ordinal entries.
type xmlIntValue struct {
	Value int `xml:",chardata"`
}

// buildPrimitive returns the typed primitive constraint for the
// node's xsi:type, or nil when xsi:type is not a primitive in the
// REQ-103 closed set. The caller (buildNode) attaches the result to
// the resulting *ComplexObject for downstream Validate use.
//
// In strict mode (strict=true), builders that decode list-item text
// surface a parse failure on the first malformed entry rather than
// silently dropping it. Lenient mode preserves the forward-compatible
// behaviour: malformed list items are skipped so a partially-broken
// OPT still yields a usable (but weaker) constraint. The same split
// already exists at the node level for unknown xsi:type values.
func buildPrimitive(o *xmlCObject, strict bool) (constraints.PrimitiveConstraint, error) {
	switch o.Type {
	case "C_BOOLEAN":
		return buildBoolean(o), nil
	case "C_INTEGER":
		return buildInteger(o, strict)
	case "C_REAL":
		return buildReal(o, strict)
	case "C_STRING":
		return buildString(o), nil
	case "C_DATE":
		return constraints.CDate{Pattern: strings.TrimSpace(o.PrimitivePattern)}, nil
	case "C_TIME":
		return constraints.CTime{Pattern: strings.TrimSpace(o.PrimitivePattern)}, nil
	case "C_DATE_TIME":
		return constraints.CDateTime{Pattern: strings.TrimSpace(o.PrimitivePattern)}, nil
	case "C_DURATION":
		return buildDuration(o), nil
	case "C_CODE_PHRASE":
		return buildCodePhrase(o), nil
	case "C_DV_QUANTITY":
		return buildDvQuantity(o), nil
	case "C_DV_ORDINAL":
		return buildDvOrdinal(o), nil
	}
	return nil, nil
}

func buildBoolean(o *xmlCObject) constraints.CBoolean {
	// AOM 1.4 declares both true_valid and false_valid as mandatory on
	// C_BOOLEAN, with the invariant "Both attributes cannot be set to
	// False" (an unsatisfiable constraint). When the OPT XML omits one
	// or both elements, default each flag *independently* to true —
	// the safe direction, since the spec's only invariant forbids the
	// {false,false} case. A literal nil → false default on a single
	// omitted element would actively synthesise that forbidden case.
	c := constraints.CBoolean{
		TrueValid:  o.TrueValid == nil || *o.TrueValid,
		FalseValid: o.FalseValid == nil || *o.FalseValid,
	}
	if v, ok := parseBool(o.AssumedValue); ok {
		c.Default = &v
	}
	return c
}

func buildInteger(o *xmlCObject, strict bool) (constraints.CInteger, error) {
	c := constraints.CInteger{Range: numericRange(o.Range)}
	for i, item := range o.PrimitiveList {
		text := strings.TrimSpace(item.Text)
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			if strict {
				return c, fmt.Errorf("%w: C_INTEGER list[%d]=%q is not a valid integer", ErrInvalidOPT, i, text)
			}
			// Lenient mode: skip malformed entries so a partially-broken
			// OPT still yields a usable (weaker) constraint. Use
			// ParseOPTStrict to surface this as ErrInvalidOPT.
			continue
		}
		c.List = append(c.List, n)
	}
	if n, err := strconv.ParseInt(strings.TrimSpace(o.AssumedValue), 10, 64); err == nil {
		c.Default = &n
	}
	return c, nil
}

func buildReal(o *xmlCObject, strict bool) (constraints.CReal, error) {
	c := constraints.CReal{Range: numericRange(o.Range)}
	for i, item := range o.PrimitiveList {
		text := strings.TrimSpace(item.Text)
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			if strict {
				return c, fmt.Errorf("%w: C_REAL list[%d]=%q is not a valid real", ErrInvalidOPT, i, text)
			}
			// Lenient mode: see buildInteger for rationale.
			continue
		}
		c.List = append(c.List, f)
	}
	if f, err := strconv.ParseFloat(strings.TrimSpace(o.AssumedValue), 64); err == nil {
		c.Default = &f
	}
	return c, nil
}

func buildString(o *xmlCObject) constraints.CString {
	c := constraints.CString{
		Pattern: strings.TrimSpace(o.PrimitivePattern),
		Default: strings.TrimSpace(o.AssumedValue),
	}
	for _, item := range o.PrimitiveList {
		if s := strings.TrimSpace(item.Text); s != "" {
			c.List = append(c.List, s)
		}
	}
	return c
}

func buildDuration(o *xmlCObject) constraints.CDuration {
	return constraints.CDuration{
		Pattern: strings.TrimSpace(o.PrimitivePattern),
	}
}

func buildCodePhrase(o *xmlCObject) constraints.CodePhrase {
	c := constraints.CodePhrase{}
	if o.TerminologyID != nil {
		c.Terminology = strings.TrimSpace(o.TerminologyID.Value)
	}
	for _, code := range o.CodeList {
		if s := strings.TrimSpace(code); s != "" {
			c.CodeList = append(c.CodeList, s)
		}
	}
	return c
}

func buildDvQuantity(o *xmlCObject) constraints.DvQuantity {
	c := constraints.DvQuantity{}
	if o.Property != nil {
		ref := constraints.CodedTermRef{}
		if o.Property.TerminologyID != nil {
			ref.Terminology = strings.TrimSpace(o.Property.TerminologyID.Value)
		}
		ref.CodeString = strings.TrimSpace(o.Property.CodeString)
		if ref.Terminology != "" || ref.CodeString != "" {
			c.Property = &ref
		}
	}
	for _, item := range o.PrimitiveList {
		unit := constraints.QuantityUnit{
			Units:     strings.TrimSpace(item.Units),
			Magnitude: numericRange(item.Magnitude),
			Precision: numericRange(item.Precision),
		}
		if unit.Units == "" && !unit.Magnitude.IsBounded() && !unit.Precision.IsBounded() {
			continue
		}
		c.Units = append(c.Units, unit)
	}
	return c
}

func buildDvOrdinal(o *xmlCObject) constraints.CDvOrdinal {
	c := constraints.CDvOrdinal{}
	for _, item := range o.PrimitiveList {
		if item.Value == nil || item.Symbol == nil {
			continue
		}
		ref := constraints.CodedTermRef{CodeString: strings.TrimSpace(item.Symbol.CodeString)}
		if item.Symbol.TerminologyID != nil {
			ref.Terminology = strings.TrimSpace(item.Symbol.TerminologyID.Value)
		}
		c.Values = append(c.Values, constraints.OrdinalSymbol{
			Value:  item.Value.Value,
			Symbol: ref,
		})
	}
	return c
}

// numericRange folds a wire numeric interval into the public range
// shape, defaulting LowerIncluded / UpperIncluded to true when the
// OPT omits them (AOM convention).
func numericRange(i *xmlNumericInterval) constraints.NumericRange {
	if i == nil {
		return constraints.NumericRange{LowerUnbounded: true, UpperUnbounded: true}
	}
	r := constraints.NumericRange{
		Lower:          i.Lower,
		Upper:          i.Upper,
		LowerUnbounded: i.LowerUnbounded,
		UpperUnbounded: i.UpperUnbounded,
		LowerInclusive: i.LowerIncluded == nil || *i.LowerIncluded,
		UpperInclusive: i.UpperIncluded == nil || *i.UpperIncluded,
	}
	return r
}

// parseBool reads the AOM XSD-style boolean strings ("true" /
// "false") and the common "0" / "1" shorthand. Empty / unparseable
// input returns (false, false).
func parseBool(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	}
	return false, false
}
