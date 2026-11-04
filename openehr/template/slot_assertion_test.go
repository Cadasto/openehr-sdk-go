package template_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-103: the OPT parser turns a slot's include assertions into parsed slot
// rules.
func TestParseFile_VitalSigns_SlotAssertionsParsed(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	var deviceSlot *template.Slot
	walkSlots(opt.Root(), func(s *template.Slot) {
		for _, inc := range s.ParsedIncludes() {
			if inc.MatchesArchetypeID("openEHR-EHR-CLUSTER.device.v1") {
				deviceSlot = s
			}
		}
	})
	if deviceSlot == nil {
		t.Fatal("expected a CLUSTER.device slot with parsed includes in vital_signs.opt")
	}
	if !deviceSlot.AllowsArchetypeID("openEHR-EHR-CLUSTER.device.v1") {
		t.Error("CLUSTER.device slot should allow matching archetype id")
	}
	if deviceSlot.AllowsArchetypeID("openEHR-EHR-OBSERVATION.heart_rate.v1") {
		t.Error("CLUSTER.device slot should reject unrelated archetype id")
	}
}

func TestSlot_AllowsRMTypePrefix(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	var checked bool
	walkSlots(opt.Root(), func(s *template.Slot) {
		if len(s.ParsedIncludes()) > 0 {
			return // prefix fallback only when no parsed includes
		}
		checked = true
		id := "openEHR-EHR-" + s.RMTypeName() + ".example.v1"
		if !s.AllowsRMType(id) {
			t.Errorf("slot %s: AllowsRMType prefix fallback failed for %q", s.NodeID(), id)
		}
	})
	if !checked {
		t.Skip("fixture has no slots without parsed includes")
	}
}

// Demonstration.v1.opt is the only fixture carrying <excludes>: an
// ELEMENT slot pairs a closed includes list with the editor's
// auto-generated catch-all `.*` exclude. The catch-all must be parsed
// but must not reject the slot's own includes.
func TestParseFile_Demonstration_ExcludesParsed(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("Demonstration.v1"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	const included = "openEHR-EHR-ELEMENT.ctg_codes.v1"
	var slot *template.Slot
	walkSlots(opt.Root(), func(s *template.Slot) {
		if slot != nil || len(s.ParsedExcludes()) == 0 {
			return
		}
		for _, inc := range s.ParsedIncludes() {
			if inc.MatchesArchetypeID(included) {
				slot = s
				return
			}
		}
	})
	if slot == nil {
		t.Fatal("expected a slot with parsed includes and excludes in Demonstration.v1.opt")
	}
	// The included id fits despite the catch-all exclude.
	if !slot.AllowsArchetypeID(included) {
		t.Errorf("AllowsArchetypeID(%q) = false, want true (catch-all exclude must not reject includes)", included)
	}
	// A non-included id is still rejected.
	if slot.AllowsArchetypeID("openEHR-EHR-ELEMENT.something_else.v1") {
		t.Error("non-included archetype id should be rejected")
	}
}

func TestParseFile_TextSlotAssertionWithRegexQuantifier(t *testing.T) {
	path := filepath.Join(t.TempDir(), "slot-quantifier.opt")
	if err := os.WriteFile(path, []byte(textSlotAssertionWithQuantifierOPT), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	opt, err := template.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	var slot *template.Slot
	walkSlots(opt.Root(), func(s *template.Slot) {
		if slot == nil && len(s.Includes()) > 0 {
			slot = s
		}
	})
	if slot == nil {
		t.Fatal("expected synthetic OPT to contain a slot")
	}
	if got := len(slot.ParsedIncludes()); got != 1 {
		t.Fatalf("len(ParsedIncludes()) = %d, want 1", got)
	}
	if !slot.AllowsArchetypeID("openEHR-EHR-CLUSTER.device.v12") {
		t.Error("expected quantifier pattern to allow v12")
	}
	if slot.AllowsArchetypeID("openEHR-EHR-CLUSTER.device.v123") {
		t.Error("expected quantifier pattern to reject v123")
	}
}

// REQ-103, REQ-108: the parser reads the pattern of a slot include's
// `archetype_id … matches {…}` text, the two words in any ASCII case, where
// runes before, between or inside them take more bytes lowercased than as
// written, as Ⱥ (U+023A, two bytes) does lowercased to ⱥ (U+2C65, three
// bytes): it does not panic, and it reads the pattern from its own place in
// the text.
func TestParseOPT_SlotTextAfterLengtheningRunes(t *testing.T) {
	lengthening := strings.Repeat("Ⱥ", 30)
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "lengthening runes before the match",
			text: lengthening + " archetype_id/value matches {/.*/}",
			want: []string{".*"},
		},
		{
			name: "lengthening runes between the two words",
			text: "archetype_id/value " + lengthening + ` matches {/openEHR-EHR-CLUSTER\.device\.v1/}`,
			want: []string{`openEHR-EHR-CLUSTER\.device\.v1`},
		},
		{
			name: "the two words in capitals, lengthening runes in the pattern too",
			text: lengthening + ` ARCHETYPE_ID/value MATCHES {/openEHR-EHR-CLUSTER\.Ⱥ\.v1/}`,
			want: []string{`openEHR-EHR-CLUSTER\.Ⱥ\.v1`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opt, err := template.ParseOPT(strings.NewReader(stringExpressionSlotOPTWith(tc.text)))
			if err != nil {
				t.Fatalf("ParseOPT: %v", err)
			}
			var slot *template.Slot
			walkSlots(opt.Root(), func(s *template.Slot) {
				if slot == nil {
					slot = s
				}
			})
			if slot == nil {
				t.Fatal("no slot in the OPT")
			}
			if got := slotPatterns(slot.ParsedIncludes()); !slices.Equal(got, tc.want) {
				t.Errorf("ParsedIncludes() patterns of a slot whose include reads %q = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

// stringExpressionSlotOPTWith is an OPT whose one slot has one include, the
// <string_expression> text.
func stringExpressionSlotOPTWith(text string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
  <template_id><value>slot_text_scan</value></template_id>
  <concept>slot_text_scan</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>
      <children xsi:type="ARCHETYPE_SLOT">
        <rm_type_name>SECTION</rm_type_name>
        <node_id>at0001</node_id>
        <includes><string_expression>` + text + `</string_expression></includes>
      </children>
    </attributes>
  </definition>
</template>`
}

func TestParseFile_XMLSlotAssertionRequiresSupportedShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "slot-unsupported-xml.opt")
	if err := os.WriteFile(path, []byte(xmlSlotAssertionUnsupportedShapeOPT), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	opt, err := template.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	var slot *template.Slot
	walkSlots(opt.Root(), func(s *template.Slot) {
		if slot == nil && len(s.Includes()) > 0 {
			slot = s
		}
	})
	if slot == nil {
		t.Fatal("expected synthetic OPT to contain a slot")
	}
	if got := len(slot.ParsedIncludes()); got != 0 {
		t.Fatalf("len(ParsedIncludes()) = %d, want 0 for unsupported XML expression shape", got)
	}
	if !slot.SlotRules().IncludesDroppedUnparsed() {
		t.Fatal("expected raw include to be retained as dropped/unparsed")
	}
}

func TestSlot_SlotRulesReturnsDefensiveCopies(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	var slot *template.Slot
	walkSlots(opt.Root(), func(s *template.Slot) {
		if slot == nil && len(s.ParsedIncludes()) > 0 {
			slot = s
		}
	})
	if slot == nil {
		t.Fatal("expected a slot with parsed includes")
	}
	const id = "openEHR-EHR-CLUSTER.device.v1"
	if !slot.AllowsArchetypeID(id) {
		t.Fatalf("fixture slot should allow %q before mutation attempt", id)
	}

	rules := slot.SlotRules()
	rules.Includes[0] = constraints.SlotAssertion{}
	if !slot.AllowsArchetypeID(id) {
		t.Fatal("mutating returned SlotRules.Includes changed the slot's internal parsed rules")
	}
}

// REQ-111: the term-definition accessor returns a copy the caller cannot use
// to change the parsed template.
func TestArchetypeRoot_TermsReturnsDeepCopy(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	root, ok := opt.Root().(*template.ArchetypeRoot)
	if !ok {
		t.Fatalf("Root() = %T, want *ArchetypeRoot", opt.Root())
	}
	terms := root.Terms()
	if terms["at0000"].Items["text"] == "" {
		t.Fatal("fixture root term at0000.text is empty")
	}

	term := terms["at0000"]
	term.Items["text"] = "mutated"
	terms["at0000"] = term

	fresh, ok := root.Term("at0000")
	if !ok {
		t.Fatal("root.Term(at0000) missing after mutation attempt")
	}
	if fresh.Items["text"] == "mutated" {
		t.Fatal("mutating Terms()[at0000].Items changed parsed template internals")
	}
}

func walkSlots(n template.Node, fn func(*template.Slot)) {
	switch v := n.(type) {
	case template.ObjectNode:
		for _, a := range v.Attributes() {
			for _, c := range a.Children() {
				if s, ok := c.(*template.Slot); ok {
					fn(s)
				}
				walkSlots(c, fn)
			}
		}
	}
}

const textSlotAssertionWithQuantifierOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns="http://schemas.openehr.org/v1">
  <language>
    <terminology_id><value>ISO_639-1</value></terminology_id>
    <code_string>en</code_string>
  </language>
  <template_id><value>slot_quantifier</value></template_id>
  <concept>slot_quantifier</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>
      <children xsi:type="ARCHETYPE_SLOT">
        <rm_type_name>CLUSTER</rm_type_name>
        <node_id>at9000</node_id>
        <includes>archetype_id matches {openEHR-EHR-CLUSTER\.device\.v[0-9]{1,2}}</includes>
      </children>
    </attributes>
  </definition>
</template>`

const xmlSlotAssertionUnsupportedShapeOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns="http://schemas.openehr.org/v1">
  <language>
    <terminology_id><value>ISO_639-1</value></terminology_id>
    <code_string>en</code_string>
  </language>
  <template_id><value>slot_unsupported_xml</value></template_id>
  <concept>slot_unsupported_xml</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>
      <children xsi:type="ARCHETYPE_SLOT">
        <rm_type_name>CLUSTER</rm_type_name>
        <node_id>at9000</node_id>
        <includes>
          <expression xsi:type="EXPR_BINARY_OPERATOR">
            <type>Boolean</type>
            <operator>2007</operator>
            <precedence_overridden>false</precedence_overridden>
            <left_operand xsi:type="EXPR_LEAF">
              <type>String</type>
              <item xsi:type="xsd:string">other_attribute/value</item>
              <reference_type>attribute</reference_type>
            </left_operand>
            <right_operand xsi:type="EXPR_LEAF">
              <type>C_STRING</type>
              <item xsi:type="C_STRING">
                <pattern>openEHR-EHR-CLUSTER\.device\.v1</pattern>
              </item>
              <reference_type>constraint</reference_type>
            </right_operand>
          </expression>
        </includes>
      </children>
    </attributes>
  </definition>
</template>`
