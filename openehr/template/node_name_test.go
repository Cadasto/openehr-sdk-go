package template_test

// REQ-116 (Phase 1) / REQ-100 — parse and expose the template-level node
// name: the fixed C_STRING an OPT pins on a node's name attribute. Synthetic
// table cases pin the shape contract (fixed single-entry list only, never the
// archetype concept term); the vendored Corona_Anamnese oracle pins the real
// reused-archetype payload the reference derives ids and name predicates from.

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// nameOPT wraps one definition-level attribute fragment in a minimal OPT.
func nameOPT(fragment string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>name-test</value></template_id>
  <concept>name-test-concept</concept>
  <definition xsi:type="C_COMPLEX_OBJECT">
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    %s
  </definition>
</template>`, fragment)
}

// nameAttr renders a name attribute whose child constrains value with the
// given C_STRING body, in the C_PRIMITIVE_OBJECT-wrapped shape real OPTs use.
func nameAttr(childRMType, cstringBody string) string {
	return fmt.Sprintf(`<attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>name</rm_attribute_name>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>%s</rm_type_name>
        <node_id/>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>value</rm_attribute_name>
          <children xsi:type="C_PRIMITIVE_OBJECT">
            <rm_type_name>STRING</rm_type_name>
            <node_id/>
            <item xsi:type="C_STRING">%s</item>
          </children>
        </attributes>
      </children>
    </attributes>`, childRMType, cstringBody)
}

func TestNodeName_Synthetic(t *testing.T) {
	tests := []struct {
		name     string
		fragment string
		want     string
	}{
		{
			name:     "pinned single-entry list (corona shape)",
			fragment: nameAttr("DV_TEXT", "<list>Husten</list>"),
			want:     "Husten",
		},
		{
			name:     "pinned via DV_CODED_TEXT alternative",
			fragment: nameAttr("DV_CODED_TEXT", "<list>Symptome</list>"),
			want:     "Symptome",
		},
		{
			name: "direct C_STRING child (unwrapped wire variant)",
			fragment: `<attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>name</rm_attribute_name>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_TEXT</rm_type_name>
        <node_id/>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>value</rm_attribute_name>
          <children xsi:type="C_STRING">
            <rm_type_name>STRING</rm_type_name>
            <node_id/>
            <list>Kontakt</list>
          </children>
        </attributes>
      </children>
    </attributes>`,
			want: "Kontakt",
		},
		{
			// Two admissible values pin nothing — the name is a choice,
			// not a fixed template-level name.
			name:     "multi-entry list is not a fixed name",
			fragment: nameAttr("DV_TEXT", "<list>A</list><list>B</list>"),
			want:     "",
		},
		{
			// Fixedness is decided on the RAW wire list: buildString drops
			// blank entries, so the built constraint here has one entry —
			// but the OPT declared a choice, and a choice is not a name.
			name:     "multi-entry list with a blank entry is still not fixed",
			fragment: nameAttr("DV_TEXT", "<list>A</list><list> </list>"),
			want:     "",
		},
		{
			// The name attribute may carry alternatives; the first child
			// that pins a fixed value wins.
			name: "first pinning alternative wins",
			fragment: `<attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>name</rm_attribute_name>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_TEXT</rm_type_name>
        <node_id/>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>value</rm_attribute_name>
          <children xsi:type="C_PRIMITIVE_OBJECT">
            <rm_type_name>STRING</rm_type_name>
            <node_id/>
            <item xsi:type="C_STRING"><list>First</list></item>
          </children>
        </attributes>
      </children>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
        <node_id/>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>value</rm_attribute_name>
          <children xsi:type="C_PRIMITIVE_OBJECT">
            <rm_type_name>STRING</rm_type_name>
            <node_id/>
            <item xsi:type="C_STRING"><list>Second</list></item>
          </children>
        </attributes>
      </children>
    </attributes>`,
			want: "First",
		},
		{
			// An alternative that pins nothing falls through to one that does.
			name: "unpinned first alternative falls through",
			fragment: `<attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>name</rm_attribute_name>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
        <node_id/>
      </children>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_TEXT</rm_type_name>
        <node_id/>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>value</rm_attribute_name>
          <children xsi:type="C_PRIMITIVE_OBJECT">
            <rm_type_name>STRING</rm_type_name>
            <node_id/>
            <item xsi:type="C_STRING"><list>Second</list></item>
          </children>
        </attributes>
      </children>
    </attributes>`,
			want: "Second",
		},
		{
			name:     "pattern-only constraint is not a fixed name",
			fragment: nameAttr("DV_TEXT", "<pattern>.*</pattern>"),
			want:     "",
		},
		{
			// No name attribute at all: absent means "", never the
			// template concept ("name-test-concept") nor an archetype
			// concept term (REQ-116).
			name:     "absent name attribute",
			fragment: "",
			want:     "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opt, err := template.ParseOPT(strings.NewReader(nameOPT(tc.fragment)))
			if err != nil {
				t.Fatalf("ParseOPT: %v", err)
			}
			co, ok := opt.Root().(*template.ComplexObject)
			if !ok {
				t.Fatalf("root is %T, want *template.ComplexObject", opt.Root())
			}
			if got := co.NodeName(); got != tc.want {
				t.Errorf("NodeName() = %q, want %q", got, tc.want)
			}
		})
	}
}

// An archetype root with a concept term but no name attribute reports "" —
// the concept term ("Concept Term" here) is exactly what REQ-116 forbids as a
// fallback, because every occurrence of a reused archetype shares it.
func TestNodeName_ArchetypeRootNeverFallsBackToConceptTerm(t *testing.T) {
	const fragment = `<archetype_id><value>openEHR-EHR-COMPOSITION.report.v1</value></archetype_id>
    <term_definitions code="at0000">
      <items id="text">Concept Term</items>
      <items id="description">The archetype concept.</items>
    </term_definitions>`
	opt, err := template.ParseOPT(strings.NewReader(nameOPT(fragment)))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	root, ok := opt.Root().(*template.ArchetypeRoot)
	if !ok {
		t.Fatalf("root is %T, want *template.ArchetypeRoot", opt.Root())
	}
	if term, ok := root.Term("at0000"); !ok || term.Items["text"] != "Concept Term" {
		t.Fatalf("fixture broken: concept term not parsed (%v, %v)", term, ok)
	}
	if got := root.NodeName(); got != "" {
		t.Errorf("NodeName() = %q, want \"\" — concept term must never be substituted", got)
	}
}

// findAttribute returns the named attribute or fails the test.
func findAttribute(t *testing.T, attrs []*template.Attribute, name string) *template.Attribute {
	t.Helper()
	for _, a := range attrs {
		if a.Name() == name {
			return a
		}
	}
	t.Fatalf("attribute %q not found", name)
	return nil
}

// rootsWithArchetypeID filters an attribute's children to the archetype
// roots carrying the given archetype id, in document order.
func rootsWithArchetypeID(attr *template.Attribute, archetypeID string) []*template.ArchetypeRoot {
	var out []*template.ArchetypeRoot
	for _, ch := range attr.Children() {
		if ar, ok := ch.(*template.ArchetypeRoot); ok && ar.ArchetypeID() == archetypeID {
			out = append(out, ar)
		}
	}
	return out
}

func TestNodeName_CoronaOracle(t *testing.T) {
	opt, err := template.ParseFile(fixtures.WebTemplateOpt("Corona_Anamnese"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	root, ok := opt.Root().(*template.ArchetypeRoot)
	if !ok {
		t.Fatalf("root is %T, want *template.ArchetypeRoot", opt.Root())
	}

	// The four SECTION.adhoc.v1 siblings reuse one archetype; their pinned
	// names are the only thing distinguishing them (the reference derives
	// ids symptome/kontakt/risikogebiet/allgemeine_angaben from these).
	content := findAttribute(t, root.Attributes(), "content")
	sections := rootsWithArchetypeID(content, "openEHR-EHR-SECTION.adhoc.v1")
	// Fatal, not Errorf: sections[0] is dereferenced below, and an explicit
	// count makes a fixture regression readable at a glance.
	if len(sections) != 4 {
		t.Fatalf("got %d SECTION.adhoc.v1 siblings, want 4", len(sections))
	}
	var sectionNames []string
	for _, s := range sections {
		sectionNames = append(sectionNames, s.NodeName())
	}
	wantSections := []string{"Symptome", "Kontakt", "Risikogebiet", "Allgemeine Angaben"}
	if !slices.Equal(sectionNames, wantSections) {
		t.Errorf("SECTION.adhoc.v1 names = %q, want %q", sectionNames, wantSections)
	}

	// Inside Symptome, eight OBSERVATION.symptom_sign_screening.v0 siblings
	// reuse one archetype under items — each pins its own name.
	items := findAttribute(t, sections[0].Attributes(), "items")
	observations := rootsWithArchetypeID(items, "openEHR-EHR-OBSERVATION.symptom_sign_screening.v0")
	if len(observations) != 8 {
		t.Fatalf("got %d screening OBSERVATION siblings under Symptome, want 8", len(observations))
	}
	var obsNames []string
	for _, o := range observations {
		obsNames = append(obsNames, o.NodeName())
	}
	wantObs := []string{
		"Husten", "Schnupfen", "Heiserkeit", "Fieber oder erhöhte Körpertemperatur",
		"Gestörter Geruchssinn", "Gestörter Geschmackssinn", "Durchfall", "Weitere Symptome",
	}
	if !slices.Equal(obsNames, wantObs) {
		t.Errorf("Symptome observation names = %q, want %q", obsNames, wantObs)
	}
}
