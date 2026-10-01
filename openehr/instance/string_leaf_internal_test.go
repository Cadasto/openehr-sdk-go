package instance

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// TestREQ107_StringForConstraintMatchesWholePattern is the REQ-107 /
// REQ-103 check that a pattern-only C_STRING gets a value the whole
// pattern matches, for the pattern shapes vendored OPTs use.
func TestREQ107_StringForConstraintMatchesWholePattern(t *testing.T) {
	for _, pattern := range []string{
		`XYZ[0-9]+`,
		`openEHR-EHR-ACTION\.medication\.v1`,
		`openEHR-EHR-ACTION\.medication(-[a-zA-Z0-9_]+)*\.v1`,
		`openEHR-EHR-ACTION\..*`,
		`[A-Z]{3}-\d{2}`,
		`(foo|bar)baz`,
		`^\w+$`,
		`[^a]x?`,
	} {
		cs := constraints.NewCString(pattern, nil, "")
		got, err := stringForConstraint(cs, cs.ExampleValue())
		if err != nil {
			t.Errorf("stringForConstraint(%q): %v", pattern, err)
			continue
		}
		if !regexp.MustCompile(`^(?:` + pattern + `)$`).MatchString(got) {
			t.Errorf("stringForConstraint(%q) = %q, want a whole match", pattern, got)
		}
	}
}

// TestREQ107_StringForConstraintPrefersExampleThenList pins the order of
// choice: the example when the constraint accepts it, then the first
// list member the pattern accepts.
func TestREQ107_StringForConstraintPrefersExampleThenList(t *testing.T) {
	cases := []struct {
		name    string
		cs      constraints.CString
		example string
		want    string
	}{
		{name: "open string keeps the example", cs: constraints.NewCString("", nil, ""), example: "example", want: "example"},
		{name: "pattern the whole example matches", cs: constraints.NewCString("ex[a-z]+", nil, ""), example: "example", want: "example"},
		{name: "pattern matching only part of the example", cs: constraints.NewCString("amp", nil, ""), example: "example", want: "amp"},
		{name: "first member the pattern accepts", cs: constraints.NewCString("[0-9]+", []string{"abc", "123"}, ""), example: "abc", want: "123"},
	}
	for _, tc := range cases {
		got, err := stringForConstraint(tc.cs, tc.example)
		if err != nil || got != tc.want {
			t.Errorf("%s: stringForConstraint = %q, %v; want %q, nil", tc.name, got, err, tc.want)
		}
	}
	if _, err := stringForConstraint(constraints.NewCString("", nil, ""), 7); err == nil {
		t.Error("stringForConstraint(7) = nil error, want one for a non-string example")
	}
	if _, err := stringForConstraint(constraints.NewCString("[0-9]+", []string{"abc"}, ""), "abc"); !errors.Is(err, errNoStringValue) {
		t.Errorf("no member matches: error = %v, want errNoStringValue", err)
	}
}

// TestREQ107_StringLeafWritesTheNamedAttribute pins the attribute
// dispatch of applyStringLeaf for each required String attribute it
// writes, including the "" form that names the value's main attribute.
func TestREQ107_StringLeafWritesTheNamedAttribute(t *testing.T) {
	cs := constraints.NewCString("", []string{"XYZ"}, "")
	cases := []struct {
		name  string
		value any
		attr  string
		read  func(any) string
	}{
		{"DV_TEXT.value", &rm.DVText{}, "value", func(v any) string { return v.(*rm.DVText).Value }},
		{"DV_TEXT main", &rm.DVText{}, "", func(v any) string { return v.(*rm.DVText).Value }},
		{"DV_CODED_TEXT.value", &rm.DVCodedText{}, "value", func(v any) string { return v.(*rm.DVCodedText).Value }},
		{"CODE_PHRASE main", &rm.CodePhrase{}, "", func(v any) string { return v.(*rm.CodePhrase).CodeString }},
		{"DV_PARSABLE.value", &rm.DVParsable{}, "value", func(v any) string { return v.(*rm.DVParsable).Value }},
		{"DV_PARSABLE.formalism", &rm.DVParsable{}, "formalism", func(v any) string { return v.(*rm.DVParsable).Formalism }},
		{"DV_IDENTIFIER main", &rm.DVIdentifier{}, "", func(v any) string { return v.(*rm.DVIdentifier).ID }},
		{"DV_URI.value", &rm.DVURI{}, "value", func(v any) string { return v.(*rm.DVURI).Value }},
		{"DV_EHR_URI.value", &rm.DVEHRURI{}, "value", func(v any) string { return v.(*rm.DVEHRURI).Value }},
		{"DV_DATE.value", &rm.DVDate{}, "value", func(v any) string { return v.(*rm.DVDate).Value }},
		{"DV_QUANTITY.units", &rm.DVQuantity{}, "units", func(v any) string { return v.(*rm.DVQuantity).Units }},
		{"ACTIVITY main", &rm.Activity{}, "", func(v any) string { return v.(*rm.Activity).ActionArchetypeID }},
		{"TERMINOLOGY_ID.value", &rm.TerminologyID{}, "value", func(v any) string { return v.(*rm.TerminologyID).Value }},
	}
	for _, tc := range cases {
		if err := applyStringLeaf(nil, tc.value, tc.attr, cs, "XYZ"); err != nil {
			t.Errorf("%s: applyStringLeaf: %v", tc.name, err)
			continue
		}
		if got := tc.read(tc.value); got != "XYZ" {
			t.Errorf("%s = %q after applyStringLeaf, want XYZ", tc.name, got)
		}
	}

	// An optional attribute and an unknown one are left alone.
	text := &rm.DVText{}
	if err := applyStringLeaf(nil, text, "formatting", cs, "XYZ"); err != nil || text.Formatting != nil || text.Value != "" {
		t.Errorf("DV_TEXT.formatting: applyStringLeaf = %v, value %q, formatting %v; want nil, both unset", err, text.Value, text.Formatting)
	}
	if err := applyStringLeaf(nil, text, "no_such", cs, "XYZ"); err != nil || text.Value != "" {
		t.Errorf("DV_TEXT.no_such: applyStringLeaf = %v, value %q; want nil, value unset", err, text.Value)
	}

	// No satisfying value: the error wraps ErrConstraintUnsatisfiable,
	// not the slot sentinel, names the RM type and the attribute, and the
	// field keeps its earlier value.
	parsable := &rm.DVParsable{Formalism: "text/plain"}
	err := applyStringLeaf(nil, parsable, "formalism", constraints.NewCString("[0-9]+", []string{"abc"}, ""), "abc")
	if !errors.Is(err, ErrConstraintUnsatisfiable) || errors.Is(err, ErrSlotFillUnsupported) {
		t.Errorf("unsatisfiable formalism: err %v, want ErrConstraintUnsatisfiable and not ErrSlotFillUnsupported", err)
	}
	if err != nil && !strings.Contains(err.Error(), "DV_PARSABLE.formalism") {
		t.Errorf("unsatisfiable formalism: err %q, want it to name DV_PARSABLE.formalism", err)
	}
	if parsable.Formalism != "text/plain" {
		t.Errorf("unsatisfiable formalism: formalism %q after the error, want text/plain kept", parsable.Formalism)
	}
}
