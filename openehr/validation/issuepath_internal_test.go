package validation

// issuepath_internal_test.go: REQ-102 § Issue paths, REQ-112 — the shared
// path rule on its own, and the floor fed the names a template walk would
// bind, which the floor alone never has.

import (
	"fmt"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// REQ-102 — the predicate of each item of one multi-valued attribute.
func TestItemPaths_REQ102_Rule(t *testing.T) {
	tests := []struct {
		name  string
		items []pathItem
		want  []string
	}{
		{
			name:  "unique ids",
			items: []pathItem{{id: "at0004"}, {id: "at0005"}},
			want:  []string{"/items[at0004]", "/items[at0005]"},
		},
		{
			name:  "a unique id keeps no name, even one the OPT pins",
			items: []pathItem{{id: "at0004", name: "Systolic"}},
			want:  []string{"/items[at0004]"},
		},
		{
			name:  "shared id, distinct pinned names",
			items: []pathItem{{id: "at0004", name: "Systolic"}, {id: "at0004", name: "Diastolic"}},
			want:  []string{"/items[at0004, 'Systolic']", "/items[at0004, 'Diastolic']"},
		},
		{
			name:  "shared id, no pinned names",
			items: []pathItem{{id: "at0004"}, {id: "at0004"}},
			want:  []string{"/items[1]", "/items[2]"},
		},
		{
			// An item whose name a child pins binds to that child, so the
			// sibling bound where no name is pinned does not carry it.
			name:  "shared id, one pinned name and one sibling that pins none",
			items: []pathItem{{id: "at0004", name: "Systolic"}, {id: "at0004"}},
			want:  []string{"/items[at0004, 'Systolic']", "/items[2]"},
		},
		{
			name:  "shared id, two items bound to one named node",
			items: []pathItem{{id: "at0004", name: "Systolic"}, {id: "at0004", name: "Systolic"}, {id: "at0004", name: "Diastolic"}},
			want:  []string{"/items[1]", "/items[2]", "/items[at0004, 'Diastolic']"},
		},
		{
			name:  "no id is positional and 1-based",
			items: []pathItem{{}, {id: "at0004"}, {}},
			want:  []string{"/items[1]", "/items[at0004]", "/items[3]"},
		},
		{
			name:  "a name without an id is never written",
			items: []pathItem{{name: "Systolic"}},
			want:  []string{"/items[1]"},
		},
		{
			name: "name escaping",
			items: []pathItem{
				{id: "openEHR-EHR-SECTION.adhoc.v1", name: `O'Brien`},
				{id: "openEHR-EHR-SECTION.adhoc.v1", name: `C:\notes`},
				{id: "openEHR-EHR-SECTION.adhoc.v1", name: `ends in \`},
				{id: "openEHR-EHR-SECTION.adhoc.v1", name: `\'`},
			},
			want: []string{
				`/items[openEHR-EHR-SECTION.adhoc.v1, 'O\'Brien']`,
				`/items[openEHR-EHR-SECTION.adhoc.v1, 'C:\\notes']`,
				`/items[openEHR-EHR-SECTION.adhoc.v1, 'ends in \\']`,
				`/items[openEHR-EHR-SECTION.adhoc.v1, '\\\'']`,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := itemPaths("/items", tc.items); !slices.Equal(got, tc.want) {
				t.Errorf("itemPaths(%+v) = %q, want %q", tc.items, got, tc.want)
			}
		})
	}
}

// REQ-102 — a node the instance lacks keeps the OPT child's own predicate:
// no item exists to number, so a shared id with no telling name keeps the id
// and a child with no id has no predicate.
func TestPredicates_REQ102_OPTChildrenAreNeverNumbered(t *testing.T) {
	children := []pathItem{
		{id: "at0004", name: "Systolic"},
		{id: "at0004", name: "Diastolic"},
		{id: "at0005"},
		{id: "at0005"},
		{},
	}
	want := []string{"[at0004, 'Systolic']", "[at0004, 'Diastolic']", "[at0005]", "[at0005]", ""}
	if got := predicates(children, false); !slices.Equal(got, want) {
		t.Errorf("predicates(%+v, false) = %q, want %q", children, got, want)
	}
}

// pinnedCluster is a CLUSTER whose two ELEMENTs share at0004 and each carry
// neither value nor null_flavour, so the RM rules report each one on its own
// path. Their instance names are not names a template pins.
func pinnedCluster() *rm.Cluster {
	return &rm.Cluster{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "group"}, Items: []rm.Item{
		&rm.Element{ArchetypeNodeID: "at0004", Name: rm.DVText{Value: "instance one"}},
		&rm.Element{ArchetypeNodeID: "at0004", Name: rm.DVText{Value: "instance two"}},
	}}
}

// REQ-102 Issue paths, REQ-112 — the RM rules alone bind no template node,
// so they never write a name predicate: two items that share at0004 are
// written by their positions, whatever names the instance gives them.
func TestValidateRM_REQ112_REQ102_WritesNoNamePredicate(t *testing.T) {
	var paths []string
	for _, iss := range ValidateRM(pinnedCluster()).Issues {
		paths = append(paths, iss.Path)
	}
	if want := []string{"/items[1]", "/items[2]"}; !slices.Equal(paths, want) {
		t.Errorf("ValidateRM(two ELEMENTs sharing at0004) paths = %q, want %q", paths, want)
	}
}

// handoffOPT has a COMPOSITION whose content holds two archetype roots of
// openEHR-EHR-OBSERVATION.reused.v1, told apart by the names Erste and
// Zweite the template pins on them.
var handoffOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <template_id><value>handoff</value></template_id>
  <concept>handoff</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>` + handoffChild("Erste") + handoffChild("Zweite") + `
    </attributes>
    <archetype_id><value>openEHR-EHR-COMPOSITION.reused.v1</value></archetype_id>
  </definition>
</template>`

func handoffChild(name string) string {
	return fmt.Sprintf(`
      <children xsi:type="C_ARCHETYPE_ROOT">
        <rm_type_name>OBSERVATION</rm_type_name>
        <node_id>at0000</node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>name</rm_attribute_name>
          <children xsi:type="C_COMPLEX_OBJECT">
            <rm_type_name>DV_TEXT</rm_type_name>
            <node_id/>
            <attributes xsi:type="C_SINGLE_ATTRIBUTE">
              <rm_attribute_name>value</rm_attribute_name>
              <children xsi:type="C_PRIMITIVE_OBJECT">
                <rm_type_name>STRING</rm_type_name>
                <node_id/>
                <item xsi:type="C_STRING"><list>%s</list></item>
              </children>
            </attributes>
          </children>
        </attributes>
        <archetype_id><value>openEHR-EHR-OBSERVATION.reused.v1</value></archetype_id>
      </children>`, name)
}

// handoffObservation returns a reused OBSERVATION named name whose
// RM-mandatory language is empty, a defect the RM rules report.
func handoffObservation(name string) *rm.Observation {
	return &rm.Observation{
		ArchetypeNodeID: "openEHR-EHR-OBSERVATION.reused.v1",
		Name:            rm.DVText{Value: name},
		Encoding:        rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}, CodeString: "UTF-8"},
		Subject:         rm.PartySelf{},
	}
}
