package validation_test

// optional_list_counts_test.go: REQ-102 § A mandatory child under an
// optional list. On an object the instance carries, each allowed child of a
// multi-valued attribute that is not a slot is counted against its
// occurrences whether the list holds items, is present and empty, or is
// absent, except an absent list the template requires by its existence,
// which fails existence alone, a list of primitive values, and a list the
// template prohibits, which holding no item draws nothing from its
// children. On a list the template leaves optional by its
// existence, the list's own cardinality is not reported when the list is
// absent or present and empty, since canonical JSON carries the one as the
// other.

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// optionalListOPT returns a template whose ITEM_TREE root carries items with
// the existence interval body existence and the cardinality interval body
// cardinality, each left out of the OPT where it is empty, admitting
// children.
func optionalListOPT(existence, cardinality string, children ...string) string {
	ex := ""
	if existence != "" {
		ex = `<existence>` + existence + `</existence>`
	}
	card := ""
	if cardinality != "" {
		card = `<cardinality><is_ordered>true</is_ordered><is_unique>false</is_unique><interval>` + cardinality + `</interval></cardinality>`
	}
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>existence_container</value></template_id><concept>existence_container</concept>
<definition><rm_type_name>ITEM_TREE</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>` + ex + strings.Join(children, "") + card + `
</attributes>
<archetype_id><value>` + containerArchetype("ITEM_TREE") + `</value></archetype_id></definition>
</template>`
}

// REQ-102 — a list the template leaves optional by its existence (0..1, or
// none stated) has each child counted against its occurrences whether it is
// absent, present and empty, or holds items, so a child whose occurrences
// lower bound is 1 draws its occurrences finding, at the child's own path,
// both when the list is absent and when it is present and empty. The list's
// own cardinality is reported in neither case, and still is when the list
// holds an item. A child that is optional, or that states no occurrences,
// draws nothing on an absent list. A list the template prohibits (existence
// 0..0) draws nothing from its children while it holds none, absent or
// present and empty, so an instance that leaves it out validates; holding
// an item, it draws `prohibited` alone. A list the template requires by its
// existence keeps its rules: absent, it draws `required` alone; present and
// empty, `required`, its cardinality and the child's occurrences.
func TestValidate_REQ102_MandatoryChildUnderAnOptionalList(t *testing.T) {
	t.Parallel()
	optional := fiInterval("0", "1")
	required := fiInterval("1", "1")
	prohibited := fiInterval("0", "0")
	oneOrMore := fromLowerUnbounded("1")
	tests := []struct {
		name string
		opt  string
		// items is the ITEM_TREE's items: nil for an absent list.
		items []rm.Item
		// want is the whole report, as identityKeys writes it.
		want []string
	}{
		{
			name: "absent optional list, child at occurrences 1..1",
			opt:  optionalListOPT(optional, oneOrMore, containerElement("1")),
			want: []string{"cardinality /items[at0001] template occurrences"},
		},
		{
			name:  "present empty optional list, child at occurrences 1..1",
			opt:   optionalListOPT(optional, oneOrMore, containerElement("1")),
			items: []rm.Item{},
			want:  []string{"cardinality /items[at0001] template occurrences"},
		},
		{
			name: "absent list with no existence stated, child at occurrences 1..1",
			opt:  optionalListOPT("", oneOrMore, containerElement("1")),
			want: []string{"cardinality /items[at0001] template occurrences"},
		},
		{
			name: "absent optional list, cardinality 1..*, optional child",
			opt:  optionalListOPT(optional, oneOrMore, containerElement("0")),
		},
		{
			name:  "present empty optional list, cardinality 1..*, optional child",
			opt:   optionalListOPT(optional, oneOrMore, containerElement("0")),
			items: []rm.Item{},
		},
		{
			name:  "present empty optional list, cardinality 1..*, no child",
			opt:   optionalListOPT(optional, oneOrMore),
			items: []rm.Item{},
		},
		{
			name: "absent optional list, child with no occurrences",
			opt:  optionalListOPT(optional, oneOrMore, containerElement("")),
		},
		{
			name:  "optional list holding an item, cardinality 2..*",
			opt:   optionalListOPT(optional, fromLowerUnbounded("2"), containerElement("1")),
			items: []rm.Item{textElement()},
			want:  []string{"cardinality /items template cardinality"},
		},
		{
			name: "absent prohibited list, child at occurrences 1..1",
			opt:  optionalListOPT(prohibited, oneOrMore, containerElement("1")),
		},
		{
			name:  "present empty prohibited list, child at occurrences 1..1",
			opt:   optionalListOPT(prohibited, oneOrMore, containerElement("1")),
			items: []rm.Item{},
		},
		{
			name:  "prohibited list holding an item, child at occurrences 1..1",
			opt:   optionalListOPT(prohibited, oneOrMore, containerElement("1")),
			items: []rm.Item{textElement()},
			want:  []string{"prohibited /items template existence"},
		},
		{
			name: "absent required list, child at occurrences 1..1",
			opt:  optionalListOPT(required, oneOrMore, containerElement("1")),
			want: []string{"required /items template existence"},
		},
		{
			name:  "present empty required list, child at occurrences 1..1",
			opt:   optionalListOPT(required, oneOrMore, containerElement("1")),
			items: []rm.Item{},
			want: []string{
				"cardinality /items template cardinality",
				"cardinality /items[at0001] template occurrences",
				"required /items template existence",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, tc.opt)
			root := &rm.ItemTree{ArchetypeNodeID: containerArchetype("ITEM_TREE"), Name: rm.DVText{Value: "tree"}, Items: tc.items}
			r := validation.Validate(root, c)
			if got := identityKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%s) issues = %q, want %q (REQ-102 § A mandatory child under an optional list)", tc.name, got, tc.want)
			}
		})
	}
}

// textElement is an ELEMENT at0001 whose value is a DV_TEXT, as
// containerElement requires.
func textElement() *rm.Element {
	return &rm.Element{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "element"}, Value: &rm.DVText{Value: "text"}}
}

// REQ-102 — the vendored family_history.v.1.2.3.json example, whose
// EVALUATION's data/items the template leaves optional by their existence
// and over a CLUSTER at0003 at occurrences 1..*, draws that CLUSTER's
// occurrences finding once the items are taken out, absent or present and
// empty, beside what the example draws as it is outside those items.
func TestValidateComposition_REQ102_FamilyHistoryMandatoryClusterUnderOptionalItems(t *testing.T) {
	t.Parallel()
	c := mustCompile(t, "family_history.v.1.2.3")
	raw, err := os.ReadFile(fixtures.CompositionJSON("family_history.v.1.2.3"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	decode := func(t *testing.T) (*rm.Composition, *rm.ItemTree) {
		t.Helper()
		var comp rm.Composition
		if err := canjson.Unmarshal(raw, &comp); err != nil {
			t.Fatalf("canjson.Unmarshal: %v", err)
		}
		if len(comp.Content) != 1 {
			t.Fatalf("content holds %d items, want the one EVALUATION", len(comp.Content))
		}
		ev, ok := comp.Content[0].(*rm.Evaluation)
		if !ok {
			t.Fatalf("content[0] is %T, want *rm.Evaluation", comp.Content[0])
		}
		tree, ok := ev.Data.(*rm.ItemTree)
		if !ok {
			t.Fatalf("EVALUATION.data is %T, want *rm.ItemTree", ev.Data)
		}
		return &comp, tree
	}
	const items = "/content[openEHR-EHR-EVALUATION.family_history.v2]/data/items"
	whole, _ := decode(t)
	var outside []validation.Issue
	for _, iss := range validation.ValidateComposition(whole, c).Issues {
		if iss.Path != items && !strings.HasPrefix(iss.Path, items+"[") {
			outside = append(outside, iss)
		}
	}
	base := identityKeys(outside)
	const cluster = "cardinality " + items + "[at0003] template occurrences"
	for _, tc := range []struct {
		name  string
		items []rm.Item
	}{
		{name: "absent items"},
		{name: "present empty items", items: []rm.Item{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			comp, tree := decode(t)
			tree.Items = tc.items
			want := append(slices.Clone(base), cluster)
			slices.Sort(want)
			if got := identityKeys(validation.ValidateComposition(comp, c).Issues); !slices.Equal(got, want) {
				t.Errorf("ValidateComposition(family_history with %s) issues = %q, want %q", tc.name, got, want)
			}
		})
	}
}

// contentOPT returns a COMPOSITION template, of the archetype
// participationsComposition carries, whose content the template writes as
// the attribute xsiType names, with existence 0..1, over one EVALUATION
// archetype root with occurrences 1..1.
func contentOPT(xsiType string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>content</value></template_id><concept>content</concept>
<definition><rm_type_name>COMPOSITION</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="` + xsiType + `"><rm_attribute_name>content</rm_attribute_name>
<existence>` + fiInterval("0", "1") + `</existence>
<children xsi:type="C_ARCHETYPE_ROOT"><rm_type_name>EVALUATION</rm_type_name>
<occurrences>` + fiInterval("1", "1") + `</occurrences><node_id>at0000</node_id>
<archetype_id><value>openEHR-EHR-EVALUATION.content.v1</value></archetype_id></children>
</attributes>
<archetype_id><value>openEHR-EHR-COMPOSITION.participations.v1</value></archetype_id></definition>
</template>`
}

// REQ-102 — only a list the template declares multi-valued has its
// children counted while it is absent: a COMPOSITION with no content draws
// the occurrences finding of the 1..1 EVALUATION a C_MULTIPLE_ATTRIBUTE
// content names, and nothing where the template writes the content as a
// C_SINGLE_ATTRIBUTE, whose children are alternatives the existence 0..1
// leaves out (REQ-107 Visit rule: the child of a single-valued attribute
// never makes it required).
func TestValidateComposition_REQ102_AbsentContentCountsOnlyAListsChildren(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		xsiType string
		want    []string
	}{
		{xsiType: "C_MULTIPLE_ATTRIBUTE", want: []string{"cardinality /content[openEHR-EHR-EVALUATION.content.v1] template occurrences"}},
		{xsiType: "C_SINGLE_ATTRIBUTE"},
	} {
		t.Run(tc.xsiType, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, contentOPT(tc.xsiType))
			r := validation.ValidateComposition(participationsComposition(), c)
			if got := identityKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateComposition(no content, %s) issues = %q, want %q", tc.xsiType, got, tc.want)
			}
		})
	}
}

// REQ-102, REQ-171 § Structural access — a list of a primitive type, such
// as DV_MULTIMEDIA data, an Array of Octet, is counted and never read item
// by item, so no item ever binds to a child the template names under it:
// its children are not counted while it is present, and so not while it is
// absent either.
func TestValidate_REQ102_AbsentPrimitiveListCountsNoChild(t *testing.T) {
	t.Parallel()
	opt := `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>multimedia</value></template_id><concept>multimedia</concept>
<definition><rm_type_name>ELEMENT</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>
<existence>` + fiInterval("1", "1") + `</existence>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_MULTIMEDIA</rm_type_name><node_id></node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>data</rm_attribute_name>
<existence>` + fiInterval("0", "1") + `</existence>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>OCTET</rm_type_name>
<occurrences>` + fiInterval("1", "1") + `</occurrences><node_id>at0001</node_id></children>
</attributes></children>
</attributes>
<archetype_id><value>openEHR-EHR-ELEMENT.multimedia.v1</value></archetype_id></definition>
</template>`
	c := mustCompileInline(t, opt)
	media := func(data []byte) *rm.Element {
		return &rm.Element{
			ArchetypeNodeID: "openEHR-EHR-ELEMENT.multimedia.v1",
			Name:            rm.DVText{Value: "element"},
			Value: &rm.DVMultimedia{
				MediaType: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "IANA_media-types"}, CodeString: "image/png"},
				URI:       &rm.DVURI{Value: "https://example.org/image.png"},
				Data:      data,
			},
		}
	}
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{name: "absent data"},
		{name: "present data", data: []byte{1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := identityKeys(validation.Validate(media(tc.data), c).Issues); len(got) != 0 {
				t.Errorf("Validate(DV_MULTIMEDIA with %s) issues = %q, want none", tc.name, got)
			}
		})
	}
}
