package parse_test

// REQ-119 · PROBE-090 — issue #99.
//
// predicate_confrontation_test.go holds [aql.ValidatePathPredicate] AND
// [aql.ValidateVersionPredicate] — through the class and VERSION brackets
// respectively — to the grammar they were hand-derived from, over a GENERATED
// corpus.
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
	`{/]\/}`,     // …with a BRACKET the escaped reading would expose
	"{/a/]/}",    // a bare `/` is no body char, so this `{` starts no regex
	"{x}",        // a `{` that starts no regex: SYM_LEFT_CURLY, ordinary content
	// `CONTAINED_REGEX`'s optional `(';' WS* STRING)?` TAIL. Every regex
	// fragment above stops at the body, so no concatenation could reach the
	// tail — and the token-boundary defect lived there: a SHORTER body whose
	// tail runs further yields the LONGER token, which is the one the lexer
	// takes. Spelled whole for the same reason the display-name fragment is.
	`{/a\/;'x/}'}`, // a shorter body reaching past a longer one via its tail
	`{/]/; 'i'}`,   // a bracket the tail must keep inside the token
	"{/x",          // a body still OPEN at the end of the text
	// `!=` is ONE operator (`SYM_NE`), and its `!` is the byte the operator
	// must open at — counted at the `=`, the `!` reads as a left operand.
	"!=",
	"!",
}

// predicateBases are the query shapes the corpus is spliced into.
//
// The base MATTERS, and getting it wrong made the soundness arm unable to fail
// for ANY guard: with nothing after the class expression, escaping text can only
// produce trailing garbage, which is a LOUD error the arm permits. Two
// properties are needed for the arm to be live — something AFTER the bracket to
// be swallowed, and a NEWLINE later in the query for a comment run to resume on.
// The BOTH-POSITIONS requirement is a property of the bases too. One struct
// field feeds two sub-grammars, so a corpus spliced only into a class bracket
// confronts only ValidatePathPredicate; PROBE-090 asks for the generated
// confrontation at the VERSION bracket as well, and the guards there differ —
// `versionPredicate` is held to its whole production, so the VERSION base's
// accept set is a strict subset of the class one and the tightening arm has
// something else to say.
//
// seed is the bracket text the base query is PARSED with, before the generated
// text replaces it: it must be valid in the position, which `at0002` is not in
// a VERSION bracket.
//
// This corpus deliberately carries JUNK operands (`!!a/b='c'`, `{`, `--`), so
// its arms are the two below and no more. "Accepted text must re-parse" cannot
// be asserted over it at either position: in the class position a contained
// malformation is permitted to be refused LOUDLY by the parser, and in the
// VERSION position the operands are exempt too — § The class predicate positions
// decides that position's SHAPE and leaves its OPERANDS loud. The closure claim
// for the VERSION shape therefore needs a corpus whose operands are legal BY
// CONSTRUCTION; that is TestVersionPredicateShapeAgreesWithTheParser below.
var predicateBases = []struct {
	name    string
	prefix  string
	seed    string
	suffix  string
	shapeOK func(q *parse.Query) bool
}{
	{
		name:   "bare",
		prefix: "SELECT c/x FROM COMPOSITION c",
		seed:   "at0002",
		shapeOK: func(q *parse.Query) bool {
			return q.From.Contains == nil && q.Where == nil
		},
	},
	{
		name:   "tailed and multi-line",
		prefix: "SELECT c/x FROM COMPOSITION c",
		seed:   "at0002",
		suffix: " CONTAINS OBSERVATION o[at0001\n] WHERE c/y = 1",
		shapeOK: func(q *parse.Query) bool {
			return q.From.Contains != nil && q.Where != nil
		},
	},
	{
		name:   "VERSION, tailed and multi-line",
		prefix: "SELECT v/x FROM VERSION v",
		seed:   "LATEST_VERSION",
		suffix: " CONTAINS COMPOSITION c[at0001\n] WHERE v/y = 1",
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

				emitted, emitErr := emitWithPredicate(t, base.prefix+"["+base.seed+"]"+base.suffix, text)

				// What the naive splice WOULD have produced — the oracle both
				// directions are measured against.
				spliced := base.prefix + "[" + text + "]" + base.suffix
				// …and the oracle is only ABOUT the emitted text while the two
				// agree. Nothing else asserts that, so a later change to how
				// Emit renders the clause would silently re-point every arm
				// below at a different string.
				if emitErr == nil && emitted != spliced {
					t.Fatalf("the oracle no longer describes what Emit produces:\n  emitted %q\n  spliced %q",
						emitted, spliced)
				}
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
			t.Logf("confronted %d generated predicates: %d accepted, %d refused; "+
				"%d splices parse, of which %d parse as a DIFFERENT query",
				checked, accepted, refused, live, discriminating)
			// Sized to the real corpus (depth 3 over the fragment list) rather
			// than to a token amount: at 500 this passed at depth 2, so a
			// generator regression surfaced only as a downstream symptom.
			if checked < 10_000 {
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
			// The rail that was missing, and the one that matters — asserted
			// per BASE, because a base that cannot discriminate contributes
			// nothing however well the others do. Counting guard VERDICTS says
			// nothing about whether the oracle can DISCRIMINATE: a corpus whose
			// splices either round-trip or fail to parse leaves the soundness
			// arm unable to fire for any guard implementation, while still
			// reporting a healthy accept/refuse split and a large case count.
			if discriminating == 0 {
				t.Errorf("no generated splice parses as a DIFFERENT query in this base, so the "+
					"soundness arm cannot fail here for ANY guard; the base needs something after "+
					"the bracket to swallow and a later newline (%d of %d parsed at all)", live, checked)
			}
			totalDiscriminating += discriminating
		})
	}
	if totalDiscriminating == 0 {
		t.Error("no base discriminates at all")
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

// versionOperandCorpus generates `versionPredicate` candidates whose OPERANDS
// are legal by construction, which is what makes the closure claim assertable
// there.
//
// The shared corpus above cannot carry it: its fragments are chosen for scanner
// states, so most concatenations have junk operands (`!!a/b='c'`), and § The
// class predicate positions leaves an operand-level malformation LOUD even at
// this position. Its arms therefore stop at "never a DIFFERENT query", and a
// hole in the SHAPE rule — which the § does decide — slips through: `!= 1`
// passed validation and `VERSION v[!= 1]` did not parse, invisibly, because the
// soundness arm only fires when the emitted text parses AND differs.
//
// Crossing legal `objectPath` spellings with every COMPARISON_OPERATOR spelling
// and legal `pathPredicateOperand`s removes that escape hatch: every accepted
// case must re-parse, full stop. The shape violations are then added
// deliberately — a missing operand, two comparisons, a junction — so the refusal
// side is exercised by the same generator rather than by a hand list.
func versionOperandCorpus() []string {
	lefts := []string{"a", "a/b", "commit_audit/time_committed", "a[at0001]/b", "a[b='c']/d"}
	ops := []string{"=", "!=", ">", ">=", "<", "<="}
	rights := []string{"1", "-1", "1.5", "'c'", `"c"`, "$p", "at0001", "id9", "a/b"}
	pads := []string{"", " ", "  ", "\t", "\n", " -- note\n"}

	var out []string
	for _, l := range lefts {
		for _, o := range ops {
			for _, r := range rights {
				for _, p := range pads {
					// The whole production, padded with every trivia form.
					out = append(out, l+p+o+p+r)
					// …and the shapes the production does NOT have, each built
					// from the same legal parts so only the SHAPE is at fault.
					out = append(out,
						p+o+p+r,             // no objectPath before the operator
						l+p+o+p,             // no pathPredicateOperand after it
						l+p+o+p+r+p+o+p+r,   // two comparisons, no junction to join them
						l+p+o+p+r+" AND "+l, // a junction the position has no alternative for
						l+p+o+p+r+" OR "+l,
					)
				}
			}
		}
	}
	// The two keyword alternatives, which must survive every trivia form.
	for _, kw := range []string{"LATEST_VERSION", "ALL_VERSIONS", "latest_version"} {
		for _, p := range pads {
			out = append(out, p+kw+p)
		}
	}
	return out
}

// TestVersionPredicateShapeAgreesWithTheParser confronts the VERSION guard over
// the legal-operand corpus, where BOTH directions are total: accepted text must
// re-parse (closure — no loud-operand exemption applies, the operands are legal)
// and refused text must not be text the parser reads back unchanged.
func TestVersionPredicateShapeAgreesWithTheParser(t *testing.T) {
	const (
		prefix = "SELECT v/x FROM VERSION v"
		seed   = "LATEST_VERSION"
		suffix = " CONTAINS COMPOSITION c[at0001\n] WHERE v/y = 1"
	)

	var checked, accepted, refused int
	for _, text := range versionOperandCorpus() {
		if strings.TrimSpace(text) == "" {
			continue
		}
		checked++

		emitted, emitErr := emitWithPredicate(t, prefix+"["+seed+"]"+suffix, text)
		spliced := prefix + "[" + text + "]" + suffix
		reparsed, reparseErr := parse.ParseQuery(spliced)
		faithful := reparseErr == nil &&
			reparsed.From.Root.Predicate == text &&
			reparsed.From.Contains != nil && reparsed.Where != nil

		if emitErr != nil {
			refused++
			// NO TIGHTENING, unchanged in meaning from the shared corpus.
			if faithful {
				t.Errorf("VERSION guard REFUSED %q, but the parser reads it back unchanged\n  error %v",
					text, emitErr)
			}
			continue
		}
		accepted++
		// CLOSURE. Every operand here is legal, so the only thing that can make
		// the emitted query unparseable is the SHAPE — which this position is
		// held to. `loud` is not an available answer.
		if _, err := parse.ParseQuery(emitted); err != nil {
			t.Errorf("VERSION guard ACCEPTED %q, but the emitted query does not parse — the operands "+
				"are legal by construction, so this is a SHAPE the production does not have\n"+
				"  emitted %q\n  error %v", text, emitted, err)
			continue
		}
		// …and IDENTITY, so an accepted case cannot quietly become another query.
		if !faithful {
			t.Errorf("VERSION guard ACCEPTED %q, but the emitted query re-parses as a DIFFERENT query\n"+
				"  emitted %q\n  root predicate %q", text, emitted, reparsed.From.Root.Predicate)
		}
	}

	t.Logf("confronted %d generated VERSION predicates: %d accepted, %d refused", checked, accepted, refused)
	if checked < 5_000 {
		t.Errorf("corpus collapsed to %d cases; the generator is no longer generating", checked)
	}
	// Two-sided by construction: the whole production is generated beside the
	// shapes it does not have, so a guard that always answers the same way fails.
	if accepted == 0 || refused == 0 {
		t.Errorf("corpus is one-sided (%d accepted, %d refused); it cannot detect a guard that "+
			"always answers the same way", accepted, refused)
	}
}
