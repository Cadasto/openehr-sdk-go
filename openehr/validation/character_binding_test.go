package validation_test

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// termMappingMatchChild is an OPT child of RM type DV_TEXT whose mappings
// hold one TERM_MAPPING, and whose match is the AOM 1.4 STRING node a
// C_STRING with body constrains. AOM 1.4 has no C_CHARACTER, so a C_STRING
// is the one way an OPT constrains the BMM Character a match is.
func termMappingMatchChild(body string) string {
	return `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_TEXT</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
          <rm_attribute_name>mappings</rm_attribute_name>
          <children xsi:type="C_COMPLEX_OBJECT">
            <rm_type_name>TERM_MAPPING</rm_type_name>
            <node_id></node_id>
            <attributes xsi:type="C_SINGLE_ATTRIBUTE">
              <rm_attribute_name>match</rm_attribute_name>` + shortNameChild("STRING", "C_STRING", body) + `
            </attributes>
          </children>
          <cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>
            <lower_included>true</lower_included><lower_unbounded>false</lower_unbounded>
            <upper_unbounded>true</upper_unbounded><lower>1</lower></interval></cardinality>
        </attributes>
      </children>`
}

// mappedText is a DV_TEXT holding one mapping whose match is match.
func mappedText(match rm.Character) *rm.DVText {
	return &rm.DVText{
		Value: "asthma",
		Mappings: []rm.TermMapping{{
			Match:  match,
			Target: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ICD10"}, CodeString: "J45"},
		}},
	}
}

// REQ-102 (Primitive constraints) — a TERM_MAPPING.match is a BMM Character,
// an rm.Character, and an OPT constrains it with a C_STRING on an AOM 1.4
// STRING node. The node binds the match as the one-character string it is,
// so a match the C_STRING admits validates, and one it refuses is the
// constraint's own finding at the match, never an rm_type_mismatch.
func TestValidate_REQ102_CharacterBindsUnderAString(t *testing.T) {
	t.Parallel()
	const match = "/value/mappings[1]/match"
	tests := []struct {
		name  string
		body  string
		match rm.Character
		want  []pinnedIssue
	}{
		{name: "open C_STRING", body: "", match: "?"},
		{name: "list holding the match", body: "<list>=</list>", match: "="},
		{name: "list of a character and a word, the character", body: "<list>broader</list><list>&gt;</list>", match: ">"},
		{name: "pattern admitting the match", body: "<pattern>[&lt;&gt;]</pattern>", match: "<"},
		{name: "list not holding the match", body: "<list>=</list>", match: "<", want: []pinnedIssue{templateFinding("not_in_list", match)}},
		{name: "pattern refusing the match", body: "<pattern>[&lt;&gt;]</pattern>", match: "=", want: []pinnedIssue{templateFinding("pattern_mismatch", match)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, elementValueOPT(termMappingMatchChild(tc.body)))
			got := pinnedIssues(validation.Validate(valueElement(mappedText(tc.match)), c).Issues)
			if !slices.Equal(got, tc.want) {
				t.Errorf("Validate(match %q under C_STRING %q) issues =\n  %+v\nwant\n  %+v", tc.match, tc.body, got, tc.want)
			}
		})
	}
}
