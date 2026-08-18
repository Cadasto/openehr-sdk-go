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
	"strings"
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
		// One bad byte per NON-PATH component. Every refusal above puts its bad
		// byte in the path or the scheme, so the query / fragment / userinfo /
		// host / port checks were each pinned zero ways: deleting any one of
		// them left the whole suite green while the builder emitted AQL this
		// SDK's own parser rejects.
		"http://example.org/p?x=<y>",
		"http://example.org/p#f<g",
		"http://us<er@example.org/p",
		"http://ex ample.org/p",
		"http://example.org:80a0/p",
		// Spellable as a URI, but TERM_CODE is declared first and wins the tie,
		// and matchesOperand admits URI only. Only these two REACH that check —
		// the display-name and paren forms below are refused earlier, by the
		// path and scheme alphabets, so they do not exercise it.
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
		// Query AND fragment, in the RFC-3986 order. Its absence meant the two
		// `strings.Cut` calls could be swapped without failing CI — and under
		// that swap this ordinary URL is REFUSED. An anti-tightening control,
		// like TestEmitAcceptsAnAliaslessContainment.
		"http://example.org/p?a=1#f",
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
// refuses a class node without an RM type; Emit had no counterpart, so the
// read/write parity claim held only for the junction-placement rule.
//
// It matters twice over, because the read side infers the node KIND from an
// absent RMType: an incomplete class node otherwise looks like a junction, so it
// either emitted a dangling `CONTAINS ` or got diagnosed as a misplaced junction
// the caller never wrote. Completeness is therefore checked first.
//
// An ALIAS is deliberately not required — see
// TestEmitAcceptsAnAliaslessContainment, which locks that in so the check is
// not "tightened" into refusing valid AQL.
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
		// `terminologyFunction` fixes BOTH the arity and the argument type, and a
		// projected call is modelled by parse.FunctionCall rather than
		// aql.FuncCall, so aql.ValidateValue's arity check does not reach it.
		"projected TERMINOLOGY 1 arg":  parse.FunctionCall{Name: "TERMINOLOGY", Args: selectLits("a")},
		"projected TERMINOLOGY 2 args": parse.FunctionCall{Name: "TERMINOLOGY", Args: selectLits("a", "b")},
		"projected TERMINOLOGY 4 args": parse.FunctionCall{Name: "TERMINOLOGY", Args: selectLits("a", "b", "c", "d")},
		"projected TERMINOLOGY 0 args": parse.FunctionCall{Name: "TERMINOLOGY"},
		"projected TERMINOLOGY lower":  parse.FunctionCall{Name: "terminology", Args: selectLits("a")},
		"projected TERMINOLOGY path arg": parse.FunctionCall{Name: "TERMINOLOGY", Args: []parse.SelectExpr{
			parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "c/x"}}},
			parse.LiteralExpr{Value: aql.String("b")},
			parse.LiteralExpr{Value: aql.String("c")},
		}},
		"projected TERMINOLOGY int arg": parse.FunctionCall{Name: "TERMINOLOGY", Args: []parse.SelectExpr{
			parse.LiteralExpr{Value: aql.Int(1)},
			parse.LiteralExpr{Value: aql.String("b")},
			parse.LiteralExpr{Value: aql.String("c")},
		}},
		"projected TERMINOLOGY star":     parse.FunctionCall{Name: "TERMINOLOGY", Star: true},
		"projected TERMINOLOGY distinct": parse.FunctionCall{Name: "TERMINOLOGY", Distinct: true, Args: selectLits("a", "b", "c")},
		// ı upper-cases to ASCII `I`, so an alphabet walk over the ToUpper'd
		// name accepted a spelling the lexer cannot tokenise — and SELECT
		// emits the name AS WRITTEN (`SELECT ı('a')`, token recognition
		// error, err == nil).
		"projected name that only case-folds to ASCII": parse.FunctionCall{Name: "ı", Args: selectLits("a")},
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
	// else, so the SELECT-side name check must admit it; a well-formed
	// TERMINOLOGY must survive the new arity check; and an ordinary literal must
	// still emit and re-parse.
	for name, e := range map[string]parse.SelectExpr{
		"aggregate COUNT": parse.FunctionCall{Name: "COUNT", Star: true},
		"aggregate AVG": parse.FunctionCall{Name: "AVG", Args: []parse.SelectExpr{
			parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "c/x"}}},
		}},
		"aggregate DISTINCT": parse.FunctionCall{Name: "COUNT", Distinct: true, Args: []parse.SelectExpr{
			parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "c/x"}}},
		}},
		"TERMINOLOGY 3 strings": parse.FunctionCall{Name: "TERMINOLOGY", Args: selectLits("expand", "//fhir", "url=x")},
		// The POINTER twin of the same three arguments. A `*aql.StringValue` is
		// a string literal too, and the value-position guard already accepts
		// one, so refusing it here bound the same rule to one carrier only
		// (REQ-119, "every shape, including its pointer twin").
		"TERMINOLOGY 3 pointer strings": parse.FunctionCall{Name: "TERMINOLOGY", Args: []parse.SelectExpr{
			parse.LiteralExpr{Value: &aql.StringValue{S: "expand"}},
			parse.LiteralExpr{Value: &aql.StringValue{S: "//fhir"}},
			parse.LiteralExpr{Value: &aql.StringValue{S: "url=x"}},
		}},
		"pointer string literal": parse.LiteralExpr{Value: &aql.StringValue{S: "O'Brien"}},
		"pointer func literal":   parse.LiteralExpr{Value: &aql.FuncCall{Name: "LENGTH", Args: []aql.Value{aql.Path("c/x")}}},
		"string literal":         parse.LiteralExpr{Value: aql.String("O'Brien")},
		"real literal":           parse.LiteralExpr{Value: aql.Real(2)},
		"func literal":           parse.LiteralExpr{Value: aql.Func("LENGTH", aql.Path("c/x"))},
	} {
		t.Run("emits/"+name, func(t *testing.T) {
			out, err := mk(e).Emit()
			if err != nil {
				t.Fatalf("Emit refused a legal SELECT item: %v", err)
			}
			doc, err := parse.ParseQuery(out)
			if err != nil {
				t.Fatalf("emitted %q does not re-parse: %v", out, err)
			}
			// Text fixed point, for every shape: re-emitting what we just read
			// must reproduce the same AQL. This is the SELECT-side form of the
			// closure property and holds regardless of which carrier the read
			// side chose.
			again, err := doc.Emit()
			if err != nil {
				t.Fatalf("re-emit of %q failed: %v", out, err)
			}
			if again != out {
				t.Errorf("SELECT item is not a fixed point\n  first:  %q\n  second: %q", out, again)
			}
			// For a literal, closure additionally means the recovered VALUE is
			// equal — re-parse alone left the SELECT half weaker than the WHERE
			// half. The exception is a FuncCall: the SELECT side models a
			// projected call as parse.FunctionCall (see aql.FuncCall's godoc), so
			// carrying one in a LiteralExpr normalises to that carrier on re-read.
			// The text is identical either way, which the fixed point above pins.
			lit, ok := e.(parse.LiteralExpr)
			if !ok {
				return
			}
			// Normalised, because a `*aql.FuncCall` is a projected call too —
			// this carve-out had the very shape-vs-pointer-twin bug the guards
			// under test exist to prevent, and the pointer case below caught it.
			inner, _ := aql.DerefValue(lit.Value)
			if _, isCall := inner.(aql.FuncCall); isCall {
				if _, ok := doc.Select.Items[0].Expr.(parse.FunctionCall); !ok {
					t.Errorf("a FuncCall carried in a LiteralExpr came back %T, want parse.FunctionCall",
						doc.Select.Items[0].Expr)
				}
				return
			}
			back, ok := doc.Select.Items[0].Expr.(parse.LiteralExpr)
			if !ok {
				t.Fatalf("SELECT item came back %T, want parse.LiteralExpr (wire %q)",
					doc.Select.Items[0].Expr, out)
			}
			if !aql.EqualValues(lit.Value, back.Value) {
				t.Errorf("SELECT literal did not round-trip to an equal value\n  in:   %#v\n  wire: %q\n  out:  %#v",
					lit.Value, out, back.Value)
			}
		})
	}
}

// TestEmitRefusesUngrammaticalAggregateShapes — [aql.ValidateSelectFuncName]
// admits the aggregate NAMES; `aggregateFunctionCall` also fixes their SHAPE —
// `COUNT '(' (DISTINCT? identifiedPath | '*') ')' | (MIN|MAX|SUM|AVG) '('
// identifiedPath ')'` — and the general `functionCall` admits neither DISTINCT
// nor `*` at all. Unvalidated, the emitter's body switch PICKED A WINNER:
// `COUNT{Star, Args}` emitted `COUNT(*)` with the argument silently gone
// (valid AQL counting rows instead of path values — the substitution class),
// and the rest emitted text this SDK's own parser rejects.
func TestEmitRefusesUngrammaticalAggregateShapes(t *testing.T) {
	mk := func(e parse.SelectExpr) *parse.Query {
		return &parse.Query{
			Select: parse.SelectClause{Items: []parse.SelectItem{{Expr: e}}},
			From:   parse.FromClause{Root: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"}},
		}
	}
	path := func(raw string) parse.SelectExpr {
		return parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: raw}}}
	}
	for name, e := range map[string]parse.SelectExpr{
		// The silent pair: a star beside arguments or DISTINCT — emitting the
		// star alone dropped them with err == nil.
		"COUNT star beside argument": parse.FunctionCall{Name: "COUNT", Star: true, Args: []parse.SelectExpr{path("c/uid/value")}},
		"COUNT star beside DISTINCT": parse.FunctionCall{Name: "COUNT", Star: true, Distinct: true},
		// The loud set: emitted before, refused by the parser after.
		"MIN star":               parse.FunctionCall{Name: "MIN", Star: true},
		"MIN DISTINCT":           parse.FunctionCall{Name: "MIN", Distinct: true, Args: []parse.SelectExpr{path("c/x")}},
		"COUNT no args":          parse.FunctionCall{Name: "COUNT"},
		"COUNT two args":         parse.FunctionCall{Name: "COUNT", Args: []parse.SelectExpr{path("c/a"), path("c/b")}},
		"MAX literal arg":        parse.FunctionCall{Name: "MAX", Args: []parse.SelectExpr{parse.LiteralExpr{Value: aql.Int(1)}}},
		"SUM two args":           parse.FunctionCall{Name: "SUM", Args: []parse.SelectExpr{path("c/a"), path("c/b")}},
		"non-aggregate DISTINCT": parse.FunctionCall{Name: "LENGTH", Distinct: true, Args: []parse.SelectExpr{path("c/x")}},
		"non-aggregate star":     parse.FunctionCall{Name: "LENGTH", Star: true},
	} {
		t.Run("refused/"+name, func(t *testing.T) {
			out, err := mk(e).Emit()
			if err == nil {
				t.Fatalf("Emit produced %q with err == nil", out)
			}
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("err = %v, want ErrInvalidQuery", err)
			}
		})
	}
	// Every legal aggregate shape must still emit, re-parse, and reach a text
	// fixed point — the guard narrows to the grammar, not past it.
	for name, e := range map[string]parse.SelectExpr{
		"COUNT star":     parse.FunctionCall{Name: "COUNT", Star: true},
		"COUNT path":     parse.FunctionCall{Name: "COUNT", Args: []parse.SelectExpr{path("c/uid/value")}},
		"COUNT DISTINCT": parse.FunctionCall{Name: "COUNT", Distinct: true, Args: []parse.SelectExpr{path("c/uid/value")}},
		"MIN path":       parse.FunctionCall{Name: "MIN", Args: []parse.SelectExpr{path("c/x")}},
		"pointer arg":    parse.FunctionCall{Name: "AVG", Args: []parse.SelectExpr{&parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "c/x"}}}}},
	} {
		t.Run("emits/"+name, func(t *testing.T) {
			out, err := mk(e).Emit()
			if err != nil {
				t.Fatalf("Emit refused a legal aggregate: %v", err)
			}
			doc, err := parse.ParseQuery(out)
			if err != nil {
				t.Fatalf("emitted %q does not re-parse: %v", out, err)
			}
			again, err := doc.Emit()
			if err != nil || again != out {
				t.Errorf("not a text fixed point: %q -> %q (err %v)", out, again, err)
			}
		})
	}
}

// TestLowercaseProjectedFunctionNameEmitsAndReparses — the alphabet fix for
// case-folding names (see TestFuncNameAlphabetRunsOnTheOriginalSpelling) must
// not smuggle in case SENSITIVITY: a lower-case ASCII name lexes as written.
// It is deliberately not in the fixed-point loop above — Emit writes the name
// as written while the read side canonicalises to upper case, so `upper`
// re-emits as `UPPER`; this pins that read-side canonicalisation instead.
func TestLowercaseProjectedFunctionNameEmitsAndReparses(t *testing.T) {
	q := &parse.Query{
		Select: parse.SelectClause{Items: []parse.SelectItem{{
			Expr: parse.FunctionCall{Name: "upper", Args: selectLits("a")},
		}}},
		From: parse.FromClause{Root: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"}},
	}
	out, err := q.Emit()
	if err != nil {
		t.Fatalf("Emit refused a lower-case function name: %v", err)
	}
	doc, err := parse.ParseQuery(out)
	if err != nil {
		t.Fatalf("emitted %q does not re-parse: %v", out, err)
	}
	fc, ok := doc.Select.Items[0].Expr.(parse.FunctionCall)
	if !ok || fc.Name != "UPPER" {
		t.Fatalf("re-parsed projection = %#v, want FunctionCall named UPPER (read-side canonical form)",
			doc.Select.Items[0].Expr)
	}
}

// TestEmitCountsAVersionRootAsPresent — REQ-119.
//
// FROM-root presence keyed on `RMType != ""` alone, but [ParseQuery] is the
// only writer that pairs RMType "VERSION" with the Version flag: a hand-built
// `ClassExpr{Version: true}` is the same grammar class (`classExprOperand :
// … | VERSION …`) with the flag as its only marker. Keying on RMType both
// refused it standing alone ("missing FROM root") and — the substitution
// class — silently DROPPED it beside a junction, emitting only the junction
// with err == nil. Presence is now `RMType != "" || Version`, exactly as the
// containment walk has always counted a class node.
func TestEmitCountsAVersionRootAsPresent(t *testing.T) {
	sel := parse.SelectClause{Items: []parse.SelectItem{{Expr: parse.PathExpr{
		IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "v/x"}},
	}}}}

	t.Run("version root beside a junction is refused, not dropped", func(t *testing.T) {
		q := &parse.Query{
			Select: sel,
			From: parse.FromClause{
				Root: parse.ClassExpr{Version: true, Alias: "v"},
				Junction: &parse.Containment{
					ChildJoin: parse.ContainsOr,
					Children: []parse.Containment{
						{Class: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c1"}},
						{Class: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c2"}},
					},
				},
			},
		}
		out, err := q.Emit()
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery (emitted %q — the VERSION root would be silently dropped)",
				err, out)
		}
	})

	t.Run("version root alone emits and round-trips", func(t *testing.T) {
		q := &parse.Query{Select: sel, From: parse.FromClause{Root: parse.ClassExpr{Version: true, Alias: "v"}}}
		out, err := q.Emit()
		if err != nil {
			t.Fatalf("Emit refused a legal VERSION root: %v", err)
		}
		doc, err := parse.ParseQuery(out)
		if err != nil {
			t.Fatalf("emitted %q does not re-parse: %v", out, err)
		}
		if !doc.From.Root.Version || doc.From.Root.RMType != "VERSION" {
			t.Fatalf("re-parsed root = %+v, want the VERSION class", doc.From.Root)
		}
		again, err := doc.Emit()
		if err != nil || again != out {
			t.Errorf("not a text fixed point: %q -> %q (err %v)", out, again, err)
		}
	})
}

// TestEmitRefusesAnOutOfVocabularyContainsJoin — [parse.ContainsJoin.String]
// spells any value outside {ContainsAnd, ContainsOr} as AND, and AND vs OR
// changes the RESULT SET: a hand-built junction carrying ContainsJoin(7)
// emitted a valid query joining with AND, err == nil — the same silent
// re-spelling refused for OrderDir, SELECT TOP and the WHERE BoolOp. This was
// the last emission vocabulary with a total spelling on a validating path
// (aql's write-side containsJoin is private and constructor-fed, so Build
// cannot carry an out-of-vocabulary join).
func TestEmitRefusesAnOutOfVocabularyContainsJoin(t *testing.T) {
	sel := parse.SelectClause{Items: []parse.SelectItem{{Expr: parse.PathExpr{
		IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "c/x"}},
	}}}}
	junction := func(j parse.ContainsJoin) parse.Containment {
		return parse.Containment{
			ChildJoin: j,
			Children: []parse.Containment{
				{Class: parse.ClassExpr{RMType: "OBSERVATION", Alias: "o"}},
				{Class: parse.ClassExpr{RMType: "EVALUATION", Alias: "ev"}},
			},
		}
	}

	// Both junction positions: the FROM-root junction and one nested below a
	// class — the walk covers them through different entry points.
	for name, q := range map[string]*parse.Query{
		"root junction": {
			Select: sel,
			From:   parse.FromClause{Junction: func() *parse.Containment { j := junction(parse.ContainsJoin(7)); return &j }()},
		},
		"nested junction": {
			Select: sel,
			From: parse.FromClause{
				Root:     parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"},
				Contains: func() *parse.Containment { j := junction(parse.ContainsJoin(7)); return &j }(),
			},
		},
	} {
		t.Run("refused/"+name, func(t *testing.T) {
			out, err := q.Emit()
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Fatalf("err = %v, want ErrInvalidQuery (emitted %q)", err, out)
			}
		})
	}

	// Positive controls: both vocabulary members emit and hold the text fixed
	// point, so a tightened guard fails here rather than shipping.
	for name, j := range map[string]parse.ContainsJoin{"AND": parse.ContainsAnd, "OR": parse.ContainsOr} {
		t.Run("accepted/"+name, func(t *testing.T) {
			q := &parse.Query{
				Select: sel,
				From: parse.FromClause{
					Root:     parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"},
					Contains: func() *parse.Containment { jn := junction(j); return &jn }(),
				},
			}
			out, err := q.Emit()
			if err != nil {
				t.Fatalf("Emit refused a legal %s junction: %v", name, err)
			}
			doc, err := parse.ParseQuery(out)
			if err != nil {
				t.Fatalf("emitted %q does not re-parse: %v", out, err)
			}
			again, err := doc.Emit()
			if err != nil || again != out {
				t.Errorf("not a text fixed point: %q -> %q (err %v)", out, again, err)
			}
		})
	}
}

// TestEmitRefusesDualClassOperands — [parse.ClassExpr.Archetype] and
// [parse.ClassExpr.Predicate] are the two mutually exclusive spellings of the
// ONE bracket position, and the emitter renders whichever its switch reaches
// first: setting both silently dropped the standing predicate — a row FILTER —
// so the emitted query returned MORE rows than the AST asked for, err == nil.
// The same dual-operand rule already guards [aql.Comparison] (Path vs Left)
// and [aql.MatchesExpr] (three operand forms); ClassExpr was the third type
// with two spellings of one position and no twin of the rule.
func TestEmitRefusesDualClassOperands(t *testing.T) {
	dual := parse.ClassExpr{
		RMType: "COMPOSITION", Alias: "c", HasPredicate: true,
		Archetype: "openEHR-EHR-COMPOSITION.encounter.v1", Predicate: "uid/value=$u",
	}
	sel := parse.SelectClause{Items: []parse.SelectItem{{Expr: parse.PathExpr{
		IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "c/uid/value"}},
	}}}}

	for name, from := range map[string]parse.FromClause{
		"FROM root": {Root: dual},
		"nested CONTAINS": {
			Root:     parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"},
			Contains: &parse.Containment{Class: parse.ClassExpr{RMType: "OBSERVATION", Alias: "o", HasPredicate: true, Archetype: "openEHR-EHR-OBSERVATION.bp.v1", Predicate: "name/value=$n"}},
		},
		"junction operand": {
			Junction: &parse.Containment{ChildJoin: parse.ContainsOr, Children: []parse.Containment{
				{Class: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c1"}},
				{Class: dual},
			}},
		},
	} {
		t.Run("refused/"+name, func(t *testing.T) {
			q := &parse.Query{Select: sel, From: from}
			out, err := q.Emit()
			if err == nil {
				t.Fatalf("Emit produced %q with err == nil; the standing predicate was silently dropped", out)
			}
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("err = %v, want ErrInvalidQuery", err)
			}
		})
	}
	// One spelling at a time stays emittable and round-trips.
	for name, root := range map[string]parse.ClassExpr{
		"archetype only": {RMType: "COMPOSITION", Alias: "c", HasPredicate: true, Archetype: "openEHR-EHR-COMPOSITION.encounter.v1"},
		"predicate only": {RMType: "COMPOSITION", Alias: "c", HasPredicate: true, Predicate: "uid/value=$u"},
	} {
		t.Run("emits/"+name, func(t *testing.T) {
			q := &parse.Query{Select: sel, From: parse.FromClause{Root: root}}
			out, err := q.Emit()
			if err != nil {
				t.Fatalf("Emit refused a single-operand class: %v", err)
			}
			if _, err := parse.ParseQuery(out); err != nil {
				t.Errorf("emitted %q does not re-parse: %v", out, err)
			}
		})
	}
}

// TestEmitRefusesBareParameterProjection — `columnExpr : identifiedPath |
// primitive | aggregateFunctionCall | functionCall` has NO PARAMETER
// alternative, while a function ARGUMENT is a `terminal`, which has. The
// refusal is therefore POSITIONAL: `SELECT $p` emitted text the parser
// rejects, while `SELECT CONCAT('a', $p)` is legal and must stay so. Also
// pins the empty-path refusal beside it — the one path position in the
// subsystem that had no emptiness check (`SELECT  FROM …`, err == nil).
func TestEmitRefusesBareParameterProjection(t *testing.T) {
	mk := func(e parse.SelectExpr) *parse.Query {
		return &parse.Query{
			Select: parse.SelectClause{Items: []parse.SelectItem{{Expr: e}}},
			From:   parse.FromClause{Root: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"}},
		}
	}
	for name, e := range map[string]parse.SelectExpr{
		"bare parameter":        parse.LiteralExpr{Value: aql.Param("p")},
		"pointer-carried param": parse.LiteralExpr{Value: &aql.ParamValue{Name: "p"}},
		"pointer literal param": &parse.LiteralExpr{Value: aql.Param("p")},
		"empty path projection": parse.PathExpr{},
		"blank path projection": parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "   "}}},
	} {
		t.Run("refused/"+name, func(t *testing.T) {
			out, err := mk(e).Emit()
			if err == nil {
				t.Fatalf("Emit produced %q with err == nil; ParseQuery cannot read that back", out)
			}
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("err = %v, want ErrInvalidQuery", err)
			}
		})
	}
	t.Run("emits/parameter as function argument", func(t *testing.T) {
		q := mk(parse.FunctionCall{Name: "CONCAT", Args: []parse.SelectExpr{
			parse.LiteralExpr{Value: aql.String("a")},
			parse.LiteralExpr{Value: aql.Param("p")},
		}})
		out, err := q.Emit()
		if err != nil {
			t.Fatalf("Emit refused the legal argument position: %v", err)
		}
		if _, err := parse.ParseQuery(out); err != nil {
			t.Errorf("emitted %q does not re-parse: %v", out, err)
		}
	})
}

// selectLits wraps string literals as projected SELECT items.
func selectLits(ss ...string) []parse.SelectExpr {
	out := make([]parse.SelectExpr, 0, len(ss))
	for _, s := range ss {
		out = append(out, parse.LiteralExpr{Value: aql.String(s)})
	}
	return out
}

// TestFormatValueDoesNotPanicOnAValueWithNoWireForm — [aql.FormatValue] is the
// deliberately unvalidated formatter, which makes a typed-nil pointer shape
// reachable through it: the validating paths refuse one, but by contract this one
// does not validate. An escape hatch that panics is worse than one that renders
// nothing, so it returns "".
//
// [aql.MatchesExpr.Terminology] is a `*aql.FuncCall`, so its zero value is
// exactly that pointer — this is reachable without anyone writing `(*T)(nil)`.
func TestFormatValueDoesNotPanicOnAValueWithNoWireForm(t *testing.T) {
	var m aql.MatchesExpr
	for name, v := range map[string]aql.Value{
		"untyped nil":            nil,
		"zero MatchesExpr field": m.Terminology,
		"typed-nil *FuncCall":    (*aql.FuncCall)(nil),
		"typed-nil *PathValue":   (*aql.PathValue)(nil),
		"typed-nil *RealValue":   (*aql.RealValue)(nil),
		"typed-nil *StringValue": (*aql.StringValue)(nil),
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("FormatValue panicked: %v", r)
				}
			}()
			if got := aql.FormatValue(v); got != "" {
				t.Errorf("FormatValue = %q, want \"\"", got)
			}
		})
	}
	// A pointer to a POPULATED shape must still render, and identically to the
	// value shape it addresses.
	if got, want := aql.FormatValue(&aql.StringValue{S: "O'Brien"}), aql.FormatValue(aql.String("O'Brien")); got != want {
		t.Errorf("pointer shape rendered %q, value shape %q", got, want)
	}
}

// TestFormatWhereDoesNotPanicOnItsOwnRefusalPath — the last reachable
// typed-nil panic, and the worst-placed one: on the way to REPORTING a refusal.
//
// [aql.Comparison.validate] checks the operator FIRST and names the left
// operand in that error, so it renders the operand before the operand has been
// validated. With an unknown operator and a typed-nil Left, building the error
// message panicked — a public entry point crashing while trying to tell the
// caller their query was invalid. The known-operator paths were already clean,
// because they reach `ValidateValue`, which refuses a typed-nil first; only the
// error-formatting path was exposed.
func TestFormatWhereDoesNotPanicOnItsOwnRefusalPath(t *testing.T) {
	for name, c := range map[string]aql.Comparison{
		"unknown op + typed-nil *FuncCall": {
			Left: (*aql.FuncCall)(nil), Op: aql.Operator("???"), Val: aql.String("x"),
		},
		"unknown op + typed-nil *PathValue": {
			Left: (*aql.PathValue)(nil), Op: aql.Operator("???"), Val: aql.String("x"),
		},
		"unknown op + typed-nil Left + nil Val": {
			Left: (*aql.FuncCall)(nil), Op: aql.Operator("???"),
		},
		"known op + typed-nil Left": {
			Left: (*aql.FuncCall)(nil), Op: aql.OpEq, Val: aql.String("x"),
		},
		"unknown op + plain path": {
			Path: "o/x", Op: aql.Operator("???"), Val: aql.String("x"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("FormatWhere panicked while refusing: %v", r)
				}
			}()
			out, err := aql.FormatWhere(c)
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Fatalf("err = %v, want ErrInvalidQuery", err)
			}
			if out != "" {
				t.Errorf("a refused comparison still emitted %q", out)
			}
		})
	}
}

// TestEmitAcceptsAnAliaslessContainment locks in what Emit must NOT refuse.
//
// The completeness walk checks the RM type and deliberately says nothing about
// the alias: `classExprOperand` takes the alias as optional, so an alias-less
// CONTAINS is valid AQL that ParseQuery accepts. [aql.Builder] does require one,
// which makes "Build refuses it, so Emit should too" a tempting and WRONG
// inference — it would have Emit reject a query the parser had just read.
//
// Every other Emit fixture carries aliases, so without this case a tightened
// check would pass CI.
func TestEmitAcceptsAnAliaslessContainment(t *testing.T) {
	for _, q := range []string{
		"SELECT c/x FROM COMPOSITION c CONTAINS OBSERVATION",
		"SELECT c/x FROM COMPOSITION c CONTAINS OBSERVATION CONTAINS CLUSTER",
		"SELECT c/x FROM COMPOSITION CONTAINS OBSERVATION o",
		"SELECT * FROM EHR",
	} {
		t.Run(q, func(t *testing.T) {
			doc, err := parse.ParseQuery(q)
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}
			out, err := doc.Emit()
			if err != nil {
				t.Fatalf("Emit refused AQL that ParseQuery accepted: %v", err)
			}
			if out != q {
				t.Errorf("Emit = %q, want the canonical input %q", out, q)
			}
		})
	}
}

// TestFormatWhereDoesNotPanicOnATypedNilPredicate — the WhereExpr sibling of
// the Value-side pointer-twin rule.
//
// expr and validate have value receivers, so `*Comparison` satisfies WhereExpr
// and calling either on a nil one panics. Absence means the same thing in both
// vocabularies but has different consequences by POSITION: at the top level a
// missing predicate is simply no WHERE clause, while inside a NOT or a junction
// an operand that vanished would silently change what the query asks, so it is
// refused there.
func TestFormatWhereDoesNotPanicOnATypedNilPredicate(t *testing.T) {
	nilCmp := (*aql.Comparison)(nil)

	t.Run("top level denotes no clause", func(t *testing.T) {
		for name, w := range map[string]aql.WhereExpr{
			"untyped nil":            nil,
			"typed-nil *Comparison":  nilCmp,
			"typed-nil *MatchesExpr": (*aql.MatchesExpr)(nil),
			"typed-nil *Junction":    (*aql.Junction)(nil),
			"typed-nil *NotExpr":     (*aql.NotExpr)(nil),
		} {
			t.Run(name, func(t *testing.T) {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("FormatWhere panicked: %v", r)
					}
				}()
				out, err := aql.FormatWhere(w)
				if err != nil || out != "" {
					t.Errorf("out=%q err=%v, want \"\" and no error", out, err)
				}
			})
		}
	})

	t.Run("inside a composite it is refused", func(t *testing.T) {
		for name, w := range map[string]aql.WhereExpr{
			"NOT operand":   aql.NotExpr{Operand: nilCmp},
			"AND term":      aql.Junction{Op: aql.OpAnd, Terms: []aql.WhereExpr{aql.Eq("c/x", aql.Int(1)), nilCmp}},
			"OR term first": aql.Junction{Op: aql.OpOr, Terms: []aql.WhereExpr{nilCmp, aql.Eq("c/x", aql.Int(1))}},
			"nested":        aql.NotExpr{Operand: aql.Junction{Op: aql.OpAnd, Terms: []aql.WhereExpr{nilCmp}}},
		} {
			t.Run(name, func(t *testing.T) {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("FormatWhere panicked: %v", r)
					}
				}()
				out, err := aql.FormatWhere(w)
				if !errors.Is(err, aql.ErrInvalidQuery) {
					t.Fatalf("err = %v, want ErrInvalidQuery", err)
				}
				if out != "" {
					t.Errorf("a refused predicate still emitted %q", out)
				}
			})
		}
	})

	// A pointer to a POPULATED predicate must still render, identically to the
	// value shape — the guard normalises, it does not reject pointers.
	t.Run("populated pointer renders", func(t *testing.T) {
		c := aql.Comparison{Path: "c/x", Op: aql.OpEq, Val: aql.Int(1)}
		viaPtr, err := aql.FormatWhere(&c)
		if err != nil {
			t.Fatalf("pointer predicate refused: %v", err)
		}
		viaVal, err := aql.FormatWhere(c)
		if err != nil {
			t.Fatalf("value predicate refused: %v", err)
		}
		if viaPtr != viaVal {
			t.Errorf("pointer rendered %q, value %q", viaPtr, viaVal)
		}
	})
}

// TestJunctionParenthesisationBindsThePointerTwin — REQ-119.
//
// [aql.Junction.expr] and [aql.NotExpr.expr] decide whether to emit precedence
// parentheses from the operand's CONCRETE SHAPE. Both asserted the value shape
// only, so a `*aql.Junction` fell through and a nested OR emitted unparenthesised.
//
// This is the worst version of the pointer-twin defect, and the reason it needs
// its own test rather than a line in the typed-nil table: the output is VALID
// AQL that re-parses and re-emits to itself, so it is a fixed point of the round
// trip — every existing check passes while the query means something else. The
// assertion is therefore byte-identity between the two carriers, not
// parseability.
//
// The typed-nil test's "populated pointer renders" case could not see this: it
// uses a top-level `*Comparison`, a LEAF, which is the one shape where no
// dispatch on the operand happens.
func TestJunctionParenthesisationBindsThePointerTwin(t *testing.T) {
	a := aql.Eq("c/a", aql.Int(1))
	or := aql.Junction{Op: aql.OpOr, Terms: []aql.WhereExpr{
		aql.Eq("c/b", aql.Int(2)), aql.Eq("c/c", aql.Int(3)),
	}}
	and := aql.Junction{Op: aql.OpAnd, Terms: []aql.WhereExpr{
		aql.Eq("c/b", aql.Int(2)), aql.Eq("c/c", aql.Int(3)),
	}}

	for name, tc := range map[string]struct{ value, pointer aql.WhereExpr }{
		"OR nested in an AND": {
			value:   aql.Junction{Op: aql.OpAnd, Terms: []aql.WhereExpr{a, or}},
			pointer: aql.Junction{Op: aql.OpAnd, Terms: []aql.WhereExpr{a, &or}},
		},
		"OR under a NOT": {
			value:   aql.NotExpr{Operand: or},
			pointer: aql.NotExpr{Operand: &or},
		},
		"AND under a NOT": {
			value:   aql.NotExpr{Operand: and},
			pointer: aql.NotExpr{Operand: &and},
		},
		"through the And constructor": {
			value:   aql.And(a, or),
			pointer: aql.And(a, &or),
		},
		"through the Not constructor": {
			value:   aql.Not(or),
			pointer: aql.Not(&or),
		},
		"nested two deep": {
			value:   aql.Junction{Op: aql.OpAnd, Terms: []aql.WhereExpr{a, aql.NotExpr{Operand: or}}},
			pointer: aql.Junction{Op: aql.OpAnd, Terms: []aql.WhereExpr{a, &aql.NotExpr{Operand: &or}}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			viaValue, err := aql.FormatWhere(tc.value)
			if err != nil {
				t.Fatalf("value shape refused: %v", err)
			}
			viaPointer, err := aql.FormatWhere(tc.pointer)
			if err != nil {
				t.Fatalf("pointer shape refused: %v", err)
			}
			if viaPointer != viaValue {
				t.Errorf("pointer emitted %q, value emitted %q — the same tree must render the same text",
					viaPointer, viaValue)
			}
			// Belt and braces: the text must also mean what it says. A dropped
			// parenthesis re-parses cleanly, so only the tree shape shows it.
			q := "SELECT c/x FROM COMPOSITION c WHERE " + viaPointer
			doc, err := parse.ParseQuery(q)
			if err != nil {
				t.Fatalf("ParseQuery(%q): %v", q, err)
			}
			back, err := doc.Emit()
			if err != nil {
				t.Fatalf("Emit: %v", err)
			}
			if back != q {
				t.Errorf("not a fixed point: %q -> %q", q, back)
			}
		})
	}

	// The same tree through aql.Builder.Build, which REQ-119 names as a
	// validating write path alongside FormatWhere.
	t.Run("through Builder.Build", func(t *testing.T) {
		build := func(w aql.WhereExpr) string {
			q, err := aql.NewBuilder().Select(aql.Col("c/x")).From("COMPOSITION", "c").Where(w).Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			return q.Q
		}
		if got, want := build(aql.And(a, &or)), build(aql.And(a, or)); got != want {
			t.Errorf("Build with a pointer term = %q, want %q", got, want)
		}
	})
}

// TestBuildDoesNotPanicOnATypedNilPredicate — REQ-119, "absence is positional".
//
// [aql.FormatWhere] was made total over a typed-nil predicate; [aql.Builder.Build]
// is the OTHER validating write path the closure clause names, and it still
// called validate through the nil pointer. `where != nil` is true for a typed
// nil, and validate has a value receiver.
//
// The FromEHR case is the interesting one: effectiveWhere AND-combines the
// implicit ehr_id filter with the caller's predicate, so an un-normalised typed
// nil became a junction TERM — where absence is correctly refused — giving one
// input three different behaviours depending on the route.
func TestBuildDoesNotPanicOnATypedNilPredicate(t *testing.T) {
	nilCmp := (*aql.Comparison)(nil)

	for name, tc := range map[string]struct {
		where aql.WhereExpr
		want  string
	}{
		"untyped nil":            {nil, "SELECT c/x FROM COMPOSITION c"},
		"typed-nil *Comparison":  {nilCmp, "SELECT c/x FROM COMPOSITION c"},
		"typed-nil *Junction":    {(*aql.Junction)(nil), "SELECT c/x FROM COMPOSITION c"},
		"typed-nil *MatchesExpr": {(*aql.MatchesExpr)(nil), "SELECT c/x FROM COMPOSITION c"},
		"typed-nil *NotExpr":     {(*aql.NotExpr)(nil), "SELECT c/x FROM COMPOSITION c"},
		// And() keeps a TYPED nil (it drops only an untyped one), so a
		// single typed-nil term becomes the whole predicate.
		"And of one typed nil": {aql.And(nilCmp), "SELECT c/x FROM COMPOSITION c"},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Build panicked: %v", r)
				}
			}()
			q, err := aql.NewBuilder().Select(aql.Col("c/x")).From("COMPOSITION", "c").Where(tc.where).Build()
			if err != nil {
				t.Fatalf("Build refused a top-level absence: %v", err)
			}
			if q.Q != tc.want {
				t.Errorf("Build = %q, want %q", q.Q, tc.want)
			}
		})
	}

	t.Run("FromEHR keeps the implicit filter and drops the absent predicate", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Build panicked: %v", r)
			}
		}()
		q, err := aql.NewBuilder().Select(aql.Col("c/x")).FromEHR("e", aql.Param("id")).Where(nilCmp).Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		const want = "SELECT c/x FROM EHR e WHERE e/ehr_id/value = $id"
		if q.Q != want {
			t.Errorf("Build = %q, want %q", q.Q, want)
		}
	})

	// A POPULATED pointer predicate must still build, identically to the value.
	t.Run("populated pointer builds", func(t *testing.T) {
		c := aql.Comparison{Path: "c/x", Op: aql.OpEq, Val: aql.Int(1)}
		viaPtr, err := aql.NewBuilder().Select(aql.Col("c/x")).From("COMPOSITION", "c").Where(&c).Build()
		if err != nil {
			t.Fatalf("pointer predicate refused: %v", err)
		}
		viaVal, err := aql.NewBuilder().Select(aql.Col("c/x")).From("COMPOSITION", "c").Where(c).Build()
		if err != nil {
			t.Fatalf("value predicate refused: %v", err)
		}
		if viaPtr.Q != viaVal.Q {
			t.Errorf("pointer built %q, value built %q", viaPtr.Q, viaVal.Q)
		}
	})
}

// TestEmitValidatesPagingAndOrderPositions — REQ-119 binds the value guards to
// EVERY position Emit renders, not the WHERE clause alone.
//
// `limitValue : INTEGER | PARAMETER` and `orderByExpr : identifiedPath …` are
// value positions, and [aql.Builder] has always guarded them. Emit did not, so
// the read and write sides disagreed on the same operand — and [parse.LimitExpr]
// turned out to be a THIRD sealed interface with value receivers, whose pointer
// twin `q.Limit != nil` waved through to a panicking token().
func TestEmitValidatesPagingAndOrderPositions(t *testing.T) {
	base := func(t *testing.T) *parse.Query {
		t.Helper()
		q, err := parse.ParseQuery("SELECT c/x FROM COMPOSITION c")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		return q
	}

	t.Run("refused", func(t *testing.T) {
		for name, mut := range map[string]func(*parse.Query){
			"typed-nil *IntLimit":    func(q *parse.Query) { q.Limit = (*parse.IntLimit)(nil) },
			"typed-nil *ParamLimit":  func(q *parse.Query) { q.Limit = (*parse.ParamLimit)(nil) },
			"negative LIMIT":         func(q *parse.Query) { q.Limit = parse.IntLimit{N: -1} },
			"empty LIMIT parameter":  func(q *parse.Query) { q.Limit = parse.ParamLimit{} },
			"LIMIT parameter with $": func(q *parse.Query) { q.Limit = parse.ParamLimit{Name: "$n"} },
			"LIMIT parameter spaced": func(q *parse.Query) { q.Limit = parse.ParamLimit{Name: "a b"} },
			"negative OFFSET": func(q *parse.Query) {
				q.Limit, q.Offset = parse.IntLimit{N: 5}, parse.IntLimit{N: -1}
			},
			"empty ORDER BY path": func(q *parse.Query) { q.OrderBy = []parse.OrderTerm{{}} },
			// OrderDir.String() spells any other value as ASC — the silently
			// re-directed sort Build refuses since Direction.known(); the read
			// side must refuse its own carrier of the same vocabulary.
			"out-of-vocabulary ORDER BY direction": func(q *parse.Query) {
				q.OrderBy = []parse.OrderTerm{{
					Path: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "c/y"}},
					Dir:  parse.OrderDir(7),
				}}
			},
		} {
			t.Run(name, func(t *testing.T) {
				q := base(t)
				mut(q)
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("Emit panicked: %v", r)
					}
				}()
				out, err := q.Emit()
				if !errors.Is(err, aql.ErrInvalidQuery) {
					t.Fatalf("err = %v, want ErrInvalidQuery (emitted %q)", err, out)
				}
				if out != "" {
					t.Errorf("a refused query still emitted %q", out)
				}
			})
		}
	})

	// Positive controls: what Emit must keep accepting, asserted as a fixed
	// point so a tightened guard fails here rather than shipping.
	t.Run("accepted", func(t *testing.T) {
		for _, q := range []string{
			"SELECT c/x FROM COMPOSITION c LIMIT 5",
			"SELECT c/x FROM COMPOSITION c LIMIT 0",
			"SELECT c/x FROM COMPOSITION c LIMIT 5 OFFSET 10",
			"SELECT c/x FROM COMPOSITION c LIMIT $n",
			"SELECT c/x FROM COMPOSITION c LIMIT $n OFFSET $m",
			"SELECT c/x FROM COMPOSITION c ORDER BY c/y ASC",
			"SELECT c/x FROM COMPOSITION c ORDER BY c/y DESC LIMIT 5",
		} {
			t.Run(q, func(t *testing.T) {
				doc, err := parse.ParseQuery(q)
				if err != nil {
					t.Fatalf("ParseQuery: %v", err)
				}
				out, err := doc.Emit()
				if err != nil {
					t.Fatalf("Emit refused AQL that ParseQuery accepted: %v", err)
				}
				if out != q {
					t.Errorf("Emit = %q, want the canonical input %q", out, q)
				}
			})
		}
	})

	// A POPULATED pointer operand must render, identically to the value shape —
	// the normalisation accepts pointers, it does not merely reject nil ones.
	// Without this the deref could be deleted and the refusals above would
	// still pass, since the default arm catches a pointer type too.
	t.Run("populated pointer operands render", func(t *testing.T) {
		for name, tc := range map[string]struct{ value, pointer parse.LimitExpr }{
			"IntLimit":   {parse.IntLimit{N: 5}, &parse.IntLimit{N: 5}},
			"ParamLimit": {parse.ParamLimit{Name: "n"}, &parse.ParamLimit{Name: "n"}},
		} {
			t.Run(name, func(t *testing.T) {
				q := base(t)
				q.Limit = tc.value
				viaValue, err := q.Emit()
				if err != nil {
					t.Fatalf("value shape refused: %v", err)
				}
				q = base(t)
				q.Limit = tc.pointer
				viaPointer, err := q.Emit()
				if err != nil {
					t.Fatalf("pointer shape refused: %v", err)
				}
				if viaPointer != viaValue {
					t.Errorf("pointer emitted %q, value emitted %q", viaPointer, viaValue)
				}
			})
		}
	})
}

// TestEmitTreatsAVersionClassAsAClassNode — REQ-119.
//
// checkComplete and isContainmentJunction are two spellings of "is this a class
// node?", and they disagreed: the spec says a class node carries an RM type OR
// is a VERSION class expression, checkComplete implements that, and
// isContainmentJunction read RMType alone. A `VERSION v` node with no RMType was
// therefore blessed as complete and then reclassified as a boolean grouping, so
// Emit dropped it from the chain — silently, into AQL that parses cleanly.
//
// [emitClassExpr] already treats Version as authoritative, so the two carriers
// of the same question now agree.
func TestEmitTreatsAVersionClassAsAClassNode(t *testing.T) {
	mk := func(cls parse.ClassExpr) *parse.Query {
		return &parse.Query{
			Select: parse.SelectClause{Items: []parse.SelectItem{{Expr: parse.PathExpr{
				IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "c/x"}},
			}}}},
			From: parse.FromClause{
				Root: parse.ClassExpr{RMType: "EHR", Alias: "e"},
				Contains: &parse.Containment{
					Class:    cls,
					Children: []parse.Containment{{Class: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"}}},
				},
			},
		}
	}
	const want = "SELECT c/x FROM EHR e CONTAINS VERSION v CONTAINS COMPOSITION c"

	// The extractor always sets RMType too, so only the first shape is
	// reachable by hand — which is exactly the path validateContainmentTree
	// exists to bind.
	for name, cls := range map[string]parse.ClassExpr{
		"Version flag only":      {Version: true, Alias: "v"},
		"Version flag + RM type": {RMType: "VERSION", Version: true, Alias: "v"},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := mk(cls).Emit()
			if err != nil {
				t.Fatalf("Emit: %v", err)
			}
			if out != want {
				t.Errorf("Emit = %q, want %q", out, want)
			}
		})
	}
}

// TestEmitSelectBindsThePointerTwin — REQ-119. [parse.SelectExpr] is the third
// sealed interface here whose methods have value receivers.
//
// It failed CLOSED (`unsupported SELECT expression *parse.LiteralExpr`), which
// is safe but still bound the rule to one carrier while the value side accepts
// `*aql.StringValue` — so a hand-built AST was refused for a shape the SDK
// otherwise treats as equivalent.
func TestEmitSelectBindsThePointerTwin(t *testing.T) {
	mk := func(e parse.SelectExpr) *parse.Query {
		return &parse.Query{
			Select: parse.SelectClause{Items: []parse.SelectItem{{Expr: e}}},
			From:   parse.FromClause{Root: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"}},
		}
	}

	for name, tc := range map[string]struct{ value, pointer parse.SelectExpr }{
		"literal": {
			value:   parse.LiteralExpr{Value: aql.String("x")},
			pointer: &parse.LiteralExpr{Value: aql.String("x")},
		},
		"path": {
			value: parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{
				IdentifiedPath: aql.IdentifiedPath{Raw: "c/y"},
			}},
			pointer: &parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{
				IdentifiedPath: aql.IdentifiedPath{Raw: "c/y"},
			}},
		},
		"function call": {
			value:   parse.FunctionCall{Name: "COUNT", Star: true},
			pointer: &parse.FunctionCall{Name: "COUNT", Star: true},
		},
		// The cases above twin the TOP-LEVEL carrier, which emitSelectExpr
		// normalises once at its head — they cannot see a nested position
		// that skips its own deref. The two rows below pin the argument
		// positions, and TERMINOLOGY is the one this test must hold alone:
		// validateSelectTerminology derefs twice (the SelectExpr wrapper and
		// the Value inside it), so dropping either call leaves another
		// `deref*` in the function and the function-granular source tripwire
		// stays quiet. Only this byte-identity check fails then.
		"TERMINOLOGY pointer-carrier args": {
			value: parse.FunctionCall{Name: "TERMINOLOGY", Args: selectLits("expand", "//fhir", "url=x")},
			pointer: parse.FunctionCall{Name: "TERMINOLOGY", Args: []parse.SelectExpr{
				&parse.LiteralExpr{Value: aql.String("expand")},
				&parse.LiteralExpr{Value: aql.String("//fhir")},
				&parse.LiteralExpr{Value: aql.String("url=x")},
			}},
		},
		"aggregate pointer-carrier arg": {
			value: parse.FunctionCall{Name: "AVG", Args: []parse.SelectExpr{
				parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{
					IdentifiedPath: aql.IdentifiedPath{Raw: "c/x"},
				}},
			}},
			pointer: parse.FunctionCall{Name: "AVG", Args: []parse.SelectExpr{
				&parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{
					IdentifiedPath: aql.IdentifiedPath{Raw: "c/x"},
				}},
			}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			viaValue, err := mk(tc.value).Emit()
			if err != nil {
				t.Fatalf("value shape refused: %v", err)
			}
			viaPointer, err := mk(tc.pointer).Emit()
			if err != nil {
				t.Fatalf("pointer shape refused: %v", err)
			}
			if viaPointer != viaValue {
				t.Errorf("pointer emitted %q, value emitted %q", viaPointer, viaValue)
			}
		})
	}

	t.Run("typed nil is refused, not panicked", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Emit panicked: %v", r)
			}
		}()
		out, err := mk((*parse.LiteralExpr)(nil)).Emit()
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery (emitted %q)", err, out)
		}
	})
}

// TestEmitRefusesAnIncompleteNodeInsideARootJunction — REQ-119.
//
// The completeness pass walks From.Junction as well as From.Contains, but only
// the PLACEMENT walk had a root-junction case, so restricting completeness to
// From.Contains left the suite green while Emit produced `FROM OBSERVATION o OR `.
func TestEmitRefusesAnIncompleteNodeInsideARootJunction(t *testing.T) {
	// Root and Junction are mutually exclusive — setting both is refused by an
	// earlier rule, which would mask this one entirely.
	root := parse.Containment{
		ChildJoin: parse.ContainsOr,
		Children: []parse.Containment{
			{Class: parse.ClassExpr{RMType: "OBSERVATION", Alias: "o"}},
			{Class: parse.ClassExpr{Alias: "broken"}}, // no RM type, no children
		},
	}
	q := &parse.Query{
		Select: parse.SelectClause{Items: []parse.SelectItem{{Expr: parse.PathExpr{
			IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "c/x"}},
		}}}},
		From: parse.FromClause{Junction: &root},
	}
	out, err := q.Emit()
	if err == nil || !strings.Contains(err.Error(), "requires an RM type") {
		t.Fatalf("err = %v, want the completeness rule (emitted %q)", err, out)
	}
	if !errors.Is(err, aql.ErrInvalidQuery) {
		t.Fatalf("err = %v, want ErrInvalidQuery (emitted %q)", err, out)
	}
	if out != "" {
		t.Errorf("a refused query still emitted %q", out)
	}
}

// TestContainmentRefusalsReportTheRuleThatFired — REQ-119 requires completeness
// to be checked BEFORE placement, and states why: the read side infers a node's
// KIND from an absent RM type, so in the other order a caller is told about a
// junction they never wrote.
//
// Neither containment test looked past errors.Is(err, ErrInvalidQuery), so the
// two guards were mutually substitutable as far as CI was concerned and the
// documented ordering was unpinned — swapping the two passes left the suite
// green. The tree below trips BOTH rules, so the message identifies the order.
func TestContainmentRefusalsReportTheRuleThatFired(t *testing.T) {
	mk := func(c parse.Containment) *parse.Query {
		return &parse.Query{
			Select: parse.SelectClause{Items: []parse.SelectItem{{Expr: parse.PathExpr{
				IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "c/x"}},
			}}}},
			From: parse.FromClause{
				Root:     parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"},
				Contains: &c,
			},
		}
	}

	// OBSERVATION carries a ChildJoin (a completeness violation) AND its chain
	// ends in a junction while a further term follows (a placement violation).
	both := parse.Containment{Class: parse.ClassExpr{RMType: "SECTION", Alias: "s"}}
	both.Children = []parse.Containment{
		{
			Class:     parse.ClassExpr{RMType: "OBSERVATION", Alias: "o"},
			ChildJoin: parse.ContainsOr,
			Children: []parse.Containment{{Children: []parse.Containment{
				{Class: parse.ClassExpr{RMType: "CLUSTER", Alias: "cl"}},
				{Class: parse.ClassExpr{RMType: "EVALUATION", Alias: "ev"}},
			}, ChildJoin: parse.ContainsOr}},
		},
		{Class: parse.ClassExpr{RMType: "ACTION", Alias: "a"}},
	}

	out, err := mk(both).Emit()
	if !errors.Is(err, aql.ErrInvalidQuery) {
		t.Fatalf("err = %v, want ErrInvalidQuery (emitted %q)", err, out)
	}
	if got := err.Error(); !strings.Contains(got, "ChildJoin") {
		t.Errorf("completeness must be diagnosed before placement, got %q", got)
	}

	// And the plain incomplete node still names the rule it broke, rather than
	// a junction the caller never wrote.
	incomplete := parse.Containment{Class: parse.ClassExpr{RMType: "SECTION", Alias: "s"}}
	incomplete.Children = []parse.Containment{{Class: parse.ClassExpr{Alias: "x"}}}
	_, err = mk(incomplete).Emit()
	if err == nil || !strings.Contains(err.Error(), "requires an RM type") {
		t.Errorf("incomplete node reported as %v, want the RM-type rule", err)
	}
}

// TestEmitRefusesImpureJunctionNodes — a junction node renders only its
// operands and the keyword, so data parked anywhere else on it has no wire
// form. [isContainmentJunction] classifies on RMType/Version/Children alone,
// so a junction whose Class carries a standing predicate or a $param flag
// skipped the class-operand checks AND rendered nothing — `err == nil` with
// the filter gone (REQ-119's substitution class; PROBE-090's converse rule:
// a field emission never renders in the given shape is refused, not dropped).
// Negation is the same hole at two positions the grammar cannot spell: the
// FROM root junction (nothing precedes it to carry a NOT) and a junction
// OPERAND (`NOT` belongs to a CONTAINS keyword).
func TestEmitRefusesImpureJunctionNodes(t *testing.T) {
	mk := func(from parse.FromClause) *parse.Query {
		return &parse.Query{
			Select: parse.SelectClause{Items: []parse.SelectItem{{Expr: parse.PathExpr{
				IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "c/x"}},
			}}}},
			From: from,
		}
	}
	operands := func() []parse.Containment {
		return []parse.Containment{
			{Class: parse.ClassExpr{RMType: "OBSERVATION", Alias: "o"}},
			{Class: parse.ClassExpr{RMType: "EVALUATION", Alias: "ev"}},
		}
	}
	nested := func(class parse.ClassExpr) parse.FromClause {
		return parse.FromClause{
			Root: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"},
			Contains: &parse.Containment{
				ChildJoin: parse.ContainsOr,
				Class:     class,
				Children:  operands(),
			},
		}
	}

	for name, tc := range map[string]struct {
		from parse.FromClause
		rule string // the diagnostic must name the rule, not just the sentinel
	}{
		"standing predicate comparison on a junction": {
			nested(parse.ClassExpr{PredicateComparison: &aql.Comparison{
				Path: "ehr_id/value", Op: aql.OpEq, Val: aql.StringValue{S: "x"},
			}}),
			"class data",
		},
		"$param archetype flag on a junction": {
			nested(parse.ClassExpr{ParamArchetype: true}),
			"class data",
		},
		"alias on a junction": {
			nested(parse.ClassExpr{Alias: "zz"}),
			"class data",
		},
		"archetype on a junction": {
			nested(parse.ClassExpr{Archetype: "openEHR-EHR-OBSERVATION.bp.v1"}),
			"class data",
		},
		"predicate text on a junction": {
			nested(parse.ClassExpr{Predicate: "at0001"}),
			"class data",
		},
		"negated FROM root junction": {
			parse.FromClause{Junction: &parse.Containment{
				Negated: true, ChildJoin: parse.ContainsAnd, Children: operands(),
			}},
			"root junction is negated",
		},
		"negated junction operand": {
			parse.FromClause{Junction: &parse.Containment{
				ChildJoin: parse.ContainsAnd,
				Children: []parse.Containment{
					{Class: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"}},
					{Negated: true, Class: parse.ClassExpr{RMType: "OBSERVATION", Alias: "o"}},
				},
			}},
			"operand 1 is negated",
		},
	} {
		t.Run("refused/"+name, func(t *testing.T) {
			out, err := mk(tc.from).Emit()
			if err == nil {
				t.Fatalf("Emit produced %q; the junction's extra data has no wire form", out)
			}
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("err = %v, want ErrInvalidQuery", err)
			}
			if !strings.Contains(err.Error(), tc.rule) {
				t.Errorf("err = %v, want the junction-purity rule (%q)", err, tc.rule)
			}
		})
	}

	// The positive control: the same trees WITHOUT the extra data emit and
	// round-trip, so the purity rules cannot be tightened into refusing pure
	// junctions.
	for name, from := range map[string]parse.FromClause{
		"pure nested junction": nested(parse.ClassExpr{}),
		"pure root junction": {Junction: &parse.Containment{
			ChildJoin: parse.ContainsAnd,
			Children: []parse.Containment{
				{Class: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"}},
				{Class: parse.ClassExpr{RMType: "OBSERVATION", Alias: "o"}},
			},
		}},
	} {
		t.Run("emits/"+name, func(t *testing.T) {
			out, err := mk(from).Emit()
			if err != nil {
				t.Fatalf("Emit refused a pure junction (tightening): %v", err)
			}
			if _, err := parse.ParseQuery(out); err != nil {
				t.Errorf("emitted %q does not re-parse: %v", out, err)
			}
		})
	}
}
