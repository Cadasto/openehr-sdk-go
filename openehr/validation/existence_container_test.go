package validation_test

// existence_container_test.go: REQ-102 § Validation dimensions, Structural —
// required attributes, and § An existence lower bound alone asks for no
// member. On a multi-valued attribute the template's existence asks for the
// container and its cardinality for the members, and canonical JSON leaves a
// list with no member out, so the template reports `required` for a list that
// is absent, or present and empty, only where its cardinality lower bound is
// bounded and at least 1. A single-valued attribute keeps its reading, and
// the RM layer still reports a container the Reference Model requires.

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// fromLowerUnbounded is an AOM interval body from lower with no upper bound:
// the upper is marked unbounded and carries no value.
func fromLowerUnbounded(lower string) string {
	return `<lower_included>true</lower_included><upper_included>false</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>` + lower + `</lower>`
}

// containerArchetype is the archetype of the root of containerOPT, of rmType.
func containerArchetype(rmType string) string {
	return "openEHR-EHR-" + rmType + ".existence_container.v1"
}

// containerElement is an ELEMENT child at0001, occurring lower..1 times,
// whose value the template requires (existence 1..1) as a DV_TEXT.
func containerElement(lower string) string {
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ELEMENT</rm_type_name>` +
		`<occurrences>` + fiInterval(lower, "1") + `</occurrences><node_id>at0001</node_id>` +
		`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>` +
		`<existence>` + fiInterval("1", "1") + `</existence>` +
		`<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_TEXT</rm_type_name><node_id></node_id></children>` +
		`</attributes></children>`
}

// containerOPT returns a template whose root, of rmType, carries items with
// existence 1..1 and the cardinality interval body cardinality, left out of
// the OPT where it is empty, admitting children.
func containerOPT(rmType, cardinality string, children ...string) string {
	card := ""
	if cardinality != "" {
		card = `<cardinality><is_ordered>true</is_ordered><is_unique>false</is_unique><interval>` + cardinality + `</interval></cardinality>`
	}
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>existence_container</value></template_id><concept>existence_container</concept>
<definition><rm_type_name>` + rmType + `</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>
<existence>` + fiInterval("1", "1") + `</existence>` + strings.Join(children, "") + card + `
</attributes>
<archetype_id><value>` + containerArchetype(rmType) + `</value></archetype_id></definition>
</template>`
}

// noInformationElement is an ELEMENT at0001 with no value, whose null
// flavour says why, so the RM's own rule on ELEMENT holds.
func noInformationElement() *rm.Element {
	return &rm.Element{
		ArchetypeNodeID: "at0001",
		Name:            rm.DVText{Value: "element"},
		NullFlavour: &rm.DVCodedText{
			DVText:       rm.DVText{Value: "no information"},
			DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "271"},
		},
	}
}

// REQ-102 — a list whose existence lower bound is 1 is asked for as a
// container only: absent, or present and empty, it draws no `required` from
// the template unless its cardinality lower bound is bounded and at least 1,
// whether the template admits a child under it or not, and whether it states
// a cardinality or not. Where the cardinality lower bound is 1, an absent
// list draws `required` alone (no `cardinality`, no `occurrences` of the
// child it requires, REQ-171 § Structural access) and a present, empty one
// both `required` and `cardinality`, with that child's occurrences beside
// them. A single-valued attribute at existence 1..1
// that is absent still draws `required`, and the RM layer still reports a
// container the Reference Model requires (CLUSTER.items).
func TestValidate_REQ102_ListExistenceAsksForTheContainerOnly(t *testing.T) {
	t.Parallel()
	zeroOrMore := mustCompileInline(t, containerOPT("ITEM_TREE", fromLowerUnbounded("0"), containerElement("0")))
	noChild := mustCompileInline(t, containerOPT("ITEM_TREE", fromLowerUnbounded("0")))
	noCardinality := mustCompileInline(t, containerOPT("ITEM_TREE", "", containerElement("0")))
	oneOrMore := mustCompileInline(t, containerOPT("ITEM_TREE", fromLowerUnbounded("1"), containerElement("1")))
	cluster := mustCompileInline(t, containerOPT("CLUSTER", fromLowerUnbounded("0"), containerElement("0")))
	tree := func(items []rm.Item) *rm.ItemTree {
		return &rm.ItemTree{ArchetypeNodeID: containerArchetype("ITEM_TREE"), Name: rm.DVText{Value: "tree"}, Items: items}
	}
	tests := []struct {
		name string
		root rm.Locatable
		// under is the template validated against.
		under *templatecompile.Compiled
		// want is the whole report, as identityKeys writes it.
		want []string
	}{
		{name: "absent list, cardinality 0..*", root: tree(nil), under: zeroOrMore},
		{name: "present empty list, cardinality 0..*", root: tree([]rm.Item{}), under: zeroOrMore},
		{name: "absent list, no child", root: tree(nil), under: noChild},
		{name: "present empty list, no child", root: tree([]rm.Item{}), under: noChild},
		{name: "absent list, no cardinality stated", root: tree(nil), under: noCardinality},
		{name: "present empty list, no cardinality stated", root: tree([]rm.Item{}), under: noCardinality},
		{
			name:  "absent list, cardinality 1..*",
			root:  tree(nil),
			under: oneOrMore,
			want:  []string{"required /items template existence"},
		},
		{
			name:  "present empty list, cardinality 1..*",
			root:  tree([]rm.Item{}),
			under: oneOrMore,
			want: []string{
				"cardinality /items template cardinality",
				"cardinality /items[at0001] template occurrences",
				"required /items template existence",
			},
		},
		{
			name:  "absent single-valued attribute at existence 1..1",
			root:  tree([]rm.Item{noInformationElement()}),
			under: zeroOrMore,
			want:  []string{"required /items[at0001]/value template existence"},
		},
		{
			name:  "absent RM-mandatory list, cardinality 0..*",
			root:  &rm.Cluster{ArchetypeNodeID: containerArchetype("CLUSTER"), Name: rm.DVText{Value: "cluster"}},
			under: cluster,
			want:  []string{"required /items rm mandatory"},
		},
		{
			name:  "present empty RM-mandatory list, cardinality 0..*",
			root:  &rm.Cluster{ArchetypeNodeID: containerArchetype("CLUSTER"), Name: rm.DVText{Value: "cluster"}, Items: []rm.Item{}},
			under: cluster,
			want:  []string{"cardinality /items rm cardinality"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := validation.Validate(tc.root, tc.under)
			if got := identityKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%s) issues = %q, want %q (REQ-102 § An existence lower bound alone asks for no member)", tc.name, got, tc.want)
			}
		})
	}
}

// REQ-102 — the vendored vital_signs.json, demo data, carries no state or
// protocol items where vital_signs.opt gives those lists existence 1..1 and
// cardinality 0..* (or 0..1): the template asks for no member there, so the
// sample draws only its magnitude and units issues.
func TestValidateComposition_REQ102_CorpusVitalSignsListsWithNoMember(t *testing.T) {
	t.Parallel()
	c := mustCompile(t, "vital_signs")
	raw, err := os.ReadFile(fixtures.CompositionJSON("vital_signs"))
	if err != nil {
		t.Fatalf("read vital_signs.json: %v", err)
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(raw, &comp); err != nil {
		t.Fatalf("canjson.Unmarshal(vital_signs.json): %v", err)
	}
	const bp = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items"
	const temp = "/content[openEHR-EHR-OBSERVATION.body_temperature.v1]/data/events[at0003]/data/item/value"
	want := []string{
		"primitive_out_of_range " + bp + "[at0004]/value template out_of_range",
		"primitive_unit_unknown " + bp + "[at0005]/value template unit_unknown",
		"primitive_unit_unknown " + temp + " template unit_unknown",
	}
	r := validation.ValidateComposition(&comp, c)
	if got := identityKeys(r.Issues); !slices.Equal(got, want) {
		t.Errorf("ValidateComposition(vital_signs.json) issues = %q, want %q", got, want)
	}
}
