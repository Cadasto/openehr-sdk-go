package parse_test

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
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// TestMatchesURIGuardTracksTheGrammar — the `MATCHES {uri}` operand guard in
// [aql.MatchesExpr.validate] is a character-level rule derived from the URI
// token's alphabet. This confronts it with the grammar.
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
		// Spellable URI tokens — must survive it unchanged.
		"http://example.org/path",
		"http://example.org/p?a=1&b=2",
		"http://example.org/p#frag",
		"http://example.org/'",
		"http://example.org/a!$&'()*+,;=",
		"http://example.org/a%20b",
		"http://example.org:8080/p",
		"terminology://openehr.org/subsets/SNOMED-CT",
		"urn:ietf:rfc:3986",
		"mailto:someone@example.org",
		"svn+ssh://example.org/repo",
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

	t.Run("refused", func(t *testing.T) {
		out, err := mk(misplaced).Emit()
		if err == nil {
			t.Fatalf("Emit produced %q; a junction may only end a containment chain", out)
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
