package validation_test

// nested_list_items_test.go: the walk reads the items of a list it is inside
// while it walks the lists nested in them, each nested list read above the
// outer one, so a long nested list, which outgrows the room the walk keeps
// for the items it reads, leaves the items of the outer list as they were.

import (
	"slices"
	"strconv"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// nestedListsArchetype is the archetype of the root of nestedListsOPT.
const nestedListsArchetype = "openEHR-EHR-CLUSTER.nested_lists.v1"

// nestedListsOPT is a CLUSTER whose items hold any number of CLUSTER at0001,
// whose own items hold any number of ELEMENT at0002 with a DV_TEXT value,
// and at most one ELEMENT at0003 with a DV_TEXT value.
var nestedListsOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>nested_lists</value></template_id><concept>nested_lists</concept>
<definition><rm_type_name>CLUSTER</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>CLUSTER</rm_type_name>
<occurrences>` + unboundedFromZero + `</occurrences><node_id>at0001</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>` +
	fiElement("at0002", "0", "1000", "", "DV_TEXT") + `
<cardinality><is_ordered>true</is_ordered><is_unique>false</is_unique><interval>` + unboundedFromZero + `</interval></cardinality>
</attributes></children>` +
	fiElement("at0003", "0", "1", "", "DV_TEXT") + `
<cardinality><is_ordered>true</is_ordered><is_unique>false</is_unique><interval>` + unboundedFromZero + `</interval></cardinality>
</attributes>
<archetype_id><value>` + nestedListsArchetype + `</value></archetype_id></definition>
</template>`

// nestedLongList is the length of the long nested lists: longer than the
// room the walk starts with for the items of the lists it is inside, so
// reading one grows that room while the outer list's items are held.
const nestedLongList = 40

// nestedListsCluster returns the root of nestedListsOPT: four items, the
// first three a CLUSTER at0001 holding 4, nestedLongList and nestedLongList
// ELEMENT at0002, the last an ELEMENT at0003. The short list comes first, so
// it is read while the room the walk started with still has space above the
// outer items; the long ones grow it. The last ELEMENT of each long list,
// and the ELEMENT at0003, have no name, which the RM requires.
func nestedListsCluster() *rm.Cluster {
	elements := func(n int) []rm.Item {
		items := make([]rm.Item, n)
		for i := range items {
			items[i] = &rm.Element{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "element " + strconv.Itoa(i+1)}, Value: &rm.DVText{Value: "text"}}
		}
		return items
	}
	cluster := func(name string, n int) *rm.Cluster {
		items := elements(n)
		if n == nestedLongList {
			items[n-1].(*rm.Element).Name = nil
		}
		return &rm.Cluster{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: name}, Items: items}
	}
	return &rm.Cluster{
		ArchetypeNodeID:  nestedListsArchetype,
		ArchetypeDetails: archetypeDetails(nestedListsArchetype),
		Name:             rm.DVText{Value: "nested lists"},
		Items: []rm.Item{
			cluster("short", 4),
			cluster("long", nestedLongList),
			cluster("longer", nestedLongList),
			&rm.Element{ArchetypeNodeID: "at0003", Value: &rm.DVText{Value: "text"}},
		},
	}
}

// The items of an outer list are walked as they are, each at its own path,
// while the lists nested in them are read and walked: a nameless ELEMENT at
// the end of each long nested list, and the nameless ELEMENT that is the
// outer list's last item, are each reported at their own path, with the
// template and with the RM rules alone, and nothing else is.
func TestValidate_NestedListsKeepOuterItems(t *testing.T) {
	t.Parallel()
	c := mustCompileInline(t, nestedListsOPT)
	last := strconv.Itoa(nestedLongList)
	want := []string{
		"required /items[2]/items[" + last + "]/name rm mandatory",
		"required /items[3]/items[" + last + "]/name rm mandatory",
		"required /items[at0003]/name rm mandatory",
	}
	for _, tc := range []struct {
		name     string
		validate func() validation.Result
	}{
		{name: "template", validate: func() validation.Result { return validation.Validate(nestedListsCluster(), c) }},
		{name: "rm", validate: func() validation.Result { return validation.ValidateRM(nestedListsCluster()) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := walkOrderKeys(tc.validate().Issues); !slices.Equal(got, want) {
				t.Errorf("issues (code path layer rule), in walk order = %q, want %q", got, want)
			}
		})
	}
}
