package instance_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// usedObservationID is the archetype every OBSERVATION use in these cases
// names.
const usedObservationID = "openEHR-EHR-OBSERVATION.used.v1"

// optPinnedName is the name attribute of a node whose name the template
// fixes to name.
func optPinnedName(name string) string {
	return optSingle("name", optNode("DV_TEXT", "", optSingle("value", optPrimitive("STRING", "C_STRING", "<list>"+name+"</list>"))))
}

// optObservationUse is one use of usedObservationID: an OBSERVATION
// archetype root with occurrences lower..upper (upper -1 is unbounded),
// that pins name, or no name when name is empty. It describes its
// RM-required data, a HISTORY at0001, so it needs no content the template
// does not describe.
func optObservationUse(name string, lower, upper int) string {
	attrs := []string{optSingle("data", optNode("HISTORY", "at0001"))}
	if name != "" {
		attrs = append(attrs, optPinnedName(name))
	}
	return optRootOccurring("OBSERVATION", usedObservationID, lower, upper, attrs...)
}

// optUsesTemplate is a COMPOSITION template whose content, at cardinality
// 0..*, holds uses.
func optUsesTemplate(uses ...string) string {
	return optTemplate("COMPOSITION", optCardinal("content", 0, -1, uses...))
}

// optTwinElements is a COMPOSITION template whose context's other_context
// holds an ITEM_TREE with two optional ELEMENTs that share the node id
// at0002 and pin no name: the first takes a DV_TEXT, the second a
// DV_COUNT.
func optTwinElements() string {
	twin := func(valueType string) string {
		return optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0002", 0, 1, optSingle("value", optNode(valueType, "")))
	}
	tree := optNode("ITEM_TREE", "at0001", optCardinal("items", 0, -1, twin("DV_TEXT"), twin("DV_COUNT")))
	return optTemplate("COMPOSITION", optSingle("context", optNode("EVENT_CONTEXT", "", optOptionalSingleOver("other_context", tree))))
}

// usesOptions are the settings a uses case runs under policy, one per
// value fill.
func usesOptions(policy instance.Policy) []instance.Options {
	return corpusOptions(policy)
}

// contentNames returns the name of each member of comp's content, in
// order.
func contentNames(comp *rm.Composition) []string {
	names := make([]string, 0, len(comp.Content))
	for _, item := range comp.Content {
		loc, ok := item.(rm.Locatable)
		if !ok {
			names = append(names, fmt.Sprintf("%T", item))
			continue
		}
		names = append(names, loc.GetName().GetValue())
	}
	return names
}

// otherContextItems returns the members of comp's other_context, an
// ITEM_TREE, or nil when it holds none.
func otherContextItems(t *testing.T, comp *rm.Composition) []rm.Item {
	t.Helper()
	if comp.Context == nil || comp.Context.OtherContext == nil {
		return nil
	}
	tree, ok := comp.Context.OtherContext.(*rm.ItemTree)
	if !ok {
		t.Fatalf("other_context is %T, want *rm.ItemTree", comp.Context.OtherContext)
	}
	return tree.Items
}

// TestREQ107_LargeWritesEveryNamedUseOfAnArchetype is the REQ-107 check of
// What a visit writes that, under Large, each optional use of one archetype
// that the template tells apart by a pinned name gets its member, in OPT
// order, while under Medium the uses after the first get none. Both roots
// pass the template validator, which binds each member by its name
// (REQ-102 Structural — binding items), and the RM floor, under both
// value fills.
func TestREQ107_LargeWritesEveryNamedUseOfAnArchetype(t *testing.T) {
	c := compileOPTText(t, optUsesTemplate(optObservationUse("Erste", 0, 1), optObservationUse("Zweite", 0, 1)), true)
	for policy, want := range map[instance.Policy][]string{
		instance.Large:  {"Erste", "Zweite"},
		instance.Medium: {"Erste"},
	} {
		for _, opts := range usesOptions(policy) {
			t.Run(fmt.Sprintf("%v/%v", policy, opts.ValueFill), func(t *testing.T) {
				comp := generateComposition(t, c, opts)
				if got := contentNames(comp); !slices.Equal(got, want) {
					t.Errorf("Generate(%v) content names = %q, want %q", policy, got, want)
				}
				noValidationErrors(t, comp, c)
				noFloorErrors(t, comp)
			})
		}
	}
}

// TestREQ107_REQ102_LargeWritesEveryUnnamedSiblingOfOneIdentity is the
// REQ-107 check of What a visit writes that, under Large, each optional
// sibling of one identity that pins no name gets its member, where under
// Medium the siblings after the first get none: two optional OBSERVATION
// roots of one archetype, the first allowing one occurrence or any number,
// and two optional ELEMENTs of one node id whose values differ in type.
// The template validator binds each member to the first sibling with room
// under whose constraints it validates (REQ-102 Structural — binding
// items), so both roots pass it, and the RM floor, under both value fills.
func TestREQ107_REQ102_LargeWritesEveryUnnamedSiblingOfOneIdentity(t *testing.T) {
	cases := []struct {
		name  string
		opt   string
		count func(t *testing.T, comp *rm.Composition) int
	}{
		{
			name:  "two roots of one occurrence",
			opt:   optUsesTemplate(optObservationUse("", 0, 1), optObservationUse("", 0, 1)),
			count: func(_ *testing.T, comp *rm.Composition) int { return len(comp.Content) },
		},
		{
			name:  "an open root before a root of one occurrence",
			opt:   optUsesTemplate(optObservationUse("", 0, -1), optObservationUse("", 0, 1)),
			count: func(_ *testing.T, comp *rm.Composition) int { return len(comp.Content) },
		},
		{
			name:  "two ELEMENTs of one node id",
			opt:   optTwinElements(),
			count: func(t *testing.T, comp *rm.Composition) int { return len(otherContextItems(t, comp)) },
		},
	}
	for _, tc := range cases {
		c := compileOPTText(t, tc.opt, true)
		for policy, want := range map[instance.Policy]int{instance.Large: 2, instance.Medium: 1} {
			for _, opts := range usesOptions(policy) {
				t.Run(fmt.Sprintf("%s/%v/%v", tc.name, policy, opts.ValueFill), func(t *testing.T) {
					comp := generateComposition(t, c, opts)
					if got := tc.count(t, comp); got != want {
						t.Errorf("Generate(%v) built %d siblings of one identity, want %d", policy, got, want)
					}
					noValidationErrors(t, comp, c)
					noFloorErrors(t, comp)
				})
			}
		}
	}
}

// TestREQ107_REQ102_LargeFillsAnOptionalSiblingBeforeARequiredOne is the
// REQ-107 check, beside Exceptions and known gaps, that under Large an
// optional sibling of one occurrence before a required one of its
// identity, neither pinning a name, gets a member of its own: the member
// fills it, so the validator binds the required sibling's member to the
// required sibling (REQ-102 Structural — binding items), and the root
// validates. Under Medium the optional sibling gets none and the gap stays
// (TestGenerate_REQ107_REQ102_OptionalSiblingBeforeARequiredOneKeepsItsGap).
func TestREQ107_REQ102_LargeFillsAnOptionalSiblingBeforeARequiredOne(t *testing.T) {
	c := compileOPTText(t, optUsesTemplate(optObservationUse("", 0, 1), optObservationUse("", 1, 1)), true)
	for _, opts := range usesOptions(instance.Large) {
		t.Run(opts.ValueFill.String(), func(t *testing.T) {
			comp := generateComposition(t, c, opts)
			if len(comp.Content) != 2 {
				t.Fatalf("Generate(Large) built %d content items, want one per sibling", len(comp.Content))
			}
			noValidationErrors(t, comp, c)
			noFloorErrors(t, comp)
		})
	}
}

// TestREQ107_REQ102_LargeKeepsTheGapOfAnOpenOptionalSibling is the REQ-107
// Exceptions and known gaps check that, under Large, an optional sibling
// that allows any number of occurrences before a required one of its
// identity, neither pinning a name, still takes the required sibling's
// member: it gets a member of its own and keeps room for another, so the
// validator binds the second member there too (REQ-102 Structural —
// binding items), and the root fails by the required sibling's
// occurrences.
func TestREQ107_REQ102_LargeKeepsTheGapOfAnOpenOptionalSibling(t *testing.T) {
	c := compileOPTText(t, optUsesTemplate(optObservationUse("", 0, -1), optObservationUse("", 1, 1)), true)
	comp := generateComposition(t, c, corpusOptions(instance.Large)[0])
	if len(comp.Content) != 2 {
		t.Fatalf("Generate(Large) built %d content items, want one per sibling", len(comp.Content))
	}
	r := validation.ValidateComposition(comp, c)
	want := []string{"cardinality /content[" + usedObservationID + "]"}
	var got []string
	for _, iss := range r.Issues {
		got = append(got, iss.Code+" "+iss.Path)
	}
	if !slices.Equal(got, want) || !strings.Contains(r.Issues[0].Detail, "appears 0 times") {
		t.Errorf("ValidateComposition(generated) issues = %q (%v), want %q: the required sibling appears 0 times", got, r.Issues, want)
	}
}
