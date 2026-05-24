package constraints_test

import (
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

// REQ-103 — temporal validators accept ISO 8601 shapes (full,
// year-month, year for dates; RFC 3339 for date-times).
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
}
