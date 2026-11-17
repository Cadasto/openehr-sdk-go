package instance_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/builderseed"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
)

// optRepeatingItems is a CLUSTER template whose items, at cardinality
// lower..upper (upper -1 is unbounded), hold children.
func optRepeatingItems(lower, upper int, children ...string) string {
	return optTemplate("CLUSTER", optCardinal("items", lower, upper, children...))
}

// TestREQ107_LargeBuildsTwoMembersOfARepeatingNode is the REQ-107 check of
// Members of a list that, under Large, an allowed child whose occurrences
// upper bound is above 1, or unbounded, gets max(occurrences.lower, 2)
// members, and any other child max(occurrences.lower, 1): one where its
// upper bound is 1 or it states no occurrences. The cardinality upper
// bound still caps the list, and a child that asks for a member still gets
// the room first, so a second member takes only the room left. Medium
// keeps one member of each such child, and so does the composition
// builder's seed (REQ-101), which is Large with no repeats. Every root
// passes the template validator and the RM floor, under both value fills.
func TestREQ107_LargeBuildsTwoMembersOfARepeatingNode(t *testing.T) {
	element := func(nodeID string, lower, upper int) string {
		return optOccurring("C_COMPLEX_OBJECT", "ELEMENT", nodeID, lower, upper)
	}
	cases := []struct {
		name string
		opt  string
		// large is the items under Large; medium those under Medium and in
		// the builder's seed.
		large, medium []string
	}{
		{
			name:   "optional child, unbounded",
			opt:    optRepeatingItems(0, -1, element("at0001", 0, -1)),
			large:  []string{"at0001", "at0001"},
			medium: []string{"at0001"},
		},
		{
			name:   "optional child, upper bound 3",
			opt:    optRepeatingItems(0, -1, element("at0001", 0, 3)),
			large:  []string{"at0001", "at0001"},
			medium: []string{"at0001"},
		},
		{
			name:   "required child, 1..2",
			opt:    optRepeatingItems(0, -1, element("at0001", 1, 2)),
			large:  []string{"at0001", "at0001"},
			medium: []string{"at0001"},
		},
		{
			name:   "child whose lower bound is above 2",
			opt:    optRepeatingItems(0, -1, element("at0001", 3, -1)),
			large:  []string{"at0001", "at0001", "at0001"},
			medium: []string{"at0001", "at0001", "at0001"},
		},
		{
			name:   "child whose upper bound is 1",
			opt:    optRepeatingItems(0, -1, element("at0001", 0, 1)),
			large:  []string{"at0001"},
			medium: []string{"at0001"},
		},
		{
			name:   "child with no occurrences",
			opt:    optRepeatingItems(0, -1, optNode("ELEMENT", "at0001")),
			large:  []string{"at0001"},
			medium: []string{"at0001"},
		},
		{
			name:   "cardinality upper bound 1",
			opt:    optRepeatingItems(0, 1, element("at0001", 0, -1)),
			large:  []string{"at0001"},
			medium: []string{"at0001"},
		},
		{
			// The second at0001 member would take the room the later
			// required at0002 asks for.
			name:   "room kept for a later required child",
			opt:    optRepeatingItems(0, 2, element("at0001", 0, -1), element("at0002", 1, 1)),
			large:  []string{"at0001", "at0002"},
			medium: []string{"at0001", "at0002"},
		},
		{
			// The required at0001 gets its asked member and a second one; the
			// optional at0002 gets the one place left.
			name:   "second member of a required child within the bound",
			opt:    optRepeatingItems(0, 3, element("at0001", 1, -1), element("at0002", 0, -1)),
			large:  []string{"at0001", "at0001", "at0002"},
			medium: []string{"at0001", "at0002"},
		},
		{
			name:   "each repeating child twice",
			opt:    optRepeatingItems(0, -1, element("at0001", 0, -1), element("at0002", 0, 5)),
			large:  []string{"at0001", "at0001", "at0002", "at0002"},
			medium: []string{"at0001", "at0002"},
		},
	}
	for _, tc := range cases {
		c := compileOPTText(t, tc.opt, true)
		runs := []struct {
			name   string
			ctx    func(context.Context) context.Context
			policy instance.Policy
			want   []string
		}{
			{name: "Large", ctx: plainContext, policy: instance.Large, want: tc.large},
			{name: "Medium", ctx: plainContext, policy: instance.Medium, want: tc.medium},
			{name: "builder seed", ctx: builderseed.With, policy: instance.Large, want: tc.medium},
		}
		for _, run := range runs {
			for _, opts := range optionsFor([]instance.Policy{run.policy}) {
				t.Run(fmt.Sprintf("%s/%s/%v", tc.name, run.name, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(run.ctx(t.Context()), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					if got := itemIDs(t, out); !slices.Equal(got, run.want) {
						t.Errorf("Generate(%s) items = %v, want %v", run.name, got, run.want)
					}
					for _, iss := range templateErrors(out, c) {
						t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
					}
					noFloorErrors(t, out)
				})
			}
		}
	}
}

// TestREQ107_LargeBuildsTwoMembersOfARepeatingArchetype is the REQ-107
// check of Members of a list that, under Large, an optional archetype root
// a composition's content allows any number of gets two members, each one
// built in full, while Medium and the composition builder's seed (REQ-101)
// build one. The root passes the template validator and the RM floor, under
// both value fills.
func TestREQ107_LargeBuildsTwoMembersOfARepeatingArchetype(t *testing.T) {
	c := compileOPTText(t, optUsesTemplate(optObservationUse("", 0, -1)), true)
	runs := []struct {
		name   string
		ctx    func(context.Context) context.Context
		policy instance.Policy
		want   int
	}{
		{name: "Large", ctx: plainContext, policy: instance.Large, want: 2},
		{name: "Medium", ctx: plainContext, policy: instance.Medium, want: 1},
		{name: "builder seed", ctx: builderseed.With, policy: instance.Large, want: 1},
	}
	for _, run := range runs {
		for _, opts := range corpusOptions(run.policy) {
			t.Run(fmt.Sprintf("%s/%v", run.name, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(run.ctx(t.Context()), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				comp, err := instance.AsComposition(out)
				if err != nil {
					t.Fatalf("AsComposition: %v", err)
				}
				ids := contentIDs(comp.Content)
				if len(ids) != run.want {
					t.Fatalf("Generate(%s) content = %v, want %d members", run.name, ids, run.want)
				}
				for _, id := range ids {
					if id != usedObservationID {
						t.Errorf("Generate(%s) content member %q, want %q", run.name, id, usedObservationID)
					}
				}
				noValidationErrors(t, comp, c)
				noFloorErrors(t, comp)
			})
		}
	}
}

// plainContext returns ctx as it is: the context of a Generate call the
// composition builder does not make.
func plainContext(ctx context.Context) context.Context { return ctx }
