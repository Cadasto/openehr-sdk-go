package parse_test

// REQ-119 · PROBE-090 — issue #103.
//
// The emission closure verified AFTER emission. Every other REQ-119 guard
// decides one position from the text handed to it; this one confronts the whole
// emitted query with the parser, which is the only way to reach a token
// boundary that depends on text the predicate does not contain.
//
// Two directions, as REQ-119 requires of every guard:
//
//   - SOUNDNESS — the cross-bracket residual is refused, and the refusal is the
//     STRUCTURAL arm (the silent class), not the parser's verdict.
//   - ANTI-TIGHTENING — nothing the parser reads back as the same query is
//     refused, including the AST ENCODINGS the parser is free to re-nest. This
//     arm is the one a naive AST comparison fails, so it carries named rows
//     rather than resting on the rest of the suite passing.

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// TestEmitRefusesACrossBracketRegexSubstitution is issue #103 exactly as filed.
//
// Both predicates pass ValidatePathPredicate on their own: neither escapes its
// own bracket by any condition a per-predicate scan can see. Together they do —
// `{/a\/}` completes a token inside the root bracket AND leaves the body
// reachable past it, so the lexer's longest match runs the body on until the
// `{/q/}` in the CONTAINED predicate closes it, swallowing the whole
// `CONTAINS OBSERVATION` term.
//
// Before verify-after-emit this emitted with err == nil and re-parsed with
// err == nil into a query with no containment at all.
func TestEmitRefusesACrossBracketRegexSubstitution(t *testing.T) {
	const rootPred = `x MATCHES {/a\/}`
	const nestedPred = `y MATCHES {/} AND w MATCHES {/q/}`

	// The premise: each predicate is individually clean, so the refusal below
	// cannot be credited to a per-position guard.
	if err := aql.ValidatePathPredicate(rootPred); err != nil {
		t.Fatalf("root predicate refused per-position (%v); the cross-bracket premise is gone", err)
	}
	if err := aql.ValidatePathPredicate(nestedPred); err != nil {
		t.Fatalf("nested predicate refused per-position (%v); the cross-bracket premise is gone", err)
	}

	q, err := parse.ParseQuery("SELECT c/x FROM COMPOSITION c CONTAINS OBSERVATION o")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	q.From.Root.Predicate = rootPred
	q.From.Contains.Class.Predicate = nestedPred

	out, err := q.Emit()
	if err == nil {
		t.Fatalf("Emit produced %q with err == nil — the substitution is silent again", out)
	}
	if !errors.Is(err, aql.ErrInvalidQuery) {
		t.Errorf("err = %v, want ErrInvalidQuery", err)
	}
	// The STRUCTURAL arm: the emitted text parses perfectly well, which is what
	// makes this the silent class rather than a loud malformation. A refusal on
	// the re-parse arm here would mean the test had stopped exercising #103.
	if !strings.Contains(err.Error(), "DIFFERENT query") {
		t.Errorf("err = %v, want the structural arm — the emitted text re-parses cleanly, "+
			"so a re-parse failure means this row no longer tests #103", err)
	}
	// The coordinate names where the trees part — here `from.root`, the class
	// whose predicate absorbed the text, which is reached before the swallowed
	// `from.class[1]` goes missing — and carries no values.
	if !strings.Contains(err.Error(), "from.") {
		t.Errorf("err = %v does not name the FROM coordinate that changed", err)
	}
	for _, leak := range []string{rootPred, nestedPred, "MATCHES", "ehr_id"} {
		if strings.Contains(err.Error(), leak) {
			t.Errorf("err = %v leaks predicate content (%q)", err, leak)
		}
	}
}

// TestEmitVerificationAcceptsEveryReNestedEncoding is the anti-tightening arm
// that a field-by-field AST comparison fails.
//
// The AST admits several ENCODINGS of one emitted text and the parser picks
// one: a containment junction's operands nest differently from a hand-built
// tree, a VERSION class ignores RMType, and HasPredicate is a read-side signal
// the emitter never consults. Each row below is a shape whose re-parse is NOT
// field-equal to the original and which MUST still emit, because the emitted
// text means exactly what the AST said.
func TestEmitVerificationAcceptsEveryReNestedEncoding(t *testing.T) {
	class := func(rmType, alias string) parse.Containment {
		return parse.Containment{Class: parse.ClassExpr{RMType: rmType, Alias: alias}}
	}
	junction := func(join parse.ContainsJoin, kids ...parse.Containment) parse.Containment {
		return parse.Containment{Children: kids, ChildJoin: join}
	}
	mk := func(from parse.FromClause) *parse.Query {
		return &parse.Query{
			Select: parse.SelectClause{Items: []parse.SelectItem{
				{Expr: parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{
					IdentifiedPath: aql.IdentifiedPath{Raw: "c/uid/value"},
				}}},
			}},
			From: from,
		}
	}

	chain := class("SECTION", "s")
	chain.Children = []parse.Containment{
		class("ACTION", "a"),
		junction(parse.ContainsOr, class("OBSERVATION", "o"), class("EVALUATION", "ev")),
	}

	rootJunction := junction(parse.ContainsAnd, class("OBSERVATION", "o"), class("EVALUATION", "ev"))

	for _, tc := range []struct {
		name string
		from parse.FromClause
		why  string
	}{
		{
			"junction operands re-nest",
			parse.FromClause{Root: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"}, Contains: &chain},
			"the parser groups the flattened chain its own way; the class SEQUENCE is what must match",
		},
		{
			"FROM-root junction",
			parse.FromClause{Junction: &rootJunction},
			"a junction node carries no class of its own, so it contributes no skeleton slot",
		},
		{
			"VERSION class ignores RMType",
			parse.FromClause{Root: parse.ClassExpr{Version: true, Alias: "v", Predicate: "LATEST_VERSION"}},
			"emitClassExpr writes the keyword and drops RMType; the re-parse reports RMType VERSION",
		},
		{
			"predicate without the read-side flag",
			parse.FromClause{Root: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c", Predicate: "at0001"}},
			"HasPredicate is false here and the emitter brackets on Predicate alone",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := mk(tc.from)
			out, err := q.Emit()
			if err != nil {
				t.Fatalf("Emit refused a legal encoding (%s): %v", tc.why, err)
			}
			// And the emitted text is genuinely the same query: re-emitting the
			// re-parse reproduces it, so the acceptance is not merely tolerated.
			re, err := parse.ParseQuery(out)
			if err != nil {
				t.Fatalf("emitted %q does not re-parse: %v", out, err)
			}
			again, err := re.Emit()
			if err != nil {
				t.Fatalf("re-emit refused: %v", err)
			}
			if again != out {
				t.Errorf("re-emit = %q, want %q", again, out)
			}
		})
	}
}

// TestEmitVerificationIsMutationDetectable pins the REQ's mutation rule for
// this guard by naming what must fail if the verification call is deleted from
// Emit: TestEmitRefusesACrossBracketRegexSubstitution above, and the loud rows
// of TestEmitClassPredicateAcceptsLoudMalformations, which now assert that Emit
// surfaces the parser's verdict rather than emitting text it cannot read.
//
// The check here is the property those depend on and which no other test states
// directly: Emit NEVER returns text that fails to re-parse.
func TestEmitNeverReturnsUnparseableText(t *testing.T) {
	// A corpus of predicates spanning the scanner's states, each spliced into a
	// base carrying material after the bracket — the shape a substitution needs.
	predicates := []string{
		"at0001",
		"a/b='c'",
		`a/b MATCHES {/a\/}`,
		`a/b MATCHES {/re/}`,
		"a/b='c' AND d/e='f'",
		"a b c",
		"a/b MATCHES {x}",
		`a/b MATCHES {/re/`,
		"a/b='c'--",
		"a/b='c' -- note\n",
		"at0001,SNOMED-CT::22298006|Barrett's oesophagus|",
	}
	const base = "SELECT c/x FROM COMPOSITION c[at0002] CONTAINS OBSERVATION o[at0003] WHERE c/y = 1"

	emitted, refused := 0, 0
	for _, p := range predicates {
		q, err := parse.ParseQuery(base)
		if err != nil {
			t.Fatalf("ParseQuery: %v", err)
		}
		q.From.Root.Predicate = p
		q.From.Root.PredicateComparison = nil

		out, err := q.Emit()
		if err != nil {
			refused++
			continue
		}
		emitted++
		if _, err := parse.ParseQuery(out); err != nil {
			t.Errorf("Emit returned text that does not re-parse for predicate %d: %v", len(p), err)
		}
	}
	// The corpus must exercise BOTH outcomes, or the property above is vacuous.
	if emitted == 0 || refused == 0 {
		t.Errorf("corpus is one-sided: %d emitted, %d refused — it can no longer show "+
			"the verification both accepting and refusing", emitted, refused)
	}
}
