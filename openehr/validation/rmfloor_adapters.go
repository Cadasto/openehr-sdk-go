package validation

// rmfloor_adapters.go: adapters that lift a polymorphic RM value into the
// concrete shape the RM rules' invariant evaluators ([findings]) need. Most
// are closed type switches, in the discipline of rmTypeInfo/describeRMType
// in composition.go, so a new BMM concrete they read means editing the
// switch that reads it. The DV_INTERVAL bound reader ([intervalBounds]) is
// not a switch: it reads the bounds through the generated access, which
// covers every DV_INTERVAL instantiation the registry knows.

import (
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/iso8601"
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

// asDVEHRURI recovers a DV_EHR_URI value (by value or by pointer). It is
// not a DV_URI: only the class whose Scheme_valid invariant fixes the scheme
// matches.
func asDVEHRURI(value any) (rm.DVEHRURI, bool) {
	switch v := value.(type) {
	case *rm.DVEHRURI:
		if v == nil {
			return rm.DVEHRURI{}, false
		}
		return *v, true
	case rm.DVEHRURI:
		return v, true
	}
	return rm.DVEHRURI{}, false
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
	// inverted reports that lower is above upper by the order
	// [orderBounds] gives class.
	inverted bool
	// magnitudes are the two values the order compares, lower first, and
	// for two dates, times or date-times ordered at the precision they
	// share, which compares no single value, their magnitudes, a zoned
	// date-time's read in UTC. They are reported as read and need not show
	// the inversion: two values closer than a float64 magnitude can tell
	// apart read equal, and a leap second, which the magnitude counts into
	// the next minute, can put 2021-12-31T23:59:60.5 above 2022-01-01.
	magnitudes [2]float64
}

// notStrictlyComparable reports whether the floor reads two bounds of a
// DV_INTERVAL, neither side unbounded, as not strictly comparable, and gives
// the Detail of the finding that says so. The RM states the rule as
// DV_INTERVAL.Limits_consistent: with neither side open,
// lower.is_strictly_comparable_to(upper) and lower <= upper. Each class
// defines is_strictly_comparable_to against its own class, so bounds of two
// classes are not strictly comparable, a DV_COUNT beside another DV_ORDERED
// included, although DV_COUNT's definition sets no condition. Of two bounds
// of one class:
//
//   - two DV_QUANTITY are not when their units differ, an empty units
//     included. The RM's definition also asks for a matching units_system
//     where one is set, which is not compared;
//   - two DV_PROPORTION are not when their types differ.
//
// Every other pair of one class is read as strictly comparable, two
// DV_ORDINAL or DV_SCALE coded in different terminologies or in none
// included: the RM leaves theirs undefined, so the floor reports nothing for
// them, and orders them only where [codedInOneTerminology] holds
// ([orderBounds]). A nil or typed-nil bound names no class, so a pair that
// holds one is not reported either.
//
// The Detail names the classes of the bounds and never a bound's value,
// units or type (REQ-168).
func notStrictlyComparable(lower, upper rm.DVOrdered) (detail string, apart bool) {
	lowerClass, _ := rm.RMTypeName(lower)
	upperClass, _ := rm.RMTypeName(upper)
	switch {
	case lowerClass == "" || upperClass == "":
		return "", false
	case lowerClass != upperClass:
		return "DV_INTERVAL: lower " + lowerClass + " and upper " + upperClass + " are not strictly comparable (RM Limits_consistent)", true
	case lowerClass == "DV_QUANTITY" && boundAs[rm.DVQuantity](lower).Units != boundAs[rm.DVQuantity](upper).Units:
		return "DV_INTERVAL<DV_QUANTITY>: lower and upper are not strictly comparable, their units differ (RM Limits_consistent)", true
	case lowerClass == "DV_PROPORTION" && boundAs[rm.DVProportion](lower).Type != boundAs[rm.DVProportion](upper).Type:
		return "DV_INTERVAL<DV_PROPORTION>: lower and upper are not strictly comparable, their types differ (RM Limits_consistent)", true
	}
	return "", false
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
// comparable, by the RM's less_than for their type, except where the list
// below orders two dates, times or date-times otherwise, and returns
// ok=false for any other pair, and for a pair it has no order for. The
// RM's less_than, which DV_INTERVAL.Limits_consistent reads as `<=`, has
// the precondition Pre_comparable, is_strictly_comparable_to(other), so the
// floor orders two bounds only when the RM makes them strictly comparable;
// a pair it reads as not strictly comparable is reported as that
// ([notStrictlyComparable]).
// Two bounds of different types are never ordered, a DV_COUNT beside a
// unitless DV_QUANTITY included: no less_than takes both. A nil bound names
// no type, so it is not ordered either. For two bounds of one type:
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
//   - DV_DATE: always comparable; ordered at the precision the two share
//     ([iso8601.Fields.Compare], [temporalOrder]): field by field from the
//     year down, over the fields both carry, so a partial value covers the
//     whole period it names and 2021-05 is equal to 2021. This departs from
//     the RM's less_than, whose magnitude reads a partial value as the
//     start of its period; the REQ-103 temporal range check compares a
//     value with a bound the same way, so the two agree.
//   - DV_TIME: always comparable. Two values written with one time zone
//     designator, or both with none, are ordered at the precision they
//     share, as two dates are, on their clocks as written. Two values whose
//     designators differ are ordered as instants, each by its magnitude less
//     its zone's offset from UTC, partial values and leap seconds included
//     ([instantOrder]). A value with a zone is not ordered against one
//     without.
//   - DV_DATE_TIME: always comparable. Two values written with one zone
//     designator, or both with none, are ordered at the precision they
//     share, as two dates are, a date with no time carrying no time fields.
//     Two zoned values whose designators differ are ordered by their
//     Compare, whose magnitude reads each in UTC, so as instants, partial
//     values and leap seconds included. A value with a zone is not ordered
//     against one without.
//   - DV_DURATION: always comparable; ordered by its Compare, which reads
//     the magnitude of each parsed value ([temporalOrder]).
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
		sign := dateFields(lo.Value).Compare(dateFields(hi.Value))
		return temporalOrder(class, sign, float64(lo.Magnitude()), float64(hi.Magnitude()), lo.ValidISO8601(), hi.ValidISO8601())
	case "DV_TIME":
		lo, hi := boundAs[rm.DVTime](lower), boundAs[rm.DVTime](upper)
		if loZone, hiZone := lo.Timezone(), hi.Timezone(); loZone != hiZone {
			return instantOrder(class, loZone, hiZone, float64(lo.Magnitude()), float64(hi.Magnitude()))
		}
		sign := timeFields(lo.Value).Compare(timeFields(hi.Value))
		return temporalOrder(class, sign, float64(lo.Magnitude()), float64(hi.Magnitude()), lo.ValidISO8601(), hi.ValidISO8601())
	case "DV_DATE_TIME":
		lo, hi := boundAs[rm.DVDateTime](lower), boundAs[rm.DVDateTime](upper)
		loZone, hiZone := lo.Timezone(), hi.Timezone()
		var sign int
		switch {
		case loZone == hiZone:
			sign = dateTimeFields(lo.Value).Compare(dateTimeFields(hi.Value))
		case loZone == "" || hiZone == "":
			return intervalOrder{}, false
		default:
			sign = lo.Compare(hi)
		}
		return temporalOrder(class, sign, lo.Magnitude(), hi.Magnitude(), lo.ValidISO8601(), hi.ValidISO8601())
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

// temporalOrder orders two temporal bounds of class. sign is the order of
// lower against upper that [orderBounds] reads: for two dates, and for two
// times or date-times written with one zone designator or both with none,
// the order at the precision the two share ([iso8601.Fields.Compare]); for
// two zoned date-times whose designators differ, and for two durations,
// their Compare ([rm.DVDateTime.Compare], [rm.DVDuration.Compare]), which
// reads the magnitude of each parsed value. lowerMag and upperMag are the
// magnitudes of the two values, which the finding's Value carries. A value
// the parse refuses has no fields and no magnitude (each would read as
// zero), so a pair that holds one is not ordered. An empty value is
// reported by the mandatory-presence rule, as `required` at the bound's
// …/value; any other value the parse refuses, by the bound's own
// Value_valid. The magnitude of a DV_TIME reads the clock in the value's own
// time zone, so [orderBounds] sends two times whose designators differ to
// [instantOrder] instead. The magnitude of a DV_DATE_TIME reads a value with
// a zone in UTC, so its Compare orders two zoned date-times as instants.
func temporalOrder(class string, sign int, lowerMag, upperMag float64, lowerValid, upperValid bool) (intervalOrder, bool) {
	if !lowerValid || !upperValid {
		return intervalOrder{}, false
	}
	return intervalOrder{class: class, inverted: sign > 0, magnitudes: [2]float64{lowerMag, upperMag}}, true
}

// instantOrder orders two DV_TIME bounds of class whose time zone
// designators, lowerZone and upperZone, differ. Their magnitudes,
// lowerMag and upperMag, read each clock in its own zone, so they are not
// compared as they stand: each value is ordered by its magnitude less its
// zone's offset from UTC, which is its clock read in UTC, and those are the
// magnitudes the order compares. That holds for every value the parse
// accepts: a partial value is ordered by its magnitude, as it is in one zone,
// and so is a value that holds a leap second. Two designators of one offset,
// Z and +00:00 or +05:00 and +0500, order a pair as its clocks would. The
// subtraction is [iso8601.UTCMagnitude], which the REQ-103 temporal range
// check shares, so the two order zoned values alike.
//
// The pair is not ordered when either value has no zone: a value without one
// names no instant to set against a value with one. A value the parse refuses
// reads as having no zone, so it is not ordered either. It is reported as
// [temporalOrder] says: an empty value as `required` at its …/value, any
// other by its own Value_valid.
func instantOrder(class, lowerZone, upperZone string, lowerMag, upperMag float64) (intervalOrder, bool) {
	lowerUTC, lowerZoned := iso8601.UTCMagnitude(lowerMag, lowerZone)
	upperUTC, upperZoned := iso8601.UTCMagnitude(upperMag, upperZone)
	if !lowerZoned || !upperZoned {
		return intervalOrder{}, false
	}
	return intervalOrder{class: class, inverted: lowerUTC > upperUTC, magnitudes: [2]float64{lowerUTC, upperUTC}}, true
}

// dateFields, timeFields and dateTimeFields read the value of a DV_DATE,
// DV_TIME or DV_DATE_TIME field by field as written, its zone not applied
// ([iso8601.Fields]), for the order at the precision two bounds share. A
// value the parse refuses reads as zero fields, which [temporalOrder] does
// not order.
func dateFields(s string) iso8601.Fields {
	d, _ := iso8601.ParseDate(s)
	return iso8601.DateFields(d)
}

func timeFields(s string) iso8601.Fields {
	t, _ := iso8601.ParseTime(s)
	return iso8601.TimeFields(t)
}

// dateTimeFields reads a date-time's time only where its text holds a "T":
// a date carries none, so a date-time the parse accepts has a time exactly
// when it holds one.
func dateTimeFields(s string) iso8601.Fields {
	d, t, _ := iso8601.ParseDateTime(s)
	return iso8601.DateTimeFields(d, t, strings.Contains(s, "T"))
}

// codedInOneTerminology reports whether the symbols of two DV_ORDINAL or
// DV_SCALE bounds are coded in one named terminology: their terminology ids
// are equal and not empty. A symbol with an empty terminology id is coded in
// none, so it is comparable with no symbol, another such symbol included.
func codedInOneTerminology(a, b rm.DVCodedText) bool {
	id := a.DefiningCode.TerminologyID.Value
	return id != "" && id == b.DefiningCode.TerminologyID.Value
}
