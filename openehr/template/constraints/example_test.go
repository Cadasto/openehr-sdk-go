package constraints_test

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/iso8601"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// REQ-107 — every bounded PrimitiveConstraint MUST round-trip
// Validate(ExampleValue()) to zero violations. The table covers all
// 13 implementations with at least one bounded subcase apiece, plus
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

		// CodeReference — the code phrase it refines, and none.
		{"CodeReference/closed", constraints.CodeReference{
			Terminology:     "SNOMED-CT",
			CodeList:        []string{"22298006"},
			ReferenceSetURI: "terminology:SNOMED-CT",
		}},
		{"CodeReference/unbounded", constraints.CodeReference{ReferenceSetURI: "terminology:SNOMED-CT"}},

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
// in the first entry of PropertyUnits, the primary unit the compile step
// puts first; a Units list keeps precedence, and with neither the example is
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

// REQ-107 — the sealing contract still holds: only the 13 known
// types in this package implement PrimitiveConstraint. A new
// external implementer would break the closed type-switch the
// validator relies on, which stays free of reflection.
//
// REQ-103: each of the thirteen OPT primitive xsi:type values has its
// PrimitiveConstraint implementation.
func TestPrimitiveConstraint_ExampleValueSeal(_ *testing.T) {
	// Each entry asserts the type satisfies the full interface
	// (Validate + ExampleValue + isPrimitive). Adding a 14th type
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
	var _ constraints.PrimitiveConstraint = constraints.CodeReference{}
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
// range's lower bound and its upper bound, each written to the pattern, the
// value halfway between the bounds with its fraction of a second, and, for a
// time or a date-time, each bound written in a zone an hour across, that
// Validate accepts, so the sentinel stays wherever it satisfies both. A
// value written to the pattern keeps its zone, takes Z where a zone is
// required, and is read in UTC where Z is. Written with its own zone, a bound
// equals itself at the precision the two share, so an included one keeps
// its place and an excluded one moves by one unit of the last field they
// share; read in UTC, it moves as an instant. Where none is accepted, it is the
// sentinel, which Validate then refuses: when no value satisfies the pattern
// and the range, when the pattern or a bound is malformed, and, in the
// exceptions the spec names, when a literal field after the first moves each
// bound out of the range though a valid value exists, and when only a value
// the bounds do not order, within 14 hours of a bound with a zone where it
// carries none or the reverse, meets the range.
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
		// Validate refuses: one no value meets, or an exception the spec
		// names.
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
		{name: "date, day dropped inside the range", c: constraints.CDate{Pattern: "yyyy-mm-XX", Range: closed("2004-05-20", "2004-06-02")}, want: "2004-05"},
		{name: "date, excluded year bound under a full pattern", c: constraints.CDate{Pattern: "yyyy-mm-dd", Range: constraints.TemporalRange{Lower: "2021", Upper: "2030", UpperInclusive: true}}, want: "2022-01-01"},
		{name: "date, excluded upper with its day dropped", c: constraints.CDate{Pattern: "yyyy-mm-XX", Range: upTo("2019-05-15", false)}, want: "2019-04"},
		{name: "date, included upper with its day dropped", c: constraints.CDate{Pattern: "yyyy-mm-XX", Range: upTo("2019-05-15", true)}, want: "2019-05"},
		{name: "date, sentinel inside the range", c: constraints.CDate{Range: from("2004-05-20", false)}, want: "2020-01-01"},
		{name: "date, excluded lower", c: constraints.CDate{Range: from("2024-05-20", false)}, want: "2024-05-21"},
		{name: "date, excluded lower at the year end", c: constraints.CDate{Range: from("2024-12-31", false)}, want: "2025-01-01"},
		{name: "date, excluded upper", c: constraints.CDate{Range: upTo("2004-03-01", false)}, want: "2004-02-29"},
		{name: "date, literal year outside the range", c: constraints.CDate{Pattern: "1995-??-XX", Range: from("2000", true)}, want: "2020-01-01", unsatisfiable: true},
		// The documented exception: 2003-06-01 meets both, but the literal
		// month moves the sentinel and the bound past the range.
		{name: "date, literal month beside a range", c: constraints.CDate{Pattern: "yyyy-06-dd", Range: upTo("2004-05-20", true)}, want: "2020-01-01", unsatisfiable: true},
		{name: "time, hour of the lower bound", c: constraints.CTime{Pattern: "hh:XX:XX", Range: closed("09:30:00", "10:00:00")}, want: "09"},
		{name: "time, excluded lower", c: constraints.CTime{Range: from("13:30:00", false)}, want: "13:30:01"},
		{name: "time, excluded hour bound under a full pattern", c: constraints.CTime{Pattern: "hh:mm:ss", Range: from("13", false)}, want: "14:00:00"},
		{name: "time, fraction left out of an excluded lower", c: constraints.CTime{Pattern: "hh:mm:ss", Range: from("13:30:00.5", false)}, want: "13:30:01"},
		{name: "time, fraction left out of an excluded upper", c: constraints.CTime{Pattern: "hh:mm:ss", Range: upTo("11:30:00.5", false)}, want: "11:30:00"},
		{name: "time, zone Z, sentinel after the bound as instants", c: constraints.CTime{Pattern: "hh:mm:ssZ", Range: from("13:30:00+02:00", true)}, want: "12:00:00Z"},
		{name: "time, zone Z, bound read in UTC", c: constraints.CTime{Pattern: "hh:mm:ssZ", Range: closed("13:30:00+02:00", "13:45:00+02:00")}, want: "11:30:00Z"},
		{name: "time, zone Z, bound read in UTC leaves the day", c: constraints.CTime{Pattern: "hh:mm:ssZ", Range: closed("01:00:00+02:00", "01:30:00+02:00")}, want: "12:00:00", unsatisfiable: true},
		{name: "time, zone required, bound keeps its zone", c: constraints.CTime{Pattern: "hh:mm:XX+hh:mm", Range: closed("13:30:30+02:00", "13:32:00+02:00")}, want: "13:30+02:00"},
		{name: "time, zone Z, lower read in UTC drops its seconds", c: constraints.CTime{Pattern: "hh:mm:XXZ", Range: closed("13:30:30+02:00", "13:32:00+02:00")}, want: "11:31Z"},
		{name: "time, sentinel within 14 hours of a zoned bound", c: constraints.CTime{Range: from("13:30:00+02:00", false)}, want: "12:00:00"},
		{name: "date-time, minute of the lower bound", c: constraints.CDateTime{Pattern: "yyyy-mm-ddThh:mm:XX", Range: from("2024-05-20T10:15:30Z", true)}, want: "2024-05-20T10:15Z"},
		{name: "date-time, bound keeps its zone", c: constraints.CDateTime{Pattern: "yyyy-mm-ddThh:mm:XX", Range: closed("2024-05-20T10:15:30+02:00", "2024-05-20T10:17:00+02:00")}, want: "2024-05-20T10:15+02:00"},
		{name: "date-time, excluded date bound under a full pattern", c: constraints.CDateTime{Pattern: "yyyy-mm-ddThh:mm:ss", Range: from("2024-05-20", false), TimezoneValidity: constraints.ValidityDisallowed}, want: "2024-05-21T00:00:00"},
		{name: "date-time, excluded upper across midnight", c: constraints.CDateTime{Pattern: "yyyy-mm-ddThh:XX:XX", Range: upTo("2004-01-01T00:00:00Z", false)}, want: "2003-12-31T23Z"},
		{name: "date-time, sentinel more than 14 hours before a date bound", c: constraints.CDateTime{Pattern: "yyyy-mm-ddThh:mm:ss", Range: from("2024-05-20", true)}, want: "2024-05-20T00:00:00"},
		{name: "date-time, sentinel within 14 hours of a date bound", c: constraints.CDateTime{Pattern: "yyyy-mm-ddThh:mm:ss", Range: from("2020-01-02", true)}, want: "2020-01-01T12:00:00Z"},
		{name: "time, zone mandatory", c: constraints.CTime{TimezoneValidity: constraints.ValidityMandatory}, want: "12:00:00Z"},
		{name: "time, zone disallowed", c: constraints.CTime{Pattern: "hh:mm:XX", TimezoneValidity: constraints.ValidityDisallowed}, want: "12:00"},
		{name: "time, zone mandatory, bound in range", c: constraints.CTime{Range: closed("13:30:00+02:00", "13:45:00+02:00"), TimezoneValidity: constraints.ValidityMandatory}, want: "13:30:00+02:00"},
		{name: "time, zone disallowed, bound in range", c: constraints.CTime{Pattern: "hh:mm:ss", Range: closed("13:30:00", "13:45:00"), TimezoneValidity: constraints.ValidityDisallowed}, want: "13:30:00"},
		{name: "date-time, zone mandatory", c: constraints.CDateTime{Pattern: "yyyy-mm-ddTHH:MM:XX", TimezoneValidity: constraints.ValidityMandatory}, want: "2020-01-01T12:00Z"},
		{name: "date-time, zone disallowed", c: constraints.CDateTime{TimezoneValidity: constraints.ValidityDisallowed}, want: "2020-01-01T12:00:00"},
		{name: "date-time, date bound, zone disallowed", c: constraints.CDateTime{Pattern: "yyyy-mm-ddThh:mm:ss", Range: from("2024-05-20", true), TimezoneValidity: constraints.ValidityDisallowed}, want: "2024-05-20T00:00:00"},
		{name: "date-time, zone disallowed against a zoned bound", c: constraints.CDateTime{Range: upTo("2004-01-01T00:00:00+01:00", true), TimezoneValidity: constraints.ValidityDisallowed}, want: "2004-01-01T00:00:00"},
		{name: "date, zone mandatory", c: constraints.CDate{TimezoneValidity: constraints.ValidityMandatory}, want: "2020-01-01", unsatisfiable: true},
		{name: "time, zone suffix and zone disallowed", c: constraints.CTime{Pattern: "hh:mm:ss+hh", TimezoneValidity: constraints.ValidityDisallowed}, want: "12:00:00", unsatisfiable: true},
		{name: "duration, zero inside the range", c: constraints.CDuration{Range: from("PT0S", true)}, want: "P0D"},
		// A range only a value with a fraction of a second meets takes the
		// value halfway, and one only a time across midnight in its zone
		// meets, the bound written an hour across in its zone.
		{name: "time, excluded lower a second before the end of the day", c: constraints.CTime{Range: from("23:59:59", false)}, want: "23:59:59.5"},
		{name: "time, excluded lower with a fraction before the end of the day", c: constraints.CTime{Range: from("23:59:59.5", false)}, want: "23:59:59.75"},
		{name: "time, minutes, excluded lower in the last minute of its zone's day", c: constraints.CTime{Pattern: "hh:mm:XX±hh:mm", Range: from("23:59Z", false)}, want: "23:00-01:00"},
		{name: "time, excluded upper at the start of its zone's day", c: constraints.CTime{Pattern: "hh:mm:ss±hh:mm", Range: upTo("00:00:00Z", false)}, want: "00:59:59.5+01:00"},
		{name: "date-time, excluded bounds a second apart", c: constraints.CDateTime{Range: constraints.TemporalRange{Lower: "2021-05-15T10:00:00Z", Upper: "2021-05-15T10:00:01Z"}}, want: "2021-05-15T10:00:00.5Z"},
		{name: "date-time, minutes, bounds a second apart, in another zone", c: constraints.CDateTime{Pattern: "yyyy-mm-ddThh:mm:XX", Range: constraints.TemporalRange{Lower: "2021-05-15T10:00:00+02:00", Upper: "2021-05-15T10:00:01+02:00", LowerInclusive: true}, TimezoneValidity: constraints.ValidityMandatory}, want: "2021-05-15T09:00+01:00"},
		{name: "duration, excluded bounds half a second apart", c: constraints.CDuration{Range: constraints.TemporalRange{Lower: "PT0S", Upper: "PT0.5S"}}, want: "PT0.25S"},
		{name: "duration, excluded bounds either side of zero", c: constraints.CDuration{Range: constraints.TemporalRange{Lower: "-PT1S", Upper: "PT0S"}}, want: "-PT0.5S"},
		// The value halfway is written to the microsecond, with no digit
		// the float64 sum leaves past it, unless that reaches a bound.
		{name: "duration, excluded bounds a tenth of a second apart", c: constraints.CDuration{Range: constraints.TemporalRange{Lower: "PT0.1S", Upper: "PT0.2S"}}, want: "PT0.15S"},
		{name: "duration, excluded bounds a tenth of a second apart below zero", c: constraints.CDuration{Range: constraints.TemporalRange{Lower: "-PT0.2S", Upper: "-PT0.1S"}}, want: "-PT0.15S"},
		{name: "duration, excluded bounds a microsecond apart", c: constraints.CDuration{Range: constraints.TemporalRange{Lower: "PT0S", Upper: "PT0.000001S"}}, want: "PT0.0000005S"},
		// The exception the spec names: only a value the bounds do not
		// order, within 14 hours of them, meets the range, 00:00:00Z here,
		// and 2021-05-15T10:00 below.
		{name: "time, only a zoned time meets a range with no zone", c: constraints.CTime{Range: upTo("00:00:00", false)}, want: "12:00:00", unsatisfiable: true},
		{name: "date-time, only a date-time with no zone meets a zoned range", c: constraints.CDateTime{Pattern: "yyyy-mm-ddThh:mm:XX", Range: constraints.TemporalRange{Lower: "2021-05-15T10:00:00Z", Upper: "2021-05-15T10:00:01Z"}}, want: "2020-01-01T12:00:00Z", unsatisfiable: true},
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

// TestREQ103_TemporalExampleValueCloseBoundsAcrossZones pins the upper
// bound's move under REQ-103 § Example value emission when the two excluded
// bounds sit in different zones less than one unit of the upper bound's last
// field apart. The lower bound, moved to the start of its next period, meets
// the upper bound's instant, so only the upper bound can give the example; it
// moves down by one unit of its own last field, to just before its period,
// and so stays above the lower bound's instant. Moved by a whole shared field
// to the start of the period before, it would meet that instant and the
// example would be the sentinel Validate refuses.
func TestREQ103_TemporalExampleValueCloseBoundsAcrossZones(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		c    constraints.PrimitiveConstraint
		want string
	}{
		{
			name: "date-time (10+02:00..09Z)",
			c:    constraints.CDateTime{Range: constraints.TemporalRange{Lower: "2021-05-20T10+02:00", Upper: "2021-05-20T09Z"}, TimezoneValidity: constraints.ValidityMandatory},
			want: "2021-05-20T08:59:59Z",
		},
		{
			name: "time (10+02:00..09Z)",
			c:    constraints.CTime{Range: constraints.TemporalRange{Lower: "10+02:00", Upper: "09Z"}, TimezoneValidity: constraints.ValidityMandatory},
			want: "08:59:59Z",
		},
		{
			name: "time (10:30+02:00..08:31Z)",
			c:    constraints.CTime{Range: constraints.TemporalRange{Lower: "10:30+02:00", Upper: "08:31Z"}, TimezoneValidity: constraints.ValidityMandatory},
			want: "08:30:59Z",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.c.ExampleValue()
			if got != tc.want {
				t.Errorf("ExampleValue() = %#v, want %q", got, tc.want)
			}
			if v := tc.c.Validate(got); len(v) != 0 {
				t.Errorf("Validate(ExampleValue()=%#v) = %v, want no violation", got, v)
			}
		})
	}
}

// TestREQ103_TemporalExampleValueSweep holds REQ-103 § Example value
// emission over a sweep of bounded date, time and date-time constraints: for
// each pattern, full or reduced, and each range from or up to a bound,
// included or excluded, and each range between a lower and an upper bound
// far enough apart, Validate(ExampleValue()) has no violation. The bounds
// are written to every precision, with a fraction of a second, and, for a
// time and a date-time, with a zone the pattern keeps or reads in UTC.
func TestREQ103_TemporalExampleValueSweep(t *testing.T) {
	t.Parallel()
	ranges := func(lows, highs []string) []constraints.TemporalRange {
		var out []constraints.TemporalRange
		for _, b := range append(append([]string(nil), lows...), highs...) {
			for _, in := range []bool{true, false} {
				out = append(out,
					constraints.TemporalRange{Lower: b, LowerInclusive: in, UpperUnbounded: true},
					constraints.TemporalRange{LowerUnbounded: true, Upper: b, UpperInclusive: in})
			}
		}
		for _, lo := range lows {
			for _, hi := range highs {
				for _, flags := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
					out = append(out, constraints.TemporalRange{Lower: lo, Upper: hi, LowerInclusive: flags[0], UpperInclusive: flags[1]})
				}
			}
		}
		return out
	}
	tests := []struct {
		name     string
		patterns []string
		ranges   []constraints.TemporalRange
		build    func(pattern string, r constraints.TemporalRange) constraints.PrimitiveConstraint
	}{
		{
			name:     "date",
			patterns: []string{"", "yyyy-mm-dd", "yyyy-mm-??", "yyyy-??-??", "yyyy-mm-XX", "yyyy-??-XX", "yyyy-XX-XX"},
			ranges: ranges(
				[]string{"2021", "2021-01", "2021-05", "2021-05-15", "2021-12-31"},
				[]string{"2023", "2023-02", "2023-02-28", "2023-01-01"},
			),
			build: func(p string, r constraints.TemporalRange) constraints.PrimitiveConstraint {
				return constraints.CDate{Pattern: p, Range: r}
			},
		},
		{
			name: "time",
			patterns: []string{
				"", "hh:mm:ss", "hh:mm:??", "hh:??:??", "hh:mm:XX", "hh:??:XX", "hh:XX:XX",
				"hh:mm:ssZ", "hh:mm:XXZ", "hh:XX:XXZ", "hh:mm:ss±hh:mm",
			},
			ranges: ranges(
				[]string{"10", "10:30", "10:30:15", "10:30:15.5", "10:59:59.5", "10+02:00", "10:30:15+02:00", "10:59:59.5Z"},
				[]string{"14", "14:00", "14:15:30", "14:15:30.25", "14+02:00", "14:15:30.25+02:00", "14:15Z"},
			),
			build: func(p string, r constraints.TemporalRange) constraints.PrimitiveConstraint {
				return constraints.CTime{Pattern: p, Range: r}
			},
		},
		{
			name: "date-time",
			patterns: []string{
				"", "yyyy-mm-ddThh:mm:ss", "yyyy-mm-ddThh:mm:XX", "yyyy-mm-ddThh:XX:XX", "yyyy-mm-ddTXX:XX:XX",
				"yyyy-mm-XXTXX:XX:XX", "yyyy-XX-XXTXX:XX:XX", "yyyy-mm-ddT??:??:??", "yyyy-??-??T??:??:??",
				"yyyy-mm-ddThh:mm:ssZ", "yyyy-mm-ddThh:mm:XXZ",
			},
			ranges: ranges(
				[]string{
					"2021", "2021-05", "2021-05-15", "2021-05-15T10", "2021-05-15T10:30", "2021-05-15T10:30:15",
					"2021-05-15T10:30:15.5", "2021-05-15T23:59:59.5", "2021-05-15T10:30:15.5Z", "2021-05-15T10:30+02:00",
				},
				[]string{"2023", "2023-02", "2023-02-28", "2023-02-28T14", "2023-02-28T14:15:30.25", "2023-02-28T00:00Z", "2023-02-28T14+02:00"},
			),
			build: func(p string, r constraints.TemporalRange) constraints.PrimitiveConstraint {
				return constraints.CDateTime{Pattern: p, Range: r}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, p := range tc.patterns {
				for _, r := range tc.ranges {
					c := tc.build(p, r)
					got := c.ExampleValue()
					if v := c.Validate(got); len(v) != 0 {
						t.Errorf("pattern %q, range %s: Validate(ExampleValue()=%#v) = %v, want no violation", p, r, got, v)
					}
				}
			}
		})
	}
}

// TestREQ103_TemporalExampleValueNarrowRanges holds REQ-103 § Example value
// emission over a generated sweep of narrow ranges: a time range from a
// bound less than a second, or one unit of its last field, before the end
// of the day, or up to one at its start, and time, date-time and duration
// ranges between two bounds less than a second apart. The bounds are
// excluded or included, and written with no zone, with Z and with +02:00.
// Each constraint takes every pattern the ADL 1.4 grammar admits, with no
// zone suffix, a Z suffix and a ±hh:mm one, under each timezone_validity,
// and a duration every set of designators. Where Validate refuses the
// example, a search of values around each bound, in several zones and with
// fractions of a second, finds none Validate accepts, except in the one
// exception the spec names: every value it finds is one a bound does not
// order, a time with a zone where the bounds carry none, or a date-time
// with none where they carry one, within 14 hours of that bound. A test
// that pins one such case is TestREQ103_TemporalExampleValue.
func TestREQ103_TemporalExampleValueNarrowRanges(t *testing.T) {
	t.Parallel()
	exceptions := 0
	for _, nc := range narrowCases(t) {
		got := nc.c.ExampleValue()
		if len(nc.c.Validate(got)) == 0 {
			continue
		}
		var ordered []string
		valid := false
		for _, probe := range nc.probes {
			if len(nc.c.Validate(probe)) != 0 {
				continue
			}
			valid = true
			if !unorderedAcrossZone(t, nc.dated, probe, nc.bounds) {
				ordered = append(ordered, probe)
			}
		}
		switch {
		case len(ordered) > 0:
			t.Errorf("%s: example %#v refused, though %q is valid", nc.name, got, ordered[0])
		case valid:
			exceptions++
		}
	}
	if exceptions == 0 {
		t.Error("no case is the exception the spec names, want the sweep to reach it")
	}
}

// narrowCase is one constraint of the narrow-range sweep, with the values
// the search tries where Validate refuses its example. dated marks a
// date-time, and bounds holds the bounds of a time's or a date-time's
// range on each side that is not unbounded; a duration's are left out.
type narrowCase struct {
	name   string
	c      constraints.PrimitiveConstraint
	probes []string
	bounds []string
	dated  bool
}

// unorderedAcrossZone reports whether v, a time, or a date-time when dated,
// is not ordered against one of bounds that carries a zone where v carries
// none, or none where v carries one: whether the two lie within 14 hours of
// where one starts or ends, as iso8601.OrderAcrossZone reads them. A case
// with no clock bounds, a duration's, has none to leave v unordered, so v
// is not read as a time and the sweep names the example it refuses.
func unorderedAcrossZone(t *testing.T, dated bool, v string, bounds []string) bool {
	t.Helper()
	if len(bounds) == 0 {
		return false
	}
	span, zone := clockSpan(t, dated, v)
	for _, b := range bounds {
		bspan, bzone := clockSpan(t, dated, b)
		var ordered bool
		switch {
		case zone != "" && bzone == "":
			_, ordered = iso8601.OrderAcrossZone(span, zone, bspan)
		case zone == "" && bzone != "":
			_, ordered = iso8601.OrderAcrossZone(bspan, bzone, span)
		default:
			continue
		}
		if !ordered {
			return true
		}
	}
	return false
}

// clockSpan reads v, a time, or a date-time when dated, into the span of
// time it covers and its zone.
func clockSpan(t *testing.T, dated bool, v string) (iso8601.Span, string) {
	t.Helper()
	if dated {
		d, tm, err := iso8601.ParseDateTime(v)
		if err != nil {
			t.Fatalf("value %q: %v", v, err)
		}
		return iso8601.DateTimeSpan(d, tm, strings.Contains(v, "T")), tm.Zone
	}
	tm, err := iso8601.ParseTime(v)
	if err != nil {
		t.Fatalf("value %q: %v", v, err)
	}
	return iso8601.TimeSpan(tm), tm.Zone
}

// TestREQ103_NarrowSweepReadsNoDurationAsATime holds the narrow-range sweep
// on a duration, whose case has no clock bounds: unorderedAcrossZone reports
// its value ordered without reading it as a time, so a refused duration
// example is named instead of stopping the sweep on the time parse.
func TestREQ103_NarrowSweepReadsNoDurationAsATime(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"P1D", "-PT0.15S"} {
		if unorderedAcrossZone(t, false, v, nil) {
			t.Errorf("unorderedAcrossZone(%q, no bounds) = true, want false", v)
		}
	}
}

// narrowCases generates the constraints of the narrow-range sweep.
func narrowCases(t *testing.T) []narrowCase {
	t.Helper()
	validities := []constraints.ValidityKind{constraints.ValidityOptional, constraints.ValidityMandatory, constraints.ValidityDisallowed}
	var out []narrowCase
	for _, r := range narrowClockRanges("", narrowTimeLowers, narrowTimeUppers, narrowTimePairs) {
		probes := clockProbes(t, false, r)
		for _, p := range clockPatterns("") {
			for _, tz := range validities {
				out = append(out, narrowCase{
					name:   fmt.Sprintf("time %q %s tz %d", p, r, tz),
					c:      constraints.CTime{Pattern: p, Range: r, TimezoneValidity: tz},
					probes: probes,
					bounds: boundsOf(r),
				})
			}
		}
	}
	for _, r := range narrowClockRanges("2021-05-15T", narrowTimeLowers, nil, narrowTimePairs) {
		probes := clockProbes(t, true, r)
		for _, p := range clockPatterns("yyyy-mm-ddT") {
			for _, tz := range validities {
				out = append(out, narrowCase{
					name:   fmt.Sprintf("date-time %q %s tz %d", p, r, tz),
					c:      constraints.CDateTime{Pattern: p, Range: r, TimezoneValidity: tz},
					probes: probes,
					bounds: boundsOf(r),
					dated:  true,
				})
			}
		}
	}
	for _, r := range narrowDurationRanges() {
		probes := durationProbes(t, r)
		for _, p := range durationPatterns() {
			out = append(out, narrowCase{
				name:   fmt.Sprintf("duration %q %s", p, r),
				c:      constraints.CDuration{Pattern: p, Range: r},
				probes: probes,
			})
		}
	}
	return out
}

var (
	// narrowTimeLowers are lower bounds less than a second, or one unit of
	// their last field, before the end of the day.
	narrowTimeLowers = []string{"23:59:59", "23:59:59.5", "23:59", "23"}
	// narrowTimeUppers are upper bounds at the start of the day.
	narrowTimeUppers = []string{"00:00:00", "00:00:00.5", "00:00", "00"}
	// narrowTimePairs are bounds less than a second apart.
	narrowTimePairs = [][2]string{{"10:00:00", "10:00:01"}, {"10:00:00.25", "10:00:00.75"}, {"10:59:59", "11:00:00"}}
	// narrowZones are the zones a bound is written with: none, Z and an
	// offset.
	narrowZones = []string{"", "Z", "+02:00"}
)

// boundsOf returns the bounds of r on each side that is not unbounded.
func boundsOf(r constraints.TemporalRange) []string {
	var out []string
	if !r.LowerUnbounded {
		out = append(out, r.Lower)
	}
	if !r.UpperUnbounded {
		out = append(out, r.Upper)
	}
	return out
}

// narrowClockRanges returns the narrow ranges of a time, each clock written
// after datePrefix, the date of a date-time or "" for a time, and with each
// of narrowZones: from each lower and up to each upper, excluded and
// included, and between each pair, with either or both bounds excluded.
// Two pairs whose bounds sit in different zones are added.
func narrowClockRanges(datePrefix string, lowers, uppers []string, pairs [][2]string) []constraints.TemporalRange {
	var out []constraints.TemporalRange
	for _, z := range narrowZones {
		for _, in := range []bool{false, true} {
			for _, lo := range lowers {
				out = append(out, constraints.TemporalRange{Lower: datePrefix + lo + z, LowerInclusive: in, UpperUnbounded: true})
			}
			for _, hi := range uppers {
				out = append(out, constraints.TemporalRange{LowerUnbounded: true, Upper: datePrefix + hi + z, UpperInclusive: in})
			}
		}
		for _, pr := range pairs {
			for _, flags := range [][2]bool{{false, false}, {true, false}, {false, true}} {
				out = append(out, constraints.TemporalRange{
					Lower: datePrefix + pr[0] + z, Upper: datePrefix + pr[1] + z,
					LowerInclusive: flags[0], UpperInclusive: flags[1],
				})
			}
		}
	}
	return append(out,
		constraints.TemporalRange{Lower: datePrefix + "10:00:00+02:00", Upper: datePrefix + "08:00:01Z"},
		constraints.TemporalRange{Lower: datePrefix + "10:00:00.25Z", Upper: datePrefix + "12:00:00.75+02:00"},
	)
}

// clockPatterns returns every time pattern the ADL 1.4 grammar admits after
// datePrefix: the minute and the second each in letters, ?? or XX, in an
// order the grammar allows, with no zone suffix, a Z suffix or a ±hh:mm
// one, and the empty pattern.
func clockPatterns(datePrefix string) []string {
	out := []string{""}
	for _, tail := range []string{"mm:ss", "mm:??", "mm:XX", "??:??", "??:XX", "XX:XX"} {
		for _, zone := range []string{"", "Z", "±hh:mm"} {
			out = append(out, datePrefix+"hh:"+tail+zone)
		}
	}
	return out
}

// probeZones are the zones the search writes a value in, none first.
var probeZones = []string{"", "Z", "+01:00", "-01:00", "+02:00", "-02:00", "+03:00", "-03:00", "+14:00", "-12:00"}

// probeSteps are the seconds the search moves a bound's instant by.
var probeSteps = []float64{0, 0.25, 0.5, 0.75, 1, 60, 3600, -0.25, -0.5, -0.75, -1, -60, -3600}

// clockProbes returns the values the search tries for a time, or a
// date-time when dated, in the range r: each bound's instant moved by each
// of probeSteps, written in each of probeZones with the clock that zone
// gives it, a bound with no zone read as UTC, and with no zone on its own
// clock; each to the hour, the minute, the second and with its fraction,
// and a date-time also on the day before and after its date and as its
// date alone. A time whose clock leaves the day is left out.
func clockProbes(t *testing.T, dated bool, r constraints.TemporalRange) []string {
	t.Helper()
	var out []string
	for _, bound := range []string{r.Lower, r.Upper} {
		if bound == "" {
			continue
		}
		day, clock, zone := parseProbeBound(t, dated, bound)
		offset := 0
		if zone != "" {
			var err error
			if offset, err = iso8601.ParseZone(zone); err != nil {
				t.Fatalf("bound %q: %v", bound, err)
			}
		}
		for _, step := range probeSteps {
			for _, z := range probeZones {
				local := clock + step
				if z != "" {
					o, err := iso8601.ParseZone(z)
					if err != nil {
						t.Fatalf("zone %q: %v", z, err)
					}
					local += float64(o - offset)
				}
				shift := int(math.Floor(local / 86400))
				if !dated && shift != 0 {
					continue
				}
				local -= float64(shift) * 86400
				for _, c := range clockForms(local) {
					if !dated {
						out = append(out, c+z)
						continue
					}
					for _, d := range []int{-1, 0, 1} {
						date := probeDate(day + shift + d)
						out = append(out, date+"T"+c+z, date)
					}
				}
			}
		}
	}
	return out
}

// parseProbeBound reads bound, a time or, when dated, a date-time, into the
// magnitude of its date (0 for a time), the seconds of its clock and its
// zone.
func parseProbeBound(t *testing.T, dated bool, bound string) (day int, clock float64, zone string) {
	t.Helper()
	if !dated {
		tm, err := iso8601.ParseTime(bound)
		if err != nil {
			t.Fatalf("bound %q: %v", bound, err)
		}
		return 0, tm.Magnitude(), tm.Zone
	}
	d, tm, err := iso8601.ParseDateTime(bound)
	if err != nil {
		t.Fatalf("bound %q: %v", bound, err)
	}
	return d.Magnitude(), tm.Magnitude(), tm.Zone
}

// clockForms writes secs, the seconds of a clock, to the hour, the minute,
// the second and with its fraction, each cut short rather than rounded.
func clockForms(secs float64) []string {
	whole := int(math.Floor(secs))
	h, m, s := whole/3600, whole%3600/60, whole%60
	out := []string{
		fmt.Sprintf("%02d", h),
		fmt.Sprintf("%02d:%02d", h, m),
		fmt.Sprintf("%02d:%02d:%02d", h, m, s),
	}
	if frac := secs - float64(whole); frac > 0 {
		digits := strconv.FormatFloat(frac, 'f', 6, 64)
		out = append(out, fmt.Sprintf("%02d:%02d:%02d%s", h, m, s, strings.TrimRight(digits[1:], "0")))
	}
	return out
}

// probeDate writes the date whose magnitude is days.
func probeDate(days int) string {
	d := iso8601.DateOfMagnitude(days)
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// narrowDurationRanges are duration ranges between two bounds less than a
// second apart, or a second apart, with either or both bounds excluded,
// and a range from or up to half a second.
func narrowDurationRanges() []constraints.TemporalRange {
	var out []constraints.TemporalRange
	for _, pr := range [][2]string{{"PT0S", "PT0.5S"}, {"PT0.25S", "PT0.75S"}, {"PT1S", "PT2S"}, {"P1D", "P1DT1S"}, {"-PT1S", "PT0S"}} {
		for _, flags := range [][2]bool{{false, false}, {true, false}, {false, true}} {
			out = append(out, constraints.TemporalRange{Lower: pr[0], Upper: pr[1], LowerInclusive: flags[0], UpperInclusive: flags[1]})
		}
	}
	for _, in := range []bool{false, true} {
		out = append(out,
			constraints.TemporalRange{Lower: "PT0.5S", LowerInclusive: in, UpperUnbounded: true},
			constraints.TemporalRange{LowerUnbounded: true, Upper: "PT0.5S", UpperInclusive: in})
	}
	return out
}

// durationPatterns returns every duration pattern the ADL 1.4 grammar
// admits: each set of designators that is not empty, in written order, and
// the empty pattern.
func durationPatterns() []string {
	out := []string{""}
	for set := 1; set < 1<<7; set++ {
		var b strings.Builder
		b.WriteByte('P')
		for i, c := range "YMWD" {
			if set&(1<<i) != 0 {
				b.WriteRune(c)
			}
		}
		if set>>4 != 0 {
			b.WriteByte('T')
			for i, c := range "HMS" {
				if set&(1<<(4+i)) != 0 {
					b.WriteRune(c)
				}
			}
		}
		out = append(out, b.String())
	}
	return out
}

// durationProbes returns the durations the search tries in the range r:
// each bound's length, the midpoint of two bounds and those moved by a
// quarter, a half and a whole second either way, each in seconds with its
// fraction and in each designator it is a whole count of.
func durationProbes(t *testing.T, r constraints.TemporalRange) []string {
	t.Helper()
	var lengths []float64
	for _, bound := range []string{r.Lower, r.Upper} {
		if bound == "" {
			continue
		}
		d, err := iso8601.ParseDuration(bound)
		if err != nil {
			t.Fatalf("bound %q: %v", bound, err)
		}
		lengths = append(lengths, d.ToSeconds())
	}
	if len(lengths) == 2 {
		lengths = append(lengths, (lengths[0]+lengths[1])/2)
	}
	units := []struct {
		secs float64
		unit string
	}{{365.24 * 86400, "Y"}, {30.42 * 86400, "M"}, {7 * 86400, "W"}, {86400, "D"}, {3600, "TH"}, {60, "TM"}, {1, "TS"}}
	out := []string{"P0D", "PT0S"}
	for _, length := range lengths {
		for _, step := range []float64{0, 0.25, 0.5, 1, -0.25, -0.5, -1} {
			secs := length + step
			sign := ""
			if secs < 0 {
				sign, secs = "-", -secs
			}
			out = append(out, sign+"PT"+strconv.FormatFloat(secs, 'f', -1, 64)+"S")
			for _, u := range units {
				if n := secs / u.secs; n == math.Trunc(n) {
					out = append(out, fmt.Sprintf("%sP%s%d%s", sign, strings.TrimSuffix(u.unit, u.unit[len(u.unit)-1:]), int64(n), u.unit[len(u.unit)-1:]))
				}
			}
		}
	}
	return out
}
