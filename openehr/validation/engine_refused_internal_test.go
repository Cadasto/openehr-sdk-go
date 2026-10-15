package validation

// engine_refused_internal_test.go: REQ-171 § One walk (Admission) — an item
// bound to a child the template does not allow is refused content: the walk
// records its path with the other refusals, so a finding of the parent's
// rules at or below it is withdrawn as for any refused content.

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// forbiddenChildInternalOPT is a CLUSTER whose items allow an ELEMENT at0001
// and declare an ELEMENT at0002 that occurs 0..0.
const forbiddenChildInternalOPT = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
<template_id><value>forbidden-child</value></template_id><concept>forbidden-child</concept>
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<definition><rm_type_name>CLUSTER</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>
<existence><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>1</upper></existence>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ELEMENT</rm_type_name>
<occurrences><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>1</upper></occurrences>
<node_id>at0001</node_id></children>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ELEMENT</rm_type_name>
<occurrences><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>0</upper></occurrences>
<node_id>at0002</node_id></children>
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>
</attributes>
<archetype_id><value>openEHR-EHR-CLUSTER.forbidden_child.v1</value></archetype_id></definition>
</template>`

// REQ-171 § One walk (Admission) — the walk records the path of an item bound
// to a child the template does not allow among the paths of refused content,
// and not the path of an item bound to an allowed child, so the withdrawal of
// a parent rule's findings covers the refused item as it covers an item that
// binds to no child.
func TestEngine_REQ171_RecordsAnItemBoundToAForbiddenChildAsRefused(t *testing.T) {
	t.Parallel()
	opt, err := template.ParseOPT(strings.NewReader(forbiddenChildInternalOPT))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	element := func(id string) *rm.Element {
		return &rm.Element{ArchetypeNodeID: id, Name: rm.DVText{Value: id}, Value: &rm.DVText{Value: "text"}}
	}
	root := &rm.Cluster{
		ArchetypeNodeID: "openEHR-EHR-CLUSTER.forbidden_child.v1",
		Name:            &rm.DVText{Value: "cluster"},
		Items:           []rm.Item{element("at0001"), element("at0002")},
	}
	_, e := engineWalk(root, c, Limits{})
	if e == nil {
		t.Fatal("engineWalk(CLUSTER with an allowed and a forbidden item) returned no engine: a guard refused the call")
	}
	if want := []string{"/items[at0002]"}; !slices.Equal(e.refused, want) {
		t.Errorf("engineWalk(CLUSTER with an allowed and a forbidden item) refused %q, want %q: the forbidden item alone", e.refused, want)
	}
}
