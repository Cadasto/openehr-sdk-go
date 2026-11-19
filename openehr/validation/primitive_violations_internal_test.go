package validation

// primitive_violations_internal_test.go: the walk checks a code, a quantity
// and an ordinal with the typed check, so a value its constraint admits costs
// no allocation, whichever of the four typed arms reads it.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// REQ-103 — primitiveViolations checks each of the three typed inputs, and a
// CODE_REFERENCE through the code phrase it carries, without converting the
// value to an any: an admitted value allocates nothing. The BMI walk bound
// (walk_alloc_test.go) covers only the arms BMI's template reaches, codes
// and quantities, so each arm is held here.
func TestPrimitiveViolations_REQ103_TypedArmsAllocateNothing(t *testing.T) {
	code := rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at0001"}
	phrase := constraints.CodePhrase{Terminology: "local", CodeList: []string{"at0001", "at0002"}}
	cases := []struct {
		name  string
		pc    constraints.PrimitiveConstraint
		value any
	}{
		{"code phrase", phrase, &code},
		{"code reference", constraints.CodeReference{CodePhrase: phrase, ReferenceSetURI: "terminology:local"}, &code},
		{"quantity", constraints.DvQuantity{Units: []constraints.QuantityUnit{{Units: "kg"}}}, &rm.DVQuantity{Magnitude: 72, Units: "kg"}},
		{"ordinal", constraints.CDvOrdinal{Values: []constraints.OrdinalSymbol{
			{Value: 1, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0002"}},
		}}, &rm.DVOrdinal{Value: 1, Symbol: rm.DVCodedText{DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at0002"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if v := primitiveViolations(tc.pc, tc.value); len(v) != 0 {
				t.Fatalf("precondition: primitiveViolations(%T, %T) = %v, want none", tc.pc, tc.value, v)
			}
			if got := testing.AllocsPerRun(50, func() { _ = primitiveViolations(tc.pc, tc.value) }); got != 0 {
				t.Errorf("primitiveViolations(%T, %T) allocates %.0f objects per call, want 0: the value is converted to an any for Validate", tc.pc, tc.value, got)
			}
		})
	}
}
