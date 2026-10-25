package templatecompile_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// oneAttributeOPT is an OPT rooted at a node of RM type %s that declares the
// one attribute %s, required, with an untyped child of RM type DV_TEXT.
const oneAttributeOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>one_attribute</value></template_id>
  <concept>one_attribute</concept>
  <definition xsi:type="C_COMPLEX_OBJECT">
    <rm_type_name>%s</rm_type_name>
    <node_id>at0001</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>%s</rm_attribute_name>
      <existence><lower>1</lower><upper>1</upper></existence>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_TEXT</rm_type_name>
        <node_id></node_id>
      </children>
    </attributes>
  </definition>
</template>`

// compileOneAttribute compiles oneAttributeOPT for rmType and attr, parsed in
// strict mode, with opts, and returns its root.
func compileOneAttribute(t *testing.T, rmType, attr string, opts ...templatecompile.Option) *templatecompile.CompiledNode {
	t.Helper()
	escaped := strings.NewReplacer("<", "&lt;", ">", "&gt;").Replace(rmType)
	opt, err := template.ParseOPTStrict(strings.NewReader(fmt.Sprintf(oneAttributeOPT, escaped, attr)))
	if err != nil {
		t.Fatalf("ParseOPTStrict(%s.%s): %v", rmType, attr, err)
	}
	c, err := templatecompile.Compile(opt, opts...)
	if err != nil {
		t.Fatalf("Compile(%s.%s): %v", rmType, attr, err)
	}
	return c.Root()
}

// TestREQ100_CompileMarksAttributeNotInRM pins § REQ-100 Node taxonomy: the
// compile marks a declared attribute whose node's RM class, read without
// generic parameters, has no property of that name in the pinned BMM and
// that is not a function rminfo.IsNonStorableAttr names. A property, one
// of those functions, and an attribute of a class the lookup does not know
// stay unmarked, and so does every attribute the compile injects.
func TestREQ100_CompileMarksAttributeNotInRM(t *testing.T) {
	tests := []struct {
		rmType, attr string
		want         bool
	}{
		{rmType: "ISM_TRANSITION", attr: "name", want: true},
		{rmType: "ISM_TRANSITION", attr: "current_state", want: false},
		{rmType: "DV_INTERVAL<DV_QUANTITY>", attr: "name", want: true},
		{rmType: "DV_INTERVAL<DV_QUANTITY>", attr: "lower", want: false},
		{rmType: "DV_PROPORTION", attr: "is_integral", want: false},
		{rmType: "DV_QUANTITY", attr: "is_integral", want: false},
		{rmType: "POINT_EVENT", attr: "offset", want: false},
		{rmType: "INTERVAL_EVENT", attr: "offset", want: false},
		{rmType: "POINT_EVENT<ITEM_TREE>", attr: "offset", want: false},
		{rmType: "NOT_AN_RM_CLASS", attr: "name", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.rmType+"."+tc.attr, func(t *testing.T) {
			root := compileOneAttribute(t, tc.rmType, tc.attr)
			a := root.Attribute(tc.attr)
			if a == nil {
				t.Fatalf("Compile(%s.%s): no attribute %s", tc.rmType, tc.attr, tc.attr)
			}
			if got := a.NotInRM(); got != tc.want {
				t.Errorf("Compile(%s.%s): NotInRM() = %t, want %t", tc.rmType, tc.attr, got, tc.want)
			}
			for _, other := range root.Attributes() {
				if other.Implicit() && other.NotInRM() {
					t.Errorf("Compile(%s.%s): injected attribute %s is marked NotInRM", tc.rmType, tc.attr, other.Name())
				}
			}
		})
	}
}

// TestREQ100_CompileMarksAttributeNotInRMByItsLookup pins that the mark comes
// from the lookup the compile uses: a class the lookup knows is judged by
// the properties the lookup gives it, and a class it does not know is not
// judged at all, whatever the pinned RM says of either.
func TestREQ100_CompileMarksAttributeNotInRMByItsLookup(t *testing.T) {
	lookup := rminfo.New(map[string]rminfo.ClassMeta{
		"ISM_TRANSITION": {
			Attributes: map[string]rminfo.AttrMeta{"name": {TypeName: "DV_TEXT", DeclaredIn: "ISM_TRANSITION"}},
			AttrOrder:  []string{"name"},
		},
	})
	tests := []struct {
		rmType, attr string
		want         bool
	}{
		{rmType: "ISM_TRANSITION", attr: "name", want: false},
		{rmType: "ISM_TRANSITION", attr: "current_state", want: true},
		{rmType: "ELEMENT", attr: "flavour", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.rmType+"."+tc.attr, func(t *testing.T) {
			a := compileOneAttribute(t, tc.rmType, tc.attr, templatecompile.WithRMInfo(lookup)).Attribute(tc.attr)
			if a == nil {
				t.Fatalf("Compile(%s.%s): no attribute %s", tc.rmType, tc.attr, tc.attr)
			}
			if got := a.NotInRM(); got != tc.want {
				t.Errorf("Compile(%s.%s) with a synthetic lookup: NotInRM() = %t, want %t", tc.rmType, tc.attr, got, tc.want)
			}
		})
	}
}
