package constraints

import (
	"fmt"
)

// QuantityValue is the runtime value shape accepted by
// [DvQuantity.Validate] and [CDvOrdinal.Validate]. Construct directly
// — the constraints package keeps it minimal so callers needn't
// import the rm package to validate a single value.
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
// value MUST match one of them on units and lie inside that entry's
// magnitude range. Property is the optional terminology binding for
// the measured quantity (e.g. "blood pressure"); v1 surfaces it for
// inspection but does not enforce it during Validate.
type DvQuantity struct {
	Units    []QuantityUnit
	Property *CodedTermRef
}

func (DvQuantity) isPrimitive() {}

// Validate accepts a [QuantityValue]. Anything else returns
// CodeWrongType. When Units is empty the constraint accepts any
// units / magnitude — the OPT may have omitted a list to mark the
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
				Detail: fmt.Sprintf("magnitude %v outside %s for units %q", q.Magnitude, u.Magnitude, u.Units),
			})
		}
		if u.Precision.IsBounded() && q.Precision >= 0 && !u.Precision.Contains(float64(q.Precision)) {
			out = append(out, Violation{
				Code:   CodeOutOfRange,
				Detail: fmt.Sprintf("precision %d outside %s for units %q", q.Precision, u.Precision, u.Units),
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
		Detail: fmt.Sprintf("units %q not in allowed %v", q.Units, allowed),
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
// incoming ordinal value MUST match one of them.
type CDvOrdinal struct {
	Values []OrdinalSymbol
}

func (CDvOrdinal) isPrimitive() {}

// Validate accepts either an int (the ordinal value) or a full
// [OrdinalSymbol] (value + symbol). For [OrdinalSymbol] inputs both
// the value AND the symbol MUST match an entry in Values.
func (c CDvOrdinal) Validate(value any) []Violation {
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
			Detail: fmt.Sprintf("ordinal value %d not in allowed %v", v, allowed),
		}}
	case OrdinalSymbol:
		for _, s := range c.Values {
			if s.Value == v.Value && s.Symbol == v.Symbol {
				return nil
			}
		}
		return []Violation{{
			Code:   CodeNotInList,
			Detail: fmt.Sprintf("(%d, %s) not in allowed ordinal list", v.Value, v.Symbol),
		}}
	default:
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected int or OrdinalSymbol, got %T", value)}}
	}
}
