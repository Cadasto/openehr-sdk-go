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

// TestEmitVerificationFactorsOutWhereEncodings is the WHERE half of the
// anti-tightening arm, and every row here was a real refusal before it existed.
//
// [aql.FormatWhere] renders no parentheses for a same-operator chain and the
// parser reads one back flat, while `aql.And` / `aql.Or` do NOT flatten on
// construction. So `aql.And(parsedWhere, extraFilter)` — the canonical
// inject-a-tenant-filter rewrite, and the population REQ-119 § binds — produces
// a nested tree whose emitted text is byte-identical to the flat form's.
func TestEmitVerificationFactorsOutWhereEncodings(t *testing.T) {
	eq := func(p string, n int64) aql.WhereExpr {
		return aql.Comparison{Path: p, Op: aql.OpEq, Val: aql.IntValue{N: n}}
	}
	const base = "SELECT c/uid/value FROM COMPOSITION c WHERE c/a = 1 AND c/b = 2"

	for _, tc := range []struct {
		name  string
		where func(parsed aql.WhereExpr) aql.WhereExpr
		want  string
		why   string
	}{
		{
			"and of a parsed and junction",
			func(p aql.WhereExpr) aql.WhereExpr { return aql.And(p, eq("c/z", 9)) },
			"SELECT c/uid/value FROM COMPOSITION c WHERE c/a = 1 AND c/b = 2 AND c/z = 9",
			"the rewrite pattern parse.Query is mutable for",
		},
		{
			"right-nested same operator",
			func(aql.WhereExpr) aql.WhereExpr {
				return aql.And(eq("c/a", 1), aql.And(eq("c/b", 2), eq("c/z", 9)))
			},
			"SELECT c/uid/value FROM COMPOSITION c WHERE c/a = 1 AND c/b = 2 AND c/z = 9",
			"associativity the emitted text erases",
		},
		{
			"single-term junction",
			func(aql.WhereExpr) aql.WhereExpr {
				return aql.Junction{Op: aql.OpAnd, Terms: []aql.WhereExpr{eq("c/a", 1)}}
			},
			"SELECT c/uid/value FROM COMPOSITION c WHERE c/a = 1",
			"Junction.validate accepts one term; only the constructors collapse it",
		},
		{
			"path LHS on the Left carrier",
			func(aql.WhereExpr) aql.WhereExpr {
				return aql.Compare(aql.Path("c/a"), aql.OpEq, aql.Path("c/b"))
			},
			"SELECT c/uid/value FROM COMPOSITION c WHERE c/a = c/b",
			"the parser reports a path LHS in Path; aql.Compare can put one in Left",
		},
		{
			"terminology operand with an untrimmed name",
			func(aql.WhereExpr) aql.WhereExpr {
				return aql.MatchesExpr{Path: "c/a", Terminology: &aql.FuncCall{
					Name: " terminology ",
					Args: []aql.Value{aql.String("openehr"), aql.String("compo"), aql.String("x")},
				}}
			},
			"SELECT c/uid/value FROM COMPOSITION c WHERE c/a MATCHES TERMINOLOGY('openehr', 'compo', 'x')",
			"the operand renderer trims; a skeleton that re-decides the choice refused it",
		},
		{
			"whitespace-only URI falls through to the value list",
			func(aql.WhereExpr) aql.WhereExpr {
				return aql.MatchesExpr{Path: "c/a", URI: "  ", Values: []aql.Value{aql.Int(1), aql.Int(2)}}
			},
			"SELECT c/uid/value FROM COMPOSITION c WHERE c/a MATCHES {1, 2}",
			"MatchesExpr.expr tests TrimSpace, not != \"\"",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, err := parse.ParseQuery(base)
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}
			q.Where = tc.where(q.Where)
			out, err := q.Emit()
			if err != nil {
				t.Fatalf("Emit refused a legal WHERE encoding (%s): %v", tc.why, err)
			}
			if out != tc.want {
				t.Errorf("Emit = %q, want %q", out, tc.want)
			}
			// And the three write paths must agree on the same predicate, which
			// is what § The emission closure property requires.
			if _, err := aql.FormatWhere(q.Where); err != nil {
				t.Errorf("FormatWhere refused what Emit accepted: %v", err)
			}
		})
	}
}

// TestEmitRefusesADroppedWhereClause is the other direction at the same slot:
// absence and UNREADABILITY render the same nothing, so without a presence
// coordinate they alias and a dropped WHERE passes as the same query.
//
// The trap is ordinary Go: a constructor returning a typed nil pointer lands in
// the interface as non-nil, `Emit` gates on the interface, and
// [aql.FormatWhere] renders "" for a value it cannot read — by design. The
// clause silently disappeared, which at this position is the `ehr_id` scoping
// filter.
func TestEmitRefusesADroppedWhereClause(t *testing.T) {
	q, err := parse.ParseQuery("SELECT c/uid/value FROM EHR e CONTAINS COMPOSITION c")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	var typedNil *aql.Comparison
	q.Where = typedNil

	out, err := q.Emit()
	if err == nil {
		t.Fatalf("Emit produced %q with err == nil; the WHERE clause was dropped silently", out)
	}
	if !errors.Is(err, aql.ErrInvalidQuery) {
		t.Errorf("err = %v, want ErrInvalidQuery", err)
	}
	if !strings.Contains(err.Error(), "where.presence") {
		t.Errorf("err = %v, want the where.presence coordinate", err)
	}
}

// TestEmitVerificationRefusesAnUnmodelledShape holds the skeleton FAIL-CLOSED on
// a WhereExpr it cannot read.
//
// A shape from outside the sealed set is reachable because the marker method is
// promoted through embedding, and it renders NOTHING — [aql.FormatWhere] returns
// "" for a value it cannot read. So it is refused at `where.presence` rather
// than at the unmodelled-token arm, which stands behind it for a future
// IN-package shape that dereferences cleanly but has no case here. Either way the
// refusal is the point: a constant token would make two different instances
// compare equal and the oracle would go blind at exactly the coordinate a newly
// added shape occupies — in the file that exists to be the last line of defence.
func TestEmitVerificationRefusesAnUnmodelledShape(t *testing.T) {
	q, err := parse.ParseQuery("SELECT c/uid/value FROM COMPOSITION c WHERE c/a = 1")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	q.Where = unmodelledWhere{}

	out, err := q.Emit()
	if err == nil {
		t.Fatalf("Emit produced %q for a WhereExpr the skeleton cannot model", out)
	}
	if !errors.Is(err, aql.ErrInvalidQuery) {
		t.Errorf("err = %v, want ErrInvalidQuery", err)
	}
	if !strings.Contains(err.Error(), "where.presence") {
		// The arm that fires, named exactly: an out-of-package shape renders
		// nothing, so presence catches it. A bare "where" also matched the
		// unmodelled arm's message, letting either arm's deletion pass unseen.
		t.Errorf("err = %v, want the where.presence coordinate", err)
	}
}

// unmodelledWhere satisfies [aql.WhereExpr] from outside the sealed set, which
// is reachable because the marker method is promoted through embedding.
type unmodelledWhere struct{ aql.WhereExpr }

// TestEmitSyntaxRefusalKeepsItsSentinel pins the machine-readable half of the
// re-parse arm: a caller branches on the sentinel, not on message text, and the
// parser's own message — which echoes the offending source — never rides along.
//
// The fixture is a CONTAINED loud malformation (`a b c`): the per-position
// guard accepts it by design (§ The class predicate positions reserves refusal
// for the silent mode), so the refusal below can only come from the re-parse
// arm of verifyEmitted. An earlier spelling used an unterminated string, which
// the guard refused BEFORE any text was built — the test passed without ever
// reaching the arm it pins, and deleting the ErrSyntax wrap kept the suite
// green.
func TestEmitSyntaxRefusalKeepsItsSentinel(t *testing.T) {
	const contained = "a b c"
	if verr := aql.ValidatePathPredicate(contained); verr != nil {
		t.Fatalf("fixture invalid: the guard refused %q (%v); this test must reach the "+
			"re-parse arm, not a per-position guard", contained, verr)
	}
	_, err := emitWithPredicate(t, "SELECT c/x FROM COMPOSITION c", contained)
	if err == nil {
		t.Fatal("Emit accepted text that cannot re-parse")
	}
	if !errors.Is(err, aql.ErrInvalidQuery) {
		t.Errorf("err = %v, want ErrInvalidQuery", err)
	}
	if !errors.Is(err, aql.ErrSyntax) {
		t.Errorf("err = %v, want aql.ErrSyntax riding along (the branchable sentinel)", err)
	}
	if !strings.Contains(err.Error(), "does not re-parse") {
		t.Errorf("err = %v, want the re-parse arm's own wording", err)
	}
	if strings.Contains(err.Error(), "a b c") {
		t.Errorf("err = %v echoes the offending source text", err)
	}
}

// TestEmitNeverReturnsUnparseableText pins the property the soundness rows rest
// on and which no other test states directly: Emit NEVER returns text that fails
// to re-parse.
//
// The REQ's mutation rule is satisfied for this guard by deleting the
// verifyEmitted call from Emit, which fails this test,
// TestEmitRefusesACrossBracketRegexSubstitution above, and the loud rows of
// TestEmitClassPredicateAcceptsLoudMalformations.
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
			t.Errorf("Emit returned text that does not re-parse for predicate %q: %v", p, err)
		}
	}
	// The corpus must exercise BOTH outcomes, or the property above is vacuous.
	if emitted == 0 || refused == 0 {
		t.Errorf("corpus is one-sided: %d emitted, %d refused — it can no longer show "+
			"the verification both accepting and refusing", emitted, refused)
	}
}

// TestEmitVerificationPinsEachCarriedCoordinate is the end-to-end half of the
// per-slot soundness table § Emission verified after emission requires: one
// substitution per splice-REACHABLE coordinate, preserving everything BEFORE
// it and diverging exactly there — so removing that slot from the skeleton
// fails a named row (the REQ's mutation rule, per coordinate rather than per
// call site).
//
// The vector is a SELECT item's Raw path — spliced verbatim by REQ-055 rule
// 3's contract — whose payload re-writes the query's tail and comments out the
// genuine one. That vector is exempt from the per-position guards BY the rule-3
// contract, which is exactly what makes it the right probe for the WHOLE-QUERY
// oracle. It starts AFTER the SELECT keyword's own modifiers, so it can only
// ADD text there (the DISTINCT row) and cannot rewrite a `TOP` the hand side
// already carries; the coordinates no splice reaches are pinned one-by-one at
// the unit level instead, by TestSkeletonDistinguishesEveryCoordinate.
func TestEmitVerificationPinsEachCarriedCoordinate(t *testing.T) {
	for _, tc := range []struct {
		name string
		base string // parsed to build the AST the payload then betrays
		raw  string // the SELECT item Raw payload; `--` comments out the genuine tail
		at   string // the coordinate the divergence must be reported at
	}{
		{
			"containment connective flipped AND to OR",
			"SELECT c/x FROM COMPOSITION c CONTAINS (OBSERVATION o AND EVALUATION ev)",
			"c/x FROM COMPOSITION c CONTAINS (OBSERVATION o OR EVALUATION ev) --",
			"from.contains.junction",
		},
		{
			"junction group negation dropped",
			"SELECT c/x FROM COMPOSITION c NOT CONTAINS (OBSERVATION o AND EVALUATION ev)",
			"c/x FROM COMPOSITION c CONTAINS (OBSERVATION o AND EVALUATION ev) --",
			"from.contains.junction",
		},
		{
			"junction rewritten as a chain",
			"SELECT c/x FROM COMPOSITION c CONTAINS (OBSERVATION o AND EVALUATION ev)",
			"c/x FROM COMPOSITION c CONTAINS OBSERVATION o CONTAINS EVALUATION ev --",
			"from.contains.junction",
		},
		{
			// The regrouping the in-order connective sequence could not see:
			// both parenthesisations read [CONTAINS, AND, OR] as a sequence,
			// while the structural skeleton keeps the OR at its coordinate.
			"junction operands re-grouped under the other operator",
			"SELECT c/x FROM COMPOSITION c CONTAINS (OBSERVATION o AND (EVALUATION ev OR INSTRUCTION i))",
			"c/x FROM COMPOSITION c CONTAINS (OBSERVATION o AND EVALUATION ev) OR INSTRUCTION i --",
			"from.contains.junction",
		},
		{
			// The deepest form: SAME top operator, same class sequence, same
			// connective multiset — only the association moved.
			"mixed association re-grouped under the same top operator",
			"SELECT c/x FROM COMPOSITION c CONTAINS ((OBSERVATION o AND EVALUATION ev) OR INSTRUCTION i)",
			"c/x FROM COMPOSITION c CONTAINS (OBSERVATION o OR (EVALUATION ev AND INSTRUCTION i)) --",
			"from.contains.op[0].junction",
		},
		{
			"DISTINCT injected",
			"SELECT c/x FROM COMPOSITION c",
			"DISTINCT c/x FROM COMPOSITION c --",
			"select.distinct",
		},
		{
			"FROM root class rewritten",
			"SELECT c/x FROM COMPOSITION c",
			"c/x FROM EHR c --",
			"from.root",
		},
		{
			"contained class rewritten",
			"SELECT c/x FROM COMPOSITION c CONTAINS OBSERVATION o",
			"c/x FROM COMPOSITION c CONTAINS EVALUATION o --",
			"from.contains.class",
		},
		{
			"WHERE clause dropped",
			"SELECT c/x FROM COMPOSITION c WHERE c/a = 1",
			"c/x FROM COMPOSITION c --",
			"where.presence",
		},
		{
			"WHERE junction flipped AND to OR",
			"SELECT c/x FROM COMPOSITION c WHERE c/a = 1 AND c/b = 2",
			"c/x FROM COMPOSITION c WHERE c/a = 1 OR c/b = 2 --",
			"where.junction",
		},
		{
			"WHERE leaf comparison rewritten",
			"SELECT c/x FROM COMPOSITION c WHERE c/a = 1 AND c/b = 2",
			"c/x FROM COMPOSITION c WHERE c/a = 1 AND c/b >= 2 --",
			"where.term[1]",
		},
		{
			"ORDER BY direction flipped",
			"SELECT c/x FROM COMPOSITION c ORDER BY c/x DESC",
			"c/x FROM COMPOSITION c ORDER BY c/x ASC --",
			"orderBy[0].direction",
		},
		{
			"ORDER BY path rewritten",
			"SELECT c/x FROM COMPOSITION c ORDER BY c/x DESC",
			"c/x FROM COMPOSITION c ORDER BY c/y DESC --",
			"orderBy[0].path text",
		},
		{
			"OFFSET dropped",
			"SELECT c/x FROM COMPOSITION c LIMIT 10 OFFSET 5",
			"c/x FROM COMPOSITION c LIMIT 10 --",
			"offset",
		},
		{
			"projection alias rewritten",
			"SELECT c/x AS col FROM COMPOSITION c",
			"c/x AS forged FROM COMPOSITION c --",
			"select.items[0].alias",
		},
		{
			"projection column added",
			"SELECT c/x FROM COMPOSITION c",
			"c/x, c/uid/value FROM COMPOSITION c --",
			"select item count",
		},
		{
			"projection star flipped on",
			"SELECT c/x FROM COMPOSITION c",
			"* FROM COMPOSITION c --",
			"select.star",
		},
		{
			"ORDER BY dropped",
			"SELECT c/x FROM COMPOSITION c ORDER BY c/x DESC",
			"c/x FROM COMPOSITION c --",
			"order by term count",
		},
		{
			"LIMIT dropped",
			"SELECT c/x FROM COMPOSITION c LIMIT 10",
			"c/x FROM COMPOSITION c --",
			"limit",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, err := parse.ParseQuery(tc.base)
			if err != nil {
				t.Fatalf("ParseQuery(%q): %v", tc.base, err)
			}
			q.Select.Items[0].Expr = parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{
				IdentifiedPath: aql.IdentifiedPath{Raw: tc.raw},
			}}
			out, err := q.Emit()
			if err == nil {
				t.Fatalf("the substitution emitted silently: %s", out)
			}
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("err = %v, want ErrInvalidQuery", err)
			}
			if !strings.Contains(err.Error(), tc.at) {
				t.Errorf("refused at the wrong coordinate:\n  err  %v\n  want %s", err, tc.at)
			}
		})
	}
}

// TestEmitVerificationAcceptsPaddedCarriers is the anti-tightening arm for the
// path-carrying slots: the primary constructors do not trim, the emitters
// render the padding, and the parser's span strips it on read-back — one
// ENCODING of the trimmed text, which MUST emit. Each row failed before
// pathToken normalised the slot ends.
func TestEmitVerificationAcceptsPaddedCarriers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(q *parse.Query)
	}{
		{"padded comparison LHS", func(q *parse.Query) {
			q.Where = aql.Eq(" c/a ", aql.Int(1))
		}},
		{"newline-padded comparison LHS", func(q *parse.Query) {
			q.Where = aql.Eq("c/a\n", aql.Int(1))
		}},
		{"padded EXISTS path", func(q *parse.Query) {
			q.Where = aql.Exists(" c/a ")
		}},
		{"padded LIKE path", func(q *parse.Query) {
			q.Where = aql.Like(" c/a ", aql.String("x%"))
		}},
		{"padded MATCHES path", func(q *parse.Query) {
			q.Where = aql.Matches(" c/a ", aql.Int(1))
		}},
		{"padded ORDER BY raw path", func(q *parse.Query) {
			q.OrderBy = []parse.OrderTerm{{Path: parse.IdentifiedPath{
				IdentifiedPath: aql.IdentifiedPath{Raw: " c/uid/value "},
			}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, err := parse.ParseQuery("SELECT c/x FROM COMPOSITION c")
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}
			tc.build(q)
			if _, err := q.Emit(); err != nil {
				t.Errorf("a padded carrier was refused (tightening): %v", err)
			}
		})
	}
}

// TestEmitVerificationCollapsesTheStarEncodings — `SELECT *` has two AST
// spellings: the bare `Star` flag and a sole unaliased [parse.StarExpr] item.
// The emitted text is identical and re-parses as the FLAG form, so both MUST
// reduce to the same skeleton; comparing the encodings refused the item form.
func TestEmitVerificationCollapsesTheStarEncodings(t *testing.T) {
	q, err := parse.ParseQuery("SELECT c/x FROM COMPOSITION c")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	q.Select.Star = false
	q.Select.Items = []parse.SelectItem{{Expr: parse.StarExpr{}}}
	out, err := q.Emit()
	if err != nil {
		t.Fatalf("a sole StarExpr item was refused (tightening): %v", err)
	}
	if !strings.Contains(out, "SELECT *") {
		t.Fatalf("emitted %q, want a bare star projection", out)
	}

	// The flag's OTHER ignored spelling: `Star: true` BESIDE populated items.
	// The emitter renders the items and never consults the flag, and the
	// re-parse reads the flag back false — so the skeleton must take the
	// EFFECTIVE star, not the raw field, or this legal query is refused.
	// (This is the row that fails if the emptiness half of that reduction is
	// dropped.)
	q2, err := parse.ParseQuery("SELECT c/x FROM COMPOSITION c")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	q2.Select.Star = true
	out2, err := q2.Emit()
	if err != nil {
		t.Fatalf("Star beside populated items was refused (tightening): %v", err)
	}
	if strings.Contains(out2, "*") {
		t.Fatalf("emitted %q; the emitter renders the items, never the ignored flag", out2)
	}
}
