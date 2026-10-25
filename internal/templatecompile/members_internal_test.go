package templatecompile

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// TestREQ102_REQ107_MandatoryListChildrenReadsTheAttribute checks the
// guards of MandatoryListChildren that a parsed template cannot reach or
// pin alone, on one parsed ELEMENT child at occurrences 1..1. The parser
// keeps no occurrences for a slot, so the same node is read as a plain
// child and as a slot: only the plain child is yielded (REQ-107 Visit rule:
// an allowed child that is not a slot). A list whose items the RM types as
// octets is a list of primitive values and yields none, while a Hash,
// whose RM type is primitive too, is no list of items and yields the
// child, as the template validator reads a Hash entry by entry. A list the
// template prohibits, and an attribute it writes as single-valued, yield
// none.
func TestREQ102_REQ107_MandatoryListChildrenReadsTheAttribute(t *testing.T) {
	const opt = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>members</value></template_id><concept>members</concept>
<definition><rm_type_name>ITEM_TREE</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ELEMENT</rm_type_name>
<occurrences><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></occurrences>
<node_id>at0001</node_id></children>
</attributes></definition>
</template>`
	parsed, err := template.ParseOPT(strings.NewReader(opt))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	c, err := Compile(parsed)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	items := c.Root().Attribute("items")
	if items.rmTypeName != "ITEM" {
		t.Fatalf("items RM type = %q, want ITEM", items.rmTypeName)
	}
	element := items.allowed[0]
	for _, tc := range []struct {
		name string
		// edit changes a copy of the compiled items and its child.
		edit func(a *CompiledAttribute, child *CompiledNode)
		want bool
	}{
		{name: "a 1..1 child", edit: func(*CompiledAttribute, *CompiledNode) {}, want: true},
		{name: "a 1..1 slot", edit: func(_ *CompiledAttribute, child *CompiledNode) { child.slot = &slotInfo{} }},
		{name: "a list of octets", edit: func(a *CompiledAttribute, _ *CompiledNode) { a.rmTypeName = "Octet" }},
		{name: "a list of strings", edit: func(a *CompiledAttribute, _ *CompiledNode) { a.rmTypeName = "String" }},
		{name: "a Hash", edit: func(a *CompiledAttribute, _ *CompiledNode) { a.rmTypeName = "Hash" }, want: true},
		{name: "a list the template prohibits", edit: func(a *CompiledAttribute, _ *CompiledNode) { a.prohibited = true }},
		{name: "a single-valued attribute", edit: func(a *CompiledAttribute, _ *CompiledNode) { a.cardinality = template.Single }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			child := &CompiledNode{rmTypeName: element.rmTypeName, nodeID: element.nodeID, occurrences: element.Occurrences()}
			a := &CompiledAttribute{name: "items", cardinality: items.cardinality, rmTypeName: items.rmTypeName}
			tc.edit(a, child)
			a.allowed = []*CompiledNode{child}
			got := false
			for c := range MandatoryListChildren(a) {
				if c != child {
					t.Errorf("MandatoryListChildren over %s yields %v, want the child", tc.name, c)
				}
				got = true
			}
			if got != tc.want {
				t.Errorf("MandatoryListChildren over %s yields a child = %t, want %t", tc.name, got, tc.want)
			}
		})
	}
}
