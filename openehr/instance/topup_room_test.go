package instance_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// topUpLabels names each member of a generated CLUSTER's items by its
// archetype_node_id, its name and, for an ELEMENT with a value, its value's
// RM type: "at0001 'x' DV_TEXT".
func topUpLabels(t *testing.T, out any) []string {
	t.Helper()
	cluster, ok := out.(*rm.Cluster)
	if !ok {
		t.Fatalf("generated root is %T, want a CLUSTER", out)
	}
	labels := make([]string, 0, len(cluster.Items))
	for _, item := range cluster.Items {
		loc, ok := item.(rm.Locatable)
		if !ok {
			t.Fatalf("CLUSTER.items member is %T, want a LOCATABLE", item)
		}
		label := fmt.Sprintf("%s '%s'", loc.GetArchetypeNodeID(), loc.GetName().GetValue())
		if el, ok := item.(*rm.Element); ok && el.Value != nil && !rm.IsTypedNil(el.Value) {
			label += " " + rmTypeName(el.Value)
		}
		labels = append(labels, label)
	}
	return labels
}

// TestREQ107_TopUpTakesAChildWithRoom is the REQ-107 What a visit writes
// check of the top-up: where the members the children get of their own fall
// short of what the list needs, each member the top-up adds comes, in OPT
// order, from an allowed child that still has room under its occurrences
// upper bound, a child Large or Medium gave no member of its own included,
// so the root keeps every child's occurrences and passes the template
// validator and the RM floor. Only where no allowed child has room, as in an
// OPT whose cardinality lower bound exceeds what its children's occurrences
// allow, does the top-up repeat its first allowed child that is not a slot,
// past that child's occurrences. It holds under every policy, both value
// fills and both compile modes.
func TestREQ107_TopUpTakesAChildWithRoom(t *testing.T) {
	element := func(nodeID string, lower, upper int, valueType string) string {
		return optOccurring("C_COMPLEX_OBJECT", "ELEMENT", nodeID, lower, upper, optSingle("value", optNode(valueType, "")))
	}
	named := func(name string) string {
		return optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0001", 0, 1,
			optPinnedName(name), optSingle("value", optNode("DV_TEXT", "")))
	}
	everyPolicyGets := func(labels ...string) map[instance.Policy][]string {
		return map[instance.Policy][]string{instance.Large: labels, instance.Medium: labels, instance.Required: labels}
	}
	cases := []struct {
		name  string
		lower int
		// children are the allowed children of CLUSTER.items, at
		// cardinality lower..*.
		children []string
		want     map[instance.Policy][]string
		// overrun is the path of the one child whose occurrences the root
		// breaks, where no child has room for the top-up; "" for none.
		overrun string
	}{
		{
			// The issue's evidence: at0001 is full after its own member, so
			// the top-up takes at0002, which has no upper bound.
			name:     "a later child with room",
			lower:    3,
			children: []string{element("at0001", 1, 1, "DV_TEXT"), element("at0002", 0, -1, "DV_COUNT")},
			want:     everyPolicyGets("at0001 'ELEMENT' DV_TEXT", "at0002 'ELEMENT' DV_COUNT", "at0002 'ELEMENT' DV_COUNT"),
		},
		{
			// Required gives the optional at0002 no member of its own, and
			// the top-up builds it.
			name:     "an optional child Required builds no member of",
			lower:    2,
			children: []string{element("at0001", 1, 1, "DV_TEXT"), element("at0002", 0, 1, "DV_COUNT")},
			want:     everyPolicyGets("at0001 'ELEMENT' DV_TEXT", "at0002 'ELEMENT' DV_COUNT"),
		},
		{
			// Medium skips the optional at0001, whose identity the required
			// one carries; the required one is full, so the top-up takes the
			// optional one, and the validator binds its member there.
			name:     "a skipped sibling of a full child's identity",
			lower:    2,
			children: []string{element("at0001", 1, 1, "DV_TEXT"), element("at0001", 0, 1, "DV_COUNT")},
			want:     everyPolicyGets("at0001 'ELEMENT' DV_TEXT", "at0001 'ELEMENT' DV_COUNT"),
		},
		{
			// As above, with an optional CLUSTER between them that needs
			// undescribed content, since its items admit only a slot. The
			// optional at0001 needs none, and the one earlier sibling that
			// carries its identity is full, so the top-up takes it rather
			// than the CLUSTER, the first child with room.
			name:  "a described sibling of a full child's identity after an undescribed one",
			lower: 2,
			children: []string{
				element("at0001", 1, 1, "DV_TEXT"), slotOnlyCluster("at0002", guardOccurrences01),
				element("at0001", 0, 1, "DV_COUNT"),
			},
			want: everyPolicyGets("at0001 'ELEMENT' DV_TEXT", "at0001 'ELEMENT' DV_COUNT"),
		},
		{
			// Two optional uses of one node id, named x and y. Medium skips
			// y, whose identity x carries, and Required builds neither of
			// its own; the top-up takes x, then y once x is full.
			name:     "named siblings of one identity",
			lower:    2,
			children: []string{named("x"), named("y")},
			want:     everyPolicyGets("at0001 'x' DV_TEXT", "at0001 'y' DV_TEXT"),
		},
		{
			// at0001 is full and the only other child is a slot, whose
			// occurrences the parser does not read, so it has room: the
			// top-up fills it.
			name:     "a slot after a full child",
			lower:    2,
			children: []string{element("at0001", 1, 1, "DV_TEXT"), optSlot("ELEMENT", "at0002")},
			want:     everyPolicyGets("at0001 'ELEMENT' DV_TEXT", "openEHR-EHR-ELEMENT.example.v1 'ELEMENT'"),
		},
		{
			// The OPT contradicts itself: three members, but its children
			// allow two. No child has room, so the top-up repeats the first.
			name:     "no child with room",
			lower:    3,
			children: []string{element("at0001", 1, 1, "DV_TEXT"), element("at0002", 0, 1, "DV_COUNT")},
			want:     everyPolicyGets("at0001 'ELEMENT' DV_TEXT", "at0002 'ELEMENT' DV_COUNT", "at0001 'ELEMENT' DV_TEXT"),
			overrun:  "/items[at0001]",
		},
	}
	for _, tc := range cases {
		opt := optTemplate("CLUSTER", optCardinal("items", tc.lower, -1, tc.children...))
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, opt, implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					if got, want := topUpLabels(t, out), tc.want[opts.Policy]; !slices.Equal(got, want) {
						t.Errorf("CLUSTER.items members = %q, want %q", got, want)
					}
					noFloorErrors(t, out)
					errs := templateErrors(out, c)
					if tc.overrun == "" {
						for _, iss := range errs {
							t.Errorf("Validate: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
						}
						return
					}
					if len(errs) != 1 || !hasCardinalityAt(errs, tc.overrun) {
						t.Errorf("Validate reports %s, want the occurrences of %s alone", issueSummary(errs), tc.overrun)
					}
				})
			}
		}
	}
}
