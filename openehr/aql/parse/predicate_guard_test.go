package parse_test

// REQ-119 · PROBE-090 — issue #99.
//
// predicate_guard_test.go holds the EMISSION side of the bracket positions:
// [parse.ClassExpr.Predicate] is spliced verbatim, so text that terminates the
// bracket early re-parses as a DIFFERENT query with no error — REQ-119's
// silent-substitution class, the only failure mode that justifies refusing an
// operand which may itself be valid AQL.
//
// The extraction side (whitespace fidelity, round-trip identity) lives in
// predicate_parity_test.go.
//
// One struct field carries TWO grammar positions, and they do not have the same
// accept set:
//
//	classExprOperand : IDENTIFIER variable=IDENTIFIER? pathPredicate?
//	                 | VERSION variable=IDENTIFIER? ('[' versionPredicate ']')?
//	pathPredicate    : '[' (standardPredicate | archetypePredicate | nodePredicate) ']'
//	versionPredicate : LATEST_VERSION | ALL_VERSIONS | standardPredicate
//
// `versionPredicate` admits NO node predicate, so a guard that treats the field
// uniformly is wrong in both directions at once: it lets `VERSION v[at0001]`
// through (the parser rejects it) and it refuses `VERSION v[LATEST_VERSION]`
// (which the extractor itself produces). Both directions are asserted here.

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// emitWithPredicate parses base, overwrites the FROM root's standing predicate
// with text, and re-emits — the direct-construction path a consumer reaches by
// assembling or rewriting an AST by hand, which is what these guards bind.
func emitWithPredicate(t *testing.T, base, text string) (string, error) {
	t.Helper()
	q, err := parse.ParseQuery(base)
	if err != nil {
		t.Fatalf("ParseQuery(%q) = %v", base, err)
	}
	q.From.Root.Predicate = text
	q.From.Root.PredicateComparison = nil
	return q.Emit()
}

// TestEmitVersionPredicateHoldsItsOwnSubGrammar is the position split: the
// VERSION bracket is `versionPredicate`, not `pathPredicate`.
func TestEmitVersionPredicateHoldsItsOwnSubGrammar(t *testing.T) {
	const base = "SELECT v/data FROM VERSION v[LATEST_VERSION]"

	t.Run("accepts every versionPredicate alternative", func(t *testing.T) {
		// The POSITIVE CONTROL REQ-119 requires: this fails the moment the
		// class-position rule is applied here, which is the tightening
		// failure the REQ guards against as squarely as the splice.
		for _, ok := range []string{
			"LATEST_VERSION",
			"ALL_VERSIONS",
			// Case-insensitive: the lexer builds both keywords out of
			// case-insensitive letter fragments (L A T E S T '_' …).
			"latest_version",
			"all_versions",
			"Latest_Version",
			// standardPredicate : objectPath COMPARISON_OPERATOR pathPredicateOperand
			"commit_audit/time_committed > '2020'",
			"commit_audit/time_committed>'2020'",
			"commit_audit/committer/name = $who",
			// Every COMPARISON_OPERATOR spelling. `<=` and `>=` are ONE
			// operator, and a shape check that counts their characters reads
			// two comparisons and refuses AQL the parser accepts.
			"commit_audit/time_committed <= '2020'",
			"commit_audit/time_committed >= '2020'",
			"commit_audit/time_committed < '2020'",
			// An operator inside a literal or a nested bracket is not a
			// top-level one.
			"commit_audit/description = 'a=b'",
			"commit_audit[at0001]/time_committed = '2020'",
		} {
			out, err := emitWithPredicate(t, base, ok)
			if err != nil {
				t.Errorf("Emit with VERSION predicate %q = %v; want it accepted", ok, err)
				continue
			}
			if _, err := parse.ParseQuery(out); err != nil {
				t.Errorf("emitted %q does not re-parse: %v", out, err)
			}
		}
	})

	t.Run("refuses a shape standardPredicate does not have", func(t *testing.T) {
		// REQ-119 holds this position to its whole PRODUCTION, not to the
		// necessary condition a top-level comparison operator gives: with three
		// non-recursive alternatives the position's SHAPE is decidable in one
		// pass, so the closure clause governs and none of these may reach the
		// wire. Each is accepted in the CLASS position, where `nodePredicate`
		// makes the same text lawful or merely loud.
		for _, bad := range []string{
			"= 1",           // no objectPath before the operator
			"a/b =",         // no pathPredicateOperand after it
			"a/b = 1 = 2",   // two operators, and no junction alternative to join them
			"a/b = 1 b = 2", // two comparisons juxtaposed
		} {
			out, err := emitWithPredicate(t, base, bad)
			if err == nil {
				t.Errorf("Emit with VERSION predicate %q = %q, want an error; `standardPredicate` is "+
					"ONE `objectPath COMPARISON_OPERATOR pathPredicateOperand`", bad, out)
				continue
			}
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("Emit with VERSION predicate %q: error %v does not wrap ErrInvalidQuery", bad, err)
			}
		}
	})

	t.Run("refuses a node predicate the position cannot carry", func(t *testing.T) {
		// `versionPredicate` has no nodePredicate alternative, so these emit
		// text the SDK's own parser rejects — REQ-119's LOUD class, which the
		// closure property still forbids ("ParseQuery of the emitted text
		// MUST succeed").
		for _, bad := range []string{
			"at0001",
			"at0001,'name'",
			"$p",
			"openEHR-EHR-COMPOSITION.encounter.v1",
		} {
			out, err := emitWithPredicate(t, base, bad)
			if err == nil {
				t.Errorf("Emit with VERSION predicate %q = %q, want an error; "+
					"versionPredicate admits only LATEST_VERSION, ALL_VERSIONS and standardPredicate", bad, out)
				continue
			}
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("Emit with VERSION predicate %q: error %v does not wrap ErrInvalidQuery", bad, err)
			}
		}
	})
}

// TestEmitClassPredicateRefusesBracketEscape is issue #99's own case: the class
// bracket is spliced verbatim, so text that terminates it early re-parses as a
// DIFFERENT query with no error.
//
// The condition is exact rather than approximate. Emission writes
// `"[" + Predicate + "]"`, so the only way the text changes the query's
// STRUCTURE is by escaping that bracket; anything that stays inside can at
// worst be a malformed predicate, which the parser rejects loudly and which
// REQ-119 deliberately does not make a refusal.
func TestEmitClassPredicateRefusesBracketEscape(t *testing.T) {
	const base = "SELECT c/x FROM COMPOSITION c"

	t.Run("refuses text that escapes the bracket", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			text string
		}{
			{"issue_99_containment_splice", "a/b='c'] CONTAINS OBSERVATION o[d/e='f'"},
			{"bare_close", "a/b='c']"},
			{"close_then_where", "at0001] WHERE c/secret = 1"},
			// An unclosed `[` is a substitution too, not merely a loud error:
			// the emitter's own `]` closes the INNER bracket, leaving the
			// outer one to close on whatever `]` appears later in the query.
			{"unclosed_open", "a[at0001"},
			// An unterminated literal swallows the emitter's `]` as content
			// and runs on into the following clause.
			{"unterminated_string", "a/b='c"},
			{"unterminated_dq_string", `a/b="c`},
			{"unterminated_comment", "a/b='c' -- x"},
			{"term_code_name_carrying_a_bracket", "at0001,X::1|a]b|"},
			// The comment body cannot span a lone CR, so the `]` after one is
			// the class bracket's own delimiter and the text escapes.
			{"bare_cr_does_not_extend_the_comment", "a/b='c' -- x\r] AND d/e='f'\n"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				out, err := emitWithPredicate(t, base, tc.text)
				if err == nil {
					t.Fatalf("Emit with predicate %q = %q, want an error: the text escapes its bracket", tc.text, out)
				}
				if !errors.Is(err, aql.ErrInvalidQuery) {
					t.Errorf("error %v does not wrap ErrInvalidQuery", err)
				}
			})
		}
	})

	t.Run("accepts every bracket the grammar makes content", func(t *testing.T) {
		// The POSITIVE CONTROL: each of these carries a bracket or quote that
		// a naive scan miscounts, and each is legal AQL. A tightened guard
		// fails here rather than in some consumer's query.
		for _, tc := range []struct {
			name string
			text string
		}{
			{"nested_path_predicate", "a[at0001]/b='c'"},
			{"nested_twice", "a[at0001]/b[at0002]/c='d'"},
			{"bracket_in_regex_class", "a/b MATCHES {/[0-9]+/}"},
			{"brace_in_regex_quantifier", "a/b MATCHES {/a{2}/}"},
			{"regex_with_flags", "a/b MATCHES {/re/; 'i'}"},
			{"bracket_inside_string", "a/b=']'"},
			{"escaped_quote_then_bracket", `a/b='it\'s]'`},
			{"node_and", "a/b='c' AND d/e='f'"},
			{"at_code_with_name", "at0001,'name'"},
			{"param", "$p"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				out, err := emitWithPredicate(t, base, tc.text)
				if err != nil {
					t.Fatalf("Emit with predicate %q = %v; want it accepted", tc.text, err)
				}
				if _, err := parse.ParseQuery(out); err != nil {
					t.Errorf("emitted %q does not re-parse: %v", out, err)
				}
			})
		}
	})
}

// TestEmitClassPredicateGuardRefusesNothingTheParserAccepts is the tightening
// control stated as a PROPERTY rather than a list: every predicate the parser
// reads back must survive emission. This is the claim that separates the
// bracket-escape scan from the conservative sub-grammar approximation REQ-119
// warns against, so it is asserted over the whole round-trip corpus.
func TestEmitClassPredicateGuardRefusesNothingTheParserAccepts(t *testing.T) {
	for _, tc := range predicateCorpus {
		t.Run(tc.name, func(t *testing.T) {
			q, err := parse.ParseQuery(tc.in)
			if err != nil {
				t.Fatalf("ParseQuery(%q) = %v", tc.in, err)
			}
			if _, err := q.Emit(); err != nil {
				t.Errorf("the guard refused %q, which ParseQuery accepts: %v", tc.in, err)
			}
		})
	}
}

// TestEmitVersionPredicateRefusalNamesThePosition keeps the diagnostic useful:
// a caller who wrote a node predicate needs to be told the VERSION bracket is a
// different position, not merely that something was invalid.
func TestEmitVersionPredicateRefusalNamesThePosition(t *testing.T) {
	_, err := emitWithPredicate(t, "SELECT v/data FROM VERSION v[LATEST_VERSION]", "at0001")
	if err == nil {
		t.Fatal("want an error for a node predicate in the VERSION position")
	}
	if !strings.Contains(err.Error(), "VERSION") {
		t.Errorf("error %q does not mention the VERSION position", err)
	}
}

// TestEmitClassPredicateAcceptsLoudMalformations pins the negative space the
// generated confrontation structurally cannot reach.
//
// REQ-119 reserves refusal for the SILENT mode: text that stays inside its
// brackets can at worst be a malformed predicate, which the parser rejects
// loudly, and § The class predicate positions requires such text NOT be
// refused. The confrontation's no-tightening arm cannot check that — it fires
// only when the splice round-trips, and by definition none of these do. So the
// rule needs a control of its own or it is enforced by nothing.
//
// Every row here was REFUSED before the scanner distinguished a region that can
// consume the emitted `]` from one that merely fails to lex.
func TestEmitClassPredicateAcceptsLoudMalformations(t *testing.T) {
	const base = "SELECT c/x FROM COMPOSITION c[at0002]"

	for _, tc := range []struct{ name, text, why string }{
		{"bare words", "a b c", "no delimiter at all, just not a predicate"},
		{"brace that starts no regex", "a/b MATCHES {x}", "`{` falls back to SYM_LEFT_CURLY"},
		{"regex body never closes", "a/b MATCHES {/re", "the token cannot complete, so nothing is consumed"},
		{"regex closes but the token does not", "a/b MATCHES {/re/", "same: no CONTAINED_REGEX, no swallowing"},
		{"double dash is not a comment", "a/b='c'--x", "`--x` is SYM_DOUBLE_DASH; the `]` stays a delimiter"},
		{"comment body ending on a bare CR", "a/b='c' -- x\r", "`~[\\r\\n]*` stops at the CR and only `'\\r'? '\\n'` closes the token, so this is SYM_DOUBLE_DASH and nothing is left open"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := aql.ValidatePathPredicate(tc.text); err != nil {
				t.Fatalf("guard refused a CONTAINED malformation (%s): %v", tc.why, err)
			}
			out, err := emitWithPredicate(t, base, tc.text)
			if err != nil {
				t.Fatalf("Emit refused it: %v", err)
			}
			if !strings.Contains(out, "["+tc.text+"]") {
				t.Errorf("Emit = %q, which does not carry the predicate verbatim", out)
			}
			// …and the whole point of admitting it: the parser is the one that
			// says no, LOUDLY, where the caller sees it.
			if _, err := parse.ParseQuery(out); err == nil {
				t.Errorf("emitted %q parses cleanly — this row no longer tests a "+
					"loud malformation and needs replacing", out)
			}
		})
	}
}

// TestEmitClassPredicateGuardsEveryClassPosition — the guard is reached through
// the containment walk as well as the FROM root, and every test above writes
// only the root. A refactor that split the two paths would not be caught.
func TestEmitClassPredicateGuardsEveryClassPosition(t *testing.T) {
	const escape = "a/b='c'] CONTAINS OBSERVATION x[d/e='f'"

	for _, tc := range []struct {
		name string
		set  func(q *parse.Query)
	}{
		{"FROM root", func(q *parse.Query) { q.From.Root.Predicate = escape }},
		{"CONTAINS class", func(q *parse.Query) { q.From.Contains.Class.Predicate = escape }},
		{"nested CONTAINS class", func(q *parse.Query) {
			q.From.Contains.Children[0].Class.Predicate = escape
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, err := parse.ParseQuery(
				"SELECT c/x FROM COMPOSITION c CONTAINS OBSERVATION o CONTAINS CLUSTER cl")
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}
			tc.set(q)
			out, err := q.Emit()
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Fatalf("err = %v, want ErrInvalidQuery (emitted %q)", err, out)
			}
		})
	}
}
