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
// pointer variants, with no reflection.
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
//
// A quantity, a coded text and an ordinal are read by the readers the typed
// checks read them with ([quantityOf], [codedTermOf], [ordinalOf]), so the
// two paths read one value alike.
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
	if q, ok := quantityOf(dv); ok {
		return q, true
	}
	if ref, ok := codedTermOf(dv); ok {
		return ref, true
	}
	if s, ok := ordinalOf(dv); ok {
		return s, true
	}
	switch v := dv.(type) {
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

// codedTermOf returns the code a code phrase constraint checks for rmValue,
// the RM value bound to a node, typed ([constraints.CodePhrase.ValidateCode]),
// and whether [primitiveInput] reads rmValue as a code: the defining_code of
// a DV_CODED_TEXT, or a CODE_PHRASE, a nil *rm.CodePhrase giving the zero
// code.
func codedTermOf(rmValue any) (constraints.CodedTermRef, bool) {
	switch v := rmValue.(type) {
	case *rm.DVCodedText:
		return codePhraseInput(&v.DefiningCode), true
	case rm.DVCodedText:
		return codePhraseInput(&v.DefiningCode), true
	case rm.CodePhrase:
		return codePhraseInput(&v), true
	case *rm.CodePhrase:
		if v == nil {
			return constraints.CodedTermRef{}, true
		}
		return codePhraseInput(v), true
	}
	return constraints.CodedTermRef{}, false
}

// quantityOf returns the quantity a quantity constraint checks for rmValue,
// the RM value bound to a node, typed ([constraints.DvQuantity.ValidateQuantity]),
// and whether [primitiveInput] reads rmValue as a quantity: a DV_QUANTITY.
func quantityOf(rmValue any) (constraints.QuantityValue, bool) {
	switch v := rmValue.(type) {
	case *rm.DVQuantity:
		return quantityInput(v), true
	case rm.DVQuantity:
		return quantityInput(&v), true
	}
	return constraints.QuantityValue{}, false
}

// ordinalOf returns the pair an ordinal constraint checks for rmValue, the
// RM value bound to a node, typed ([constraints.CDvOrdinal.ValidateOrdinal]),
// and whether [primitiveInput] reads rmValue as a pair: a DV_ORDINAL.
func ordinalOf(rmValue any) (constraints.OrdinalSymbol, bool) {
	switch v := rmValue.(type) {
	case *rm.DVOrdinal:
		return ordinalInput(v), true
	case rm.DVOrdinal:
		return ordinalInput(&v), true
	}
	return constraints.OrdinalSymbol{}, false
}

// codePhraseInput returns the code a code phrase constraint checks for c:
// its terminology id and its code string.
func codePhraseInput(c *rm.CodePhrase) constraints.CodedTermRef {
	return constraints.CodedTermRef{
		Terminology: c.TerminologyID.Value,
		CodeString:  c.CodeString,
	}
}

// quantityInput returns the quantity a quantity constraint checks for q: its
// magnitude, its units and its precision, [noPrecision] where it has none.
func quantityInput(q *rm.DVQuantity) constraints.QuantityValue {
	return constraints.QuantityValue{
		Magnitude: float64(q.Magnitude),
		Units:     q.Units,
		Precision: int(deref(q.Precision, noPrecision)),
	}
}

// ordinalInput returns the value [constraints.CDvOrdinal.ValidateOrdinal]
// checks for o: the value and the symbol's defining_code as one
// [constraints.OrdinalSymbol], so a symbol that does not match its value is
// reported; a code naming no terminology matches a listed pair by its code
// (REQ-103) while the RM rules report the missing terminology. An absent
// part, an omitted symbol, a code with an empty code string or a value a
// decode reports missing, is passed on as it is: where the RM rules report
// it, the walk has the constraint judge only what the other part leaves
// ([engine.judgeLeaf]).
//
// DV_ORDINAL.value is Integer (int32) per the RM; widening it to int is
// safe on every supported Go platform.
func ordinalInput(o *rm.DVOrdinal) constraints.OrdinalSymbol {
	return constraints.OrdinalSymbol{
		Value:  int(o.Value),
		Symbol: codePhraseInput(&o.Symbol.DefiningCode),
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
