package instance

import (
	"strings"
	"testing"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// visitRootOPT is a template whose root is a C_COMPLEX_OBJECT of rmType
// carrying one attribute, attr. rmType may be a generic instantiation,
// whose angle brackets it escapes.
func visitRootOPT(rmType, attr string) string {
	rmType = strings.NewReplacer("<", "&lt;", ">", "&gt;").Replace(rmType)
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>visits</value></template_id><concept>visits</concept>
<definition><rm_type_name>` + rmType + `</rm_type_name><node_id>at0000</node_id>` + attr + `
<archetype_id><value>openEHR-EHR-CLUSTER.visits.v1</value></archetype_id></definition>
</template>`
}

// visitSingle is a C_SINGLE_ATTRIBUTE called name, existence 1..1, over
// child.
func visitSingle(name, child string) string {
	return `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		`<existence><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>1</lower><upper>1</upper></existence>` + child + `</attributes>`
}

// visitOptionalSingle is a C_SINGLE_ATTRIBUTE called name with existence
// 0..1 and no child.
func visitOptionalSingle(name string) string {
	return `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		`<existence><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>0</lower><upper>1</upper></existence></attributes>`
}

const (
	visitDuration = `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_DURATION</rm_type_name><node_id></node_id></children>`
	visitBoolean  = `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>BOOLEAN</rm_type_name><node_id></node_id>` +
		`<item xsi:type="C_BOOLEAN"><true_valid>true</true_valid><false_valid>false</false_valid></item></children>`
	visitReal = `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>REAL</rm_type_name><node_id></node_id>` +
		`<item xsi:type="C_REAL"><list>3</list></item></children>`
)

// visitAttribute compiles opt and returns its root and the root's
// attribute name. It stops the test when the compiled root does not carry
// the attribute, so a case cannot pass because the compiler dropped it.
func visitAttribute(t *testing.T, opt, name string) (*tcimpl.CompiledNode, *tcimpl.CompiledAttribute) {
	t.Helper()
	parsed, err := fixtures.ParseOPTBytes([]byte(opt))
	if err != nil {
		t.Fatalf("ParseOPTBytes: %v", err)
	}
	c, err := templatecompile.Compile(parsed)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	attr := c.Root().Attribute(name)
	if attr == nil {
		t.Fatalf("compiled root %s has no attribute %q", c.Root().RMTypeName(), name)
	}
	return c.Root(), attr
}

// TestREQ107_VisitsSkipsNonStorableAttributes is the REQ-107 check of the
// walk's visit predicate: it never visits offset on POINT_EVENT or
// INTERVAL_EVENT, or is_integral on DV_QUANTITY or DV_PROPORTION, under
// either policy, also when the OPT spells the class as a generic
// instantiation. A storable sibling is visited as the policy says. The
// visit of DV_PROPORTION.is_integral writes nothing, so this check reads
// the predicate rather than the output.
func TestREQ107_VisitsSkipsNonStorableAttributes(t *testing.T) {
	cases := []struct {
		rmType, attr, child string
		// visited is what the predicate returns under Minimal and under
		// Example.
		minimal, example bool
	}{
		{rmType: "POINT_EVENT", attr: "offset", child: visitDuration},
		{rmType: "INTERVAL_EVENT", attr: "offset", child: visitDuration},
		{rmType: "DV_QUANTITY", attr: "is_integral", child: visitBoolean},
		{rmType: "DV_PROPORTION", attr: "is_integral", child: visitBoolean},
		{rmType: "POINT_EVENT<ITEM_TREE>", attr: "offset", child: visitDuration},
		{rmType: "INTERVAL_EVENT<ITEM_LIST>", attr: "offset", child: visitDuration},
		{rmType: "DV_PROPORTION", attr: "numerator", child: visitReal, minimal: true, example: true},
		{rmType: "POINT_EVENT", attr: "time", child: "", minimal: true, example: true},
	}
	for _, tc := range cases {
		t.Run(tc.rmType+"."+tc.attr, func(t *testing.T) {
			node, attr := visitAttribute(t, visitRootOPT(tc.rmType, visitSingle(tc.attr, tc.child)), tc.attr)
			if got := node.RMTypeName(); got != tc.rmType {
				t.Fatalf("compiled root type = %q, want %q", got, tc.rmType)
			}
			for policy, want := range map[Policy]bool{Minimal: tc.minimal, Example: tc.example} {
				g := &generator{opts: Options{Policy: policy}}
				if got := g.visits(node, attr); got != want {
					t.Errorf("visits(%s, %s) under %v = %t, want %t", tc.rmType, tc.attr, policy, got, want)
				}
			}
		})
	}
}

// visitProhibitedSingle is a C_SINGLE_ATTRIBUTE called name with existence
// 0..0, over child: the OPT prohibits it.
func visitProhibitedSingle(name, child string) string {
	return `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		`<existence><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>0</lower><upper>0</upper></existence>` + child + `</attributes>`
}

// TestREQ107_VisitsSkipsProhibitedAttributes is the REQ-107 check that the
// visit predicate never visits an attribute the OPT prohibits (existence
// 0..0), under either policy, with or without a child under it.
func TestREQ107_VisitsSkipsProhibitedAttributes(t *testing.T) {
	visitTree := `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ITEM_TREE</rm_type_name><node_id>at0001</node_id></children>`
	for _, child := range []string{"", visitTree} {
		node, attr := visitAttribute(t, visitRootOPT("OBSERVATION", visitProhibitedSingle("protocol", child)), "protocol")
		for _, policy := range []Policy{Minimal, Example} {
			g := &generator{opts: Options{Policy: policy}}
			if g.visits(node, attr) {
				t.Errorf("visits(OBSERVATION, protocol 0..0, %d children) under %v = true, want false", len(attr.Children()), policy)
			}
		}
	}
}

// TestREQ107_VisitsReadsAnOpenExistenceAsAllowed is the REQ-107 check that
// an existence whose upper bound is unbounded does not prohibit the
// attribute, although its upper value reads 0: Example visits it.
func TestREQ107_VisitsReadsAnOpenExistenceAsAllowed(t *testing.T) {
	open := `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>protocol</rm_attribute_name>` +
		`<existence><lower_included>true</lower_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded>` +
		`<lower>0</lower></existence></attributes>`
	node, attr := visitAttribute(t, visitRootOPT("OBSERVATION", open), "protocol")
	if e := attr.Existence(); e == nil || !e.UpperUnbounded() || e.Upper() != 0 {
		t.Fatalf("protocol existence = %+v, want an unbounded upper that reads 0", e)
	}
	g := &generator{opts: Options{Policy: Example}}
	if !g.visits(node, attr) {
		t.Errorf("visits(OBSERVATION, protocol 0..*) under example = false, want true")
	}
}

// TestREQ107_VisitsFollowsThePolicy is the REQ-107 check that the visit
// predicate applies the policy to a storable attribute: an optional
// attribute the OPT names with no children is visited under Example and
// not under Minimal.
func TestREQ107_VisitsFollowsThePolicy(t *testing.T) {
	node, attr := visitAttribute(t, visitRootOPT("OBSERVATION", visitOptionalSingle("protocol")), "protocol")
	for policy, want := range map[Policy]bool{Minimal: false, Example: true} {
		g := &generator{opts: Options{Policy: policy}}
		if got := g.visits(node, attr); got != want {
			t.Errorf("visits(OBSERVATION, protocol) under %v = %t, want %t", policy, got, want)
		}
	}
}
