package rminfo_test

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// REQ-100 follow-up Phase 4-bis — Default must report the
// well-known RM-mandatory composition attributes: category, language,
// territory, composer. The OPT typically omits these — downstream
// compiled-template logic relies on Default.RequiredAttributes to
// inject them.
func TestDefault_CompositionMandatory(t *testing.T) {
	want := []string{"category", "composer", "language", "territory"}
	got := rminfo.Default.RequiredAttributes("COMPOSITION")
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("RequiredAttributes(COMPOSITION) missing %q (got %v)", w, got)
		}
	}
}

// Phase 4-bis — AttributeRMType returns the BMM-declared type;
// containers report the element type, not the container type.
func TestDefault_AttributeRMType(t *testing.T) {
	cases := []struct {
		parent, attr string
		want         string
	}{
		{"COMPOSITION", "category", "DV_CODED_TEXT"},
		{"COMPOSITION", "language", "CODE_PHRASE"},
		{"COMPOSITION", "content", "CONTENT_ITEM"}, // element type, container unwrapped
		{"OBSERVATION", "data", "HISTORY"},
		{"SECTION", "items", "CONTENT_ITEM"},
	}
	for _, tc := range cases {
		got, ok := rminfo.Default.AttributeRMType(tc.parent, tc.attr)
		if !ok {
			t.Errorf("AttributeRMType(%q, %q) not found", tc.parent, tc.attr)
			continue
		}
		if got != tc.want {
			t.Errorf("AttributeRMType(%q, %q) = %q, want %q", tc.parent, tc.attr, got, tc.want)
		}
	}
}

// Phase 4-bis — IsContainer reports multi-valued attributes.
func TestDefault_IsContainer(t *testing.T) {
	cases := []struct {
		parent, attr string
		want         bool
	}{
		{"COMPOSITION", "content", true},
		{"COMPOSITION", "links", true},
		{"COMPOSITION", "category", false},
		{"OBSERVATION", "data", false},
		{"SECTION", "items", true},
	}
	for _, tc := range cases {
		got, ok := rminfo.Default.IsContainer(tc.parent, tc.attr)
		if !ok {
			t.Errorf("IsContainer(%q, %q) not found", tc.parent, tc.attr)
			continue
		}
		if got != tc.want {
			t.Errorf("IsContainer(%q, %q) = %v, want %v", tc.parent, tc.attr, got, tc.want)
		}
	}
}

// Phase 4-bis — KnownRMTypes returns the full sorted set; spot-check
// a representative sample so the test fails when a class is
// accidentally pruned from the codegen plan.
func TestDefault_KnownRMTypes(t *testing.T) {
	got := rminfo.Default.KnownRMTypes()
	if len(got) < 50 {
		t.Errorf("KnownRMTypes returned %d entries, expected >= 50 (codegen pruned too aggressively?)", len(got))
	}
	if !slices.IsSorted(got) {
		t.Errorf("KnownRMTypes not sorted")
	}
	want := []string{
		"COMPOSITION", "OBSERVATION", "EVALUATION", "INSTRUCTION",
		"ACTION", "SECTION", "ADMIN_ENTRY", "CLUSTER", "ELEMENT",
		"DV_QUANTITY", "DV_CODED_TEXT", "DV_TEXT", "CODE_PHRASE",
		"HISTORY", "EVENT",
	}
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("KnownRMTypes missing %q", w)
		}
	}
}

// Phase 4-bis — unknown parent / attribute returns ok=false; never
// panics on bad input.
func TestDefault_UnknownLookups(t *testing.T) {
	if got := rminfo.Default.RequiredAttributes("NOT_AN_RM_TYPE"); got != nil {
		t.Errorf("RequiredAttributes(unknown) = %v, want nil", got)
	}
	if _, ok := rminfo.Default.AttributeRMType("COMPOSITION", "nope"); ok {
		t.Errorf("AttributeRMType(COMPOSITION, nope) reported ok")
	}
	if _, ok := rminfo.Default.IsContainer("NOT_A_TYPE", "any"); ok {
		t.Errorf("IsContainer(unknown, any) reported ok")
	}
}

// Phase 4-bis — New accepts caller-supplied synthetic data for
// unit-test substitution.
func TestNew_AcceptsSyntheticData(t *testing.T) {
	data := map[string]rminfo.ClassMeta{
		"FAKE": {
			Attributes: map[string]rminfo.AttrMeta{
				"only_field": {TypeName: "String", Required: true, Container: false},
			},
			AttrOrder: []string{"only_field"},
		},
	}
	l := rminfo.New(data)
	if got := l.RequiredAttributes("FAKE"); !slices.Equal(got, []string{"only_field"}) {
		t.Errorf("New(...).RequiredAttributes(FAKE) = %v, want [only_field]", got)
	}
	if tn, ok := l.AttributeRMType("FAKE", "only_field"); !ok || tn != "String" {
		t.Errorf("AttributeRMType(FAKE, only_field) = (%q, %v), want (String, true)", tn, ok)
	}
}
