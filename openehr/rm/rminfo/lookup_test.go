package rminfo_test

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// REQ-111 (§ Reading the model), Phase 4-bis — Default must report the
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

// REQ-048 — the bound attribute type keeps the generic parameters the BMM
// declaration binds, spelled as ITS-JSON spells a generic instantiation,
// while AttributeRMType keeps answering the bare class. A container answers
// for its element, and a parameter left open on the declaring class keeps
// the bare spelling.
func TestDefault_AttributeBoundType_REQ048(t *testing.T) {
	b, ok := rminfo.Default.(rminfo.BoundTypeReporter)
	if !ok {
		t.Fatal("rminfo.Default does not implement BoundTypeReporter")
	}
	cases := []struct {
		parent, attr string
		wantBare     string
		wantBound    string
	}{
		{"PARTY_RELATIONSHIP", "time_validity", "DV_INTERVAL", "DV_INTERVAL<DV_DATE>"},
		{"CONTACT", "time_validity", "DV_INTERVAL", "DV_INTERVAL<DV_DATE>"},
		{"CAPABILITY", "time_validity", "DV_INTERVAL", "DV_INTERVAL<DV_DATE>"},
		{"ROLE", "time_validity", "DV_INTERVAL", "DV_INTERVAL<DV_DATE>"},
		{"PARTICIPATION", "time", "DV_INTERVAL", "DV_INTERVAL<DV_DATE_TIME>"},
		{"DV_QUANTITY", "normal_range", "DV_INTERVAL", "DV_INTERVAL<DV_QUANTITY>"},
		{"DV_COUNT", "normal_range", "DV_INTERVAL", "DV_INTERVAL<DV_COUNT>"},
		{"DV_PROPORTION", "normal_range", "DV_INTERVAL", "DV_INTERVAL<DV_PROPORTION>"},
		{"DV_QUANTITY", "other_reference_ranges", "REFERENCE_RANGE", "REFERENCE_RANGE<DV_QUANTITY>"},
		{"DV_COUNT", "other_reference_ranges", "REFERENCE_RANGE", "REFERENCE_RANGE<DV_COUNT>"},
		{"DV_PROPORTION", "other_reference_ranges", "REFERENCE_RANGE", "REFERENCE_RANGE<DV_PROPORTION>"},
		{"DV_ORDERED", "other_reference_ranges", "REFERENCE_RANGE", "REFERENCE_RANGE<DV_ORDERED>"},
		{"OBSERVATION", "data", "HISTORY", "HISTORY<ITEM_STRUCTURE>"},
		{"OBSERVATION", "state", "HISTORY", "HISTORY<ITEM_STRUCTURE>"},
		{"AUTHORED_RESOURCE", "translations", "Hash", "Hash<String,TRANSLATION_DETAILS>"},
		// Open on the declaring class, or declared with no parameter: bare.
		{"HISTORY", "events", "EVENT", "EVENT"},
		{"EVENT", "data", "T", "T"},
		{"DV_INTERVAL", "lower", "T", "T"},
		{"REFERENCE_RANGE", "range", "DV_INTERVAL", "DV_INTERVAL"},
		{"COMPOSITION", "category", "DV_CODED_TEXT", "DV_CODED_TEXT"},
	}
	for _, tc := range cases {
		got, ok := b.AttributeBoundType(tc.parent, tc.attr)
		if !ok || got != tc.wantBound {
			t.Errorf("AttributeBoundType(%q, %q) = (%q, %t), want (%q, true)", tc.parent, tc.attr, got, ok, tc.wantBound)
		}
		bare, ok := rminfo.Default.AttributeRMType(tc.parent, tc.attr)
		if !ok || bare != tc.wantBare {
			t.Errorf("AttributeRMType(%q, %q) = (%q, %t), want (%q, true)", tc.parent, tc.attr, bare, ok, tc.wantBare)
		}
	}
}

// REQ-048 — the bound type answers not-found exactly where AttributeRMType
// does, rather than a guessed or empty name reported as success.
func TestDefault_AttributeBoundTypeUnknown_REQ048(t *testing.T) {
	b, ok := rminfo.Default.(rminfo.BoundTypeReporter)
	if !ok {
		t.Fatal("rminfo.Default does not implement BoundTypeReporter")
	}
	for _, q := range [][2]string{{"NOT_AN_RM_TYPE", "data"}, {"COMPOSITION", "nope"}, {"", ""}} {
		if got, ok := b.AttributeBoundType(q[0], q[1]); ok || got != "" {
			t.Errorf("AttributeBoundType(%q, %q) = (%q, %t), want (\"\", false)", q[0], q[1], got, ok)
		}
	}
}

// REQ-048 — data built without the bound type, as every synthetic table
// written before the field existed is, answers the bare TypeName rather than
// an empty name.
func TestNew_AttributeBoundTypeFallsBackToTypeName_REQ048(t *testing.T) {
	l := rminfo.New(map[string]rminfo.ClassMeta{
		"FAKE": {
			Attributes: map[string]rminfo.AttrMeta{
				"unbound": {TypeName: "DV_INTERVAL"},
				"bound":   {TypeName: "DV_INTERVAL", BoundTypeName: "DV_INTERVAL<DV_DATE>"},
			},
			AttrOrder: []string{"unbound", "bound"},
		},
	})
	b, ok := l.(rminfo.BoundTypeReporter)
	if !ok {
		t.Fatal("rminfo.New(...) does not implement BoundTypeReporter")
	}
	for attr, want := range map[string]string{"unbound": "DV_INTERVAL", "bound": "DV_INTERVAL<DV_DATE>"} {
		if got, ok := b.AttributeBoundType("FAKE", attr); !ok || got != want {
			t.Errorf("AttributeBoundType(FAKE, %q) = (%q, %t), want (%q, true)", attr, got, ok, want)
		}
	}
}
