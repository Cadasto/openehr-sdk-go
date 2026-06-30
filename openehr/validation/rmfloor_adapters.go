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

// dvIntervalNumericBounds returns the lower/upper magnitudes of a DV_INTERVAL
// when both bounds are numerically comparable — same-unit DV_QUANTITY, or
// DV_COUNT — and neither side is unbounded. It handles the monomorphised
// instantiations RM data actually carries (DVInterval[DVQuantity], e.g.
// DV_QUANTITY.normal_range; DVInterval[DVCount]) and the bare
// DVInterval[DVOrdered] collapsed form. Returns ok=false otherwise:
//
//   - an unbounded side (the comparison is undefined);
//   - bounds with different DV_QUANTITY units — the RM defines ordering only
//     between strictly-comparable (same-unit) quantities, so a cross-unit
//     interval has no magnitude ordering and the floor must not assert one;
//   - non-numeric bound types (DV_DATE / DV_TIME / … — richer comparison
//     deferred to a follow-up cycle, see REQ-123's temporal helpers).
func dvIntervalNumericBounds(value any) (lower, upper float64, ok bool) {
	lo, hi, bounded := intervalBounds(value)
	if !bounded {
		return 0, 0, false
	}
	loMag, loUnit, loOK := numericMagnitude(lo)
	hiMag, hiUnit, hiOK := numericMagnitude(hi)
	if !loOK || !hiOK || loUnit != hiUnit {
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

// numericMagnitude lifts a DVOrdered bound to (magnitude, unit, ok). unit is
// the DV_QUANTITY units string (empty for the dimensionless DV_COUNT); two
// bounds are comparable only when their units match. Returns ok=false for any
// other DVOrdered concrete (DV_DATE/TIME/DURATION/ORDINAL/…), which need
// RM-spec-aware comparison handled by REQ-123 follow-ups.
func numericMagnitude(v rm.DVOrdered) (mag float64, unit string, ok bool) {
	switch x := v.(type) {
	case rm.DVQuantity:
		return float64(x.Magnitude), x.Units, true
	case *rm.DVQuantity:
		if x == nil {
			return 0, "", false
		}
		return float64(x.Magnitude), x.Units, true
	case rm.DVCount:
		return float64(x.Magnitude), "", true
	case *rm.DVCount:
		if x == nil {
			return 0, "", false
		}
		return float64(x.Magnitude), "", true
	}
	return 0, "", false
}
