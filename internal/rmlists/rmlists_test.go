package rmlists_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/rmlists"
)

// bmmClass is the part of a P_BMM class definition the test reads.
type bmmClass struct {
	Ancestors  []string               `json:"ancestors"`
	IsAbstract bool                   `json:"is_abstract"`
	Properties map[string]bmmProperty `json:"properties"`
	Invariants map[string]string      `json:"invariants"`
}

// bmmProperty is the part of a P_BMM property the test reads.
type bmmProperty struct {
	Type        string          `json:"_type"`
	IsMandatory bool            `json:"is_mandatory"`
	Cardinality *bmmCardinality `json:"cardinality"`
}

// bmmCardinality is the part of a P_BMM cardinality the test reads.
type bmmCardinality struct {
	Lower *int `json:"lower"`
}

// readRMBMM decodes the class definitions of the vendored RM BMM.
func readRMBMM(t *testing.T) map[string]bmmClass {
	t.Helper()
	return readBMM(t, "openehr_rm_1.2.0.bmm.json")
}

// readBMM decodes the class definitions of the vendored BMM file name.
func readBMM(t *testing.T, name string) map[string]bmmClass {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "resources", "bmm", name))
	if err != nil {
		t.Fatalf("read vendored BMM %s: %v", name, err)
	}
	var schema struct {
		ClassDefinitions map[string]bmmClass `json:"class_definitions"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("decode vendored BMM %s: %v", name, err)
	}
	if len(schema.ClassDefinitions) == 0 {
		t.Fatalf("the vendored BMM %s defines no classes", name)
	}
	return schema.ClassDefinitions
}

// nonEmptyInvariants returns the names of the invariants of class, and of
// every ancestor the BMM defines, that keep its attribute attr non-empty: an
// expression that says attr is not empty, such as DV_TEXT's
// "mappings /= void implies not mappings.is_empty".
func nonEmptyInvariants(defs map[string]bmmClass, class, attr string) []string {
	notEmpty := regexp.MustCompile(`\bnot\s+` + regexp.QuoteMeta(attr) + `\s*\.\s*(is_)?empty\b`)
	var found []string
	seen := map[string]bool{}
	var visit func(string)
	visit = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		c, ok := defs[name]
		if !ok {
			return
		}
		for inv, expr := range c.Invariants {
			if notEmpty.MatchString(expr) {
				found = append(found, name+"."+inv)
			}
		}
		for _, a := range c.Ancestors {
			visit(a)
		}
	}
	visit(class)
	slices.Sort(found)
	return found
}

// TestREQ052_KeepsEmptyMatchesBMM pins the closed list of item-structure
// lists canonical JSON writes as [] when present and empty to the vendored
// RM BMM (REQ-052 § An absent mandatory member, issue 341).
//
// Each listed attribute must be an optional P_BMM_CONTAINER_PROPERTY its own
// concrete class declares, with a cardinality lower bound of 0 or none, and
// no invariant of the class or an ancestor may keep it non-empty: writing []
// for an attribute such an invariant guards would encode an RM-invalid value,
// as DV_TEXT.mappings would. The generator keys the tag on the class that
// declares the field, so the class must be concrete and declare it itself.
// For every property the BMM defines, KeepsEmpty must answer true exactly for
// the listed pairs.
func TestREQ052_KeepsEmptyMatchesBMM(t *testing.T) {
	defs := readRMBMM(t)

	// The control: the invariant scan finds the guards it must, so an empty
	// result below is not a broken scan.
	for _, tc := range []struct{ class, attr, want string }{
		{class: "DV_TEXT", attr: "mappings", want: "DV_TEXT.Mappings_valid"},
		{class: "SECTION", attr: "items", want: "SECTION.Items_valid"},
		{class: "DV_PARAGRAPH", attr: "items", want: "DV_PARAGRAPH.Items_valid"},
		{class: "ITEM_LIST", attr: "links", want: "LOCATABLE.Links_valid"},
	} {
		if got := nonEmptyInvariants(defs, tc.class, tc.attr); !slices.Contains(got, tc.want) {
			t.Errorf("nonEmptyInvariants(%s, %s) = %v, want it to include %s: the scan misses an invariant the BMM declares", tc.class, tc.attr, got, tc.want)
		}
	}

	want := [][2]string{{"ITEM_LIST", "items"}, {"ITEM_TABLE", "rows"}, {"ITEM_TREE", "items"}}
	for _, pair := range want {
		class, attr := pair[0], pair[1]
		c, ok := defs[class]
		if !ok {
			t.Errorf("the vendored RM BMM defines no class %s", class)
			continue
		}
		if c.IsAbstract {
			t.Errorf("BMM %s is abstract; the generator keys the tag on a concrete class that declares the list", class)
		}
		p, ok := c.Properties[attr]
		if !ok {
			t.Errorf("BMM %s declares no property %s itself", class, attr)
			continue
		}
		if p.Type != "P_BMM_CONTAINER_PROPERTY" {
			t.Errorf("BMM %s.%s is a %s, want a P_BMM_CONTAINER_PROPERTY", class, attr, p.Type)
		}
		if p.IsMandatory {
			t.Errorf("BMM %s.%s is mandatory; a mandatory list already keeps a present empty one", class, attr)
		}
		if p.Cardinality != nil && p.Cardinality.Lower != nil && *p.Cardinality.Lower != 0 {
			t.Errorf("BMM %s.%s has cardinality lower bound %d, want 0 or none", class, attr, *p.Cardinality.Lower)
		}
		if got := nonEmptyInvariants(defs, class, attr); len(got) != 0 {
			t.Errorf("BMM %s.%s is kept non-empty by %v, so writing a present empty one as [] would be RM-invalid", class, attr, got)
		}
	}

	for class, c := range defs {
		for attr := range c.Properties {
			wantKeep := slices.Contains(want, [2]string{class, attr})
			if got := rmlists.KeepsEmpty(class, attr); got != wantKeep {
				t.Errorf("KeepsEmpty(%q, %q) = %t, want %t", class, attr, got, wantKeep)
			}
		}
	}

	for _, tc := range []struct{ class, attr string }{
		{class: "", attr: ""},
		{class: "ITEM_LIST", attr: "rows"},
		{class: "ITEM_TABLE", attr: "items"},
		{class: "item_list", attr: "items"},
		{class: "ITEM_LIST", attr: "Items"},
		{class: " ITEM_TREE", attr: "items"},
		{class: "ITEM_STRUCTURE", attr: "items"},
		{class: "DV_TEXT", attr: "mappings"},
	} {
		if rmlists.KeepsEmpty(tc.class, tc.attr) {
			t.Errorf("KeepsEmpty(%q, %q) = true, want false: not one of the listed pairs", tc.class, tc.attr)
		}
	}
}

// mandatoryListsWithoutLowerBound returns, sorted as "CLASS.attr", the
// P_BMM_CONTAINER_PROPERTY attributes of defs that are mandatory with a
// cardinality lower bound of 0 or none.
func mandatoryListsWithoutLowerBound(defs map[string]bmmClass) []string {
	var found []string
	for class, c := range defs {
		for attr, p := range c.Properties {
			if p.Type != "P_BMM_CONTAINER_PROPERTY" || !p.IsMandatory {
				continue
			}
			if p.Cardinality == nil || p.Cardinality.Lower == nil || *p.Cardinality.Lower == 0 {
				found = append(found, class+"."+attr)
			}
		}
	}
	slices.Sort(found)
	return found
}

// TestREQ052_MandatoryListsHaveLowerBound pins an agreement the vendored RM
// and BASE BMM keep: every mandatory list has a cardinality lower bound of 1
// or more (REQ-052 § An absent mandatory member, PROBE-030).
//
// The generator tags a list field by its lower bound alone, so, the lists
// KeepsEmpty names aside, a present empty list is written only when the
// bound is 1 or more, while
// testkit/expectedform keeps a present empty list the BMM marks mandatory.
// A mandatory list with a lower bound of 0 would be left out by the encoder
// and kept by the expected form, and PROBE-030 would fail on valid input.
func TestREQ052_MandatoryListsHaveLowerBound(t *testing.T) {
	// The control: the scan reports the pairs it must.
	zero, one := 0, 1
	control := map[string]bmmClass{"X": {Properties: map[string]bmmProperty{
		"lower_zero": {Type: "P_BMM_CONTAINER_PROPERTY", IsMandatory: true, Cardinality: &bmmCardinality{Lower: &zero}},
		"no_bound":   {Type: "P_BMM_CONTAINER_PROPERTY", IsMandatory: true},
		"lower_one":  {Type: "P_BMM_CONTAINER_PROPERTY", IsMandatory: true, Cardinality: &bmmCardinality{Lower: &one}},
		"optional":   {Type: "P_BMM_CONTAINER_PROPERTY"},
		"single":     {Type: "P_BMM_SINGLE_PROPERTY", IsMandatory: true},
	}}}
	if got, want := mandatoryListsWithoutLowerBound(control), []string{"X.lower_zero", "X.no_bound"}; !slices.Equal(got, want) {
		t.Errorf("mandatoryListsWithoutLowerBound(control) = %v, want %v", got, want)
	}

	for _, name := range []string{"openehr_rm_1.2.0.bmm.json", "openehr_base_1.3.0.bmm.json"} {
		if got := mandatoryListsWithoutLowerBound(readBMM(t, name)); len(got) != 0 {
			t.Errorf("%s: mandatory lists with a cardinality lower bound of 0 or none: %v; the generated encoder leaves a present empty one out and the expected form keeps it, so align the two before re-pinning", name, got)
		}
	}
}
