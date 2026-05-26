package constraints_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// REQ-107 — every bounded PrimitiveConstraint MUST round-trip
// Validate(ExampleValue()) to zero violations. The table covers all
// 11 implementations with at least one bounded subcase apiece, plus
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

		// CString — closed list, unbounded.
		{"CString/list", constraints.CString{List: []string{"yes", "no"}}},
		{"CString/unbounded", constraints.CString{}},

		// Temporal — every sentinel must parse against its validator.
		{"CDate", constraints.CDate{}},
		{"CTime", constraints.CTime{}},
		{"CDateTime", constraints.CDateTime{}},
		{"CDuration", constraints.CDuration{}},

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

		// CDvOrdinal — closed value list.
		{"CDvOrdinal/values", constraints.CDvOrdinal{Values: []constraints.OrdinalSymbol{
			{Value: 0, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0001"}},
			{Value: 1, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0002"}},
		}}},
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

// REQ-107 — the sealing contract still holds: only the 11 known
// types in this package implement PrimitiveConstraint. A new
// external implementer would break the closed type-switch the
// validator relies on (REQ-024 — no reflection).
func TestPrimitiveConstraint_ExampleValueSeal(_ *testing.T) {
	// Each entry asserts the type satisfies the full interface
	// (Validate + ExampleValue + isPrimitive). Adding a 12th type
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
}
