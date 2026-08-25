package lint

// semantic_internal_test.go: white-box pins for the unexported REQ-161
// fan-out helpers that no query [parse.Parse] can produce a tree for. See
// semantic_test.go's TestFanoutRowGrainFires doc comment for the
// parsed-query-side half of this story. It also pins [containCheck.walk]'s
// chain-tail return against the same class of hand-built, parser-unreachable
// shape — see TestWalkTracksChainTailAcrossFlattenedSiblings below.
//
// Importing openehr/aql here (for the write-side half of that test) is not a
// cycle: lint.go/path.go/resolve.go already import it in production code, and
// aql imports nothing from lint.

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/internal/semcheck"
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

// TestAndFrontierTreatsNegatedClassNodeAsBoundary is the CLASS-node half of
// the test above. andFrontier's contract is stated over nodes ("a non-AND
// (OR) node, or a NEGATED node"), but its Negated test used to sit BELOW the
// junction/class split, so a negated class node fell through to the leaf arm
// and was counted towards REQ-161's ">= 2 projected leaves" threshold — a
// filter flattened into a row-multiplying leaf, contradicting the function's
// own doc.
//
// Like its junction twin this is defensive rather than reachable: the parser
// sets Negated only on the node a CONTAINS/NOT CONTAINS chain step targets,
// so a junction's direct Children can never individually carry it
// ([parse.Containment]'s own doc comment). Pinning it is what keeps the
// ordering of the two tests from being "tidied" back.
func TestAndFrontierTreatsNegatedClassNodeAsBoundary(t *testing.T) {
	t.Parallel()
	n := parse.Containment{
		Class:   parse.ClassExpr{RMType: "OBSERVATION", Alias: "o1"},
		Negated: true,
	}
	leaves, boundary := andFrontier(n)
	if len(leaves) != 0 {
		t.Errorf("leaves = %v, want none: a negated node is a boundary whatever its kind", leaves)
	}
	if len(boundary) != 1 || !boundary[0].Negated || boundary[0].Class.Alias != "o1" {
		t.Fatalf("boundary = %+v, want exactly the negated class node returned whole", boundary)
	}
}

// TestWalkJunctionNeverBecomesPredecessor is the READ-side twin of
// openehr/aql's TestVerifyContainmentJunctionNeverBecomesPredecessor: the
// same axis, previously swept on one side only.
//
// [containCheck.walk]'s chain loop advances `prev` to the operand a child's
// own downward chain ends on — but ONLY when that child is a class node
// (`if decided := c.walk(…); !isJunction(ch)`). A junction node has no class
// of its own and returns the ZERO Operand, so letting it become `prev` would
// make every LATER sibling in the same chain be checked against nothing at
// all: the zero Operand suppresses every pair (semanticIssues' own doc
// comment), so the findings would vanish silently rather than change. A
// junction may only END a chain ([containCheck.walk]'s Children comment), so
// the guard is a contract statement, not a workaround.
//
// The shape is hand-built for the same reason
// [TestWalkTracksChainTailAcrossFlattenedSiblings] is: the read-side
// extractor gives a class node at most one child, so a junction followed by a
// FURTHER sibling in one chain is not a tree [parse.Parse] produces. The
// verdicts are REQ-160 acceptance-table facts, not invented here:
// OBSERVATION→ELEMENT and OBSERVATION→CLUSTER are admissible (so the junction
// arm contributes nothing of its own and cannot mask the assertion), while
// OBSERVATION→SECTION is Never ("entries never contain sections"). Replacing
// the guard with an unconditional `prev = c.walk(…)` therefore yields the
// EMPTY multiset here, not a different code.
func TestWalkJunctionNeverBecomesPredecessor(t *testing.T) {
	t.Parallel()

	// OBSERVATION o CONTAINS (ELEMENT e1 OR CLUSTER cl1) CONTAINS SECTION s,
	// as ONE chain: the junction is o's first child and SECTION its second,
	// so SECTION's predecessor must still be o itself.
	junction := parse.Containment{
		ChildJoin: parse.ContainsOr,
		Children: []parse.Containment{
			{Class: parse.ClassExpr{RMType: "ELEMENT", Alias: "e1"}},
			{Class: parse.ClassExpr{RMType: "CLUSTER", Alias: "cl1"}},
		},
	}
	sect := parse.Containment{Class: parse.ClassExpr{RMType: "SECTION", Alias: "s"}}
	root := parse.Containment{
		Class:    parse.ClassExpr{RMType: "OBSERVATION", Alias: "o"},
		Children: []parse.Containment{junction, sect},
	}

	cc := containCheck{ck: semcheck.New(nil)}
	cc.walk(semcheck.Operand{}, semcheck.RoleRoot, root)
	var got []string
	for _, iss := range cc.issues {
		got = append(got, iss.Code)
	}
	if want := []string{"aql_impossible_containment"}; !slices.Equal(got, want) {
		t.Fatalf("codes = %v, want exactly %v — a junction must never become the predecessor "+
			"of a later sibling; the zero Operand it returns would suppress the pair entirely", got, want)
	}
}

// TestWalkTracksChainTailAcrossFlattenedSiblings pins REQ-162's read/write
// "no drift" structurally, for the one containment shape [lint.LintString]
// can never itself reach: a class node whose Children hold TWO OR MORE
// entries (the flattened spelling, [parse.Containment.Children]'s own doc)
// where the FIRST of them carries its own further chain. The read-side
// extractor never builds this shape (every class node it produces has at
// most one child — semanticIssues' own doc comment), so it is exercised
// here only by a hand-built tree, mirrored against the write side's
// EQUIVALENT tree built through the ordinary, reachable
// [aql.Containment.Contains] fluent API ("[r]epeated calls extend the chain
// in call order", that method's own doc) — the write side reaches this
// shape naturally; the read side does not.
//
// The tree: `COMPOSITION n CONTAINS OBSERVATION a CONTAINS FOLDER a1`,
// with `COMPOSITION b` a SECOND child of `n` alongside `a` (so `a`, which
// itself carries the one-element chain `a1`, is n's "first child with its
// own chain", and `b` is the sibling that must be checked adjacent to
// whichever operand is `a`'s TRUE chain tail).
//
// [containCheck.walk] previously returned the node's own operand instead
// of its chain's tail (`return self` where `return prev` belongs), so `b`
// was checked adjacent to `a` itself rather than to `a1`. That is
// observable here because the two candidate predecessors disagree on the
// verdict for `b` (COMPOSITION), per [contain.Default]'s acceptance table:
// OBSERVATION→COMPOSITION is Never, so the bug manufactures a SECOND
// aql_impossible_containment (the first already comes from the always-
// correct a→a1 pair, OBSERVATION→FOLDER, also Never); the fix's
// FOLDER→COMPOSITION is ByReference, raising aql_containment_by_reference
// instead of the duplicate — a different multiset, not merely a duplicate
// finding, so a mutation back to `return self` fails this test loudly.
func TestWalkTracksChainTailAcrossFlattenedSiblings(t *testing.T) {
	t.Parallel()

	a1 := parse.Containment{Class: parse.ClassExpr{RMType: "FOLDER", Alias: "a1"}}
	a := parse.Containment{
		Class:    parse.ClassExpr{RMType: "OBSERVATION", Alias: "a"},
		Children: []parse.Containment{a1},
	}
	b := parse.Containment{Class: parse.ClassExpr{RMType: "COMPOSITION", Alias: "b"}}
	n := parse.Containment{
		Class:    parse.ClassExpr{RMType: "COMPOSITION", Alias: "n"},
		Children: []parse.Containment{a, b},
	}

	cc := containCheck{ck: semcheck.New(nil)}
	cc.walk(semcheck.Operand{}, semcheck.RoleRoot, n)
	var readCodes []string
	for _, iss := range cc.issues {
		readCodes = append(readCodes, iss.Code)
	}
	slices.Sort(readCodes)

	// Same logical tree, the write side's own way: two stacked Contains
	// calls under the FROM root give n's chain exactly [a, b], and a's own
	// Contains call gives a its own one-element chain [a1] — no hand-built
	// hack needed on this side.
	wa := aql.Class("OBSERVATION", "a").Contains(aql.Class("FOLDER", "a1"))
	wb := aql.Class("COMPOSITION", "b")
	built := aql.NewBuilder().From("COMPOSITION", "n").Contains(wa).Contains(wb)
	var writeCodes []string
	for _, f := range built.VerifyContainment(nil) {
		writeCodes = append(writeCodes, f.Code)
	}
	slices.Sort(writeCodes)

	if !slices.Equal(readCodes, writeCodes) {
		t.Fatalf("read codes = %v, write codes = %v, want equal (REQ-162 read/write parity)", readCodes, writeCodes)
	}
	// Non-vacuity, pinned to the exact expected shape rather than just
	// cross-engine equality (which a shared latent bug could satisfy
	// vacuously): ONE aql_impossible_containment from the always-correct
	// a→a1 pair (OBSERVATION→FOLDER, Never), plus ONE
	// aql_containment_by_reference from b's correct adjacency to a1
	// (FOLDER→COMPOSITION, ByReference) — not a second
	// aql_impossible_containment from b wrongly paired with a
	// (OBSERVATION→COMPOSITION, also Never), which is what `return self`
	// produces instead.
	if want := []string{"aql_containment_by_reference", "aql_impossible_containment"}; !slices.Equal(readCodes, want) {
		t.Fatalf("read codes = %v, want exactly %v", readCodes, want)
	}
}

// TestWalkEmptyClassNodeStaysTransparent is the read-side twin of
// openehr/aql/verify.go's TestVerifyContainmentEmptyClassNodeStaysTransparent.
//
// A class node carrying no RM type decides nothing, so it must pass its
// enclosing parent through rather than become the predecessor of what follows:
// its [semcheck.Operand] is the zero one, whose verdict is UnknownClass, so
// Suppresses is true and EVERY later pair in the chain is silently dropped —
// including a provable Never. That is the failure mode this pins, and it is
// worse than a missed advisory: the linter reports clean on a tree that holds a
// real defect.
//
// [isJunction] already covers the RM-type-less node WITH children (it is
// classified as a junction, which passes the parent through), so only the
// CHILDLESS shape reaches the class path — and no parsed document carries
// either, since the extractor records a gap rather than emitting a nameless
// class node. Hand-built only, hence this file.
func TestWalkEmptyClassNodeStaysTransparent(t *testing.T) {
	t.Parallel()

	// OBSERVATION o CONTAINS <no RM type> CONTAINS SECTION s, as ONE chain:
	// the nameless node is o's first child and SECTION its second, so SECTION's
	// predecessor must still be o — and OBSERVATION→SECTION is Never.
	empty := parse.Containment{Class: parse.ClassExpr{Alias: "x"}}
	sect := parse.Containment{Class: parse.ClassExpr{RMType: "SECTION", Alias: "s"}}
	root := parse.Containment{
		Class:    parse.ClassExpr{RMType: "OBSERVATION", Alias: "o"},
		Children: []parse.Containment{empty, sect},
	}

	cc := containCheck{ck: semcheck.New(nil)}
	cc.walk(semcheck.Operand{}, semcheck.RoleRoot, root)
	var got []string
	for _, iss := range cc.issues {
		got = append(got, iss.Code)
	}
	if want := []string{"aql_impossible_containment"}; !slices.Equal(got, want) {
		t.Fatalf("codes = %v, want exactly %v — a class node with no RM type must stay transparent; "+
			"the zero Operand it would otherwise contribute suppresses every pair built on it, so the "+
			"Never pair behind it vanishes and the query lints clean", got, want)
	}
}
