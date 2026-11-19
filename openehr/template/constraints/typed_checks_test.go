package constraints_test

// typed_checks_test.go: REQ-103. clinical-modeling.md § Validate contract
// gives three constraints a check that takes its structured input typed:
// CodePhrase.ValidateCode, which a CodeReference has through the code phrase
// it carries, DvQuantity.ValidateQuantity and CDvOrdinal.ValidateOrdinal. Each
// returns what Validate returns for the same value. The tables below cover
// each violation each check reports, and the values it admits, and pin the
// codes as well, so a clause dropped from the one body both share fails here
// too. The second test holds each check to no allocation on an admitted
// value, which is what a caller gains by not converting it to any.

import (
	"fmt"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// typedSink keeps the result of a check measured by testing.AllocsPerRun.
var typedSink []constraints.Violation

// typedResultProblem says how typed, the result of a typed check, departs
// from untyped, the result of Validate for the same value, or from want, the
// codes it must carry in order. A nil want means the value is admitted, so
// typed must be nil. It returns "" when typed is as it must be.
func typedResultProblem(typed, untyped []constraints.Violation, want []constraints.ViolationCode) string {
	if !slices.Equal(typed, untyped) {
		return fmt.Sprintf("= %+v, Validate gives %+v for the same value; want the same", typed, untyped)
	}
	if want == nil {
		if typed != nil {
			return fmt.Sprintf("= %+v, want nil (admitted)", typed)
		}
		return ""
	}
	codes := make([]constraints.ViolationCode, len(typed))
	for i, v := range typed {
		codes[i] = v.Code
	}
	if !slices.Equal(codes, want) {
		return fmt.Sprintf("= %+v, want codes %v", typed, want)
	}
	return ""
}

// codeChecker is what a CodePhrase and a CodeReference share: the untyped
// check and the typed one.
type codeChecker interface {
	Validate(value any) []constraints.Violation
	ValidateCode(ref constraints.CodedTermRef) []constraints.Violation
}

func TestREQ103_TypedChecksReturnWhatValidateReturns(t *testing.T) {
	t.Parallel()
	closed := func(lo, hi float64) constraints.NumericRange {
		return constraints.NumericRange{Lower: lo, Upper: hi, LowerInclusive: true, UpperInclusive: true}
	}
	ref := func(terminology, code string) constraints.CodedTermRef {
		return constraints.CodedTermRef{Terminology: terminology, CodeString: code}
	}

	t.Run("code", func(t *testing.T) {
		t.Parallel()
		list := constraints.CodePhrase{Terminology: "openehr", CodeList: []string{"433", "434"}}
		external := constraints.CodePhrase{Terminology: "SNOMED-CT"}
		anyTerminology := constraints.CodePhrase{CodeList: []string{"at0001"}}
		// A code reference is judged by the code phrase it carries; the
		// terminology its URI names is not consulted.
		reference := constraints.CodeReference{CodePhrase: list, ReferenceSetURI: "terminology:SNOMED-CT"}
		openReference := constraints.CodeReference{ReferenceSetURI: "terminology:SNOMED-CT"}
		cases := []struct {
			name string
			c    codeChecker
			ref  constraints.CodedTermRef
			want []constraints.ViolationCode
		}{
			{name: "listed code in the terminology", c: list, ref: ref("openehr", "433")},
			{name: "listed code, no terminology given", c: list, ref: ref("", "434")},
			{name: "external, any code in the terminology", c: external, ref: ref("SNOMED-CT", "any code")},
			{name: "listed code, any terminology", c: anyTerminology, ref: ref("local", "at0001")},
			{name: "terminology mismatch", c: list, ref: ref("SNOMED-CT", "433"), want: []constraints.ViolationCode{constraints.CodeInvalidValue}},
			{name: "external, terminology mismatch", c: external, ref: ref("LOINC", "1234-5"), want: []constraints.ViolationCode{constraints.CodeInvalidValue}},
			{name: "code not in list", c: list, ref: ref("openehr", "999"), want: []constraints.ViolationCode{constraints.CodeNotInList}},
			{name: "terminology mismatch and code not in list", c: list, ref: ref("SNOMED-CT", "999"), want: []constraints.ViolationCode{constraints.CodeInvalidValue, constraints.CodeNotInList}},
			{name: "reference, listed code", c: reference, ref: ref("openehr", "433")},
			{name: "reference, the URI's terminology", c: reference, ref: ref("SNOMED-CT", "433"), want: []constraints.ViolationCode{constraints.CodeInvalidValue}},
			{name: "reference, code not in list", c: reference, ref: ref("openehr", "999"), want: []constraints.ViolationCode{constraints.CodeNotInList}},
			{name: "reference naming no terminology, another terminology", c: openReference, ref: ref("LOINC", "1234-5")},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				if p := typedResultProblem(tc.c.ValidateCode(tc.ref), tc.c.Validate(tc.ref), tc.want); p != "" {
					t.Errorf("%T.ValidateCode(%s) %s", tc.c, tc.ref, p)
				}
			})
		}
	})

	t.Run("quantity", func(t *testing.T) {
		t.Parallel()
		pressure := constraints.DvQuantity{Units: []constraints.QuantityUnit{
			{Units: "mm[Hg]", Magnitude: closed(0, 300)},
			{Units: "kPa", Magnitude: closed(0, 40)},
		}}
		// Two entries share the units mm; the first also bounds the precision.
		several := constraints.DvQuantity{Units: []constraints.QuantityUnit{
			{Units: "mm", Magnitude: closed(0, 10), Precision: closed(0, 0)},
			{Units: "kg", Magnitude: closed(0, 1000)},
			{Units: "mm", Magnitude: closed(100, 200)},
		}}
		length := constraints.DvQuantity{
			Property:      &constraints.CodedTermRef{Terminology: "openehr", CodeString: "122"},
			PropertyUnits: []string{"m", "cm", "mm"},
		}
		q := func(magnitude float64, units string, precision int) constraints.QuantityValue {
			return constraints.QuantityValue{Magnitude: magnitude, Units: units, Precision: precision}
		}
		cases := []struct {
			name string
			c    constraints.DvQuantity
			q    constraints.QuantityValue
			want []constraints.ViolationCode
		}{
			{name: "listed units, magnitude inside", c: pressure, q: q(120, "mm[Hg]", -1)},
			{name: "second entry's units, magnitude inside", c: pressure, q: q(20, "kPa", 1)},
			{name: "units unknown, units listed", c: pressure, q: q(120, "psi", -1), want: []constraints.ViolationCode{constraints.CodeUnitUnknown}},
			{name: "magnitude out of range", c: pressure, q: q(400, "mm[Hg]", -1), want: []constraints.ViolationCode{constraints.CodeOutOfRange}},
			{name: "precision out of range", c: several, q: q(5, "mm", 3), want: []constraints.ViolationCode{constraints.CodeOutOfRange}},
			{name: "magnitude and precision out of range", c: several, q: q(50, "mm", 3), want: []constraints.ViolationCode{constraints.CodeOutOfRange, constraints.CodeOutOfRange}},
			{name: "several entries with the units, one admitting", c: several, q: q(150, "mm", 3)},
			{name: "several entries with the units, none admitting", c: several, q: q(50, "mm", -1), want: []constraints.ViolationCode{constraints.CodeOutOfRange}},
			{name: "property units only, units of the property", c: length, q: q(1, "cm", -1)},
			{name: "property units only, units unknown", c: length, q: q(1, "mg", -1), want: []constraints.ViolationCode{constraints.CodeUnitUnknown}},
			{name: "no units and no property units", c: constraints.DvQuantity{}, q: q(1, "furlong", -1)},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				if p := typedResultProblem(tc.c.ValidateQuantity(tc.q), tc.c.Validate(tc.q), tc.want); p != "" {
					t.Errorf("ValidateQuantity(%v %s, precision %d) %s", tc.q.Magnitude, tc.q.Units, tc.q.Precision, p)
				}
			})
		}
	})

	t.Run("ordinal", func(t *testing.T) {
		t.Parallel()
		scale := constraints.CDvOrdinal{Values: []constraints.OrdinalSymbol{
			{Value: 0, Symbol: ref("local", "at0001")},
			{Value: 1, Symbol: ref("", "at0002")},
		}}
		pair := func(value int, terminology, code string) constraints.OrdinalSymbol {
			return constraints.OrdinalSymbol{Value: value, Symbol: ref(terminology, code)}
		}
		cases := []struct {
			name string
			c    constraints.CDvOrdinal
			s    constraints.OrdinalSymbol
			want []constraints.ViolationCode
		}{
			{name: "listed pair", c: scale, s: pair(0, "local", "at0001")},
			{name: "listed pair, listed terminology empty", c: scale, s: pair(1, "SNOMED-CT", "at0002")},
			{name: "listed pair, input terminology empty", c: scale, s: pair(0, "", "at0001")},
			{name: "listed value, other code", c: scale, s: pair(0, "local", "at0002"), want: []constraints.ViolationCode{constraints.CodeNotInList}},
			{name: "unlisted value, listed code", c: scale, s: pair(5, "local", "at0001"), want: []constraints.ViolationCode{constraints.CodeNotInList}},
			{name: "listed pair, other non-empty terminology", c: scale, s: pair(0, "SNOMED-CT", "at0001"), want: []constraints.ViolationCode{constraints.CodeNotInList}},
			{name: "empty Values", c: constraints.CDvOrdinal{}, s: pair(3, "local", "at0003")},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				if p := typedResultProblem(tc.c.ValidateOrdinal(tc.s), tc.c.Validate(tc.s), tc.want); p != "" {
					t.Errorf("ValidateOrdinal(%d %s) %s", tc.s.Value, tc.s.Symbol, p)
				}
			})
		}
	})
}

// TestREQ103_TypedChecksDoNotBoxTheirInput holds each typed check to no
// allocation on an admitted value. Converting the value to any for Validate
// allocates, so a typed check that went through Validate fails here. It does
// not run in parallel: testing.AllocsPerRun counts the allocations of the
// whole process.
func TestREQ103_TypedChecksDoNotBoxTheirInput(t *testing.T) {
	closed := func(lo, hi float64) constraints.NumericRange {
		return constraints.NumericRange{Lower: lo, Upper: hi, LowerInclusive: true, UpperInclusive: true}
	}
	code := constraints.CodePhrase{Terminology: "openehr", CodeList: []string{"433", "434"}}
	reference := constraints.CodeReference{CodePhrase: code, ReferenceSetURI: "terminology:openehr"}
	ref := constraints.CodedTermRef{Terminology: "openehr", CodeString: "434"}
	// The first entry with the value's units admits it: a same-units entry
	// that refuses the value builds its violations before a later one
	// admits it, which allocates whatever the input's type.
	listed := constraints.DvQuantity{Units: []constraints.QuantityUnit{
		{Units: "kg", Magnitude: closed(0, 1000)},
		{Units: "mm", Magnitude: closed(0, 10), Precision: closed(0, 2)},
		{Units: "mm", Magnitude: closed(100, 200)},
	}}
	property := constraints.DvQuantity{PropertyUnits: []string{"m", "cm", "mm"}}
	q := constraints.QuantityValue{Magnitude: 5, Units: "mm", Precision: 1}
	ordinal := constraints.CDvOrdinal{Values: []constraints.OrdinalSymbol{
		{Value: 0, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0001"}},
		{Value: 1, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0002"}},
	}}
	s := constraints.OrdinalSymbol{Value: 1, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0002"}}

	checks := []struct {
		name  string
		check func() []constraints.Violation
	}{
		{name: "CodePhrase.ValidateCode", check: func() []constraints.Violation { return code.ValidateCode(ref) }},
		{name: "CodeReference.ValidateCode", check: func() []constraints.Violation { return reference.ValidateCode(ref) }},
		{name: "DvQuantity.ValidateQuantity, units listed", check: func() []constraints.Violation { return listed.ValidateQuantity(q) }},
		{name: "DvQuantity.ValidateQuantity, property units", check: func() []constraints.Violation { return property.ValidateQuantity(q) }},
		{name: "CDvOrdinal.ValidateOrdinal", check: func() []constraints.Violation { return ordinal.ValidateOrdinal(s) }},
	}
	for _, tc := range checks {
		if got := tc.check(); got != nil {
			t.Fatalf("%s = %+v, want nil: the value must be admitted for the count to mean anything", tc.name, got)
		}
		if n := testing.AllocsPerRun(100, func() { typedSink = tc.check() }); n != 0 {
			t.Errorf("%s allocates %v times per call on an admitted value, want 0", tc.name, n)
		}
	}
}
