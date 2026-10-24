package validation_test

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// dvTextStringChild admits a DV_TEXT whose value the C_STRING body
// cString constrains.
func dvTextStringChild(cString string) string {
	return `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_TEXT</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>value</rm_attribute_name>
          <children xsi:type="C_PRIMITIVE_OBJECT">
            <rm_type_name>STRING</rm_type_name>
            <node_id></node_id>
            <item xsi:type="C_STRING">` + cString + `</item>
          </children>
        </attributes>
      </children>`
}

// REQ-102 (Validation dimensions, Primitive constraints) and REQ-103 — a
// C_STRING list with list_open true is not exhaustive, so a DV_TEXT value
// outside it draws no primitive_not_in_list, while the pattern is still
// checked. A closed list, which an OPT writes by setting list_open false or
// by leaving it out, still reports the value.
func TestValidate_REQ102_REQ103_CStringListOpen(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cString string
		value   string
		want    []pinnedIssue
	}{
		{
			name:    "open list, value outside",
			cString: `<list>Husten</list><list_open>true</list_open>`,
			value:   "Schnupfen",
		},
		{
			name:    "open list, value outside fails pattern",
			cString: `<pattern>[A-Z][a-z]+</pattern><list>Husten</list><list_open>true</list_open>`,
			value:   "schnupfen",
			want:    []pinnedIssue{templateFinding("pattern_mismatch", "/value/value")},
		},
		{
			name:    "closed list, list_open false, value outside",
			cString: `<list>Husten</list><list_open>false</list_open>`,
			value:   "Schnupfen",
			want:    []pinnedIssue{templateFinding("not_in_list", "/value/value")},
		},
		{
			name:    "closed list, list_open absent, value outside",
			cString: `<list>Husten</list>`,
			value:   "Schnupfen",
			want:    []pinnedIssue{templateFinding("not_in_list", "/value/value")},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, elementValueOPT(dvTextStringChild(tc.cString)))
			got := pinnedIssues(validation.Validate(valueElement(rm.DVText{Value: tc.value}), c).Issues)
			sortPinned(tc.want)
			if !slices.Equal(got, tc.want) {
				t.Errorf("Validate(DV_TEXT %q under C_STRING %s) issues =\n  %+v\nwant\n  %+v", tc.value, tc.cString, got, tc.want)
			}
		})
	}
}
