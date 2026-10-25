package constraints

import (
	"fmt"
	"slices"
)

// QuantityValue is the runtime value shape accepted by
// [DvQuantity.Validate] and [CDvOrdinal.Validate]. Construct it directly.
// It is kept minimal so callers need not import the rm package to
// validate a single value.
type QuantityValue struct {
	Magnitude float64
	Units     string
	Precision int // -1 when unknown
}

// QuantityUnit is one allowed (units, magnitude-range, precision-range)
// triple from a DV_QUANTITY constraint <list> entry. UCUM-canonical
// units strings (e.g. "mm[Hg]", "kg/m2") are the openEHR convention,
// but the constraint stores them verbatim.
type QuantityUnit struct {
	Units     string
	Magnitude NumericRange
	Precision NumericRange
}

// DvQuantity constrains an RM DV_QUANTITY value (C_DV_QUANTITY).
// Units enumerates the allowed (units, range) combinations; the
// value must match one of them on units and lie inside that entry's
// magnitude range. Property is the optional binding of the physical
// property the quantity measures, such as openehr::125 (pressure).
//
// PropertyUnits lists the UCUM units that property admits, its primary
// units first and each part in source order. The OPT parser fills it from
// the openEHR property-unit table (TERM PropertyUnitData.xml, exposed by
// openehr/terminology) when Property is an openehr code the table lists,
// and leaves it empty otherwise. [DvQuantity.Validate] checks the units
// against it only when Units is empty: a Units list takes precedence, so a
// listed unit the property does not admit is accepted.
type DvQuantity struct {
	Units         []QuantityUnit
	Property      *CodedTermRef
	PropertyUnits []string
}

func (DvQuantity) isPrimitive() {}

// ExampleValue returns a minimal-valid [QuantityValue].
// An entry of Units gives the example its units, copied verbatim, and a
// magnitude from the entry's range (lower bound when set; midpoint or zero
// otherwise). The example is that of the first entry whose range admits
// that magnitude, or the first entry's when none does, as when every range
// is inverted. With no Units, the example is magnitude 0 in the first
// entry of PropertyUnits, a primary unit of the property when it has one.
// It falls back to QuantityValue{0, "1"} when the constraint is open-ended
// (any units / any magnitude).
func (c DvQuantity) ExampleValue() any {
	if len(c.Units) == 0 {
		if len(c.PropertyUnits) > 0 {
			return QuantityValue{Magnitude: 0, Units: c.PropertyUnits[0], Precision: -1}
		}
		return QuantityValue{Magnitude: 0, Units: "1", Precision: -1}
	}
	for _, u := range c.Units {
		if q := u.example(); len(u.violations(q)) == 0 {
			return q
		}
	}
	return c.Units[0].example()
}

// example returns the quantity in u's units whose magnitude
// [exampleMagnitude] picks from u's range, its precision unknown.
func (u QuantityUnit) example() QuantityValue {
	return QuantityValue{Magnitude: exampleMagnitude(u.Magnitude), Units: u.Units, Precision: -1}
}

// exampleMagnitude picks an in-range float from r, for a quantity
// unit's magnitude and for [CReal.ExampleValue]: the lower bound nudged
// inside an exclusive end, else the upper bound likewise, else 0.
func exampleMagnitude(r NumericRange) float64 {
	if !r.IsBounded() {
		return 0
	}
	if !r.LowerUnbounded {
		f := r.Lower
		if !r.LowerInclusive {
			if !r.UpperUnbounded {
				f = (r.Lower + r.Upper) / 2
			} else {
				f = r.Lower + 1
			}
		}
		return f
	}
	if !r.UpperUnbounded {
		f := r.Upper
		if !r.UpperInclusive {
			f = r.Upper - 1
		}
		return f
	}
	return 0
}

// Validate accepts a [QuantityValue]. Anything else returns
// CodeWrongType. When Units is empty the constraint accepts any
// magnitude, and any units the property admits: units outside a
// non-empty PropertyUnits are a CodeUnitUnknown, and with PropertyUnits
// empty too any units are accepted, the OPT having marked the node
// "DV_QUANTITY without further constraint". A non-empty Units decides
// the units alone. When several entries have the value's units, any one
// whose ranges admit the value is enough; when none does, the violations
// name the first of them.
func (c DvQuantity) Validate(value any) []Violation {
	q, ok := value.(QuantityValue)
	if !ok {
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected QuantityValue, got %T", value)}}
	}
	if len(c.Units) == 0 {
		return c.propertyViolations(q)
	}
	// Any entry with the supplied units that admits the value is
	// enough. When none does, report the ranges of the first one.
	// Mismatched units short-circuits to CodeUnitUnknown so the
	// violation list does not pile up a spurious out-of-range against
	// an unrelated unit.
	var first []Violation
	matched := false
	for _, u := range c.Units {
		if u.Units != q.Units {
			continue
		}
		out := u.violations(q)
		if len(out) == 0 {
			return nil
		}
		if !matched {
			first, matched = out, true
		}
	}
	if matched {
		return first
	}
	allowed := make([]string, len(c.Units))
	for i, u := range c.Units {
		allowed[i] = u.Units
	}
	return []Violation{{
		Code:   CodeUnitUnknown,
		Detail: fmt.Sprintf("units not in allowed %v", allowed),
		Value:  Redact(q.Units),
	}}
}

// propertyViolations checks the units of q against PropertyUnits, for a
// constraint with no Units list. Matching is exact, as UCUM codes are
// case-sensitive.
func (c DvQuantity) propertyViolations(q QuantityValue) []Violation {
	if len(c.PropertyUnits) == 0 || slices.Contains(c.PropertyUnits, q.Units) {
		return nil
	}
	property := "the property"
	if c.Property != nil {
		property = "property " + c.Property.String()
	}
	return []Violation{{
		Code:   CodeUnitUnknown,
		Detail: fmt.Sprintf("units not among the units %v of %s", c.PropertyUnits, property),
		Value:  Redact(q.Units),
	}}
}

// violations checks q, whose units match u's, against u's magnitude
// and precision ranges.
func (u QuantityUnit) violations(q QuantityValue) []Violation {
	var out []Violation
	if u.Magnitude.IsBounded() && !u.Magnitude.Contains(q.Magnitude) {
		out = append(out, Violation{
			Code:   CodeOutOfRange,
			Detail: fmt.Sprintf("magnitude outside %s for units %q", u.Magnitude, u.Units),
			Value:  Redact(q.Magnitude),
		})
	}
	if u.Precision.IsBounded() && q.Precision >= 0 && !u.Precision.Contains(float64(q.Precision)) {
		out = append(out, Violation{
			Code:   CodeOutOfRange,
			Detail: fmt.Sprintf("precision outside %s for units %q", u.Precision, u.Units),
			Value:  Redact(q.Precision),
		})
	}
	return out
}

// OrdinalSymbol is one (value, symbol) pair from a DV_ORDINAL
// constraint <list> entry. Symbol carries the terminology binding
// for the ordinal label (e.g. SNOMED-CT::260349002 = "moderate").
type OrdinalSymbol struct {
	Value  int
	Symbol CodedTermRef
}

// CDvOrdinal constrains an RM DV_ORDINAL value (C_DV_ORDINAL). The
// constraint enumerates a closed list of (value, symbol) pairs; an
// incoming ordinal value must match one of them.
type CDvOrdinal struct {
	Values []OrdinalSymbol
}

func (CDvOrdinal) isPrimitive() {}

// ExampleValue returns the first ordinal value when the constraint
// enumerates a closed list; 0 otherwise. Validate accepts
// int (the ordinal value), which matches the example shape; pair
// matching against OrdinalSymbol is the caller's choice.
func (c CDvOrdinal) ExampleValue() any {
	if len(c.Values) > 0 {
		return c.Values[0].Value
	}
	return 0
}

// Validate accepts either an int (the ordinal value) or a full
// [OrdinalSymbol] (value + symbol). When Values is empty the constraint
// fixes the type only, so any int or OrdinalSymbol is accepted. Otherwise
// an int must match the value of an entry in Values, and an
// [OrdinalSymbol] must match both the value and the symbol of one: the
// code strings must be equal, and the terminologies equal or either one
// empty, as [CodePhrase.Validate] compares a terminology. A listed symbol
// that names no terminology therefore admits the one the input carries.
func (c CDvOrdinal) Validate(value any) []Violation {
	// An OPT that names DV_ORDINAL and lists no pairs constrains the
	// type only. A closed list is what rejects a value.
	if len(c.Values) == 0 {
		switch value.(type) {
		case int, OrdinalSymbol:
			return nil
		default:
			return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected int or OrdinalSymbol, got %T", value)}}
		}
	}
	switch v := value.(type) {
	case int:
		for _, s := range c.Values {
			if s.Value == v {
				return nil
			}
		}
		allowed := make([]int, len(c.Values))
		for i, s := range c.Values {
			allowed[i] = s.Value
		}
		return []Violation{{
			Code:   CodeNotInList,
			Detail: fmt.Sprintf("ordinal value not in allowed %v", allowed),
			Value:  Redact(value),
		}}
	case OrdinalSymbol:
		for _, s := range c.Values {
			if s.Value == v.Value && s.Symbol.CodeString == v.Symbol.CodeString &&
				(s.Symbol.Terminology == "" || v.Symbol.Terminology == "" || s.Symbol.Terminology == v.Symbol.Terminology) {
				return nil
			}
		}
		allowed := make([]string, len(c.Values))
		for i, s := range c.Values {
			allowed[i] = fmt.Sprintf("%d|%s", s.Value, s.Symbol)
		}
		return []Violation{{
			Code:   CodeNotInList,
			Detail: fmt.Sprintf("ordinal (value, symbol) pair not in allowed %v", allowed),
			Value:  Redact(value),
		}}
	default:
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected int or OrdinalSymbol, got %T", value)}}
	}
}
