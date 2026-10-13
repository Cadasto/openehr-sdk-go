package validation_test

// prohibited_test.go: REQ-102 § Validation dimensions, Structural —
// prohibited attributes. An attribute whose OPT existence has an upper bound
// of 0 is reported once, as `prohibited` at the attribute's path, when it is
// present: a single-valued attribute that holds a value, a multi-valued one
// that holds at least one item. The template walk does not walk into it. No
// vendored OPT prohibits an attribute, so the template here is built inline;
// the same instances under a template that allows the attributes draw a
// finding from inside each, so "nothing from inside" is a claim that can fail.

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// prohibitedArchetype is the archetype of the root of prohibitedOPT.
const prohibitedArchetype = "openEHR-EHR-ELEMENT.prohibited.v1"

// localAt0001 is a C_CODE_PHRASE child that admits local::at0001 alone.
const localAt0001 = `<children xsi:type="C_CODE_PHRASE"><rm_type_name>CODE_PHRASE</rm_type_name><node_id></node_id>` +
	`<terminology_id><value>local</value></terminology_id><code_list>at0001</code_list></children>`

// prohibitedOPT returns an ELEMENT template whose null_flavour, a
// single-valued attribute, and whose DV_TEXT value's mappings, a
// multi-valued one, both have the existence lower..upper. Below each, the
// template admits only local::at0001: as the null flavour's defining_code,
// and as a mapping's target. The value itself is optional.
func prohibitedOPT(lower, upper string) string {
	existence := `<existence>` + fiInterval(lower, upper) + `</existence>`
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>prohibited</value></template_id><concept>prohibited</concept>
<definition><rm_type_name>ELEMENT</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>null_flavour</rm_attribute_name>` + existence + `
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_CODED_TEXT</rm_type_name><node_id></node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>defining_code</rm_attribute_name>` + localAt0001 + `</attributes>
</children></attributes>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>
<existence>` + fiInterval("0", "1") + `</existence>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_TEXT</rm_type_name><node_id></node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>mappings</rm_attribute_name>` + existence + `
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>TERM_MAPPING</rm_type_name><node_id></node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>target</rm_attribute_name>` + localAt0001 + `</attributes>
</children>
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		`<lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower>` +
		`</interval></cardinality>
</attributes></children></attributes>
<archetype_id><value>` + prohibitedArchetype + `</value></archetype_id></definition>
</template>`
}

// prohibitedElement returns the root of prohibitedOPT carrying value and
// nullFlavour, either of which may be nil.
func prohibitedElement(value rm.DataValue, nullFlavour *rm.DVCodedText) *rm.Element {
	return &rm.Element{ArchetypeNodeID: prohibitedArchetype, Name: rm.DVText{Value: "element"}, Value: value, NullFlavour: nullFlavour}
}

// templateIssues returns the template-layer issues of r, as identityKeys
// writes them.
func templateIssues(r validation.Result) []string {
	var out []validation.Issue
	for _, i := range r.Issues {
		if i.Layer == validation.LayerTemplate {
			out = append(out, i)
		}
	}
	return identityKeys(out)
}

// REQ-102, REQ-171 — a prohibited attribute that is present draws one
// `prohibited` issue at its path, on the template layer with the rule
// existence, an error whose sentinel is ErrProhibited and whose Detail
// names the attribute and the RM type but no value; the template walk reports
// nothing from inside it. An absent prohibited attribute, and a multi-valued
// one that holds no item, draw nothing from the template walk. Under a
// template that allows the attributes, the same instances draw the finding
// from inside each, which the prohibition hides.
func TestValidate_REQ102_REQ171_ProhibitedAttribute(t *testing.T) {
	t.Parallel()
	prohibits := mustCompileInline(t, prohibitedOPT("0", "0"))
	allows := mustCompileInline(t, prohibitedOPT("0", "1"))
	snomed := rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "271649006"}
	noInformation := &rm.DVCodedText{Value: "no information", DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "271"}}
	mapped := &rm.DVText{Value: "text", Mappings: []rm.TermMapping{{Match: rm.Character("="), Target: snomed}}}
	tests := []struct {
		name string
		root *rm.Element
		// want is the whole template-layer report under the template that
		// prohibits the attributes; inside is what the template walk reports
		// under the one that allows them.
		want, inside []string
	}{
		{
			name: "single-valued attribute holding a value",
			root: prohibitedElement(nil, noInformation),
			want: []string{"prohibited /null_flavour template existence"},
			inside: []string{
				"primitive_invalid_value /null_flavour/defining_code template invalid_value",
				"primitive_not_in_list /null_flavour/defining_code template not_in_list",
			},
		},
		{
			name: "multi-valued attribute holding an item",
			root: prohibitedElement(mapped, nil),
			want: []string{"prohibited /value/mappings template existence"},
			inside: []string{
				"primitive_invalid_value /value/mappings[1]/target template invalid_value",
				"primitive_not_in_list /value/mappings[1]/target template not_in_list",
			},
		},
		{
			name: "both absent",
			root: prohibitedElement(&rm.DVText{Value: "text"}, nil),
		},
		{
			name: "multi-valued attribute holding no item",
			root: prohibitedElement(&rm.DVText{Value: "text", Mappings: []rm.TermMapping{}}, nil),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := templateIssues(validation.Validate(tc.root, allows)); !slices.Equal(got, tc.inside) {
				t.Fatalf("precondition: under a template that allows the attributes, template issues = %q, want %q", got, tc.inside)
			}
			r := validation.Validate(tc.root, prohibits)
			if got := templateIssues(r); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%s) template issues = %q, want %q (REQ-102 § Structural — prohibited attributes)", tc.name, got, tc.want)
			}
			for _, i := range r.Issues {
				if i.Code != "prohibited" {
					continue
				}
				if !errors.Is(i.Err(), validation.ErrProhibited) {
					t.Errorf("prohibited at %s: Err() = %v, want ErrProhibited", i.Path, i.Err())
				}
				if i.Severity != validation.Error || r.OK {
					t.Errorf("prohibited at %s: Severity %v and Result.OK %t, want an error that fails the result", i.Path, i.Severity, r.OK)
				}
				if v := i.Value.Reveal(); v != nil {
					t.Errorf("prohibited at %s: Value.Reveal() = %v, want empty", i.Path, v)
				}
				attr := i.Path[strings.LastIndex(i.Path, "/")+1:]
				if !strings.Contains(i.Detail, `"`+attr+`"`) || !strings.Contains(i.Detail, "ELEMENT") && !strings.Contains(i.Detail, "DV_TEXT") {
					t.Errorf("prohibited at %s: Detail %q does not name the attribute %q and its parent's RM type", i.Path, i.Detail, attr)
				}
				for _, value := range []string{"no information", "271", "SNOMED-CT", "271649006", "text"} {
					if strings.Contains(i.Detail, value) {
						t.Errorf("prohibited at %s: Detail %q carries the instance's %q (REQ-168)", i.Path, i.Detail, value)
					}
				}
			}
			assertIdentities(t, "Validate("+tc.name+")", r, mustBMMInvariants(t))
		})
	}
}
