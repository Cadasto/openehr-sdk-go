package validation

// issuepath_internal_test.go: REQ-102 § Issue paths, REQ-112 — the shared
// path rule on its own, and the floor fed the names a template walk would
// bind, which the floor alone never has.

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
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
// neither value nor null_flavour, so the floor reports each one on its own
// path. Their instance names are not the names a test pins.
func pinnedCluster() *rm.Cluster {
	return &rm.Cluster{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "group"}, Items: []rm.Item{
		&rm.Element{ArchetypeNodeID: "at0004", Name: rm.DVText{Value: "instance one"}},
		&rm.Element{ArchetypeNodeID: "at0004", Name: rm.DVText{Value: "instance two"}},
	}}
}

// floorPaths runs the floor over root with names and returns the path of
// each issue, sorted.
func floorPaths(root any, names pinnedNames) []string {
	rmType, _, _ := rmTypeInfo(root)
	w := newFloorWalker(names, Limits{})
	w.walk(root, rmType, "/", roleNone, 0)
	paths := make([]string, 0, len(w.issues))
	for _, iss := range w.issues {
		paths = append(paths, iss.Path)
	}
	slices.Sort(paths)
	return paths
}

// REQ-112, REQ-102 — the floor takes, per item, the name the template walk
// bound, so inside a template-driven validation it writes the name
// predicate the template walk writes; on its own it has no names and writes
// positions.
func TestRMFloor_REQ112_REQ102_TakesPinnedNames(t *testing.T) {
	tests := []struct {
		name  string
		names pinnedNames
		want  []string
	}{
		{
			name: "alone",
			want: []string{"/items[1]", "/items[2]"},
		},
		{
			name: "with names that tell the items apart",
			names: pinnedNames{
				{attrPath: "/items", index: 0}: "Systolic",
				{attrPath: "/items", index: 1}: "Diastolic",
			},
			want: []string{"/items[at0004, 'Diastolic']", "/items[at0004, 'Systolic']"},
		},
		{
			name: "with names keyed to another attribute",
			names: pinnedNames{
				{attrPath: "/other", index: 0}: "Systolic",
				{attrPath: "/other", index: 1}: "Diastolic",
			},
			want: []string{"/items[1]", "/items[2]"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := floorPaths(pinnedCluster(), tc.names); !slices.Equal(got, tc.want) {
				t.Errorf("floor paths = %q, want %q", got, tc.want)
			}
		})
	}
}

// nestedPinnedCluster is a CLUSTER holding two CLUSTERs that share at0002,
// each holding two ELEMENTs that share at0004 and carry neither value nor
// null_flavour.
func nestedPinnedCluster() *rm.Cluster {
	inner := func() *rm.Cluster {
		c := pinnedCluster()
		c.ArchetypeNodeID = "at0002"
		return c
	}
	return &rm.Cluster{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "outer"}, Items: []rm.Item{inner(), inner()}}
}

// REQ-112, REQ-102 — a pinned name is found by the whole path of its
// attribute, not by the attribute's name: names given to the items of the
// outer /items reach neither inner /items, and names given to the items of
// one inner /items reach only those.
func TestRMFloor_REQ112_REQ102_PinnedNamesKeyedByWholePath(t *testing.T) {
	const first, second = "/items[at0002, 'Erste']", "/items[at0002, 'Zweite']"
	outer := pinnedNames{
		{attrPath: "/items", index: 0}: "Erste",
		{attrPath: "/items", index: 1}: "Zweite",
	}
	tests := []struct {
		name  string
		names pinnedNames
		want  []string
	}{
		{
			name:  "names for the outer items",
			names: outer,
			want:  []string{first + "/items[1]", first + "/items[2]", second + "/items[1]", second + "/items[2]"},
		},
		{
			name: "names for the outer items and the first inner items",
			names: pinnedNames{
				{attrPath: "/items", index: 0}:         "Erste",
				{attrPath: "/items", index: 1}:         "Zweite",
				{attrPath: first + "/items", index: 0}: "Systolic",
				{attrPath: first + "/items", index: 1}: "Diastolic",
			},
			want: []string{
				first + "/items[at0004, 'Diastolic']",
				first + "/items[at0004, 'Systolic']",
				second + "/items[1]",
				second + "/items[2]",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := floorPaths(nestedPinnedCluster(), tc.names); !slices.Equal(got, tc.want) {
				t.Errorf("floor paths = %q, want %q", got, tc.want)
			}
		})
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
// RM-mandatory language is empty, a defect both walkers report.
func handoffObservation(name string) *rm.Observation {
	return &rm.Observation{
		ArchetypeNodeID: "openEHR-EHR-OBSERVATION.reused.v1",
		Name:            rm.DVText{Value: name},
		Encoding:        rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}, CodeString: "UTF-8"},
		Subject:         rm.PartySelf{},
	}
}

// REQ-102, REQ-112, REQ-116 — the names the template walk binds, recorded
// by the attribute's path and each item's index, make the floor write the
// path the template walk writes for the same node.
func TestBindings_REQ102_REQ112_HandOffGivesOnePath(t *testing.T) {
	opt, err := template.ParseOPT(strings.NewReader(handoffOPT))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	comp := &rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.reused.v1",
		Name:            rm.DVText{Value: "Reused"},
		Content:         []rm.ContentItem{handoffObservation("Zweite"), handoffObservation("Erste")},
	}
	var content *tcimpl.CompiledAttribute
	for _, a := range c.Root().Attributes() {
		if a.Name() == "content" {
			content = a
		}
	}
	if content == nil {
		t.Fatal("compiled root has no content attribute")
	}
	items, ok := rmread.ReadMultiple(comp, "COMPOSITION", "content")
	if !ok {
		t.Fatal("rmread cannot read COMPOSITION.content")
	}
	bound, _ := bindItems("COMPOSITION", "content", content.Children(), items)
	names := pinnedNames{}
	for i, it := range boundPathItems(items, bound) {
		names[itemKey{attrPath: "/content", index: i}] = it.name
	}

	w := newWalker(c)
	w.walkNode(c.Root(), comp, "/")
	floor := newFloorWalker(names, Limits{})
	floor.walk(comp, "COMPOSITION", "/", roleNone, 0)

	for _, want := range []string{
		"/content[openEHR-EHR-OBSERVATION.reused.v1, 'Zweite']/language",
		"/content[openEHR-EHR-OBSERVATION.reused.v1, 'Erste']/language",
	} {
		for name, issues := range map[string][]Issue{"template walk": w.issues, "floor": floor.issues} {
			if !slices.ContainsFunc(issues, func(i Issue) bool { return i.Code == "required" && i.Path == want }) {
				t.Errorf("%s: no required at %s; got %+v", name, want, issues)
			}
		}
	}
}
