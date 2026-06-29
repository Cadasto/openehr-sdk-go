package validation

// rmfloor_adapters.go: type-switch adapters that lift a polymorphic RM
// value into the concrete shape the [rmFloorWalker]'s invariant
// evaluators need. Mirrors the closed-set discipline of
// rmTypeInfo/describeRMType in composition.go — adding a new BMM
// concrete means editing one switch.

import (
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
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
// DV_INTERVAL when its bound type is a numerically-comparable RM
// concrete (DV_QUANTITY or DV_COUNT) and neither side is unbounded.
// Returns ok=false otherwise (including for DV_DATE / DV_TIME / etc. —
// temporal interval bounds need richer comparison semantics deferred
// to a follow-up cycle, see REQ-123's temporal helpers).
func dvIntervalNumericBounds(value any) (lower, upper float64, ok bool) {
	switch v := value.(type) {
	case *rm.DVInterval[rm.DVOrdered]:
		if v == nil {
			return 0, 0, false
		}
		return numericBounds(v)
	case rm.DVInterval[rm.DVOrdered]:
		return numericBounds(&v)
	}
	return 0, 0, false
}

// numericBounds is the worker for dvIntervalNumericBounds. Honours the
// `lower_unbounded` / `upper_unbounded` flags — an unbounded side means
// the comparison is undefined and we skip the invariant check by
// returning ok=false.
func numericBounds(iv *rm.DVInterval[rm.DVOrdered]) (lower, upper float64, ok bool) {
	if iv.LowerUnbounded || iv.UpperUnbounded {
		return 0, 0, false
	}
	lo, loOK := dvOrderedAsFloat(iv.Lower)
	hi, hiOK := dvOrderedAsFloat(iv.Upper)
	if !loOK || !hiOK {
		return 0, 0, false
	}
	return lo, hi, true
}

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

// objectRefBaseFields recovers the OBJECT_REF parent's id (rendered),
// type, and namespace from any OBJECT_REF subtype. Routes through the
// SDK-GAP-11 [rm.ObjectRefLike] interface so adding a new BMM subtype
// is a no-op here — satisfying the interface is enough. The typed-nil
// guard upfront prevents a panic when a nil pointer is passed boxed in
// the interface (the *Like accessors have value receivers; calling
// them on a nil pointer would deref).
func objectRefBaseFields(value any) (id, refType, namespace string, ok bool) {
	if value == nil || rmread.IsTypedNilPointer(value) {
		return "", "", "", false
	}
	ref, isRef := value.(rm.ObjectRefLike)
	if !isRef {
		return "", "", "", false
	}
	if oid := ref.GetID(); oid != nil {
		id = describeObjectID(oid)
	}
	return id, ref.GetType(), ref.GetNamespace(), true
}

// describeObjectID renders an OBJECT_ID for diagnostic purposes. The
// rendition is loose — the invariant check only cares whether the id
// has any printable identity at all.
func describeObjectID(oid rm.ObjectID) string {
	if oid == nil {
		return ""
	}
	// Every ObjectID concrete carries a .Value string (HierObjectID,
	// UUID, ISO_OID, GenericID, …); render it through fmt to keep the
	// adapter generic without importing the closed UID set.
	type valuer interface{ GetValue() string }
	if vv, ok := oid.(valuer); ok {
		return vv.GetValue()
	}
	// Fallback: a Stringer or default Go formatting.
	type stringer interface{ String() string }
	if vv, ok := oid.(stringer); ok {
		return vv.String()
	}
	return ""
}
