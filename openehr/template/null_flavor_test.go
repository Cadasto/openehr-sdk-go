package template_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// nullFlavorOPT is a minimal OPT rooted at a node of RM type %s that
// declares the attribute %s, constrained to the null flavour 271.
const nullFlavorOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>null_flavor</value></template_id>
  <concept>null_flavor</concept>
  <definition xsi:type="C_COMPLEX_OBJECT">
    <rm_type_name>%s</rm_type_name>
    <node_id>at0001</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>%s</rm_attribute_name>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>defining_code</rm_attribute_name>
          <children xsi:type="C_CODE_PHRASE">
            <rm_type_name>CODE_PHRASE</rm_type_name>
            <node_id></node_id>
            <terminology_id><value>openehr</value></terminology_id>
            <code_list>271</code_list>
          </children>
        </attributes>
      </children>
    </attributes>
  </definition>
</template>`

// TestREQ100_NullFlavorReadAsNullFlavour pins § REQ-100 Node taxonomy: both
// modes read an ELEMENT's null_flavor as null_flavour, so the path and the
// constraint are the RM attribute's. That one spelling on that one class is
// the whole reading: the same name on another class, and any other name on
// an ELEMENT, stay as written.
func TestREQ100_NullFlavorReadAsNullFlavour(t *testing.T) {
	tests := []struct {
		name     string
		rmType   string
		attr     string
		wantAttr string
	}{
		{name: "ELEMENT null_flavor reads as null_flavour", rmType: "ELEMENT", attr: "null_flavor", wantAttr: "null_flavour"},
		{name: "ELEMENT null_flavour stays", rmType: "ELEMENT", attr: "null_flavour", wantAttr: "null_flavour"},
		{name: "null_flavor on another class stays", rmType: "CLUSTER", attr: "null_flavor", wantAttr: "null_flavor"},
		{name: "another misspelling on ELEMENT stays", rmType: "ELEMENT", attr: "null_flavours", wantAttr: "null_flavours"},
	}
	parsers := []struct {
		name  string
		parse func(string) (*template.OperationalTemplate, error)
	}{
		{name: "ParseOPT", parse: func(s string) (*template.OperationalTemplate, error) { return template.ParseOPT(strings.NewReader(s)) }},
		{name: "ParseOPTStrict", parse: func(s string) (*template.OperationalTemplate, error) {
			return template.ParseOPTStrict(strings.NewReader(s))
		}},
	}
	for _, tc := range tests {
		for _, p := range parsers {
			t.Run(tc.name+"/"+p.name, func(t *testing.T) {
				opt, err := p.parse(fmt.Sprintf(nullFlavorOPT, tc.rmType, tc.attr))
				if err != nil {
					t.Fatalf("%s(%s.%s) = %v, want nil error", p.name, tc.rmType, tc.attr, err)
				}
				root, ok := opt.Root().(template.ObjectNode)
				if !ok {
					t.Fatalf("%s: Root() = %T, want an ObjectNode", p.name, opt.Root())
				}
				var names []string
				for _, a := range root.Attributes() {
					names = append(names, a.Name())
				}
				if want := []string{tc.wantAttr}; !slices.Equal(names, want) {
					t.Fatalf("%s(%s.%s): attribute names = %q, want %q", p.name, tc.rmType, tc.attr, names, want)
				}
				path, err := opt.ParsePath("/" + tc.wantAttr + "/defining_code")
				if err != nil {
					t.Fatalf("ParsePath: %v", err)
				}
				leaf, err := opt.NodeAt(path)
				if err != nil {
					t.Fatalf("%s(%s.%s): NodeAt(%s) = %v, want the CODE_PHRASE leaf", p.name, tc.rmType, tc.attr, path, err)
				}
				if co, ok := leaf.(*template.ComplexObject); !ok || co.PrimitiveConstraint() == nil {
					t.Errorf("%s(%s.%s): NodeAt(%s) = %T, want a leaf carrying its C_CODE_PHRASE constraint", p.name, tc.rmType, tc.attr, path, leaf)
				}
			})
		}
	}
}

// nullFlavorBothOPT is a minimal OPT rooted at an ELEMENT that declares the
// attribute %s, constrained to the null flavour %s, then the attribute %s,
// constrained to the null flavour %s.
const nullFlavorBothOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>null_flavor_both</value></template_id>
  <concept>null_flavor_both</concept>
  <definition xsi:type="C_COMPLEX_OBJECT">
    <rm_type_name>ELEMENT</rm_type_name>
    <node_id>at0001</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>%s</rm_attribute_name>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>defining_code</rm_attribute_name>
          <children xsi:type="C_CODE_PHRASE">
            <rm_type_name>CODE_PHRASE</rm_type_name>
            <node_id></node_id>
            <terminology_id><value>openehr</value></terminology_id>
            <code_list>%s</code_list>
          </children>
        </attributes>
      </children>
    </attributes>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>%s</rm_attribute_name>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>defining_code</rm_attribute_name>
          <children xsi:type="C_CODE_PHRASE">
            <rm_type_name>CODE_PHRASE</rm_type_name>
            <node_id></node_id>
            <terminology_id><value>openehr</value></terminology_id>
            <code_list>%s</code_list>
          </children>
        </attributes>
      </children>
    </attributes>
  </definition>
</template>`

// TestREQ100_NullFlavorKeptBesideNullFlavour pins § REQ-100 Node taxonomy:
// an ELEMENT that declares both null_flavor and null_flavour keeps
// null_flavor as written in both modes, in either order, so the node does
// not declare null_flavour twice and each attribute keeps its own
// constraint.
func TestREQ100_NullFlavorKeptBesideNullFlavour(t *testing.T) {
	parsers := []struct {
		name  string
		parse func(string) (*template.OperationalTemplate, error)
	}{
		{name: "ParseOPT", parse: func(s string) (*template.OperationalTemplate, error) { return template.ParseOPT(strings.NewReader(s)) }},
		{name: "ParseOPTStrict", parse: func(s string) (*template.OperationalTemplate, error) {
			return template.ParseOPTStrict(strings.NewReader(s))
		}},
	}
	orders := []struct {
		name      string
		body      string
		wantNames []string
	}{
		{
			name:      "null_flavor first",
			body:      fmt.Sprintf(nullFlavorBothOPT, "null_flavor", "271", "null_flavour", "253"),
			wantNames: []string{"null_flavor", "null_flavour"},
		},
		{
			name:      "null_flavour first",
			body:      fmt.Sprintf(nullFlavorBothOPT, "null_flavour", "253", "null_flavor", "271"),
			wantNames: []string{"null_flavour", "null_flavor"},
		},
	}
	for _, o := range orders {
		for _, p := range parsers {
			t.Run(o.name+"/"+p.name, func(t *testing.T) {
				opt, err := p.parse(o.body)
				if err != nil {
					t.Fatalf("%s = %v, want nil error", p.name, err)
				}
				root, ok := opt.Root().(template.ObjectNode)
				if !ok {
					t.Fatalf("%s: Root() = %T, want an ObjectNode", p.name, opt.Root())
				}
				var names []string
				for _, a := range root.Attributes() {
					names = append(names, a.Name())
				}
				if !slices.Equal(names, o.wantNames) {
					t.Fatalf("%s: attribute names = %q, want %q", p.name, names, o.wantNames)
				}
				for attr, code := range map[string]string{"null_flavor": "271", "null_flavour": "253"} {
					path, err := opt.ParsePath("/" + attr + "/defining_code")
					if err != nil {
						t.Fatalf("ParsePath: %v", err)
					}
					leaf, err := opt.NodeAt(path)
					if err != nil {
						t.Fatalf("%s: NodeAt(%s) = %v, want the CODE_PHRASE leaf", p.name, path, err)
					}
					co, ok := leaf.(*template.ComplexObject)
					if !ok {
						t.Fatalf("%s: NodeAt(%s) = %T, want a *template.ComplexObject", p.name, path, leaf)
					}
					got, ok := co.PrimitiveConstraint().(constraints.CodePhrase)
					if want := []string{code}; !ok || !slices.Equal(got.CodeList, want) {
						t.Errorf("%s: NodeAt(%s) constraint = %#v, want a CodePhrase listing %q", p.name, path, co.PrimitiveConstraint(), want)
					}
				}
			})
		}
	}
}
