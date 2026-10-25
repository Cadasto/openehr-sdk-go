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
// walk's visit predicate: it never visits an attribute no serialization
// carries (offset on POINT_EVENT or INTERVAL_EVENT, is_integral on
// DV_QUANTITY or DV_PROPORTION, parent_resource on RESOURCE_DESCRIPTION)
// or a locatable's uid, even a required one, under every policy, also when
// the OPT spells the class as a generic instantiation. A storable sibling
// is visited as the policy says. The visit of DV_PROPORTION.is_integral
// writes nothing, so this check reads the predicate rather than the
// output.
func TestREQ107_VisitsSkipsNonStorableAttributes(t *testing.T) {
	cases := []struct {
		rmType, attr, child string
		// visited is what the predicate returns under Large, Medium
		// and Required.
		large, medium, required bool
	}{
		{rmType: "POINT_EVENT", attr: "offset", child: visitDuration},
		{rmType: "INTERVAL_EVENT", attr: "offset", child: visitDuration},
		{rmType: "DV_QUANTITY", attr: "is_integral", child: visitBoolean},
		{rmType: "DV_PROPORTION", attr: "is_integral", child: visitBoolean},
		{rmType: "POINT_EVENT<ITEM_TREE>", attr: "offset", child: visitDuration},
		{rmType: "INTERVAL_EVENT<ITEM_LIST>", attr: "offset", child: visitDuration},
		// RM-mandatory, but typed AUTHORED_RESOURCE, which has no concrete
		// class, so no value can be built there.
		{rmType: "RESOURCE_DESCRIPTION", attr: "parent_resource", child: ""},
		// A locatable's uid: the identity rule decides it, not the walk.
		{rmType: "CLUSTER", attr: "uid", child: ""},
		{rmType: "OBSERVATION", attr: "uid", child: ""},
		{rmType: "DV_PROPORTION", attr: "numerator", child: visitReal, large: true, medium: true, required: true},
		{rmType: "POINT_EVENT", attr: "time", child: "", large: true, medium: true, required: true},
	}
	for _, tc := range cases {
		t.Run(tc.rmType+"."+tc.attr, func(t *testing.T) {
			node, attr := visitAttribute(t, visitRootOPT(tc.rmType, visitSingle(tc.attr, tc.child)), tc.attr)
			if got := node.RMTypeName(); got != tc.rmType {
				t.Fatalf("compiled root type = %q, want %q", got, tc.rmType)
			}
			for policy, want := range map[Policy]bool{Large: tc.large, Medium: tc.medium, Required: tc.required} {
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
// 0..0), under every policy, with or without a child under it.
func TestREQ107_VisitsSkipsProhibitedAttributes(t *testing.T) {
	for _, child := range []string{"", visitTree} {
		node, attr := visitAttribute(t, visitRootOPT("OBSERVATION", visitProhibitedSingle("protocol", child)), "protocol")
		for _, policy := range []Policy{Large, Medium, Required} {
			g := &generator{opts: Options{Policy: policy}}
			if g.visits(node, attr) {
				t.Errorf("visits(OBSERVATION, protocol 0..0, %d children) under %v = true, want false", len(attr.Children()), policy)
			}
		}
	}
}

// TestREQ107_VisitsSkipsAnAttributeWithOnlyProhibitedChildren is the
// REQ-107 check that the visit predicate counts an attribute all of whose
// OPT children are prohibited as prohibited: an optional one is not
// visited under any policy, and a mandatory one is, since the RM needs
// it.
func TestREQ107_VisitsSkipsAnAttributeWithOnlyProhibitedChildren(t *testing.T) {
	prohibitedTree := `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ITEM_TREE</rm_type_name>` +
		`<occurrences><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>0</lower><upper>0</upper></occurrences><node_id>at0001</node_id></children>`
	optional := `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>protocol</rm_attribute_name>` +
		`<existence><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>0</lower><upper>1</upper></existence>` + prohibitedTree + `</attributes>`
	node, attr := visitAttribute(t, visitRootOPT("OBSERVATION", optional), "protocol")
	for _, policy := range []Policy{Large, Medium, Required} {
		g := &generator{opts: Options{Policy: policy}}
		if g.visits(node, attr) {
			t.Errorf("visits(OBSERVATION, protocol with only prohibited children) under %v = true, want false", policy)
		}
	}
	node, attr = visitAttribute(t, visitRootOPT("OBSERVATION", visitSingle("data", strings.ReplaceAll(prohibitedTree, "ITEM_TREE", "HISTORY"))), "data")
	for _, policy := range []Policy{Large, Medium, Required} {
		g := &generator{opts: Options{Policy: policy}}
		if !g.visits(node, attr) {
			t.Errorf("visits(OBSERVATION, mandatory data with only prohibited children) under %v = false, want true", policy)
		}
	}
}

// visitTree is an ITEM_TREE the OPT describes, whose items hold an optional
// ELEMENT: it needs no content the OPT does not describe, and holds an item
// wherever the policy builds an optional child.
const visitTree = `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ITEM_TREE</rm_type_name><node_id>at0001</node_id>` +
	`<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>` +
	`<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ELEMENT</rm_type_name>` +
	`<occurrences><lower_included>true</lower_included><upper_included>true</upper_included>` +
	`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
	`<lower>0</lower><upper>1</upper></occurrences><node_id>at0002</node_id></children>` +
	`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
	`<lower_included>true</lower_included><lower_unbounded>false</lower_unbounded>` +
	`<upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality></attributes></children>`

// visitEmptyTree is an ITEM_TREE the OPT describes with no attributes: its
// items are optional in the RM, so it would hold no item.
const visitEmptyTree = `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ITEM_TREE</rm_type_name><node_id>at0001</node_id></children>`

// TestREQ107_VisitsReadsAnOpenExistenceAsAllowed is the REQ-107 check that
// an existence whose upper bound is unbounded does not prohibit the
// attribute, although its upper value reads 0: Large visits it. The
// protocol has an ITEM_TREE child the OPT describes, so the rule on
// undescribed members does not decide it.
func TestREQ107_VisitsReadsAnOpenExistenceAsAllowed(t *testing.T) {
	open := `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>protocol</rm_attribute_name>` +
		`<existence><lower_included>true</lower_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded>` +
		`<lower>0</lower></existence>` + visitTree + `</attributes>`
	node, attr := visitAttribute(t, visitRootOPT("OBSERVATION", open), "protocol")
	if e := attr.Existence(); e == nil || !e.UpperUnbounded() || e.Upper() != 0 {
		t.Fatalf("protocol existence = %+v, want an unbounded upper that reads 0", e)
	}
	g := &generator{opts: Options{Policy: Large}}
	if !g.visits(node, attr) {
		t.Errorf("visits(OBSERVATION, protocol 0..*) under large = false, want true")
	}
}

// TestREQ107_VisitsFollowsThePolicy is the REQ-107 check that the visit
// predicate applies the policy to a storable attribute that is not
// required. One the OPT names with no children is visited under Large and
// not under Medium or Required, unless its member, built from its BMM
// type, would be a LOCATABLE no OPT node describes: then Large does not
// visit it either. One with a child the OPT describes is visited under
// Large and Medium and not under Required, unless that child is an item
// structure that would hold no item: then no policy visits it.
func TestREQ107_VisitsFollowsThePolicy(t *testing.T) {
	cases := []struct {
		name, rmType, attr, opt string
		want                    map[Policy]bool
	}{
		{
			name: "a data value with no child", rmType: "INSTRUCTION", attr: "expiry_time",
			opt:  visitOptionalSingle("expiry_time"),
			want: map[Policy]bool{Large: true, Medium: false, Required: false},
		},
		{
			name: "a structure with no child", rmType: "OBSERVATION", attr: "protocol",
			opt:  visitOptionalSingle("protocol"),
			want: map[Policy]bool{Large: false, Medium: false, Required: false},
		},
		{
			name: "a structure with a described child", rmType: "OBSERVATION", attr: "protocol",
			opt:  strings.Replace(visitOptionalSingle("protocol"), "</attributes>", visitTree+"</attributes>", 1),
			want: map[Policy]bool{Large: true, Medium: true, Required: false},
		},
		{
			name: "a structure that would hold no item", rmType: "OBSERVATION", attr: "protocol",
			opt:  strings.Replace(visitOptionalSingle("protocol"), "</attributes>", visitEmptyTree+"</attributes>", 1),
			want: map[Policy]bool{Large: false, Medium: false, Required: false},
		},
	}
	for _, tc := range cases {
		node, attr := visitAttribute(t, visitRootOPT(tc.rmType, tc.opt), tc.attr)
		for policy, want := range tc.want {
			g := &generator{opts: Options{Policy: policy}}
			if got := g.visits(node, attr); got != want {
				t.Errorf("%s: visits(%s, %s) under %v = %t, want %t", tc.name, tc.rmType, tc.attr, policy, got, want)
			}
		}
	}
}
