package constraints_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// REQ-107 — every bounded PrimitiveConstraint MUST round-trip
// Validate(ExampleValue()) to zero violations. The table covers all
// 12 implementations with at least one bounded subcase apiece, plus
// unbounded sentinels where the contract documents one.
func TestPrimitiveConstraint_ExampleValueValidates(t *testing.T) {
	cases := []struct {
		name string
		c    constraints.PrimitiveConstraint
	}{
		// CBoolean — true allowed, false allowed, both allowed.
		{"CBoolean/true-only", constraints.CBoolean{TrueValid: true}},
		{"CBoolean/false-only", constraints.CBoolean{FalseValid: true}},
		{"CBoolean/both", constraints.CBoolean{TrueValid: true, FalseValid: true}},

		// CInteger — list, inclusive range, exclusive lower range.
		{"CInteger/list", constraints.CInteger{List: []int64{2, 4, 6}}},
		{"CInteger/range-inclusive", constraints.CInteger{
			Range: constraints.NumericRange{Lower: 0, Upper: 10, LowerInclusive: true, UpperInclusive: true},
		}},
		{"CInteger/range-exclusive-lower", constraints.CInteger{
			Range: constraints.NumericRange{Lower: 0, Upper: 10, UpperInclusive: true},
		}},
		{"CInteger/range-list-intersection", constraints.CInteger{
			Range: constraints.NumericRange{Lower: 0, Upper: 10, LowerInclusive: true, UpperInclusive: true},
			List:  []int64{2, 4, 6},
		}},
		// REQ-107 invariant: when List and Range disagree on the first
		// list entry, ExampleValue picks a member that satisfies both
		// (Validate enforces both membership AND range containment).
		{"CInteger/list-with-out-of-range-leading", constraints.CInteger{
			Range: constraints.NumericRange{Lower: 0, Upper: 10, LowerInclusive: true, UpperInclusive: true},
			List:  []int64{100, 5, 200},
		}},
		{"CInteger/unbounded", constraints.CInteger{}},

		// CReal — list, inclusive range, exclusive lower range.
		{"CReal/list", constraints.CReal{List: []float64{0.5, 1.0}}},
		{"CReal/range-inclusive", constraints.CReal{
			Range: constraints.NumericRange{Lower: 0, Upper: 100, LowerInclusive: true, UpperInclusive: true},
		}},
		{"CReal/range-exclusive-lower", constraints.CReal{
			Range: constraints.NumericRange{Lower: 0, Upper: 100, UpperInclusive: true},
		}},
		// REQ-107 invariant: same list/range disagreement pattern as CInteger.
		{"CReal/list-with-out-of-range-leading", constraints.CReal{
			Range: constraints.NumericRange{Lower: 0, Upper: 10, LowerInclusive: true, UpperInclusive: true},
			List:  []float64{99.9, 5.5, 200.0},
		}},
		{"CReal/unbounded", constraints.CReal{}},

		// CString — closed list, open list (REQ-103 list_open), unbounded.
		{"CString/list", constraints.CString{List: []string{"yes", "no"}}},
		{"CString/open-list", constraints.CString{List: []string{"yes", "no"}, ListOpen: true}},
		// REQ-103 invariant: when Pattern rejects the first list entry,
		// ExampleValue picks an entry that satisfies both.
		{"CString/list-with-pattern-rejecting-leading", constraints.CString{List: []string{"a1", "bb"}, Pattern: "[a-z]+"}},
		{"CString/unbounded", constraints.CString{}},

		// Temporal — every sentinel must parse against its validator.
		{"CDate", constraints.CDate{}},
		{"CTime", constraints.CTime{}},
		{"CDateTime", constraints.CDateTime{}},
		{"CDuration", constraints.CDuration{}},
		// REQ-103 — a pattern that leaves a field out, and a range, bound
		// a temporal constraint: the patterns and ranges the vendored OPTs
		// write, and those of the specification's examples.
		{"CDate/yyyy-??-XX", constraints.CDate{Pattern: "yyyy-??-XX"}},
		{"CDate/yyyy-mm-XX", constraints.CDate{Pattern: "yyyy-mm-XX"}},
		{"CDate/range", constraints.CDate{Range: constraints.TemporalRange{Lower: "2004-05-20", Upper: "2004-06-02", LowerInclusive: true, UpperInclusive: true}}},
		{"CTime/hh:mm:XX", constraints.CTime{Pattern: "hh:mm:XX"}},
		{"CTime/zone", constraints.CTime{Pattern: "hh:mm:ss+hh:mm"}},
		{"CTime/range", constraints.CTime{Range: constraints.TemporalRange{Lower: "09:30:00", LowerUnbounded: false, UpperUnbounded: true}}},
		{"CDateTime/yyyy-mm-ddTHH:MM:SS", constraints.CDateTime{Pattern: "yyyy-mm-ddTHH:MM:SS"}},
		{"CDateTime/yyyy-mm-ddTHH:??:??", constraints.CDateTime{Pattern: "yyyy-mm-ddTHH:??:??"}},
		{"CDateTime/yyyy-mm-ddThh:mm:XX", constraints.CDateTime{Pattern: "yyyy-mm-ddThh:mm:XX"}},
		{"CDateTime/range", constraints.CDateTime{Range: constraints.TemporalRange{LowerUnbounded: true, Upper: "2004-01-01T00:00:00"}}},
		{"CTime/zone-mandatory", constraints.CTime{TimezoneValidity: constraints.ValidityMandatory}},
		{"CDateTime/zone-disallowed", constraints.CDateTime{TimezoneValidity: constraints.ValidityDisallowed}},
		{"CDuration/PY", constraints.CDuration{Pattern: "PY"}},
		{"CDuration/PTM", constraints.CDuration{Pattern: "PTM"}},
		{"CDuration/PY+range", constraints.CDuration{Pattern: "PY", Range: constraints.TemporalRange{Lower: "P0Y", Upper: "P150Y", LowerInclusive: true, UpperInclusive: true}}},
		{"CDuration/PYMWD+range", constraints.CDuration{Pattern: "PYMWD", Range: constraints.TemporalRange{Lower: "P4Y", LowerInclusive: true, UpperUnbounded: true}}},
		{"CDuration/range", constraints.CDuration{Range: constraints.TemporalRange{Lower: "PT24H", Upper: "PT24H", LowerInclusive: true, UpperInclusive: true}}},
		{"CDuration/PWD+range", constraints.CDuration{Pattern: "PWD", Range: constraints.TemporalRange{Lower: "P0W", Upper: "P50W", LowerInclusive: true, UpperInclusive: true}}},

		// CodePhrase — closed list, external (terminology only).
		{"CodePhrase/closed", constraints.CodePhrase{Terminology: "openehr", CodeList: []string{"433", "434"}}},
		{"CodePhrase/external", constraints.CodePhrase{Terminology: "SNOMED-CT"}},
		{"CodePhrase/unbounded", constraints.CodePhrase{}},

		// DvQuantity — single unit with inclusive range, multi-unit, open.
		{"DvQuantity/single-unit", constraints.DvQuantity{Units: []constraints.QuantityUnit{
			{Units: "mm[Hg]", Magnitude: constraints.NumericRange{Lower: 0, Upper: 300, LowerInclusive: true, UpperInclusive: true}},
		}}},
		{"DvQuantity/multi-unit", constraints.DvQuantity{Units: []constraints.QuantityUnit{
			{Units: "mm[Hg]", Magnitude: constraints.NumericRange{Lower: 0, Upper: 300, LowerInclusive: true, UpperInclusive: true}},
			{Units: "kPa", Magnitude: constraints.NumericRange{Lower: 0, Upper: 40, LowerInclusive: true, UpperInclusive: true}},
		}}},
		{"DvQuantity/open", constraints.DvQuantity{}},
		// REQ-103: a property's units bound the units when no list does.
		{"DvQuantity/property-units", constraints.DvQuantity{
			Property:      &constraints.CodedTermRef{Terminology: "openehr", CodeString: "125"},
			PropertyUnits: []string{"N/m2", "mm[Hg]", "kPa"},
		}},
		{"DvQuantity/property-units-and-list", constraints.DvQuantity{
			Units:         []constraints.QuantityUnit{{Units: "mg"}},
			PropertyUnits: []string{"N/m2", "mm[Hg]"},
		}},

		// CDvOrdinal — closed value list.
		{"CDvOrdinal/values", constraints.CDvOrdinal{Values: []constraints.OrdinalSymbol{
			{Value: 0, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0001"}},
			{Value: 1, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0002"}},
		}}},

		// ConstraintRef — external value set, with and without a reference.
		{"ConstraintRef/reference", constraints.ConstraintRef{Reference: "ac0001"}},
		{"ConstraintRef/no-reference", constraints.ConstraintRef{}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			v := tt.c.ExampleValue()
			if violations := tt.c.Validate(v); len(violations) != 0 {
				t.Fatalf("Validate(ExampleValue()=%#v) = %v, want no violations", v, violations)
			}
		})
	}
}

// REQ-103 — with a List, open or closed, CString.ExampleValue returns the
// first entry Pattern matches as a whole string, or the first entry when no
// Pattern is set or none matches.
func TestREQ103_CStringExampleValue(t *testing.T) {
	cases := []struct {
		name string
		c    constraints.CString
		want string
	}{
		{name: "open list, no pattern", c: constraints.CString{List: []string{"yes", "no"}, ListOpen: true}, want: "yes"},
		// "a1" contains a match of [a-z]+ but is not one, so the whole-string
		// reading Validate applies skips it.
		{name: "closed list, pattern rejects the first entry", c: constraints.CString{List: []string{"a1", "bb"}, Pattern: "[a-z]+"}, want: "bb"},
		{name: "closed list, pattern compiled by NewCString", c: constraints.NewCString("[a-z]+", []string{"a1", "bb"}, ""), want: "bb"},
		{name: "open list, pattern rejects the first entry", c: constraints.CString{List: []string{"a1", "bb"}, Pattern: "[a-z]+", ListOpen: true}, want: "bb"},
		{name: "closed list, pattern matches no entry", c: constraints.CString{List: []string{"a1", "b2"}, Pattern: "[a-z]+"}, want: "a1"},
		{name: "closed list, malformed pattern", c: constraints.CString{List: []string{"x", "y"}, Pattern: "("}, want: "x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.ExampleValue(); got != tc.want {
				t.Errorf("CString{List: %q, Pattern: %q, ListOpen: %t}.ExampleValue() = %#v, want %q", tc.c.List, tc.c.Pattern, tc.c.ListOpen, got, tc.want)
			}
		})
	}
}

// REQ-103 — with no Units list, DvQuantity.ExampleValue gives magnitude 0
// in the first entry of PropertyUnits, the primary unit the parser puts
// first; a Units list keeps precedence, and with neither the example is
// {0, "1"}. With a Units list the example is that of the first entry whose
// magnitude range admits it, or the first entry's when none does.
func TestREQ103_DvQuantityExampleValue(t *testing.T) {
	cases := []struct {
		name string
		c    constraints.DvQuantity
		want constraints.QuantityValue
	}{
		{
			name: "property units, no list",
			c:    constraints.DvQuantity{PropertyUnits: []string{"N/m2", "mm[Hg]"}},
			want: constraints.QuantityValue{Magnitude: 0, Units: "N/m2", Precision: -1},
		},
		{
			name: "property units and a list",
			c: constraints.DvQuantity{
				Units:         []constraints.QuantityUnit{{Units: "mm[Hg]", Magnitude: constraints.NumericRange{Lower: 5, Upper: 10, LowerInclusive: true, UpperInclusive: true}}},
				PropertyUnits: []string{"N/m2"},
			},
			want: constraints.QuantityValue{Magnitude: 5, Units: "mm[Hg]", Precision: -1},
		},
		{
			name: "neither",
			c:    constraints.DvQuantity{},
			want: constraints.QuantityValue{Magnitude: 0, Units: "1", Precision: -1},
		},
		{
			name: "a first entry that admits no magnitude",
			c: constraints.DvQuantity{Units: []constraints.QuantityUnit{
				{Units: "mg", Magnitude: constraints.NumericRange{Lower: 5, Upper: 3, LowerInclusive: true, UpperInclusive: true}},
				{Units: "g", Magnitude: constraints.NumericRange{Lower: 1, Upper: 10, LowerInclusive: true, UpperInclusive: true}},
			}},
			want: constraints.QuantityValue{Magnitude: 1, Units: "g", Precision: -1},
		},
		{
			name: "no entry that admits a magnitude",
			c: constraints.DvQuantity{Units: []constraints.QuantityUnit{
				{Units: "mg", Magnitude: constraints.NumericRange{Lower: 5, Upper: 3, LowerInclusive: true, UpperInclusive: true}},
				{Units: "g", Magnitude: constraints.NumericRange{Lower: 9, Upper: 2, LowerInclusive: true, UpperInclusive: true}},
			}},
			want: constraints.QuantityValue{Magnitude: 5, Units: "mg", Precision: -1},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.ExampleValue(); got != tc.want {
				t.Errorf("DvQuantity{Units: %v, PropertyUnits: %q}.ExampleValue() = %#v, want %#v", tc.c.Units, tc.c.PropertyUnits, got, tc.want)
			}
		})
	}
}

// REQ-107 — the sealing contract still holds: only the 12 known
// types in this package implement PrimitiveConstraint. A new
// external implementer would break the closed type-switch the
// validator relies on, which stays free of reflection.
//
// REQ-103: each of the twelve OPT primitive xsi:type values has its
// PrimitiveConstraint implementation.
func TestPrimitiveConstraint_ExampleValueSeal(_ *testing.T) {
	// Each entry asserts the type satisfies the full interface
	// (Validate + ExampleValue + isPrimitive). Adding a 13th type
	// to this list MUST coincide with a new REQ row + spec entry.
	var _ constraints.PrimitiveConstraint = constraints.CBoolean{}
	var _ constraints.PrimitiveConstraint = constraints.CInteger{}
	var _ constraints.PrimitiveConstraint = constraints.CReal{}
	var _ constraints.PrimitiveConstraint = constraints.CString{}
	var _ constraints.PrimitiveConstraint = constraints.CDate{}
	var _ constraints.PrimitiveConstraint = constraints.CTime{}
	var _ constraints.PrimitiveConstraint = constraints.CDateTime{}
	var _ constraints.PrimitiveConstraint = constraints.CDuration{}
	var _ constraints.PrimitiveConstraint = constraints.CodePhrase{}
	var _ constraints.PrimitiveConstraint = constraints.DvQuantity{}
	var _ constraints.PrimitiveConstraint = constraints.CDvOrdinal{}
	var _ constraints.PrimitiveConstraint = constraints.ConstraintRef{}
}

// TestREQ107_CodePhraseEmptyListPlaceholder is the REQ-107 check that a
// C_CODE_PHRASE with an empty code list gives the code at0000, under the
// terminology it names, or under local when it names none.
func TestREQ107_CodePhraseEmptyListPlaceholder(t *testing.T) {
	cases := []struct {
		name string
		c    constraints.CodePhrase
		want constraints.CodedTermRef
	}{
		{
			name: "terminology named",
			c:    constraints.CodePhrase{Terminology: "SNOMED-CT"},
			want: constraints.CodedTermRef{Terminology: "SNOMED-CT", CodeString: "at0000"},
		},
		{
			name: "no terminology",
			c:    constraints.CodePhrase{},
			want: constraints.CodedTermRef{Terminology: "local", CodeString: "at0000"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.ExampleValue(); got != tc.want {
				t.Errorf("%#v.ExampleValue() = %#v, want %#v", tc.c, got, tc.want)
			}
		})
	}
}

// TestREQ103_TemporalExampleValue pins REQ-103 § Example value emission on
// the temporal constraints: the example is the first of the sentinel, the
// range's lower bound and its upper bound, each written to the pattern, that
// Validate accepts, so the sentinel stays wherever it satisfies both. A
// value written to the pattern keeps its zone, takes Z where a zone is
// required, and is read in UTC where Z is. Where none is accepted, it is the
// sentinel, which Validate then refuses: when no value satisfies the pattern
// and the range, when the pattern or a bound is malformed, and, in the
// exception the spec names, when a literal field after the first moves each
// bound out of the range though a valid value exists.
func TestREQ103_TemporalExampleValue(t *testing.T) {
	t.Parallel()
	from := func(lower string, included bool) constraints.TemporalRange {
		return constraints.TemporalRange{Lower: lower, LowerInclusive: included, UpperUnbounded: true}
	}
	upTo := func(upper string, included bool) constraints.TemporalRange {
		return constraints.TemporalRange{LowerUnbounded: true, Upper: upper, UpperInclusive: included}
	}
	closed := func(lower, upper string) constraints.TemporalRange {
		return constraints.TemporalRange{Lower: lower, Upper: upper, LowerInclusive: true, UpperInclusive: true}
	}
	tests := []struct {
		name string
		c    constraints.PrimitiveConstraint
		want string
		// unsatisfiable marks a constraint whose example is the sentinel
		// Validate refuses: one no value meets, or the literal exception.
		unsatisfiable bool
	}{
		{name: "date, no pattern", c: constraints.CDate{}, want: "2020-01-01"},
		{name: "time, no pattern", c: constraints.CTime{}, want: "12:00:00"},
		{name: "date-time, no pattern", c: constraints.CDateTime{}, want: "2020-01-01T12:00:00Z"},
		{name: "duration, no pattern", c: constraints.CDuration{}, want: "P0D"},
		{name: "date, day not allowed", c: constraints.CDate{Pattern: "yyyy-??-XX"}, want: "2020-01"},
		{name: "date, month not allowed", c: constraints.CDate{Pattern: "yyyy-XX-XX"}, want: "2020"},
		{name: "date, literal year", c: constraints.CDate{Pattern: "1995-??-XX"}, want: "1995-01"},
		{name: "date, full", c: constraints.CDate{Pattern: "yyyy-mm-dd"}, want: "2020-01-01"},
		{name: "time, no seconds", c: constraints.CTime{Pattern: "hh:??:XX"}, want: "12:00"},
		{name: "time, zone Z", c: constraints.CTime{Pattern: "hh:mm:ssZ"}, want: "12:00:00Z"},
		{name: "time, zone required", c: constraints.CTime{Pattern: "hh:mm:ss±hh:mm"}, want: "12:00:00Z"},
		{name: "date-time, no seconds", c: constraints.CDateTime{Pattern: "yyyy-mm-ddTHH:MM:XX"}, want: "2020-01-01T12:00Z"},
		{name: "date-time, optional seconds", c: constraints.CDateTime{Pattern: "yyyy-mm-ddTHH:??:??"}, want: "2020-01-01T12:00:00Z"},
		{name: "date-time, no time", c: constraints.CDateTime{Pattern: "yyyy-mm-ddTXX:XX:XX"}, want: "2020-01-01"},
		{name: "duration, years", c: constraints.CDuration{Pattern: "PY"}, want: "P0Y"},
		{name: "duration, minutes", c: constraints.CDuration{Pattern: "PTM"}, want: "PT0M"},
		{name: "duration, days allowed", c: constraints.CDuration{Pattern: "PMWD"}, want: "P0D"},
		{name: "duration, years in range", c: constraints.CDuration{Pattern: "PY", Range: closed("P0Y", "P150Y")}, want: "P0Y"},
		{name: "duration, hours in range", c: constraints.CDuration{Pattern: "PTH", Range: closed("PT24H", "PT24H")}, want: "PT24H"},
		{name: "duration, range only", c: constraints.CDuration{Range: closed("PT5M", "PT5M")}, want: "PT5M"},
		{name: "duration, at least 4 years", c: constraints.CDuration{Pattern: "PYMWD", Range: from("P4Y", true)}, want: "P4Y"},
		{name: "duration, bound in another designator", c: constraints.CDuration{Pattern: "PY", Range: from("PT0S", true)}, want: "P0Y"},
		{name: "duration, excluded bound", c: constraints.CDuration{Pattern: "PD", Range: from("P1Y", false)}, want: "P366D"},
		{name: "duration, upper bound only", c: constraints.CDuration{Pattern: "PD", Range: upTo("P0D", false)}, want: "-P1D"},
		{name: "duration, no years in an hour", c: constraints.CDuration{Pattern: "PY", Range: constraints.TemporalRange{Lower: "PT0S", Upper: "PT1H"}}, want: "P0D", unsatisfiable: true},
		{name: "duration, malformed pattern", c: constraints.CDuration{Pattern: "PX"}, want: "P0D", unsatisfiable: true},
		{name: "duration, malformed bound", c: constraints.CDuration{Range: from("P1X", true)}, want: "P0D", unsatisfiable: true},
		{name: "date, range", c: constraints.CDate{Range: closed("2004-05-20", "2004-06-02")}, want: "2004-05-20"},
		{name: "date, day dropped inside the range", c: constraints.CDate{Pattern: "yyyy-mm-XX", Range: closed("2004-05-20", "2004-06-02")}, want: "2004-06"},
		{name: "date, sentinel inside the range", c: constraints.CDate{Range: from("2004-05-20", false)}, want: "2020-01-01"},
		{name: "date, excluded lower", c: constraints.CDate{Range: from("2024-05-20", false)}, want: "2024-05-21"},
		{name: "date, excluded lower at the year end", c: constraints.CDate{Range: from("2024-12-31", false)}, want: "2025-01-01"},
		{name: "date, excluded upper", c: constraints.CDate{Range: upTo("2004-03-01", false)}, want: "2004-02-29"},
		{name: "date, literal year outside the range", c: constraints.CDate{Pattern: "1995-??-XX", Range: from("2000", true)}, want: "2020-01-01", unsatisfiable: true},
		// The documented exception: 2003-06-01 meets both, but the literal
		// month moves the sentinel and the bound past the range.
		{name: "date, literal month beside a range", c: constraints.CDate{Pattern: "yyyy-06-dd", Range: upTo("2004-05-20", true)}, want: "2020-01-01", unsatisfiable: true},
		{name: "time, hour rounded up", c: constraints.CTime{Pattern: "hh:XX:XX", Range: closed("09:30:00", "10:00:00")}, want: "10"},
		{name: "time, excluded lower", c: constraints.CTime{Range: from("13:30:00", false)}, want: "13:30:01"},
		{name: "time, zone Z, sentinel after the bound as instants", c: constraints.CTime{Pattern: "hh:mm:ssZ", Range: from("13:30:00+02:00", true)}, want: "12:00:00Z"},
		{name: "time, zone Z, bound read in UTC", c: constraints.CTime{Pattern: "hh:mm:ssZ", Range: closed("13:30:00+02:00", "13:45:00+02:00")}, want: "11:30:00Z"},
		{name: "time, zone Z, bound read in UTC leaves the day", c: constraints.CTime{Pattern: "hh:mm:ssZ", Range: closed("01:00:00+02:00", "01:30:00+02:00")}, want: "12:00:00", unsatisfiable: true},
		{name: "time, zone required, bound keeps its zone", c: constraints.CTime{Pattern: "hh:mm:XX+hh:mm", Range: closed("13:30:30+02:00", "13:32:00+02:00")}, want: "13:31+02:00"},
		{name: "time, sentinel not ordered against a zoned bound", c: constraints.CTime{Range: from("13:30:00+02:00", false)}, want: "12:00:00"},
		{name: "date-time, minute rounded up", c: constraints.CDateTime{Pattern: "yyyy-mm-ddThh:mm:XX", Range: from("2024-05-20T10:15:30Z", true)}, want: "2024-05-20T10:16Z"},
		{name: "date-time, bound keeps its zone", c: constraints.CDateTime{Pattern: "yyyy-mm-ddThh:mm:XX", Range: closed("2024-05-20T10:15:30+02:00", "2024-05-20T10:17:00+02:00")}, want: "2024-05-20T10:16+02:00"},
		{name: "date-time, excluded upper across midnight", c: constraints.CDateTime{Pattern: "yyyy-mm-ddThh:XX:XX", Range: upTo("2004-01-01T00:00:00Z", false)}, want: "2003-12-31T23Z"},
		{name: "date-time, sentinel not ordered against a date bound", c: constraints.CDateTime{Pattern: "yyyy-mm-ddThh:mm:ss", Range: from("2024-05-20", true)}, want: "2020-01-01T12:00:00Z"},
		{name: "time, zone mandatory", c: constraints.CTime{TimezoneValidity: constraints.ValidityMandatory}, want: "12:00:00Z"},
		{name: "time, zone disallowed", c: constraints.CTime{Pattern: "hh:mm:XX", TimezoneValidity: constraints.ValidityDisallowed}, want: "12:00"},
		{name: "time, zone mandatory, bound in range", c: constraints.CTime{Range: closed("13:30:00+02:00", "13:45:00+02:00"), TimezoneValidity: constraints.ValidityMandatory}, want: "13:30:00+02:00"},
		{name: "time, zone disallowed, bound in range", c: constraints.CTime{Pattern: "hh:mm:ss", Range: closed("13:30:00", "13:45:00"), TimezoneValidity: constraints.ValidityDisallowed}, want: "13:30:00"},
		{name: "date-time, zone mandatory", c: constraints.CDateTime{Pattern: "yyyy-mm-ddTHH:MM:XX", TimezoneValidity: constraints.ValidityMandatory}, want: "2020-01-01T12:00Z"},
		{name: "date-time, zone disallowed", c: constraints.CDateTime{TimezoneValidity: constraints.ValidityDisallowed}, want: "2020-01-01T12:00:00"},
		{name: "date-time, date bound, zone disallowed", c: constraints.CDateTime{Pattern: "yyyy-mm-ddThh:mm:ss", Range: from("2024-05-20", true), TimezoneValidity: constraints.ValidityDisallowed}, want: "2024-05-20T00:00:00"},
		{name: "date-time, zone disallowed against a zoned bound", c: constraints.CDateTime{Range: upTo("2004-01-01T00:00:00+01:00", true), TimezoneValidity: constraints.ValidityDisallowed}, want: "2020-01-01T12:00:00"},
		{name: "date, zone mandatory", c: constraints.CDate{TimezoneValidity: constraints.ValidityMandatory}, want: "2020-01-01", unsatisfiable: true},
		{name: "time, zone suffix and zone disallowed", c: constraints.CTime{Pattern: "hh:mm:ss+hh", TimezoneValidity: constraints.ValidityDisallowed}, want: "12:00:00", unsatisfiable: true},
		{name: "duration, zero inside the range", c: constraints.CDuration{Range: from("PT0S", true)}, want: "P0D"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.c.ExampleValue()
			if got != tc.want {
				t.Errorf("ExampleValue() = %#v, want %q", got, tc.want)
			}
			violations := tc.c.Validate(got)
			switch {
			case tc.unsatisfiable && len(violations) == 0:
				t.Errorf("Validate(ExampleValue()=%#v) = no violation, want the sentinel refused", got)
			case !tc.unsatisfiable && len(violations) != 0:
				t.Errorf("Validate(ExampleValue()=%#v) = %v, want no violation", got, violations)
			}
		})
	}
}
