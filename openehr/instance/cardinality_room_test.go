package instance_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
)

// TestREQ107_RequiredChildrenGetTheRoomFirst is the REQ-107 What a visit
// writes check that, within a bounded cardinality, the members the children
// whose occurrences lower bound is bounded and at least 1 ask for get the
// room first, and any other child, before or after them, gets only the room
// they leave, while the members stand in OPT order. A CLUSTER's items, which
// every policy visits, at cardinality 0..1 over an optional at0001 and a
// required at0002 hold the at0002 member alone, so the root does not fail
// the template validator by at0002's occurrences; a child that states no
// occurrences, which gets a member under every policy, yields the room the
// same way. Where room is left, an optional child before the required one
// still takes it under Large and Medium. Each root passes the RM floor and
// the template validator under every policy, both value fills and both
// compile modes.
func TestREQ107_RequiredChildrenGetTheRoomFirst(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		want policyIDs
	}{
		{
			name: "cardinality 0..1 over an optional and a required ELEMENT",
			opt:  optTemplate("CLUSTER", optCardinal("items", 0, 1, optElement("at0001", 0, 1), optElement("at0002", 1, 1))),
			want: sameUnderEveryPolicy("at0002"),
		},
		{
			name: "cardinality 0..1 over an ELEMENT with no occurrences and a required one",
			opt:  optTemplate("CLUSTER", optCardinal("items", 0, 1, optNode("ELEMENT", "at0001"), optElement("at0002", 1, 1))),
			want: sameUnderEveryPolicy("at0002"),
		},
		{
			name: "cardinality 0..2 over an optional ELEMENT and a 2..2 one",
			opt:  optTemplate("CLUSTER", optCardinal("items", 0, 2, optElement("at0001", 0, 1), optElement("at0002", 2, 2))),
			want: sameUnderEveryPolicy("at0002", "at0002"),
		},
		{
			name: "cardinality 0..2 over an optional, a required and an optional ELEMENT",
			opt: optTemplate("CLUSTER", optCardinal("items", 0, 2,
				optElement("at0001", 0, 1), optElement("at0002", 1, 1), optElement("at0003", 0, 1))),
			want: policyIDs{
				instance.Large:    {"at0001", "at0002"},
				instance.Medium:   {"at0001", "at0002"},
				instance.Required: {"at0002"},
			},
		},
		{
			name: "control: cardinality 0..1 over a required and an optional ELEMENT",
			opt:  optTemplate("CLUSTER", optCardinal("items", 0, 1, optElement("at0001", 1, 1), optElement("at0002", 0, 1))),
			want: sameUnderEveryPolicy("at0001"),
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
					if got, want := itemIDs(t, out), tc.want[opts.Policy]; !slices.Equal(got, want) {
						t.Errorf("CLUSTER.items = %v, want %v", got, want)
					}
					checkValid(t, out, c)
				})
			}
		}
	}
}
