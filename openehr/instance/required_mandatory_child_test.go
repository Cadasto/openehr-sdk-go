package instance_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// optList is a C_MULTIPLE_ATTRIBUTE called name over children, with an
// existence of existenceLower..1 and a cardinality of cardinalityLower..*.
func optList(name string, existenceLower, cardinalityLower int, children ...string) string {
	return `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		`<existence>` + optInterval(existenceLower, 1) + `</existence>` +
		strings.Join(children, "") +
		`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		optInterval(cardinalityLower, -1) + `</interval></cardinality></attributes>`
}

// optRootOccurring is a C_ARCHETYPE_ROOT of rmType naming archetypeID, with
// occurrences lower..upper (upper -1 is unbounded).
func optRootOccurring(rmType, archetypeID string, lower, upper int, attrs ...string) string {
	return `<children xsi:type="C_ARCHETYPE_ROOT"><rm_type_name>` + rmType + `</rm_type_name>` +
		`<occurrences>` + optInterval(lower, upper) + `</occurrences>` +
		`<node_id>at0000</node_id>` + strings.Join(attrs, "") +
		`<archetype_id><value>` + archetypeID + `</value></archetype_id></children>`
}

// optElement is an ELEMENT child with the given node id and occurrences
// lower..upper (upper -1 is unbounded).
func optElement(nodeID string, lower, upper int) string {
	return optOccurring("C_COMPLEX_OBJECT", "ELEMENT", nodeID, lower, upper)
}

// evaluationID is the archetype of the EVALUATION the content cases below
// name.
const evaluationID = "openEHR-EHR-EVALUATION.rm_defaults.v1"

// policyIDs is, per policy, the archetype_node_id of every member the
// generator writes in the list under test, in order.
type policyIDs map[instance.Policy][]string

// sameUnderEveryPolicy is ids under each of the three policies.
func sameUnderEveryPolicy(ids ...string) policyIDs {
	return policyIDs{instance.Large: ids, instance.Medium: ids, instance.Required: ids}
}

// TestREQ107_RequiredVisitsAListForItsMandatoryChild is the REQ-107 check of
// the Visit rule's required attribute: a multi-valued attribute the OPT
// leaves optional by its existence is required when an allowed child of it
// that is not a slot is not optional, so Required visits it, and the child
// gets its occurrences lower bound of members. An optional list holding a
// 1..1 ELEMENT, as an ITEM_TREE's items over an ELEMENT, gets one member
// under every policy; beside an optional sibling, Required builds the
// mandatory one alone. A slot, or an optional child, makes no list required,
// and neither does the child of a single-valued attribute, whose existence
// decides: Required leaves an optional protocol out although its ITEM_TREE
// has occurrences 1..1. A list it visits is counted by its cardinality too.
// It holds under both value fills and both compile modes, and each tree
// passes the RM floor and the template validator.
func TestREQ107_RequiredVisitsAListForItsMandatoryChild(t *testing.T) {
	cases := []struct {
		name string
		root string
		opt  string
		// ids reads what the generator wrote in the list under test.
		ids  func(t *testing.T, out any) []string
		want policyIDs
	}{
		{
			name: "ITEM_TREE items over a 1..1 ELEMENT",
			root: "ITEM_TREE",
			opt:  optTemplate("ITEM_TREE", optList("items", 0, 0, optElement("at0001", 1, 1))),
			ids:  itemIDs,
			want: sameUnderEveryPolicy("at0001"),
		},
		{
			name: "ITEM_TREE items over an optional ELEMENT and a 1..1 one",
			root: "ITEM_TREE",
			opt:  optTemplate("ITEM_TREE", optList("items", 0, 0, optElement("at0001", 0, 1), optElement("at0002", 1, 1))),
			ids:  itemIDs,
			want: policyIDs{
				instance.Large:    {"at0001", "at0002"},
				instance.Medium:   {"at0001", "at0002"},
				instance.Required: {"at0002"},
			},
		},
		{
			name: "ITEM_TREE items at cardinality 2..* over a 1..* ELEMENT",
			root: "ITEM_TREE",
			opt:  optTemplate("ITEM_TREE", optList("items", 0, 2, optElement("at0001", 1, -1))),
			ids:  itemIDs,
			want: sameUnderEveryPolicy("at0001", "at0001"),
		},
		{
			name: "ITEM_TREE items over a slot",
			root: "ITEM_TREE",
			opt:  optTemplate("ITEM_TREE", optList("items", 0, 0, optSlot("CLUSTER", "at0001"))),
			ids:  itemIDs,
			want: sameUnderEveryPolicy(),
		},
		{
			name: "ITEM_TREE items over an optional ELEMENT",
			root: "ITEM_TREE",
			opt:  optTemplate("ITEM_TREE", optList("items", 0, 0, optElement("at0001", 0, 1))),
			ids:  itemIDs,
			want: policyIDs{
				instance.Large:    {"at0001"},
				instance.Medium:   {"at0001"},
				instance.Required: {},
			},
		},
		{
			name: "OBSERVATION protocol over a 1..1 ITEM_TREE",
			root: "OBSERVATION",
			opt: optTemplate("OBSERVATION", optOptionalSingleOver("protocol",
				optOccurring("C_COMPLEX_OBJECT", "ITEM_TREE", "at0001", 1, 1,
					optList("items", 1, 1, optElement("at0002", 1, 1))))),
			ids: func(_ *testing.T, out any) []string {
				p := out.(*rm.Observation).Protocol
				if p == nil || rm.IsTypedNil(p) {
					return []string{}
				}
				return []string{nodeID(p)}
			},
			want: policyIDs{
				instance.Large:    {"at0001"},
				instance.Medium:   {"at0001"},
				instance.Required: {},
			},
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					if got, want := tc.ids(t, out), tc.want[opts.Policy]; !slices.Equal(got, want) {
						t.Errorf("Generate(%s) wrote %v, want %v", tc.root, got, want)
					}
					checkValid(t, out, c)
				})
			}
		}
	}
}

// TestREQ107_RequiredContentFollowsItsChildrenNotItsCardinality is the
// REQ-107 check of the Visit rule on a COMPOSITION's content the OPT leaves
// optional by its existence. With a 1..1 archetype root the content is
// required, so every policy, Required included, writes that one member,
// also where that member holds content no template node describes, which
// keeps an optional attribute out under Large and Medium.
// With cardinality 1..* and only an optional archetype root, only the
// cardinality asks for a member, and a cardinality lower bound counts the
// members of a list that exists: Required leaves the content out, while
// Large and Medium, under which that bound still makes the content
// required, write the one member. It holds under both value fills, and each
// tree passes the RM floor and the template validator.
func TestREQ107_RequiredContentFollowsItsChildrenNotItsCardinality(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		want policyIDs
	}{
		{
			name: "a 1..1 archetype root",
			opt: optTemplate("COMPOSITION", optList("content", 0, 0,
				optRootOccurring("EVALUATION", evaluationID, 1, 1, optSingle("data", optNode("ITEM_TREE", "at0001"))))),
			want: sameUnderEveryPolicy(evaluationID),
		},
		{
			// The EVALUATION's data is built from its BMM type, content no
			// template node describes, so Large and Medium would leave an
			// optional content out; a required one they visit.
			name: "a 1..1 archetype root whose data the OPT leaves silent",
			opt:  optTemplate("COMPOSITION", optList("content", 0, 0, optRootOccurring("EVALUATION", evaluationID, 1, 1))),
			want: sameUnderEveryPolicy(evaluationID),
		},
		{
			name: "cardinality 1..* over an optional archetype root",
			opt:  optTemplate("COMPOSITION", optList("content", 0, 1, optRootOccurring("EVALUATION", evaluationID, 0, 1))),
			want: policyIDs{
				instance.Large:    {evaluationID},
				instance.Medium:   {evaluationID},
				instance.Required: {},
			},
		},
	}
	for _, tc := range cases {
		c := compileOPTText(t, tc.opt, true)
		for _, policy := range everyPolicy {
			for _, opts := range corpusOptions(policy) {
				t.Run(fmt.Sprintf("%s/%v/%v", tc.name, policy, opts.ValueFill), func(t *testing.T) {
					comp := generateComposition(t, c, opts)
					if got, want := contentIDs(comp.Content), tc.want[policy]; !slices.Equal(got, want) {
						t.Errorf("COMPOSITION.content = %v, want %v", got, want)
					}
					checkValid(t, comp, c)
				})
			}
		}
	}
}

// TestREQ107_FamilyHistoryRequiredLeavesOutItsOptionalContent is the
// REQ-107 check of the Visit rule on the vendored family_history.v.1.2.3
// template. Its content has existence 0..1 and cardinality 1..*, and its one
// EVALUATION has occurrences 0..1, so only the cardinality asks for a
// member: Required writes no content. Large and Medium write the
// EVALUATION, and its data/items, which the template leaves optional by
// their existence, get the 1..* CLUSTER at0003 the Visit rule makes them
// required for. Each tree passes the RM floor and the template validator.
func TestREQ107_FamilyHistoryRequiredLeavesOutItsOptionalContent(t *testing.T) {
	c := compileCorpusOPT(t, "templates/family_history.v.1.2.3")
	for _, policy := range everyPolicy {
		for _, opts := range corpusOptions(policy) {
			t.Run(fmt.Sprintf("%v/%v", policy, opts.ValueFill), func(t *testing.T) {
				comp := generateComposition(t, c, opts)
				checkValid(t, comp, c)
				if policy == instance.Required {
					if len(comp.Content) != 0 {
						t.Errorf("COMPOSITION.content = %v, want none", contentIDs(comp.Content))
					}
					return
				}
				if len(comp.Content) != 1 {
					t.Fatalf("COMPOSITION.content = %v, want the one EVALUATION", contentIDs(comp.Content))
				}
				ev, ok := comp.Content[0].(*rm.Evaluation)
				if !ok {
					t.Fatalf("COMPOSITION.content[0] is %T, want *rm.Evaluation", comp.Content[0])
				}
				tree, ok := ev.Data.(*rm.ItemTree)
				if !ok {
					t.Fatalf("EVALUATION.data is %T, want *rm.ItemTree", ev.Data)
				}
				if ids := contentIDs(tree.Items); !slices.Contains(ids, "at0003") {
					t.Errorf("EVALUATION data/items = %v, want the CLUSTER at0003", ids)
				}
			})
		}
	}
}
