package constraints_test

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// REQ-103: C_CODE_REFERENCE maps to CodeReference, which carries the code
// phrase constraint it refines. Validate and ExampleValue are that
// constraint's: a value is judged by its terminology and code list, and the
// reference set the URI names, external terminology, is not looked up.
func TestREQ103_CodeReference_JudgesByItsCodePhrase(t *testing.T) {
	t.Parallel()
	phrase := constraints.CodePhrase{Terminology: "SNOMED-CT", CodeList: []string{"22298006", "38341003"}}
	ref := constraints.CodeReference{CodePhrase: phrase, ReferenceSetURI: "terminology:SNOMED-CT"}
	tests := []struct {
		name  string
		value any
		want  []constraints.ViolationCode
	}{
		{name: "listed code", value: constraints.CodedTermRef{Terminology: "SNOMED-CT", CodeString: "22298006"}},
		{name: "listed bare code", value: "38341003"},
		{name: "code not listed", value: "1234", want: []constraints.ViolationCode{constraints.CodeNotInList}},
		{name: "other terminology", value: constraints.CodedTermRef{Terminology: "LOINC", CodeString: "22298006"}, want: []constraints.ViolationCode{constraints.CodeInvalidValue}},
		{name: "wrong Go type", value: 7, want: []constraints.ViolationCode{constraints.CodeWrongType}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ref.Validate(tc.value)
			if codes := violationCodes(got); !slices.Equal(codes, tc.want) {
				t.Errorf("CodeReference.Validate(%#v) codes = %v, want %v", tc.value, codes, tc.want)
			}
			if w := phrase.Validate(tc.value); !slices.Equal(violationCodes(w), violationCodes(got)) {
				t.Errorf("CodeReference.Validate(%#v) = %+v, want its CodePhrase's %+v", tc.value, got, w)
			}
		})
	}
	if got, want := ref.ExampleValue(), phrase.ExampleValue(); got != want {
		t.Errorf("CodeReference.ExampleValue() = %#v, want its CodePhrase's %#v", got, want)
	}
}

// REQ-103: CodePhraseOf reads a CodePhrase and a CodeReference alike, as the
// code phrase constraint each carries, and reports false for any other
// constraint and for none.
func TestREQ103_CodePhraseOf(t *testing.T) {
	t.Parallel()
	phrase := constraints.CodePhrase{Terminology: "local", CodeList: []string{"at0001"}}
	tests := []struct {
		name   string
		pc     constraints.PrimitiveConstraint
		want   constraints.CodePhrase
		wantOK bool
	}{
		{name: "code phrase", pc: phrase, want: phrase, wantOK: true},
		{name: "code reference", pc: constraints.CodeReference{CodePhrase: phrase, ReferenceSetURI: "terminology:local"}, want: phrase, wantOK: true},
		{name: "code reference with no terminology", pc: constraints.CodeReference{ReferenceSetURI: "terminology:SNOMED-CT"}, wantOK: true},
		{name: "constraint reference", pc: constraints.ConstraintRef{Reference: "ac0001"}},
		{name: "string", pc: constraints.CString{}},
		{name: "none", pc: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := constraints.CodePhraseOf(tc.pc)
			if ok != tc.wantOK || got.Terminology != tc.want.Terminology || !slices.Equal(got.CodeList, tc.want.CodeList) {
				t.Errorf("CodePhraseOf(%#v) = %#v, %t; want %#v, %t", tc.pc, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// violationCodes returns the code of each violation, in order, or nil for
// none.
func violationCodes(vs []constraints.Violation) []constraints.ViolationCode {
	var codes []constraints.ViolationCode
	for _, v := range vs {
		codes = append(codes, v.Code)
	}
	return codes
}
