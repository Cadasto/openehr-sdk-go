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

// asDVProportion recovers a DV_PROPORTION value (by value or by pointer)
// from any concrete carrying it.
func asDVProportion(value any) (rm.DVProportion, bool) {
	switch v := value.(type) {
	case *rm.DVProportion:
		if v == nil {
			return rm.DVProportion{}, false
		}
		return *v, true
	case rm.DVProportion:
		return v, true
	}
	return rm.DVProportion{}, false
}

// asElement recovers an ELEMENT value (by value or by pointer).
func asElement(value any) (rm.Element, bool) {
	switch v := value.(type) {
	case *rm.Element:
		if v == nil {
			return rm.Element{}, false
		}
		return *v, true
	case rm.Element:
		return v, true
	}
	return rm.Element{}, false
}

// temporalValueValid reports whether value, one of DV_DATE_TIME / DV_DATE /
// DV_TIME / DV_DURATION (by value or by pointer), satisfies its ISO 8601
// predicate, using the REQ-123 parse. ok is false for any other value.
func temporalValueValid(value any) (valid, ok bool) {
	switch v := value.(type) {
	case *rm.DVDateTime:
		return v != nil && v.ValidISO8601(), v != nil
	case rm.DVDateTime:
		return v.ValidISO8601(), true
	case *rm.DVDate:
		return v != nil && v.ValidISO8601(), v != nil
	case rm.DVDate:
		return v.ValidISO8601(), true
	case *rm.DVTime:
		return v != nil && v.ValidISO8601(), v != nil
	case rm.DVTime:
		return v.ValidISO8601(), true
	case *rm.DVDuration:
		return v != nil && v.ValidISO8601(), v != nil
	case rm.DVDuration:
		return v.ValidISO8601(), true
	}
	return false, false
}

// asMappings recovers the DV_TEXT.mappings slice (by value or by
// pointer) from any concrete carrying it — DV_TEXT itself, and
// DV_CODED_TEXT, which inherits the attribute via its embedded DV_TEXT.
// Returns ok=false when value is neither.
func asMappings(value any) ([]rm.TermMapping, bool) {
	switch v := value.(type) {
	case *rm.DVText:
		if v == nil {
			return nil, false
		}
		return v.Mappings, true
	case rm.DVText:
		return v.Mappings, true
	case *rm.DVCodedText:
		if v == nil {
			return nil, false
		}
		return v.Mappings, true
	case rm.DVCodedText:
		return v.Mappings, true
	}
	return nil, false
}

// asDVCodedText recovers a DV_CODED_TEXT value (by value or by non-nil
// pointer). It is how a coded check tells a DV_CODED_TEXT from a plain
// DV_TEXT in a DV_TEXT slot such as PARTICIPATION.function.
func asDVCodedText(value any) (rm.DVCodedText, bool) {
	switch v := value.(type) {
	case *rm.DVCodedText:
		if v == nil {
			return rm.DVCodedText{}, false
		}
		return *v, true
	case rm.DVCodedText:
		return v, true
	}
	return rm.DVCodedText{}, false
}

// asTermMapping recovers a TERM_MAPPING value (by value or by pointer).
// The walk boxes `mappings` elements as pointers, so the pointer arm is
// the one that fires during a descent; the value arm covers a caller
// handing a TERM_MAPPING to [ValidateRM] directly.
func asTermMapping(value any) (rm.TermMapping, bool) {
	switch v := value.(type) {
	case *rm.TermMapping:
		if v == nil {
			return rm.TermMapping{}, false
		}
		return *v, true
	case rm.TermMapping:
		return v, true
	}
	return rm.TermMapping{}, false
}

// asArchetyped recovers an ARCHETYPED value (by value or by pointer). The
// walk reaches an ARCHETYPED through LOCATABLE.archetype_details, a pointer,
// so the pointer arm is the one that fires during a descent; the value arm
// covers a caller handing an ARCHETYPED to [ValidateRM] directly.
func asArchetyped(value any) (rm.Archetyped, bool) {
	switch v := value.(type) {
	case *rm.Archetyped:
		if v == nil {
			return rm.Archetyped{}, false
		}
		return *v, true
	case rm.Archetyped:
		return v, true
	}
	return rm.Archetyped{}, false
}

// dvIntervalNumericBounds returns the lower/upper magnitudes of a DV_INTERVAL
// when neither side is unbounded and the floor can order the two bounds. The
// RM states the rule as DV_INTERVAL.Limits_consistent: with neither side
// open, lower.is_strictly_comparable_to(upper) and lower <= upper. `<=` is
// defined per type: DV_COUNT.less_than takes a DV_COUNT and
// DV_QUANTITY.less_than a DV_QUANTITY, each under the precondition
// Pre_comparable (is_strictly_comparable_to(other)). So the floor orders two
// DV_COUNT bounds, or two DV_QUANTITY bounds with the same units. It handles
// the monomorphised instantiations RM data actually carries
// (DVInterval[DVQuantity], e.g. DV_QUANTITY.normal_range; DVInterval[DVCount])
// and the bare DVInterval[DVOrdered] collapsed form. Returns ok=false
// otherwise:
//
//   - an unbounded side (the comparison is undefined);
//   - bounds of different RM types, such as a DV_COUNT beside a DV_QUANTITY
//     on the bare form: no less_than takes both, so the pair has no ordering,
//     even when the quantity is unitless;
//   - DV_QUANTITY bounds with different units, which are not strictly
//     comparable, so a cross-unit interval has no magnitude ordering and the
//     floor must not assert one;
//   - non-numeric bound types (DV_DATE / DV_TIME / … — richer comparison
//     deferred to a follow-up cycle, see REQ-123's temporal helpers).
//
// The check is a conservative subset of Limits_consistent: it tests
// lower <= upper only where that comparison is defined, and reports nothing
// for a pair it cannot order, although the invariant cannot hold for one.
// The comparability half is not checked on its own. DV_QUANTITY's
// is_strictly_comparable_to also needs a matching units_system where one is
// set, which is not compared: a same-units pair with different units_systems
// is ordered by magnitude, and a lower > upper found there breaks the
// invariant all the same.
func dvIntervalNumericBounds(value any) (lower, upper float64, ok bool) {
	lo, hi, bounded := intervalBounds(value)
	if !bounded {
		return 0, 0, false
	}
	loMag, loClass, loUnit, loOK := numericMagnitude(lo)
	hiMag, hiClass, hiUnit, hiOK := numericMagnitude(hi)
	if !loOK || !hiOK || loClass != hiClass || loUnit != hiUnit {
		return 0, 0, false
	}
	return loMag, hiMag, true
}

// intervalBounds extracts the lower/upper bounds of a DV_INTERVAL as DVOrdered
// values when neither side is unbounded, across the typed instantiations and
// the bare collapsed form. bounded is false for an unbounded or unknown shape.
func intervalBounds(value any) (lower, upper rm.DVOrdered, bounded bool) {
	switch v := value.(type) {
	case *rm.DVInterval[rm.DVQuantity]:
		if v == nil || v.LowerUnbounded || v.UpperUnbounded {
			return nil, nil, false
		}
		return v.Lower, v.Upper, true
	case rm.DVInterval[rm.DVQuantity]:
		if v.LowerUnbounded || v.UpperUnbounded {
			return nil, nil, false
		}
		return v.Lower, v.Upper, true
	case *rm.DVInterval[rm.DVCount]:
		if v == nil || v.LowerUnbounded || v.UpperUnbounded {
			return nil, nil, false
		}
		return v.Lower, v.Upper, true
	case rm.DVInterval[rm.DVCount]:
		if v.LowerUnbounded || v.UpperUnbounded {
			return nil, nil, false
		}
		return v.Lower, v.Upper, true
	case *rm.DVInterval[rm.DVOrdered]:
		if v == nil || v.LowerUnbounded || v.UpperUnbounded {
			return nil, nil, false
		}
		return v.Lower, v.Upper, true
	case rm.DVInterval[rm.DVOrdered]:
		if v.LowerUnbounded || v.UpperUnbounded {
			return nil, nil, false
		}
		return v.Lower, v.Upper, true
	}
	return nil, nil, false
}

// numericMagnitude lifts a DVOrdered bound to (magnitude, class, unit, ok).
// class is the bound's RM type; unit is the DV_QUANTITY units string (empty
// for the dimensionless DV_COUNT). Two bounds are comparable only when both
// their class and their units match. Returns ok=false for any other DVOrdered
// concrete (DV_DATE/TIME/DURATION/ORDINAL/…), which need RM-spec-aware
// comparison handled by REQ-123 follow-ups.
func numericMagnitude(v rm.DVOrdered) (mag float64, class, unit string, ok bool) {
	switch x := v.(type) {
	case rm.DVQuantity:
		return float64(x.Magnitude), "DV_QUANTITY", x.Units, true
	case *rm.DVQuantity:
		if x == nil {
			return 0, "", "", false
		}
		return float64(x.Magnitude), "DV_QUANTITY", x.Units, true
	case rm.DVCount:
		return float64(x.Magnitude), "DV_COUNT", "", true
	case *rm.DVCount:
		if x == nil {
			return 0, "", "", false
		}
		return float64(x.Magnitude), "DV_COUNT", "", true
	}
	return 0, "", "", false
}
