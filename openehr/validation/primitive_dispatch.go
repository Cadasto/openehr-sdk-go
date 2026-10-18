package validation

import (
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// noPrecision is the sentinel passed to
// [constraints.QuantityValue.Precision] when DV_QUANTITY.Precision
// is nil on the wire. [constraints.DvQuantity.Validate] treats
// negative precision as "unspecified" (skips precision checks).
const noPrecision rm.Integer = -1

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
//   - DV_COUNT     → int64 (magnitude)
//   - DV_ORDINAL   → [constraints.OrdinalSymbol] (value, symbol's
//     defining_code)
//   - DV_DATE      → string (value, ISO 8601)
//   - DV_TIME      → string (value, ISO 8601)
//   - DV_DATE_TIME → string (value, ISO 8601)
//   - DV_DURATION  → string (value, ISO 8601 duration)
func dataValueInput(dv rm.DataValue) (any, bool) {
	// Belt-and-suspenders typed-nil guard. The walk's primary
	// defence is the generated access, which reads an interface holding
	// a nil pointer as absent, plus the walk's own `IsTypedNilPointer`
	// check before it visits a node; both reject typed-nil before
	// reaching this dispatcher. A bare-nil interface still bypasses both
	// (rare on the bound-RM-value path but cheap to guard here).
	if dv == nil {
		return nil, false
	}
	switch v := dv.(type) {
	case *rm.DVQuantity:
		return constraints.QuantityValue{
			Magnitude: float64(v.Magnitude),
			Units:     v.Units,
			Precision: int(deref(v.Precision, noPrecision)),
		}, true
	case rm.DVQuantity:
		return constraints.QuantityValue{
			Magnitude: float64(v.Magnitude),
			Units:     v.Units,
			Precision: int(deref(v.Precision, noPrecision)),
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
		// DV_COUNT.magnitude is an rm.Integer64. The constraints package
		// cannot name the RM type, so it gets the bare int64, which
		// [constraints.CInteger.Validate] reads without loss.
		return int64(v.Magnitude), true
	case rm.DVCount:
		return int64(v.Magnitude), true
	case *rm.DVOrdinal:
		return ordinalInput(v), true
	case rm.DVOrdinal:
		return ordinalInput(&v), true
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

// ordinalInput returns the value [constraints.CDvOrdinal.Validate] checks
// for o: the value and the symbol's defining_code as one
// [constraints.OrdinalSymbol], so a symbol that does not match its value is
// reported; a code naming no terminology matches a listed pair by its code
// (REQ-103) while the RM rules report the missing terminology. A code with
// an empty code string is passed on as it is: where the RM rules report it,
// the walk has the constraint judge the value alone
// ([engine.judgeLeaf]).
//
// DV_ORDINAL.value is Integer (int32) per the RM; widening it to int is
// safe on every supported Go platform.
func ordinalInput(o *rm.DVOrdinal) constraints.OrdinalSymbol {
	code := o.Symbol.DefiningCode
	return constraints.OrdinalSymbol{
		Value: int(o.Value),
		Symbol: constraints.CodedTermRef{
			Terminology: code.TerminologyID.Value,
			CodeString:  code.CodeString,
		},
	}
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
