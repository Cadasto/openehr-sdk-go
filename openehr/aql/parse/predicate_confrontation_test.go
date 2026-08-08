package parse_test

// REQ-119 · PROBE-090 — issue #99.
//
// predicate_confrontation_test.go holds [aql.ValidatePathPredicate] to the
// grammar it was hand-derived from, over a GENERATED corpus.
//
// REQ-119 § Acceptance forbids a hand-written corpus where a position has no
// token NAMES to walk, and this position has none: the guard's subject is
// bracket text, not a token, so there is nothing in AqlLexer.g4 to enumerate.
// The corpus is therefore a cross product of the fragments that put the scanner
// into each of its states — bracket, quote, regex, escape, comment — and the
// oracle is the real parser.
//
// The property is TWO-SIDED, because the guard is a necessary condition rather
// than a sub-grammar parser and each direction can fail on its own:
//
//   - SOUNDNESS (no silent substitution). If the guard ACCEPTS, the emitted
//     query must not quietly become a different one: re-parsing it either
//     fails loudly — which REQ-119 permits, since the caller sees the error
//     the moment they read their own query back — or returns the SAME
//     predicate text.
//   - NO TIGHTENING. If the guard REFUSES, the splice must genuinely have been
//     unfaithful. A refusal of text the parser reads back unchanged is the
//     tightening failure REQ-119 guards against as squarely as the splice, and
//     is the specific risk issue #99 raised against a conservative validator.

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// predicateFragments are chosen for the SCANNER STATES they exercise, not for
// being plausible AQL: a fragment that closes a bracket, opens one, opens a
// quote without closing it, hides a bracket inside a regex, escapes a
// delimiter, or opens a comment. Concatenations of them reach the interleavings
// a hand corpus does not think to write.
var predicateFragments = []string{
	"a/b='c'",       // a benign standard predicate
	"at0001",        // a node predicate
	"]",             // closes the emitter's bracket
	"[",             // opens one the emitter's `]` will close
	"'x'",           // a terminated literal
	"'",             // an unterminated one
	`"`,             // the other delimiter, unterminated
	"{/re/}",        // a contained regex
	"{/[/}",         // a bracket the regex makes content
	"{",             // an unterminated regex
	`\'`,            // an escape outside a literal
	" AND ",         // the keyword that needs surrounding whitespace
	"-- c\n",        // a comment, which the lexer SKIPS into the source text
	"a[at0001]/b=1", // a legal nested path predicate
}

// TestPredicateGuardAgreesWithTheParser generates the corpus and confronts both
// directions of the property against ParseQuery.
func TestPredicateGuardAgreesWithTheParser(t *testing.T) {
	var checked, accepted, refused int
	for _, text := range generatedPredicates(3) {
		// A predicate carrying the emitter's own delimiters is the whole
		// point, but one that is empty or blank belongs to a different rule
		// (the blank-predicate refusal), so it is skipped here.
		if strings.TrimSpace(text) == "" {
			continue
		}
		checked++

		emitted, emitErr := emitWithPredicate(t, "SELECT c/x FROM COMPOSITION c", text)

		// What the naive splice WOULD have produced — the oracle both
		// directions are measured against.
		spliced := "SELECT c/x FROM COMPOSITION c[" + text + "]"
		reparsed, reparseErr := parse.ParseQuery(spliced)
		faithful := reparseErr == nil &&
			reparsed.From.Root.Predicate == text &&
			reparsed.From.Contains == nil &&
			reparsed.Where == nil

		if emitErr == nil {
			accepted++
			// SOUNDNESS: accepted text may fail to parse (loud), but it must
			// never parse into something else.
			if _, err := parse.ParseQuery(emitted); err == nil && !faithful {
				t.Errorf("guard ACCEPTED %q, but the emitted query re-parses as a DIFFERENT query\n"+
					"  emitted %q\n  root predicate %q", text, emitted, reparsed.From.Root.Predicate)
			}
			continue
		}
		refused++
		// NO TIGHTENING: a refusal must correspond to a real defect.
		if faithful {
			t.Errorf("guard REFUSED %q, but the parser reads it back unchanged — a tightening failure\n"+
				"  error %v", text, emitErr)
		}
	}
	t.Logf("confronted %d generated predicates: %d accepted, %d refused", checked, accepted, refused)
	if checked < 500 {
		t.Errorf("corpus collapsed to %d cases; the generator is no longer generating", checked)
	}
	if accepted == 0 || refused == 0 {
		t.Errorf("corpus is one-sided (%d accepted, %d refused); it cannot detect a guard that always "+
			"answers the same way", accepted, refused)
	}
}

// generatedPredicates returns every concatenation of up to n fragments.
func generatedPredicates(n int) []string {
	out := []string{}
	cur := []string{""}
	for range n {
		next := make([]string, 0, len(cur)*len(predicateFragments))
		for _, prefix := range cur {
			for _, f := range predicateFragments {
				next = append(next, prefix+f)
			}
		}
		out = append(out, next...)
		cur = next
	}
	return out
}
