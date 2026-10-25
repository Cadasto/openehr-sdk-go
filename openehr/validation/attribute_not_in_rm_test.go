package validation_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// compileStrictOPT parses body in strict mode and compiles it.
func compileStrictOPT(t *testing.T, body string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseOPTStrict(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseOPTStrict: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return c
}

// nullFlavorElementOPT is an OPT rooted at an ELEMENT that constrains the
// attribute %s to a DV_CODED_TEXT whose code is openehr 271.
const nullFlavorElementOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>null_flavor_element</value></template_id>
  <concept>null_flavor_element</concept>
  <definition xsi:type="C_COMPLEX_OBJECT">
    <rm_type_name>ELEMENT</rm_type_name>
    <node_id>at0001</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>%s</rm_attribute_name>
      <existence><lower>0</lower><upper>1</upper></existence>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>defining_code</rm_attribute_name>
          <existence><lower>1</lower><upper>1</upper></existence>
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

// TestREQ100_NullFlavorConstraintApplies pins § REQ-100 Node taxonomy end to
// end: an OPT that spells an ELEMENT's null_flavour null_flavor compiles to an
// attribute named null_flavour, of the RM type the BMM gives it, and the
// validator applies its code list to the ELEMENT's null_flavour, exactly as
// for the RM spelling.
func TestREQ100_NullFlavorConstraintApplies(t *testing.T) {
	element := func(code string) *rm.Element {
		return &rm.Element{
			ArchetypeNodeID: "at0001",
			Name:            rm.DVText{Value: "item"},
			NullFlavour: &rm.DVCodedText{
				Value:        "no information",
				DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: code},
			},
		}
	}
	for _, spelling := range []string{"null_flavor", "null_flavour"} {
		t.Run(spelling, func(t *testing.T) {
			c := compileStrictOPT(t, fmt.Sprintf(nullFlavorElementOPT, spelling))
			attr := c.Root().Attribute("null_flavour")
			if attr == nil {
				var names []string
				for _, a := range c.Root().Attributes() {
					names = append(names, a.Name())
				}
				t.Fatalf("Compile(ELEMENT.%s): no attribute null_flavour; attributes %q", spelling, names)
			}
			if got := attr.RMTypeName(); got != "DV_CODED_TEXT" {
				t.Errorf("Compile(ELEMENT.%s): null_flavour RMTypeName() = %q, want DV_CODED_TEXT", spelling, got)
			}

			if r := validation.Validate(element("271"), c); len(r.Issues) != 0 {
				t.Errorf("Validate(null_flavour 271, listed) issues = %+v, want none", issueKeys(r.Issues))
			}
			// 253 is a valid null flavour the OPT does not list.
			r := validation.Validate(element("253"), c)
			want := []string{"primitive_not_in_list /null_flavour/defining_code"}
			if got := issueKeys(r.Issues); !slices.Equal(got, want) {
				t.Errorf("Validate(null_flavour 253, not listed) issues = %+v, want %+v", got, want)
			}
		})
	}
}
