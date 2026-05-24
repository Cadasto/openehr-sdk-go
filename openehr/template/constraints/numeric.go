package constraints

import (
	"fmt"
	"slices"
)

// CBoolean constrains an RM Boolean value (C_BOOLEAN). At least one
// of TrueValid / FalseValid is true in a well-formed OPT — both
// false would constrain the attribute to no legal value, which the
// XSD allows but readers should reject as a modelling error.
type CBoolean struct {
	TrueValid  bool
	FalseValid bool

	// Default carries the OPT <assumed_value>; nil when omitted.
	Default *bool
}

func (CBoolean) isPrimitive() {}

// Validate accepts a Go bool. Any other type returns CodeWrongType.
func (c CBoolean) Validate(value any) []Violation {
	b, ok := value.(bool)
	if !ok {
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected bool, got %T", value)}}
	}
	if b && !c.TrueValid {
		return []Violation{{Code: CodeNotInList, Detail: "value true not allowed"}}
	}
	if !b && !c.FalseValid {
		return []Violation{{Code: CodeNotInList, Detail: "value false not allowed"}}
	}
	return nil
}

// CInteger constrains an RM Integer value (C_INTEGER). Range is the
// allowed numeric interval; List is the optional closed enumeration
// (when non-empty, the value MUST appear in it). When both are set,
// the value MUST satisfy both.
type CInteger struct {
	Range NumericRange
	List  []int64

	// Default carries the OPT <assumed_value>; nil when omitted.
	Default *int64
}

func (CInteger) isPrimitive() {}

// Validate accepts int / int8..int64 / uint / uint8..uint32. Larger
// uints (uint64 above MaxInt64) return CodeWrongType to avoid silent
// overflow. Float types are rejected — use [CReal] for fractional
// values.
func (c CInteger) Validate(value any) []Violation {
	n, ok := toInt64(value)
	if !ok {
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected integer, got %T", value)}}
	}
	var out []Violation
	if len(c.List) > 0 && !slices.Contains(c.List, n) {
		out = append(out, Violation{
			Code:   CodeNotInList,
			Detail: fmt.Sprintf("%d not in allowed list %v", n, c.List),
		})
	}
	if c.Range.IsBounded() && !c.Range.Contains(float64(n)) {
		out = append(out, Violation{
			Code:   CodeOutOfRange,
			Detail: fmt.Sprintf("%d outside %s", n, c.Range),
		})
	}
	return out
}

// CReal constrains an RM Real value (C_REAL). Range / List semantics
// mirror [CInteger]; List membership uses exact float equality, so
// callers that need tolerant comparison should pre-round.
type CReal struct {
	Range NumericRange
	List  []float64

	// Default carries the OPT <assumed_value>; nil when omitted.
	Default *float64
}

func (CReal) isPrimitive() {}

// Validate accepts float32 / float64 and any integer type (widened
// to float64). Returns CodeWrongType for non-numeric inputs.
func (c CReal) Validate(value any) []Violation {
	f, ok := toFloat64(value)
	if !ok {
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected real, got %T", value)}}
	}
	var out []Violation
	if len(c.List) > 0 && !slices.Contains(c.List, f) {
		out = append(out, Violation{
			Code:   CodeNotInList,
			Detail: fmt.Sprintf("%v not in allowed list %v", f, c.List),
		})
	}
	if c.Range.IsBounded() && !c.Range.Contains(f) {
		out = append(out, Violation{
			Code:   CodeOutOfRange,
			Detail: fmt.Sprintf("%v outside %s", f, c.Range),
		})
	}
	return out
}

// toInt64 coerces value to int64 with overflow checks for unsigned
// types. Float inputs are rejected — they belong to [CReal].
func toInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		const maxInt64 = uint64(1)<<63 - 1
		if uint64(v) > maxInt64 {
			return 0, false
		}
		return int64(v), true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		// Same overflow guard as `case uint` — values that fit
		// int64 widen losslessly; uint64 > MaxInt64 returns
		// !ok so the caller reports CodeWrongType rather than
		// silently wrapping to a negative.
		const maxInt64 = uint64(1)<<63 - 1
		if v > maxInt64 {
			return 0, false
		}
		return int64(v), true
	}
	return 0, false
}

// toFloat64 coerces value to float64. Accepts integer kinds via widening.
func toFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case float32:
		return float64(v), true
	case float64:
		return v, true
	}
	if n, ok := toInt64(value); ok {
		return float64(n), true
	}
	return 0, false
}
