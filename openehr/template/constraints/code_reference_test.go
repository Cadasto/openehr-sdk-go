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

// REQ-103: CodePhraseWithTerminologyOf reads a CodePhrase and a
// CodeReference as CodePhraseOf does, except that a CodeReference whose code
// phrase names no terminology takes the one its terminology: URI names. A
// terminology the code phrase names is kept, a URI that names none leaves
// the phrase as it is, and any other constraint and none report false.
func TestREQ103_CodePhraseWithTerminologyOf(t *testing.T) {
	t.Parallel()
	codes := []string{"22298006", "38341003"}
	tests := []struct {
		name   string
		pc     constraints.PrimitiveConstraint
		want   constraints.CodePhrase
		wantOK bool
	}{
		{
			name:   "code phrase",
			pc:     constraints.CodePhrase{Terminology: "local", CodeList: []string{"at0001"}},
			want:   constraints.CodePhrase{Terminology: "local", CodeList: []string{"at0001"}},
			wantOK: true,
		},
		{name: "code phrase with no terminology", pc: constraints.CodePhrase{CodeList: codes}, want: constraints.CodePhrase{CodeList: codes}, wantOK: true},
		{
			name:   "code reference whose URI names the terminology",
			pc:     constraints.CodeReference{ReferenceSetURI: "terminology:SNOMED-CT"},
			want:   constraints.CodePhrase{Terminology: "SNOMED-CT"},
			wantOK: true,
		},
		{
			name:   "code reference whose URI names the terminology, with codes",
			pc:     constraints.CodeReference{CodeList: codes, ReferenceSetURI: "terminology:SNOMED-CT"},
			want:   constraints.CodePhrase{Terminology: "SNOMED-CT", CodeList: codes},
			wantOK: true,
		},
		{
			name:   "code reference whose URI names the terminology and its version",
			pc:     constraints.CodeReference{ReferenceSetURI: "terminology:LOINC/2.73"},
			want:   constraints.CodePhrase{Terminology: "LOINC(2.73)"},
			wantOK: true,
		},
		{
			name:   "code reference whose code phrase names the terminology",
			pc:     constraints.CodeReference{Terminology: "LOINC", CodeList: codes, ReferenceSetURI: "terminology:SNOMED-CT"},
			want:   constraints.CodePhrase{Terminology: "LOINC", CodeList: codes},
			wantOK: true,
		},
		{
			name:   "code reference whose URI is of another scheme",
			pc:     constraints.CodeReference{CodeList: codes, ReferenceSetURI: "http://snomed.info/sct"},
			want:   constraints.CodePhrase{CodeList: codes},
			wantOK: true,
		},
		{name: "code reference whose URI names no terminology", pc: constraints.CodeReference{ReferenceSetURI: "terminology:"}, wantOK: true},
		{name: "constraint reference", pc: constraints.ConstraintRef{Reference: "ac0001"}},
		{name: "string", pc: constraints.CString{}},
		{name: "none", pc: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := constraints.CodePhraseWithTerminologyOf(tc.pc)
			if ok != tc.wantOK || got.Terminology != tc.want.Terminology || !slices.Equal(got.CodeList, tc.want.CodeList) {
				t.Errorf("CodePhraseWithTerminologyOf(%#v) = %#v, %t; want %#v, %t", tc.pc, got, ok, tc.want, tc.wantOK)
			}
			if plain, _ := constraints.CodePhraseOf(tc.pc); !slices.Equal(got.CodeList, plain.CodeList) {
				t.Errorf("CodePhraseWithTerminologyOf(%#v) codes = %q, want CodePhraseOf's %q", tc.pc, got.CodeList, plain.CodeList)
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

// REQ-103: ReferenceSetTerminology reads the terminology id a terminology:
// URI names, terminology:<name>[/<version>] with any query or fragment after
// it, as the TERMINOLOGY_ID <name> or <name>(<version>). The scheme is
// matched in any case and the name is kept as written. A URI of another
// scheme, one with no name, and one that does not take that form name none.
func TestREQ103_CodeReference_ReferenceSetTerminology(t *testing.T) {
	t.Parallel()
	tests := []struct {
		uri    string
		want   string
		wantOK bool
	}{
		{uri: "terminology:SNOMED-CT", want: "SNOMED-CT", wantOK: true},
		{uri: "terminology:SNOMED-CT?subset=x", want: "SNOMED-CT", wantOK: true},
		{uri: "terminology:LOINC/2.73", want: "LOINC(2.73)", wantOK: true},
		{uri: "terminology:LOINC/2.73?subset=x#part", want: "LOINC(2.73)", wantOK: true},
		{uri: "terminology:ICD10#part?x", want: "ICD10", wantOK: true},
		{uri: "TERMINOLOGY:ICD10", want: "ICD10", wantOK: true},
		{uri: "Terminology:snomed-ct", want: "snomed-ct", wantOK: true},
		{uri: "terminology:"},
		{uri: "terminology:/v1"},
		{uri: "terminology:?subset=x"},
		{uri: "terminology://snomed.info/sct"},
		{uri: "terminology:LOINC/"},
		{uri: "terminology:LOINC/2.73/x"},
		{uri: "terminology:SNOMED CT"},
		{uri: "terminology:LOINC/2.73 x"},
		{uri: "terminology:SNOMED\tCT"},
		{uri: "terminology:SNOMED\x7fCT"},
		{uri: "terminology:LOINC(2.73)"},
		{uri: "terminology:LOINC/(2.73)"},
		{uri: "terminology:LOINC)"},
		{uri: "http://snomed.info/sct"},
		{uri: "urn:oid:2.16.840.1.113883.6.96"},
		{uri: "//fhir.hl7.org/ValueSet/$expand?url=http://snomed.info/sct"},
		{uri: "terminologies:SNOMED-CT"},
		{uri: "SNOMED-CT"},
		{uri: ""},
	}
	for _, tc := range tests {
		t.Run(tc.uri, func(t *testing.T) {
			t.Parallel()
			ref := constraints.CodeReference{ReferenceSetURI: tc.uri}
			got, ok := ref.ReferenceSetTerminology()
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("CodeReference{ReferenceSetURI: %q}.ReferenceSetTerminology() = %q, %t; want %q, %t", tc.uri, got, ok, tc.want, tc.wantOK)
			}
		})
	}
	t.Run("a code phrase that names a terminology", func(t *testing.T) {
		t.Parallel()
		ref := constraints.CodeReference{Terminology: "LOINC", ReferenceSetURI: "terminology:SNOMED-CT"}
		if got, ok := ref.ReferenceSetTerminology(); got != "SNOMED-CT" || !ok {
			t.Errorf("ReferenceSetTerminology() = %q, %t; want the URI's SNOMED-CT, true: it reads the URI alone", got, ok)
		}
	})
}
