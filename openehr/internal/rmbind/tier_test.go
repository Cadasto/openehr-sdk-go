package rmbind_test

import (
	"slices"
	"strings"
	"testing"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/internal/rmbind"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// elementValueChildren compiles an OPT rooted at an ELEMENT whose value
// holds one child per RM type in types, constraining nothing below it, and
// returns those children.
func elementValueChildren(t *testing.T, types ...string) []*tcimpl.CompiledNode {
	t.Helper()
	var children strings.Builder
	for _, rmType := range types {
		children.WriteString(`<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>` + rmType + `</rm_type_name><node_id></node_id></children>`)
	}
	opt, err := template.ParseOPT(strings.NewReader(`<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>tier</value></template_id>
  <concept>tier</concept>
  <definition>
    <rm_type_name>ELEMENT</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>` + children.String() + `</attributes>
    <archetype_id><value>openEHR-EHR-ELEMENT.tier.v1</value></archetype_id>
  </definition>
</template>`))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return tcimpl.ChildNodes(c.Root().Attribute("value"))
}

// REQ-102: Tier gives the nodes of the first of Bind's three tiers that
// admits the value, in template order, so its first node is the one Bind
// returns: every node naming the value's own type, else every node naming
// the short name it wraps, else every node its type conforms to. It gives
// none where no node admits the value, and it appends to what dst holds.
func TestREQ102_TierHoldsTheNodesBindChoosesAmong(t *testing.T) {
	t.Parallel()
	children := elementValueChildren(t, "DATA_VALUE", "DV_TEXT", "DV_QUANTITY", "DURATION", "DV_QUANTITY", "DV_CODED_TEXT")
	all := func(*tcimpl.CompiledNode) bool { return true }
	tests := []struct {
		name  string
		value any
		want  []int // indexes into children
	}{
		{name: "own type, twice", value: &rm.DVQuantity{Magnitude: 1, Units: "kg"}, want: []int{2, 4}},
		{name: "own type, once", value: &rm.DVCodedText{Value: "x"}, want: []int{5}},
		{name: "short name", value: &rm.DVDuration{Value: "PT1H"}, want: []int{3}},
		{name: "conformance", value: &rm.DVCount{Magnitude: 1}, want: []int{0}},
		{name: "no node", value: "text"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var want []*tcimpl.CompiledNode
			for _, i := range tc.want {
				want = append(want, children[i])
			}
			got := rmbind.Tier(nil, children, tc.value, all)
			if !slices.Equal(got, want) {
				t.Errorf("Tier(%T) = %d nodes, want %d: the nodes of the first tier that admits it", tc.value, len(got), len(want))
			}
			if bound := rmbind.Bind(children, tc.value, all); len(got) > 0 && got[0] != bound || len(got) == 0 && bound != nil {
				t.Errorf("Tier(%T) does not start with the node Bind returns", tc.value)
			}
			held := children[1:2]
			if again := rmbind.Tier(slices.Clip(held), children, tc.value, all); !slices.Equal(again, append(slices.Clip(held), want...)) {
				t.Errorf("Tier(dst holding one node, %T) = %d nodes, want that node and the tier after it", tc.value, len(again))
			}
		})
	}
}
