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

// unnamedElement returns an ELEMENT[at0002] child on which the template
// fixes no name, with the given occurrences and a DV_TEXT value.
func unnamedElement(occ string) string {
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ELEMENT</rm_type_name>` + occ + `<node_id>at0002</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_TEXT</rm_type_name><node_id></node_id></children>
</attributes>
</children>`
}

// namedSiblingsOPT is a COMPOSITION template whose context's other_context
// holds two ELEMENT[at0002] siblings that differ only by the name the
// template fixes on each, 'A' then 'B', with the occurrences given for
// each, in an items list whose cardinality upper bound is upper, or
// unbounded when upper is empty.
func namedSiblingsOPT(occA, occB, upper string) string {
	return siblingsOPT(namedElement("A", occA)+namedElement("B", occB), upper)
}

// siblingsOPT is a COMPOSITION template whose context's other_context
// holds the ELEMENT children siblings in an items list whose cardinality
// upper bound is upper, or unbounded when upper is empty.
func siblingsOPT(siblings, upper string) string {
	bound := `<upper_unbounded>true</upper_unbounded>`
	if upper != "" {
		bound = `<upper_included>true</upper_included><upper_unbounded>false</upper_unbounded><upper>` + upper + `</upper>`
	}
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
` + siblings + `
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded>` + bound + `<lower>0</lower></interval></cardinality>
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

// compileNamedSiblings compiles namedSiblingsOPT with an unbounded items
// list and checks that each sibling compiled at its own name-predicated
// path.
func compileNamedSiblings(t *testing.T, occA, occB string) *templatecompile.Compiled {
	t.Helper()
	return compileNamedSiblingsWithin(t, occA, occB, "")
}

// compileNamedSiblingsWithin is compileNamedSiblings with the items list's
// cardinality upper bound upper.
func compileNamedSiblingsWithin(t *testing.T, occA, occB, upper string) *templatecompile.Compiled {
	t.Helper()
	c := compileInline(t, namedSiblingsOPT(occA, occB, upper))
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
// that shares the at-code. The template leaves both siblings optional in a
// list that holds one item at most, and the Large skeleton gives that room
// to the first, 'A', in OPT order (REQ-107 What a visit writes).
func TestREQ101_REQ116_SetRefusesANamedSiblingTheSkeletonLacks(t *testing.T) {
	t.Parallel()
	optional := occurrences("0", "1")
	c := compileNamedSiblingsWithin(t, optional, optional, "1")
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

// TestREQ101_REQ107_SetWritesAnOptionalNamedSibling checks that the Large
// skeleton NewBuilder seeds from holds a member of each optional sibling
// that shares an at-code and differs only by the name the template fixes
// on it (REQ-107 What a visit writes), so Set writes into either sibling
// its path names, the second included, and the other keeps its own value.
func TestREQ101_REQ107_SetWritesAnOptionalNamedSibling(t *testing.T) {
	t.Parallel()
	optional := occurrences("0", "1")
	c := compileNamedSiblings(t, optional, optional)
	for _, tc := range []struct {
		path, target, other string
	}{
		{path: siblingBValuePath, target: "B", other: "A"},
		{path: siblingAValuePath, target: "A", other: "B"},
	} {
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
				t.Fatalf("skeleton holds siblings %v, want A and B: Large writes every optional named sibling", before)
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

// The path of the ELEMENT[at0002] uses of an OPT built by siblingsOPT from
// unnamedElement children, and the path of their value.
const (
	unnamedUsePath      = "/context/other_context/items[at0002]"
	unnamedUseValuePath = unnamedUsePath + "/value"
)

// elementValues returns, in list order, the DV_TEXT value of each ELEMENT
// in comp's other_context.
func elementValues(t *testing.T, comp *rm.Composition) []string {
	t.Helper()
	if comp == nil || comp.Context == nil {
		t.Fatal("composition has no context")
	}
	tree, ok := comp.Context.OtherContext.(*rm.ItemTree)
	if !ok {
		t.Fatalf("other_context is %T, want *rm.ItemTree", comp.Context.OtherContext)
	}
	values := make([]string, 0, len(tree.Items))
	for i, it := range tree.Items {
		el, ok := it.(*rm.Element)
		if !ok {
			t.Fatalf("items[%d] is %T, want *rm.Element", i, it)
		}
		text, ok := el.Value.(*rm.DVText)
		if !ok {
			t.Fatalf("items[%d] value is %T, want *rm.DVText", i, el.Value)
		}
		values = append(values, text.Value)
	}
	return values
}

// TestREQ101_REQ107_SetReachesTheFirstOfUnnamedUses checks the REQ-101
// Skeleton policy reading of two optional uses of one node id on which the
// template fixes no name. The Large skeleton NewBuilder seeds from holds a
// member of each (REQ-107 What a visit writes), in OPT order, but the later
// use has the compiled path of the first, so Set at that path reaches the
// first use's member alone, and the member built for the later use keeps
// the value the skeleton gave it.
func TestREQ101_REQ107_SetReachesTheFirstOfUnnamedUses(t *testing.T) {
	t.Parallel()
	optional := occurrences("0", "1")
	c := compileInline(t, siblingsOPT(unnamedElement(optional)+unnamedElement(optional), ""))
	first, err := c.NodeAt(unnamedUsePath)
	if err != nil {
		t.Fatalf("NodeAt(%s): %v (fixture drift)", unnamedUsePath, err)
	}
	uses := first.Parent().Attribute("items").Children()
	if len(uses) != 2 || uses[0] != first {
		t.Fatalf("NodeAt(%s) is not the first of two uses under items (fixture drift)", unnamedUsePath)
	}
	if got := uses[1].AQLPath(); got != unnamedUsePath {
		t.Fatalf("the later use compiled at %s, want the first use's path %s (fixture drift)", got, unnamedUsePath)
	}

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
	before := elementValues(t, skeleton)
	if len(before) != 2 {
		t.Fatalf("skeleton holds ELEMENT values %q, want one member of each use: Large writes every optional use", before)
	}

	const written = "written"
	if err := b.Set(unnamedUseValuePath, &rm.DVText{Value: written}); err != nil {
		t.Fatalf("Set(%s) = %v, want nil", unnamedUseValuePath, err)
	}
	comp, err := b.Build()
	if err != nil {
		t.Fatalf("Build after Set(%s) = %v, want nil", unnamedUseValuePath, err)
	}
	after := elementValues(t, comp)
	if len(after) != 2 {
		t.Fatalf("after Set(%s), other_context holds ELEMENT values %q, want the skeleton's two members", unnamedUseValuePath, after)
	}
	if after[0] != written {
		t.Errorf("after Set(%s), the first use's member holds %q, want %q", unnamedUseValuePath, after[0], written)
	}
	if after[1] != before[1] {
		t.Errorf("after Set(%s), the later use's member holds %q, want its skeleton value %q", unnamedUseValuePath, after[1], before[1])
	}
}
