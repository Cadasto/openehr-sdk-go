package template_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// REQ-111 § Path lookup: ParsePath accepts the forms of the grammar.
func TestParsePath_ValidForms(t *testing.T) {
	cases := []struct {
		in   string
		want string // String() round-trip
	}{
		{"/", "/"},
		{"/content", "/content"},
		{"/category/defining_code", "/category/defining_code"},
		{"/content[at0001]", "/content[at0001]"},
		{"/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]", "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]"},
		{"/content[at0001]/data", "/content[at0001]/data"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			p, err := template.ParsePath(tc.in)
			if err != nil {
				t.Fatalf("ParsePath(%q): %v", tc.in, err)
			}
			if got := p.String(); got != tc.want {
				t.Errorf("round-trip = %q, want %q", got, tc.want)
			}
		})
	}
}

// REQ-111 § Path lookup: ParsePath rejects anything outside the grammar
// with ErrPathSyntax.
func TestParsePath_RejectsMalformed(t *testing.T) {
	cases := []struct {
		in     string
		reason string
	}{
		{"", "empty"},
		{"content", "missing leading slash"},
		{"/content/", "trailing slash"},
		{"//content", "empty segment"},
		{"/content[", "unclosed predicate"},
		{"/content[at0001", "unclosed predicate"},
		{"/content]", "unbalanced bracket"},
		{"/content[]", "empty predicate"},
		{"/[at0001]", "predicate without name"},
		// REQ-111 explicitly rejects AQL-style predicates.
		{"/content[name='Systolic']", "AQL predicate"},
		{"/content[at0001,name='x']", "multi-predicate"},
		{"/content[@id=x]", "@ marker"},
	}
	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			_, err := template.ParsePath(tc.in)
			if !errors.Is(err, template.ErrPathSyntax) {
				t.Fatalf("ParsePath(%q) = %v, want ErrPathSyntax", tc.in, err)
			}
		})
	}
}

// REQ-111 § Path lookup: the character after a closing ']' must be the
// segment separator '/' (or the end of input). Any other character is a
// grammar error, which guards against AQL-style trailing tags.
func TestParsePath_RejectsCharAfterCloseBracket(t *testing.T) {
	_, err := template.ParsePath("/content[at0001]extra")
	if !errors.Is(err, template.ErrPathSyntax) {
		t.Fatalf("got %v, want ErrPathSyntax", err)
	}
}

// Cardinality.String and IsValid on the two constants and one out of range.
func TestCardinality_StringAndIsValid(t *testing.T) {
	if got := template.Single.String(); got != "single" {
		t.Errorf("Single.String() = %q, want %q", got, "single")
	}
	if got := template.Multiple.String(); got != "multiple" {
		t.Errorf("Multiple.String() = %q, want %q", got, "multiple")
	}
	if !template.Single.IsValid() || !template.Multiple.IsValid() {
		t.Errorf("Single and Multiple must report IsValid()=true")
	}
	if template.Cardinality(99).IsValid() {
		t.Errorf("out-of-range Cardinality(99) must report IsValid()=false")
	}
	if got := template.Cardinality(99).String(); !strings.Contains(got, "99") {
		t.Errorf("Cardinality(99).String() = %q, want token containing 99", got)
	}
}
