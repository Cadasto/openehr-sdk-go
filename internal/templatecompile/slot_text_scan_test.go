package templatecompile_test

// slot_text_scan_test.go: how Compile reads the archetype-id pattern of a
// slot's text form where runes in the text take more bytes lowercased than
// as written.

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
)

// TestREQ108_CompileReadsSlotTextAfterLengtheningRunes pins that Compile
// reads the pattern of a slot include's `archetype_id … matches {…}` text,
// the two words in any ASCII case, where runes before, between or inside
// them take more bytes lowercased than as written, as Ⱥ (U+023A, two bytes)
// does lowercased to ⱥ (U+2C65, three bytes): it does not panic, and it reads
// the pattern from its own place in the text.
func TestREQ108_CompileReadsSlotTextAfterLengtheningRunes(t *testing.T) {
	lengthening := strings.Repeat("Ⱥ", 30)
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "lengthening runes before the match",
			text: lengthening + " archetype_id/value matches {/.*/}",
			want: []string{".*"},
		},
		{
			name: "lengthening runes between the two words",
			text: "archetype_id/value " + lengthening + " matches {/openEHR-EHR-CLUSTER\\.device\\.v1/}",
			want: []string{`openEHR-EHR-CLUSTER\.device\.v1`},
		},
		{
			name: "the two words in capitals, lengthening runes in the pattern too",
			text: lengthening + " ARCHETYPE_ID/value MATCHES {/openEHR-EHR-CLUSTER\\.Ⱥ\\.v1/}",
			want: []string{`openEHR-EHR-CLUSTER\.Ⱥ\.v1`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			slot := &aom14.ArchetypeSlot{
				RMTypeName:  "SECTION",
				NodeID:      "at0001",
				Occurrences: once,
				Includes:    []aom14.Assertion{{StringExpression: new(tc.text)}},
			}
			c, err := templatecompile.Compile(modelOf(multiple("content", slot)))
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			node, err := c.NodeAt("/content[at0001]")
			if err != nil {
				t.Fatalf("NodeAt(/content[at0001]): %v", err)
			}
			if got := node.SlotIncludes(); !slices.Equal(got, tc.want) {
				t.Errorf("SlotIncludes() of a slot whose include reads %q = %q; want %q", tc.text, got, tc.want)
			}
		})
	}
}
