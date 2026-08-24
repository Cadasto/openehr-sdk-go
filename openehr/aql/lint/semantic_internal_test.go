package lint

// semantic_internal_test.go: white-box pins for the unexported REQ-161
// fan-out helpers that no query [parse.Parse] can produce a tree for. See
// semantic_test.go's TestFanoutRowGrainFires doc comment for the
// parsed-query-side half of this story.

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// TestAndFrontierFlattensHandBuiltNestedAnd exercises andFrontier's own
// recursive AND-in-AND branch directly, on a hand-built tree — fix round 1,
// Important 2.
//
// REQ-161 § Checks states the fan-out rule as "flattening nested AND
// junctions", and andFrontier implements that literally with a branch that
// recurses when a child is ITSELF an AND junction. But REQ-117's
// same-operator splicing (openehr/aql/parse/extract_query.go:513-520) always
// merges a same-operator child's Children into its parent BEFORE this
// package ever sees the tree, so no query [parse.Parse] can produce hands
// this branch a genuine AND-under-AND node — every parsed
// `A AND (B AND C)` arrives as ONE three-child junction, indistinguishable
// from `A AND B AND C`. Without this test, that recursive branch is provably
// dead code with nothing to prove it, which is exactly the gap fix round 1
// flagged: a maintainer reading only the external test suite would believe
// flattening-through-nesting was pinned when only flattening-of-an-already-
// flat list was.
//
// The rule stays implemented as written rather than deleted as dead code
// (the controller's ruling): REQ-161 states it in terms of nesting, and
// keeping the recursive branch means the check stays correct even if
// REQ-117's splicing were ever relaxed. This test is what makes that branch
// answerable to a regression instead of merely present.
func TestAndFrontierFlattensHandBuiltNestedAnd(t *testing.T) {
	t.Parallel()
	// A AND (B AND C), built directly rather than parsed: the inner
	// Containment is a genuine AND nested inside another AND's Children.
	tree := parse.Containment{
		ChildJoin: parse.ContainsAnd,
		Children: []parse.Containment{
			{Class: parse.ClassExpr{RMType: "OBSERVATION", Alias: "a"}},
			{
				ChildJoin: parse.ContainsAnd,
				Children: []parse.Containment{
					{Class: parse.ClassExpr{RMType: "OBSERVATION", Alias: "b"}},
					{Class: parse.ClassExpr{RMType: "OBSERVATION", Alias: "c"}},
				},
			},
		},
	}
	leaves, boundary := andFrontier(tree)
	if len(boundary) != 0 {
		t.Fatalf("boundary = %v, want none: an all-AND tree excludes nothing", boundary)
	}
	var aliases []string
	for _, l := range leaves {
		aliases = append(aliases, l.Class.Alias)
	}
	if want := []string{"a", "b", "c"}; !slices.Equal(aliases, want) {
		t.Errorf("leaves = %v, want %v (the nested AND's operands flattened alongside the outer AND's own)", aliases, want)
	}
}

// TestAndFrontierTreatsNegatedAndAsBoundary is the andFrontier-level half of
// fix round 1's Important 1 (aql_fanout_row_grain must not fire on a negated
// AND junction): even called directly on a hand-built AND junction flagged
// Negated, andFrontier must return it whole as a boundary node rather than
// flattening it into firing leaves — the defensive check its own doc comment
// promises, exercised here because no [fanoutIssues] walk call ever reaches
// this function with n.Negated true in practice ([fanoutIssues]'s own guard
// already excludes it before calling andFrontier at all).
func TestAndFrontierTreatsNegatedAndAsBoundary(t *testing.T) {
	t.Parallel()
	tree := parse.Containment{
		ChildJoin: parse.ContainsAnd,
		Negated:   true,
		Children: []parse.Containment{
			{Class: parse.ClassExpr{RMType: "OBSERVATION", Alias: "o1"}},
			{Class: parse.ClassExpr{RMType: "OBSERVATION", Alias: "o2"}},
		},
	}
	leaves, boundary := andFrontier(tree)
	if len(leaves) != 0 {
		t.Errorf("leaves = %v, want none: a negated AND junction must not be flattened into firing leaves", leaves)
	}
	if len(boundary) != 1 {
		t.Fatalf("boundary = %v, want exactly the negated junction itself", boundary)
	}
	if !boundary[0].Negated || boundary[0].ChildJoin != parse.ContainsAnd {
		t.Errorf("boundary[0] = %+v, want the original negated AND junction returned whole", boundary[0])
	}
}
