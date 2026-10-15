package validation

// rmfloor_adapters.go: adapters that lift a polymorphic RM value into the
// concrete shape the RM rules' invariant evaluators ([findings]) need. Most
// are closed type switches, in the discipline of rmTypeInfo/describeRMType
// in composition.go, so a new BMM concrete they read means editing the
// switch that reads it. The DV_INTERVAL bound reader ([intervalBounds]) is
// not a switch: it reads the bounds through the generated access, which
// covers every DV_INTERVAL instantiation the registry knows.

import (
	"github.com/cadasto/openehr-sdk-go/internal/rmaccess"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

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

// asPartyIdentified recovers the PARTY_IDENTIFIED attributes (by value or by
// pointer) from a PARTY_IDENTIFIED, or from a PARTY_RELATED, which inherits
// them through its embedded PARTY_IDENTIFIED. Returns ok=false for a nil
// pointer and for any other value.
func asPartyIdentified(value any) (rm.PartyIdentified, bool) {
	switch v := value.(type) {
	case *rm.PartyIdentified:
		if v == nil {
			return rm.PartyIdentified{}, false
		}
		return *v, true
	case rm.PartyIdentified:
		return v, true
	case *rm.PartyRelated:
		if v == nil {
			return rm.PartyIdentified{}, false
		}
		return v.PartyIdentified, true
	case rm.PartyRelated:
		return v.PartyIdentified, true
	}
	return rm.PartyIdentified{}, false
}

// temporalValue returns the `value` string of a DV_DATE_TIME / DV_DATE /
// DV_TIME / DV_DURATION (by value or by pointer) and reports whether it
// satisfies the type's ISO 8601 predicate, using the REQ-123 parse. ok is
// false for any other value and for a nil pointer.
func temporalValue(value any) (text string, valid, ok bool) {
	switch v := value.(type) {
	case *rm.DVDateTime:
		if v == nil {
			return "", false, false
		}
		return v.Value, v.ValidISO8601(), true
	case rm.DVDateTime:
		return v.Value, v.ValidISO8601(), true
	case *rm.DVDate:
		if v == nil {
			return "", false, false
		}
		return v.Value, v.ValidISO8601(), true
	case rm.DVDate:
		return v.Value, v.ValidISO8601(), true
	case *rm.DVTime:
		if v == nil {
			return "", false, false
		}
		return v.Value, v.ValidISO8601(), true
	case rm.DVTime:
		return v.Value, v.ValidISO8601(), true
	case *rm.DVDuration:
		if v == nil {
			return "", false, false
		}
		return v.Value, v.ValidISO8601(), true
	case rm.DVDuration:
		return v.Value, v.ValidISO8601(), true
	}
	return "", false, false
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

// intervalOrder is how the floor reads the two bounds of a DV_INTERVAL that
// it orders.
type intervalOrder struct {
	// class is the RM type of the two bounds.
	class string
	// inverted reports that lower is above upper by the RM's less_than for
	// class.
	inverted bool
	// magnitudes are the two values the order compares, lower first.
	magnitudes [2]float64
}

// dvIntervalOrder reads the order of the two bounds of a DV_INTERVAL when
// neither side is unbounded and the floor orders the pair. The RM states the
// rule as DV_INTERVAL.Limits_consistent: with neither side open,
// lower.is_strictly_comparable_to(upper) and lower <= upper, where `<=` is
// the less_than of the bounds' type, whose precondition Pre_comparable is
// is_strictly_comparable_to(other). So the floor orders two bounds only when
// the RM makes them strictly comparable ([orderBounds] says when). It reads
// every DV_INTERVAL the RM types hold, typed (DVInterval[DVQuantity] is a
// DV_QUANTITY's normal_range, DVInterval[DVDate] a PARTY's time_validity) or
// bare (DVInterval[DVOrdered], which the registry decodes), by value or by
// pointer. ok is false for an unbounded side, where the comparison is
// undefined, and for a pair the floor does not order.
//
// The check is a conservative subset of Limits_consistent: it tests
// lower <= upper only where the RM defines that comparison, and reports
// nothing for a pair that is not strictly comparable, although the invariant
// cannot hold for one. That half is Interval.Limits_comparable, which the
// floor does not evaluate, so Coverage lists it.
func dvIntervalOrder(value any) (order intervalOrder, ok bool) {
	lower, upper, bounded := intervalBounds(value)
	if !bounded {
		return intervalOrder{}, false
	}
	return orderBounds(lower, upper)
}

// intervalBounds returns the lower and upper bounds of a DV_INTERVAL when
// neither side is unbounded. It reads the flags and the bounds through the
// generated access ([rmaccess.Single]), which serves every DV_INTERVAL
// instantiation the registry knows, typed or bare, by value or by pointer, so
// an instantiation a BMM bump adds is read with no change here. A typed
// interval gives the T it holds, a bare one whatever its DV_ORDERED interface
// holds; [orderBounds] reads either, and does not order a typed-nil bound.
//
// bounded is false for a value whose class is not DV_INTERVAL, such as a BASE
// Point_interval or Proper_interval, whose Limits_consistent is out of scope;
// for a side marked unbounded, even where it still holds a bound; for a nil
// bound; and for a nil interval.
func intervalBounds(value any) (lower, upper rm.DVOrdered, bounded bool) {
	if class, _ := rm.RMTypeName(value); class != "DV_INTERVAL" {
		return nil, nil, false
	}
	if intervalFlagSet(value, "lower_unbounded") || intervalFlagSet(value, "upper_unbounded") {
		return nil, nil, false
	}
	l, _ := rmaccess.Single(value, "lower")
	u, _ := rmaccess.Single(value, "upper")
	lower, lowerOK := l.(rm.DVOrdered)
	upper, upperOK := u.(rm.DVOrdered)
	if !lowerOK || !upperOK {
		return nil, nil, false
	}
	return lower, upper, true
}

// orderBounds reads the order of two bounds that the RM makes strictly
// comparable, by the RM's less_than for their type, and returns ok=false for
// any other pair. Two bounds of different types are never ordered, a DV_COUNT
// beside a unitless DV_QUANTITY included: no less_than takes both. A nil
// bound names no type, so it is not ordered either. For two bounds of one
// type:
//
//   - DV_COUNT: always comparable; ordered by magnitude.
//   - DV_QUANTITY: comparable when the units are equal; ordered by
//     magnitude. The RM's is_strictly_comparable_to also needs a matching
//     units_system where one is set, which is not compared, so a same-units
//     pair is ordered whatever its units_system.
//   - DV_ORDINAL and DV_SCALE: comparable when both symbols are coded in one
//     named terminology ([codedInOneTerminology]); ordered by value.
//   - DV_PROPORTION: comparable when the type is the same; ordered by
//     magnitude, numerator / denominator. A zero denominator gives no
//     magnitude (the RM's Valid_denominator forbids it), so a pair holding
//     one is not ordered and reports nothing here.
//   - DV_DATE and DV_DURATION: always comparable; ordered by their Compare,
//     which reads the magnitude of each parsed value ([temporalOrder]).
//   - DV_TIME and DV_DATE_TIME: always comparable. Two values written with
//     one time zone designator, or both with none, are ordered by their
//     Compare ([temporalOrder]). Two values whose designators differ are
//     ordered as instants, each by its magnitude less its zone's offset from
//     UTC, partial values and leap seconds included ([instantOrder]). A value
//     with a zone is not ordered against one without.
//
// The generated IsStrictlyComparableTo and LessThan of the non-temporal types
// are not called: they are stubs that panic.
func orderBounds(lower, upper rm.DVOrdered) (intervalOrder, bool) {
	// A nil or typed-nil bound reads as "", which no case below takes.
	class, _ := rm.RMTypeName(lower)
	if upperClass, _ := rm.RMTypeName(upper); upperClass != class {
		return intervalOrder{}, false
	}
	switch class {
	case "DV_COUNT":
		lo, hi := boundAs[rm.DVCount](lower), boundAs[rm.DVCount](upper)
		return numberOrder(class, lo.Magnitude, hi.Magnitude), true
	case "DV_QUANTITY":
		lo, hi := boundAs[rm.DVQuantity](lower), boundAs[rm.DVQuantity](upper)
		if lo.Units != hi.Units {
			return intervalOrder{}, false
		}
		return numberOrder(class, lo.Magnitude, hi.Magnitude), true
	case "DV_ORDINAL":
		lo, hi := boundAs[rm.DVOrdinal](lower), boundAs[rm.DVOrdinal](upper)
		if !codedInOneTerminology(lo.Symbol, hi.Symbol) {
			return intervalOrder{}, false
		}
		return numberOrder(class, lo.Value, hi.Value), true
	case "DV_SCALE":
		lo, hi := boundAs[rm.DVScale](lower), boundAs[rm.DVScale](upper)
		if !codedInOneTerminology(lo.Symbol, hi.Symbol) {
			return intervalOrder{}, false
		}
		return numberOrder(class, lo.Value, hi.Value), true
	case "DV_PROPORTION":
		lo, hi := boundAs[rm.DVProportion](lower), boundAs[rm.DVProportion](upper)
		if lo.Type != hi.Type || lo.Denominator == 0 || hi.Denominator == 0 {
			return intervalOrder{}, false
		}
		return numberOrder(class, lo.Numerator/lo.Denominator, hi.Numerator/hi.Denominator), true
	case "DV_DATE":
		lo, hi := boundAs[rm.DVDate](lower), boundAs[rm.DVDate](upper)
		return temporalOrder(class, lo.Compare(hi), float64(lo.Magnitude()), float64(hi.Magnitude()), lo.ValidISO8601(), hi.ValidISO8601())
	case "DV_TIME":
		lo, hi := boundAs[rm.DVTime](lower), boundAs[rm.DVTime](upper)
		if loZone, hiZone := lo.Timezone(), hi.Timezone(); loZone != hiZone {
			return instantOrder(class, loZone, hiZone, float64(lo.Magnitude()), float64(hi.Magnitude()))
		}
		return temporalOrder(class, lo.Compare(hi), float64(lo.Magnitude()), float64(hi.Magnitude()), lo.ValidISO8601(), hi.ValidISO8601())
	case "DV_DATE_TIME":
		lo, hi := boundAs[rm.DVDateTime](lower), boundAs[rm.DVDateTime](upper)
		if loZone, hiZone := lo.Timezone(), hi.Timezone(); loZone != hiZone {
			return instantOrder(class, loZone, hiZone, lo.Magnitude(), hi.Magnitude())
		}
		return temporalOrder(class, lo.Compare(hi), lo.Magnitude(), hi.Magnitude(), lo.ValidISO8601(), hi.ValidISO8601())
	case "DV_DURATION":
		lo, hi := boundAs[rm.DVDuration](lower), boundAs[rm.DVDuration](upper)
		return temporalOrder(class, lo.Compare(hi), lo.Magnitude(), hi.Magnitude(), lo.ValidISO8601(), hi.ValidISO8601())
	}
	return intervalOrder{}, false
}

// boundAs returns a DV_INTERVAL bound as the T it holds, whether the interval
// holds it by value, as a typed interval does, or by pointer, as the bare
// form the registry decodes does. Any other bound reads as the zero T;
// [orderBounds] asks only for the type the bound names.
func boundAs[T rm.DVOrdered](bound rm.DVOrdered) T {
	// The switch is on any: Go does not count *T as a DVOrdered, although
	// every *T the bound can hold is one.
	switch b := any(bound).(type) {
	case T:
		return b
	case *T:
		if b != nil {
			return *b
		}
	}
	var zero T
	return zero
}

// numberOrder orders two numbers of class, lower against upper. A NaN is
// neither above nor below another number, so a pair holding one is read as
// in order.
func numberOrder[N ~int32 | ~int64 | ~float64](class string, lower, upper N) intervalOrder {
	return intervalOrder{class: class, inverted: lower > upper, magnitudes: [2]float64{float64(lower), float64(upper)}}
}

// temporalOrder orders two temporal bounds of class. sign is the Compare of
// lower with upper ([rm.DVDate.Compare] and its kin), which reads the
// magnitude of each parsed value, and lowerMag and upperMag are those
// magnitudes. A value the parse refuses has no magnitude (Compare would read
// it as zero), so a pair that holds one is not ordered. An empty value is
// reported by the mandatory-presence rule, as `required` at the bound's
// …/value; any other value the parse refuses, by the bound's own Value_valid.
// The magnitude of a DV_TIME or a DV_DATE_TIME reads the clock in the value's
// own time zone, so [orderBounds] sends two of them here only when they are
// written with one zone designator, or both with none, and sends the others
// to [instantOrder].
func temporalOrder(class string, sign int, lowerMag, upperMag float64, lowerValid, upperValid bool) (intervalOrder, bool) {
	if !lowerValid || !upperValid {
		return intervalOrder{}, false
	}
	return intervalOrder{class: class, inverted: sign > 0, magnitudes: [2]float64{lowerMag, upperMag}}, true
}

// instantOrder orders two DV_TIME or two DV_DATE_TIME bounds of class whose
// time zone designators, lowerZone and upperZone, differ. Their magnitudes,
// lowerMag and upperMag, read each clock in its own zone, so they are not
// compared as they stand: each value is ordered by its magnitude less its
// zone's offset from UTC, which is its clock read in UTC, and those are the
// magnitudes the order compares. That holds for every value the parse
// accepts: a partial value is ordered by its magnitude, as it is in one zone,
// and so is a value that holds a leap second. Two designators of one offset,
// Z and +00:00 or +05:00 and +0500, order a pair as its clocks would.
//
// The pair is not ordered when either value has no zone: a value without one
// names no instant to set against a value with one. A value the parse refuses
// reads as having no zone, so it is not ordered either. It is reported as
// [temporalOrder] says: an empty value as `required` at its …/value, any
// other by its own Value_valid.
func instantOrder(class, lowerZone, upperZone string, lowerMag, upperMag float64) (intervalOrder, bool) {
	lowerOffset, lowerZoned := zoneOffset(lowerZone)
	upperOffset, upperZoned := zoneOffset(upperZone)
	if !lowerZoned || !upperZoned {
		return intervalOrder{}, false
	}
	lowerUTC, upperUTC := lowerMag-float64(lowerOffset), upperMag-float64(upperOffset)
	return intervalOrder{class: class, inverted: lowerUTC > upperUTC, magnitudes: [2]float64{lowerUTC, upperUTC}}, true
}

// zoneOffset returns the offset from UTC, in seconds east, of a time zone
// designator that the REQ-123 parse read from a DV_TIME or a DV_DATE_TIME: Z,
// ±hh, ±hhmm or ±hh:mm. It reads the offset through that same parse, as the
// zone of a full DV_TIME at midnight. ok is false when tz is empty, since the
// value then names no zone. It is false as well if the parse refuses tz,
// which it does not do for a designator it has read.
func zoneOffset(tz string) (offset int, ok bool) {
	if tz == "" {
		return 0, false
	}
	midnight, err := (&rm.DVTime{Value: "00:00:00" + tz}).ToTime()
	if err != nil {
		return 0, false
	}
	_, offset = midnight.Zone()
	return offset, true
}

// codedInOneTerminology reports whether the symbols of two DV_ORDINAL or
// DV_SCALE bounds are coded in one named terminology: their terminology ids
// are equal and not empty. A symbol with an empty terminology id is coded in
// none, so it is comparable with no symbol, another such symbol included.
func codedInOneTerminology(a, b rm.DVCodedText) bool {
	id := a.DefiningCode.TerminologyID.Value
	return id != "" && id == b.DefiningCode.TerminologyID.Value
}
