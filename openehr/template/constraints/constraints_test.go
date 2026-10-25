package constraints_test

import (
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// REQ-103 — NumericRange Contains honours inclusive / exclusive
// bounds and unbounded sides. Spot-checks the four corner cases.
func TestNumericRange_Contains(t *testing.T) {
	tests := []struct {
		name string
		r    constraints.NumericRange
		v    float64
		want bool
	}{
		{"closed-inside", constraints.NumericRange{Lower: 0, Upper: 10, LowerInclusive: true, UpperInclusive: true}, 5, true},
		{"closed-on-lower", constraints.NumericRange{Lower: 0, Upper: 10, LowerInclusive: true, UpperInclusive: true}, 0, true},
		{"closed-on-upper", constraints.NumericRange{Lower: 0, Upper: 10, LowerInclusive: true, UpperInclusive: true}, 10, true},
		{"open-on-lower", constraints.NumericRange{Lower: 0, Upper: 10}, 0, false},
		{"open-on-upper", constraints.NumericRange{Lower: 0, Upper: 10}, 10, false},
		{"below-lower", constraints.NumericRange{Lower: 0, Upper: 10, LowerInclusive: true, UpperInclusive: true}, -1, false},
		{"above-upper", constraints.NumericRange{Lower: 0, Upper: 10, LowerInclusive: true, UpperInclusive: true}, 11, false},
		{"lower-unbounded", constraints.NumericRange{Upper: 10, UpperInclusive: true, LowerUnbounded: true}, -1e6, true},
		{"upper-unbounded", constraints.NumericRange{Lower: 0, LowerInclusive: true, UpperUnbounded: true}, 1e6, true},
		{"fully-unbounded", constraints.NumericRange{LowerUnbounded: true, UpperUnbounded: true}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.Contains(tt.v); got != tt.want {
				t.Errorf("Contains(%v) = %v, want %v (range %s)", tt.v, got, tt.want, tt.r)
			}
		})
	}
}

// REQ-103 — a bounded NumericRange contains no NaN. Every comparison
// with a NaN is false, so a range that only refuses values below its
// lower or above its upper bound would let one through; a bounded
// C_REAL or DV_QUANTITY magnitude range must report it as
// CodeOutOfRange. A range that constrains nothing accepts a NaN
// (TestREQ103_NumericRangeWithNoBoundsAcceptsNaN).
func TestREQ103_NumericRangeRefusesNaN(t *testing.T) {
	nan := math.NaN()
	ranges := []struct {
		name string
		r    constraints.NumericRange
	}{
		{"closed", constraints.NumericRange{Lower: 0, Upper: 10, LowerInclusive: true, UpperInclusive: true}},
		{"half-open", constraints.NumericRange{Lower: 0, Upper: 10, LowerInclusive: true}},
		{"exclusive", constraints.NumericRange{Lower: 0, Upper: 10}},
		{"lower-bounded-only", constraints.NumericRange{Lower: 0, LowerInclusive: true, UpperUnbounded: true}},
		{"upper-bounded-only", constraints.NumericRange{Upper: 10, UpperInclusive: true, LowerUnbounded: true}},
	}
	for _, tc := range ranges {
		t.Run("Contains/"+tc.name, func(t *testing.T) {
			if tc.r.Contains(nan) {
				t.Errorf("Contains(NaN) = true, want false (range %s)", tc.r)
			}
		})
	}

	closed := constraints.NumericRange{Lower: 0, Upper: 300, LowerInclusive: true, UpperInclusive: true}
	validators := []struct {
		name  string
		c     constraints.PrimitiveConstraint
		value any
	}{
		{"CReal", constraints.CReal{Range: closed}, nan},
		{
			"DvQuantity",
			constraints.DvQuantity{Units: []constraints.QuantityUnit{{Units: "mm[Hg]", Magnitude: closed}}},
			constraints.QuantityValue{Magnitude: nan, Units: "mm[Hg]", Precision: -1},
		},
	}
	for _, tc := range validators {
		t.Run("Validate/"+tc.name, func(t *testing.T) {
			v := tc.c.Validate(tc.value)
			if len(v) != 1 || v[0].Code != constraints.CodeOutOfRange {
				t.Errorf("%s.Validate(NaN) against %s = %v, want one CodeOutOfRange", tc.name, closed, v)
			}
		})
	}
}

// REQ-103 — the zero NumericRange means "no constraint": IsBounded
// reports false for it, and Contains accepts every value, a NaN
// included, as on a range with both sides unbounded. Read field by field, its zero
// bounds would form the empty interval (0..0) and refuse every value.
func TestREQ103_NumericRangeZeroValueAcceptsAnyValue(t *testing.T) {
	zero := constraints.NumericRange{}
	if zero.IsBounded() {
		t.Error("NumericRange{}.IsBounded() = true, want false")
	}
	for _, v := range []float64{math.Inf(-1), -1e6, -1, 0, 1, 1e6, math.Inf(1)} {
		if !zero.Contains(v) {
			t.Errorf("NumericRange{}.Contains(%v) = false, want true", v)
		}
	}
	if !zero.Contains(math.NaN()) {
		t.Error("NumericRange{}.Contains(NaN) = false, want true")
	}
}

// REQ-103 — a range that constrains nothing (the zero value, or both
// sides unbounded) accepts a NaN, and so do the validators, which skip
// such a range: Contains and Validate agree. Only a bounded range
// refuses a NaN (TestREQ103_NumericRangeRefusesNaN).
func TestREQ103_NumericRangeWithNoBoundsAcceptsNaN(t *testing.T) {
	nan := math.NaN()
	for _, tc := range []struct {
		name string
		r    constraints.NumericRange
	}{
		{"zero-value", constraints.NumericRange{}},
		{"fully-unbounded", constraints.NumericRange{LowerUnbounded: true, UpperUnbounded: true}},
		{"fully-unbounded-inclusive", constraints.NumericRange{LowerUnbounded: true, UpperUnbounded: true, LowerInclusive: true, UpperInclusive: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.r.Contains(nan) {
				t.Errorf("Contains(NaN) = false, want true (range %s constrains nothing)", tc.r)
			}
			if v := (constraints.CReal{Range: tc.r}).Validate(nan); len(v) != 0 {
				t.Errorf("CReal{Range: %s}.Validate(NaN) = %v, want no violation", tc.r, v)
			}
			q := constraints.DvQuantity{Units: []constraints.QuantityUnit{{Units: "mm[Hg]", Magnitude: tc.r}}}
			if v := q.Validate(constraints.QuantityValue{Magnitude: nan, Units: "mm[Hg]", Precision: -1}); len(v) != 0 {
				t.Errorf("DvQuantity magnitude %s Validate(NaN) = %v, want no violation", tc.r, v)
			}
		})
	}
}

// REQ-103 — IsValid and String read the zero NumericRange as if both
// sides were unbounded, as IsBounded and Contains do: it is valid, and
// it prints as (*..*). Read field by field, its zero bounds would form
// the empty interval (0..0). A range with both sides explicitly
// unbounded keeps the brackets its inclusivity flags give it. One flag
// away from the zero value, the zero bounds are real bounds again.
func TestREQ103_NumericRangeZeroValueReadsAsUnbounded(t *testing.T) {
	tests := []struct {
		name      string
		r         constraints.NumericRange
		wantValid bool
		wantText  string
	}{
		{name: "zero-value", r: constraints.NumericRange{}, wantValid: true, wantText: "(*..*)"},
		{
			name:      "unbounded-exclusive",
			r:         constraints.NumericRange{LowerUnbounded: true, UpperUnbounded: true},
			wantValid: true, wantText: "(*..*)",
		},
		{
			name:      "unbounded-inclusive",
			r:         constraints.NumericRange{LowerUnbounded: true, UpperUnbounded: true, LowerInclusive: true, UpperInclusive: true},
			wantValid: true, wantText: "[*..*]",
		},
		{
			name:      "unbounded-lower-inclusive",
			r:         constraints.NumericRange{LowerUnbounded: true, UpperUnbounded: true, LowerInclusive: true},
			wantValid: true, wantText: "[*..*)",
		},
		{
			name:      "unbounded-upper-inclusive",
			r:         constraints.NumericRange{LowerUnbounded: true, UpperUnbounded: true, UpperInclusive: true},
			wantValid: true, wantText: "(*..*]",
		},
		{
			name:      "zero-bounds-upper-inclusive",
			r:         constraints.NumericRange{UpperInclusive: true},
			wantValid: false, wantText: "(0..0]",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.r.IsValid(); got != tc.wantValid {
				t.Errorf("%#v.IsValid() = %v, want %v", tc.r, got, tc.wantValid)
			}
			if got := tc.r.String(); got != tc.wantText {
				t.Errorf("%#v.String() = %q, want %q", tc.r, got, tc.wantText)
			}
		})
	}
}

func TestNumericRange_IsValid(t *testing.T) {
	tests := []struct {
		name string
		r    constraints.NumericRange
		want bool
	}{
		{"normal", constraints.NumericRange{Lower: 0, Upper: 10}, true},
		{"point-closed", constraints.NumericRange{Lower: 5, Upper: 5, LowerInclusive: true, UpperInclusive: true}, true},
		{"point-open", constraints.NumericRange{Lower: 5, Upper: 5}, false},
		{"inverted", constraints.NumericRange{Lower: 10, Upper: 0}, false},
		{"unbounded", constraints.NumericRange{LowerUnbounded: true, UpperUnbounded: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

// REQ-103 — CBoolean.Validate honours true_valid / false_valid; wrong
// type surfaces as CodeWrongType.
func TestCBoolean_Validate(t *testing.T) {
	c := constraints.CBoolean{TrueValid: true, FalseValid: false}
	if v := c.Validate(true); len(v) != 0 {
		t.Errorf("Validate(true) = %v, want nil", v)
	}
	v := c.Validate(false)
	if len(v) != 1 || v[0].Code != constraints.CodeNotInList {
		t.Errorf("Validate(false) = %v, want one CodeNotInList", v)
	}
	v = c.Validate("not a bool")
	if len(v) != 1 || v[0].Code != constraints.CodeWrongType {
		t.Errorf("Validate(string) = %v, want one CodeWrongType", v)
	}
}

// REQ-103 — CInteger.Validate enforces range + list intersection and
// rejects floats via CodeWrongType (CReal owns those).
func TestCInteger_Validate(t *testing.T) {
	c := constraints.CInteger{
		Range: constraints.NumericRange{Lower: 0, Upper: 10, LowerInclusive: true, UpperInclusive: true},
		List:  []int64{2, 4, 6},
	}
	if v := c.Validate(4); len(v) != 0 {
		t.Errorf("Validate(4) = %v, want nil", v)
	}
	if v := c.Validate(int32(2)); len(v) != 0 {
		t.Errorf("Validate(int32(2)) = %v, want nil (widening)", v)
	}
	if v := c.Validate(uint64(4)); len(v) != 0 {
		t.Errorf("Validate(uint64(4)) = %v, want nil (in-range uint64 widens to int64)", v)
	}
	// uint64 above MaxInt64 surfaces as CodeWrongType, NOT silent wrap.
	if v := c.Validate(uint64(1 << 63)); len(v) != 1 || v[0].Code != constraints.CodeWrongType {
		t.Errorf("Validate(uint64 > MaxInt64) = %v, want CodeWrongType", v)
	}
	// Not in list — should report.
	v := c.Validate(5)
	if len(v) != 1 || v[0].Code != constraints.CodeNotInList {
		t.Errorf("Validate(5) violation = %v, want one CodeNotInList", v)
	}
	// Out of range and not in list — both violations.
	v = c.Validate(20)
	if len(v) != 2 {
		t.Errorf("Validate(20) returned %d violations, want 2 (out-of-range + not-in-list)", len(v))
	}
	// Float rejected.
	v = c.Validate(1.5)
	if len(v) != 1 || v[0].Code != constraints.CodeWrongType {
		t.Errorf("Validate(float) = %v, want CodeWrongType", v)
	}
}

// REQ-103 — CReal accepts ints (widening) and floats; list match
// uses exact equality.
func TestCReal_Validate(t *testing.T) {
	c := constraints.CReal{
		Range: constraints.NumericRange{Lower: 0, Upper: 100, LowerInclusive: true, UpperInclusive: true},
	}
	if v := c.Validate(50.5); len(v) != 0 {
		t.Errorf("Validate(50.5) = %v, want nil", v)
	}
	if v := c.Validate(10); len(v) != 0 {
		t.Errorf("Validate(int 10) = %v, want nil", v)
	}
	if v := c.Validate("abc"); len(v) != 1 || v[0].Code != constraints.CodeWrongType {
		t.Errorf("Validate(string) = %v, want CodeWrongType", v)
	}
}

// REQ-103 — CString.Validate enforces pattern + list. An unparseable
// constraint pattern surfaces as CodeInvalidValue (constraint defect,
// not user value).
func TestCString_Validate(t *testing.T) {
	c := constraints.CString{Pattern: `^[A-Z][a-z]+$`}
	if v := c.Validate("Alpha"); len(v) != 0 {
		t.Errorf("Validate(Alpha) = %v, want nil", v)
	}
	v := c.Validate("alpha")
	if len(v) != 1 || v[0].Code != constraints.CodePatternMismatch {
		t.Errorf("Validate(alpha) = %v, want CodePatternMismatch", v)
	}
	// Closed list.
	c = constraints.CString{List: []string{"yes", "no", "maybe"}}
	if v := c.Validate("yes"); len(v) != 0 {
		t.Errorf("Validate(yes) = %v, want nil", v)
	}
	v = c.Validate("nope")
	if len(v) != 1 || v[0].Code != constraints.CodeNotInList {
		t.Errorf("Validate(nope) = %v, want CodeNotInList", v)
	}
	// Bad pattern surfaces as constraint defect.
	c = constraints.CString{Pattern: `(`}
	v = c.Validate("anything")
	if len(v) != 1 || v[0].Code != constraints.CodeInvalidValue {
		t.Errorf("Validate(bad pattern) = %v, want CodeInvalidValue", v)
	}
}

// REQ-103 — an open CString list (AOM 1.4 C_STRING.list_open) is not
// exhaustive, so Validate reports no value outside it, and it still checks
// the pattern. A closed list still reports a value outside it.
func TestCString_Validate_REQ103_ListOpen(t *testing.T) {
	t.Parallel()
	openWithPattern := constraints.NewCString(`[A-Z][a-z]+`, []string{"Husten"}, "")
	openWithPattern.ListOpen = true
	tests := []struct {
		name  string
		c     constraints.CString
		value string
		want  []constraints.ViolationCode
	}{
		{name: "open list, value outside", c: constraints.CString{List: []string{"Husten"}, ListOpen: true}, value: "Schnupfen"},
		{name: "open list, value inside", c: constraints.CString{List: []string{"Husten"}, ListOpen: true}, value: "Husten"},
		{name: "closed list, value outside", c: constraints.CString{List: []string{"Husten"}}, value: "Schnupfen", want: []constraints.ViolationCode{constraints.CodeNotInList}},
		{name: "open list, value outside matches pattern", c: openWithPattern, value: "Schnupfen"},
		{name: "open list, value outside fails pattern", c: openWithPattern, value: "schnupfen", want: []constraints.ViolationCode{constraints.CodePatternMismatch}},
		{name: "open list, pattern compiled on first use", c: constraints.CString{Pattern: `[A-Z][a-z]+`, List: []string{"Husten"}, ListOpen: true}, value: "schnupfen", want: []constraints.ViolationCode{constraints.CodePatternMismatch}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got []constraints.ViolationCode
			for _, v := range tc.c.Validate(tc.value) {
				got = append(got, v.Code)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("CString{Pattern: %q, List: %v, ListOpen: %v}.Validate(%q) codes = %v, want %v",
					tc.c.Pattern, tc.c.List, tc.c.ListOpen, tc.value, got, tc.want)
			}
		})
	}
}

// REQ-103 — temporal validators accept exactly the ISO 8601 forms the
// RM's temporal parse accepts (rm.DVDate.ValidISO8601 and its DV_TIME,
// DV_DATE_TIME and DV_DURATION siblings), partial dates included, and
// refuse text that is not ISO 8601.
func TestTemporal_Validate(t *testing.T) {
	if v := (constraints.CDate{}).Validate("2026-05-24"); len(v) != 0 {
		t.Errorf("CDate(2026-05-24) = %v, want nil", v)
	}
	if v := (constraints.CDate{}).Validate("2026-05"); len(v) != 0 {
		t.Errorf("CDate(2026-05) = %v, want nil (partial)", v)
	}
	if v := (constraints.CDate{}).Validate("not-a-date"); len(v) == 0 {
		t.Errorf("CDate(not-a-date) returned no violations, want CodeInvalidValue")
	}
	if v := (constraints.CTime{}).Validate("14:30:00"); len(v) != 0 {
		t.Errorf("CTime(14:30:00) = %v, want nil", v)
	}
	if v := (constraints.CDateTime{}).Validate("2026-05-24T14:30:00Z"); len(v) != 0 {
		t.Errorf("CDateTime(RFC3339) = %v, want nil", v)
	}
	if v := (constraints.CDuration{}).Validate("P1Y2M3DT4H5M6S"); len(v) != 0 {
		t.Errorf("CDuration(full) = %v, want nil", v)
	}
	if v := (constraints.CDuration{}).Validate("PT5M"); len(v) != 0 {
		t.Errorf("CDuration(PT5M) = %v, want nil", v)
	}
	if v := (constraints.CDuration{}).Validate("not-iso"); len(v) == 0 {
		t.Errorf("CDuration(not-iso) returned no violations, want CodeInvalidValue")
	}
}

// REQ-103 — CodePhrase.Validate accepts bare string (treated as code
// under constrained terminology) and full CodedTermRef. Mismatched
// terminology AND missing code both surface.
func TestCodePhrase_Validate(t *testing.T) {
	c := constraints.CodePhrase{Terminology: "openehr", CodeList: []string{"433", "434"}}
	if v := c.Validate("433"); len(v) != 0 {
		t.Errorf("Validate(string code) = %v, want nil", v)
	}
	if v := c.Validate(constraints.CodedTermRef{Terminology: "openehr", CodeString: "433"}); len(v) != 0 {
		t.Errorf("Validate(typed) = %v, want nil", v)
	}
	v := c.Validate(constraints.CodedTermRef{Terminology: "SNOMED-CT", CodeString: "433"})
	if len(v) != 1 || v[0].Code != constraints.CodeInvalidValue {
		t.Errorf("Validate(mismatch terminology) = %v, want CodeInvalidValue", v)
	}
	v = c.Validate("999")
	if len(v) != 1 || v[0].Code != constraints.CodeNotInList {
		t.Errorf("Validate(unknown code) = %v, want CodeNotInList", v)
	}
	// External (open) — empty code list accepts anything.
	open := constraints.CodePhrase{Terminology: "SNOMED-CT"}
	if !open.External() {
		t.Errorf("External() = false, want true for empty CodeList")
	}
	if v := open.Validate("any code"); len(v) != 0 {
		t.Errorf("Validate(any) on external = %v, want nil", v)
	}
}

// REQ-103 — DvQuantity.Validate enforces (units, magnitude) match.
// Mismatched units short-circuits to CodeUnitUnknown.
func TestDvQuantity_Validate(t *testing.T) {
	c := constraints.DvQuantity{Units: []constraints.QuantityUnit{
		{
			Units:     "mm[Hg]",
			Magnitude: constraints.NumericRange{Lower: 0, Upper: 300, LowerInclusive: true, UpperInclusive: true},
		},
		{
			Units:     "kPa",
			Magnitude: constraints.NumericRange{Lower: 0, Upper: 40, LowerInclusive: true, UpperInclusive: true},
		},
	}}
	if v := c.Validate(constraints.QuantityValue{Magnitude: 120, Units: "mm[Hg]"}); len(v) != 0 {
		t.Errorf("Validate(120 mm[Hg]) = %v, want nil", v)
	}
	v := c.Validate(constraints.QuantityValue{Magnitude: 400, Units: "mm[Hg]"})
	if len(v) != 1 || v[0].Code != constraints.CodeOutOfRange {
		t.Errorf("Validate(400 mm[Hg]) = %v, want CodeOutOfRange", v)
	}
	v = c.Validate(constraints.QuantityValue{Magnitude: 100, Units: "psi"})
	if len(v) != 1 || v[0].Code != constraints.CodeUnitUnknown {
		t.Errorf("Validate(psi) = %v, want CodeUnitUnknown", v)
	}
	v = c.Validate("not a quantity")
	if len(v) != 1 || v[0].Code != constraints.CodeWrongType {
		t.Errorf("Validate(wrong type) = %v, want CodeWrongType", v)
	}
}

// REQ-103 — with no Units list, DvQuantity.Validate refuses units outside
// the property's, as the specifications-CNF conformance schedule's case
// CONT-DV_QUANTITY-validate_property does under openehr::122 (length). A
// Units list takes precedence, so the property is not consulted: the case
// CONT-DV_QUANTITY-validate_property_units refuses km, a length unit, against
// the list [cm, m], and a listed unit outside the property is accepted. With
// neither, any units are accepted. The NULL rows of both cases are the RM's.
func TestREQ103_DvQuantityPropertyUnits(t *testing.T) {
	length := &constraints.CodedTermRef{Terminology: "openehr", CodeString: "122"}
	lengthUnits := []string{"m", "[in_i]/1000", "cm", "km", "mm", "um"}
	list := func(units ...string) []constraints.QuantityUnit {
		out := make([]constraints.QuantityUnit, len(units))
		for i, u := range units {
			out[i] = constraints.QuantityUnit{Units: u}
		}
		return out
	}
	tests := []struct {
		name      string
		c         constraints.DvQuantity
		magnitude float64
		units     string
		want      bool // accepted
	}{
		// CONT-DV_QUANTITY-validate_property.
		{name: "property, 0.0 mg", c: constraints.DvQuantity{Property: length, PropertyUnits: lengthUnits}, magnitude: 0, units: "mg"},
		{name: "property, 0.0 cm", c: constraints.DvQuantity{Property: length, PropertyUnits: lengthUnits}, magnitude: 0, units: "cm", want: true},
		{name: "property, 1.0 cm", c: constraints.DvQuantity{Property: length, PropertyUnits: lengthUnits}, magnitude: 1, units: "cm", want: true},
		{name: "property, 5.7 cm", c: constraints.DvQuantity{Property: length, PropertyUnits: lengthUnits}, magnitude: 5.7, units: "cm", want: true},
		{name: "property, 10.0 cm", c: constraints.DvQuantity{Property: length, PropertyUnits: lengthUnits}, magnitude: 10, units: "cm", want: true},
		// UCUM codes are case-sensitive.
		{name: "property, 1.0 CM", c: constraints.DvQuantity{Property: length, PropertyUnits: lengthUnits}, magnitude: 1, units: "CM"},
		// CONT-DV_QUANTITY-validate_property_units.
		{name: "property and list, 0.0 mg", c: constraints.DvQuantity{Property: length, PropertyUnits: lengthUnits, Units: list("cm", "m")}, magnitude: 0, units: "mg"},
		{name: "property and list, 0.0 cm", c: constraints.DvQuantity{Property: length, PropertyUnits: lengthUnits, Units: list("cm", "m")}, magnitude: 0, units: "cm", want: true},
		{name: "property and list, 0.0 km", c: constraints.DvQuantity{Property: length, PropertyUnits: lengthUnits, Units: list("cm", "m")}, magnitude: 0, units: "km"},
		{name: "property and list, 1.0 cm", c: constraints.DvQuantity{Property: length, PropertyUnits: lengthUnits, Units: list("cm", "m")}, magnitude: 1, units: "cm", want: true},
		{name: "property and list, 5.7 cm", c: constraints.DvQuantity{Property: length, PropertyUnits: lengthUnits, Units: list("cm", "m")}, magnitude: 5.7, units: "cm", want: true},
		{name: "property and list, 10.0 cm", c: constraints.DvQuantity{Property: length, PropertyUnits: lengthUnits, Units: list("cm", "m")}, magnitude: 10, units: "cm", want: true},
		// The list keeps precedence: a listed unit the property does not
		// admit is accepted.
		{name: "property and a list naming mg, 1.0 mg", c: constraints.DvQuantity{Property: length, PropertyUnits: lengthUnits, Units: list("mg")}, magnitude: 1, units: "mg", want: true},
		// No list and no property units: any units.
		{name: "property the table does not list, 1.0 mg", c: constraints.DvQuantity{Property: &constraints.CodedTermRef{Terminology: "local", CodeString: "at0001"}}, magnitude: 1, units: "mg", want: true},
		{name: "no constraint, 1.0 furlong", c: constraints.DvQuantity{}, magnitude: 1, units: "furlong", want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.c.Validate(constraints.QuantityValue{Magnitude: tc.magnitude, Units: tc.units, Precision: -1})
			if tc.want {
				if len(got) != 0 {
					t.Errorf("Validate(%v %s) = %v, want accepted", tc.magnitude, tc.units, got)
				}
				return
			}
			if len(got) != 1 || got[0].Code != constraints.CodeUnitUnknown {
				t.Fatalf("Validate(%v %s) = %v, want one CodeUnitUnknown", tc.magnitude, tc.units, got)
			}
			// REQ-168: the units travel in Value alone, redacted.
			if r := got[0].Value.Reveal(); r != tc.units {
				t.Errorf("Validate(%v %s) Value.Reveal() = %v, want %q", tc.magnitude, tc.units, r, tc.units)
			}
			if strings.Contains(got[0].Detail, tc.units) {
				t.Errorf("Validate(%v %s) Detail = %q repeats the units under validation", tc.magnitude, tc.units, got[0].Detail)
			}
		})
	}
}

// REQ-103 — CDvOrdinal.Validate accepts int (ordinal value) and the
// full OrdinalSymbol pair; both miss → CodeNotInList.
func TestCDvOrdinal_Validate(t *testing.T) {
	c := constraints.CDvOrdinal{Values: []constraints.OrdinalSymbol{
		{Value: 0, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0001"}},
		{Value: 1, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0002"}},
	}}
	if v := c.Validate(0); len(v) != 0 {
		t.Errorf("Validate(0) = %v, want nil", v)
	}
	v := c.Validate(99)
	if len(v) != 1 || v[0].Code != constraints.CodeNotInList {
		t.Errorf("Validate(99) = %v, want CodeNotInList", v)
	}
	// Pair-match — wrong symbol with right value still fails.
	v = c.Validate(constraints.OrdinalSymbol{
		Value:  0,
		Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "wrong"},
	})
	if len(v) != 1 || v[0].Code != constraints.CodeNotInList {
		t.Errorf("Validate(value+wrong symbol) = %v, want CodeNotInList", v)
	}

	// REQ-103 — a DV_ORDINAL with no pairs is unconstrained. An empty
	// list must not reject every integer.
	open := constraints.CDvOrdinal{}
	if v := open.Validate(0); len(v) != 0 {
		t.Errorf("Validate(open 0) = %v, want nil", v)
	}
	if v := open.Validate(constraints.OrdinalSymbol{
		Value:  3,
		Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0003"},
	}); len(v) != 0 {
		t.Errorf("Validate(open OrdinalSymbol) = %v, want nil", v)
	}
	if v := open.Validate("nope"); len(v) != 1 || v[0].Code != constraints.CodeWrongType {
		t.Errorf("Validate(open wrong type) = %v, want CodeWrongType", v)
	}
}

// REQ-103 — an OrdinalSymbol matches a listed pair when the values and the
// code strings are equal and the terminologies are equal or either is empty,
// as CodePhrase.Validate compares a terminology. A listed symbol whose OPT
// names no terminology admits the one the instance carries, and a listed
// terminology admits an instance symbol that names none.
func TestCDvOrdinal_REQ103_PairTerminologyEqualOrEmpty(t *testing.T) {
	t.Parallel()
	c := constraints.CDvOrdinal{Values: []constraints.OrdinalSymbol{
		{Value: 0, Symbol: constraints.CodedTermRef{CodeString: "at0001"}},
		{Value: 1, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0002"}},
	}}
	pair := func(value int, terminology, code string) constraints.OrdinalSymbol {
		return constraints.OrdinalSymbol{Value: value, Symbol: constraints.CodedTermRef{Terminology: terminology, CodeString: code}}
	}
	tests := []struct {
		name  string
		input constraints.OrdinalSymbol
		match bool
	}{
		{name: "unnamed listed terminology, instance in local", input: pair(0, "local", "at0001"), match: true},
		{name: "unnamed listed terminology, instance in another terminology", input: pair(0, "SNOMED-CT", "at0001"), match: true},
		{name: "unnamed listed terminology, instance names none", input: pair(0, "", "at0001"), match: true},
		{name: "listed local, instance in local", input: pair(1, "local", "at0002"), match: true},
		{name: "listed local, instance names no terminology", input: pair(1, "", "at0002"), match: true},
		{name: "unnamed listed terminology, different code", input: pair(0, "local", "at0002"), match: false},
		{name: "listed local, different code", input: pair(1, "local", "at0001"), match: false},
		{name: "listed local, different non-empty terminology", input: pair(1, "SNOMED-CT", "at0002"), match: false},
		{name: "listed code under another value", input: pair(1, "", "at0001"), match: false},
		{name: "unlisted value", input: pair(2, "local", "at0002"), match: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := c.Validate(tc.input)
			if tc.match {
				if len(got) != 0 {
					t.Errorf("Validate(%d %s) = %+v, want nil", tc.input.Value, tc.input.Symbol, got)
				}
				return
			}
			if len(got) != 1 || got[0].Code != constraints.CodeNotInList {
				t.Errorf("Validate(%d %s) = %+v, want one CodeNotInList", tc.input.Value, tc.input.Symbol, got)
			}
		})
	}
}

// REQ-103, REQ-168 — the pair check's Detail names the allowed pairs from
// the template, value|terminology::code in list order, as the int check
// names the allowed values. The submitted pair goes only in Value.
func TestCDvOrdinal_REQ103_PairDetailNamesAllowedPairs(t *testing.T) {
	t.Parallel()
	c := constraints.CDvOrdinal{Values: []constraints.OrdinalSymbol{
		{Value: 0, Symbol: constraints.CodedTermRef{CodeString: "at0001"}},
		{Value: 1, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0002"}},
		{Value: 2, Symbol: constraints.CodedTermRef{Terminology: "SNOMED-CT", CodeString: "260349002"}},
	}}
	submitted := constraints.OrdinalSymbol{
		Value:  1,
		Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "SUBMITTED-CODE"},
	}
	got := c.Validate(submitted)
	if len(got) != 1 || got[0].Code != constraints.CodeNotInList {
		t.Fatalf("Validate(%+v) = %+v, want one CodeNotInList", submitted, got)
	}
	const want = "ordinal (value, symbol) pair not in allowed [0|::at0001 1|local::at0002 2|SNOMED-CT::260349002]"
	if got[0].Detail != want {
		t.Errorf("Validate(%+v).Detail = %q, want %q", submitted, got[0].Detail, want)
	}
	if strings.Contains(got[0].Detail, submitted.Symbol.CodeString) {
		t.Errorf("Validate(%+v).Detail = %q, carries the submitted code %q", submitted, got[0].Detail, submitted.Symbol.CodeString)
	}
	if rv, ok := got[0].Value.Reveal().(constraints.OrdinalSymbol); !ok || rv != submitted {
		t.Errorf("Validate(%+v).Value.Reveal() = %#v, want the submitted pair", submitted, got[0].Value.Reveal())
	}
}

// REQ-103 — PrimitiveConstraint is a closed interface; every
// concrete type the package exports implements it. Compile-time
// assertion documents that the type set stays in sync with the
// interface seal.
func TestPrimitiveConstraint_InterfaceSeal(_ *testing.T) {
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
