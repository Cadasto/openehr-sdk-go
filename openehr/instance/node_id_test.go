package instance_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_LocatableNodeIDPlaceholder is the REQ-107 check that the
// generator writes the node id at0000 on a locatable it has no OPT node id
// for. Each row reaches one place that writes it: a node the OPT names
// without a node id, the placeholder ELEMENT for an items list the OPT
// does not describe, and a locatable built from the BMM alone. The
// PARTY_RELATIONSHIP fill has its own internal test, because Generate
// names every relationship before it reaches that fill.
func TestREQ107_LocatableNodeIDPlaceholder(t *testing.T) {
	cases := []struct {
		name     string
		opt      string
		implicit bool
	}{
		{
			name:     "OPT node without a node id",
			opt:      optTemplate("CLUSTER", optMultiple("items", optNode("ELEMENT", ""))),
			implicit: true,
		},
		{
			// Without the implicit attributes the CLUSTER has no items
			// attribute, so the RM floor's one member is the placeholder.
			name:     "placeholder ELEMENT of an items list the OPT does not describe",
			opt:      optTemplate("CLUSTER"),
			implicit: false,
		},
		{
			// items is named with no children and a lower bound of 1, so
			// its member is built from the BMM.
			name:     "locatable built from the BMM",
			opt:      optTemplate("CLUSTER", optMultiple("items")),
			implicit: true,
		},
	}
	for _, tc := range cases {
		c := compileOPTText(t, tc.opt, tc.implicit)
		for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
			t.Run(fmt.Sprintf("%s/%v", tc.name, policy), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				items := out.(*rm.Cluster).Items
				if len(items) != 1 {
					t.Fatalf("CLUSTER.items has %d members, want 1", len(items))
				}
				if got := nodeID(items[0]); got != "at0000" {
					t.Errorf("CLUSTER.items[0] (%T) archetype_node_id = %q, want %q", items[0], got, "at0000")
				}
			})
		}
	}
}
