package composition_test

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// occurrences returns an occurrences block from lower to upper.
func occurrences(lower, upper string) string {
	return `<occurrences><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>` + lower + `</lower><upper>` + upper + `</upper></occurrences>`
}

// namedElement returns an ELEMENT[at0002] child whose name the template
// fixes to name, with the given occurrences and a DV_TEXT value.
func namedElement(name, occ string) string {
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ELEMENT</rm_type_name>` + occ + `<node_id>at0002</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>name</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_TEXT</rm_type_name><node_id/>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>
<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>STRING</rm_type_name><node_id/><item xsi:type="C_STRING"><list>` + name + `</list></item></children>
</attributes></children></attributes>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_TEXT</rm_type_name><node_id></node_id></children>
</attributes>
</children>`
}

// namedSiblingsOPT is a COMPOSITION template whose context's other_context
// holds two ELEMENT[at0002] siblings that differ only by the name the
// template fixes on each, 'A' then 'B', with the occurrences given for
// each.
func namedSiblingsOPT(occA, occB string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>named_siblings</value></template_id><concept>named_siblings</concept>
<definition><rm_type_name>COMPOSITION</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>context</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>EVENT_CONTEXT</rm_type_name><node_id></node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>other_context</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ITEM_TREE</rm_type_name><node_id>at0001</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>
` + namedElement("A", occA) + namedElement("B", occB) + `
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>
</attributes>
</children>
</attributes>
</children>
</attributes>
<archetype_id><value>openEHR-EHR-COMPOSITION.report.v1</value></archetype_id>
<term_definitions code="at0000"><items id="text">Report</items><items id="description">Report</items></term_definitions>
<term_definitions code="at0001"><items id="text">Tree</items><items id="description">Tree</items></term_definitions>
<term_definitions code="at0002"><items id="text">Reading</items><items id="description">Reading</items></term_definitions>
</definition>
</template>`
}

// The value paths of the two named siblings of namedSiblingsOPT.
const (
	siblingAValuePath = "/context/other_context/items[at0002,'A']/value"
	siblingBValuePath = "/context/other_context/items[at0002,'B']/value"
)

// compileNamedSiblings compiles namedSiblingsOPT and checks that each
// sibling compiled at its own name-predicated path.
func compileNamedSiblings(t *testing.T, occA, occB string) *templatecompile.Compiled {
	t.Helper()
	c := compileInline(t, namedSiblingsOPT(occA, occB))
	for path, name := range map[string]string{siblingAValuePath: "A", siblingBValuePath: "B"} {
		n, err := c.NodeAt(path)
		if err != nil {
			t.Fatalf("NodeAt(%s): %v (fixture drift)", path, err)
		}
		if got := n.Parent().NodeName(); got != name {
			t.Fatalf("NodeAt(%s) sits under an ELEMENT named %q, want %q (fixture drift)", path, got, name)
		}
	}
	return c
}

// siblingValues returns, by the name each carries, the DV_TEXT value of
// each ELEMENT in comp's other_context.
func siblingValues(t *testing.T, comp *rm.Composition) map[string]string {
	t.Helper()
	if comp == nil || comp.Context == nil {
		t.Fatal("composition has no context")
	}
	tree, ok := comp.Context.OtherContext.(*rm.ItemTree)
	if !ok {
		t.Fatalf("other_context is %T, want *rm.ItemTree", comp.Context.OtherContext)
	}
	values := make(map[string]string, len(tree.Items))
	for i, it := range tree.Items {
		el, ok := it.(*rm.Element)
		if !ok || el.Name == nil {
			t.Fatalf("items[%d] is %T with no name, want a named *rm.Element", i, it)
		}
		text, ok := el.Value.(*rm.DVText)
		if !ok {
			t.Fatalf("items[%d] value is %T, want *rm.DVText", i, el.Value)
		}
		values[el.Name.GetValue()] = text.Value
	}
	return values
}

// TestREQ101_REQ116_SetWritesTheNamedSibling checks, for REQ-101 and
// REQ-116, that Set writes into the sibling its path names when two siblings share an at-code and differ
// only by the name the template fixes on each. A name predicate is how
// openEHR addresses one of several siblings that share an archetype node
// id, and Set routes the assignment through the parent attribute, so the
// value lands on the named sibling and the other keeps its own.
func TestREQ101_REQ116_SetWritesTheNamedSibling(t *testing.T) {
	t.Parallel()
	required := occurrences("1", "1")
	c := compileNamedSiblings(t, required, required)

	cases := []struct {
		path, target, other string
	}{
		{path: siblingBValuePath, target: "B", other: "A"},
		{path: siblingAValuePath, target: "A", other: "B"},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			t.Parallel()
			b, err := composition.NewBuilder(t.Context(), c,
				composition.WithTerritory("NL"),
				composition.WithComposer(testComposer()),
			)
			if err != nil {
				t.Fatalf("NewBuilder: %v", err)
			}
			skeleton, err := b.Build()
			if err != nil {
				t.Fatalf("Build of the bare skeleton: %v", err)
			}
			before := siblingValues(t, skeleton)
			if len(before) != 2 {
				t.Fatalf("skeleton holds siblings %v, want A and B", before)
			}

			const written = "written"
			if err := b.Set(tc.path, &rm.DVText{Value: written}); err != nil {
				t.Fatalf("Set(%s) = %v, want nil", tc.path, err)
			}
			comp, err := b.Build()
			if err != nil {
				t.Fatalf("Build after Set(%s) = %v, want nil", tc.path, err)
			}
			after := siblingValues(t, comp)
			if after[tc.target] != written {
				t.Errorf("after Set(%s), sibling %s holds %q, want %q", tc.path, tc.target, after[tc.target], written)
			}
			if after[tc.other] != before[tc.other] {
				t.Errorf("after Set(%s), sibling %s holds %q, want its skeleton value %q", tc.path, tc.other, after[tc.other], before[tc.other])
			}
		})
	}
}

// TestREQ101_REQ116_SetRefusesANamedSiblingTheSkeletonLacks checks that a
// value set at the path of a named sibling the skeleton does not hold is
// refused with ErrInvalidPath, not written into a sibling of another name
// that shares the at-code. The template leaves both siblings optional, and
// of optional siblings that share an at-code the skeleton holds the first
// alone, 'A'.
func TestREQ101_REQ116_SetRefusesANamedSiblingTheSkeletonLacks(t *testing.T) {
	t.Parallel()
	optional := occurrences("0", "1")
	c := compileNamedSiblings(t, optional, optional)
	b, err := composition.NewBuilder(t.Context(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	skeleton, err := b.Build()
	if err != nil {
		t.Fatalf("Build of the bare skeleton: %v", err)
	}
	before := siblingValues(t, skeleton)
	if _, ok := before["B"]; ok || len(before) != 1 {
		t.Fatalf("skeleton holds siblings %v, want A alone (fixture drift)", before)
	}

	if err := b.Set(siblingBValuePath, &rm.DVText{Value: "written"}); err != nil {
		t.Fatalf("Set(%s) = %v, want nil: the path is in the template", siblingBValuePath, err)
	}
	comp, err := b.Build()
	if !errors.Is(err, composition.ErrInvalidPath) {
		t.Errorf("Build after Set(%s) = %v, want ErrInvalidPath", siblingBValuePath, err)
	}
	if after := siblingValues(t, comp); after["A"] != before["A"] {
		t.Errorf("after Set(%s), sibling A holds %q, want its skeleton value %q", siblingBValuePath, after["A"], before["A"])
	}
}
