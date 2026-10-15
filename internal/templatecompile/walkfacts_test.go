package templatecompile_test

// walkfacts_test.go: REQ-171 § Surface — what the compile resolves for a
// validation walk equals what the walk would read at run time. Over every
// vendored OPT, at every compiled node and attribute, each precomputed
// answer is compared with the answer rminfo.Default, or the attribute's own
// children, give when asked directly: the allowed children and the
// prohibition, and whether each class the hierarchy knows, and some it does
// not, conforms to the node's RM type. Every slice the compiled template
// hands out has no room to append into.

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// runtimeAllowed returns the children of a whose occurrences state no upper
// bound of 0, and whether the template prohibits a: its existence's upper
// bound is a bounded 0, or it has a child and allows none.
func runtimeAllowed(a *templatecompile.CompiledAttribute) (allowed []*templatecompile.CompiledNode, prohibited bool) {
	children := a.Children()
	for _, c := range children {
		if occ := c.Occurrences(); occ == nil || occ.UpperUnbounded() || occ.Upper() != 0 {
			allowed = append(allowed, c)
		}
	}
	e := a.Existence()
	return allowed, e != nil && !e.UpperUnbounded() && e.Upper() == 0 || len(children) > 0 && len(allowed) == 0
}

// conformanceProbes returns every class rminfo.Default knows and names it
// does not: a Go type name, a parameterised class, a short name and the
// empty name.
func conformanceProbes() []string {
	return append(rminfo.Default.KnownRMTypes(), "string", "DV_INTERVAL<DV_QUANTITY>", "INTEGER", "")
}

// clipped reports whether s leaves no room to append into: an append to it
// allocates a new array rather than write into the compiled template's.
func clipped[T any](s []T) bool { return cap(s) == len(s) }

// checkNode compares every precomputed answer of n and of its attributes with
// the runtime one, checks that each slice it hands out is clipped, then does
// the same below each attribute's children, and returns the number of nodes
// and of attributes it checked.
func checkNode(t *testing.T, opt string, n *templatecompile.CompiledNode) (nodes, attributes int) {
	t.Helper()
	nodes = 1
	h, _ := rminfo.Default.(rminfo.Hierarchy)
	for _, probe := range conformanceProbes() {
		gotConforms, gotKnown := templatecompile.ConformsTo(n, probe)
		wantConforms, wantKnown := h.ConformsTo(probe, n.RMTypeName())
		if gotConforms != wantConforms || gotKnown != wantKnown {
			t.Errorf("%s %s: ConformsTo(%q to %q) = (%t, %t), want (%t, %t)", opt, n.AQLPath(), probe, n.RMTypeName(), gotConforms, gotKnown, wantConforms, wantKnown)
		}
	}
	if !slices.Equal(templatecompile.NodeAttributes(n), n.Attributes()) {
		t.Errorf("%s %s: NodeAttributes differ from Attributes", opt, n.AQLPath())
	}
	if !clipped(templatecompile.NodeAttributes(n)) {
		t.Errorf("%s %s: NodeAttributes has room to append into", opt, n.AQLPath())
	}
	for _, a := range n.Attributes() {
		attributes++
		if !slices.Equal(templatecompile.ChildNodes(a), a.Children()) {
			t.Errorf("%s %s/%s: ChildNodes differ from Children", opt, n.AQLPath(), a.Name())
		}
		allowed, prohibited := runtimeAllowed(a)
		if got := templatecompile.AllowedChildNodes(a); !slices.Equal(got, allowed) {
			t.Errorf("%s %s/%s: AllowedChildNodes = %d nodes, want %d", opt, n.AQLPath(), a.Name(), len(got), len(allowed))
		}
		if !clipped(templatecompile.ChildNodes(a)) || !clipped(templatecompile.AllowedChildNodes(a)) {
			t.Errorf("%s %s/%s: ChildNodes or AllowedChildNodes has room to append into", opt, n.AQLPath(), a.Name())
		}
		if got := templatecompile.Prohibited(a); got != prohibited {
			t.Errorf("%s %s/%s: Prohibited = %t, want %t", opt, n.AQLPath(), a.Name(), got, prohibited)
		}
		for _, c := range a.Children() {
			n, a := checkNode(t, opt, c)
			nodes += n
			attributes += a
		}
	}
	return nodes, attributes
}

// REQ-171 § Surface — over every vendored OPT, what the compile resolved for
// a validation walk at each node and each attribute equals what rminfo, the
// attribute's children and the class hierarchy answer at run time.
func TestREQ171_WalkFactsEqualTheRuntimeAnswers(t *testing.T) {
	t.Parallel()
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatalf("ListAllOPTs: %v", err)
	}
	var templates, nodes, attributes int
	for _, ref := range refs {
		parsed, err := template.ParseFile(ref.Path)
		if err != nil {
			continue // a template that does not parse is the parser's tests' concern
		}
		c, err := templatecompile.Compile(parsed)
		if err != nil {
			continue
		}
		templates++
		n, a := checkNode(t, ref.Name, c.Root())
		nodes += n
		attributes += a
	}
	if templates < 50 {
		t.Fatalf("compiled %d vendored OPTs, want at least 50: the listing lost templates", templates)
	}
	t.Logf("compared %d templates, %d nodes, %d attributes", templates, nodes, attributes)
}

// interval is an AOM interval body from lower to upper.
func interval(lower, upper string) string {
	return `<lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>` + lower + `</lower><upper>` + upper + `</upper>`
}

// prohibitingOPT is an ELEMENT template whose null_flavour the existence
// 0..0 prohibits, whose value allows a DV_QUANTITY but not a DV_TEXT, whose
// occurrences are 0..0, and whose name allows none of its one child, a
// DV_TEXT whose occurrences are 0..0.
var prohibitingOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>prohibiting</value></template_id><concept>prohibiting</concept>
<definition><rm_type_name>ELEMENT</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>null_flavour</rm_attribute_name><existence>` + interval("0", "0") + `</existence></attributes>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_TEXT</rm_type_name><occurrences>` + interval("0", "0") + `</occurrences><node_id></node_id></children>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_QUANTITY</rm_type_name><node_id></node_id></children>
</attributes>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>name</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_TEXT</rm_type_name><occurrences>` + interval("0", "0") + `</occurrences><node_id></node_id></children>
</attributes>
<archetype_id><value>openEHR-EHR-ELEMENT.prohibiting.v1</value></archetype_id></definition>
</template>`

// REQ-171 § Surface; REQ-102 Structural — prohibited attributes — the
// compile resolves which children of an attribute the template allows and
// whether it prohibits the attribute: an existence of 0..0 prohibits
// null_flavour, which has no child; value allows its DV_QUANTITY and not its
// DV_TEXT, whose occurrences are 0..0, and is not prohibited; name allows
// none of its one child and is prohibited; an implicit attribute, with no
// child and no existence, is open. Each answer equals the runtime one.
func TestREQ171_AllowedChildrenAndProhibitionResolvedAtCompile(t *testing.T) {
	t.Parallel()
	parsed, err := template.ParseOPT(strings.NewReader(prohibitingOPT))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	c, err := templatecompile.Compile(parsed)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	tests := []struct {
		attr       string
		allowed    []string // the RM types of the allowed children
		prohibited bool
	}{
		{attr: "null_flavour", prohibited: true},
		{attr: "value", allowed: []string{"DV_QUANTITY"}},
		{attr: "name", prohibited: true},
		{attr: "archetype_node_id"},
	}
	for _, tc := range tests {
		a := c.Root().Attribute(tc.attr)
		if a == nil {
			t.Fatalf("the compiled ELEMENT has no %s", tc.attr)
		}
		var allowed []string
		for _, child := range templatecompile.AllowedChildNodes(a) {
			allowed = append(allowed, child.RMTypeName())
		}
		if !slices.Equal(allowed, tc.allowed) || templatecompile.Prohibited(a) != tc.prohibited {
			t.Errorf("%s: AllowedChildNodes = %q, Prohibited = %t; want %q, %t", tc.attr, allowed, templatecompile.Prohibited(a), tc.allowed, tc.prohibited)
		}
	}
	checkNode(t, "prohibiting", c.Root())
}
