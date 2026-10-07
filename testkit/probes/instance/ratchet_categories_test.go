package instanceprobes_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

func categoryElement() *rm.Element {
	return &rm.Element{
		ArchetypeNodeID: "at0001",
		Name:            rm.DVText{Value: "item"},
	}
}

func categoryNullFlavour() *rm.DVCodedText {
	return &rm.DVCodedText{
		Value: "unknown",
		DefiningCode: rm.CodePhrase{
			CodeString:    "253",
			TerminologyID: rm.TerminologyID{Value: "openehr"},
		},
	}
}

// ratchetCategories runs ValidateRM over root and returns the sorted
// category of every finding at the given code, as issueReason keys it.
func ratchetCategories(root any, code string) []string {
	var cats []string
	for _, iss := range validation.ValidateRM(root).Issues {
		if iss.Code != code {
			continue
		}
		cat, _, _ := strings.Cut(issueReason(iss, instance.ExampleFill), ":")
		cats = append(cats, cat)
	}
	slices.Sort(cats)
	return cats
}

// TestREQ112_RatchetKeysFloorRules pins the census categories for the two
// rm_invariant rules of the REQ-112 catalogue that the generator no longer
// emits (PROBE-027), so a regression of either keeps a precise key.
func TestREQ112_RatchetKeysFloorRules(t *testing.T) {
	both := categoryElement()
	both.Value = &rm.DVText{Value: "x"}
	both.NullFlavour = categoryNullFlavour()

	neither := categoryElement()

	badTime := categoryElement()
	badTime.Value = &rm.DVDateTime{Value: "example"}

	cases := []struct {
		name string
		root any
		want []string
	}{
		{"element with value and null_flavour", both, []string{reasonFloorElementValue}},
		{"element with neither", neither, []string{reasonFloorElementValue}},
		{"element with placeholder date-time", badTime, []string{reasonFloorTemporal}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ratchetCategories(tc.root, "rm_invariant")
			if !slices.Equal(got, tc.want) {
				t.Errorf("rm_invariant categories = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestREQ112_RatchetKeysCodedInvariants pins the census category of the
// REQ-112 coded invariants: a code outside the group or code set an RM rule
// names keys as floor_coded, never as validator_other, so a row the
// generator adds there names its rule family.
func TestREQ112_RatchetKeysCodedInvariants(t *testing.T) {
	noValue := categoryElement()
	noValue.NullFlavour = categoryNullFlavour()
	noValue.NullFlavour.DefiningCode.CodeString = "999"

	charset := rm.CodePhrase{CodeString: "UTF-99", TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}}
	text := categoryElement()
	text.Value = &rm.DVText{Value: "x", Encoding: &charset}

	cases := []struct {
		name string
		root any
		want []string
	}{
		{"null_flavour outside the null flavours group", noValue, []string{reasonFloorCoded}},
		{"DV_TEXT encoding outside the character sets", text, []string{reasonFloorCoded}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ratchetCategories(tc.root, "code_not_in_value_set"); !slices.Equal(got, tc.want) {
				t.Errorf("code_not_in_value_set categories = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestREQ107_HollowBodyFloor pins the coverage floor: a body with no ELEMENT
// is reported, and a body with one is not.
func TestREQ107_HollowBodyFloor(t *testing.T) {
	hollow := &rm.Composition{}
	got, err := bodyReasons(hollow)
	if err != nil {
		t.Fatalf("bodyReasons(hollow): %v", err)
	}
	if _, ok := got[reasonHollowBody+":/"]; !ok {
		t.Errorf("bodyReasons(hollow) = %v, want a %s finding", got, reasonHollowBody)
	}

	el := categoryElement()
	el.Value = &rm.DVText{Value: "x"}
	got, err = bodyReasons(&rm.ItemTree{Items: []rm.Item{el}})
	if err != nil {
		t.Fatalf("bodyReasons(with element): %v", err)
	}
	if _, ok := got[reasonHollowBody+":/"]; ok {
		t.Errorf("bodyReasons(with element) = %v, want no %s finding", got, reasonHollowBody)
	}
}

// archetypeIDMissingOPT is a COMPOSITION template whose required content
// attribute names no child, so Generate refuses the OBSERVATION it would
// build there.
const archetypeIDMissingOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>archetype_id_missing</value></template_id><concept>archetype_id_missing</concept>
<definition><rm_type_name>COMPOSITION</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>content</rm_attribute_name>
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>1</lower></interval></cardinality>
</attributes>
<archetype_id><value>openEHR-EHR-COMPOSITION.encounter.v1</value></archetype_id></definition>
</template>`

// TestREQ107_RatchetKeysArchetypeIDMissing pins the census key for the
// generator's refusal of an archetype root the template names no archetype
// for (REQ-107): the refusal category, then archetype_id_missing and a
// locator. The locator is the OPT path the error names, whatever note
// follows the path and whether or not the builder wraps the error; for a
// required attribute the template leaves without children it is the path
// of that attribute, so the refusal keys apart from one at the node that
// holds the attribute, such as the template root.
func TestREQ107_RatchetKeysArchetypeIDMissing(t *testing.T) {
	const spaced = "/content[openEHR-EHR-SECTION.adhoc.v1,'Allgemeine Angaben (A)']/items[at0001]"
	missing := instance.ErrArchetypeIDMissing
	key := func(locator string) string { return reasonRefusalOther + ":archetype_id_missing:" + locator }
	cases := []struct {
		name    string
		err     error
		locator string
	}{
		{"node without an archetype id", fmt.Errorf("%w: OBSERVATION at /content[at0000]", missing), "/content[at0000]"},
		{"abstract node", fmt.Errorf("%w: CONTENT_ITEM (built as OBSERVATION) at /content[at0001]", missing), "/content[at0001]"},
		{"name predicate with a space", fmt.Errorf("%w: OBSERVATION at %s", missing, spaced), spaced},
		{
			"required attribute of a nested node",
			fmt.Errorf("%w: OBSERVATION for SECTION.items at %s (required, but the template names no child)", missing, spaced),
			spaced + "/items",
		},
		{
			"required attribute of the template root",
			fmt.Errorf("%w: OBSERVATION for COMPOSITION.content at / (required, but the template names no child)", missing),
			"/content",
		},
		{"template root", fmt.Errorf("%w: COMPOSITION at / (the template root)", missing), "/"},
		{"wrapped by the builder", fmt.Errorf("composition.NewSkeleton: %w", fmt.Errorf("%w: OBSERVATION at /content[at0000]", missing)), "/content[at0000]"},
	}
	got := map[string]string{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got[tc.name] = generateReason(t, tc.err)
			if want := key(tc.locator); got[tc.name] != want {
				t.Errorf("generateReason(%q) = %q, want %q", tc.err, got[tc.name], want)
			}
		})
	}
	// Distinct refusals at one node keep distinct keys.
	for _, pair := range [][2]string{
		{"template root", "required attribute of the template root"},
		{"name predicate with a space", "required attribute of a nested node"},
	} {
		if got[pair[0]] == got[pair[1]] {
			t.Errorf("%s and %s share the key %q, want distinct keys", pair[0], pair[1], got[pair[0]])
		}
	}

	t.Run("refusal from Generate", func(t *testing.T) {
		opt, err := template.ParseOPT(strings.NewReader(archetypeIDMissingOPT))
		if err != nil {
			t.Fatalf("ParseOPT: %v", err)
		}
		c, err := templatecompile.Compile(opt)
		if err != nil {
			t.Fatalf("Compile: %v", err)
		}
		_, err = instance.Generate(t.Context(), c, instance.Options{Territory: "NL", Composer: testComposer()})
		if !errors.Is(err, missing) {
			t.Fatalf("Generate error = %v, want one wrapping ErrArchetypeIDMissing", err)
		}
		if got, want := generateReason(t, err), key("/content"); got != want {
			t.Errorf("generateReason(%q) = %q, want %q", err, got, want)
		}
	})
}
