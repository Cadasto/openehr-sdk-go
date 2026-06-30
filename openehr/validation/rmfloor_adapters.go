package validation

// rmfloor_adapters.go: type-switch adapters that lift a polymorphic RM
// value into the concrete shape the [rmFloorWalker]'s invariant
// evaluators need. Mirrors the closed-set discipline of
// rmTypeInfo/describeRMType in composition.go — adding a new BMM
// concrete means editing one switch.

import (
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// asCodePhrase recovers a CODE_PHRASE value (by value or by pointer)
// from any concrete carrying it. Returns ok=false when value is not a
// CODE_PHRASE.
func asCodePhrase(value any) (rm.CodePhrase, bool) {
	switch v := value.(type) {
	case *rm.CodePhrase:
		if v == nil {
			return rm.CodePhrase{}, false
		}
		return *v, true
	case rm.CodePhrase:
		return v, true
	}
	return rm.CodePhrase{}, false
}

// asDVQuantity recovers a DV_QUANTITY value (by value or by pointer)
// from any concrete carrying it.
func asDVQuantity(value any) (rm.DVQuantity, bool) {
	switch v := value.(type) {
	case *rm.DVQuantity:
		if v == nil {
			return rm.DVQuantity{}, false
		}
		return *v, true
	case rm.DVQuantity:
		return v, true
	}
	return rm.DVQuantity{}, false
}

// dvIntervalNumericBounds returns the lower/upper magnitudes of a
// DV_INTERVAL when its bound type is a numerically-comparable RM concrete
// (DV_QUANTITY or DV_COUNT) and neither side is unbounded. It handles the
// monomorphised instantiations RM data actually carries —
// DVInterval[DVQuantity] (e.g. DV_QUANTITY.normal_range), DVInterval[DVCount]
// — as well as the bare DVInterval[DVOrdered] collapsed form. Returns
// ok=false otherwise (including DV_DATE / DV_TIME / etc. — temporal interval
// bounds need richer comparison semantics deferred to a follow-up cycle, see
// REQ-123's temporal helpers).
func dvIntervalNumericBounds(value any) (lower, upper float64, ok bool) {
	switch v := value.(type) {
	case *rm.DVInterval[rm.DVQuantity]:
		if v == nil {
			return 0, 0, false
		}
		return numericBounds(v, dvQuantityMagnitude)
	case rm.DVInterval[rm.DVQuantity]:
		return numericBounds(&v, dvQuantityMagnitude)
	case *rm.DVInterval[rm.DVCount]:
		if v == nil {
			return 0, 0, false
		}
		return numericBounds(v, dvCountMagnitude)
	case rm.DVInterval[rm.DVCount]:
		return numericBounds(&v, dvCountMagnitude)
	case *rm.DVInterval[rm.DVOrdered]:
		if v == nil {
			return 0, 0, false
		}
		return numericBounds(v, dvOrderedAsFloat)
	case rm.DVInterval[rm.DVOrdered]:
		return numericBounds(&v, dvOrderedAsFloat)
	}
	return 0, 0, false
}

// numericBounds is the worker for dvIntervalNumericBounds, generic over the
// interval's bound element type. mag lifts a bound to a float64 magnitude
// (ok=false when it carries none). Honours the `lower_unbounded` /
// `upper_unbounded` flags — an unbounded side means the comparison is
// undefined and we skip the invariant check by returning ok=false.
func numericBounds[T rm.DVOrdered](iv *rm.DVInterval[T], mag func(T) (float64, bool)) (lower, upper float64, ok bool) {
	if iv.LowerUnbounded || iv.UpperUnbounded {
		return 0, 0, false
	}
	lo, loOK := mag(iv.Lower)
	hi, hiOK := mag(iv.Upper)
	if !loOK || !hiOK {
		return 0, 0, false
	}
	return lo, hi, true
}

// dvQuantityMagnitude / dvCountMagnitude are the per-element magnitude
// liftings for the numeric DV_INTERVAL instantiations; both bound elements
// are always present (typed value fields), so ok is always true.
func dvQuantityMagnitude(q rm.DVQuantity) (float64, bool) { return float64(q.Magnitude), true }

func dvCountMagnitude(c rm.DVCount) (float64, bool) { return float64(c.Magnitude), true }

// dvOrderedAsFloat lifts a DVOrdered bound to a float64 magnitude when
// the concrete carries one (DV_QUANTITY, DV_COUNT). Returns ok=false
// for any other DVOrdered concrete (DV_DATE/TIME/DURATION/ORDINAL/…) —
// those need RM-spec-aware comparison handled by REQ-123 follow-ups.
func dvOrderedAsFloat(v rm.DVOrdered) (float64, bool) {
	switch x := v.(type) {
	case rm.DVQuantity:
		return float64(x.Magnitude), true
	case *rm.DVQuantity:
		if x == nil {
			return 0, false
		}
		return float64(x.Magnitude), true
	case rm.DVCount:
		return float64(x.Magnitude), true
	case *rm.DVCount:
		if x == nil {
			return 0, false
		}
		return float64(x.Magnitude), true
	}
	return 0, false
}
