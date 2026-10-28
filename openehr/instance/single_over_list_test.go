package instance_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// listMemberIDs returns the archetype_node_id of each member of the list a
// generated PERSON's identities or ITEM_TREE's items hold.
func listMemberIDs(t *testing.T, out any) []string {
	t.Helper()
	ids := []string{}
	switch v := out.(type) {
	case *rm.Person:
		for _, id := range v.Identities {
			ids = append(ids, id.ArchetypeNodeID)
		}
	case *rm.ItemTree:
		for _, item := range v.Items {
			ids = append(ids, nodeID(item))
		}
	default:
		t.Fatalf("generated root is %T, want a PERSON or an ITEM_TREE", out)
	}
	return ids
}

// TestREQ107_SingleAttributeOverAListIsWrittenAsTheList is the REQ-107 What
// a visit writes check of an attribute the RM declares a list that the OPT
// writes as a C_SINGLE_ATTRIBUTE: the generator writes it as the list the
// RM declares, as the template validator reads it. A PERSON's identities,
// which the BMM requires, get the one member they need, built from the BMM
// type where the OPT names no child and from the child where it names one.
// An ITEM_TREE's items over an ELEMENT get that ELEMENT under every policy
// that visits them: every policy at an existence of 1..1, and Large and
// Medium at 0..1. Each root passes the template validator and the RM floor,
// under every policy, both value fills and both compile modes.
func TestREQ107_SingleAttributeOverAListIsWrittenAsTheList(t *testing.T) {
	everyPolicyGets := func(ids ...string) map[instance.Policy][]string {
		return map[instance.Policy][]string{instance.Large: ids, instance.Medium: ids, instance.Required: ids}
	}
	cases := []struct {
		name string
		opt  string
		// want is the archetype_node_id of each list member, per policy;
		// "*" takes any, for a member built from the BMM.
		want map[instance.Policy][]string
	}{
		{
			name: "PERSON identities with no child",
			opt:  optTemplate("PERSON", optSingle("identities")),
			want: everyPolicyGets("*"),
		},
		{
			name: "PERSON identities over a PARTY_IDENTITY",
			opt:  optTemplate("PERSON", optSingle("identities", optNode("PARTY_IDENTITY", "at0001"))),
			want: everyPolicyGets("at0001"),
		},
		{
			name: "ITEM_TREE items over an ELEMENT, existence 1..1",
			opt:  optTemplate("ITEM_TREE", optSingle("items", optNode("ELEMENT", "at0001"))),
			want: everyPolicyGets("at0001"),
		},
		{
			name: "ITEM_TREE items over an ELEMENT, existence 0..1",
			opt:  optTemplate("ITEM_TREE", optOptionalSingleOver("items", optNode("ELEMENT", "at0001"))),
			want: map[instance.Policy][]string{instance.Large: {"at0001"}, instance.Medium: {"at0001"}, instance.Required: {}},
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
					got, want := listMemberIDs(t, out), tc.want[opts.Policy]
					if len(want) == 1 && want[0] == "*" && len(got) == 1 {
						want = got
					}
					if !slices.Equal(got, want) {
						t.Errorf("list members = %q, want %q", got, want)
					}
					noFloorErrors(t, out)
					for _, iss := range templateErrors(out, c) {
						t.Errorf("Validate: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
					}
				})
			}
		}
	}
}
