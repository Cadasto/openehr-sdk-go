package instance_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// optInlineAdminEntry is an ADMIN_ENTRY the OPT defines inline, with no
// archetype id, at node id at0001 and occurrences lower..1. Its data takes
// an ITEM_TREE the OPT describes, so the entry's only undescribed content is
// itself.
func optInlineAdminEntry(lower int) string {
	return optOccurring("C_COMPLEX_OBJECT", "ADMIN_ENTRY", "at0001", lower, 1, optSingle("data", optNode("ITEM_TREE", "at0002")))
}

// sectionItems returns the members of the SECTION out.
func sectionItems(t *testing.T, out any) []rm.ContentItem {
	t.Helper()
	s, ok := out.(*rm.Section)
	if !ok {
		t.Fatalf("generated root is %T, want *rm.Section", out)
	}
	return s.Items
}

// TestREQ107_UnnamedArchetypeRootIsUndescribed is the REQ-107 Slot fills,
// Visit rule and Errors check on an OPT node of an archetype-root class for
// which the OPT names no archetype id, an ADMIN_ENTRY defined inline under a
// SECTION, which the generator reads as undescribed content, as a slot
// nothing can fill. Under Large and Medium, an attribute that is not
// required and whose members would all need it is not visited, so Generate
// returns a root with that attribute empty, which passes the RM floor and
// the template validator; Required does not visit such an attribute either.
// That holds for the SECTION's own items, and for a COMPOSITION's content
// whose optional SECTION requires its items. Where the OPT or the RM
// requires the entry, as a 1..1 child, also below a 1..1 SECTION that an
// optional content holds, which the list rule makes the content and the
// SECTION's items required for, or as the only source of a member of a
// SECTION's items or a COMPOSITION's content at existence 1..1, lists the
// RM makes non-empty once present, Generate returns ErrArchetypeIDMissing
// and no root. Both hold under every policy,
// both value fills and both compile modes.
func TestREQ107_UnnamedArchetypeRootIsUndescribed(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		// detail is what the error says after the sentinel's own text; ""
		// marks a control row, which must generate.
		detail string
		// empty, on a control row, reads the attribute that stays empty.
		empty func(t *testing.T, out any) int
	}{
		{
			name: "control: optional SECTION items over an optional inline ADMIN_ENTRY",
			opt:  optTemplate("SECTION", optListWithin("items", 0, 1, optInlineAdminEntry(0))),
			empty: func(t *testing.T, out any) int {
				t.Helper()
				return len(sectionItems(t, out))
			},
		},
		{
			name: "control: optional content over an optional SECTION whose items require an optional inline ADMIN_ENTRY",
			opt: optTemplate("COMPOSITION", optListWithin("content", 0, 1,
				optRootOccurring("SECTION", "openEHR-EHR-SECTION.adhoc.v1", 0, 1, optListWithin("items", 1, 1, optInlineAdminEntry(0))))),
			empty: func(t *testing.T, out any) int {
				t.Helper()
				comp, err := instance.AsComposition(out)
				if err != nil {
					t.Fatalf("AsComposition: %v", err)
				}
				return len(comp.Content)
			},
		},
		{
			name:   "SECTION items over a 1..1 inline ADMIN_ENTRY",
			opt:    optTemplate("SECTION", optListWithin("items", 0, 1, optInlineAdminEntry(1))),
			detail: "ADMIN_ENTRY at /items[at0001]",
		},
		{
			name: "optional content over a 1..1 SECTION whose optional items hold a 1..1 inline ADMIN_ENTRY",
			opt: optTemplate("COMPOSITION", optListWithin("content", 0, 1,
				optRootOccurring("SECTION", "openEHR-EHR-SECTION.adhoc.v1", 1, 1, optListWithin("items", 0, 1, optInlineAdminEntry(1))))),
			detail: "ADMIN_ENTRY at /content[openEHR-EHR-SECTION.adhoc.v1]/items[at0001]",
		},
		{
			name:   "SECTION items at existence 1..1 over an optional inline ADMIN_ENTRY",
			opt:    optTemplate("SECTION", optListWithin("items", 1, 1, optInlineAdminEntry(0))),
			detail: "ADMIN_ENTRY at /items[at0001]",
		},
		{
			name: "content at existence 1..1 over an optional SECTION whose items require an optional inline ADMIN_ENTRY",
			opt: optTemplate("COMPOSITION", optListWithin("content", 1, 1,
				optRootOccurring("SECTION", "openEHR-EHR-SECTION.adhoc.v1", 0, 1, optListWithin("items", 1, 1, optInlineAdminEntry(0))))),
			detail: "ADMIN_ENTRY at /content[openEHR-EHR-SECTION.adhoc.v1]/items[at0001]",
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range guardOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if tc.detail == "" {
						if err != nil {
							t.Fatalf("Generate: %v, want a root", err)
						}
						if n := tc.empty(t, out); n != 0 {
							t.Errorf("Generate wrote %d members in the attribute that holds the inline entry, want none", n)
						}
						checkValid(t, out, c)
						return
					}
					checkArchetypeIDMissing(t, "Generate", err, tc.detail)
					if out != nil {
						t.Errorf("Generate returned %T with the error, want no root", out)
					}
				})
			}
		}
	}
}

// TestREQ107_RMNonEmptyListGetsAMember is the REQ-107 What a visit writes
// check on a list the RM makes non-empty once present, other than a
// HISTORY's events, at an existence of 1..1 that only its existence would
// otherwise ask a member of: it is topped up to one member from every
// source, so it holds one, here a SECTION's items from its only child, a
// slot, which takes the RM-type-prefix fill, and an EVENT_CONTEXT's
// participations and an INSTRUCTION's activities, which the OPT leaves
// without a child, from their BMM types. The root passes the RM floor and
// the template validator under every policy, both value fills and both
// compile modes. TestREQ107_UnnamedArchetypeRootIsRefused and
// TestREQ107_SingleAttributeRMListIsReadAsTheRMList pin the lists whose only
// member would be an archetype root the OPT names no archetype id for.
func TestREQ107_RMNonEmptyListGetsAMember(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		// members reads the list under test.
		members func(t *testing.T, out any) int
		// check, when set, runs on the generated root.
		check func(t *testing.T, out any)
	}{
		{
			name: "SECTION items over a slot",
			opt:  optTemplate("SECTION", optListWithin("items", 1, 1, optSlot("OBSERVATION", "at0001"))),
			members: func(t *testing.T, out any) int {
				t.Helper()
				return len(sectionItems(t, out))
			},
			check: func(t *testing.T, out any) {
				t.Helper()
				checkSlotFallbackStamp(t, "Generate", sectionItems(t, out)[0], "OBSERVATION")
			},
		},
		{
			name: "EVENT_CONTEXT participations with no child",
			opt:  optTemplate("COMPOSITION", optSingle("context", optNode("EVENT_CONTEXT", "", optListWithin("participations", 1, 1)))),
			members: func(t *testing.T, out any) int {
				t.Helper()
				comp, err := instance.AsComposition(out)
				if err != nil {
					t.Fatalf("AsComposition: %v", err)
				}
				if comp.Context == nil {
					t.Fatal("COMPOSITION.context is absent")
				}
				return len(comp.Context.Participations)
			},
		},
		{
			name: "INSTRUCTION activities with no child",
			opt:  optTemplate("INSTRUCTION", optListWithin("activities", 1, 1)),
			members: func(t *testing.T, out any) int {
				t.Helper()
				in, ok := out.(*rm.Instruction)
				if !ok {
					t.Fatalf("generated root is %T, want *rm.Instruction", out)
				}
				return len(in.Activities)
			},
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
					if n := tc.members(t, out); n != 1 {
						t.Fatalf("Generate wrote %d members in the list, want 1", n)
					}
					if tc.check != nil {
						tc.check(t, out)
					}
					checkValid(t, out, c)
				})
			}
		}
	}
}
