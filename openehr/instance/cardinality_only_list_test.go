package instance_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
)

// compositionContent returns the number of members of the COMPOSITION out's
// content.
func compositionContent(t *testing.T, out any) int {
	t.Helper()
	comp, err := instance.AsComposition(out)
	if err != nil {
		t.Fatalf("AsComposition: %v", err)
	}
	return len(comp.Content)
}

// TestREQ107_CardinalityOnlyListOverUndescribedContentIsLeftOut is the
// REQ-107 Visit rule check on a list that only its cardinality lower bound
// requires, at existence 0..1 or with no existence and cardinality 1..*,
// whose members would all be undescribed or need undescribed content: an
// optional ADMIN_ENTRY the OPT names no archetype id for, an optional
// SECTION whose items require one, the fill of a slot, or an ELEMENT built
// from the BMM type of a list with no allowed child. A cardinality lower
// bound does not ask for the container, so
// under Large and Medium the rule on undescribed content leaves the list out,
// as Required does, and Generate returns a root with the list empty, which
// passes the RM floor and the template validator. A list with a described
// child beside the entry is still visited under Large and Medium, which give
// it the described member, and left out under Required. All of it holds
// under both value fills and both compile modes.
func TestREQ107_CardinalityOnlyListOverUndescribedContentIsLeftOut(t *testing.T) {
	sectionWithEntry := optRootOccurring("SECTION", "openEHR-EHR-SECTION.adhoc.v1", 0, 1,
		optListWithin("items", 0, 1, optInlineAdminEntry(1)))
	cases := []struct {
		name string
		opt  string
		// members reads the list under test.
		members func(t *testing.T, out any) int
		// described is the number of members Large and Medium give the
		// list; Required gives it none.
		described int
	}{
		{
			name:    "SECTION items at existence 0..1 and cardinality 1..* over an optional inline ADMIN_ENTRY",
			opt:     optTemplate("SECTION", optList("items", 0, 1, optInlineAdminEntry(0))),
			members: func(t *testing.T, out any) int { t.Helper(); return len(sectionItems(t, out)) },
		},
		{
			name:    "SECTION items with no existence and cardinality 1..* over an optional inline ADMIN_ENTRY",
			opt:     optTemplate("SECTION", optCardinal("items", 1, -1, optInlineAdminEntry(0))),
			members: func(t *testing.T, out any) int { t.Helper(); return len(sectionItems(t, out)) },
		},
		{
			name:    "COMPOSITION content at existence 0..1 and cardinality 1..* over an optional SECTION that requires an inline ADMIN_ENTRY",
			opt:     optTemplate("COMPOSITION", optList("content", 0, 1, sectionWithEntry)),
			members: compositionContent,
		},
		{
			name:    "COMPOSITION content with no existence and cardinality 1..* over an optional SECTION that requires an inline ADMIN_ENTRY",
			opt:     optTemplate("COMPOSITION", optCardinal("content", 1, -1, sectionWithEntry)),
			members: compositionContent,
		},
		{
			name:    "ITEM_TREE items at existence 0..1 and cardinality 1..* over a slot",
			opt:     optTemplate("ITEM_TREE", optList("items", 0, 1, optSlot("CLUSTER", "at0001"))),
			members: func(t *testing.T, out any) int { t.Helper(); return len(itemIDs(t, out)) },
		},
		{
			name:    "ITEM_TREE items at existence 0..1 and cardinality 1..* with no child",
			opt:     optTemplate("ITEM_TREE", optList("items", 0, 1)),
			members: func(t *testing.T, out any) int { t.Helper(); return len(itemIDs(t, out)) },
		},
		{
			name: "control: SECTION items at existence 0..1 and cardinality 1..* over an optional inline ADMIN_ENTRY and an optional SECTION",
			opt: optTemplate("SECTION", optList("items", 0, 1,
				optInlineAdminEntry(0), optSectionChild("at0002", 0, 1))),
			members:   func(t *testing.T, out any) int { t.Helper(); return len(sectionItems(t, out)) },
			described: 1,
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range guardOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v, want a root", err)
					}
					want := tc.described
					if opts.Policy == instance.Required {
						want = 0
					}
					if n := tc.members(t, out); n != want {
						t.Errorf("Generate wrote %d members in the list, want %d", n, want)
					}
					checkValid(t, out, c)
				})
			}
		}
	}
}
