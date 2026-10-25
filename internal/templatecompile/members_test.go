package templatecompile_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// closedInterval is the body of an OPT interval from lower to upper, both
// bounded and included.
func closedInterval(lower, upper int) string {
	return `<lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>` + strconv.Itoa(lower) + `</lower><upper>` + strconv.Itoa(upper) + `</upper>`
}

// openAboveInterval is the body of an OPT interval from lower, bounded and
// included, with no upper bound.
func openAboveInterval(lower int) string {
	return `<lower_included>true</lower_included><lower_unbounded>false</lower_unbounded>` +
		`<upper_unbounded>true</upper_unbounded><lower>` + strconv.Itoa(lower) + `</lower>`
}

// unboundedLowerInterval is the body of an OPT interval that writes a lower
// bound of 1 but flags the lower side unbounded, so it has no lower bound.
const unboundedLowerInterval = `<lower_unbounded>true</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>1</lower>`

// membersOPT is an ITEM_TREE template whose items have the given cardinality
// interval body ("" for none) and one ELEMENT child per entry of
// occurrences, each with that occurrences interval body ("" for none).
func membersOPT(cardinality string, occurrences ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>members</value></template_id><concept>members</concept>
<definition><rm_type_name>ITEM_TREE</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>`)
	if cardinality != "" {
		b.WriteString(`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
			cardinality + `</interval></cardinality>`)
	}
	for i, occ := range occurrences {
		b.WriteString(`<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ELEMENT</rm_type_name>`)
		if occ != "" {
			b.WriteString(`<occurrences>` + occ + `</occurrences>`)
		}
		b.WriteString(`<node_id>at000` + strconv.Itoa(i+1) + `</node_id></children>`)
	}
	b.WriteString(`</attributes></definition>
</template>`)
	return b.String()
}

// TestREQ102_REQ107_AsksForMember checks the rule the template validator
// (REQ-102) and the example generator (REQ-107) share on a list: a lower
// bound counts when it is bounded and at least 1, and the template asks
// the list for a member when its cardinality, or the occurrences of a
// child it allows, has such a lower bound. The list's own cardinality is
// also read with LowerAtLeastOne, so its rows cover a nil, a 0, a 1 and an
// unbounded lower bound.
func TestREQ102_REQ107_AsksForMember(t *testing.T) {
	if templatecompile.LowerAtLeastOne(nil) {
		t.Error("LowerAtLeastOne(nil) = true, want false")
	}
	cases := []struct {
		name        string
		cardinality string
		occurrences []string
		// wantLower is LowerAtLeastOne of the cardinality, wantAsks
		// AsksForMember of the attribute.
		wantLower, wantAsks bool
	}{
		{name: "no cardinality, no child"},
		{name: "cardinality 0..*", cardinality: openAboveInterval(0)},
		{name: "cardinality 1..*", cardinality: openAboveInterval(1), wantLower: true, wantAsks: true},
		{name: "cardinality 2..5", cardinality: closedInterval(2, 5), wantLower: true, wantAsks: true},
		{name: "cardinality with an unbounded lower side", cardinality: unboundedLowerInterval},
		{name: "an optional child", cardinality: openAboveInterval(0), occurrences: []string{closedInterval(0, 1)}},
		{
			name: "a required child at cardinality 0..*", cardinality: openAboveInterval(0),
			occurrences: []string{closedInterval(1, 1)}, wantAsks: true,
		},
		{name: "a required child and no cardinality", occurrences: []string{closedInterval(1, 1)}, wantAsks: true},
		{name: "a child with no occurrences", cardinality: openAboveInterval(0), occurrences: []string{""}},
		{
			name: "a child whose occurrences have an unbounded lower side", cardinality: openAboveInterval(0),
			occurrences: []string{unboundedLowerInterval},
		},
		{
			name: "a required child after an optional one", cardinality: openAboveInterval(0),
			occurrences: []string{closedInterval(0, 1), closedInterval(1, 1)}, wantAsks: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opt, err := template.ParseOPT(strings.NewReader(membersOPT(tc.cardinality, tc.occurrences...)))
			if err != nil {
				t.Fatalf("ParseOPT: %v", err)
			}
			c, err := templatecompile.Compile(opt)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			items := c.Root().Attribute("items")
			if items == nil {
				t.Fatal("compiled root has no items attribute")
			}
			if got := len(templatecompile.AllowedChildNodes(items)); got != len(tc.occurrences) {
				t.Fatalf("items allows %d children, want %d", got, len(tc.occurrences))
			}
			if got := templatecompile.LowerAtLeastOne(items.ChildMultiplicity()); got != tc.wantLower {
				t.Errorf("LowerAtLeastOne(items cardinality) = %t, want %t", got, tc.wantLower)
			}
			if got := templatecompile.AsksForMember(items); got != tc.wantAsks {
				t.Errorf("AsksForMember(items) = %t, want %t", got, tc.wantAsks)
			}
		})
	}
}

// occurringChild is a C_COMPLEX_OBJECT child of rmType at nodeID with the
// occurrences interval body occurrences ("" for none).
func occurringChild(rmType, nodeID, occurrences string) string {
	if occurrences != "" {
		occurrences = `<occurrences>` + occurrences + `</occurrences>`
	}
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>` + rmType + `</rm_type_name>` +
		occurrences + `<node_id>` + nodeID + `</node_id></children>`
}

// attributeShapeOPT is a template whose root, of rmType, carries the
// attribute name, written as the attribute xsiType names, with the
// existence interval body existence ("" for none), over children.
func attributeShapeOPT(rmType, xsiType, name, existence string, children ...string) string {
	if existence != "" {
		existence = `<existence>` + existence + `</existence>`
	}
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>members</value></template_id><concept>members</concept>
<definition><rm_type_name>` + rmType + `</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="` + xsiType + `"><rm_attribute_name>` + name + `</rm_attribute_name>` + existence +
		strings.Join(children, "") + `</attributes></definition>
</template>`
}

// TestREQ102_REQ107_MandatoryListChildren checks the one reading the
// template validator (REQ-102 A mandatory child under an optional list) and
// the example generator (REQ-107 Visit rule) share of the children that ask
// a list holding no item for a member: each child the template allows,
// other than a slot, whose occurrences lower bound is bounded and at least
// 1, in template order. The attribute's existence is not read. It yields
// none for an attribute the template writes as single-valued, for a list
// the template prohibits, and for a list whose items the RM types as
// primitive values, such as a DV_MULTIMEDIA's data.
func TestREQ102_REQ107_MandatoryListChildren(t *testing.T) {
	const (
		list   = "C_MULTIPLE_ATTRIBUTE"
		single = "C_SINGLE_ATTRIBUTE"
	)
	element := func(nodeID, occurrences string) string { return occurringChild("ELEMENT", nodeID, occurrences) }
	cases := []struct {
		name string
		opt  string
		// attr is the root attribute the case reads.
		attr string
		// want is the node id of each child yielded, in order.
		want []string
	}{
		{name: "a 1..1 child", attr: "items", opt: attributeShapeOPT("ITEM_TREE", list, "items", "", element("at0001", closedInterval(1, 1))), want: []string{"at0001"}},
		{name: "a 1..* child", attr: "items", opt: attributeShapeOPT("ITEM_TREE", list, "items", "", element("at0001", openAboveInterval(1))), want: []string{"at0001"}},
		{
			name: "existence 0..1 over a 1..1 child", attr: "items",
			opt:  attributeShapeOPT("ITEM_TREE", list, "items", closedInterval(0, 1), element("at0001", closedInterval(1, 1))),
			want: []string{"at0001"},
		},
		{
			name: "existence 1..1 over a 1..1 child", attr: "items",
			opt:  attributeShapeOPT("ITEM_TREE", list, "items", closedInterval(1, 1), element("at0001", closedInterval(1, 1))),
			want: []string{"at0001"},
		},
		{name: "an optional child", attr: "items", opt: attributeShapeOPT("ITEM_TREE", list, "items", "", element("at0001", closedInterval(0, 1)))},
		{name: "a child with no occurrences", attr: "items", opt: attributeShapeOPT("ITEM_TREE", list, "items", "", element("at0001", ""))},
		{
			name: "a child whose occurrences have an unbounded lower side", attr: "items",
			opt: attributeShapeOPT("ITEM_TREE", list, "items", "", element("at0001", unboundedLowerInterval)),
		},
		{name: "a child the template does not allow", attr: "items", opt: attributeShapeOPT("ITEM_TREE", list, "items", "", element("at0001", closedInterval(0, 0)))},
		{
			name: "mandatory children around an optional one", attr: "items",
			opt: attributeShapeOPT("ITEM_TREE", list, "items", "",
				element("at0001", closedInterval(1, 1)), element("at0002", closedInterval(0, 1)), element("at0003", closedInterval(2, 5))),
			want: []string{"at0001", "at0003"},
		},
		{
			name: "a single-valued attribute over a 1..1 child", attr: "items",
			opt: attributeShapeOPT("ITEM_TREE", single, "items", "", element("at0001", closedInterval(1, 1))),
		},
		{
			name: "a list the template prohibits, over a 1..1 child", attr: "items",
			opt: attributeShapeOPT("ITEM_TREE", list, "items", closedInterval(0, 0), element("at0001", closedInterval(1, 1))),
		},
		{
			name: "a list of primitive values, over a 1..1 child", attr: "data",
			opt: attributeShapeOPT("DV_MULTIMEDIA", list, "data", closedInterval(0, 1), occurringChild("OCTET", "at0001", closedInterval(1, 1))),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opt, err := template.ParseOPT(strings.NewReader(tc.opt))
			if err != nil {
				t.Fatalf("ParseOPT: %v", err)
			}
			c, err := templatecompile.Compile(opt)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			a := c.Root().Attribute(tc.attr)
			if a == nil {
				t.Fatalf("compiled root has no %s attribute", tc.attr)
			}
			got := []string{}
			for child := range templatecompile.MandatoryListChildren(a) {
				got = append(got, child.NodeID())
			}
			if want := append([]string{}, tc.want...); !slices.Equal(got, want) {
				t.Errorf("MandatoryListChildren(%s) yields %q, want %q", tc.attr, got, want)
			}
		})
	}
}

// TestREQ107_HasMandatoryChild checks the reader the example generator
// (REQ-107 Visit rule) uses to decide whether a child makes a multi-valued
// attribute required: a child the template allows whose occurrences lower
// bound is bounded and at least 1. The attribute's cardinality is not
// read, and a child that states no occurrences, or occurrences with an
// unbounded lower bound, counts for none, as AsksForMember reads it.
func TestREQ107_HasMandatoryChild(t *testing.T) {
	cases := []struct {
		name        string
		cardinality string
		occurrences []string
		want        bool
	}{
		{name: "no child"},
		{name: "cardinality 1..* and no child", cardinality: openAboveInterval(1)},
		{name: "cardinality 1..* over an optional child", cardinality: openAboveInterval(1), occurrences: []string{closedInterval(0, 1)}},
		{name: "a required child", cardinality: openAboveInterval(0), occurrences: []string{closedInterval(1, 1)}, want: true},
		{name: "a 2..5 child and no cardinality", occurrences: []string{closedInterval(2, 5)}, want: true},
		{name: "a child with no occurrences", cardinality: openAboveInterval(0), occurrences: []string{""}},
		{
			name: "a child whose occurrences have an unbounded lower side", cardinality: openAboveInterval(0),
			occurrences: []string{unboundedLowerInterval},
		},
		{
			name: "a required child after an optional one", cardinality: openAboveInterval(0),
			occurrences: []string{closedInterval(0, 1), closedInterval(1, 1)}, want: true,
		},
		{
			name: "a prohibited child", cardinality: openAboveInterval(0),
			occurrences: []string{closedInterval(0, 0)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opt, err := template.ParseOPT(strings.NewReader(membersOPT(tc.cardinality, tc.occurrences...)))
			if err != nil {
				t.Fatalf("ParseOPT: %v", err)
			}
			c, err := templatecompile.Compile(opt)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			items := c.Root().Attribute("items")
			if items == nil {
				t.Fatal("compiled root has no items attribute")
			}
			if got := templatecompile.HasMandatoryChild(items); got != tc.want {
				t.Errorf("HasMandatoryChild(items) = %t, want %t", got, tc.want)
			}
		})
	}
}
