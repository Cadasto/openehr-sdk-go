package validation_test

// unread_attribute_test.go: REQ-110 § Known limitations — the template walk
// reads an RM attribute through the reader for its parent's class, and an
// attribute that reader does not read counts as absent. LOCATABLE's
// feeder_audit and links are such attributes on an ELEMENT. So one the
// template prohibits is not reported as prohibited, and one it requires is
// reported as required although the instance carries it. These tests record
// that known limitation: generated structural access (REQ-171) removes it,
// and their assertions flip then.

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
)

// unreadArchetype is the archetype of the root of unreadAttributeOPT.
const unreadArchetype = "openEHR-EHR-ELEMENT.unread.v1"

// unreadAttributeOPT returns an ELEMENT template whose feeder_audit, a
// single-valued attribute, and whose links, a multi-valued one, both have
// the existence lower..upper and no child.
func unreadAttributeOPT(lower, upper string) string {
	existence := `<existence>` + fiInterval(lower, upper) + `</existence>`
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>unread</value></template_id><concept>unread</concept>
<definition><rm_type_name>ELEMENT</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>feeder_audit</rm_attribute_name>` + existence + `</attributes>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>links</rm_attribute_name>` + existence + `
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		`<lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower>` +
		`</interval></cardinality></attributes>
<archetype_id><value>` + unreadArchetype + `</value></archetype_id></definition>
</template>`
}

// REQ-110 § Known limitations — an ELEMENT that carries a feeder_audit and a
// link draws no `prohibited` for either under a template that prohibits
// both, and draws `required` for both under a template that requires both:
// the reader for ELEMENT reads neither, so the template walk sees both as
// absent. When generated access reads them, the precondition below fails
// first, and the assertions flip: `prohibited` at each path under the first
// template and nothing under the second.
func TestREQ110_KnownLimitation_UnreadAttributeReadsAsAbsent(t *testing.T) {
	t.Parallel()
	el := &rm.Element{
		ArchetypeNodeID: unreadArchetype,
		Name:            &rm.DVText{Value: "element"},
		Value:           &rm.DVBoolean{Value: true},
		FeederAudit:     &rm.FeederAudit{OriginatingSystemAudit: rm.FeederAuditDetails{SystemID: "system"}},
		Links: []rm.Link{{
			Meaning: &rm.DVText{Value: "follow-up to"},
			Type:    &rm.DVText{Value: "issue"},
			Target:  rm.DVEHRURI{Value: "ehr:/compositions/1"},
		}},
	}
	if _, read := rmread.ReadSingle(el, "ELEMENT", "feeder_audit"); read {
		t.Fatal("the ELEMENT reader now reads feeder_audit: the known limitation is closed, so flip this test")
	}
	if _, read := rmread.ReadMultiple(el, "ELEMENT", "links"); read {
		t.Fatal("the ELEMENT reader now reads links: the known limitation is closed, so flip this test")
	}

	prohibits := validation.Validate(el, mustCompileInline(t, unreadAttributeOPT("0", "0")))
	if got := templateIssues(prohibits); len(got) != 0 {
		t.Errorf("Validate(ELEMENT with feeder_audit and links) under a template that prohibits both: template issues = %q, want none while the reader does not read them", got)
	}

	requires := validation.Validate(el, mustCompileInline(t, unreadAttributeOPT("1", "1")))
	want := []string{"required /feeder_audit template existence", "required /links template existence"}
	if got := templateIssues(requires); !slices.Equal(got, want) {
		t.Errorf("Validate(ELEMENT with feeder_audit and links) under a template that requires both: template issues = %q, want %q while the reader does not read them", got, want)
	}
}
