package parse_test

// REQ-119 · PROBE-090 — issue #96.
//
// identifier_parity_test.go holds [aql.ValidateIdentifier] and
// [aql.ValidateArchetypeID] to the grammar they were hand-derived from.
//
// `openehr/aql` may not import the generated lexer (REQ-013 — the dependency
// runs the other way), so those guards carry hand-written rules. The property
// asserted here is not "the guard matches a list a maintainer remembered to
// update" but AGREEMENT with the parser: the guard accepts a string exactly when
// every position that splices it verbatim reads it back unchanged. A keyword
// added upstream fails here instead of silently becoming emittable.

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// identifierPositions are the three places the emitters write an IDENTIFIER
// token verbatim. Each returns the text the parser read back for that position,
// so "survives" means the SAME string returned, not merely that the query
// parsed — a splice parses perfectly well, as a different query.
var identifierPositions = map[string]func(s string) (string, bool){
	"SELECT AS alias": func(s string) (string, bool) {
		doc, err := parse.ParseQuery("SELECT c/x AS " + s + " FROM COMPOSITION c")
		if err != nil || len(doc.Select.Items) != 1 {
			return "", false
		}
		return doc.Select.Items[0].Alias, true
	},
	"class alias": func(s string) (string, bool) {
		doc, err := parse.ParseQuery("SELECT c/x FROM COMPOSITION " + s)
		if err != nil {
			return "", false
		}
		return doc.From.Root.Alias, true
	},
	"class RM type": func(s string) (string, bool) {
		doc, err := parse.ParseQuery("SELECT c/x FROM " + s + " c")
		if err != nil {
			return "", false
		}
		return doc.From.Root.RMType, true
	},
}

// TestIdentifierGuardTracksTheGrammar — the accept set of
// [aql.ValidateIdentifier] MUST equal "lexes as exactly one IDENTIFIER".
//
// The corpus is every token NAME in the vendored lexer, plus the spellings that
// live only in fragments (see keywordFragmentTexts — `true` / `false` are the
// counter-example that matters, since BOOLEAN is declared AFTER IDENTIFIER and
// they ARE valid aliases), plus the shapes below that no token name reaches.
func TestIdentifierGuardTracksTheGrammar(t *testing.T) {
	path := filepath.Join("..", "..", "..", "resources", "aql", "grammar", "active", "AqlLexer.g4")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read grammar: %v", err)
	}
	names := tokenNameRE.FindAllStringSubmatch(string(src), -1)
	if len(names) < 40 {
		t.Fatalf("only %d token names matched — the extraction regexp has drifted", len(names))
	}

	candidates := slices.Clone(keywordFragmentTexts)
	for _, m := range names {
		// SYM_* are the symbol tokens and are not identifier-shaped; every
		// other token NAME is itself a legal identifier spelling, so the parser
		// must agree about it.
		if !strings.HasPrefix(m[1], "SYM_") {
			candidates = append(candidates, m[1])
		}
	}
	candidates = append(candidates,
		// Ordinary aliases — the accept side of the property.
		"c", "o", "obs2", "a_b", "X", "x1", "COMPOSITION", "EHR", "myAlias",
		// ID_CODE / AT_CODE shadow IDENTIFIER (declared at :124-125), and their
		// `at` / `id` prefixes are LOWERCASE LITERALS, not the case-insensitive
		// letter fragments the keywords use — so the upper-case spellings are
		// perfectly good aliases. An alphabet check misses this entirely.
		"at0001", "id123", "at0", "AT0001", "ID123", "at", "id", "atx", "idx",
		"attribute", "identifier", "at0001x",
		// Alphabet failures.
		"", " ", "   ", "1x", "_x", "a-b", "a.b", "a/b", "a b", "ä", "ıd",
		// The splices this issue is about.
		"x, c/y", "c CONTAINS OBSERVATION o", "COMPOSITION c CONTAINS OBSERVATION",
		"c] OR c/y MATCHES {u://z", "c WHERE 1=1",
		// Lower- and mixed-case keywords: the lexer's letter fragments are
		// case-insensitive, so these shadow IDENTIFIER exactly as the upper-case
		// spellings do.
		"and", "And", "select", "Contains", "length", "Terminology", "count",
	)

	for _, s := range candidates {
		// Two strings are not identifier CANDIDATES at all, and both would
		// otherwise look like a position disagreement:
		//
		//   ""        — absence. `FROM COMPOSITION` is an anonymous class, so
		//               the alias position "round-trips" it while the RM type
		//               position cannot. Callers guard only a non-empty alias;
		//               asserted on its own below.
		//   "VERSION" — a different grammar ALTERNATIVE, not an identifier:
		//               `classExprOperand` has `VERSION variable=IDENTIFIER?`
		//               beside the IDENTIFIER form. The SDK carries it as
		//               [parse.ClassExpr.Version], so the guard refuses the word
		//               in every identifier position; asserted on its own below.
		if s == "" || strings.EqualFold(s, "VERSION") {
			continue
		}
		t.Run("candidate="+s, func(t *testing.T) {
			guardErr := aql.ValidateIdentifier(s)

			// A string is a legal identifier only if EVERY position reads it
			// back unchanged. They are all the same token, so they must agree;
			// a disagreement is itself a finding.
			var survivingIn, failingIn []string
			for name, position := range identifierPositions {
				if got, ok := position(s); ok && got == s {
					survivingIn = append(survivingIn, name)
				} else {
					failingIn = append(failingIn, name)
				}
			}
			slices.Sort(survivingIn)
			slices.Sort(failingIn)
			if len(survivingIn) != 0 && len(failingIn) != 0 {
				t.Fatalf("%q survives %v but not %v — the three IDENTIFIER positions disagree, "+
					"so one guard cannot serve all of them", s, survivingIn, failingIn)
			}
			survives := len(failingIn) == 0

			switch {
			case guardErr == nil && !survives:
				t.Errorf("aql.ValidateIdentifier accepts %q, but no IDENTIFIER position reads it back "+
					"unchanged — the guard in openehr/aql/identifier.go is too permissive", s)
			case guardErr != nil && survives:
				t.Errorf("aql.ValidateIdentifier refuses %q (%v), but every IDENTIFIER position "+
					"round-trips it intact — the guard is too strict", s, guardErr)
			}
		})
	}

	// The two carve-outs the loop skips, asserted rather than assumed.
	t.Run("empty is absence, not an identifier", func(t *testing.T) {
		if err := aql.ValidateIdentifier(""); !errors.Is(err, aql.ErrInvalidQuery) {
			t.Errorf("ValidateIdentifier(\"\") = %v, want ErrInvalidQuery", err)
		}
		// …and the emitters must still accept an ANONYMOUS class, which is what
		// an empty alias denotes. This is the anti-tightening half.
		const anon = "SELECT c/x FROM COMPOSITION"
		doc, err := parse.ParseQuery(anon)
		if err != nil {
			t.Fatalf("ParseQuery: %v", err)
		}
		if out, err := doc.Emit(); err != nil || out != anon {
			t.Errorf("Emit = %q, %v; want %q with no error — an empty alias is absence", out, err, anon)
		}
	})

	t.Run("VERSION is a grammar alternative, not an identifier", func(t *testing.T) {
		for _, spelling := range []string{"VERSION", "version", "Version"} {
			if err := aql.ValidateIdentifier(spelling); !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("ValidateIdentifier(%q) = %v, want ErrInvalidQuery", spelling, err)
			}
		}
		// The SDK's carrier for that alternative still emits and round-trips —
		// refusing the WORD must not refuse the CLASS.
		const versioned = "SELECT c/x FROM EHR e CONTAINS VERSION v CONTAINS COMPOSITION c"
		doc, err := parse.ParseQuery(versioned)
		if err != nil {
			t.Fatalf("ParseQuery: %v", err)
		}
		if out, err := doc.Emit(); err != nil || out != versioned {
			t.Errorf("Emit = %q, %v; want %q with no error", out, err, versioned)
		}
	})
}

// TestArchetypeIDGuardTracksTheGrammar — the same property for
// [aql.ValidateArchetypeID] against `archetypePredicate : ARCHETYPE_HRID`.
//
// Round-trip IDENTITY is the oracle again, not parseability: a spliced HRID
// closes the bracket early and the tail becomes more query, which parses
// cleanly as something else.
func TestArchetypeIDGuardTracksTheGrammar(t *testing.T) {
	for _, id := range []string{
		// Spellable HRIDs.
		"openEHR-EHR-OBSERVATION.blood_pressure.v1",
		"openEHR-EHR-COMPOSITION.encounter.v1",
		"openEHR-EHR-CLUSTER.device.v0",
		"openEHR-EHR-OBSERVATION.body_temperature.v2",
		"openEHR-EHR-OBSERVATION.x.v1.0.2",
		"openEHR-EHR-OBSERVATION.x-y.v1",
		"org.example::openEHR-EHR-OBSERVATION.x.v1",
		"org.example.sub::openEHR-EHR-OBSERVATION.x.v1",
		"openEHR-EHR-OBSERVATION.x.v1-rc",
		"openEHR-EHR-OBSERVATION.x.v1-rc.2",
		"openEHR-EHR-OBSERVATION.x.v1-alpha",
		// Not spellable.
		"",
		"openEHR-EHR-OBSERVATION.x",
		"openEHR-EHR-OBSERVATION.x.1",
		"openEHR-OBSERVATION.x.v1",
		"openEHR-EHR-OBS-ERVATION.x.v1",
		"openEHR-EHR-OBSERVATION..v1",
		"openEHR-EHR-OBSERVATION.x.vv1",
		"openEHR-EHR-OBSERVATION.x.v",
		"openEHR-EHR-OBSERVATION.1x.v1",
		"1openEHR-EHR-OBSERVATION.x.v1",
		"openEHR EHR-OBSERVATION.x.v1",
		// The splice this guard exists for.
		"openEHR-EHR-COMPOSITION.x.v1] CONTAINS OBSERVATION[openEHR-EHR-OBSERVATION.y.v1",
		"openEHR-EHR-COMPOSITION.x.v1] OR c/y MATCHES {u://z",
	} {
		t.Run("archetype="+id, func(t *testing.T) {
			guardErr := aql.ValidateArchetypeID(id)

			survives := false
			doc, err := parse.ParseQuery("SELECT c/x FROM COMPOSITION c[" + id + "]")
			if err == nil {
				survives = doc.From.Root.Archetype == id && doc.From.Contains == nil
			}

			switch {
			case guardErr == nil && !survives:
				t.Errorf("aql.ValidateArchetypeID accepts %q, but it does not come back as one "+
					"archetype predicate on that id (err %v) — the guard is too permissive", id, err)
			case guardErr != nil && survives:
				t.Errorf("aql.ValidateArchetypeID refuses %q (%v), but it round-trips intact — "+
					"the guard is too strict", id, guardErr)
			}
		})
	}
}

// TestEmitRefusesIdentifierSplice — the four verbatim positions, one mutation
// target each. Removing the guard at any one of them fails here.
//
// `Predicate` is deliberately NOT among them: `standardPredicate |
// archetypePredicate | nodePredicate` is a whole sub-grammar rather than one
// token, so it has no equivalent guard and is tracked separately. That is
// recorded in REQ-119 § Out of scope, not left to be rediscovered.
func TestEmitRefusesIdentifierSplice(t *testing.T) {
	pathItem := parse.SelectItem{Expr: parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{
		IdentifiedPath: aql.IdentifiedPath{Raw: "c/x"},
	}}}
	mk := func(item parse.SelectItem, root parse.ClassExpr) *parse.Query {
		return &parse.Query{
			Select: parse.SelectClause{Items: []parse.SelectItem{item}},
			From:   parse.FromClause{Root: root},
		}
	}
	comp := parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"}

	for name, q := range map[string]*parse.Query{
		"SELECT alias splice": mk(
			parse.SelectItem{Expr: pathItem.Expr, Alias: "x, c/y"}, comp),
		"SELECT alias reserved word": mk(
			parse.SelectItem{Expr: pathItem.Expr, Alias: "AND"}, comp),
		"SELECT alias node code": mk(
			parse.SelectItem{Expr: pathItem.Expr, Alias: "at0001"}, comp),
		"class alias splice": mk(pathItem,
			parse.ClassExpr{RMType: "COMPOSITION", Alias: "c CONTAINS OBSERVATION o"}),
		"class alias reserved word": mk(pathItem,
			parse.ClassExpr{RMType: "COMPOSITION", Alias: "ORDER"}),
		"RM type splice": mk(pathItem,
			parse.ClassExpr{RMType: "COMPOSITION c CONTAINS OBSERVATION", Alias: "o"}),
		"RM type whitespace": mk(pathItem,
			parse.ClassExpr{RMType: "   ", Alias: "c"}),
		"RM type function word": mk(pathItem,
			parse.ClassExpr{RMType: "LENGTH", Alias: "c"}),
		// VERSION without the flag emits `FROM VERSION v`, which re-parses with
		// Version: true — an AST the caller did not write. The flag is the
		// carrier for that grammar alternative.
		"RM type VERSION without the flag": mk(pathItem,
			parse.ClassExpr{RMType: "VERSION", Alias: "v"}),
		"archetype splice": mk(pathItem, parse.ClassExpr{
			RMType: "COMPOSITION", Alias: "c",
			Archetype: "openEHR-EHR-COMPOSITION.x.v1] CONTAINS OBSERVATION[openEHR-EHR-OBSERVATION.y.v1",
		}),
		"archetype malformed": mk(pathItem, parse.ClassExpr{
			RMType: "COMPOSITION", Alias: "c",
			Archetype: "not-an-hrid",
		}),
		"archetype param without $": mk(pathItem, parse.ClassExpr{
			RMType: "COMPOSITION", Alias: "c",
			Archetype: "arch", ParamArchetype: true,
		}),
	} {
		t.Run("refused/"+name, func(t *testing.T) {
			out, err := q.Emit()
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Fatalf("err = %v, want ErrInvalidQuery (emitted %q)", err, out)
			}
			if out != "" {
				t.Errorf("a refused query still emitted %q", out)
			}
		})
	}

	// Positive controls, asserted as a text fixed point so a TIGHTENED guard
	// fails here rather than shipping. `true` and `AT0001` are the two the
	// obvious implementations get wrong in opposite directions.
	t.Run("accepted", func(t *testing.T) {
		for _, q := range []string{
			"SELECT c/x AS alias FROM COMPOSITION c",
			"SELECT c/x AS true FROM COMPOSITION c",
			"SELECT c/x AS AT0001 FROM COMPOSITION c",
			"SELECT c/x AS x1 FROM COMPOSITION c",
			"SELECT c/x FROM COMPOSITION c",
			"SELECT c/x FROM COMPOSITION",
			"SELECT c/x FROM COMPOSITION c CONTAINS OBSERVATION o",
			"SELECT c/x FROM COMPOSITION c[openEHR-EHR-COMPOSITION.encounter.v1]",
			"SELECT c/x FROM COMPOSITION c[$arch]",
			"SELECT c/x FROM EHR e CONTAINS VERSION v CONTAINS COMPOSITION c",
			"SELECT c/x FROM COMPOSITION c1 OR COMPOSITION c2",
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
}

// TestBuilderRefusesIdentifierSplice — Build/Emit parity from day one. The
// builder reaches the same three positions through From / FromEHR / Contains,
// and REQ-055 already promises its output is syntactically valid AQL.
func TestBuilderRefusesIdentifierSplice(t *testing.T) {
	for name, build := range map[string]func() (aql.Query, error){
		"From RM type": func() (aql.Query, error) {
			return aql.NewBuilder().Select(aql.Col("c/x")).
				From("COMPOSITION c CONTAINS OBSERVATION", "o").Build()
		},
		"From alias": func() (aql.Query, error) {
			return aql.NewBuilder().Select(aql.Col("c/x")).
				From("COMPOSITION", "c CONTAINS OBSERVATION o").Build()
		},
		"From reserved alias": func() (aql.Query, error) {
			return aql.NewBuilder().Select(aql.Col("c/x")).From("COMPOSITION", "AND").Build()
		},
		"From node-code alias": func() (aql.Query, error) {
			return aql.NewBuilder().Select(aql.Col("c/x")).From("COMPOSITION", "at0001").Build()
		},
		"FromEHR alias": func() (aql.Query, error) {
			return aql.NewBuilder().Select(aql.Col("c/x")).
				FromEHR("e CONTAINS COMPOSITION c", aql.Param("id")).Build()
		},
		"Contains RM type": func() (aql.Query, error) {
			return aql.NewBuilder().Select(aql.Col("c/x")).From("COMPOSITION", "c").
				Contains(aql.Class("OBSERVATION o CONTAINS CLUSTER", "x")).Build()
		},
		"Contains alias": func() (aql.Query, error) {
			return aql.NewBuilder().Select(aql.Col("c/x")).From("COMPOSITION", "c").
				Contains(aql.Class("OBSERVATION", "o] OR c/y MATCHES {u://z")).Build()
		},
		"Contains archetype": func() (aql.Query, error) {
			return aql.NewBuilder().Select(aql.Col("c/x")).From("COMPOSITION", "c").
				Contains(aql.Archetype("OBSERVATION", "o",
					"openEHR-EHR-OBSERVATION.x.v1] CONTAINS CLUSTER[openEHR-EHR-CLUSTER.y.v1")).Build()
		},
	} {
		t.Run("refused/"+name, func(t *testing.T) {
			q, err := build()
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Fatalf("err = %v, want ErrInvalidQuery (built %q)", err, q.Q)
			}
			if q.Q != "" {
				t.Errorf("a refused build still produced %q", q.Q)
			}
		})
	}

	// Positive controls: the shapes the builder must keep producing, each
	// asserted to re-parse so a tightened guard cannot pass silently.
	t.Run("accepted", func(t *testing.T) {
		for name, build := range map[string]func() (aql.Query, error){
			"plain": func() (aql.Query, error) {
				return aql.NewBuilder().Select(aql.Col("c/x")).From("COMPOSITION", "c").Build()
			},
			"boolean-spelled alias": func() (aql.Query, error) {
				return aql.NewBuilder().Select(aql.Col("true/x")).From("COMPOSITION", "true").Build()
			},
			"contains archetype": func() (aql.Query, error) {
				return aql.NewBuilder().Select(aql.Col("c/x")).From("COMPOSITION", "c").
					Contains(aql.Archetype("OBSERVATION", "o",
						"openEHR-EHR-OBSERVATION.blood_pressure.v1")).Build()
			},
			"from EHR": func() (aql.Query, error) {
				return aql.NewBuilder().Select(aql.Col("e/x")).FromEHR("e", aql.Param("id")).Build()
			},
		} {
			t.Run(name, func(t *testing.T) {
				q, err := build()
				if err != nil {
					t.Fatalf("Build refused a well-formed query: %v", err)
				}
				if _, err := parse.ParseQuery(q.Q); err != nil {
					t.Errorf("Build emitted %q, which does not parse: %v", q.Q, err)
				}
			})
		}
	})
}
