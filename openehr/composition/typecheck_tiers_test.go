package composition

import (
	"strings"
	"testing"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// tierValuePath is the ELEMENT value of the template tierOPT builds.
const tierValuePath = "/context/other_context/items[at0002]/value"

// tierOPT returns a COMPOSITION template whose one ELEMENT, in the
// context's other_context, offers values, the OPT children given, in their
// order, as the alternatives of its value. They share one path,
// tierValuePath.
func tierOPT(values ...string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>binding_tiers</value></template_id><concept>binding_tiers</concept>
<definition><rm_type_name>COMPOSITION</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>context</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>EVENT_CONTEXT</rm_type_name><node_id></node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>other_context</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ITEM_TREE</rm_type_name><node_id>at0001</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ELEMENT</rm_type_name><node_id>at0002</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>
` + strings.Join(values, "\n") + `
</attributes>
</children>
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>
</attributes>
</children>
</attributes>
</children>
</attributes>
<archetype_id><value>openEHR-EHR-COMPOSITION.report.v1</value></archetype_id>
<term_definitions code="at0000"><items id="text">Report</items><items id="description">Report</items></term_definitions>
<term_definitions code="at0001"><items id="text">Tree</items><items id="description">Tree</items></term_definitions>
<term_definitions code="at0002"><items id="text">Reading</items><items id="description">Reading</items></term_definitions>
</definition>
</template>`
}

// tierChild returns an OPT child that types an ELEMENT's value as rmType,
// escaped for XML, with no node id; an empty rmType leaves it untyped.
func tierChild(rmType string) string {
	rmType = strings.NewReplacer("<", "&lt;", ">", "&gt;").Replace(rmType)
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>` + rmType + `</rm_type_name><node_id></node_id></children>`
}

// tierDurationChild is the AOM 1.4 primitive short name DURATION as an
// ELEMENT's value, under C_PRIMITIVE_OBJECT.
const tierDurationChild = `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>DURATION</rm_type_name><node_id></node_id>` +
	`<item xsi:type="C_DURATION"></item></children>`

// TestREQ101_BindRMTypeTierOrder pins, for REQ-101 Type enforcement, the
// order of the binding tiers the builder shares with the validator (REQ-102
// Structural — alternatives): whatever the OPT order, a value binds a node
// that names its own type before the short name it carries, that short name
// before a node it only conforms to, and a node it names before an untyped
// one or a typed interval whose bounds it only conforms to.
func TestREQ101_BindRMTypeTierOrder(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		values []string
		v      any
		// want is the RM type of the node v binds.
		want string
	}{
		{
			name:   "DV_DURATION under DATA_VALUE then DURATION binds DURATION",
			values: []string{tierChild("DATA_VALUE"), tierDurationChild},
			v:      &rm.DVDuration{Value: "PT5M"}, want: "DURATION",
		},
		{
			name:   "DV_DURATION under DURATION then DV_DURATION binds DV_DURATION",
			values: []string{tierDurationChild, tierChild("DV_DURATION")},
			v:      &rm.DVDuration{Value: "PT5M"}, want: "DV_DURATION",
		},
		{
			name:   "DV_CODED_TEXT under untyped then DV_CODED_TEXT binds DV_CODED_TEXT",
			values: []string{tierChild(""), tierChild("DV_CODED_TEXT")},
			v: &rm.DVCodedText{
				Value:        "Lorem",
				DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "12345"},
			},
			want: "DV_CODED_TEXT",
		},
		{
			name:   "DV_INTERVAL<DV_COUNT> under DV_INTERVAL<DV_ORDERED> then DV_INTERVAL<DV_COUNT> binds DV_INTERVAL<DV_COUNT>",
			values: []string{tierChild("DV_INTERVAL<DV_ORDERED>"), tierChild("DV_INTERVAL<DV_COUNT>")},
			v:      &rm.DVInterval[rm.DVCount]{Lower: rm.DVCount{Magnitude: 1}, Upper: rm.DVCount{Magnitude: 5}},
			want:   "DV_INTERVAL<DV_COUNT>",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opt, err := template.ParseOPT(strings.NewReader(tierOPT(tc.values...)))
			if err != nil {
				t.Fatalf("ParseOPT: %v", err)
			}
			c, err := tcimpl.Compile(opt)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			node, err := c.NodeAt(tierValuePath)
			if err != nil {
				t.Fatalf("NodeAt(%s): %v", tierValuePath, err)
			}
			bound, err := bindRMType(node, tc.v)
			if err != nil {
				t.Fatalf("bindRMType(%s node, %T) = %v, want nil", node.RMTypeName(), tc.v, err)
			}
			if got := bound.RMTypeName(); got != tc.want {
				t.Errorf("bindRMType(%s node, %T) binds %s, want %s", node.RMTypeName(), tc.v, got, tc.want)
			}
		})
	}
}
