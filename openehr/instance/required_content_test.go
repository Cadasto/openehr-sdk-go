package instance_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// entryShape renders the archetype node ids of a COMPOSITION's content, or
// of a SECTION root's items, in order: each SECTION followed by its own
// items in parentheses, and each entry followed, in braces, by the parts
// that tell its mandatory parts from its optional ones. For an OBSERVATION
// that is "protocol" where it has one; for an EVALUATION, the node ids of
// the items of its ITEM_TREE data.
func entryShape(v any) string {
	switch n := v.(type) {
	case *rm.Composition:
		return entriesShape(n.Content)
	case *rm.Section:
		return entriesShape(n.Items)
	}
	return fmt.Sprintf("%T", v)
}

func entriesShape[T any](items []T) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = nodeID(item)
		switch n := any(item).(type) {
		case *rm.Section:
			parts[i] += "(" + entriesShape(n.Items) + ")"
		case *rm.Observation:
			parts[i] += "{"
			if n.Protocol != nil && !rm.IsTypedNil(n.Protocol) {
				parts[i] += "protocol"
			}
			parts[i] += "}"
		case *rm.Evaluation:
			var ids []string
			if tree, ok := n.Data.(*rm.ItemTree); ok {
				for _, it := range tree.Items {
					ids = append(ids, nodeID(it))
				}
			}
			parts[i] += "{" + strings.Join(ids, ",") + "}"
		}
	}
	return strings.Join(parts, ",")
}

// requiredContentCase is a template and the shape (entryShape) each policy
// builds of it.
type requiredContentCase struct {
	name string
	opt  string
	want map[instance.Policy]string
}

// runRequiredContentCases generates each case under every policy, both
// value fills and both compile modes, and checks the shape and that the
// root passes the RM floor and the template validator.
func runRequiredContentCases(t *testing.T, cases []requiredContentCase) {
	t.Helper()
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range guardOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					if got := entryShape(out); got != tc.want[opts.Policy] {
						t.Errorf("Generate built %q, want %q", got, tc.want[opts.Policy])
					}
					noFloorErrors(t, out)
					for _, iss := range templateErrors(out, c) {
						t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
					}
				})
			}
		}
	}
}

const (
	requiredObsA    = "openEHR-EHR-OBSERVATION.required_a.v1"
	requiredEvalB   = "openEHR-EHR-EVALUATION.required_b.v1"
	requiredObsC    = "openEHR-EHR-OBSERVATION.required_c.v1"
	requiredSection = "openEHR-EHR-SECTION.adhoc.v1"
)

// requiredOptionalObservation is an optional OBSERVATION root of
// archetypeID that describes its RM-required data, a HISTORY at0001, and
// an optional protocol whose ITEM_TREE holds an optional ELEMENT.
func requiredOptionalObservation(archetypeID string) string {
	protocol := optOptionalSingleOver("protocol", optNode("ITEM_TREE", "at0002",
		optMultipleLowerZero("items", optElement("at0003", 0, 1))))
	return undescribedObservation(archetypeID, guardOccurrences01, describedHistory, protocol)
}

// requiredOptionalEvaluation is an optional EVALUATION root of
// archetypeID whose data, an ITEM_TREE at0001, lists a mandatory ELEMENT
// at0002 and an optional ELEMENT at0003.
func requiredOptionalEvaluation(archetypeID string) string {
	data := optSingle("data", optNode("ITEM_TREE", "at0001",
		optList("items", 0, 0, optElement("at0002", 1, 1), optElement("at0003", 0, 1))))
	return guardChild("C_ARCHETYPE_ROOT", "EVALUATION", "at0000", guardOccurrences01, archetypeID, data)
}

// requiredOptionalSection is an optional SECTION root of
// requiredSection whose items, at existence 0..1, list items.
func requiredOptionalSection(items ...string) string {
	return guardChild("C_ARCHETYPE_ROOT", "SECTION", "at0000", guardOccurrences01, requiredSection,
		guardMultiple("items", guardExistence01, items...))
}

// TestREQ107_RequiredBuildsEachContentEntryAtItsMandatoryParts is the
// REQ-107 check of the Required level: on a template whose
// COMPOSITION.content is optional, Required builds one member of each
// entry archetype the content names, and of a SECTION whose items name
// one, with the entry inside it. Each entry holds its mandatory parts
// only: the OBSERVATION no optional protocol, and the EVALUATION's data
// the mandatory ELEMENT alone, where Large and Medium build both. Every
// root passes the RM floor and the template validator, under every
// policy, both value fills and both compile modes.
func TestREQ107_RequiredBuildsEachContentEntryAtItsMandatoryParts(t *testing.T) {
	opt := guardOPT(guardCompositionID, guardMultiple("content", guardExistence01,
		requiredOptionalObservation(requiredObsA),
		requiredOptionalEvaluation(requiredEvalB),
		requiredOptionalSection(requiredOptionalObservation(requiredObsC))))
	full := requiredObsA + "{protocol}," + requiredEvalB + "{at0002,at0003}," +
		requiredSection + "(" + requiredObsC + "{protocol})"
	runRequiredContentCases(t, []requiredContentCase{{
		name: "optional content over two entries and a section",
		opt:  opt,
		want: map[instance.Policy]string{
			instance.Large:  full,
			instance.Medium: full,
			instance.Required: requiredObsA + "{}," + requiredEvalB + "{at0002}," +
				requiredSection + "(" + requiredObsC + "{})",
		},
	}})
}

// TestREQ107_RequiredCountsContentEntriesAsMedium is the REQ-107 check
// that Required builds the content entries Medium builds, one member of
// each archetype, and of each SECTION only where it leads to one:
//
//   - a second use of one entry archetype gets no member of its own;
//   - an optional entry that needs undescribed content, as one whose
//     protocol admits only a slot, gets none, and neither does a SECTION
//     that leads to no other entry;
//   - a SECTION whose items hold no entry gets none under Required, though
//     Large and Medium build it;
//   - a SECTION within a SECTION is built where it leads to an entry;
//   - a SECTION root's items, and a content the template writes as a
//     single-valued attribute, are built as a content list is.
//
// Every root passes the RM floor and the template validator, under every
// policy, both value fills and both compile modes.
func TestREQ107_RequiredCountsContentEntriesAsMedium(t *testing.T) {
	slottedProtocol := optSingle("protocol", optSlot("ITEM_TREE", "at0002"))
	undescribed := undescribedObservation(requiredObsC, guardOccurrences01, describedHistory, slottedProtocol)
	a := requiredObsA + "{protocol}"
	aRequired := requiredObsA + "{}"
	cases := []requiredContentCase{
		{
			name: "two uses of one entry archetype",
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence01,
				requiredOptionalObservation(requiredObsA), requiredOptionalObservation(requiredObsA))),
			want: map[instance.Policy]string{
				instance.Large:    a + "," + a,
				instance.Medium:   a,
				instance.Required: aRequired,
			},
		},
		{
			name: "an entry that needs undescribed content, alone and in a section",
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence01,
				undescribed, requiredOptionalSection(undescribed), requiredOptionalObservation(requiredObsA))),
			want: map[instance.Policy]string{
				instance.Large:    requiredSection + "()," + a,
				instance.Medium:   requiredSection + "()," + a,
				instance.Required: aRequired,
			},
		},
		{
			name: "a section that holds no entry",
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence01,
				requiredOptionalSection(optSlot("OBSERVATION", "at0001")), requiredOptionalObservation(requiredObsA))),
			want: map[instance.Policy]string{
				instance.Large:    requiredSection + "()," + a,
				instance.Medium:   requiredSection + "()," + a,
				instance.Required: aRequired,
			},
		},
		{
			name: "a section within a section",
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence01,
				requiredOptionalSection(optSectionChild("at0001", 0, 1,
					guardMultiple("items", guardExistence01, requiredOptionalObservation(requiredObsA)))))),
			want: map[instance.Policy]string{
				instance.Large:    requiredSection + "(at0001(" + a + "))",
				instance.Medium:   requiredSection + "(at0001(" + a + "))",
				instance.Required: requiredSection + "(at0001(" + aRequired + "))",
			},
		},
		{
			name: "a section root",
			opt: guardRootOPT("SECTION", requiredSection, guardMultiple("items", guardExistence01,
				requiredOptionalObservation(requiredObsA), requiredOptionalEvaluation(requiredEvalB))),
			want: map[instance.Policy]string{
				instance.Large:    a + "," + requiredEvalB + "{at0002,at0003}",
				instance.Medium:   a + "," + requiredEvalB + "{at0002,at0003}",
				instance.Required: aRequired + "," + requiredEvalB + "{at0002}",
			},
		},
		{
			name: "content written as a single-valued attribute",
			opt: guardOPT(guardCompositionID, guardSingle("content", guardExistence01,
				requiredOptionalObservation(requiredObsA))),
			want: map[instance.Policy]string{
				instance.Large:    a,
				instance.Medium:   a,
				instance.Required: aRequired,
			},
		},
	}
	runRequiredContentCases(t, cases)
}

// TestREQ107_RequiredLeavesOutACardinalityOnlyListThatLeadsToNoEntry is
// the REQ-107 check of What each policy visits that Required still leaves
// out a list that only its cardinality asks a member of, at existence 0..1
// and cardinality 1..*, where no child of it leads to an entry: an
// ITEM_TREE's items over an optional ELEMENT, and a COMPOSITION's content
// over an optional SECTION that holds only a slot. Large and Medium visit
// both. Every root passes the RM floor and the template validator, under
// every policy, both value fills and both compile modes.
func TestREQ107_RequiredLeavesOutACardinalityOnlyListThatLeadsToNoEntry(t *testing.T) {
	cases := []struct {
		name  string
		opt   string
		shape func(t *testing.T, out any) string
		want  map[instance.Policy]string
	}{
		{
			name: "ITEM_TREE items over an optional ELEMENT",
			opt:  optTemplate("ITEM_TREE", optList("items", 0, 1, optElement("at0001", 0, 1))),
			shape: func(t *testing.T, out any) string {
				t.Helper()
				return strings.Join(itemIDs(t, out), ",")
			},
			want: map[instance.Policy]string{instance.Large: "at0001", instance.Medium: "at0001", instance.Required: ""},
		},
		{
			name: "COMPOSITION content over a SECTION that holds only a slot",
			opt: optTemplate("COMPOSITION", optList("content", 0, 1,
				requiredOptionalSection(optSlot("OBSERVATION", "at0001")))),
			shape: func(t *testing.T, out any) string {
				t.Helper()
				return entryShape(out)
			},
			want: map[instance.Policy]string{
				instance.Large:    requiredSection + "()",
				instance.Medium:   requiredSection + "()",
				instance.Required: "",
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
						t.Fatalf("Generate: %v", err)
					}
					if got := tc.shape(t, out); got != tc.want[opts.Policy] {
						t.Errorf("Generate built %q, want %q", got, tc.want[opts.Policy])
					}
					noFloorErrors(t, out)
					for _, iss := range templateErrors(out, c) {
						t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
					}
				})
			}
		}
	}
}
