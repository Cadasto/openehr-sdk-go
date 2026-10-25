package templatecompile

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// TestREQ107_HasMandatoryChildPassesOverASlot checks that HasMandatoryChild
// reads no slot (REQ-107 Visit rule: an allowed child that is not a slot).
// The parser keeps no occurrences for a slot, so the case gives one the
// occurrences 1..1 of a parsed ELEMENT: the same node counts as a plain
// child and does not as a slot.
func TestREQ107_HasMandatoryChildPassesOverASlot(t *testing.T) {
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
	element := c.Root().Attribute("items").allowed[0]
	for _, tc := range []struct {
		name string
		slot *slotInfo
		want bool
	}{
		{name: "a 1..1 child", want: true},
		{name: "a 1..1 slot", slot: &slotInfo{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			child := &CompiledNode{rmTypeName: "ELEMENT", nodeID: "at0001", occurrences: element.Occurrences(), slot: tc.slot}
			a := &CompiledAttribute{name: "items", allowed: []*CompiledNode{child}}
			if got := HasMandatoryChild(a); got != tc.want {
				t.Errorf("HasMandatoryChild over %s = %t, want %t", tc.name, got, tc.want)
			}
		})
	}
}
