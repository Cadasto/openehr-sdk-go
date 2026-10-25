package template

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
)

// xmlNumericInterval is the AOM 1.4 numeric range shape used by
// C_INTEGER, C_REAL, and the magnitude / precision sub-intervals of
// C_DV_QUANTITY. Distinct from xmlInterval (which models
// existence / occurrences as integer cardinalities) — primitive
// ranges are float-typed and keep their inclusivity flags, where a
// Multiplicity closes an excluded end instead. All four flags are
// xmlBoolFlag values, so an omitted, empty or whitespace-only element
// stays unset and is never a decode error. unboundedFlag reads an unset
// unbounded flag by its side's bound, and includedFlag gives an unset
// included flag the default for its side.
type xmlNumericInterval struct {
	LowerIncluded  xmlBoolFlag `xml:"lower_included"`
	UpperIncluded  xmlBoolFlag `xml:"upper_included"`
	LowerUnbounded xmlBoolFlag `xml:"lower_unbounded"`
	UpperUnbounded xmlBoolFlag `xml:"upper_unbounded"`
	// Lower / Upper are decoded as string and parsed to float lazily
	// in numericRange. AOM 1.4 reuses the same `<range>` element name
	// for both numeric ranges (C_INTEGER / C_REAL / C_DV_QUANTITY)
	// and temporal ranges (C_DURATION / C_DATE / etc.); the latter
	// carry ISO 8601 strings (e.g. `PT0S`) that strconv.ParseFloat
	// cannot read. Lazy parse keeps decode infallible regardless of
	// which xsi:type owns the range. A nil pointer is a bound the OPT
	// omits.
	Lower *string `xml:"lower"`
	Upper *string `xml:"upper"`
}

// xmlBoolFlag is the true_valid or false_valid element of a C_BOOLEAN,
// the list_open element of a C_STRING,
// or one of the four boundary flags (lower_included, upper_included,
// lower_unbounded, upper_unbounded) of an interval: an occurrences,
// existence or cardinality interval, or a numeric range.
// A plain bool field would read an empty element as false. This type
// leaves an empty or whitespace-only element unset instead, so it reads
// exactly like an omitted one. Any other text is parsed with
// strconv.ParseBool, as a bool field would be, and text that is not a
// boolean still fails the parse.
type xmlBoolFlag struct {
	set   bool
	value bool
}

// UnmarshalText reads the element's text into the flag. encoding/xml
// passes it the element's own character data, without that of any
// child element.
func (f *xmlBoolFlag) UnmarshalText(b []byte) error {
	text := bytes.TrimSpace(b)
	if len(text) == 0 {
		return nil
	}
	v, err := strconv.ParseBool(string(text))
	if err != nil {
		return err
	}
	*f = xmlBoolFlag{set: true, value: v}
	return nil
}

// orTrue returns the flag's value, or true when the element was omitted
// or empty.
func (f xmlBoolFlag) orTrue() bool {
	return !f.set || f.value
}

// orFalse returns the flag's value, or false when the element was omitted
// or empty.
func (f xmlBoolFlag) orFalse() bool {
	return f.set && f.value
}

// ptr returns a pointer to the flag's value, or nil when the element was
// omitted or empty.
func (f xmlBoolFlag) ptr() *bool {
	if !f.set {
		return nil
	}
	return &f.value
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
	Symbol *xmlOrdinalSymbol `xml:"symbol"`
}

// xmlOrdinalSymbol is the <symbol> of a C_DV_ORDINAL list entry. Per the
// AOM 1.4 XSD the symbol is a DV_CODED_TEXT whose code sits under
// <defining_code> (the form EHRbase and template designers emit); the
// inline code_string/terminology_id form is accepted leniently.
type xmlOrdinalSymbol struct {
	DefiningCode *xmlCodePhraseRef `xml:"defining_code"`
	xmlCodePhraseRef
}

// ref resolves the symbol's code phrase, preferring the XSD-conformant
// defining_code child over the inline lenient form.
func (s *xmlOrdinalSymbol) ref() *xmlCodePhraseRef {
	if s.DefiningCode != nil {
		return s.DefiningCode
	}
	return &s.xmlCodePhraseRef
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
		return buildDvQuantity(o)
	case "C_DV_ORDINAL":
		return buildDvOrdinal(o), nil
	case "CONSTRAINT_REF":
		return buildConstraintRef(o, strict)
	case "C_PRIMITIVE_OBJECT":
		// The wrapper carries the AOM 1.4 primitive short name on
		// `rm_type_name` (DURATION / DATE / BOOLEAN / etc.); the
		// inner `<item xsi:type="C_*">` carries the actual typed
		// constraint. Recurse on Item — when missing under strict
		// mode, surface as a malformed-OPT error rather than
		// silently dropping the leaf constraint.
		if o.Item == nil {
			if strict {
				return nil, fmt.Errorf("%w: C_PRIMITIVE_OBJECT for %q has no <item> child", ErrInvalidOPT, o.RMTypeName)
			}
			return nil, nil
		}
		return buildPrimitive(o.Item, strict)
	}
	return nil, nil
}

func buildBoolean(o *xmlCObject) constraints.CBoolean {
	// AOM 1.4 declares both true_valid and false_valid as mandatory on
	// C_BOOLEAN, with the invariant "Both attributes cannot be set to
	// False" (an unsatisfiable constraint). When the OPT XML omits one
	// or both elements, or leaves one empty, default each flag
	// *independently* to true. That is the safe direction, since the
	// spec's only invariant forbids the {false,false} case. Reading a
	// missing or empty element as false would synthesise that case.
	c := constraints.CBoolean{
		TrueValid:  o.TrueValid.orTrue(),
		FalseValid: o.FalseValid.orTrue(),
	}
	if v, ok := parseBool(o.AssumedValue); ok {
		c.Default = &v
	}
	return c
}

func buildInteger(o *xmlCObject, strict bool) (constraints.CInteger, error) {
	rng, err := numericRange(o.Range, strict, "C_INTEGER")
	if err != nil {
		return constraints.CInteger{}, err
	}
	c := constraints.CInteger{Range: rng}
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
	rng, err := numericRange(o.Range, strict, "C_REAL")
	if err != nil {
		return constraints.CReal{}, err
	}
	c := constraints.CReal{Range: rng}
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
	pattern := strings.TrimSpace(o.PrimitivePattern)
	assumed := strings.TrimSpace(o.AssumedValue)
	var list []string
	for _, item := range o.PrimitiveList {
		if s := strings.TrimSpace(item.Text); s != "" {
			list = append(list, s)
		}
	}
	c := constraints.NewCString(pattern, list, assumed)
	// The ITS-XML AM 1.4 schema makes list_open optional with no default,
	// so an omitted or empty element leaves the list closed.
	c.ListOpen = o.ListOpen.orFalse()
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

// propertyUnits returns the UCUM units the pinned property-unit table lists
// for the openehr property code, its primary units first and each part in
// source order (REQ-103), or nil when the table lists the code with no units
// or not at all. The slice is the caller's own.
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

func buildDvQuantity(o *xmlCObject) (constraints.DvQuantity, error) {
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
		if ref.Terminology == terminology.ID {
			c.PropertyUnits = propertyUnits(ref.CodeString)
		}
	}
	for _, item := range o.PrimitiveList {
		// DV_QUANTITY stays lenient on magnitude / precision bounds —
		// scope of the strict numericRange path is C_INTEGER / C_REAL
		// (the primary numeric scalar types). Tightening DV_QUANTITY
		// would require threading strict through buildDvQuantity.
		// In lenient mode numericRange fails only on a side written bounded
		// with no bound, which every mode refuses (REQ-169).
		mag, err := numericRange(item.Magnitude, false, "DV_QUANTITY.magnitude")
		if err != nil {
			return c, err
		}
		prec, err := numericRange(item.Precision, false, "DV_QUANTITY.precision")
		if err != nil {
			return c, err
		}
		unit := constraints.QuantityUnit{
			Units:     strings.TrimSpace(item.Units),
			Magnitude: mag,
			Precision: prec,
		}
		if unit.Units == "" && !unit.Magnitude.IsBounded() && !unit.Precision.IsBounded() {
			continue
		}
		c.Units = append(c.Units, unit)
	}
	return c, nil
}

func buildDvOrdinal(o *xmlCObject) constraints.CDvOrdinal {
	c := constraints.CDvOrdinal{}
	for _, item := range o.PrimitiveList {
		if item.Value == nil || item.Symbol == nil {
			continue
		}
		sym := item.Symbol.ref()
		ref := constraints.CodedTermRef{CodeString: strings.TrimSpace(sym.CodeString)}
		if sym.TerminologyID != nil {
			ref.Terminology = strings.TrimSpace(sym.TerminologyID.Value)
		}
		c.Values = append(c.Values, constraints.OrdinalSymbol{
			Value:  item.Value.Value,
			Symbol: ref,
		})
	}
	return c
}

// buildConstraintRef reads a CONSTRAINT_REF: the reference (an ac code)
// that names a value set outside the archetype. The node's rm_type_name,
// which the caller keeps on the leaf, is the RM type the value set
// constrains. A reference is required, so a missing or blank one is
// refused in strict mode; lenient mode keeps the leaf with an empty
// reference, as it keeps a partly broken primitive with a weaker
// constraint.
func buildConstraintRef(o *xmlCObject, strict bool) (constraints.ConstraintRef, error) {
	ref := strings.TrimSpace(o.Reference)
	if ref == "" && strict {
		return constraints.ConstraintRef{}, fmt.Errorf("%w: CONSTRAINT_REF for %q has no <reference>", ErrInvalidOPT, o.RMTypeName)
	}
	return constraints.ConstraintRef{Reference: ref}, nil
}

// numericRange folds a wire numeric interval into the public range
// shape. A LowerIncluded / UpperIncluded flag the OPT omits or leaves
// empty defaults by its side's unbounded flag as written: true on a
// bounded side, false on an unbounded one. xmlNumericInterval.Lower /
// Upper are decoded as strings (AOM 1.4 reuses `<range>` for both numeric
// and temporal types); parsed to float64 lazily here.
//
// In lenient mode an unparseable bound falls through as the unbounded
// sentinel on that side (forward-compat, mirrors the list-item path).
// In strict mode an unparseable bound surfaces as ErrInvalidOPT —
// the same split buildInteger / buildReal already apply to list
// items. ctx names the constraint type for the diagnostic.
func numericRange(i *xmlNumericInterval, strict bool, ctx string) (constraints.NumericRange, error) {
	if i == nil {
		return constraints.NumericRange{LowerUnbounded: true, UpperUnbounded: true}, nil
	}
	lowerUnbounded, err := unboundedFlag(i.LowerUnbounded, i.Lower != nil, "lower")
	if err != nil {
		return constraints.NumericRange{}, fmt.Errorf("%w: %s range: %w", ErrInvalidOPT, ctx, err)
	}
	upperUnbounded, err := unboundedFlag(i.UpperUnbounded, i.Upper != nil, "upper")
	if err != nil {
		return constraints.NumericRange{}, fmt.Errorf("%w: %s range: %w", ErrInvalidOPT, ctx, err)
	}
	lower, upper := stringOrEmpty(i.Lower), stringOrEmpty(i.Upper)
	r := constraints.NumericRange{
		LowerUnbounded: lowerUnbounded,
		UpperUnbounded: upperUnbounded,
		LowerInclusive: includedFlag(i.LowerIncluded.ptr(), lowerUnbounded),
		UpperInclusive: includedFlag(i.UpperIncluded.ptr(), upperUnbounded),
	}
	if !r.LowerUnbounded {
		if f, err := strconv.ParseFloat(strings.TrimSpace(lower), 64); err == nil {
			r.Lower = f
		} else if strict {
			return r, fmt.Errorf("%w: %s range.lower=%q is not a valid number", ErrInvalidOPT, ctx, lower)
		} else {
			r.LowerUnbounded = true
		}
	}
	if !r.UpperUnbounded {
		if f, err := strconv.ParseFloat(strings.TrimSpace(upper), 64); err == nil {
			r.Upper = f
		} else if strict {
			return r, fmt.Errorf("%w: %s range.upper=%q is not a valid number", ErrInvalidOPT, ctx, upper)
		} else {
			r.UpperUnbounded = true
		}
	}
	return r, nil
}

// stringOrEmpty returns the bound p points to, or "" for a bound the OPT
// omits.
func stringOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
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
