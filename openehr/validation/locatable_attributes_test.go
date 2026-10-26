package validation_test

// locatable_attributes_test.go: REQ-102 — the walk reads an RM
// attribute through the access the generator emits for every registered
// class (REQ-171 § Structural access), so LOCATABLE's uid, links and
// feeder_audit on an ELEMENT are read like any other attribute: one the
// template prohibits is reported as prohibited when the instance carries it,
// and one the template requires is reported as required only when the
// instance lacks it.

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// locatableArchetype is the archetype of the root of locatableAttributesOPT.
const locatableArchetype = "openEHR-EHR-ELEMENT.locatable.v1"

// locatableAttributesOPT returns an ELEMENT template whose uid and
// feeder_audit, two single-valued attributes, and whose links, a
// multi-valued one, all have the existence lower..upper and no child. The
// links also hold from lower items up: where their existence requires them,
// their child count asks for a member too, since an existence alone asks
// for none.
func locatableAttributesOPT(lower, upper string) string {
	existence := `<existence>` + fiInterval(lower, upper) + `</existence>`
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>locatable</value></template_id><concept>locatable</concept>
<definition><rm_type_name>ELEMENT</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>uid</rm_attribute_name>` + existence + `</attributes>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>feeder_audit</rm_attribute_name>` + existence + `</attributes>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>links</rm_attribute_name>` + existence + `
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		`<lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>` + lower + `</lower>` +
		`</interval></cardinality></attributes>
<archetype_id><value>` + locatableArchetype + `</value></archetype_id></definition>
</template>`
}

// locatableElement returns an ELEMENT of locatableArchetype, carrying a uid,
// a feeder_audit and a link when full is set, and none of the three
// otherwise.
func locatableElement(full bool) *rm.Element {
	el := &rm.Element{
		ArchetypeNodeID: locatableArchetype,
		Name:            &rm.DVText{Value: "element"},
		Value:           &rm.DVBoolean{Value: true},
	}
	if full {
		el.UID = &rm.HierObjectID{Value: "8849182c-82ad-4088-a07f-48ead4180515"}
		el.FeederAudit = &rm.FeederAudit{OriginatingSystemAudit: rm.FeederAuditDetails{SystemID: "system"}}
		el.Links = []rm.Link{{
			Meaning: &rm.DVText{Value: "follow-up to"},
			Type:    &rm.DVText{Value: "issue"},
			Target:  rm.DVEHRURI{Value: "ehr:/compositions/1"},
		}}
	}
	return el
}

// REQ-102 — an ELEMENT that carries a uid, a feeder_audit and a link draws
// `prohibited` at each of the three under a template that prohibits them,
// and nothing under a template that requires them. An ELEMENT that carries
// none of them draws `required` at each under the template that requires
// them, and nothing under the one that prohibits them. Each result is the
// whole set of template-layer issues, and the RM rules alone report nothing
// on either ELEMENT.
func TestREQ110_LocatableUIDLinksAndFeederAuditAreRead(t *testing.T) {
	t.Parallel()
	prohibits := mustCompileInline(t, locatableAttributesOPT("0", "0"))
	requires := mustCompileInline(t, locatableAttributesOPT("1", "1"))
	tests := []struct {
		name     string
		full     bool // the ELEMENT carries a uid, a feeder_audit and a link
		requires bool // the template requires the three; otherwise it prohibits them
		want     []string
	}{
		{
			name: "carried, under a template that prohibits them",
			full: true,
			want: []string{
				"prohibited /feeder_audit template existence",
				"prohibited /links template existence",
				"prohibited /uid template existence",
			},
		},
		{name: "carried, under a template that requires them", full: true, requires: true},
		{
			name:     "lacked, under a template that requires them",
			requires: true,
			want: []string{
				"required /feeder_audit template existence",
				"required /links template existence",
				"required /uid template existence",
			},
		},
		{name: "lacked, under a template that prohibits them"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := prohibits
			if tc.requires {
				c = requires
			}
			el := locatableElement(tc.full)
			r := validation.Validate(el, c)
			got := templateIssues(r)
			want := slices.Clone(tc.want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("Validate(ELEMENT %s) template issues = %q, want %q", tc.name, got, want)
			}
			if floor := validation.ValidateRM(el); !floor.OK {
				t.Errorf("ValidateRM(ELEMENT %s) issues = %q, want none", tc.name, issueKeys(floor.Issues))
			}
		})
	}
}
