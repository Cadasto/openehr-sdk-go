package composition

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// TestREQ101_PlainValueTypesArePrimitives checks, for REQ-101 Type
// enforcement, that every plain value type the attribute check refuses an
// RM value under is a name the pinned RM metadata knows as a primitive
// type, so a misspelt entry, which no attribute type would ever match,
// fails here rather than letting Set accept a value the attribute cannot
// hold.
//
// It also checks the other direction: every attribute type the metadata
// reports that is no class the hierarchy knows is a plain value type, one
// of the types below that are not plain, or a formal generic parameter of
// the attribute's class, which the check reads through the parameter's
// constraint. A pinned-RM bump that types an attribute with another plain
// primitive then fails here, rather than going unchecked.
func TestREQ101_PlainValueTypesArePrimitives(t *testing.T) {
	t.Parallel()
	ar, ok := rminfo.Default.(rminfo.AbsenceReporter)
	if !ok {
		t.Fatal("rminfo.Default does not report absence reasons")
	}
	for name := range plainValueTypes {
		if got := ar.AbsenceReason(name); got != rminfo.AbsencePrimitive {
			t.Errorf("rminfo.Default.AbsenceReason(%q) = %v, want %v", name, got, rminfo.AbsencePrimitive)
		}
	}

	// notPlain lists the attribute types that are no class the hierarchy
	// knows but whose values are not plain: every RM value conforms to Any,
	// and a Hash holds entries, not one plain value.
	notPlain := map[string]bool{"Any": true, "Hash": true}
	lister, ok := rminfo.Default.(rminfo.AttributeLister)
	if !ok {
		t.Fatal("rminfo.Default does not list attributes")
	}
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		t.Fatal("rminfo.Default does not answer class hierarchy questions")
	}
	plain := 0
	for _, class := range rminfo.Default.KnownRMTypes() {
		for _, attr := range lister.AttributeNames(class) {
			typ, _ := rminfo.Default.AttributeRMType(class, attr)
			if _, known := h.IsAbstract(bmmtype.Class(typ)); known {
				continue
			}
			if plainValueTypes[typ] {
				plain++
				continue
			}
			if notPlain[bmmtype.Class(typ)] {
				continue
			}
			if _, isParameter := bmmtype.ParameterConstraint(class, typ); !isParameter {
				t.Errorf("rminfo types %s.%s as %q, which is no class the hierarchy knows, no plain value type and no generic parameter of %s: add it to plainValueTypes if its values are plain, or to notPlain", class, attr, typ, class)
			}
		}
	}
	if plain == 0 {
		t.Fatal("rminfo types no attribute with a plain value type; the tables did not load as expected")
	}
}

// TestREQ101_CanHold checks, for REQ-101 Type enforcement, which values an
// RM attribute of a given type can hold: a value of a type that is the
// attribute's or descends from it, by class; no RM value under a plain
// value type, such as the String of DV_DURATION.value or the Integer64 of
// DV_COUNT.magnitude; and no verdict, so no refusal, where the pinned
// class hierarchy does not know the attribute's type: a generic parameter
// left open, Any, and Iso8601_duration, which DV_DURATION descends from in
// the BMM although the hierarchy drops that edge.
func TestREQ101_CanHold(t *testing.T) {
	t.Parallel()
	cases := []struct {
		held, got    string
		holds, known bool
	}{
		{held: "String", got: "DV_DURATION", holds: false, known: true},
		{held: "Integer64", got: "DV_COUNT", holds: false, known: true},
		{held: "DATA_VALUE", got: "DV_QUANTITY", holds: true, known: true},
		{held: "DATA_VALUE", got: "ELEMENT", holds: false, known: true},
		{held: "DATA_VALUE", got: "PARTY_IDENTIFIED", holds: false, known: true},
		{held: "DV_COUNT", got: "DV_COUNT", holds: true, known: true},
		{held: "DV_COUNT", got: "DV_QUANTITY", holds: false, known: true},
		{held: "DV_INTERVAL", got: "DV_INTERVAL<DV_COUNT>", holds: true, known: true},
		{held: "T", got: "DV_QUANTITY", holds: false, known: false},
		{held: "Any", got: "DV_DURATION", holds: false, known: false},
		{held: "Iso8601_duration", got: "DV_DURATION", holds: false, known: false},
	}
	for _, tc := range cases {
		t.Run(tc.held+" holds "+tc.got, func(t *testing.T) {
			t.Parallel()
			holds, known := canHold(tc.held, tc.got)
			if known != tc.known || (known && holds != tc.holds) {
				t.Errorf("canHold(%q, %q) = (%v, %v), want (%v, %v)", tc.held, tc.got, holds, known, tc.holds, tc.known)
			}
		})
	}
}

// unknownOwnerOPT is a COMPOSITION template whose context's other_context
// is typed with unknownOwnerClass, a class the pinned RM metadata does not
// know, constraining an attribute named summary as a DV_TEXT.
const (
	unknownOwnerClass = "FUTURE_STRUCTURE"
	unknownOwnerOPT   = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>unknown_owner</value></template_id><concept>unknown_owner</concept>
<definition><rm_type_name>COMPOSITION</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>context</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>EVENT_CONTEXT</rm_type_name><node_id></node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>other_context</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>` + unknownOwnerClass + `</rm_type_name><node_id>at0001</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>summary</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_TEXT</rm_type_name><node_id></node_id></children>
</attributes>
</children>
</attributes>
</children>
</attributes>
<archetype_id><value>openEHR-EHR-COMPOSITION.report.v1</value></archetype_id>
</definition>
</template>`
	unknownOwnerSummaryPath = "/context/other_context/summary"
)

// TestREQ101_AttributeHoldsUnderAnUnknownClass checks, for REQ-101 Type
// enforcement, that the attribute check refuses nothing under a parent
// whose class the pinned RM metadata does not know. The metadata cannot
// tell whether such a class has the attribute, so the path is not taken
// for one that writes no RM attribute.
func TestREQ101_AttributeHoldsUnderAnUnknownClass(t *testing.T) {
	t.Parallel()
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		t.Fatal("rminfo.Default does not answer class hierarchy questions")
	}
	if _, known := h.IsAbstract(unknownOwnerClass); known {
		t.Fatalf("rminfo.Default knows %s; pick a class name it does not know (fixture drift)", unknownOwnerClass)
	}
	opt, err := template.ParseOPT(strings.NewReader(unknownOwnerOPT))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	c, err := tcimpl.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	node, err := c.NodeAt(unknownOwnerSummaryPath)
	if err != nil {
		t.Fatalf("NodeAt(%s): %v (fixture drift)", unknownOwnerSummaryPath, err)
	}
	if got := node.Parent().RMTypeName(); got != unknownOwnerClass {
		t.Fatalf("node at %s sits under RM type %q, want %q (fixture drift)", unknownOwnerSummaryPath, got, unknownOwnerClass)
	}
	attr := findAttributeContaining(node.Parent(), node)
	if attr == nil {
		t.Fatalf("node at %s sits under no attribute of its parent (fixture drift)", unknownOwnerSummaryPath)
	}
	v := &rm.DVText{Value: "x"}
	if err := attributeHolds(node, attr, v, "DV_TEXT"); err != nil {
		t.Errorf("attributeHolds(%s, %T, %q) = %v, want nil", unknownOwnerSummaryPath, v, "DV_TEXT", err)
	}
}
