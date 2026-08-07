package parse_test

// REQ-119 · PROBE-090
//
// emit_parity_test.go holds the read/write side to the same refusals. Each
// test here compares a write-side guard in `openehr/aql` against what the
// grammar (and its emitter) actually does, so a guard cannot drift into
// refusing what the parser accepts, or accepting what it rejects.
//
// The write side cannot do these checks itself: `openehr/aql` may not import
// the generated lexer — the dependency runs the other way — so `openehr/aql`
// carries hand-derived rules and this package is the only place they can be
// mechanically confronted with the grammar.

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// TestMatchesURIGuardTracksTheGrammar — the `MATCHES {uri}` operand guard in
// [aql.MatchesExpr.validate] is a hand-derived rule that follows the URI token's
// own decomposition. This confronts it with the grammar.
//
// The property is NOT "the guard refuses what the parser refuses". A brace in
// the operand closes it early, and the text that follows can itself be valid
// AQL — `{uri://a} OR c/y MATCHES {uri://b}` parses perfectly well, as a
// DIFFERENT query. Parseability therefore cannot separate a good operand from
// an injected one.
//
// The property that does is ROUND-TRIP IDENTITY: whatever the builder agrees
// to emit MUST come back as one MATCHES predicate carrying the same URI. Every
// operand the guard accepts is held to that; every operand it refuses is
// confirmed to violate it, so the guard is shown to be neither too strict nor
// too permissive against the thing that actually matters.
func TestMatchesURIGuardTracksTheGrammar(t *testing.T) {
	for _, uri := range []string{
		// Outside the URI token's alphabet — must not survive the round trip.
		"uri://a} OR c/y MATCHES {uri://b",
		"http://example.org/x}",
		"http://example.org/{x}",
		"http://example.org/a b",
		"http://example.org/<x>",
		"http://example.org/a|b",
		`http://example.org/a\b`,
		"http://example.org/a\"b",
		"ht_tp://example.org",
		"not a uri at all",
		"://example.org",
		"1http://example.org",
		// IN the flat union of the delimiter sets, but NOT spellable positionally
		// — the class the first, alphabet-only guard admitted and emitted as AQL
		// this SDK's own parser rejects. `%` leads URI_PCT_ENCODED and needs two
		// hex digits; `[`/`]` occur only as an URI_IP_LITERAL host, whose
		// URI_IPV6_LITERAL requires full quads either side of a mandatory `::`;
		// `#` separates the fragment once and appears nowhere inside it.
		"http://example.org/a%zz",
		"http://example.org/a%2",
		"http://example.org/a%",
		"http://example.org/%",
		"http://example.org/a[b]",
		"http://example.org/a]b",
		"http://example.org/p#a#b",
		"http://[::1]/p",
		"http://[2001:0db8]/p",
		"http://[2001:0db8::1]/p",
		"http://[2001:0db8::0001/p",
		// Spellable as a URI, but TERM_CODE is declared first and wins the tie,
		// and matchesOperand admits URI only.
		"a::b",
		"SNOMED-CT::73211009",
		"SNOMED-CT(2026)::73211009",
		"SNOMED-CT::73211009|diabetes mellitus|",
		// Spellable URI tokens — must survive it unchanged.
		"http://example.org/path",
		"http://example.org/p?a=1&b=2",
		"http://example.org/p?a=1?b=2",
		"http://example.org/p#frag",
		"http://example.org/p#frag?q=1",
		"http://example.org/'",
		"http://example.org/a!$&'()*+,;=",
		"http://example.org/a%20b",
		"http://example.org/a%2Fb",
		"http://example.org:8080/p",
		"http://example.org:/p",
		"http://user:pw@example.org/p",
		"http://[2001:0db8::0001]/p",
		"http://[2001:0db8:0000::0000:0001]:8080/p",
		"terminology://openehr.org/subsets/SNOMED-CT",
		"urn:ietf:rfc:3986",
		"mailto:someone@example.org",
		"svn+ssh://example.org/repo",
		"http:",
		"file:///etc/hosts",
	} {
		t.Run(uri, func(t *testing.T) {
			_, guardErr := aql.FormatWhere(aql.MatchesURI("c/y", uri))

			// Does the operand survive emission and re-parse as itself?
			survives, back := false, "<parse error>"
			doc, err := parse.ParseQuery(
				"SELECT c/x FROM COMPOSITION c WHERE c/y MATCHES {" + uri + "}")
			if err == nil {
				m, ok := doc.Where.(aql.MatchesExpr)
				survives = ok && m.URI == uri
				back = fmt.Sprintf("%#v", doc.Where)
			}

			switch {
			case guardErr == nil && !survives:
				t.Errorf("the builder emits MATCHES {%s}, but it does not come back as "+
					"one MATCHES on that URI (re-parsed as %s, err %v)\n"+
					"the URI guard in openehr/aql/where.go is too permissive", uri, back, err)
			case guardErr != nil && survives:
				t.Errorf("the builder refuses MATCHES {%s}, but it round-trips intact: %v\n"+
					"the URI guard in openehr/aql/where.go is too strict", uri, guardErr)
			}
		})
	}
}

// TestEmitRefusesMisplacedJunction — REQ-117 requires a containment junction
// to END its chain: the grammar takes a parenthesised group as a whole
// `containsExpr`, so no CONTAINS keyword may follow one.
//
// [aql.Builder] has refused this since REQ-117, but [Query.Emit] did not, and
// [Query] explicitly blesses direct construction — so a consumer assembling or
// rewriting an AST by hand got `… CONTAINS (A OR B) CONTAINS C` with err ==
// nil, text the SDK's own parser rejects. The extractor cannot produce such a
// tree (it only mirrors AQL that already parsed), which is why the gap
// survived: no round-trip case can reach it.
func TestEmitRefusesMisplacedJunction(t *testing.T) {
	class := func(rmType, alias string) parse.Containment {
		return parse.Containment{Class: parse.ClassExpr{RMType: rmType, Alias: alias}}
	}
	junction := func(join parse.ContainsJoin, kids ...parse.Containment) parse.Containment {
		return parse.Containment{Children: kids, ChildJoin: join}
	}
	// The chain under the FROM root: SECTION s CONTAINS (o OR ev) CONTAINS a.
	// Children of a class node are one FLATTENED chain, so the junction sits
	// in a non-final position and a CONTAINS follows it.
	misplaced := class("SECTION", "s")
	misplaced.Children = []parse.Containment{
		junction(parse.ContainsOr, class("OBSERVATION", "o"), class("EVALUATION", "ev")),
		class("ACTION", "a"),
	}

	// The same operands with the junction LAST — the legal shape, which must
	// still emit and re-parse, so the guard is not simply refusing junctions.
	wellPlaced := class("SECTION", "s")
	wellPlaced.Children = []parse.Containment{
		class("ACTION", "a"),
		junction(parse.ContainsOr, class("OBSERVATION", "o"), class("EVALUATION", "ev")),
	}

	mk := func(c parse.Containment) *parse.Query {
		return &parse.Query{
			Select: parse.SelectClause{Items: []parse.SelectItem{
				{Expr: parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{
					IdentifiedPath: aql.IdentifiedPath{Raw: "c/uid/value"},
				}}},
			}},
			From: parse.FromClause{
				Root:     parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"},
				Contains: &c,
			},
		}
	}

	// A child whose OWN CHAIN merely ENDS in a junction is still followed by the
	// next element's CONTAINS keyword, so it is misplaced too. Nothing exercised
	// chainEndsInJunction's recursive arm, and deleting it left the suite green.
	chainTail := class("SECTION", "s")
	chainTail.Children = []parse.Containment{
		func() parse.Containment {
			mid := class("OBSERVATION", "o")
			mid.Children = []parse.Containment{
				junction(parse.ContainsOr, class("CLUSTER", "cl1"), class("CLUSTER", "cl2")),
			}
			return mid
		}(),
		class("ACTION", "a"),
	}

	// Three levels down, so the walk's recursion is exercised rather than just
	// its first frame.
	deep := class("SECTION", "s")
	deep.Children = []parse.Containment{func() parse.Containment {
		l2 := class("OBSERVATION", "o")
		l2.Children = []parse.Containment{func() parse.Containment {
			l3 := class("CLUSTER", "cl")
			l3.Children = []parse.Containment{
				junction(parse.ContainsOr, class("ELEMENT", "e1"), class("ELEMENT", "e2")),
				class("ACTION", "a"),
			}
			return l3
		}()}
		return l2
	}()}

	for name, tree := range map[string]parse.Containment{
		"junction mid-chain":          misplaced,
		"chain tail is a junction":    chainTail,
		"misplaced three levels down": deep,
	} {
		t.Run("refused/"+name, func(t *testing.T) {
			out, err := mk(tree).Emit()
			if err == nil {
				t.Fatalf("Emit produced %q; a junction may only end a containment chain", out)
			}
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("err = %v, want ErrInvalidQuery", err)
			}
		})
	}

	// Inside a FROM-ROOT junction's operand. Nothing walked From.Junction, so
	// deleting that call also left the suite green.
	t.Run("refused/inside a root junction operand", func(t *testing.T) {
		operand := class("SECTION", "s")
		operand.Children = []parse.Containment{
			junction(parse.ContainsOr, class("OBSERVATION", "o"), class("EVALUATION", "ev")),
			class("ACTION", "a"),
		}
		root := junction(parse.ContainsAnd, operand, class("CLUSTER", "cl"))
		q := mk(class("COMPOSITION", "c2"))
		q.From = parse.FromClause{Junction: &root}
		out, err := q.Emit()
		if err == nil {
			t.Fatalf("Emit produced %q; the misplacement inside a root junction operand went unseen", out)
		}
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Errorf("err = %v, want ErrInvalidQuery", err)
		}
	})

	t.Run("legal shape still emits", func(t *testing.T) {
		out, err := mk(wellPlaced).Emit()
		if err != nil {
			t.Fatalf("Emit refused a junction in final position: %v", err)
		}
		if _, err := parse.ParseQuery(out); err != nil {
			t.Errorf("emitted %q does not re-parse: %v", out, err)
		}
	})

	// The read and write sides must refuse the SAME tree — the parity claim
	// carried in aql/containment.go. Spelling the misplaced shape through the
	// builder must fail too.
	t.Run("builder refuses the same shape", func(t *testing.T) {
		_, err := aql.NewBuilder().
			Select(aql.Col("c/uid/value")).
			From("COMPOSITION", "c").
			Contains(aql.Class("SECTION", "s").
				Contains(aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev"))).
				Contains(aql.Class("ACTION", "a"))).
			Build()
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Errorf("builder err = %v, want ErrInvalidQuery", err)
		}
	})
}

// TestEmitRefusesIncompleteContainment — REQ-119. [aql.Containment.validateTree]
// refuses a class node without an RM type and alias; Emit had no counterpart, so
// the read/write parity claim held only for the junction-placement rule.
//
// It matters twice over, because the read side infers the node KIND from an
// absent RMType: an incomplete class node otherwise looks like a junction, so it
// either emitted a dangling `CONTAINS ` or got diagnosed as a misplaced junction
// the caller never wrote. Completeness is therefore checked first.
func TestEmitRefusesIncompleteContainment(t *testing.T) {
	class := func(rmType, alias string) parse.Containment {
		return parse.Containment{Class: parse.ClassExpr{RMType: rmType, Alias: alias}}
	}
	mk := func(c parse.Containment) *parse.Query {
		return &parse.Query{
			Select: parse.SelectClause{Items: []parse.SelectItem{
				{Expr: parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{
					IdentifiedPath: aql.IdentifiedPath{Raw: "c/uid/value"},
				}}},
			}},
			From: parse.FromClause{
				Root:     parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"},
				Contains: &c,
			},
		}
	}

	zeroWithJoin := parse.Containment{ChildJoin: parse.ContainsOr}
	// A class node carrying a ChildJoin: emitContainment renders the children as
	// a plain CONTAINS chain, so the OR was silently DROPPED with err == nil.
	joinOnClass := class("SECTION", "s")
	joinOnClass.ChildJoin = parse.ContainsOr
	joinOnClass.Children = []parse.Containment{class("OBSERVATION", "o"), class("EVALUATION", "ev")}
	// A child that is incomplete, to exercise the recursion.
	incompleteChild := class("SECTION", "s")
	incompleteChild.Children = []parse.Containment{{}}

	for name, tree := range map[string]parse.Containment{
		"zero value":           {},
		"zero value with join": zeroWithJoin,
		"ChildJoin on a class": joinOnClass,
		"incomplete child":     incompleteChild,
	} {
		t.Run(name, func(t *testing.T) {
			out, err := mk(tree).Emit()
			if err == nil {
				t.Fatalf("Emit produced %q; the tree has no emittable spelling", out)
			}
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("err = %v, want ErrInvalidQuery", err)
			}
		})
	}

	// Two shapes the completeness check must NOT mistake for an incomplete class:
	// a VERSION class expr carries no RMType by design, and a node with no class
	// but at least one child IS the junction encoding (a single-operand junction
	// is legal, and emits as one parenthesised group).
	for name, tree := range map[string]parse.Containment{
		"VERSION class":           {Class: parse.ClassExpr{Version: true, Alias: "v"}},
		"single-operand junction": {Children: []parse.Containment{class("OBSERVATION", "o")}},
		"two-operand junction": {
			ChildJoin: parse.ContainsOr,
			Children:  []parse.Containment{class("OBSERVATION", "o"), class("EVALUATION", "ev")},
		},
	} {
		t.Run("emits/"+name, func(t *testing.T) {
			out, err := mk(tree).Emit()
			if err != nil {
				t.Fatalf("Emit refused a legal containment: %v", err)
			}
			if _, err := parse.ParseQuery(out); err != nil {
				t.Errorf("emitted %q does not re-parse: %v", out, err)
			}
		})
	}
}

// TestEmitValidatesSelectValuePositions — REQ-119's closure MUST names
// [parse.Query.Emit], but the SELECT arm rendered through [aql.FormatValue],
// which has no error to return and so validates nothing. Every value guard the
// requirement adds therefore bound WHERE and not SELECT.
//
// `NaN` is the case that matters most: it PARSES, as a path named `NaN` — the
// silent substitution REQ-119 singles out as materially worse than a loud
// refusal, reached through the write path the requirement explicitly names.
func TestEmitValidatesSelectValuePositions(t *testing.T) {
	mk := func(e parse.SelectExpr) *parse.Query {
		return &parse.Query{
			Select: parse.SelectClause{Items: []parse.SelectItem{{Expr: e}}},
			From:   parse.FromClause{Root: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"}},
		}
	}
	for name, e := range map[string]parse.SelectExpr{
		"+Inf literal":          parse.LiteralExpr{Value: aql.Real(math.Inf(1))},
		"-Inf literal":          parse.LiteralExpr{Value: aql.Real(math.Inf(-1))},
		"NaN literal":           parse.LiteralExpr{Value: aql.Real(math.NaN())},
		"TERMINOLOGY arity":     parse.LiteralExpr{Value: aql.Func(aql.TerminologyFunc, aql.String("a"))},
		"reserved func name":    parse.LiteralExpr{Value: aql.Func("SELECT", aql.Path("c/x"))},
		"malformed func name":   parse.LiteralExpr{Value: aql.Func("1FUNC", aql.Path("c/x"))},
		"nil func argument":     parse.LiteralExpr{Value: aql.Func("LENGTH", nil)},
		"pointer-shaped value":  parse.LiteralExpr{Value: &aql.RealValue{F: math.Inf(1)}},
		"projected SELECT name": parse.FunctionCall{Name: "SELECT", Args: []parse.SelectExpr{}},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := mk(e).Emit()
			if err == nil {
				t.Fatalf("Emit produced %q with err == nil; ParseQuery cannot read that back", out)
			}
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("err = %v, want ErrInvalidQuery", err)
			}
		})
	}

	// Positive controls: a projected AGGREGATE is legal in SELECT and nowhere
	// else, so the SELECT-side name check must admit it, and an ordinary literal
	// must still emit and re-parse.
	for name, e := range map[string]parse.SelectExpr{
		"aggregate COUNT": parse.FunctionCall{Name: "COUNT", Star: true},
		"aggregate AVG": parse.FunctionCall{Name: "AVG", Args: []parse.SelectExpr{
			parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "c/x"}}},
		}},
		"string literal": parse.LiteralExpr{Value: aql.String("O'Brien")},
		"real literal":   parse.LiteralExpr{Value: aql.Real(2)},
		"func literal":   parse.LiteralExpr{Value: aql.Func("LENGTH", aql.Path("c/x"))},
	} {
		t.Run("emits/"+name, func(t *testing.T) {
			out, err := mk(e).Emit()
			if err != nil {
				t.Fatalf("Emit refused a legal SELECT item: %v", err)
			}
			if _, err := parse.ParseQuery(out); err != nil {
				t.Errorf("emitted %q does not re-parse: %v", out, err)
			}
		})
	}
}
