package aql

// standing_internal_test.go reaches the unexported containment carrier to pin
// the REQ-163 standing-predicate guards at the BUILD seam, where the public
// combinator has already refused the same shapes at call time. The duplication
// is the point: [Containment.Predicated] is the only route in today, so without
// these rows the Build-side checks are dead code that a refactor could delete
// with every test still green. The public surface is covered from aql_test
// (standing_test.go).
// REQ-163 · PROBE-088

import (
	"errors"
	"strings"
	"testing"
)

// standingNode is a class node carrying a standing predicate, assembled
// directly rather than through [Containment.Predicated] so the combinator's own
// refusals do not pre-empt the Build-time ones under test.
func standingNode(rmType, archetypeID string) Containment {
	return Containment{
		rmType:       rmType,
		alias:        "x",
		archetypeID:  archetypeID,
		standingPred: &Comparison{Path: "uid/value", Op: OpEq, Val: Param("uid")},
	}
}

// TestPredicatedRecordsTheDefectOnTheNode pins the COMBINATOR's own half of
// each refusal, which no Build-level test can see on its own: two of the four
// are checked again at the Build seam ([Containment.validateStandingPredicate]),
// so deleting them from [Containment.Predicated] leaves every public refusal
// test still passing. Asserting the `invalid` field directly is what makes each
// one mutation-detectable — removing it fails THIS test by name.
//
// Recording the defect on the node, rather than returning an error, is the
// route the containment algebra already uses ([Containment.withChild]): the
// combinators return a Containment so they can be chained, so a misuse has no
// valid tree to return and must not be absorbed into a shape that means
// something else.
func TestPredicatedRecordsTheDefectOnTheNode(t *testing.T) {
	misuses := map[string]Containment{
		"junction receiver":    ContainsOr(Class("OBSERVATION", "o"), Class("EVALUATION", "ev")),
		"VERSION-spelled node": Class("VERSION", "v"),
		"node carrying an archetype predicate": Archetype("OBSERVATION", "o",
			"openEHR-EHR-OBSERVATION.body_temperature.v2"),
		"node already predicated": Class("COMPOSITION", "c").
			Predicated("name/value", OpEq, String("a")),
	}
	for name, recv := range misuses {
		t.Run(name, func(t *testing.T) {
			got := recv.Predicated("uid/value", OpEq, Param("uid"))
			if got.invalid == nil {
				t.Fatalf("Predicated recorded no defect on the node")
			}
			if !errors.Is(got.invalid, ErrInvalidQuery) {
				t.Errorf("recorded defect = %v, want ErrInvalidQuery", got.invalid)
			}
			// The refusal replaces the predicate, it does not accompany one: a
			// node that carried the bracket anyway would emit the very text the
			// refusal exists to keep off the wire if the defect were ever
			// dropped on the way to Build.
			if name != "node already predicated" && got.standingPred != nil {
				t.Errorf("the refused predicate was stored on the node anyway")
			}
			// …and on the one receiver that legitimately still carries a
			// bracket, WHICH comparison survived is the whole point of the
			// refusal. The rule is that a second predicate would silently drop
			// the FIRST, so a Predicated that refused and then overwrote anyway
			// would leave exactly the state the diagnostic says it prevented —
			// invisible to a nil check.
			if name == "node already predicated" {
				kept := got.standingPred
				switch {
				case kept == nil:
					t.Fatal("the refusal dropped the first predicate as well as the second")
				case kept.Path != "name/value", kept.Op != OpEq, !EqualValues(kept.Val, String("a")):
					t.Errorf("the second predicate overwrote the first: kept %q %q %q",
						kept.Path, string(kept.Op), FormatValue(kept.Val))
				}
			}
		})
	}
}

// TestPredicatedKeepsTheFirstDefect pins that a later refusal does not overwrite
// the diagnosis of an earlier one — [Containment.validateTree] reports the FIRST
// structural defect it reaches, and a combinator that clobbered `invalid` would
// hand the caller the second-order symptom instead of the cause.
func TestPredicatedKeepsTheFirstDefect(t *testing.T) {
	// The CONTAINS on a junction is the first defect; the standing predicate on
	// the same junction is the second.
	c := ContainsOr(Class("OBSERVATION", "o"), Class("EVALUATION", "ev")).
		Contains(Class("COMPOSITION", "c")).
		Predicated("uid/value", OpEq, Param("uid"))
	if c.invalid == nil {
		t.Fatal("no defect recorded")
	}
	if !strings.Contains(c.invalid.Error(), "CONTAINS below a containment junction") {
		t.Errorf("the later refusal overwrote the first defect: %v", c.invalid)
	}
}

// TestWithChildKeepsTheFirstDefect is [TestPredicatedKeepsTheFirstDefect]'s
// mirror across the OTHER combinator, and it is the direction the pair was
// missing: [Containment.Predicated] guards its write to `invalid`, so a defect
// recorded there must survive a later [Containment.Contains] on the same node.
//
// Without the guard the surviving diagnosis depends on combinator ORDER — the
// caller is handed the CONTAINS-on-a-junction message for a chain whose actual
// mistake was the standing predicate two calls earlier, i.e. the second-order
// symptom in place of the cause. Both messages are ErrInvalidQuery, so no
// errors.Is assertion can see the difference; only the text can.
//
// Delete the `if c.invalid == nil` guard in [Containment.withChild] and this
// test fails by name.
func TestWithChildKeepsTheFirstDefect(t *testing.T) {
	// The standing predicate on a junction is the FIRST defect; the CONTAINS on
	// the same junction is the second.
	c := ContainsOr(Class("OBSERVATION", "o"), Class("EVALUATION", "ev")).
		Predicated("uid/value", OpEq, Param("uid")).
		Contains(Class("COMPOSITION", "c"))
	if c.invalid == nil {
		t.Fatal("no defect recorded")
	}
	if !strings.Contains(c.invalid.Error(), "standing predicate on a containment junction") {
		t.Errorf("the later refusal overwrote the first defect: %v", c.invalid)
	}
	// …and it survives all the way to the caller, which is the seam that
	// matters: `invalid` is an internal field, the Build error is the surface.
	_, err := NewBuilder().Select(Col("x")).From("EHR", "e").Contains(c).Build()
	if !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("Build err = %v, want ErrInvalidQuery", err)
	}
	if !strings.Contains(err.Error(), "standing predicate on a containment junction") {
		t.Errorf("Build reported the second-order defect: %v", err)
	}
}

// TestStandingPredicateRefusedOnVersionNode pins REQ-163 § One carrier per
// grammar position at the Build seam: the VERSION alternative's bracket is
// `versionPredicate`, not `pathPredicate`, so a standing predicate has no
// position there — and the refusal names the constructor that does carry it.
func TestStandingPredicateRefusedOnVersionNode(t *testing.T) {
	for _, rmType := range []string{"VERSION", "version", "Version"} {
		t.Run(rmType, func(t *testing.T) {
			err := standingNode(rmType, "").validateTree(map[string]bool{})
			if !errors.Is(err, ErrInvalidQuery) {
				t.Fatalf("validateTree = %v, want ErrInvalidQuery", err)
			}
			if !strings.Contains(err.Error(), "aql.VersionCompare") {
				t.Errorf("refusal does not name the constructor that carries the shape: %v", err)
			}
		})
	}
}

// TestStandingPredicateSpellingFoldIsASCII pins the FOLD the VERSION-spelling
// test uses, which decides which of the two carriers a node is routed to.
//
// It is asked of [Containment.validateStandingPredicate] directly rather than
// through validateTree, because validateTree refuses a non-identifier RM type
// one check EARLIER ([validateRMTypeToken]) and that refusal would mask the
// answer this test is about.
//
// Three spelling classes, and the middle one is the one that matters:
//
//   - every ASCII casing of the keyword IS the keyword and routes away;
//   - `VERSIONED_COMPOSITION` — the class REQ-161's own suppression shape is
//     built on — merely STARTS with it, and must be judged on its own account.
//     A prefix test rather than a whole-token one would make the motivating
//     query unbuildable;
//   - `VERſION` (U+017F, which folds to `s`) is Unicode-fold-equal to the
//     keyword and is NOT it: the lexer's keyword fragments are ASCII, so under
//     [strings.EqualFold] a node the parser reads as an ordinary class would be
//     routed to the VERSION carrier, and the caller would be told to use a
//     constructor that fixes the type to something else entirely.
func TestStandingPredicateSpellingFoldIsASCII(t *testing.T) {
	routed := []string{"VERSION", "version", "Version"}
	for _, rmType := range routed {
		t.Run("routes "+rmType, func(t *testing.T) {
			err := standingNode(rmType, "").validateStandingPredicate()
			if !errors.Is(err, ErrInvalidQuery) {
				t.Fatalf("validateStandingPredicate(%q) = %v, want ErrInvalidQuery", rmType, err)
			}
		})
	}
	notKeyword := []string{"COMPOSITION", "VERSIONED_COMPOSITION", "VERſION"}
	for _, rmType := range notKeyword {
		t.Run("judges "+rmType+" on its own account", func(t *testing.T) {
			if err := standingNode(rmType, "").validateStandingPredicate(); err != nil {
				t.Fatalf("validateStandingPredicate(%q) = %v, want nil — the spelling is not the "+
					"VERSION keyword, so the standing bracket is this node's own", rmType, err)
			}
		})
	}
}

// TestStandingPredicateRefusedBesideArchetype pins the double-bracket refusal
// at the Build seam. [Containment.classToken] renders ONE of the two
// alternatives, so a node carrying both would emit valid AQL that asks a
// different question — the dropped predicate is a row filter, so the query
// returns more rows than the caller asked for.
func TestStandingPredicateRefusedBesideArchetype(t *testing.T) {
	err := standingNode("OBSERVATION", "openEHR-EHR-OBSERVATION.body_temperature.v2").
		validateTree(map[string]bool{})
	if !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("validateTree = %v, want ErrInvalidQuery", err)
	}
}

// TestStandingPredicateRendersOneBracket pins the emission side of that same
// rule: whatever validateTree lets through, [Containment.classToken] writes
// exactly one `[…]`. A renderer that concatenated two brackets would emit text
// the parser rejects; one that silently preferred a field would drop a filter.
func TestStandingPredicateRendersOneBracket(t *testing.T) {
	for name, c := range map[string]Containment{
		"standing alone": standingNode("COMPOSITION", ""),
		"standing + archetype": standingNode("OBSERVATION",
			"openEHR-EHR-OBSERVATION.body_temperature.v2"),
		"standing + version": {
			rmType: "VERSION", alias: "x",
			versionPred:  LatestVersion(),
			standingPred: &Comparison{Path: "uid/value", Op: OpEq, Val: Param("uid")},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if n := strings.Count(c.classToken(), "["); n != 1 {
				t.Fatalf("classToken() = %q carries %d brackets, want exactly 1", c.classToken(), n)
			}
		})
	}
}
