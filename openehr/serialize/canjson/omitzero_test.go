package canjson_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestOmitzeroPointerFieldsEncode pins the REQ-052 / Q6 ruling that the
// generator tags a POINTER field `omitzero`, not `omitempty`. Under
// encoding/json/v2 `omitempty` omits any value that encodes empty (including a
// non-nil pointer to an empty string or empty map), whereas `omitzero` omits
// only the nil pointer. So a caller who set a pointer to a deliberately empty
// value (an explicit `""` or `{}`, distinct from "absent") keeps it on the
// wire, the v1 intent the streaming codec must preserve.
//
// No existing fixture carries a pointer to an empty value, so without this test
// the tag choice is invisible. Can-fail control: flip one pointer field's tag
// back to `,omitempty` in the generator (renderField) and regenerate: the
// non-nil-empty case below then re-encodes without the member and goes red.
func TestOmitzeroPointerFieldsEncode(t *testing.T) {
	emptyString := ""
	emptyMap := map[string]string{}

	cases := []struct {
		name     string
		value    any
		contains []string // substrings the wire MUST carry
		absent   []string // member names the wire MUST NOT carry
	}{
		{
			name:     "pointer to empty string is emitted, not omitted",
			value:    &rm.DVText{Value: "x", Formatting: &emptyString},
			contains: []string{`"formatting":""`},
		},
		{
			name:   "nil pointer is omitted",
			value:  &rm.DVText{Value: "x"},
			absent: []string{"formatting"},
		},
		{
			name:     "pointer to empty map is emitted, not omitted",
			value:    &rm.ResourceDescriptionItem{Purpose: "care", OtherDetails: &emptyMap},
			contains: []string{`"other_details":{}`},
		},
		{
			name:   "nil pointer-to-map is omitted",
			value:  &rm.ResourceDescriptionItem{Purpose: "care"},
			absent: []string{"other_details"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := canjson.Marshal(tc.value)
			if err != nil {
				t.Fatalf("Marshal(%T) error: %v", tc.value, err)
			}
			wire := string(b)
			for _, want := range tc.contains {
				if !strings.Contains(wire, want) {
					t.Errorf("Marshal(%T) = %s\n want it to contain %q (omitzero keeps a non-nil pointer to an empty value)", tc.value, wire, want)
				}
			}
			for _, name := range tc.absent {
				if strings.Contains(wire, `"`+name+`"`) {
					t.Errorf("Marshal(%T) = %s\n want member %q absent (a nil pointer is omitted)", tc.value, wire, name)
				}
			}
		})
	}
}
