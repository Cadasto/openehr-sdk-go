package constraints

import (
	"fmt"
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
// magnitude range. Property is the optional terminology binding for
// the measured quantity (e.g. "blood pressure"); it is exposed for
// inspection but Validate does not enforce it.
type DvQuantity struct {
	Units    []QuantityUnit
	Property *CodedTermRef
}

func (DvQuantity) isPrimitive() {}

// ExampleValue returns a minimal-valid [QuantityValue].
// The first entry of Units drives the example: the magnitude comes from
// the entry's range (lower bound when set; midpoint or zero otherwise)
// and the units are copied verbatim. It falls back to QuantityValue{0, "1"}
// when the constraint is open-ended (any units / any magnitude).
func (c DvQuantity) ExampleValue() any {
	if len(c.Units) == 0 {
		return QuantityValue{Magnitude: 0, Units: "1", Precision: -1}
	}
	u := c.Units[0]
	mag := exampleMagnitude(u.Magnitude)
	return QuantityValue{Magnitude: mag, Units: u.Units, Precision: -1}
}

// exampleMagnitude picks an in-range float for a quantity unit. Same
// "lower bound nudged inside exclusive ends" rule as [CReal].
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
// units / magnitude: the OPT may have omitted a list to mark the
// node "DV_QUANTITY without further constraint".
func (c DvQuantity) Validate(value any) []Violation {
	q, ok := value.(QuantityValue)
	if !ok {
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected QuantityValue, got %T", value)}}
	}
	if len(c.Units) == 0 {
		return nil
	}
	// Find the entry matching the supplied units; report the
	// magnitude range against it. Mismatched units short-circuits to
	// CodeUnitUnknown so the violation list does not pile up a
	// spurious out-of-range against an unrelated unit.
	for _, u := range c.Units {
		if u.Units != q.Units {
			continue
		}
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
// [OrdinalSymbol] must match both the value and the symbol of one.
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
			if s.Value == v.Value && s.Symbol == v.Symbol {
				return nil
			}
		}
		return []Violation{{
			Code:   CodeNotInList,
			Detail: "ordinal (value, symbol) pair not in allowed list",
			Value:  Redact(value),
		}}
	default:
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected int or OrdinalSymbol, got %T", value)}}
	}
}
