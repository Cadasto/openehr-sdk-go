package aql_test

// projection_test.go pins the REQ-163 typed projection: the constructors that
// mirror parse.SelectClause, the clause-level DISTINCT flag, the canonical
// spellings that make the read-side round trip byte-IDENTITY, and the
// after-emission verification that closes the last REQ-119 write path.
//
// The canonical strings asserted here MUST equal what openehr/aql/parse emits
// for the same query; that is held mechanically by the REQ-163 projection rows
// in containment_roundtrip_test.go, which re-parse and re-emit every one of
// them. Where a refusal is pinned, the REFERENCE procedure is applied too: the
// bytes Build would have emitted are handed to the parser, which reads them back
// as a DIFFERENT query — the silent-substitution class the refusal exists for.
// PROBE-020 · PROBE-088

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// projectionQuery is the shared body of the REQ-163 projection fixtures: the
// compositions of one EHR, projected through the items under test. It mirrors
// standingQuery (standing_test.go) so the goldens differ only in the construct
// they exercise.
func projectionQuery(cols ...aql.SelectField) (aql.Query, error) {
	return aql.Select(cols...).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Class("COMPOSITION", "c")).
		Build()
}

// TestProjectionEmission tables the canonical spellings of REQ-163
// § Canonical spellings: DISTINCT directly after SELECT and before the
// deprecated TOP, AS uppercase with one space each side, projected function and
// aggregate names in upper-case ASCII, `COUNT(DISTINCT path)` with one space
// after the keyword, `COUNT(*)` with no padding, arguments comma-separated with
// one space after each comma, and a sole unaliased star as the bare `SELECT *`.
func TestProjectionEmission(t *testing.T) {
	tests := []struct {
		name  string
		build func() (aql.Query, error)
		want  string
	}{
		{
			name:  "legacy Col is unchanged",
			build: func() (aql.Query, error) { return projectionQuery(aql.Col("c/uid/value")) },
			want:  "c/uid/value",
		},
		{
			name:  "typed path with an alias",
			build: func() (aql.Query, error) { return projectionQuery(aql.ColAs("c/uid/value", "uid")) },
			want:  "c/uid/value AS uid",
		},
		{
			name:  "alias added with As",
			build: func() (aql.Query, error) { return projectionQuery(aql.Col("c/uid/value").As("uid")) },
			want:  "c/uid/value AS uid",
		},
		{
			name:  "COUNT of a path",
			build: func() (aql.Query, error) { return projectionQuery(aql.Count("c/uid/value")) },
			want:  "COUNT(c/uid/value)",
		},
		{
			name:  "COUNT DISTINCT",
			build: func() (aql.Query, error) { return projectionQuery(aql.CountDistinct("c/uid/value")) },
			want:  "COUNT(DISTINCT c/uid/value)",
		},
		{
			name:  "COUNT star",
			build: func() (aql.Query, error) { return projectionQuery(aql.CountStar()) },
			want:  "COUNT(*)",
		},
		{
			name:  "aggregate with an alias",
			build: func() (aql.Query, error) { return projectionQuery(aql.CountStar().As("n")) },
			want:  "COUNT(*) AS n",
		},
		{
			// The name is upper-cased over ASCII letters at intake, which is
			// what identity requires: the extractor upper-cases every projected
			// name while the emitter renders the name as carried.
			name:  "function name is upper-cased",
			build: func() (aql.Query, error) { return projectionQuery(aql.Fn("max", aql.Col("c/x"))) },
			want:  "MAX(c/x)",
		},
		{
			name: "function call over several arguments",
			build: func() (aql.Query, error) {
				return projectionQuery(aql.Fn("CONCAT", aql.Col("c/a"), aql.Lit(aql.String(" ")), aql.Col("c/b")))
			},
			want: "CONCAT(c/a, ' ', c/b)",
		},
		{
			name:  "projected literal",
			build: func() (aql.Query, error) { return projectionQuery(aql.Lit(aql.Int(1))) },
			want:  "1",
		},
		{
			name:  "sole unaliased star is the bare form",
			build: func() (aql.Query, error) { return projectionQuery(aql.Star()) },
			want:  "*",
		},
		{
			name:  "star mixed with a column stays in place",
			build: func() (aql.Query, error) { return projectionQuery(aql.Star(), aql.Col("c/uid/value")) },
			want:  "*, c/uid/value",
		},
		{
			name: "DISTINCT sits directly after SELECT",
			build: func() (aql.Query, error) {
				return aql.Select(aql.Col("c/uid/value")).Distinct().
					FromEHR("e", aql.Param("ehr_id")).Contains(aql.Class("COMPOSITION", "c")).Build()
			},
			want: "DISTINCT c/uid/value",
		},
		{
			// `selectClause : SELECT DISTINCT? top? selectExpr …` — the flag
			// precedes the deprecated TOP clause, whichever order the setters
			// were called in.
			name: "DISTINCT precedes the deprecated TOP",
			build: func() (aql.Query, error) {
				return aql.Select(aql.Col("c/uid/value")).Top(5).Distinct().
					FromEHR("e", aql.Param("ehr_id")).Contains(aql.Class("COMPOSITION", "c")).Build()
			},
			want: "DISTINCT TOP 5 c/uid/value",
		},
		{
			name: "DISTINCT before a directed TOP",
			build: func() (aql.Query, error) {
				return aql.Select(aql.Col("c/uid/value")).Distinct().TopDirected(5, aql.TopBackward).
					FromEHR("e", aql.Param("ehr_id")).Contains(aql.Class("COMPOSITION", "c")).Build()
			},
			want: "DISTINCT TOP 5 BACKWARD c/uid/value",
		},
		{
			name: "projected TERMINOLOGY call",
			build: func() (aql.Query, error) {
				return projectionQuery(aql.Fn(aql.TerminologyFunc,
					aql.Lit(aql.String("expand")), aql.Lit(aql.String("openehr")), aql.Lit(aql.String("x"))))
			},
			want: "TERMINOLOGY('expand', 'openehr', 'x')",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := tc.build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			want := "SELECT " + tc.want + " FROM EHR e CONTAINS COMPOSITION c WHERE e/ehr_id/value = $ehr_id"
			if q.String() != want {
				t.Fatalf("projection emission mismatch:\n got: %q\nwant: %q", q.String(), want)
			}
		})
	}
}

// TestProjectionMatchesGolden pins the committed canonical form. The goldens are
// NEW files, so no existing fixture is re-baselined (REQ-163 § Additivity).
func TestProjectionMatchesGolden(t *testing.T) {
	for name, build := range map[string]func() (aql.Query, error){
		"select_path_alias": func() (aql.Query, error) { return projectionQuery(aql.ColAs("c/uid/value", "uid")) },
		"select_star":       func() (aql.Query, error) { return projectionQuery(aql.Star()) },
		"select_count_star": func() (aql.Query, error) { return projectionQuery(aql.CountStar().As("n")) },
		"select_count_distinct": func() (aql.Query, error) {
			return projectionQuery(aql.CountDistinct("c/uid/value"))
		},
		"select_function_call": func() (aql.Query, error) {
			return projectionQuery(aql.Fn("CONCAT", aql.Col("c/a"), aql.Lit(aql.String(" ")), aql.Col("c/b")))
		},
		"select_literal": func() (aql.Query, error) { return projectionQuery(aql.Lit(aql.Int(1))) },
		"select_distinct": func() (aql.Query, error) {
			return aql.Select(aql.Col("c/uid/value")).Distinct().
				FromEHR("e", aql.Param("ehr_id")).Contains(aql.Class("COMPOSITION", "c")).Build()
		},
		"select_distinct_top": func() (aql.Query, error) {
			return aql.Select(aql.Col("c/uid/value")).Distinct().Top(5).
				FromEHR("e", aql.Param("ehr_id")).Contains(aql.Class("COMPOSITION", "c")).Build()
		},
	} {
		t.Run(name, func(t *testing.T) {
			q, err := build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if golden := readGolden(t, name+".aql"); q.String() != golden {
				t.Fatalf("built query does not match golden:\n   got: %q\n golden: %q", q.String(), golden)
			}
		})
	}
}

// TestProjectionVerbAndStructBuilderAgree holds PROBE-020's property over the
// REQ-163 additions: the two builder styles share one emitter, so they produce
// byte-identical AQL for every new construct — including the clause-level flag,
// which only one of them can set first.
func TestProjectionVerbAndStructBuilderAgree(t *testing.T) {
	cases := map[string]struct {
		verb  func() (aql.Query, error)
		struc func() (aql.Query, error)
	}{
		"typed path alias": {
			verb: func() (aql.Query, error) {
				return aql.Select(aql.ColAs("c/uid/value", "uid")).From("COMPOSITION", "c").Build()
			},
			struc: func() (aql.Query, error) {
				return aql.NewBuilder().Select(aql.ColAs("c/uid/value", "uid")).From("COMPOSITION", "c").Build()
			},
		},
		"aggregates and literals": {
			verb: func() (aql.Query, error) {
				return aql.Select(aql.CountStar().As("n"), aql.CountDistinct("c/uid/value"), aql.Lit(aql.Int(1))).
					From("COMPOSITION", "c").Build()
			},
			struc: func() (aql.Query, error) {
				return aql.NewBuilder().
					Select(aql.CountStar().As("n"), aql.CountDistinct("c/uid/value"), aql.Lit(aql.Int(1))).
					From("COMPOSITION", "c").Build()
			},
		},
		"function call": {
			verb: func() (aql.Query, error) {
				return aql.Select(aql.Fn("concat", aql.Col("c/a"), aql.Col("c/b"))).From("COMPOSITION", "c").Build()
			},
			struc: func() (aql.Query, error) {
				return aql.NewBuilder().Select(aql.Fn("concat", aql.Col("c/a"), aql.Col("c/b"))).
					From("COMPOSITION", "c").Build()
			},
		},
		"star": {
			verb: func() (aql.Query, error) {
				return aql.Select(aql.Star()).From("COMPOSITION", "c").Build()
			},
			struc: func() (aql.Query, error) {
				return aql.NewBuilder().Select(aql.Star()).From("COMPOSITION", "c").Build()
			},
		},
		"DISTINCT with TOP": {
			verb: func() (aql.Query, error) {
				return aql.Select(aql.Col("c/uid/value")).Distinct().Top(3).From("COMPOSITION", "c").Build()
			},
			struc: func() (aql.Query, error) {
				return aql.NewBuilder().Distinct().Top(3).Select(aql.Col("c/uid/value")).
					From("COMPOSITION", "c").Build()
			},
		},
		"directed TOP": {
			verb: func() (aql.Query, error) {
				return aql.Select(aql.Col("c/uid/value")).TopDirected(3, aql.TopBackward).
					From("COMPOSITION", "c").Build()
			},
			struc: func() (aql.Query, error) {
				return aql.NewBuilder().TopDirected(3, aql.TopBackward).Select(aql.Col("c/uid/value")).
					From("COMPOSITION", "c").Build()
			},
		},
		// § Acceptance asks for the property PER CONSTRUCT, and REQ-163 adds
		// three. The two class-position carriers are exercised from here rather
		// than left to the projection rows alone — the constructors live in
		// their own files, but the property under test is this one.
		"version predicate": {
			verb: func() (aql.Query, error) {
				return aql.Select(aql.Col("v/uid/value")).From("EHR", "e").
					Contains(aql.Version("v", aql.LatestVersion())).Build()
			},
			struc: func() (aql.Query, error) {
				return aql.NewBuilder().Select(aql.Col("v/uid/value")).From("EHR", "e").
					Contains(aql.Version("v", aql.LatestVersion())).Build()
			},
		},
		"standing predicate": {
			verb: func() (aql.Query, error) {
				return aql.Select(aql.Col("vo/uid/value")).From("EHR", "e").
					Contains(aql.Class("VERSIONED_COMPOSITION", "vo").
						Predicated("uid/value", aql.OpEq, aql.Param("vo"))).Build()
			},
			struc: func() (aql.Query, error) {
				return aql.NewBuilder().Select(aql.Col("vo/uid/value")).From("EHR", "e").
					Contains(aql.Class("VERSIONED_COMPOSITION", "vo").
						Predicated("uid/value", aql.OpEq, aql.Param("vo"))).Build()
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			a, err := tc.verb()
			if err != nil {
				t.Fatalf("verb-style Build: %v", err)
			}
			b, err := tc.struc()
			if err != nil {
				t.Fatalf("struct-style Build: %v", err)
			}
			if a.String() != b.String() {
				t.Fatalf("the two builder styles diverge:\n  verb: %s\nstruct: %s", a.String(), b.String())
			}
		})
	}
}

// TestProjectionRefusals pins the build-time refusals of the typed
// constructors. Each MUST wrap aql.ErrInvalidQuery rather than emit text the
// parser rejects or, worse, text that asks a different question.
func TestProjectionRefusals(t *testing.T) {
	refused := map[string]func() (aql.Query, error){
		// The zero value carries no operand at all.
		"zero SelectField": func() (aql.Query, error) {
			return projectionQuery(aql.SelectField{})
		},
		"empty typed path": func() (aql.Query, error) {
			return projectionQuery(aql.ColAs("   ", "uid"))
		},
		// `aliasName=IDENTIFIER` is ONE token, so a spliced alias would
		// re-parse as a second projection.
		"alias that is not an identifier": func() (aql.Query, error) {
			return projectionQuery(aql.Col("c/uid/value").As("n, c/x"))
		},
		"empty alias": func() (aql.Query, error) {
			return projectionQuery(aql.Col("c/uid/value").As(""))
		},
		// `selectExpr`'s star alternative is SYM_ASTERISK alone — no AS slot.
		"alias on a star item": func() (aql.Query, error) {
			return projectionQuery(aql.Star().As("everything"))
		},
		// `columnExpr` has no PARAMETER alternative.
		"bare parameter projection": func() (aql.Query, error) {
			return projectionQuery(aql.Lit(aql.Param("p")))
		},
		"literal with no AQL spelling": func() (aql.Query, error) {
			return projectionQuery(aql.Lit(aql.Real(math.Inf(1))))
		},
		"nil literal": func() (aql.Query, error) {
			return projectionQuery(aql.Lit(nil))
		},
		// The name must lex as the grammar's IDENTIFIER or a *_FUNCTION_ID.
		"reserved function name": func() (aql.Query, error) {
			return projectionQuery(aql.Fn("SELECT", aql.Col("c/x")))
		},
		"function name outside the identifier alphabet": func() (aql.Query, error) {
			return projectionQuery(aql.Fn("co ncat", aql.Col("c/x")))
		},
		// `aggregateFunctionCall` fixes each aggregate's shape.
		"aggregate with no argument": func() (aql.Query, error) {
			return projectionQuery(aql.Fn("MIN"))
		},
		"aggregate with two arguments": func() (aql.Query, error) {
			return projectionQuery(aql.Fn("MIN", aql.Col("c/a"), aql.Col("c/b")))
		},
		"aggregate over a literal": func() (aql.Query, error) {
			return projectionQuery(aql.Fn("SUM", aql.Lit(aql.Int(1))))
		},
		"star as a function argument": func() (aql.Query, error) {
			return projectionQuery(aql.Fn("MAX", aql.Star()))
		},
		"aliased function argument": func() (aql.Query, error) {
			return projectionQuery(aql.Fn("CONCAT", aql.Col("c/a").As("x")))
		},
		// `terminologyFunction` fixes the arity AND the argument type.
		"TERMINOLOGY with the wrong arity": func() (aql.Query, error) {
			return projectionQuery(aql.Fn(aql.TerminologyFunc, aql.Lit(aql.String("expand"))))
		},
		"TERMINOLOGY over a non-string argument": func() (aql.Query, error) {
			return projectionQuery(aql.Fn(aql.TerminologyFunc,
				aql.Lit(aql.String("expand")), aql.Lit(aql.String("openehr")), aql.Lit(aql.Int(1))))
		},
		// A comma inside an argument re-parses as one more argument, so the
		// call comes back with an arity the builder never recorded.
		"comma inside a function argument": func() (aql.Query, error) {
			return projectionQuery(aql.Fn("CONCAT", aql.Col("c/a, c/b")))
		},
		"clause keyword inside a function argument": func() (aql.Query, error) {
			return projectionQuery(aql.Fn("CONCAT", aql.Col("c/a FROM EHR e2")))
		},
		// A per-argument imbalance the CLAUSE-level scan cannot see: the two
		// brackets balance across the emitted call, so only the per-argument
		// scan can tell that each argument leaves its own slot open.
		"brackets that balance across a call but not inside its arguments": func() (aql.Query, error) {
			return projectionQuery(aql.Fn("CONCAT", aql.Col("c/a["), aql.Col("c/b]")))
		},
		// The zero SelectField in an ARGUMENT slot: the item-level guard is
		// 280 lines away and never reaches this one (REQ-025's nil axis).
		"zero SelectField as a function argument": func() (aql.Query, error) {
			return projectionQuery(aql.Fn("CONCAT", aql.SelectField{}))
		},
		// The star row above uses MAX, which validateShape intercepts long
		// before renderArgument sees it; CONCAT is the shape that actually
		// reaches the argument guard.
		"star as an argument of an ordinary call": func() (aql.Query, error) {
			return projectionQuery(aql.Fn("CONCAT", aql.Star()))
		},
		"empty argument text": func() (aql.Query, error) {
			return projectionQuery(aql.Fn("CONCAT", aql.Col("")))
		},
		// `terminologyFunction`'s arity is right and its argument TYPE is not:
		// the production admits STRING and nothing else.
		"TERMINOLOGY over path arguments": func() (aql.Query, error) {
			return projectionQuery(aql.Fn(aql.TerminologyFunc, aql.Col("a"), aql.Col("b"), aql.Col("c")))
		},
	}
	for name, build := range refused {
		t.Run(name, func(t *testing.T) {
			q, err := build()
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Fatalf("err = %v (query %q), want ErrInvalidQuery", err, q.String())
			}
		})
	}
}

// TestProjectionFuncNameFoldIsASCII pins the FOLD that canonicalises a projected
// function name — [aql.Fn] upper-cases over ASCII letters only, never through
// strings.ToUpper — the third site of the polarity the two class-bracket
// siblings pin (TestVersionPredicateSpellingFoldIsASCII,
// TestStandingPredicateSpellingFoldIsASCII).
//
// This is the site where the fold runs BEFORE the name validator and feeds it,
// so a wider fold accepts MORE: the Unicode mapping folds some non-ASCII letters
// INTO the ASCII alphabet (`ı` → `I`, `ſ` → `S`), which turns a spelling the
// lexer cannot tokenise into a legal-looking one (REQ-163 § Canonical spellings,
// a MUST NOT). `mın` is the worst arm — the fold does not merely respell it, it
// MOVES the call into aql.IsAggregateFunc, so the aggregate shape rule would
// then be applied to a call the caller never wrote. That is the
// silent-substitution class REQ-119 and REQ-163 exist to close.
//
// The refusal names the offending spelling, and that is deliberate rather than a
// gap in TestProjectionDiagnosticsAreValueFree below: a projected function name
// has the developer-authored-identifier status REQ-055 rule 3 gives a path, not
// the caller-data status of a value, so the redaction rule does not bind it.
func TestProjectionFuncNameFoldIsASCII(t *testing.T) {
	// Every ASCII casing IS the name, and emits the one canonical spelling. The
	// accept rows are the positive control: a fold tightened into refusing valid
	// AQL fails here (REQ-163 § Acceptance).
	for _, name := range []string{"max", "MAX", "Max"} {
		t.Run("accepts "+name, func(t *testing.T) {
			q, err := projectionQuery(aql.Fn(name, aql.Col("c/uid/value")))
			if err != nil {
				t.Fatalf("Build refused the ASCII spelling %q: %v", name, err)
			}
			if !strings.Contains(q.String(), "MAX(c/uid/value)") {
				t.Fatalf("built %q, want the canonical MAX(c/uid/value)", q.String())
			}
		})
	}
	// The Unicode fold-equal spellings. Each is refused today; each would BUILD
	// under strings.ToUpper — `maıx` into a legal-looking identifier, `mın` and
	// `ſum` into AGGREGATES the caller never named.
	for _, name := range []string{"mın", "maıx", "ſum"} {
		t.Run("refuses "+name, func(t *testing.T) {
			q, err := projectionQuery(aql.Fn(name, aql.Col("c/uid/value")))
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Fatalf("err = %v (query %q), want ErrInvalidQuery — the spelling is not one the AQL "+
					"lexer can tokenise, and folding it into the ASCII alphabet would launder it into "+
					"a name the caller never wrote", err, q.String())
			}
		})
	}
}

// TestColSmugglingShapesArePinned is REQ-163 § Acceptance's per-shape ruling on
// the smuggling shapes the audit found. Each carries a NAMED expected verdict —
// not "either way", which would let the ruling pass whichever way it was
// implemented.
func TestColSmugglingShapesArePinned(t *testing.T) {
	t.Run("COUNT with an alias is tolerated", func(t *testing.T) {
		// One item, no clause-level flag: loud, ordinary AQL that says what the
		// caller wrote, so rule 1 keeps it (REQ-163 § `Col` stays lenient).
		q, err := projectionQuery(aql.Col("COUNT(c/uid/value) AS n"))
		if err != nil {
			t.Fatalf("Build refused a legacy Col that re-parses as one item: %v", err)
		}
		mustRoundTripToIdentity(t, q.String())
	})
	t.Run("leading DISTINCT is refused", func(t *testing.T) {
		// One item PLUS a clause-level flag the builder never set:
		// `selectClause : SELECT DISTINCT? top? selectExpr …` consumes the
		// keyword into the CLAUSE, so this is rule 2's silent mode wearing
		// rule 1's shape.
		_, err := projectionQuery(aql.Col("DISTINCT c/uid/value"))
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery", err)
		}
		if !strings.Contains(err.Error(), "DISTINCT") {
			t.Errorf("refusal does not name the flag it refused: %v", err)
		}
	})
	t.Run("a split item list is refused", func(t *testing.T) {
		_, err := projectionQuery(aql.Col("c/a, c/b"))
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery", err)
		}
		if !strings.Contains(err.Error(), "SELECT item 0") {
			t.Errorf("refusal does not name the item at fault: %v", err)
		}
	})
	t.Run("a leading TOP is refused", func(t *testing.T) {
		_, err := projectionQuery(aql.Col("TOP 5 c/uid/value"))
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery", err)
		}
		if !strings.Contains(err.Error(), "TOP") {
			t.Errorf("refusal does not name the flag it refused: %v", err)
		}
	})
	t.Run("a clause spill is refused", func(t *testing.T) {
		_, err := projectionQuery(aql.Col("c/uid/value FROM EHR e2"))
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery", err)
		}
		if !strings.Contains(err.Error(), "FROM") {
			t.Errorf("refusal does not name the keyword that spilled: %v", err)
		}
	})
	t.Run("an alias the builder did not record is refused", func(t *testing.T) {
		// The alias half of the compared structure: the builder recorded `n`,
		// the emitted text reads back `m AS n`, so the result set would come
		// back under a column name the caller never asked for.
		_, err := projectionQuery(aql.ColAs("c/uid/value AS m", "n"))
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery", err)
		}
	})
}

// TestColSmugglingWasASilentSubstitution is the REFERENCE procedure for the
// refusals above, and the reason each is a refusal rather than a lint warning:
// the bytes Build used to emit PARSE — as a different query. openehr/aql cannot
// import openehr/aql/parse (the arrow runs the other way), so the scan inside
// the package states the rule and this test proves the statement right.
func TestColSmugglingWasASilentSubstitution(t *testing.T) {
	t.Run("a split item list re-parsed as two projections", func(t *testing.T) {
		q, err := parse.ParseQuery("SELECT c/a, c/b FROM EHR e")
		if err != nil {
			t.Fatalf("the old emission no longer demonstrates the substitution: %v", err)
		}
		if len(q.Select.Items) != 2 {
			t.Fatalf("expected the one recorded item to have become 2, got %d", len(q.Select.Items))
		}
	})
	t.Run("a leading DISTINCT re-parsed as a clause flag", func(t *testing.T) {
		q, err := parse.ParseQuery("SELECT DISTINCT c/uid/value FROM EHR e")
		if err != nil {
			t.Fatalf("the old emission no longer demonstrates the substitution: %v", err)
		}
		if !q.Select.Distinct || len(q.Select.Items) != 1 {
			t.Fatalf("expected one item plus a clause-level DISTINCT, got Distinct=%t items=%d",
				q.Select.Distinct, len(q.Select.Items))
		}
	})
	t.Run("a leading TOP re-parsed as a clause row bound", func(t *testing.T) {
		q, err := parse.ParseQuery("SELECT TOP 5 c/uid/value FROM EHR e")
		if err != nil {
			t.Fatalf("the old emission no longer demonstrates the substitution: %v", err)
		}
		if q.Select.Top == nil || q.Select.Top.N != 5 {
			t.Fatalf("expected a clause-level TOP 5, got %+v", q.Select.Top)
		}
	})
	t.Run("an aliased legacy Col re-parses as the one item it looks like", func(t *testing.T) {
		// The control that keeps rule 1 honest: THIS shape is not a
		// substitution at all — one item, one alias, no clause flag — so
		// refusing it would be an over-refusal.
		q, err := parse.ParseQuery("SELECT COUNT(c/uid/value) AS n FROM EHR e")
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if q.Select.Distinct || q.Select.Top != nil || len(q.Select.Items) != 1 {
			t.Fatalf("expected exactly one flag-free item, got Distinct=%t top=%+v items=%d",
				q.Select.Distinct, q.Select.Top, len(q.Select.Items))
		}
	})
}

// TestProjectionAcceptRows carries the ACCEPT direction of REQ-163
// § Acceptance, which the refusal table deliberately does not stand in for:
// REQ-119's own record is that a corpus of refusal rows alone let THREE
// over-refusals ship green, each refusing text the parser reads back as the
// same query.
//
// One row per encoding the emitted text erases, plus the legacy shapes the
// leniency rule protects. Each is checked by the property that matters — the
// emitted query re-parses and re-emits to the same bytes, so nothing split out
// of the projection.
func TestProjectionAcceptRows(t *testing.T) {
	tests := []struct {
		name string
		cols []aql.SelectField
	}{
		{
			// The sole-star reduction: `SELECT *` re-parses as the flag with
			// ZERO items, so a raw carrier count would refuse it.
			name: "a sole unaliased star item",
			cols: []aql.SelectField{aql.Star()},
		},
		{
			name: "a sole unaliased star written as a legacy Col",
			cols: []aql.SelectField{aql.Col("*")},
		},
		{
			name: "a star mixed with a column",
			cols: []aql.SelectField{aql.Star(), aql.Col("c/uid/value")},
		},
		{
			// Path text enters its slot with edge whitespace normalised, so the
			// padded and unpadded spellings are ONE encoding.
			name: "edge-padded path text",
			cols: []aql.SelectField{aql.Col("  c/uid/value  ")},
		},
		{
			name: "edge-padded typed path and alias",
			cols: []aql.SelectField{aql.ColAs("  c/uid/value  ", "  uid  ")},
		},
		{
			name: "a legacy single-item Col",
			cols: []aql.SelectField{aql.Col("c/uid/value")},
		},
		{
			name: "a legacy Col carrying a function call and an alias",
			cols: []aql.SelectField{aql.Col("COUNT(c/uid/value) AS n")},
		},
		{
			// A comma inside a string literal is CONTENT: refusing it would
			// refuse an ordinary literal that merely looks like AQL.
			name: "a string literal carrying a comma",
			cols: []aql.SelectField{aql.Lit(aql.String("a, b"))},
		},
		{
			name: "a string literal carrying clause keywords",
			cols: []aql.SelectField{aql.Lit(aql.String("FROM EHR e DISTINCT"))},
		},
		{
			name: "a path carrying a node predicate with a comma",
			cols: []aql.SelectField{aql.Col("c/content[openEHR-EHR-OBSERVATION.body_temperature.v2,'Temp']/name/value")},
		},
		{
			// A path segment whose name merely CONTAINS a keyword's letters is
			// not that keyword: the match is bounded on both sides.
			name: "paths whose segments contain keyword letters",
			cols: []aql.SelectField{aql.Col("c/from_date"), aql.Col("c/topic"), aql.Col("c/as_of"), aql.Col("c/orderly")},
		},
		{
			name: "an aggregate over a path carrying a node predicate",
			cols: []aql.SelectField{aql.Count("c/content[openEHR-EHR-OBSERVATION.body_temperature.v2]/uid/value")},
		},
		{
			name: "a function call whose argument is a quoted separator",
			cols: []aql.SelectField{aql.Fn("CONCAT", aql.Col("c/a"), aql.Lit(aql.String(", ")), aql.Col("c/b"))},
		},
		{
			// A PARAMETER is a `terminal`, so it is legal in an argument slot
			// even though `columnExpr` has no PARAMETER alternative and the bare
			// projection is refused. The two rules are positional and opposite;
			// this row keeps the argument one from being tightened into the
			// projection one.
			name: "a parameter inside a function call's arguments",
			cols: []aql.SelectField{aql.Fn("CONCAT", aql.Lit(aql.String("id-")), aql.Lit(aql.Param("p")))},
		},
		{
			// `terminal` reaches an ordinary `functionCall`, so a NESTED plain
			// call is legal — the accept row the aggregate-argument refusal must
			// not swallow.
			name: "a nested ordinary function call",
			cols: []aql.SelectField{aql.Fn("CONCAT", aql.Fn("LENGTH", aql.Col("c/x")), aql.Lit(aql.String("!")))},
		},
		{
			// The direction keyword belongs to the TOP clause, and a query that
			// records one must still build: the comparison compares it, it does
			// not forbid it.
			name: "a path whose segments contain the direction keywords' letters",
			cols: []aql.SelectField{aql.Col("c/step_forward"), aql.Col("c/look_backward")},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := projectionQuery(tc.cols...)
			if err != nil {
				t.Fatalf("Build refused text it must accept: %v", err)
			}
			mustRoundTripToIdentity(t, q.String())
		})
	}
}

// TestSoleStarItemReducesToTheBareForm is the named mutation target for the
// star half of the reduction (REQ-163 § `Col` stays lenient, and `Build()`
// verifies what it emitted): the builder
// records ONE star item and the emitted text reads back as the bare `SELECT *`
// flag with ZERO items. Drop the reduction on either side of the comparison and
// the two disagree, so Build refuses the sole-star query — a self-refusal of a
// shape this REQ requires.
func TestSoleStarItemReducesToTheBareForm(t *testing.T) {
	q, err := projectionQuery(aql.Star())
	if err != nil {
		t.Fatalf("Build refused the sole-star projection: %v", err)
	}
	if !strings.HasPrefix(q.String(), "SELECT * FROM") {
		t.Fatalf("sole star did not emit the bare form: %q", q.String())
	}
	parsed, err := parse.ParseQuery(q.String())
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", q.String(), err)
	}
	// The reduction the comparison must apply, read off the parser itself: the
	// flag carries the projection and the item list is empty.
	if !parsed.Select.Star || len(parsed.Select.Items) != 0 {
		t.Fatalf("the bare form no longer reduces to flag+0 items: Star=%t items=%d",
			parsed.Select.Star, len(parsed.Select.Items))
	}
}

// TestProjectionShapeRefusalsAgreeWithEmit holds the builder's statement of the
// projected-call shape rules against parse.Query.Emit's, which is the same rule
// stated over the read-side carrier. openehr/aql cannot call that one (parse
// imports aql), so the two are held in agreement by this test rather than by
// shared code — the Build/Emit parity discipline of REQ-119.
//
// What this test does and does NOT cover, stated so the scope is not
// over-read from the name:
//
//   - Two rows below never reach [aql.Fn]'s shape rule at all — a reserved
//     function name is refused by the name check ahead of it, and a bare
//     parameter by the positional projection rule — so they hold the two sides
//     in agreement about the ITEM, not about the shape rule.
//   - The five STAR and DISTINCT arms of the shape rule are unreachable from
//     this package in the build direction: Count, CountDistinct and CountStar
//     each build ONE admitted shape and Fn cannot set either flag, so no
//     `aql.SelectField` expresses them. They are pinned where they can be —
//     from inside the package, by constructing the shapes directly, in
//     TestFuncColumnShapeArmsRefuseWhatTheConstructorsCannotBuild.
//   - The assertion compares the two VERDICTS (does each side wrap
//     [aql.ErrInvalidQuery]?) and not the two messages, which are written for
//     different carriers and are not required to match.
//
// The accept direction has its own rows below the refusal ones, because a
// parity test made only of refusals passes for a pair of guards that both
// refuse everything.
func TestProjectionShapeRefusalsAgreeWithEmit(t *testing.T) {
	tests := []struct {
		name  string
		built aql.SelectField
		read  parse.SelectExpr
	}{
		{
			name:  "aggregate with no argument",
			built: aql.Fn("MIN"),
			read:  parse.FunctionCall{Name: "MIN"},
		},
		{
			name:  "aggregate with two arguments",
			built: aql.Fn("MIN", aql.Col("c/a"), aql.Col("c/b")),
			read: parse.FunctionCall{Name: "MIN", Args: []parse.SelectExpr{
				parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "c/a"}}},
				parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "c/b"}}},
			}},
		},
		{
			name:  "aggregate over a literal",
			built: aql.Fn("SUM", aql.Lit(aql.Int(1))),
			read: parse.FunctionCall{Name: "SUM", Args: []parse.SelectExpr{
				parse.LiteralExpr{Value: aql.Int(1)},
			}},
		},
		{
			name:  "reserved function name",
			built: aql.Fn("SELECT", aql.Col("c/a")),
			read: parse.FunctionCall{Name: "SELECT", Args: []parse.SelectExpr{
				parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "c/a"}}},
			}},
		},
		{
			name:  "TERMINOLOGY with the wrong arity",
			built: aql.Fn(aql.TerminologyFunc, aql.Lit(aql.String("expand"))),
			read: parse.FunctionCall{Name: aql.TerminologyFunc, Args: []parse.SelectExpr{
				parse.LiteralExpr{Value: aql.String("expand")},
			}},
		},
		{
			name:  "bare parameter projection",
			built: aql.Lit(aql.Param("p")),
			read:  parse.LiteralExpr{Value: aql.Param("p")},
		},
		{
			// `terminal` reaches an ordinary `functionCall` and NOT
			// `aggregateFunctionCall`, so an aggregate in an argument slot is
			// text the parser rejects — which is how Emit reaches the same
			// verdict, through its own after-emission re-parse.
			name:  "an aggregate in an argument slot",
			built: aql.Fn("CONCAT", aql.CountStar()),
			read: parse.FunctionCall{Name: "CONCAT", Args: []parse.SelectExpr{
				parse.FunctionCall{Name: "COUNT", Star: true},
			}},
		},
		{
			name:  "an aggregate over a path in an argument slot",
			built: aql.Fn("CONCAT", aql.Fn("MIN", aql.Col("c/x"))),
			read: parse.FunctionCall{Name: "CONCAT", Args: []parse.SelectExpr{
				parse.FunctionCall{Name: "MIN", Args: []parse.SelectExpr{selectPath("c/x")}},
			}},
		},
		{
			// Right arity, wrong argument TYPE: `terminologyFunction` admits
			// STRING and nothing else.
			name:  "TERMINOLOGY over path arguments",
			built: aql.Fn(aql.TerminologyFunc, aql.Col("a"), aql.Col("b"), aql.Col("c")),
			read: parse.FunctionCall{Name: aql.TerminologyFunc, Args: []parse.SelectExpr{
				selectPath("a"), selectPath("b"), selectPath("c"),
			}},
		},
		{
			// The nil operand, one level below the item guard (REQ-025's axis).
			name:  "a nil operand in an argument slot",
			built: aql.Fn("CONCAT", aql.SelectField{}),
			read:  parse.FunctionCall{Name: "CONCAT", Args: []parse.SelectExpr{nil}},
		},
		{
			name:  "an empty path",
			built: aql.ColAs("   ", "x"),
			read:  selectPath(""),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, buildErr := projectionQuery(tc.built)
			readQ := &parse.Query{
				Select: parse.SelectClause{Items: []parse.SelectItem{{Expr: tc.read}}},
				From:   parse.FromClause{Root: parse.ClassExpr{RMType: "EHR", Alias: "e"}},
			}
			_, emitErr := readQ.Emit()
			if errors.Is(buildErr, aql.ErrInvalidQuery) != errors.Is(emitErr, aql.ErrInvalidQuery) {
				t.Fatalf("Build and Emit disagree on the same shape:\n Build: %v\n  Emit: %v", buildErr, emitErr)
			}
			if buildErr == nil {
				t.Fatalf("neither side refuses the shape, so the row proves nothing")
			}
		})
	}

	// The ACCEPT direction. Without it the whole table passes for a pair of
	// guards that refuse everything, which is the failure mode REQ-163
	// § Acceptance names for a refusal-only corpus.
	accepted := map[string]struct {
		built aql.SelectField
		read  parse.SelectExpr
	}{
		"the COUNT star form": {
			built: aql.CountStar(),
			read:  parse.FunctionCall{Name: "COUNT", Star: true},
		},
		"COUNT over a DISTINCT path": {
			built: aql.CountDistinct("c/uid/value"),
			read: parse.FunctionCall{Name: "COUNT", Distinct: true, Args: []parse.SelectExpr{
				selectPath("c/uid/value"),
			}},
		},
		"an aggregate over exactly one path": {
			built: aql.Fn("MIN", aql.Col("c/x")),
			read:  parse.FunctionCall{Name: "MIN", Args: []parse.SelectExpr{selectPath("c/x")}},
		},
		"a nested ordinary call": {
			built: aql.Fn("CONCAT", aql.Fn("LENGTH", aql.Col("c/x"))),
			read: parse.FunctionCall{Name: "CONCAT", Args: []parse.SelectExpr{
				parse.FunctionCall{Name: "LENGTH", Args: []parse.SelectExpr{selectPath("c/x")}},
			}},
		},
		"a parameter in an argument slot": {
			built: aql.Fn("CONCAT", aql.Lit(aql.Param("p"))),
			read: parse.FunctionCall{Name: "CONCAT", Args: []parse.SelectExpr{
				parse.LiteralExpr{Value: aql.Param("p")},
			}},
		},
		"TERMINOLOGY at its own arity and argument type": {
			built: aql.Fn(aql.TerminologyFunc,
				aql.Lit(aql.String("expand")), aql.Lit(aql.String("openehr")), aql.Lit(aql.String("x"))),
			read: parse.FunctionCall{Name: aql.TerminologyFunc, Args: []parse.SelectExpr{
				parse.LiteralExpr{Value: aql.String("expand")},
				parse.LiteralExpr{Value: aql.String("openehr")},
				parse.LiteralExpr{Value: aql.String("x")},
			}},
		},
	}
	for name, tc := range accepted {
		t.Run("accepts "+name, func(t *testing.T) {
			if _, err := projectionQuery(tc.built); err != nil {
				t.Fatalf("Build refused a shape the grammar admits: %v", err)
			}
			readQ := &parse.Query{
				Select: parse.SelectClause{Items: []parse.SelectItem{{Expr: tc.read}}},
				From:   parse.FromClause{Root: parse.ClassExpr{RMType: "EHR", Alias: "e"}},
			}
			if _, err := readQ.Emit(); err != nil {
				t.Fatalf("Emit refused the same shape, so the two sides disagree: %v", err)
			}
		})
	}
}

// selectPath spells a read-side projected path, which is four nested struct
// literals deep and would otherwise dominate every row that uses one.
func selectPath(raw string) parse.SelectExpr {
	return parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: raw}}}
}

// TestProjectionRefusalsNameTheItem pins the COORDINATE on every per-item
// refusal, including the ones the operand raises for itself.
//
// The coordinate is the whole diagnostic (REQ-119 § Emission verified after
// emission): a refusal may not quote the offending text, so a caller with a
// ten-item projection has nothing else to locate the defect by. The shape and
// name refusals used to arrive without one — `MIN() takes exactly one path
// argument` names the rule and not the place — which left the caller to find
// the item by elimination.
func TestProjectionRefusalsNameTheItem(t *testing.T) {
	// Each row puts the defect in item 2, behind two well-formed ones, so a
	// diagnostic that merely said "SELECT item 0" would fail here too.
	for name, bad := range map[string]aql.SelectField{
		"an aggregate with no argument":    aql.Fn("MIN"),
		"an aggregate with two":            aql.Fn("MIN", aql.Col("c/a"), aql.Col("c/b")),
		"an aggregate over a literal":      aql.Fn("SUM", aql.Lit(aql.Int(1))),
		"a reserved function name":         aql.Fn("SELECT", aql.Col("c/a")),
		"a name outside the alphabet":      aql.Fn("co ncat", aql.Col("c/a")),
		"a comma inside an argument":       aql.Fn("CONCAT", aql.Col("c/a, c/b")),
		"an aggregate in an argument":      aql.Fn("CONCAT", aql.CountStar()),
		"a call through the literal route": aql.Lit(aql.Func("CONCAT", aql.Path("c/a"))),
		"a literal with no AQL spelling":   aql.Lit(aql.Real(math.Inf(1))),
		"TERMINOLOGY at the wrong arity":   aql.Fn(aql.TerminologyFunc, aql.Lit(aql.String("expand"))),
		"an empty typed path":              aql.ColAs("   ", "x"),
		"a zero SelectField":               {},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := projectionQuery(aql.Col("c/x"), aql.Col("c/y"), bad)
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Fatalf("err = %v, want ErrInvalidQuery", err)
			}
			if !strings.Contains(err.Error(), "SELECT item 2") {
				t.Errorf("refusal does not name the item at fault: %v", err)
			}
		})
	}
}

// TestProjectionDiagnosticsAreValueFree pins that a refused projection is
// reported without reproducing the offending text — a Build error is what a
// consuming CDR logs, and a projection can carry an identifiable path or
// literal (REQ-119 § Emission verified after emission).
func TestProjectionDiagnosticsAreValueFree(t *testing.T) {
	const secret = "9d3d1b4e-0000-0000-0000-000000000000"
	// Each witness must BOTH refuse and carry the value: the structural defect
	// is what earns the refusal, the value is what must not come back.
	for name, build := range map[string]func() (aql.Query, error){
		"split item list": func() (aql.Query, error) {
			return projectionQuery(aql.Col("c/uid/value, c/name/value = '" + secret + "'"))
		},
		"clause spill": func() (aql.Query, error) {
			return projectionQuery(aql.Col("c/uid/value FROM EHR e2 WHERE x = '" + secret + "'"))
		},
		"unrecorded DISTINCT": func() (aql.Query, error) {
			return projectionQuery(aql.Col("DISTINCT c/name[" + secret + "]/value"))
		},
		"unterminated literal": func() (aql.Query, error) {
			return projectionQuery(aql.Col("c/name[at0001,'" + secret))
		},
		"function argument comma": func() (aql.Query, error) {
			return projectionQuery(aql.Fn("CONCAT", aql.Col("c/a, c/b['"+secret+"']")))
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := build()
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Fatalf("err = %v, want ErrInvalidQuery — the witness no longer refuses, so it proves nothing", err)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("refusal reproduced the projection text: %v", err)
			}
		})
	}
}

// TestTopDirectionIsPartOfTheComparedStructure is the named target for the TOP
// DIRECTION half of the clause-level flag comparison (REQ-163 § `Col` stays
// lenient, and `Build()` verifies what it emitted, whose second consequence
// makes the clause-level FLAGS part of the compared structure).
//
// It is the sharpest of the flags and the last one the comparison learned to
// see. `Col("BACKWARD c/uid/value")` beside a recorded `Top(5)` emits
// `SELECT TOP 5 BACKWARD c/uid/value`, which re-parses as a BACKWARD-directed
// TOP with one item and re-emits BYTE-IDENTICALLY: every round-trip, golden and
// parser check downstream agrees with itself, and the query returns the LAST
// five rows where the builder recorded the first. Nothing but this comparison
// can see it.
func TestTopDirectionIsPartOfTheComparedStructure(t *testing.T) {
	t.Run("a direction the builder never recorded is refused", func(t *testing.T) {
		_, err := aql.Select(aql.Col("BACKWARD c/uid/value")).Top(5).From("EHR", "e").Build()
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery", err)
		}
		if !strings.Contains(err.Error(), "BACKWARD") {
			t.Errorf("refusal does not name the direction it refused: %v", err)
		}
	})
	t.Run("the substitution it refuses was invisible", func(t *testing.T) {
		// The reference procedure, applied to the bytes Build used to emit: the
		// re-parse carries a direction the builder never set, and re-emitting it
		// reproduces the same bytes — so no round-trip check could have caught it.
		const emitted = "SELECT TOP 5 BACKWARD c/uid/value FROM EHR e"
		q, err := parse.ParseQuery(emitted)
		if err != nil {
			t.Fatalf("the old emission no longer demonstrates the substitution: %v", err)
		}
		if q.Select.Top == nil || q.Select.Top.Dir != aql.TopBackward {
			t.Fatalf("expected a clause-level BACKWARD direction, got %+v", q.Select.Top)
		}
		again, err := q.Emit()
		if err != nil || again != emitted {
			t.Fatalf("expected a byte-identical round trip: %q (err %v)", again, err)
		}
	})
	t.Run("a direction the builder DID record is accepted", func(t *testing.T) {
		// The over-refusal control: the comparison compares the direction, it
		// does not forbid it.
		q, err := aql.Select(aql.Col("c/uid/value")).TopDirected(5, aql.TopBackward).
			From("EHR", "e").Build()
		if err != nil {
			t.Fatalf("Build refused a directed TOP the builder itself recorded: %v", err)
		}
		if q.String() != "SELECT TOP 5 BACKWARD c/uid/value FROM EHR e" {
			t.Fatalf("built %q, want the canonical directed TOP", q.String())
		}
		mustRoundTripToIdentity(t, q.String())
	})
	t.Run("a recorded direction the item text reverses is refused", func(t *testing.T) {
		_, err := aql.Select(aql.Col("FORWARD c/uid/value")).TopDirected(5, aql.TopBackward).
			From("EHR", "e").Build()
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery", err)
		}
	})
	t.Run("a direction keyword with no TOP at all is refused", func(t *testing.T) {
		// The LOUD sibling of the silent shape above: with no TOP clause to
		// consume it, `SELECT BACKWARD c/x` is text the parser rejects outright.
		// Refusing it here is what keeps the two spellings from having opposite
		// verdicts for no reason a caller could state.
		_, err := aql.Select(aql.Col("BACKWARD c/x")).From("EHR", "e").Build()
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery", err)
		}
		if _, perr := parse.ParseQuery("SELECT BACKWARD c/x FROM EHR e"); perr == nil {
			t.Fatal("the loud sibling now parses, so the refusal needs re-deciding rather than pinning")
		}
	})
}

// TestProjectedLiteralRefusesAFunctionCall pins the ROUTING refusal of REQ-163
// § The typed projection model: `columnExpr`'s `primitive` alternative carries a
// primitive, and [aql.Fn] is the alternative that carries a call.
//
// Routing, not tightening: every query the refusal turns away stays expressible,
// through the constructor the message names. What it closes is a smuggle —
// `Lit(Func("CONCAT", Path("a, b")))` emitted `CONCAT(a, b)`, which re-parses
// with TWO arguments where the builder recorded one and re-emits byte-identically,
// because a literal never reaches the per-argument escape scan that refuses the
// same text written as `Fn("CONCAT", Col("a, b"))`.
func TestProjectedLiteralRefusesAFunctionCall(t *testing.T) {
	t.Run("the arity smuggle was invisible", func(t *testing.T) {
		const emitted = "SELECT CONCAT(a, b) FROM COMPOSITION c"
		q, err := parse.ParseQuery(emitted)
		if err != nil {
			t.Fatalf("the old emission no longer demonstrates the substitution: %v", err)
		}
		call, ok := q.Select.Items[0].Expr.(parse.FunctionCall)
		if !ok {
			t.Fatalf("expected a projected call, got %T", q.Select.Items[0].Expr)
		}
		if len(call.Args) != 2 {
			t.Fatalf("expected the one recorded argument to have become 2, got %d", len(call.Args))
		}
		again, err := q.Emit()
		if err != nil || again != emitted {
			t.Fatalf("expected a byte-identical round trip: %q (err %v)", again, err)
		}
	})
	t.Run("at the top of a projection", func(t *testing.T) {
		_, err := projectionQuery(aql.Lit(aql.Func("CONCAT", aql.Path("a, b"))))
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery", err)
		}
		if !strings.Contains(err.Error(), "aql.Fn") {
			t.Errorf("refusal does not name the constructor that carries a call: %v", err)
		}
	})
	t.Run("in an argument slot", func(t *testing.T) {
		// Same shape one level down, and refused for the same reason: nesting a
		// call is what Fn does, so the literal route has nothing left to carry.
		_, err := projectionQuery(aql.Fn("CONCAT", aql.Lit(aql.Func("LENGTH", aql.Path("c/x")))))
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery", err)
		}
	})
	t.Run("a well-formed call through the literal route is refused too", func(t *testing.T) {
		// Refuse-do-not-reroute: the shape decides, not whether this particular
		// call happens to be harmless.
		_, err := projectionQuery(aql.Lit(aql.Func("LENGTH", aql.Path("c/x"))))
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery", err)
		}
	})
	t.Run("a pointer twin is refused with the value shape", func(t *testing.T) {
		_, err := projectionQuery(aql.Lit(&aql.FuncCall{Name: "LENGTH", Args: []aql.Value{aql.Path("c/x")}}))
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery — token has a value receiver, so *FuncCall satisfies "+
				"aql.Value beside FuncCall and a rule bound to one carrier binds neither", err)
		}
	})
	// The two accept controls the refusal must not swallow. A PRIMITIVE and a
	// PARAMETER are what Lit is for, and `terminal` admits the parameter in an
	// argument slot even though `columnExpr` admits no bare one.
	t.Run("Lit keeps carrying primitives", func(t *testing.T) {
		q, err := projectionQuery(aql.Lit(aql.Int(1)))
		if err != nil {
			t.Fatalf("Build refused a projected primitive: %v", err)
		}
		mustRoundTripToIdentity(t, q.String())
	})
	t.Run("Lit keeps carrying a parameter in an argument slot", func(t *testing.T) {
		q, err := projectionQuery(aql.Fn("CONCAT", aql.Lit(aql.Param("p"))))
		if err != nil {
			t.Fatalf("Build refused a parameter argument, which `terminal` admits: %v", err)
		}
		mustRoundTripToIdentity(t, q.String())
	})
	t.Run("the bare-parameter projection stays refused", func(t *testing.T) {
		// The positional rule beside this one, unchanged: `columnExpr` has no
		// PARAMETER alternative.
		if _, err := projectionQuery(aql.Lit(aql.Param("p"))); !errors.Is(err, aql.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery", err)
		}
	})
}

// TestAggregateArgumentIsRefused pins the argument-slot half of the shape rule.
// `terminal : primitive | PARAMETER | identifiedPath | functionCall` reaches an
// ordinary call and NOT `aggregateFunctionCall`, which only `columnExpr` does —
// so an aggregate in an argument slot emits text the parser rejects outright.
func TestAggregateArgumentIsRefused(t *testing.T) {
	for name, arg := range map[string]aql.SelectField{
		"COUNT(*)":                   aql.CountStar(),
		"COUNT(path)":                aql.Count("c/x"),
		"COUNT(DISTINCT path)":       aql.CountDistinct("c/x"),
		"an Fn-built MIN":            aql.Fn("MIN", aql.Col("c/x")),
		"an Fn-built lower-case min": aql.Fn("min", aql.Col("c/x")),
		"an Fn-built AVG":            aql.Fn("AVG", aql.Col("c/x")),
	} {
		t.Run(name, func(t *testing.T) {
			q, err := projectionQuery(aql.Fn("CONCAT", arg))
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Fatalf("err = %v (query %q), want ErrInvalidQuery", err, q.String())
			}
		})
	}
	// The reference procedure: the bytes Build used to emit do not parse, so
	// this is a loud malformation the SDK was shipping to the server.
	t.Run("the emitted text had no reading", func(t *testing.T) {
		if _, err := parse.ParseQuery("SELECT CONCAT(COUNT(*)) FROM COMPOSITION c"); err == nil {
			t.Fatal("CONCAT(COUNT(*)) now parses, so the refusal needs re-deciding rather than pinning")
		}
	})
	// The accept controls: a NESTED ordinary call is a `terminal` and stays
	// legal, and the aggregates keep their own top-level position.
	t.Run("a nested ordinary call is still accepted", func(t *testing.T) {
		q, err := projectionQuery(aql.Fn("CONCAT", aql.Fn("LENGTH", aql.Col("c/x"))))
		if err != nil {
			t.Fatalf("Build refused a nested `terminal` call: %v", err)
		}
		if !strings.Contains(q.String(), "CONCAT(LENGTH(c/x))") {
			t.Fatalf("built %q, want the canonical nested call", q.String())
		}
		mustRoundTripToIdentity(t, q.String())
	})
	t.Run("an aggregate as its own SELECT item is still accepted", func(t *testing.T) {
		q, err := projectionQuery(aql.CountStar(), aql.Fn("MIN", aql.Col("c/x")))
		if err != nil {
			t.Fatalf("Build refused an aggregate in the position it belongs in: %v", err)
		}
		mustRoundTripToIdentity(t, q.String())
	})
}

// TestClosedTopLevelCommentIsContent pins the OVER-refusal direction of REQ-163
// § rule 2, which is exact in both directions: it refuses every text that
// changes the projection's recorded structure and NOTHING that does not.
//
// A comment is skipped by the lexer, and a CLOSED run is closed by its own
// newline — so it hides the rest of its line and nothing beyond, and the
// projection either side of it reads back as the projection the builder
// recorded. Only an UNCLOSED run carries on into the emitted query.
//
// The accepted case does NOT round-trip to byte identity — re-emitting drops the
// comment, as it drops every other piece of trivia — so the property asserted
// here is the one rule 2 is about: the emitted text reads back as the STRUCTURE
// the builder recorded. Rule 1 tolerates exactly that, "loud, ordinary AQL that
// says what the caller wrote".
func TestClosedTopLevelCommentIsContent(t *testing.T) {
	t.Run("a closed comment before an item is tolerated", func(t *testing.T) {
		q, err := projectionQuery(aql.Col("-- note\nc/uid/value"))
		if err != nil {
			t.Fatalf("Build refused a closed comment, which the parser reads back as the same item: %v", err)
		}
		parsed, err := parse.ParseQuery(q.String())
		if err != nil {
			t.Fatalf("ParseQuery(%q): %v", q.String(), err)
		}
		if parsed.Select.Distinct || parsed.Select.Star || len(parsed.Select.Items) != 1 {
			t.Fatalf("the emitted query does not read back as the one recorded item: "+
				"Distinct=%t Star=%t items=%d", parsed.Select.Distinct, parsed.Select.Star,
				len(parsed.Select.Items))
		}
		if parsed.From.Root.RMType != "EHR" {
			t.Fatalf("the comment swallowed the FROM clause after all: root = %+v", parsed.From.Root)
		}
	})
	t.Run("a closed comment inside a multi-item projection is tolerated", func(t *testing.T) {
		// The stripping has to run per FRAGMENT, so a comment in the second item
		// is a different code path from one in the first.
		q, err := projectionQuery(aql.Col("-- a\nc/uid/value"), aql.Col("-- b\nc/name/value"))
		if err != nil {
			t.Fatalf("Build refused closed comments inside a two-item projection: %v", err)
		}
		parsed, err := parse.ParseQuery(q.String())
		if err != nil {
			t.Fatalf("ParseQuery(%q): %v", q.String(), err)
		}
		if len(parsed.Select.Items) != 2 {
			t.Fatalf("the emitted query reads back as %d item(s), want the 2 recorded",
				len(parsed.Select.Items))
		}
	})
	t.Run("a comment before a sole star is REFUSED", func(t *testing.T) {
		// The trap in the leniency, and the reason the comment is stripped
		// BEFORE the sole-star reduction rather than after it: the parser reads
		// the bare star form — the flag with ZERO items — while the builder
		// recorded ONE non-star item, so the query asks for every column instead
		// of the one the caller named.
		_, err := projectionQuery(aql.Col("-- x\n*"))
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery", err)
		}
		parsed, perr := parse.ParseQuery("SELECT -- x\n* FROM EHR e")
		if perr != nil {
			t.Fatalf("the witness no longer parses, so it proves nothing: %v", perr)
		}
		if !parsed.Select.Star || len(parsed.Select.Items) != 0 {
			t.Fatalf("expected the bare star form (flag + 0 items), got Star=%t items=%d",
				parsed.Select.Star, len(parsed.Select.Items))
		}
	})
	t.Run("an unterminated comment is still refused", func(t *testing.T) {
		// No terminator inside the clause, so the run DOES carry on into the
		// emitted query and comments out the clauses after it.
		_, err := projectionQuery(aql.Col("c/uid/value -- note"))
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery", err)
		}
		if !strings.Contains(err.Error(), "unterminated") {
			t.Errorf("refusal does not name the flaw: %v", err)
		}
	})
	t.Run("a comment does not hide a clause spill", func(t *testing.T) {
		// The keyword after the run is still found: the run is replaced by a
		// space and not by nothing, so `c/a-- x\nFROM y` cannot close up into
		// one word and launder the keyword that ends the projection.
		_, err := projectionQuery(aql.Col("c/a-- x\nFROM EHR e2"))
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery", err)
		}
		if !strings.Contains(err.Error(), "FROM") {
			t.Errorf("refusal does not name the keyword that spilled: %v", err)
		}
	})
	t.Run("a comment does not hide a split item list", func(t *testing.T) {
		_, err := projectionQuery(aql.Col("c/a -- x\n, c/b"))
		if !errors.Is(err, aql.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery", err)
		}
	})
}

// TestFnCopiesItsArgumentSlice pins the intake copy of [aql.Fn].
//
// A `...SelectField` call site hands the callee the CALLER's backing array when
// the arguments were spread from a slice, so retaining it makes an already-built
// projection change what it emits when the caller later writes to their own
// slice — a query that is not the one the builder recorded, and one no guard in
// this file can see, because the substitution happens after every check has run.
// [Builder.Select] takes the same copy for the same reason.
func TestFnCopiesItsArgumentSlice(t *testing.T) {
	args := []aql.SelectField{aql.Col("c/a"), aql.Col("c/b")}
	b := aql.Select(aql.Fn("CONCAT", args...)).From("COMPOSITION", "c")
	first, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	args[1] = aql.Col("c/SUBSTITUTED")
	again, err := b.Build()
	if err != nil {
		t.Fatalf("Build after mutating the caller's slice: %v", err)
	}
	if again.String() != first.String() {
		t.Fatalf("a write to the caller's slice changed an already-recorded projection:\n first: %s\n"+
			" after: %s", first.String(), again.String())
	}
}

// TestSelectFieldComparabilityClaim pins [aql.SelectField] § Comparability,
// which was asserted in prose alone. A stale claim sends a consumer looking for
// a panic that no longer happens, or leaves a real one undocumented — the same
// reason aql.Value § Comparability is pinned in value_test.go.
func TestSelectFieldComparabilityClaim(t *testing.T) {
	mustPanic := func(what string, f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s did not panic; the doc claim in SelectField § Comparability is stale", what)
			}
		}()
		f()
	}
	mustPanic("`==` on two Fn-built SelectFields", func() {
		x := aql.Fn("CONCAT", aql.Col("c/a"))
		y := aql.Fn("CONCAT", aql.Col("c/a"))
		_ = x == y //nolint:staticcheck // deliberately provoking the documented panic
	})
	mustPanic("using an Fn-built SelectField as a map key", func() {
		m := map[aql.SelectField]bool{}
		m[aql.Fn("CONCAT", aql.Col("c/a"))] = true
	})
	// The other half of the claim, and the reason it is invisible to the
	// compiler: the slice is behind an INTERFACE field, so a Col-built field of
	// the very same type compares fine and nothing warns about the pair that
	// does not.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("`==` on two Col-built SelectFields panicked (%v); SelectField § Comparability "+
					"says the panic is specific to the Fn carrier", r)
			}
		}()
		if aql.Col("c/a") != aql.Col("c/a") {
			t.Error("two identical Col-built SelectFields did not compare equal")
		}
	}()
}

// mustRoundTripToIdentity applies the REFERENCE decision procedure of REQ-163
// § rule 2 to an accepted query: the emitted string is read back once and
// re-emitted, and the two MUST be the same bytes. Identity, not mere
// re-parseability, is the bar (§ Read-side mirror duty).
func mustRoundTripToIdentity(t *testing.T, built string) {
	t.Helper()
	doc, err := parse.Parse(built)
	if err != nil {
		t.Fatalf("Parse(%q): %v", built, err)
	}
	if qerr := doc.QueryErr(); qerr != nil {
		t.Fatalf("QueryErr(%q) = %v, want nil", built, qerr)
	}
	emitted, err := doc.Query().Emit()
	if err != nil {
		t.Fatalf("Emit(%q): %v", built, err)
	}
	if emitted != built {
		t.Fatalf("accepted text does not round-trip to identity:\n builder: %s\n   parse: %s", built, emitted)
	}
}
