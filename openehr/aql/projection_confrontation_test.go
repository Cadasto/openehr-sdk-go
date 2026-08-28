package aql_test

// REQ-163 · PROBE-088
//
// projection_confrontation_test.go holds the after-emission projection
// verification — [aql.Builder.Build]'s answer to REQ-163 § rule 2 — to the
// grammar it was hand-derived from, over a GENERATED corpus, with the real
// parser as the oracle.
//
// Why a generated one beside the committed rows. The verification is a lexical
// scan written by hand because openehr/aql cannot call openehr/aql/parse (parse
// imports aql, so the call would be an import CYCLE rather than a layering
// choice). A hand-written corpus therefore samples exactly the interleavings its
// author thought of, and REQ-119's own record is that this is not enough: three
// over-refusals shipped green there against a refusal-only corpus. The review
// this file answers found four more defects, each one fragment away from a row
// that already existed — a TOP direction the scan stepped over without
// recording, a call smuggled through the literal route, an aggregate in an
// argument slot, and a closed comment refused as an escape. Crossing the
// constructors with an alphabet of the characters that put the scanner into each
// of its states is what reaches the interleavings nobody writes down.
//
// The ORACLE is two-sided, and each side can fail on its own:
//
//   - ACCEPT ⇒ FIDELITY. If Build accepts and the emitted query PARSES, it must
//     read back as the structure the builder recorded — the item count, the star
//     flag, the clause-level DISTINCT and TOP flags INCLUDING the direction, and
//     each alias the builder fixed. Anything weaker would let a silent
//     substitution through: the whole class this rule exists to close re-parses
//     cleanly, and some of it re-emits byte-identically too.
//   - ACCEPT ⇒ IDENTITY, over the TYPED constructors. REQ-163 § Read-side
//     mirror duty makes `Build → ParseQuery → Emit` byte-identity a MUST for
//     every construct the REQ ADDS, and § Canonical spellings is what makes
//     identity reachable for them.
//   - REFUSE ⇒ [aql.ErrInvalidQuery]. A refusal must stay reachable as a
//     sentinel, never a bare error and never a panic.
//
// Two things the oracle deliberately does NOT assert, both for the same reason —
// [aql.Col] splices VERBATIM text and REQ-163 § rule 1 keeps it that way:
//
//   - that accepted text PARSES. Loud, ordinary malformation stays accepted by
//     design: the parser rejects it and the caller sees the error the moment
//     they read their own query back. Only the SILENT mode is refused, and the
//     silent mode is precisely the case where the emitted text parses into a
//     different STRUCTURE — which is what the fidelity arm tests.
//   - that a query carrying verbatim `Col` text re-emits to the same bytes. A
//     comment, a double-quoted literal and interior padding are all trivia the
//     canonical form normalises away, so `Col("-- note\nc/uid/value")` re-emits
//     without the comment. That is rule 1 working, not a defect: identity is the
//     bar for the spellings this REQ FIXES, and the verbatim carrier's spelling
//     is the caller's. The identity arm therefore runs on the rows marked
//     `identity` below, and the committed corpus in projection_test.go and
//     containment_roundtrip_test.go carries the rest.
//
// The generator's randomness is a FIXED seed, never time- or crypto-derived, so
// a failure reproduces from the construction the message prints.

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// projectionAlphabet is chosen for the SCANNER STATES each fragment reaches,
// not for being plausible AQL: a fragment that opens a quote without closing it,
// hides a comma inside a bracket, opens a comment its newline closes and one
// nothing closes, spells a clause keyword, spells a clause-level flag or a TOP
// direction, or is a bare delimiter.
var projectionAlphabet = []string{
	"c/uid/value",     // a benign path
	"c/from_date",     // a path whose segment CONTAINS a keyword's letters
	"c/look_backward", // …and one ENDING in a direction keyword's letters
	"n",               // a bare identifier, so the alias slots can be filled
	"*",               // the star, whose sole unaliased form reduces to the flag
	",",               // the item separator
	", c/b",           // …and a whole second item
	"'a, b'",          // a comma the literal makes content
	"'",               // an unterminated literal
	`"`,               // the other delimiter, unterminated
	"[at0001,'T']",    // a node predicate, whose commas are its own
	"[",               // a bracket nothing closes
	"]",               // one that closes nothing
	"(",               // the paren half of the same pair
	")",
	"COUNT(c/x)",   // a call the parser reads back as one item
	"-- n\n",       // a CLOSED comment: trivia the lexer skips
	"-- n",         // one NO newline closes inside the clause
	"--",           // SYM_DOUBLE_DASH, which is not a comment at all
	" AS n",        // the alias boundary
	" AS ",         // …with no alias after it
	"DISTINCT ",    // the clause-level flag the CLAUSE consumes
	"TOP 5 ",       // the deprecated clause-level row bound
	"FORWARD ",     // the TOP directions, which the review found unrecorded
	"BACKWARD ",    //
	" FROM EHR e2", // the clause opener that ends the projection
	" WHERE x = 1", // another
	" ORDER BY c/x",
	"{/re/}",  // a contained regex, matched whole or not at all
	"{",       // one still open at the end of the text
	"X::1|n|", // a TERM_CODE carrying a display name
	" ",       // bare trivia, so padding is exercised beside content
	"\n",
}

// recordedItem is what the BUILDER recorded for one projection item, spelled
// from the CONSTRUCTION rather than read back out of the builder — the recorded
// side of the comparison has to be independent of the scan it is confronting.
//
// aliasRecorded is false for a legacy [aql.Col] with no [aql.SelectField.As]
// call: where the `AS` boundary falls inside verbatim text is not something the
// builder fixed, so there is nothing for the emitted text to disagree with
// (REQ-163 § rule 1). Such an item matches whatever alias the text carries.
type recordedItem struct {
	expr          string
	alias         string
	aliasRecorded bool
}

// recordedShape is the whole projection the builder recorded, before the
// reduction the emitted text forces.
type recordedShape struct {
	distinct bool
	top      string // "", "TOP", "TOP FORWARD", "TOP BACKWARD"
	items    []recordedItem
}

// projectionCase is one constructor under confrontation: how it builds a query
// from a generated string, and what the builder therefore recorded.
type projectionCase struct {
	name  string
	build func(text string) (aql.Query, error)
	want  func(text string) recordedShape
	// identity marks a construction that splices NO verbatim [aql.Col] text
	// into the projection, so the emitted query is canonical and the byte-
	// identity arm applies to it (see the file comment).
	identity bool
}

// one, raw and typed spell the recorded shapes without repeating the literal:
// one item; a VERBATIM Col item, whose alias the builder did not fix; and a typed
// item, whose alias it did.
func one(it recordedItem) recordedShape { return recordedShape{items: []recordedItem{it}} }
func raw(text string) recordedItem      { return recordedItem{expr: strings.TrimSpace(text)} }
func typed(expr, alias string) recordedItem {
	return recordedItem{expr: expr, alias: alias, aliasRecorded: true}
}

// projectionCases feed the alphabet to every constructor REQ-163 adds, at the
// ITEM level and at the ARGUMENT level below it — the level with its own escape
// scan, and the level where the review found the arity smuggle. Three rows carry
// a clause-level flag the builder RECORDED, which is what lets the fidelity arm
// see a direction or a DISTINCT the text changes rather than merely adds.
var projectionCases = []projectionCase{
	{
		name:  "Col(_)",
		build: func(s string) (aql.Query, error) { return projectionQuery(aql.Col(s)) },
		want:  func(s string) recordedShape { return one(raw(s)) },
	},
	{
		name:  "Col(_).As(n)",
		build: func(s string) (aql.Query, error) { return projectionQuery(aql.Col(s).As("n")) },
		want:  func(s string) recordedShape { return one(typed(strings.TrimSpace(s), "n")) },
	},
	{
		name:  "ColAs(_, n)",
		build: func(s string) (aql.Query, error) { return projectionQuery(aql.ColAs(s, "n")) },
		want:  func(s string) recordedShape { return one(typed(strings.TrimSpace(s), "n")) },
	},
	{
		// Both slots are TYPED — the path is fixed and the generated text is the
		// alias, which is a single-token identifier position — so an accepted
		// case is canonical and identity binds.
		name:     "ColAs(c/x, _)",
		build:    func(s string) (aql.Query, error) { return projectionQuery(aql.ColAs("c/x", s)) },
		want:     func(s string) recordedShape { return one(typed("c/x", strings.TrimSpace(s))) },
		identity: true,
	},
	{
		name:  "Fn(CONCAT, Col(_))",
		build: func(s string) (aql.Query, error) { return projectionQuery(aql.Fn("CONCAT", aql.Col(s))) },
		want:  func(string) recordedShape { return one(typed("", "")) },
	},
	{
		name: "Fn(CONCAT, Col(c/a), Col(_))",
		build: func(s string) (aql.Query, error) {
			return projectionQuery(aql.Fn("CONCAT", aql.Col("c/a"), aql.Col(s)))
		},
		want: func(string) recordedShape { return one(typed("", "")) },
	},
	{
		// The generated text is the NAME, which is held to
		// [aql.ValidateSelectFuncName] and rendered in the canonical upper-case
		// ASCII spelling, so an accepted case is canonical.
		name:     "Fn(_, Col(c/x))",
		build:    func(s string) (aql.Query, error) { return projectionQuery(aql.Fn(s, aql.Col("c/x"))) },
		want:     func(string) recordedShape { return one(typed("", "")) },
		identity: true,
	},
	{
		name:  "Count(_)",
		build: func(s string) (aql.Query, error) { return projectionQuery(aql.Count(s)) },
		want:  func(string) recordedShape { return one(typed("", "")) },
	},
	{
		name:  "CountDistinct(_)",
		build: func(s string) (aql.Query, error) { return projectionQuery(aql.CountDistinct(s)) },
		want:  func(string) recordedShape { return one(typed("", "")) },
	},
	{
		// A projected literal renders through the canonical value spellings, so
		// whatever the generated text is, the emitted form is the canonical one.
		name:     "Lit(String(_))",
		build:    func(s string) (aql.Query, error) { return projectionQuery(aql.Lit(aql.String(s))) },
		want:     func(string) recordedShape { return one(typed("", "")) },
		identity: true,
	},
	{
		name: "Lit(Func(CONCAT, Path(_)))",
		build: func(s string) (aql.Query, error) {
			return projectionQuery(aql.Lit(aql.Func("CONCAT", aql.Path(s))))
		},
		want: func(string) recordedShape { return one(typed("", "")) },
	},
	{
		name:  "Star(), Col(_)",
		build: func(s string) (aql.Query, error) { return projectionQuery(aql.Star(), aql.Col(s)) },
		want: func(s string) recordedShape {
			return recordedShape{items: []recordedItem{typed("*", ""), raw(s)}}
		},
	},
	{
		name: "Distinct() + Col(_)",
		build: func(s string) (aql.Query, error) {
			return aql.Select(aql.Col(s)).Distinct().From("COMPOSITION", "c").Build()
		},
		want: func(s string) recordedShape {
			return recordedShape{distinct: true, items: []recordedItem{raw(s)}}
		},
	},
	{
		name: "Top(5) + Col(_)",
		build: func(s string) (aql.Query, error) {
			return aql.Select(aql.Col(s)).Top(5).From("COMPOSITION", "c").Build()
		},
		want: func(s string) recordedShape {
			return recordedShape{top: "TOP", items: []recordedItem{raw(s)}}
		},
	},
	{
		name: "TopDirected(5, BACKWARD) + Col(_)",
		build: func(s string) (aql.Query, error) {
			return aql.Select(aql.Col(s)).TopDirected(5, aql.TopBackward).From("COMPOSITION", "c").Build()
		},
		want: func(s string) recordedShape {
			return recordedShape{top: "TOP BACKWARD", items: []recordedItem{raw(s)}}
		},
	},
}

// projectionWitnesses seed the corpus with this review's own findings, so a
// generator that drifts still fails on the defects the file was written for.
var projectionWitnesses = []struct{ shape, text string }{
	{"Top(5) + Col(_)", "BACKWARD c/uid/value"},          // fix 1: the unrecorded direction
	{"TopDirected(5, BACKWARD) + Col(_)", "FORWARD c/x"}, // …and a recorded one the text reverses
	{"Col(_)", "BACKWARD c/x"},                           // …and the loud sibling with no TOP
	{"Lit(Func(CONCAT, Path(_)))", "a, b"},               // fix 2: the arity smuggle
	{"Fn(CONCAT, Col(_))", "c/a, c/b"},                   // …and its loud sibling
	{"Fn(_, Col(c/x))", "COUNT"},                         // fix 3: an aggregate by name
	{"Fn(_, Col(c/x))", "MIN"},                           //
	{"Col(_)", "-- note\nc/uid/value"},                   // fix 4: a closed comment is content
	{"Col(_)", "-- x\n*"},                                // …with the sole-star trap inside it
	{"Col(_)", "c/a-- x\nFROM EHR e2"},                   // …which must not launder a keyword
	{"Col(_)", "c/uid/value -- note"},                    // …while an OPEN run stays a flaw
	{"Col(_)", "DISTINCT c/uid/value"},                   // the flag half of rule 2
	{"Col(_)", "TOP 5 c/uid/value"},                      //
	{"Col(_)", "COUNT(c/uid/value) AS n"},                // rule 1's tolerated shape
	{"Col(_)", "*"},                                      // the sole-star reduction
	{"Fn(CONCAT, Col(_))", "c/a["},                       // a per-argument imbalance
}

// TestProjectionVerificationAgreesWithTheParser generates the corpus and
// confronts both directions of the oracle against ParseQuery.
func TestProjectionVerificationAgreesWithTheParser(t *testing.T) {
	// A FIXED seed. A failure must reproduce from the printed construction on
	// any machine on any day, which a time- or crypto-seeded generator cannot
	// promise.
	rng := rand.New(rand.NewPCG(0x163, 0x88))
	iterations := 12_000
	if testing.Short() {
		iterations = 2_000
	}

	var checked, accepted, refused, parsedOK, identityChecked int
	seen := map[string]bool{}

	confront := func(tc projectionCase, text string) {
		key := tc.name + "\x00" + text
		if seen[key] {
			return
		}
		seen[key] = true
		checked++
		construction := tc.name + " over " + strconv.Quote(text)

		q, buildErr := tc.build(text)
		if buildErr != nil {
			refused++
			// REFUSE ⇒ the sentinel stays reachable. A refusal a consumer
			// cannot classify is as unusable as none.
			if !errors.Is(buildErr, aql.ErrInvalidQuery) {
				t.Errorf("Build REFUSED %s with an error that does not wrap ErrInvalidQuery\n  error %v",
					construction, buildErr)
			}
			return
		}
		accepted++

		// ACCEPT ⇒ FIDELITY, on the accepted text that PARSES. Loud
		// malformation stays accepted by design (REQ-163 § rule 1), so a text
		// the parser rejects leaves this arm vacuous rather than failing it.
		emitted := q.String()
		reparsed, parseErr := parse.ParseQuery(emitted)
		if parseErr != nil {
			return
		}
		parsedOK++

		if why := compareProjection(tc.want(text), reparsed); why != "" {
			t.Errorf("Build ACCEPTED %s, but the emitted query reads back as a DIFFERENT projection: %s\n"+
				"  emitted %q", construction, why, emitted)
			return
		}
		again, emitErr := reparsed.Emit()
		if emitErr != nil {
			t.Errorf("Build ACCEPTED %s, and the query it emitted parses but will not re-emit\n"+
				"  emitted %q\n  error %v", construction, emitted, emitErr)
			return
		}
		if !tc.identity {
			return // a verbatim Col carrier; see the file comment
		}
		identityChecked++
		if again != emitted {
			t.Errorf("Build ACCEPTED %s, but the emitted query does not re-emit to the same bytes\n"+
				"  builder %s\n  parse   %s", construction, emitted, again)
		}
	}

	byName := map[string]projectionCase{}
	for _, tc := range projectionCases {
		byName[tc.name] = tc
	}
	for _, w := range projectionWitnesses {
		tc, ok := byName[w.shape]
		if !ok {
			t.Fatalf("witness names the construction %q, which the generator no longer builds", w.shape)
		}
		confront(tc, w.text)
	}

	for range iterations {
		tc := projectionCases[rng.IntN(len(projectionCases))]
		var sb strings.Builder
		for range 1 + rng.IntN(4) {
			sb.WriteString(projectionAlphabet[rng.IntN(len(projectionAlphabet))])
		}
		confront(tc, sb.String())
	}

	t.Logf("confronted %d generated projections: %d accepted, %d refused; %d of the accepted parse, "+
		"%d held to byte identity", checked, accepted, refused, parsedOK, identityChecked)
	// The rails, each answering a way this test could pass while proving
	// nothing.
	if checked < 1_000 {
		t.Errorf("corpus collapsed to %d cases; the generator is no longer generating", checked)
	}
	if accepted == 0 || refused == 0 {
		t.Errorf("corpus is one-sided (%d accepted, %d refused); it cannot detect a verification that "+
			"always answers the same way", accepted, refused)
	}
	if parsedOK < 100 {
		t.Errorf("only %d accepted projections parse at all, so the FIDELITY arm is nearly vacuous — it "+
			"can only fire on text the parser reads back", parsedOK)
	}
	if identityChecked < 20 {
		t.Errorf("only %d accepted projections reached the IDENTITY arm; the typed-constructor rows are "+
			"no longer producing canonical queries for it to hold", identityChecked)
	}
}

// compareProjection reports why the emitted query's projection is not the one
// the builder recorded, or "" when the two agree.
//
// The recorded side applies the ONE reduction REQ-163 § rule 2 inherits from
// REQ-119: a SOLE unaliased star item and the bare `SELECT *` flag are one
// encoding, and the flag with ZERO items is how the text re-parses. Applying it
// here rather than assuming it is what keeps the oracle from demanding a raw
// carrier count and failing the shape the REQ requires.
func compareProjection(want recordedShape, got *parse.Query) string {
	items, star := want.items, false
	for _, it := range items {
		if it.expr == "*" {
			star = true
		}
	}
	if len(items) == 1 && star && items[0].alias == "" {
		items = nil
	}

	switch {
	case got.Select.Distinct != want.distinct:
		return fmt.Sprintf("DISTINCT is %t and the builder recorded %t", got.Select.Distinct, want.distinct)
	case topOf(got) != want.top:
		return fmt.Sprintf("the TOP clause reads %q and the builder recorded %q", topOf(got), want.top)
	case got.Select.Star != star:
		return fmt.Sprintf("the star flag is %t and the builder recorded %t", got.Select.Star, star)
	case len(got.Select.Items) != len(items):
		return fmt.Sprintf("it carries %d item(s) and the builder recorded %d",
			len(got.Select.Items), len(items))
	}
	for i, it := range items {
		// An alias the builder did not FIX is not part of the recorded
		// structure — that is the whole of rule 1's leniency — so it matches
		// whatever the text carries.
		if it.aliasRecorded && got.Select.Items[i].Alias != it.alias {
			return fmt.Sprintf("item %d reads back with the alias %q and the builder recorded %q",
				i, got.Select.Items[i].Alias, it.alias)
		}
	}
	return ""
}

// topOf spells a parsed TOP clause the way [recordedShape.top] does: presence
// plus the DIRECTION, which is the half the scan used to step over.
func topOf(q *parse.Query) string {
	if q.Select.Top == nil {
		return ""
	}
	if dir := q.Select.Top.Dir.String(); dir != "" {
		return "TOP " + dir
	}
	return "TOP"
}
