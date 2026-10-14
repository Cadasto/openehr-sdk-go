package validation_test

// prohibited_test.go: REQ-102 § Validation dimensions, Structural —
// prohibited attributes. An attribute whose OPT existence has an upper bound
// of 0 is reported once, as `prohibited` at the attribute's path, when it is
// present: a single-valued attribute that holds a value, a multi-valued one
// that holds at least one item. The walk does not walk into it. No
// vendored OPT prohibits an attribute, so the template here is built inline;
// the same instances under a template that allows the attributes draw a
// finding from inside each, so "nothing from inside" is a claim that can fail.

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
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
	return existenceOPT(fiInterval(lower, upper))
}

// unboundedFromZero is an AOM interval body from 0 with no upper bound: the
// upper is marked unbounded and carries no value.
const unboundedFromZero = `<lower_included>true</lower_included><upper_included>false</upper_included>` +
	`<lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower>`

// existenceOPT is prohibitedOPT with the existence interval body given whole.
func existenceOPT(interval string) string {
	existence := `<existence>` + interval + `</existence>`
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
// names the attribute and the RM type but no value; the template reports
// nothing from inside it. An absent prohibited attribute, and a multi-valued
// one that holds no item, draw nothing from the template. Under a
// template that allows the attributes, the same instances draw the finding
// from inside each, which the prohibition hides. An existence whose upper
// bound is unbounded prohibits nothing, though it carries no upper value:
// the walk enters the attributes and reports what is inside.
func TestValidate_REQ102_REQ171_ProhibitedAttribute(t *testing.T) {
	t.Parallel()
	prohibits := mustCompileInline(t, prohibitedOPT("0", "0"))
	allows := mustCompileInline(t, prohibitedOPT("0", "1"))
	unbounded := mustCompileInline(t, existenceOPT(unboundedFromZero))
	snomed := rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "271649006"}
	noInformation := &rm.DVCodedText{Value: "no information", DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "271"}}
	mapped := &rm.DVText{Value: "text", Mappings: []rm.TermMapping{{Match: rm.Character("="), Target: snomed}}}
	tests := []struct {
		name string
		root *rm.Element
		// under is the template validated against; nil is the one that
		// prohibits the attributes.
		under *templatecompile.Compiled
		// want is the whole template-layer report under that template;
		// inside is what the template reports under the one that allows
		// the attributes.
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
			name:  "both present under an existence with no upper bound",
			root:  prohibitedElement(mapped, noInformation),
			under: unbounded,
			want: []string{
				"primitive_invalid_value /null_flavour/defining_code template invalid_value",
				"primitive_invalid_value /value/mappings[1]/target template invalid_value",
				"primitive_not_in_list /null_flavour/defining_code template not_in_list",
				"primitive_not_in_list /value/mappings[1]/target template not_in_list",
			},
			inside: []string{
				"primitive_invalid_value /null_flavour/defining_code template invalid_value",
				"primitive_invalid_value /value/mappings[1]/target template invalid_value",
				"primitive_not_in_list /null_flavour/defining_code template not_in_list",
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
			under := prohibits
			if tc.under != nil {
				under = tc.under
			}
			r := validation.Validate(tc.root, under)
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

// notAllowedChild is an OPT child of RM type rmType that occurs
// lower..upper times, with no constraint of its own.
func notAllowedChild(rmType, lower, upper string) string {
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>` + rmType + `</rm_type_name>` +
		`<occurrences>` + fiInterval(lower, upper) + `</occurrences><node_id></node_id></children>`
}

// textInList is an OPT child DV_TEXT, occurring 0..1 times, whose value
// must be "allowed".
const textInList = `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_TEXT</rm_type_name>` +
	`<occurrences>` + `<lower_included>true</lower_included><upper_included>true</upper_included>` +
	`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>1</upper>` +
	`</occurrences><node_id></node_id>` +
	`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>` +
	`<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>STRING</rm_type_name><node_id></node_id>` +
	`<item xsi:type="C_STRING"><list>allowed</list></item></children></attributes></children>`

// notAllowedArchetype is the archetype of the root of notAllowedItemsOPT.
const notAllowedArchetype = "openEHR-EHR-CLUSTER.not_allowed.v1"

// notAllowedItemsOPT returns a CLUSTER template whose items attribute holds
// the given children and no bound on its count.
func notAllowedItemsOPT(children ...string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>not_allowed</value></template_id><concept>not_allowed</concept>
<definition><rm_type_name>CLUSTER</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>` + strings.Join(children, "") + `
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` + unboundedFromZero + `</interval></cardinality>
</attributes>
<archetype_id><value>` + notAllowedArchetype + `</value></archetype_id></definition>
</template>`
}

// REQ-102 § Structural — prohibited attributes — an OPT child whose
// occurrences upper bound is 0 is not allowed, and an attribute all of whose
// children are not allowed is prohibited: present, it draws one `prohibited`
// at its path and nothing from inside. Under a single-valued attribute a
// child that is not allowed binds no value: the value binds among the
// allowed children, or fails as rm_type_mismatch when one is allowed and
// alternative_mismatch when several are, whose Detail lists the allowed
// ones alone. A multi-valued attribute with some children allowed binds its
// items as before, and the occurrences check reports an item bound to a
// child that is not allowed. An attribute with no children prohibits
// nothing.
func TestValidate_REQ102_REQ171_AttributeWithNoAllowedChild(t *testing.T) {
	t.Parallel()
	coded := &rm.DVCodedText{Value: "coded", DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at0001"}}
	element := func(nodeID string) *rm.Element {
		return &rm.Element{ArchetypeNodeID: nodeID, Name: &rm.DVText{Value: "element"}, Value: &rm.DVText{Value: "text"}}
	}
	cluster := func(items ...rm.Item) *rm.Cluster {
		return &rm.Cluster{ArchetypeNodeID: notAllowedArchetype, Name: &rm.DVText{Value: "cluster"}, Items: items}
	}
	tests := []struct {
		name   string
		root   any
		opt    string
		want   []string // the whole template-layer report
		detail string   // a substring the one issue's Detail carries, or ""
		absent string   // a substring that Detail must not carry, or ""
	}{
		{
			name: "single-valued, its one child not allowed, holding a value",
			root: valueElement(&rm.DVText{Value: "text"}),
			opt:  elementValueOPT(notAllowedChild("DV_TEXT", "0", "0")),
			want: []string{"prohibited /value template existence"},
		},
		{
			name:   "single-valued, the value's own type not allowed, one other child",
			root:   valueElement(&rm.DVText{Value: "text"}),
			opt:    elementValueOPT(notAllowedChild("DV_TEXT", "0", "0"), notAllowedChild("DV_CODED_TEXT", "0", "1")),
			want:   []string{"rm_type_mismatch /value template type"},
			detail: "template RM type DV_CODED_TEXT",
		},
		{
			name:   "single-valued, the value's own type not allowed, two other children",
			root:   valueElement(&rm.DVText{Value: "text"}),
			opt:    elementValueOPT(notAllowedChild("DV_TEXT", "0", "0"), notAllowedChild("DV_CODED_TEXT", "0", "1"), notAllowedChild("DV_QUANTITY", "0", "1")),
			want:   []string{"alternative_mismatch /value template type"},
			detail: "[DV_CODED_TEXT, DV_QUANTITY]",
			absent: "DV_TEXT,",
		},
		{
			name: "single-valued, binding the allowed child",
			root: valueElement(coded),
			opt:  elementValueOPT(notAllowedChild("DV_TEXT", "0", "0"), notAllowedChild("DV_CODED_TEXT", "0", "1")),
		},
		{
			name: "single-valued, the exact child not allowed, a looser child allowed",
			root: valueElement(coded),
			// The DV_CODED_TEXT binds the DV_TEXT child it conforms to,
			// whose value constraint it then fails.
			opt:  elementValueOPT(notAllowedChild("DV_CODED_TEXT", "0", "0"), textInList),
			want: []string{"primitive_not_in_list /value/value template not_in_list"},
		},
		{
			name: "single-valued, its one child not allowed, holding nothing",
			root: valueElement(nil),
			opt:  elementValueOPT(notAllowedChild("DV_TEXT", "0", "0")),
		},
		{
			name: "multi-valued, every child not allowed, holding an item",
			root: cluster(element("at0001")),
			opt:  notAllowedItemsOPT(fiElement("at0001", "0", "0", "", "DV_TEXT"), fiElement("at0002", "0", "0", "", "DV_TEXT")),
			want: []string{"prohibited /items template existence"},
		},
		{
			name: "multi-valued, every child not allowed, holding no item",
			root: cluster(),
			opt:  notAllowedItemsOPT(fiElement("at0001", "0", "0", "", "DV_TEXT")),
		},
		{
			name: "multi-valued, one child allowed, an item bound to the other",
			root: cluster(element("at0001")),
			opt:  notAllowedItemsOPT(fiElement("at0001", "0", "0", "", "DV_TEXT"), fiElement("at0002", "0", "1", "", "DV_TEXT")),
			want: []string{"cardinality /items[at0001] template occurrences"},
		},
		{
			name: "multi-valued, no child at all, holding an item",
			root: cluster(element("at0001")),
			opt:  notAllowedItemsOPT(),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := validation.Validate(tc.root, mustCompileInline(t, tc.opt))
			if got := templateIssues(r); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%s) template issues = %q, want %q (REQ-102 § Structural — prohibited attributes)", tc.name, got, tc.want)
			}
			if len(tc.want) > 0 && r.OK {
				t.Errorf("Validate(%s).OK = true, want false: a template issue is an error", tc.name)
			}
			for _, i := range r.Issues {
				if i.Layer != validation.LayerTemplate {
					continue
				}
				if tc.detail != "" && !strings.Contains(i.Detail, tc.detail) {
					t.Errorf("Validate(%s) %s Detail %q does not carry %q", tc.name, i.Code, i.Detail, tc.detail)
				}
				if tc.absent != "" && strings.Contains(i.Detail, tc.absent) {
					t.Errorf("Validate(%s) %s Detail %q carries %q, a child that is not allowed", tc.name, i.Code, i.Detail, tc.absent)
				}
			}
			assertIdentities(t, "Validate("+tc.name+")", r, mustBMMInvariants(t))
		})
	}
}
