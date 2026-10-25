package template_test

import (
	"errors"
	"fmt"
	"strconv"
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

// TestREQ100_StrictRefusalNamesItsPath pins that strict mode's refusal of an
// untyped <attributes> or <children> names the OPT path of the attribute,
// built from attribute names and node or archetype ids only, so a reader can
// find the element without the message carrying a constraint value.
func TestREQ100_StrictRefusalNamesItsPath(t *testing.T) {
	typed := fmt.Sprintf(untypedNodeOPT, xsiNamespace)
	const actionID = `<archetype_id><value>openEHR-EHR-ACTION.minimal_2.v1</value></archetype_id>`
	inAction := func(attributes string) string {
		return strings.Replace(typed, actionID, attributes+"\n        "+actionID, 1)
	}
	tests := []struct {
		name string
		body string
		path string
	}{
		{
			name: "attributes on the root",
			body: strings.Replace(typed, `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE">`, `<attributes>`, 1),
			path: "/content",
		},
		{
			name: "children below a node with no id",
			body: strings.Replace(typed, `<children xsi:type="C_CODE_PHRASE">`, `<children>`, 1),
			path: "/category/defining_code",
		},
		{
			name: "children with an archetype id",
			body: strings.Replace(typed, `<children xsi:type="C_ARCHETYPE_ROOT">`, `<children>`, 1),
			path: "/content",
		},
		{
			name: "attributes inside an archetype root",
			body: inAction(`<attributes><rm_attribute_name>ism_transition</rm_attribute_name></attributes>`),
			path: "/content[openEHR-EHR-ACTION.minimal_2.v1]/ism_transition",
		},
		{
			name: "children inside an archetype root",
			body: inAction(`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>ism_transition</rm_attribute_name>` +
				`<children><rm_type_name>ISM_TRANSITION</rm_type_name><node_id>at0002</node_id></children></attributes>`),
			path: "/content[openEHR-EHR-ACTION.minimal_2.v1]/ism_transition",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.body == typed {
				t.Fatal("the row's body is the typed OPT: its replacement did not apply")
			}
			_, err := template.ParseOPTStrict(strings.NewReader(tc.body))
			if !errors.Is(err, template.ErrInvalidOPT) {
				t.Fatalf("ParseOPTStrict = %v, want an error wrapping ErrInvalidOPT", err)
			}
			if want := strconv.Quote(tc.path); !strings.Contains(err.Error(), want) {
				t.Errorf("ParseOPTStrict = %v, want the message to name the path %s", err, want)
			}
			// 433 is the code the category's C_CODE_PHRASE lists: a value,
			// not a structural name.
			if strings.Contains(err.Error(), "433") {
				t.Errorf("ParseOPTStrict = %v, want no constraint value in the message", err)
			}
		})
	}
}

// TestREQ100_InvalidOPTPrefixOnce pins that a parse error carries the
// ErrInvalidOPT prefix once, wherever the parser found the defect, and still
// matches the sentinel, and ErrUnsupportedNode where that is the cause.
func TestREQ100_InvalidOPTPrefixOnce(t *testing.T) {
	typed := fmt.Sprintf(untypedNodeOPT, xsiNamespace)
	const codePhrase = `<children xsi:type="C_CODE_PHRASE">
            <rm_type_name>CODE_PHRASE</rm_type_name>
            <node_id></node_id>
            <terminology_id><value>openehr</value></terminology_id>
            <code_list>433</code_list>
          </children>`
	if strings.Count(typed, codePhrase) != 1 {
		t.Fatal("the C_CODE_PHRASE child is not found exactly once")
	}
	withLeaf := func(leaf string) string { return strings.Replace(typed, codePhrase, leaf, 1) }
	tests := []struct {
		name        string
		body        string
		strict      bool
		unsupported bool
	}{
		{name: "untyped attributes", body: strings.Replace(typed, `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE">`, `<attributes>`, 1), strict: true},
		{name: "untyped children", body: strings.Replace(typed, `<children xsi:type="C_CODE_PHRASE">`, `<children>`, 1), strict: true},
		{name: "CONSTRAINT_REF with no reference", body: withLeaf(`<children xsi:type="CONSTRAINT_REF"><rm_type_name>CODE_PHRASE</rm_type_name><node_id/></children>`), strict: true},
		{name: "C_PRIMITIVE_OBJECT with no item", body: withLeaf(`<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>STRING</rm_type_name><node_id/></children>`), strict: true},
		{name: "untyped item", body: withLeaf(`<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>STRING</rm_type_name><node_id/><item><list>a</list></item></children>`), strict: true},
		{name: "unknown attribute type", body: strings.Replace(typed, `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE">`, `<attributes xsi:type="C_OTHER_ATTRIBUTE">`, 1), unsupported: true},
		{name: "missing template_id", body: strings.Replace(typed, `<template_id><value>untyped_node</value></template_id>`, ``, 1)},
	}
	prefix := template.ErrInvalidOPT.Error()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parse := template.ParseOPT
			if tc.strict {
				parse = template.ParseOPTStrict
			}
			_, err := parse(strings.NewReader(tc.body))
			if !errors.Is(err, template.ErrInvalidOPT) {
				t.Fatalf("parse = %v, want an error wrapping ErrInvalidOPT", err)
			}
			if got := strings.Count(err.Error(), prefix); got != 1 {
				t.Errorf("parse = %q, carries %q %d times, want once", err, prefix, got)
			}
			if got := errors.Is(err, template.ErrUnsupportedNode); got != tc.unsupported {
				t.Errorf("parse = %v, errors.Is(err, ErrUnsupportedNode) = %t, want %t", err, got, tc.unsupported)
			}
		})
	}
}
