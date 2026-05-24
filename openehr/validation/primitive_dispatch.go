package validation

import (
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// dataValueInput converts an RM DATA_VALUE into the Go value
// [constraints.PrimitiveConstraint.Validate] expects for the matching
// primitive type. Returns (input, true) for the supported set and
// (nil, false) for DV types REQ-103 does not have a typed primitive
// for (those are passed through silently — the OPT couldn't have
// declared a primitive constraint on them anyway).
//
// The closed switch enumerates concrete `rm.DV*` types and their
// pointer variants. Per REQ-024 — no reflection.
//
// Supported set (REQ-103 closed primitive types mapped):
//
//   - DV_QUANTITY  → [constraints.QuantityValue]
//   - DV_CODED_TEXT → [constraints.CodedTermRef] (defining_code)
//   - DV_TEXT      → string (value)
//   - DV_BOOLEAN   → bool
//   - DV_COUNT     → int (magnitude)
//   - DV_ORDINAL   → int (value)
//   - DV_DATE      → string (value, ISO 8601)
//   - DV_TIME      → string (value, ISO 8601)
//   - DV_DATE_TIME → string (value, ISO 8601)
//   - DV_DURATION  → string (value, ISO 8601 duration)
func dataValueInput(dv rm.DataValue) (any, bool) {
	switch v := dv.(type) {
	case *rm.DVQuantity:
		return constraints.QuantityValue{
			Magnitude: float64(v.Magnitude),
			Units:     v.Units,
			Precision: int(deref(v.Precision, -1)),
		}, true
	case rm.DVQuantity:
		return constraints.QuantityValue{
			Magnitude: float64(v.Magnitude),
			Units:     v.Units,
			Precision: int(deref(v.Precision, -1)),
		}, true
	case *rm.DVCodedText:
		return constraints.CodedTermRef{
			Terminology: v.DefiningCode.TerminologyID.Value,
			CodeString:  v.DefiningCode.CodeString,
		}, true
	case rm.DVCodedText:
		return constraints.CodedTermRef{
			Terminology: v.DefiningCode.TerminologyID.Value,
			CodeString:  v.DefiningCode.CodeString,
		}, true
	case *rm.DVText:
		return v.Value, true
	case rm.DVText:
		return v.Value, true
	case *rm.DVBoolean:
		return v.Value, true
	case rm.DVBoolean:
		return v.Value, true
	case *rm.DVCount:
		// DV_COUNT.magnitude is Integer64 (int64) per the RM spec —
		// pass-through preserves precision on 32-bit platforms.
		// [constraints.CInteger.Validate] accepts int64 directly.
		return v.Magnitude, true
	case rm.DVCount:
		return v.Magnitude, true
	case *rm.DVOrdinal:
		// DV_ORDINAL.value is Integer (int32) per the RM spec; safe
		// to widen to int (≥ int32 on every supported Go platform).
		return int(v.Value), true
	case rm.DVOrdinal:
		return int(v.Value), true
	case *rm.DVDate:
		return v.Value, true
	case rm.DVDate:
		return v.Value, true
	case *rm.DVTime:
		return v.Value, true
	case rm.DVTime:
		return v.Value, true
	case *rm.DVDateTime:
		return v.Value, true
	case rm.DVDateTime:
		return v.Value, true
	case *rm.DVDuration:
		return v.Value, true
	case rm.DVDuration:
		return v.Value, true
	}
	return nil, false
}

// deref returns *p when non-nil, otherwise fallback. Generic helper
// to fold optional integer fields (e.g. DVQuantity.Precision *Integer)
// without a per-call nil check.
func deref[T any](p *T, fallback T) T {
	if p == nil {
		return fallback
	}
	return *p
}
