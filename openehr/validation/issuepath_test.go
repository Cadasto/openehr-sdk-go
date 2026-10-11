package validation_test

// issuepath_test.go: REQ-102 § Issue paths, REQ-112 — the template-driven
// validators and the RM floor write Issue.Path by one rule. An item of a
// multi-valued attribute is written [id] when its id is unique in the
// attribute, [id, 'name'] when the name the OPT pins tells a shared id apart,
// and [n], its 1-based position, otherwise. A node the OPT requires and the
// instance lacks keeps the OPT child's own predicate. No path carries a value
// from the instance.

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// bpEventItems is the path of the systolic ITEM_LIST's items in
// validVitalSignsComposition.
const bpEventItems = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items"

// systolicItem returns a systolic ELEMENT (at0004) that the vital_signs OPT
// accepts, carrying name as its instance name. An empty name leaves the
// RM-mandatory name absent.
func systolicItem(name string) rm.Element {
	e := rm.Element{
		ArchetypeNodeID: "at0004",
		Value:           &rm.DVQuantity{Magnitude: rm.Real(120), Units: "mm[Hg]"},
	}
	if name != "" {
		e.Name = rm.DVText{Value: name}
	}
	return e
}

// vitalSignsWithEventItems returns validVitalSignsComposition with items as
// the event data's items.
func vitalSignsWithEventItems(t *testing.T, items ...rm.Element) *rm.Composition {
	t.Helper()
	comp := validVitalSignsComposition()
	obs, ok := comp.Content[0].(*rm.Observation)
	if !ok {
		t.Fatalf("fixture content[0] is %T, want *rm.Observation", comp.Content[0])
	}
	event, ok := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	if !ok {
		t.Fatalf("fixture events[0] is %T, want *rm.PointEvent[rm.ItemStructure]", obs.Data.Events[0])
	}
	list, ok := event.Data.(*rm.ItemList)
	if !ok {
		t.Fatalf("fixture event data is %T, want *rm.ItemList", event.Data)
	}
	list.Items = items
	return comp
}

// assertBothReport fails unless the template walk and the floor each report
// code at path.
func assertBothReport(t *testing.T, byTemplate, byFloor validation.Result, code, path string) {
	t.Helper()
	if !containsIssue(byTemplate.Issues, path, code) {
		t.Errorf("template validator: no %s at %s; got %q", code, path, issueKeys(byTemplate.Issues))
	}
	if !containsIssue(byFloor.Issues, path, code) {
		t.Errorf("floor: no %s at %s; got %q", code, path, issueKeys(byFloor.Issues))
	}
}

// REQ-102, REQ-112 — an item whose archetype_node_id no sibling shares is
// written by its id, an archetype root by its archetype id, and the template
// walk and the floor report the same node at the same path.
func TestIssuePath_REQ102_REQ112_UniqueIDs(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := vitalSignsWithEventItems(t, systolicItem(""))

	want := bpEventItems + "[at0004]/name"
	assertBothReport(t, validation.ValidateComposition(comp, c), validation.ValidateRM(comp), "required", want)
}

// REQ-102, REQ-112 — two items that share an archetype_node_id, bound to OPT
// nodes that pin no name, are written by their 1-based position, the same in
// both walkers. The OPT child they bind to, whose occurrences they exceed,
// keeps its own predicate.
func TestIssuePath_REQ102_REQ112_SharedIDWithoutNames(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := vitalSignsWithEventItems(t, systolicItem("Systolic"), systolicItem(""))

	byTemplate := validation.ValidateComposition(comp, c)
	byFloor := validation.ValidateRM(comp)
	assertBothReport(t, byTemplate, byFloor, "required", bpEventItems+"[2]/name")
	if want := bpEventItems + "[at0004]"; !containsIssue(byTemplate.Issues, want, "cardinality") {
		t.Errorf("template validator: no cardinality at the OPT child's own path %s; got %q", want, issueKeys(byTemplate.Issues))
	}
	for _, r := range []validation.Result{byTemplate, byFloor} {
		for _, iss := range r.Issues {
			if strings.Contains(iss.Path, "items[at0004]/") {
				t.Errorf("issue %s at %s: an item whose id a sibling shares is written by its id", iss.Code, iss.Path)
			}
		}
	}
}

// REQ-102, REQ-112 — an item with no archetype_node_id is written by its
// 1-based position, the same in both walkers. The OPT child declares no id,
// so the cardinality of the node it constrains is written at the attribute.
func TestIssuePath_REQ102_REQ112_NoIDIsPositional(t *testing.T) {
	c := mustCompileSyntheticOPT(t, participationsOPT)
	noFunction := validParticipation()
	noFunction.Function = nil
	comp := participationsComposition(validParticipation(), noFunction)

	byTemplate := validation.ValidateComposition(comp, c)
	assertBothReport(t, byTemplate, validation.ValidateRM(comp), "required", "/context/participations[2]/function")
	want := []string{"cardinality /context/participations", "required /context/participations[2]/function"}
	if got := issueKeys(byTemplate.Issues); !slices.Equal(got, want) {
		t.Errorf("template validator issues = %q, want %q", got, want)
	}
}

// reusedArchetypeOPT has a COMPOSITION whose content holds two archetype
// roots of one archetype, each required once and told apart by the name the
// template pins on it. The second name holds a quote and a backslash.
var reusedArchetypeOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <template_id><value>reused-archetype</value></template_id>
  <concept>reused-archetype</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>` +
	reusedObservationChild("Erste") +
	reusedObservationChild(`O'Brien\Notes`) + `
    </attributes>
    <archetype_id><value>openEHR-EHR-COMPOSITION.reused.v1</value></archetype_id>
  </definition>
</template>`

// reusedObservationChild renders one OBSERVATION archetype root of
// openEHR-EHR-OBSERVATION.reused.v1, occurring once, that pins name.
func reusedObservationChild(name string) string {
	return `
      <children xsi:type="C_ARCHETYPE_ROOT">
        <rm_type_name>OBSERVATION</rm_type_name>
        <occurrences>
          <lower_included>true</lower_included><upper_included>true</upper_included>
          <lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>
          <lower>1</lower><upper>1</upper>
        </occurrences>
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
                <item xsi:type="C_STRING"><list>` + name + `</list></item>
              </children>
            </attributes>
          </children>
        </attributes>
        <archetype_id><value>openEHR-EHR-OBSERVATION.reused.v1</value></archetype_id>
      </children>`
}

// reusedComposition returns a COMPOSITION of reusedArchetypeOPT whose content
// holds one reused OBSERVATION per name, each carrying that instance name.
func reusedComposition(names ...string) *rm.Composition {
	comp := validVitalSignsComposition()
	comp.ArchetypeNodeID = "openEHR-EHR-COMPOSITION.reused.v1"
	comp.Content = nil
	for _, name := range names {
		obs := validBloodPressureObservation()
		obs.ArchetypeNodeID = "openEHR-EHR-OBSERVATION.reused.v1"
		obs.Name = rm.DVText{Value: name}
		comp.Content = append(comp.Content, obs)
	}
	return comp
}

// REQ-102 — a node the OPT requires and the instance lacks is written with
// the OPT child's own predicate. Its archetype id is shared with a sibling
// child, and the name the template pins tells it apart, so the predicate
// carries that name, single-quoted, with its quote and backslash escaped.
func TestIssuePath_REQ102_MissingNodeKeepsNamedOPTPredicate(t *testing.T) {
	c := mustCompileSyntheticOPT(t, reusedArchetypeOPT)
	r := validation.ValidateComposition(reusedComposition("Erste"), c)

	want := `/content[openEHR-EHR-OBSERVATION.reused.v1, 'O\'Brien\\Notes']`
	if !containsIssue(r.Issues, want, "cardinality") {
		t.Errorf("ValidateComposition: no cardinality at %s; got %q", want, issueKeys(r.Issues))
	}
	for _, iss := range r.Issues {
		if strings.HasPrefix(iss.Path, "/content[openEHR-EHR-OBSERVATION.reused.v1, 'Erste']") {
			t.Errorf("issue %s at %s: the present node is the only item of its id and carries no name predicate", iss.Code, iss.Path)
		}
	}
}

// REQ-102, REQ-112, REQ-168 — a path never carries a value from the
// instance: sibling items that share an id and carry distinct instance names
// are written by position in both walkers, and the names appear in no path.
func TestIssuePath_REQ102_REQ112_NoInstanceNameInPath(t *testing.T) {
	const planted = "PLANTED-SIBLING"
	vital := mustCompile(t, "vital_signs")
	reused := mustCompileSyntheticOPT(t, reusedArchetypeOPT)
	bp := vitalSignsWithEventItems(t, systolicItem(planted+"-A"), systolicItem(planted+"-B"))
	twice := reusedComposition(planted+"-A", planted+"-B")

	for name, r := range map[string]validation.Result{
		"template, vital_signs":      validation.ValidateComposition(bp, vital),
		"floor, vital_signs":         validation.ValidateRM(bp),
		"template, reused archetype": validation.ValidateComposition(twice, reused),
		"floor, reused archetype":    validation.ValidateRM(twice),
	} {
		if len(r.Issues) == 0 {
			t.Fatalf("%s: no issues, so no path to inspect", name)
		}
		for _, iss := range r.Issues {
			if strings.Contains(iss.Path, planted) {
				t.Errorf("%s: issue %s at %s carries an instance name", name, iss.Code, iss.Path)
			}
		}
	}
	byTemplate := validation.ValidateComposition(twice, reused)
	if want := "/content[2]"; !slices.ContainsFunc(byTemplate.Issues, func(i validation.Issue) bool { return strings.HasPrefix(i.Path, want+"/") || i.Path == want }) {
		t.Errorf("template validator: no issue under %s, the second item of a shared id; got %q", want, issueKeys(byTemplate.Issues))
	}
}

// REQ-102 — a path with no positional predicate is a valid AQL path, a name
// predicate included; one with a positional predicate is not, since the AQL
// grammar has none.
func TestIssuePath_REQ102_AQLValidity(t *testing.T) {
	vital := mustCompile(t, "vital_signs")
	reused := mustCompileSyntheticOPT(t, reusedArchetypeOPT)
	unique := validation.ValidateComposition(vitalSignsWithEventItems(t, systolicItem("")), vital)
	named := validation.ValidateComposition(reusedComposition("Erste"), reused)
	shared := validation.ValidateComposition(vitalSignsWithEventItems(t, systolicItem("Systolic"), systolicItem("")), vital)

	tests := []struct {
		name    string
		result  validation.Result
		path    string
		wantAQL bool
	}{
		{name: "ids only", result: unique, path: bpEventItems + "[at0004]/name", wantAQL: true},
		{name: "name predicate", result: named, path: `/content[openEHR-EHR-OBSERVATION.reused.v1, 'O\'Brien\\Notes']`, wantAQL: true},
		{name: "positional predicate", result: shared, path: bpEventItems + "[2]/name", wantAQL: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !slices.ContainsFunc(tc.result.Issues, func(i validation.Issue) bool { return i.Path == tc.path }) {
				t.Fatalf("no issue at %s; got %q", tc.path, issueKeys(tc.result.Issues))
			}
			q := "SELECT c" + tc.path + " FROM COMPOSITION c"
			doc, err := parse.Parse(q)
			if gotAQL := err == nil; gotAQL != tc.wantAQL {
				t.Fatalf("parse.Parse(%q) error = %v, want AQL-valid %t", q, err, tc.wantAQL)
			}
			if !tc.wantAQL {
				return
			}
			if len(doc.Paths) != 1 || doc.Paths[0].Raw != "c"+tc.path {
				t.Errorf("parse.Parse(%q) paths = %+v, want the one path c%s", q, doc.Paths, tc.path)
			}
		})
	}
}
