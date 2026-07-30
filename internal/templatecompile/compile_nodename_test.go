package templatecompile_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-116 (Phase 2) / REQ-111 — the template-level node name survives
// compile on the plain-ComplexObject path. The root here carries no
// archetype id, so descend takes the *template.ComplexObject case.
func TestCompile_NodeNameCarriedComplexObject(t *testing.T) {
	const opt = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>nodename-carry</value></template_id>
  <concept>nodename-carry-concept</concept>
  <definition xsi:type="C_COMPLEX_OBJECT">
    <rm_type_name>COMPOSITION</rm_type_name>
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
            <item xsi:type="C_STRING"><list>Root Name</list></item>
          </children>
        </attributes>
      </children>
    </attributes>
  </definition>
</template>`
	parsed, err := template.ParseOPT(strings.NewReader(opt))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	c, err := templatecompile.Compile(parsed)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if got := c.Root().NodeName(); got != "Root Name" {
		t.Errorf("Root().NodeName() = %q, want %q", got, "Root Name")
	}
}

// REQ-116 (Phase 2) / REQ-111 — the names of reused archetype roots
// survive compile on the ArchetypeRoot path. byArchetypeID retains all
// four SECTION.adhoc.v1 siblings even though byPath keeps only the
// first, so each sibling's distinct name is reachable post-compile —
// the precondition for Phase 3's name predicates. Nodes without a
// pinned name stay "" (never the concept term).
func TestCompile_NodeNameCarriedCoronaOracle(t *testing.T) {
	parsed, err := template.ParseFile(fixtures.WebTemplateOpt("Corona_Anamnese"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	c, err := templatecompile.Compile(parsed)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	sections := c.AllByArchetypeID("openEHR-EHR-SECTION.adhoc.v1")
	if len(sections) != 4 {
		t.Fatalf("AllByArchetypeID(SECTION.adhoc.v1) = %d nodes, want 4", len(sections))
	}
	want := []string{"Symptome", "Kontakt", "Risikogebiet", "Allgemeine Angaben"}
	for i, s := range sections {
		if s.NodeName() != want[i] {
			t.Errorf("section[%d].NodeName() = %q, want %q", i, s.NodeName(), want[i])
		}
	}

	// The shared path resolves to the first sibling — its name comes along.
	n, err := c.NodeAt("/content[openEHR-EHR-SECTION.adhoc.v1]")
	if err != nil {
		t.Fatalf("NodeAt: %v", err)
	}
	if got := n.NodeName(); got != "Symptome" {
		t.Errorf("NodeAt(shared).NodeName() = %q, want Symptome", got)
	}

	// A node with no pinned name reports "" — the archetype concept term
	// ("Bericht" carries one, the EVENT_CONTEXT does not) is never used.
	ctx, err := c.NodeAt("/context")
	if err != nil {
		t.Fatalf("NodeAt(/context): %v", err)
	}
	if got := ctx.NodeName(); got != "" {
		t.Errorf("NodeAt(/context).NodeName() = %q, want \"\"", got)
	}

	// The COMPOSITION root is the sharper witness: it pins no name but
	// carries its at0000 concept term ("Bericht") right on the node —
	// a compile-layer fallback to the term would surface exactly here.
	root := c.Root()
	if term, ok := root.Term("at0000", ""); !ok || term.Items["text"] != "Bericht" {
		t.Fatalf(`Root().Term("at0000") = %v, %v — fixture no longer carries the Bericht term`, term, ok)
	}
	if got := root.NodeName(); got != "" {
		t.Errorf("Root().NodeName() = %q, want \"\" (concept term must not be substituted)", got)
	}
}
