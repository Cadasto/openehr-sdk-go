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
// One alternative of `pathPredicate` is structurally OUTSIDE this corpus, and
// silently so unless said here: for `archetypePredicate` (`ARCHETYPE_HRID |
// PARAMETER`) the extractor routes the text to ClassExpr.Archetype /
// ParamArchetype and leaves Predicate empty, so the fidelity oracle below —
// which compares the re-parsed Predicate to the text — reads every such case as
// unfaithful. Adding an HRID or `$p` fragment therefore produces FALSE soundness
// failures rather than coverage. Those two alternatives are guarded by
// aql.ValidateArchetypeID and aql.ValidateValue and confronted in
// identifier_parity_test.go instead.
//
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
	"-- ",           // one that NO newline closes inside the text
	"--",            // SYM_DOUBLE_DASH, which is not a comment at all
	"a[at0001]/b=1", // a legal nested path predicate
	"a/b",           // a bare object path, so ` MATCHES ` can form a PARSEABLE splice
	" MATCHES ",     // without it no regex fragment can sit in a parseable splice
	",X::1",         // reaches nodePredicate's `(ID_CODE|AT_CODE) ',' TERM_CODE` form
	// TERM_CODE's display-name section is spelled WHOLE rather than as a
	// separate `|…|` fragment: reaching it by concatenation costs four
	// fragments and the generator stops at three, which is why the corpus
	// never built one and the quote-transparency defect stayed invisible.
	",X::1|n's|", // quote-, brace- and dash-transparent content
	",X::1|a]b|", // …but bracket-OPAQUE, so this `]` IS a delimiter
	`{/\/}`,      // a regex body ending in a backslash
	"{x}",        // a `{` that starts no regex: SYM_LEFT_CURLY, ordinary content
}

// predicateBases are the query shapes the corpus is spliced into.
//
// The base MATTERS, and getting it wrong made the soundness arm unable to fail
// for ANY guard: with nothing after the class expression, escaping text can only
// produce trailing garbage, which is a LOUD error the arm permits. Two
// properties are needed for the arm to be live — something AFTER the bracket to
// be swallowed, and a NEWLINE later in the query for a comment run to resume on.
var predicateBases = []struct {
	name    string
	prefix  string
	suffix  string
	shapeOK func(q *parse.Query) bool
}{
	{
		name:   "bare",
		prefix: "SELECT c/x FROM COMPOSITION c",
		shapeOK: func(q *parse.Query) bool {
			return q.From.Contains == nil && q.Where == nil
		},
	},
	{
		name:   "tailed and multi-line",
		prefix: "SELECT c/x FROM COMPOSITION c",
		suffix: " CONTAINS OBSERVATION o[at0001\n] WHERE c/y = 1",
		shapeOK: func(q *parse.Query) bool {
			return q.From.Contains != nil && q.Where != nil
		},
	},
}

// TestPredicateGuardAgreesWithTheParser generates the corpus and confronts both
// directions of the property against ParseQuery.
func TestPredicateGuardAgreesWithTheParser(t *testing.T) {
	corpus := generatedPredicates(3)
	var totalDiscriminating int
	for _, base := range predicateBases {
		t.Run(base.name, func(t *testing.T) {
			var checked, accepted, refused, live, discriminating int
			for _, text := range corpus {
				// A predicate carrying the emitter's own delimiters is the whole
				// point, but one that is empty or blank belongs to a different
				// rule (the blank-predicate refusal), so it is skipped here.
				if strings.TrimSpace(text) == "" {
					continue
				}
				checked++

				emitted, emitErr := emitWithPredicate(t, base.prefix+"[at0002]"+base.suffix, text)

				// What the naive splice WOULD have produced — the oracle both
				// directions are measured against.
				spliced := base.prefix + "[" + text + "]" + base.suffix
				reparsed, reparseErr := parse.ParseQuery(spliced)
				faithful := reparseErr == nil &&
					reparsed.From.Root.Predicate == text &&
					base.shapeOK(reparsed)
				if reparseErr == nil {
					// The splice PARSES, so the oracle has an opinion; a splice
					// that does not parse at all leaves both arms vacuous.
					live++
					if !faithful {
						// …and it parses as something ELSE, which is the only
						// shape the SOUNDNESS arm can ever fire on.
						discriminating++
					}
				}

				if emitErr == nil {
					accepted++
					// SOUNDNESS: accepted text may fail to parse (loud), but it
					// must never parse into something else.
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
			totalDiscriminating += discriminating
			t.Logf("confronted %d generated predicates: %d accepted, %d refused; "+
				"%d splices parse, of which %d parse as a DIFFERENT query",
				checked, accepted, refused, live, discriminating)
			if checked < 500 {
				t.Errorf("corpus collapsed to %d cases; the generator is no longer generating", checked)
			}
			if accepted == 0 || refused == 0 {
				t.Errorf("corpus is one-sided (%d accepted, %d refused); it cannot detect a guard that "+
					"always answers the same way", accepted, refused)
			}
			if live < 40 {
				t.Errorf("only %d of %d generated splices parse at all; the oracle has almost nothing "+
					"to discriminate", live, checked)
			}
		})
	}
	// The rail that was missing, and the one that matters. Counting guard
	// VERDICTS says nothing about whether the oracle can DISCRIMINATE: a corpus
	// whose splices either round-trip or fail to parse leaves the soundness arm
	// unable to fire for any guard implementation, while still reporting a
	// healthy accept/refuse split and a large case count.
	if totalDiscriminating == 0 {
		t.Errorf("no generated splice parses as a DIFFERENT query, so the soundness arm cannot fail " +
			"for ANY guard; the bases need something after the bracket to swallow and a later newline")
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
