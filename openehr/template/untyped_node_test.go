package template_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// xsiNamespace is the XML Schema instance namespace an OPT binds its xsi
// prefix to.
const xsiNamespace = "http://www.w3.org/2001/XMLSchema-instance"

// untypedNodeOPT is a minimal OPT whose <definition> carries no xsi:type, as
// Template.xsd allows, and every <attributes> and <children> inside it
// carries one. %s is the namespace the xsi prefix is bound to.
const untypedNodeOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="%s">
  <template_id><value>untyped_node</value></template_id>
  <concept>untyped_node</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>category</rm_attribute_name>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>defining_code</rm_attribute_name>
          <children xsi:type="C_CODE_PHRASE">
            <rm_type_name>CODE_PHRASE</rm_type_name>
            <node_id></node_id>
            <terminology_id><value>openehr</value></terminology_id>
            <code_list>433</code_list>
          </children>
        </attributes>
      </children>
    </attributes>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>
      <children xsi:type="C_ARCHETYPE_ROOT">
        <rm_type_name>ACTION</rm_type_name>
        <node_id>at0000</node_id>
        <archetype_id><value>openEHR-EHR-ACTION.minimal_2.v1</value></archetype_id>
      </children>
    </attributes>
    <archetype_id><value>openEHR-EHR-COMPOSITION.minimal.v1</value></archetype_id>
  </definition>
</template>`

// untypedShape is what a parse of untypedNodeOPT gives at the three places
// a missing type changes: the attribute types and the node kinds.
type untypedShape struct {
	content          template.Cardinality // the content attribute
	codePhraseLeaf   bool                 // the defining_code child carries its C_CODE_PHRASE constraint
	actionIsArchRoot bool                 // the content child is an *ArchetypeRoot
}

// readUntypedShape reads the shape of a parse of untypedNodeOPT.
func readUntypedShape(t *testing.T, opt *template.OperationalTemplate) untypedShape {
	t.Helper()
	at := func(path string) template.Node {
		t.Helper()
		p, err := opt.ParsePath(path)
		if err != nil {
			t.Fatalf("ParsePath(%q): %v", path, err)
		}
		n, err := opt.NodeAt(p)
		if err != nil {
			t.Fatalf("NodeAt(%q): %v", path, err)
		}
		return n
	}
	root, ok := opt.Root().(template.ObjectNode)
	if !ok {
		t.Fatalf("Root() = %T, want an ObjectNode", opt.Root())
	}
	var shape untypedShape
	for _, a := range root.Attributes() {
		if a.Name() == "content" {
			shape.content = a.Cardinality()
		}
	}
	code, ok := at("/category/defining_code").(*template.ComplexObject)
	if !ok {
		t.Fatalf("/category/defining_code is %T, want *template.ComplexObject", at("/category/defining_code"))
	}
	shape.codePhraseLeaf = code.PrimitiveConstraint() != nil
	_, shape.actionIsArchRoot = at("/content").(*template.ArchetypeRoot)
	return shape
}

// TestREQ100_StrictRefusesNodeWithNoXSIType pins § REQ-100 Strict parse
// mode: strict mode refuses an <attributes> or <children> inside
// <definition> that carries no type in the XSI namespace, with
// ErrInvalidOPT, and the default mode reads it as it always has: an
// attribute as C_SINGLE_ATTRIBUTE, a node as C_ARCHETYPE_ROOT where it
// carries an archetype id and as C_COMPLEX_OBJECT otherwise. The
// <definition> itself needs no type, so the typed row parses in both modes.
func TestREQ100_StrictRefusesNodeWithNoXSIType(t *testing.T) {
	typed := fmt.Sprintf(untypedNodeOPT, xsiNamespace)
	full := untypedShape{content: template.Multiple, codePhraseLeaf: true, actionIsArchRoot: true}

	tests := []struct {
		name string
		body string
		// element names the element strict mode refuses, "" when it
		// accepts the document.
		element string
		lenient untypedShape
	}{
		{
			name:    "every node typed, the definition untyped",
			body:    typed,
			lenient: full,
		},
		{
			// The defect of two public OPTs: every xsi:type sits in a
			// namespace that is not the XSI one, so no node is typed.
			name:    "xsi prefix bound to a misspelt namespace",
			body:    fmt.Sprintf(untypedNodeOPT, "http //www.w3.org/2001/XMLSchema-instance"),
			element: "<attributes>",
			lenient: untypedShape{content: template.Single, codePhraseLeaf: false, actionIsArchRoot: true},
		},
		{
			name:    "children with no type",
			body:    strings.Replace(typed, `<children xsi:type="C_CODE_PHRASE">`, `<children>`, 1),
			element: "<children>",
			lenient: untypedShape{content: template.Multiple, codePhraseLeaf: false, actionIsArchRoot: true},
		},
		{
			name:    "children with an empty type",
			body:    strings.Replace(typed, `<children xsi:type="C_CODE_PHRASE">`, `<children xsi:type="">`, 1),
			element: "<children>",
			lenient: untypedShape{content: template.Multiple, codePhraseLeaf: false, actionIsArchRoot: true},
		},
		{
			name:    "children with no type and an archetype id",
			body:    strings.Replace(typed, `<children xsi:type="C_ARCHETYPE_ROOT">`, `<children>`, 1),
			element: "<children>",
			lenient: full,
		},
		{
			name:    "attributes with no type",
			body:    strings.Replace(typed, `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE">`, `<attributes>`, 1),
			element: "<attributes>",
			lenient: untypedShape{content: template.Single, codePhraseLeaf: true, actionIsArchRoot: true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.body == typed && tc.name != tests[0].name {
				t.Fatalf("the row's body is the typed OPT: its replacement did not apply")
			}
			lenient, err := template.ParseOPT(strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("ParseOPT = %v, want nil error", err)
			}
			if got := readUntypedShape(t, lenient); got != tc.lenient {
				t.Errorf("ParseOPT shape = %+v, want %+v", got, tc.lenient)
			}

			strict, err := template.ParseOPTStrict(strings.NewReader(tc.body))
			if tc.element == "" {
				if err != nil {
					t.Fatalf("ParseOPTStrict = %v, want nil error", err)
				}
				if got := readUntypedShape(t, strict); got != tc.lenient {
					t.Errorf("ParseOPTStrict shape = %+v, want %+v", got, tc.lenient)
				}
				return
			}
			if !errors.Is(err, template.ErrInvalidOPT) || strict != nil {
				t.Fatalf("ParseOPTStrict error = %v (template returned: %t), want an error wrapping ErrInvalidOPT and no template", err, strict != nil)
			}
			if errors.Is(err, template.ErrUnsupportedNode) {
				t.Errorf("ParseOPTStrict = %v, want no ErrUnsupportedNode: the node has no type, not an unknown one", err)
			}
			if !strings.Contains(err.Error(), tc.element) {
				t.Errorf("ParseOPTStrict = %v, want the message to name %s", err, tc.element)
			}
		})
	}
}
