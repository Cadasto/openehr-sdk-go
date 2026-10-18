package template_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func slotPatterns(as []constraints.SlotAssertion) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.Pattern()
	}
	return out
}

func hasRepeat(ps []string) bool {
	seen := make(map[string]bool, len(ps))
	for _, p := range ps {
		if seen[p] {
			return true
		}
		seen[p] = true
	}
	return false
}

// REQ-104: an Archetype Designer slot blob states its pattern twice, as
// <string_expression> text and as an <expression> tree. Every slot of
// clinical_notes.v0 has such blobs, and each pattern is listed once.
func TestParseFile_ClinicalNotesSlotPatternsListedOnce(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOpt("clinical_notes.v0"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	slots := 0
	walkSlots(opt.Root(), func(s *template.Slot) {
		slots++
		if inc := slotPatterns(s.ParsedIncludes()); hasRepeat(inc) {
			t.Errorf("slot %s: ParsedIncludes() patterns = %q, want each once", s.NodeID(), inc)
		}
		if exc := slotPatterns(s.ParsedExcludes()); hasRepeat(exc) {
			t.Errorf("slot %s: ParsedExcludes() patterns = %q, want each once", s.NodeID(), exc)
		}
	})
	if slots == 0 {
		t.Fatal("no slot found in clinical_notes.v0")
	}
}

var stringExpressionSlotOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
  <template_id><value>slot_string_expression</value></template_id>
  <concept>slot_string_expression</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>
      <children xsi:type="ARCHETYPE_SLOT">
        <rm_type_name>CLUSTER</rm_type_name>
        <node_id>at9000</node_id>
        <includes>
          <string_expression>archetype_id/value matches {/openEHR-EHR-CLUSTER\.device\.v1/}</string_expression>
          ` + supportedSlotExpression(`openEHR-EHR-CLUSTER\.device\.v1`) + `
        </includes>
        <includes>
          <string_expression>archetype_id/value matches {/openEHR-EHR-CLUSTER\.a\.v1/}</string_expression>
          ` + supportedSlotExpression(`openEHR-EHR-CLUSTER\.b\.v1`) + `
        </includes>
        <excludes>
          <string_expression>archetype_id/value matches {/.*/}</string_expression>
          ` + supportedSlotExpression(`.*`) + `
        </excludes>
      </children>
    </attributes>
  </definition>
</template>`

// supportedSlotExpression is the <expression> tree the parser reads as
// `archetype_id/value matches {pattern}`.
func supportedSlotExpression(pattern string) string {
	return `<expression xsi:type="EXPR_BINARY_OPERATOR">
            <type>Boolean</type>
            <operator>2007</operator>
            <left_operand xsi:type="EXPR_LEAF">
              <type>String</type>
              <item>archetype_id/value</item>
              <reference_type>attribute</reference_type>
            </left_operand>
            <right_operand xsi:type="EXPR_LEAF">
              <type>C_STRING</type>
              <item xsi:type="C_STRING"><pattern>` + pattern + `</pattern></item>
              <reference_type>constraint</reference_type>
            </right_operand>
          </expression>`
}

// REQ-104: a pattern one blob states more than once is listed once, in
// the place it first appears; a blob whose forms state different
// patterns keeps each of them, in order.
func TestParseOPT_SlotBlobPatternListedOnce(t *testing.T) {
	opt, err := template.ParseOPT(strings.NewReader(stringExpressionSlotOPT))
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
	wantInc := []string{`openEHR-EHR-CLUSTER\.device\.v1`, `openEHR-EHR-CLUSTER\.a\.v1`, `openEHR-EHR-CLUSTER\.b\.v1`}
	if got := slotPatterns(slot.ParsedIncludes()); !slices.Equal(got, wantInc) {
		t.Errorf("ParsedIncludes() patterns = %q, want %q", got, wantInc)
	}
	wantExc := []string{`.*`}
	if got := slotPatterns(slot.ParsedExcludes()); !slices.Equal(got, wantExc) {
		t.Errorf("ParsedExcludes() patterns = %q, want %q", got, wantExc)
	}
	rules := slot.SlotRules()
	if got := rules.RawIncludeCount; got != 2 {
		t.Errorf("SlotRules().RawIncludeCount = %d, want 2", got)
	}
	if got, want := rules.ExampleArchetypeID(), "openEHR-EHR-CLUSTER.device.v1"; got != want {
		t.Errorf("SlotRules().ExampleArchetypeID() = %q, want %q", got, want)
	}
}
