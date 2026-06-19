package rm

import "math"

// AsInt64 widens any Go integer shape — and the RM [Integer] scalar —
// to int64. uint/uint64 values above [math.MaxInt64] return ok=false
// rather than silently wrapping to a negative. Float inputs are
// rejected (they belong to [AsReal]).
//
// This is the single integer coercion shared by the RM write/validate
// paths (instance synthesis, rmwrite attach, validation matching) so
// those layers cannot drift on which Go shapes count as an integer.
func AsInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int8:
		return int64(n), true
	case int16:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case uint:
		if uint64(n) > math.MaxInt64 {
			return 0, false
		}
		return int64(n), true
	case uint8:
		return int64(n), true
	case uint16:
		return int64(n), true
	case uint32:
		return int64(n), true
	case uint64:
		if n > math.MaxInt64 {
			return 0, false
		}
		return int64(n), true
	case Integer:
		return int64(n), true
	}
	return 0, false
}

// AsReal widens any Go numeric shape — and the RM [Integer]/[Real]
// scalars — to [Real]. Integer inputs widen losslessly. It is the
// real counterpart to [AsInt64], shared across the same layers.
func AsReal(v any) (Real, bool) {
	switch n := v.(type) {
	case Real:
		return n, true
	case float64:
		return Real(n), true
	case float32:
		return Real(n), true
	}
	if i, ok := AsInt64(v); ok {
		return Real(i), true
	}
	return 0, false
}

// IsInt64 reports whether v is an integer shape [AsInt64] accepts.
func IsInt64(v any) bool {
	_, ok := AsInt64(v)
	return ok
}

// IsReal reports whether v is a numeric shape [AsReal] accepts.
func IsReal(v any) bool {
	_, ok := AsReal(v)
	return ok
}
