package templatecompile_test

import (
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
