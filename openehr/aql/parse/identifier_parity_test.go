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
		// A Unicode fold of the keyword is neither the keyword (the lexer's
		// fragments are ASCII, so `ſ` fails token recognition) nor a legal
		// IDENTIFIER — the ASCII-gated fold in validateRMTypeToken sends it to
		// ValidateIdentifier, where the bare fold once emitted it verbatim.
		"RM type unicode fold of VERSION": mk(pathItem,
			parse.ClassExpr{RMType: "VERſION", Alias: "v"}),
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
		// The `PARAMETER` alternative's own spelling. Without these three the
		// [aql.ValidateValue] call past the `$` prefix check is UNPINNED —
		// deleting it left the whole aql suite green, so the guard was
		// documented rather than tested. The splice row is the one that
		// matters: `$p AND c/secret = 1` is the shape REQ-055 rule 4 routes
		// caller data through.
		"archetype param malformed name": mk(pathItem, parse.ClassExpr{
			RMType: "COMPOSITION", Alias: "c",
			Archetype: "$1bad", ParamArchetype: true,
		}),
		"archetype param splice": mk(pathItem, parse.ClassExpr{
			RMType: "COMPOSITION", Alias: "c",
			Archetype: "$p AND c/secret = 1", ParamArchetype: true,
		}),
		"archetype param bare $": mk(pathItem, parse.ClassExpr{
			RMType: "COMPOSITION", Alias: "c",
			Archetype: "$", ParamArchetype: true,
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
		// As on the Emit side: without these the builder's own validateParamName
		// call is unpinned — deleting it left the whole aql suite green.
		"Contains archetype param malformed": func() (aql.Query, error) {
			return aql.NewBuilder().Select(aql.Col("c/x")).From("COMPOSITION", "c").
				Contains(aql.Archetype("OBSERVATION", "o", "$1bad")).Build()
		},
		"Contains archetype param splice": func() (aql.Query, error) {
			return aql.NewBuilder().Select(aql.Col("c/x")).From("COMPOSITION", "c").
				Contains(aql.Archetype("OBSERVATION", "o", "$p AND c/secret = 1")).Build()
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
			// The `$param` alternative must stay buildable — the anti-tightening
			// half of the three refusals above.
			"contains $param archetype": func() (aql.Query, error) {
				return aql.NewBuilder().Select(aql.Col("c/x")).From("COMPOSITION", "c").
					Contains(aql.Archetype("OBSERVATION", "o", "$arch")).Build()
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

// TestEmitRefusesFieldsItWouldSilentlyDrop — the OTHER direction of REQ-119's
// substitution class, and the one the identifier guards made reachable.
//
// The guards above refuse text that emits as MORE query than the AST holds.
// These refuse an AST that emits as LESS: a field [emitClassExpr] never reads
// is worse than an unguarded one, because `checkClassOperands` VALIDATES
// `Archetype` before the VERSION branch discards it — so a malformed id is
// reported while a well-formed one vanishes. The guard's own success is what
// hid the loss:
//
//	{Version: true, Alias: "v", Archetype: "…encounter.v1"}
//	-> FROM VERSION v      err == nil, a row filter gone
//
// Every row here emitted cleanly at 97732e5.
func TestEmitRefusesFieldsItWouldSilentlyDrop(t *testing.T) {
	pathItem := parse.SelectItem{Expr: parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{
		IdentifiedPath: aql.IdentifiedPath{Raw: "c/x"},
	}}}
	mk := func(from parse.FromClause) *parse.Query {
		return &parse.Query{
			Select: parse.SelectClause{Items: []parse.SelectItem{pathItem}},
			From:   from,
		}
	}
	junction := func() *parse.Containment {
		return &parse.Containment{ChildJoin: parse.ContainsOr, Children: []parse.Containment{
			{Class: parse.ClassExpr{RMType: "COMPOSITION", Alias: "a"}},
			{Class: parse.ClassExpr{RMType: "OBSERVATION", Alias: "b"}},
		}}
	}

	for name, q := range map[string]*parse.Query{
		// `VERSION variable=IDENTIFIER? ('[' versionPredicate ']')?` has no
		// RM-type identifier and no archetype slot, so both fields are dropped.
		"VERSION carries an RM type": mk(parse.FromClause{Root: parse.ClassExpr{
			Version: true, RMType: "COMPOSITION", Alias: "v",
		}}),
		// A fold-equal spelling is NOT the flag's carrier: emitClassExpr writes
		// its own literal `VERSION`, so `VERſION`'s bytes would be dropped —
		// the bare EqualFold read it as the carrier and dropped it silently.
		"VERSION carries a fold-equal RM type": mk(parse.FromClause{Root: parse.ClassExpr{
			Version: true, RMType: "VERſION", Alias: "v",
		}}),
		"VERSION carries an archetype": mk(parse.FromClause{Root: parse.ClassExpr{
			Version: true, RMType: "VERSION", Alias: "v",
			Archetype: "openEHR-EHR-COMPOSITION.encounter.v1",
		}}),
		"VERSION carries a $param archetype": mk(parse.FromClause{Root: parse.ClassExpr{
			Version: true, RMType: "VERSION", Alias: "v",
			Archetype: "$arch", ParamArchetype: true,
		}}),
		"VERSION flags $param with no carrier": mk(parse.FromClause{Root: parse.ClassExpr{
			Version: true, RMType: "VERSION", Alias: "v", ParamArchetype: true,
		}}),
		// The flag declares the bracket to BE `archetypePredicate`'s PARAMETER
		// alternative; with no parameter to render, no bracket is written at all.
		"class flags $param with no carrier": mk(parse.FromClause{Root: parse.ClassExpr{
			RMType: "COMPOSITION", Alias: "c", ParamArchetype: true,
		}}),
		// Beside a junction the root is never passed to emitClassExpr, so its
		// fields are dropped rather than refused — including splice text the
		// identifier guards would otherwise catch.
		"root junction beside a root alias": mk(parse.FromClause{
			Root: parse.ClassExpr{Alias: "ghost"}, Junction: junction(),
		}),
		"root junction beside a root archetype": mk(parse.FromClause{
			Root:     parse.ClassExpr{Archetype: "openEHR-EHR-COMPOSITION.encounter.v1"},
			Junction: junction(),
		}),
		"root junction beside a root archetype splice": mk(parse.FromClause{
			Root:     parse.ClassExpr{Archetype: "x.v1] CONTAINS OBSERVATION[y.v1"},
			Junction: junction(),
		}),
		"root junction beside a root predicate": mk(parse.FromClause{
			Root: parse.ClassExpr{Predicate: "ehr_id/value=$x"}, Junction: junction(),
		}),
		// The structured reading of the bracket without the verbatim text the
		// emitter actually renders. Unlike HasPredicate — a flag carrying no
		// content — this one carries the row filter itself.
		"structured predicate with no text": mk(parse.FromClause{Root: parse.ClassExpr{
			RMType: "EHR", Alias: "e", HasPredicate: true,
			PredicateComparison: &aql.Comparison{
				Path: "ehr_id/value", Op: aql.OpEq, Val: aql.Param("id"),
			},
		}}),
		// The emptiness EDGE of the predicate position — separate from the
		// structured-comparison rule above, and reachable WITHOUT one: the
		// emitter brackets any non-empty field, so a blank one emits `[   ]`,
		// which the parser rejects. Orthogonal to the bracket-ESCAPE scan:
		// blank text escapes nothing, and text that escapes is never blank.
		"blank standing predicate": mk(parse.FromClause{Root: parse.ClassExpr{
			RMType: "EHR", Alias: "e", HasPredicate: true, Predicate: "   ",
		}}),
		"blank standing predicate with a comparison": mk(parse.FromClause{Root: parse.ClassExpr{
			RMType: "EHR", Alias: "e", HasPredicate: true, Predicate: "\t",
			PredicateComparison: &aql.Comparison{
				Path: "ehr_id/value", Op: aql.OpEq, Val: aql.Param("id"),
			},
		}}),
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

	// Positive controls. `FROM VERSION v` round-trips with RMType "VERSION" AND
	// the flag set — the extractor pairs them, so that one spelling must stay
	// accepted or every VERSION query the parser produces becomes unemittable.
	// The junction rows are the anti-tightening half of the whole-value root
	// check: the parser leaves Root wholly zero there, and if it ever stops
	// doing so these fail rather than the refusals above silently widening.
	t.Run("accepted", func(t *testing.T) {
		for _, q := range []string{
			"SELECT c/x FROM VERSION v",
			"SELECT c/x FROM VERSION",
			"SELECT c/x FROM VERSION v[LATEST_VERSION]",
			"SELECT c/x FROM VERSION v[ALL_VERSIONS]",
			"SELECT c/x FROM EHR e CONTAINS VERSION v CONTAINS COMPOSITION c",
			"SELECT c/x FROM COMPOSITION c1 OR COMPOSITION c2",
			"SELECT c/x FROM COMPOSITION c1 AND OBSERVATION o1",
			"SELECT c/x FROM COMPOSITION c[openEHR-EHR-COMPOSITION.encounter.v1]",
			"SELECT c/x FROM COMPOSITION c[$arch]",
			"SELECT c/x FROM EHR e[ehr_id/value=$id] CONTAINS COMPOSITION c",
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

// TestArchetypeIDGuardAgreesOverGeneratedHRIDs is the mechanical half of
// [TestArchetypeIDGuardTracksTheGrammar].
//
// That test walks a hand-written corpus, which is exactly the "list a
// maintainer must remember to update" this file exists to avoid — fine for the
// named traps, useless for the branches nobody thought of. This one GENERATES
// the cross product of the four `ARCHETYPE_HRID` fragments, legal and illegal
// spellings of each, and asserts guard-parser agreement over all of it. The
// percent-encoded namespace (`LABEL : ALPHA_CHAR (NAME_CHAR | URI_PCT_ENCODED)*`)
// is the branch the hand corpus missed entirely.
func TestArchetypeIDGuardAgreesOverGeneratedHRIDs(t *testing.T) {
	namespaces := []string{
		"", "org::", "org.example::", "org.example.sub::",
		"org%2Eexample::", "org%2e::", "a-b::", "a_b::",
		// Illegal: a digit-led label, an empty label, a truncated escape,
		// a non-hex escape, an empty namespace.
		"1bad::", "org..x::", "org%2::", "org%zz::", "::",
	}
	roots := []string{
		"openEHR-EHR-OBSERVATION", "a-b-c", "a_1-b2-c3",
		// Illegal: two segments, four segments, a digit-led segment.
		"openEHR-EHR", "a-b-c-d", "1a-b-c",
	}
	concepts := []string{
		"x", "blood_pressure", "x-y", "x_1",
		// Illegal as ARCHETYPE_CONCEPT_ID (ALPHA_CHAR NAME_CHAR*): digit-led,
		// empty, and one carrying the '.' that only the version may introduce.
		"1x", "", "x.y",
	}
	versions := []string{
		"v1", "v0", "v1.0.2", "v1-rc", "v1-rc.2", "v1-alpha", "v1-alpha.3",
		// Illegal: no digits, doubled 'v', missing 'v', dangling separators,
		// a non-numeric revision.
		"v", "vv1", "1", "v1.", "v1-rc.", "v1-beta", "v1-rc.x",
	}

	var checked, mismatched int
	for _, ns := range namespaces {
		for _, root := range roots {
			for _, concept := range concepts {
				for _, version := range versions {
					id := ns + root + "." + concept + "." + version
					checked++

					guardErr := aql.ValidateArchetypeID(id)
					survives := false
					if doc, err := parse.ParseQuery("SELECT c/x FROM COMPOSITION c[" + id + "]"); err == nil {
						survives = doc.From.Root.Archetype == id && doc.From.Contains == nil
					}
					if (guardErr == nil) == survives {
						continue
					}
					mismatched++
					if mismatched > 10 {
						continue // report a sample, not a wall
					}
					if guardErr == nil {
						t.Errorf("aql.ValidateArchetypeID accepts %q, but it does not come back "+
							"as one archetype predicate on that id — the guard is too permissive", id)
					} else {
						t.Errorf("aql.ValidateArchetypeID refuses %q (%v), but it round-trips "+
							"intact — the guard is too strict", id, guardErr)
					}
				}
			}
		}
	}
	if mismatched > 10 {
		t.Errorf("%d further mismatches suppressed", mismatched-10)
	}
	// A generator that stops producing cases would pass silently otherwise.
	if checked < 3000 {
		t.Errorf("swept only %d HRIDs; the generator has lost its cross product", checked)
	}
	t.Logf("guard and parser agree on %d generated HRIDs", checked)
}

// TestBuildEmitParityModuloTrim pins the exact sense in which the two write
// paths "refuse the same strings" (REQ-119).
//
// [aql.Builder] NORMALISES leading and trailing whitespace on intake before
// applying the guard — pre-existing, documented behaviour whose own reason is
// REQ-119, since `From("   ", "c")` once emitted `FROM     c`, re-parsing with
// the alias as the RM type. So the two paths do NOT refuse the same strings;
// they refuse the same NORMALISED strings, and the untrimmed spelling is the
// one place they legitimately disagree.
//
// Trimming cannot manufacture a splice — it only removes surrounding
// whitespace, and the guard then runs on what is left — so this is a widening
// of the builder's accept set by exactly one harmless equivalence, not a hole.
// Asserted here so the spec's claim stays honest in both directions.
//
// Parity is over the normalised string with ONE documented exception, and the
// exception is asserted in the loop rather than omitted from the corpus:
// `VERSION` differs by CARRIER, not by spelling.
func TestBuildEmitParityModuloTrim(t *testing.T) {
	pathItem := parse.SelectItem{Expr: parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{
		IdentifiedPath: aql.IdentifiedPath{Raw: "c/x"},
	}}}
	emits := func(root parse.ClassExpr) bool {
		q := &parse.Query{
			Select: parse.SelectClause{Items: []parse.SelectItem{pathItem}},
			From:   parse.FromClause{Root: root},
		}
		_, err := q.Emit()
		return err == nil
	}

	corpus := []string{
		"COMPOSITION", "  COMPOSITION  ", "\tCOMPOSITION\n", "COMPOSITION c",
		"AND", "ORDER", "LENGTH", "at0001", "AT0001", "true", "false",
		"x1", "1x", "a-b", "a_b", "", "   ", "c, c/y", "$p",
		"VERSION", "  VERSION  ", "version",
	}
	for _, s := range corpus {
		trimmed := strings.TrimSpace(s)

		// `VERSION` is the ONE string the two paths legitimately disagree on in
		// the RM-type position, and it disagrees by CARRIER rather than by
		// spelling: parse.ClassExpr has a Version flag, so an unflagged
		// `RMType: "VERSION"` is refused there (it would re-parse WITH the flag,
		// an AST the caller did not write), while the builder has no flag and
		// the spelling is its carrier. Asserted rather than skipped — a claim of
		// parity that quietly omits its own exception is the overclaim this test
		// exists to prevent.
		if strings.EqualFold(trimmed, "VERSION") {
			t.Run("RM type asymmetry/"+s, func(t *testing.T) {
				if _, err := aql.NewBuilder().Select(aql.Col("c/x")).From(s, "c").Build(); err != nil {
					t.Errorf("Build(%q) = %v, want accepted — the builder's carrier is the spelling", s, err)
				}
				if emits(parse.ClassExpr{RMType: trimmed, Alias: "c"}) {
					t.Errorf("Emit(RMType: %q) accepted without the Version flag — it would "+
						"re-parse with the flag set", trimmed)
				}
				if !emits(parse.ClassExpr{RMType: trimmed, Alias: "c", Version: true}) {
					t.Errorf("Emit(RMType: %q, Version: true) refused — the flagged form is what "+
						"ParseQuery produces", trimmed)
				}
			})
			continue
		}

		t.Run("RM type/"+s, func(t *testing.T) {
			_, buildErr := aql.NewBuilder().Select(aql.Col("c/x")).From(s, "c").Build()
			if built, emitted := buildErr == nil, emits(parse.ClassExpr{RMType: trimmed, Alias: "c"}); built != emitted {
				t.Errorf("Build(%q) ok=%v (%v) but Emit(RMType: %q) ok=%v — "+
					"the paths disagree on the NORMALISED string", s, built, buildErr, trimmed, emitted)
			}
		})

		// The alias half skips what trims to empty: an absent alias is the
		// grammar's anonymous class, which Emit accepts and Build refuses. That
		// asymmetry is a documented write-side ergonomic choice (REQ-119
		// § Emit-side structural parity), not a guard disagreement.
		if trimmed == "" {
			continue
		}
		t.Run("alias/"+s, func(t *testing.T) {
			_, buildErr := aql.NewBuilder().Select(aql.Col("c/x")).From("COMPOSITION", s).Build()
			if built, emitted := buildErr == nil, emits(parse.ClassExpr{RMType: "COMPOSITION", Alias: trimmed}); built != emitted {
				t.Errorf("Build(alias %q) ok=%v (%v) but Emit(Alias: %q) ok=%v — "+
					"the paths disagree on the NORMALISED string", s, built, buildErr, trimmed, emitted)
			}
		})
	}
}

// TestBuilderExpressesTheVersionAlternative — the anti-tightening control for
// the one carve-out in the RM-type position.
//
// `VERSION` is not an identifier, and [aql.ValidateIdentifier] refuses it
// everywhere, which is correct. But `classExprOperand` has `VERSION
// variable=IDENTIFIER? …` as its own ALTERNATIVE, and the two write paths
// carry it differently: [parse.ClassExpr] has a Version FLAG, so there an
// unflagged `RMType: "VERSION"` is refused (it would re-parse WITH the flag,
// an AST the caller did not write); the builder has no such field, so there
// the spelling IS the carrier and nothing contradicts it.
//
// Guarding the position without that distinction made `EHR e CONTAINS VERSION
// v CONTAINS COMPOSITION c` unbuildable — ordinary AQL both ParseQuery and
// Emit round-trip, and buildable before the guard landed. This is what a
// positive control is for.
func TestBuilderExpressesTheVersionAlternative(t *testing.T) {
	for name, tc := range map[string]struct {
		build func() (aql.Query, error)
		want  string
	}{
		"FROM VERSION": {
			build: func() (aql.Query, error) {
				return aql.NewBuilder().Select(aql.Col("v/x")).From("VERSION", "v").Build()
			},
			want: "SELECT v/x FROM VERSION v",
		},
		"CONTAINS VERSION": {
			build: func() (aql.Query, error) {
				return aql.NewBuilder().Select(aql.Col("c/x")).FromEHR("e", nil).
					Contains(aql.Class("VERSION", "v").Contains(aql.Class("COMPOSITION", "c"))).Build()
			},
			want: "SELECT c/x FROM EHR e CONTAINS VERSION v CONTAINS COMPOSITION c",
		},
		// The lexer's keyword fragments are case-insensitive, so this is the
		// same alternative and must not be refused for its spelling.
		"lower-case spelling": {
			build: func() (aql.Query, error) {
				return aql.NewBuilder().Select(aql.Col("v/x")).From("version", "v").Build()
			},
			want: "SELECT v/x FROM version v",
		},
	} {
		t.Run("built/"+name, func(t *testing.T) {
			q, err := tc.build()
			if err != nil {
				t.Fatalf("Build refused a VERSION class expression: %v", err)
			}
			if q.Q != tc.want {
				t.Errorf("Build = %q, want %q", q.Q, tc.want)
			}
			if _, err := parse.ParseQuery(q.Q); err != nil {
				t.Errorf("Build emitted %q, which does not parse: %v", q.Q, err)
			}
		})
	}

	// The carve-out is for the RM-TYPE position only, and the alternative it
	// admits carries no archetype: `versionPredicate` has no archetype form, so
	// emitting one would break REQ-055's promise that Build output is valid AQL.
	for name, build := range map[string]func() (aql.Query, error){
		"VERSION as a class alias": func() (aql.Query, error) {
			return aql.NewBuilder().Select(aql.Col("c/x")).From("COMPOSITION", "VERSION").Build()
		},
		"VERSION as a CONTAINS alias": func() (aql.Query, error) {
			return aql.NewBuilder().Select(aql.Col("c/x")).From("COMPOSITION", "c").
				Contains(aql.Class("OBSERVATION", "VERSION")).Build()
		},
		"VERSION carrying an archetype": func() (aql.Query, error) {
			return aql.NewBuilder().Select(aql.Col("c/x")).From("COMPOSITION", "c").
				Contains(aql.Archetype("VERSION", "v", "openEHR-EHR-COMPOSITION.encounter.v1")).Build()
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
}
