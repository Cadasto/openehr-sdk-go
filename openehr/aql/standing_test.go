package aql_test

// standing_test.go pins the REQ-163 standing-predicate carrier: the one
// `standardPredicate` comparison an ordinary class expression can now carry in
// class position, the canonical bracket it emits, the four refusals, and the
// end-to-end consequence the carrier exists for — REQ-161's own documented
// suppression shape, `… CONTAINS VERSIONED_COMPOSITION vo[uid/value = $vo]
// CONTAINS VERSION v[ALL_VERSIONS]`, is buildable and raises no
// aql_versioned_object_unreferenced.
//
// The canonical strings asserted here MUST equal what openehr/aql/parse emits
// for the same query; that is held mechanically by the REQ-163 rows in
// containment_roundtrip_test.go, which re-parse and re-emit every one of them.
// PROBE-088 · PROBE-097

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// standingQuery is the shared body of the REQ-163 standing-predicate fixtures:
// the compositions of one EHR reached through the containment term under test.
// It mirrors versionQuery (version_test.go) so the two carriers' goldens differ
// only in the construct they exercise.
func standingQuery(term aql.Containment) (aql.Query, error) {
	return aql.Select(aql.Col("c/uid/value")).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(term).
		Build()
}

// versionedObjectTerm is REQ-161's motivating suppression shape as a builder
// term: all versions of ONE versioned composition, selected by the container's
// own uid. Before REQ-163 it was unbuildable — [aql.Containment] carried an RM
// type, an alias and an archetype id and nothing else.
func versionedObjectTerm() aql.Containment {
	return aql.Class("VERSIONED_COMPOSITION", "vo").
		Predicated("uid/value", aql.OpEq, aql.Param("vo")).
		Contains(aql.Version("v", aql.AllVersions()).
			Contains(aql.Class("COMPOSITION", "c")))
}

// TestStandingPredicateEmission tables the canonical bracket: one space each
// side of the comparison operator, no padding inside the brackets, and the
// value rendered through REQ-119's canonical value spellings (REQ-163
// § Canonical spellings). It is [aql.Comparison]'s own renderer, the one a
// WHERE comparison uses, so the two cannot drift.
func TestStandingPredicateEmission(t *testing.T) {
	tests := []struct {
		name string
		term aql.Containment
		want string
	}{
		{
			name: "comparison against a parameter",
			term: aql.Class("VERSIONED_COMPOSITION", "vo").Predicated("uid/value", aql.OpEq, aql.Param("vo")),
			want: "VERSIONED_COMPOSITION vo[uid/value = $vo]",
		},
		{
			name: "comparison against a string literal",
			term: aql.Class("COMPOSITION", "c").Predicated("name/value", aql.OpEq, aql.String("Vital signs")),
			want: "COMPOSITION c[name/value = 'Vital signs']",
		},
		{
			// A non-`=` operator and an integer operand: the bracket's spelling
			// comes from [aql.Comparison], so every operator and every value
			// kind it renders in WHERE renders here identically.
			name: "comparison against an integer literal",
			term: aql.Class("OBSERVATION", "o").
				Predicated("data/events/data/items/value/magnitude", aql.OpGe, aql.Int(2)),
			want: "OBSERVATION o[data/events/data/items/value/magnitude >= 2]",
		},
		{
			// Absence stays the ordinary bare class expression: the carrier adds
			// a bracket, it does not make one mandatory.
			name: "no standing predicate",
			term: aql.Class("COMPOSITION", "c"),
			want: "COMPOSITION c",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := aql.NewBuilder().Select(aql.Col("x")).From("EHR", "e").Contains(tc.term).Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			want := "SELECT x FROM EHR e CONTAINS " + tc.want
			if q.String() != want {
				t.Fatalf("standing predicate emission mismatch:\n got: %q\nwant: %q", q.String(), want)
			}
		})
	}
}

// TestStandingPredicateIsImmutable pins the containment algebra's own rule for
// the new combinator: every combinator returns a NEW value, so predicating a
// term must not reach back into the operand it was derived from. Without it a
// caller reusing a base operand would find a row filter appearing on the other
// derivation — silently, since both still emit valid AQL.
func TestStandingPredicateIsImmutable(t *testing.T) {
	base := aql.Class("COMPOSITION", "c")
	predicated := base.Predicated("name/value", aql.OpEq, aql.String("Vital signs"))

	bare, err := aql.NewBuilder().Select(aql.Col("x")).From("EHR", "e").Contains(base).Build()
	if err != nil {
		t.Fatalf("Build(base): %v", err)
	}
	if got, want := bare.String(), "SELECT x FROM EHR e CONTAINS COMPOSITION c"; got != want {
		t.Fatalf("the base operand was modified in place:\n got: %q\nwant: %q", got, want)
	}
	with, err := aql.NewBuilder().Select(aql.Col("x")).From("EHR", "e").Contains(predicated).Build()
	if err != nil {
		t.Fatalf("Build(predicated): %v", err)
	}
	if !strings.Contains(with.String(), "[name/value = 'Vital signs']") {
		t.Fatalf("the derived operand lost its predicate: %q", with.String())
	}
}

// TestStandingPredicateMatchesGolden pins the committed canonical form. The
// goldens are new files, so no existing fixture is re-baselined (REQ-163
// § Additivity).
func TestStandingPredicateMatchesGolden(t *testing.T) {
	for name, term := range map[string]aql.Containment{
		"standing_predicate_param": aql.Class("COMPOSITION", "c").
			Predicated("uid/value", aql.OpEq, aql.Param("uid")),
		"standing_predicate_literal": aql.Class("COMPOSITION", "c").
			Predicated("name/value", aql.OpEq, aql.String("Vital signs")),
		// The third `pathPredicateOperand` alternative a [aql.Value] can spell:
		// `objectPath`. ID_CODE and AT_CODE have no value shape, so the
		// promised "one golden per standing-predicate operand kind" is complete
		// at the kinds the write side can express (conformance.md, PROBE-088).
		"standing_predicate_path_operand": aql.Class("COMPOSITION", "c").
			Predicated("context/end_time/value", aql.OpEq, aql.Path("context/start_time/value")),
		"standing_versioned_object": versionedObjectTerm(),
	} {
		t.Run(name, func(t *testing.T) {
			q, err := standingQuery(term)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if golden := readGolden(t, name+".aql"); q.String() != golden {
				t.Fatalf("built query does not match golden:\n   got: %q\n golden: %q", q.String(), golden)
			}
		})
	}
}

// TestStandingPredicateRefusals pins the build-time refusals of the carrier.
// Each MUST wrap aql.ErrInvalidQuery rather than emit text the parser rejects
// or, worse, text that asks a different question (REQ-163 § Acceptance).
func TestStandingPredicateRefusals(t *testing.T) {
	sel := func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("x")).From("EHR", "e")
	}
	refused := map[string]func() (aql.Query, error){
		// A junction carries no class of its own, so it has no bracket at all.
		"standing predicate on a containment junction": func() (aql.Query, error) {
			return sel().Contains(aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev")).
				Predicated("name/value", aql.OpEq, aql.String("x"))).Build()
		},
		// The two mutually exclusive spellings of ONE `[…]` position: emitting
		// either would silently drop the other's row filter.
		"standing predicate beside an archetype predicate": func() (aql.Query, error) {
			return sel().Contains(aql.Archetype("OBSERVATION", "o", "openEHR-EHR-OBSERVATION.body_temperature.v2").
				Predicated("name/value", aql.OpEq, aql.String("Temperature"))).Build()
		},
		"a second standing predicate on one class expression": func() (aql.Query, error) {
			return sel().Contains(aql.Class("COMPOSITION", "c").
				Predicated("name/value", aql.OpEq, aql.String("a")).
				Predicated("uid/value", aql.OpEq, aql.String("b"))).Build()
		},
		// The fork: a comparison on a VERSION node is legal AQL, but it is the
		// OTHER production and has its own carrier.
		"standing predicate on a VERSION class expression": func() (aql.Query, error) {
			return sel().Contains(aql.Class("VERSION", "v").
				Predicated("commit_audit/time_committed/value", aql.OpGt, aql.Param("since"))).Build()
		},
		"standing predicate on a VERSION node built with aql.Version": func() (aql.Query, error) {
			return sel().Contains(aql.Version("v", aql.LatestVersion()).
				Predicated("commit_audit/time_committed/value", aql.OpGt, aql.Param("since"))).Build()
		},
		// A malformed comparison, one refusal per malformation the carrier
		// itself can see (aql.Comparison.validate).
		"comparison with an unknown operator": func() (aql.Query, error) {
			return sel().Contains(aql.Class("COMPOSITION", "c").Predicated("uid/value", "==", aql.Int(1))).Build()
		},
		"comparison with an empty path": func() (aql.Query, error) {
			return sel().Contains(aql.Class("COMPOSITION", "c").Predicated("", aql.OpEq, aql.Int(1))).Build()
		},
		"comparison with a nil value": func() (aql.Query, error) {
			return sel().Contains(aql.Class("COMPOSITION", "c").Predicated("uid/value", aql.OpEq, nil)).Build()
		},
		// The witness that keeps the CARRIER's own check honest. A value with no
		// AQL spelling renders as its Go text (`+Inf`), which is a perfectly
		// shaped bracket to the text guard below and so passes it — only
		// aql.Comparison.validate refuses it. Remove that call and this row
		// is the one that fails.
		"comparison against a value with no AQL spelling": func() (aql.Query, error) {
			return sel().Contains(aql.Class("COMPOSITION", "c").
				Predicated("uid/value", aql.OpGt, aql.Real(math.Inf(1)))).Build()
		},
		// …and one the carrier cannot see: a path that closes the emitter's own
		// bracket early re-parses as a DIFFERENT query, which is what
		// ValidatePathPredicate — the guard this position already has — refuses.
		// Without that call this text builds and emits.
		"comparison whose path escapes the bracket": func() (aql.Query, error) {
			return sel().Contains(aql.Class("COMPOSITION", "c").
				Predicated("x] CONTAINS OBSERVATION o[y", aql.OpEq, aql.Int(1))).Build()
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

// TestStandingPredicateAcceptRows carries the ACCEPT direction of REQ-163
// § Acceptance, which the refusal table above deliberately does not stand in
// for: REQ-119's own record is that a corpus of refusal rows alone let three
// OVER-refusals ship green, each refusing text the parser reads back as the
// same query. Every row here is text the guards MUST let through, and each is
// checked by the property that matters — the emitted query re-parses with the
// bracket still one bracket, so nothing split out of it.
//
// The witness for "still one bracket" is the class-expression count: a
// predicate that terminated its bracket early would introduce a class
// expression the builder never wrote, which is the silent-substitution mode
// [aql.ValidatePathPredicate] exists to refuse.
func TestStandingPredicateAcceptRows(t *testing.T) {
	tests := []struct {
		name string
		term aql.Containment
		// classes is the number of class expressions the built query is made
		// of: the EHR root plus every class in term. It is written down rather
		// than derived from the emitted text, so it cannot agree with a defect
		// by measuring it.
		classes int
	}{
		{
			// The value operand cannot escape the bracket, because REQ-119's
			// canonical value spelling escapes the quote before the bracket text
			// is ever assembled. Refusing it would be an over-refusal of an
			// ordinary string that merely CONTAINS AQL-looking characters — a
			// composition legitimately named `Discharge [final]`, say.
			name: "string value carrying bracket and quote characters",
			term: aql.Class("COMPOSITION", "c").
				Predicated("name/value", aql.OpEq, aql.String("a'] CONTAINS OBSERVATION o[b/c='d")),
			classes: 2,
		},
		{
			name: "string value carrying a comment marker",
			term: aql.Class("COMPOSITION", "c").
				Predicated("name/value", aql.OpEq, aql.String("before -- after")),
			classes: 2,
		},
		{
			// `objectPath : pathPart (SYM_SLASH pathPart)*` and
			// `pathPart : IDENTIFIER pathPredicate?`, so a path segment may
			// carry a node predicate of its own. The brackets are BALANCED, so
			// nothing escapes — a guard that counted brackets without matching
			// them would refuse legal AQL here.
			name: "path segment carrying its own node predicate",
			term: aql.Class("COMPOSITION", "c").
				Predicated("content[openEHR-EHR-OBSERVATION.body_temperature.v2]/name/value",
					aql.OpEq, aql.String("Temperature")),
			classes: 2,
		},
		{
			// The motivating shape itself, as an accept row rather than only as
			// a golden: it is the one this carrier was added for.
			name:    "the REQ-161 suppression shape",
			term:    versionedObjectTerm(),
			classes: 4,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := standingQuery(tc.term)
			if err != nil {
				t.Fatalf("Build refused text it must accept: %v", err)
			}
			doc, err := parse.Parse(q.String())
			if err != nil {
				t.Fatalf("Parse(%q): %v", q.String(), err)
			}
			if got := len(doc.Classes); got != tc.classes {
				t.Fatalf("the predicate changed the containment structure: %d class expressions, want %d\n  %s",
					got, tc.classes, q.String())
			}
			emitted, err := doc.Query().Emit()
			if err != nil {
				t.Fatalf("Emit(%q): %v", q.String(), err)
			}
			if emitted != q.String() {
				t.Fatalf("accepted text does not round-trip to identity:\n builder: %s\n   parse: %s",
					q.String(), emitted)
			}
		})
	}
}

// TestStandingPredicateOnVersionNamesTheCarrier is REQ-163 § One carrier per
// grammar position's own acceptance row: the VERSION-node refusal MUST name the
// constructor that DOES carry the shape. A refusal that only cited the grammar
// would leave the caller with a legal query they cannot write.
//
// Both routes to a VERSION-spelled node are checked, because the builder has
// two: the RM-type spelling through [aql.Class] and the fixed-type constructor
// [aql.Version].
func TestStandingPredicateOnVersionNamesTheCarrier(t *testing.T) {
	for name, term := range map[string]aql.Containment{
		"spelled through aql.Class": aql.Class("VERSION", "v"),
		"built with aql.Version":    aql.Version("v", nil),
		// The ASCII fold the spelling test uses accepts every ASCII casing, so
		// a lower-case spelling is the same keyword and routes the same way.
		"lower-case spelling": aql.Class("version", "v"),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := aql.NewBuilder().Select(aql.Col("x")).From("EHR", "e").
				Contains(term.Predicated("commit_audit/time_committed/value", aql.OpGt, aql.Param("since"))).
				Build()
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Fatalf("err = %v, want ErrInvalidQuery", err)
			}
			if !strings.Contains(err.Error(), "aql.VersionCompare") {
				t.Errorf("refusal does not name the constructor that carries the shape: %v", err)
			}
		})
	}
}

// TestStandingPredicateDiagnosticsAreValueFree pins that a refused bracket is
// reported without reproducing what it compared against — the class predicate
// is where openEHR carries the identifiable root, and a Build error is what a
// consuming CDR logs (REQ-119 § the redaction rule).
func TestStandingPredicateDiagnosticsAreValueFree(t *testing.T) {
	// The witness has to BOTH refuse and carry a value: the bracket-escaping
	// path is what earns the refusal, the string literal is what must not come
	// back.
	const secret = "9d3d1b4e-0000-0000-0000-000000000000"
	_, err := aql.NewBuilder().Select(aql.Col("x")).From("EHR", "e").
		Contains(aql.Class("COMPOSITION", "c").
			Predicated("uid/value] CONTAINS OBSERVATION o[name/value", aql.OpEq, aql.String(secret))).
		Build()
	if !errors.Is(err, aql.ErrInvalidQuery) {
		t.Fatalf("err = %v, want ErrInvalidQuery — the witness no longer refuses, so it proves nothing", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("refusal reproduced the predicate value: %v", err)
	}
}

// TestBuiltStandingPredicateSuppressesUnreferenced is the REQ-163 § Acceptance
// row the whole carrier exists for: REQ-161's motivating suppression shape
// builds end to end through the builder, and aql_versioned_object_unreferenced
// stays silent on it.
//
// The unpredicated control is not decoration — it pins that REQ-161's advisory
// still FIRES on the shape it was written for (REQ-163 § Additivity: no REQ-161
// code, severity or Detail text changes), so a linter that had simply stopped
// raising the code could not pass this test.
func TestBuiltStandingPredicateSuppressesUnreferenced(t *testing.T) {
	const code = "aql_versioned_object_unreferenced"
	counts := func(t *testing.T, term aql.Containment) int {
		t.Helper()
		q, err := standingQuery(term)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		n := 0
		for _, issue := range lint.LintString(q.String(), nil).Issues {
			if issue.Code == code {
				n++
			}
		}
		return n
	}
	t.Run("predicated versioned object is silent", func(t *testing.T) {
		if n := counts(t, versionedObjectTerm()); n != 0 {
			t.Fatalf("%s fired %d times on a predicated VERSIONED_COMPOSITION, want 0", code, n)
		}
	})
	t.Run("unpredicated control still fires", func(t *testing.T) {
		control := aql.Class("VERSIONED_COMPOSITION", "vo").
			Contains(aql.Version("v", aql.AllVersions()).Contains(aql.Class("COMPOSITION", "c")))
		if n := counts(t, control); n != 1 {
			t.Fatalf("%s fired %d times on a bare VERSIONED_COMPOSITION, want 1 — REQ-161's advisory is unchanged",
				code, n)
		}
	})
}

// TestStandingPredicateOperandVocabulary pins the class bracket's operand accept
// set against the production it renders:
//
//	standardPredicate    : objectPath COMPARISON_OPERATOR pathPredicateOperand
//	pathPredicateOperand : primitive | objectPath | PARAMETER | ID_CODE | AT_CODE
//
// No function-call alternative, so `Func(…)` and [aql.Terminology] emit text the
// SDK's own parser rejects. It is the SAME guard the VERSION bracket runs
// (version_test.go) — one function, because the two carriers share one
// [aql.Comparison] and a rule written twice can drift.
//
// The ACCEPT rows carry their own weight (REQ-163 § Acceptance): `Path` is the
// `objectPath` alternative and is legal, so a guard written as "refuse anything
// that is not a literal or a parameter" would fail here rather than on the
// refusal rows.
func TestStandingPredicateOperandVocabulary(t *testing.T) {
	build := func(v aql.Value) (aql.Query, error) {
		return aql.NewBuilder().Select(aql.Col("x")).From("EHR", "e").
			Contains(aql.Class("COMPOSITION", "c").Predicated("name/value", aql.OpEq, v)).
			Build()
	}
	t.Run("refuses", func(t *testing.T) {
		for name, v := range map[string]aql.Value{
			"a plain function call": aql.Func("LENGTH", aql.Path("x")),
			"a TERMINOLOGY call":    aql.Terminology("expand", "openehr", "x"),
		} {
			t.Run(name, func(t *testing.T) {
				q, err := build(v)
				if !errors.Is(err, aql.ErrInvalidQuery) {
					t.Fatalf("err = %v (query %q), want ErrInvalidQuery", err, q.String())
				}
				if !strings.Contains(err.Error(), "pathPredicateOperand") {
					t.Errorf("refusal does not name the grammar production it enforces: %v", err)
				}
			})
		}
	})
	t.Run("accepts", func(t *testing.T) {
		for name, tc := range map[string]struct {
			val  aql.Value
			want string
		}{
			"PARAMETER":            {aql.Param("name"), "$name"},
			"a string primitive":   {aql.String("Vital signs"), "'Vital signs'"},
			"an integer primitive": {aql.Int(2), "2"},
			"objectPath":           {aql.Path("other/name/value"), "other/name/value"},
		} {
			t.Run(name, func(t *testing.T) {
				q, err := build(tc.val)
				if err != nil {
					t.Fatalf("Build refused an operand the production admits: %v", err)
				}
				want := "SELECT x FROM EHR e CONTAINS COMPOSITION c[name/value = " + tc.want + "]"
				if q.String() != want {
					t.Fatalf("operand emission mismatch:\n got: %q\nwant: %q", q.String(), want)
				}
				// The accept rows are only worth anything if the accepted text
				// is text the parser reads back the same way, so each is
				// re-parsed and re-emitted here as well as in the round-trip
				// corpus — an over-narrow guard fails above, an over-wide one
				// fails here.
				doc, perr := parse.Parse(q.String())
				if perr != nil {
					t.Fatalf("Parse(%q): %v", q.String(), perr)
				}
				emitted, eerr := doc.Query().Emit()
				if eerr != nil {
					t.Fatalf("Emit(%q): %v", q.String(), eerr)
				}
				if emitted != q.String() {
					t.Fatalf("accepted operand does not round-trip to identity:\n builder: %s\n   parse: %s",
						q.String(), emitted)
				}
			})
		}
	})
}

// TestPredicatedPathIsTrimmed pins the canonical spelling against edge
// whitespace in the caller's path. Every sibling constructor trims ([aql.Col],
// [aql.ColAs], [aql.Builder.From], [aql.Builder.OrderBy]); storing the padding
// verbatim emitted `c[  uid/value   = $v]`, contradicting REQ-163 § Canonical
// spellings' "no padding inside the brackets".
func TestPredicatedPathIsTrimmed(t *testing.T) {
	q, err := aql.NewBuilder().Select(aql.Col("x")).From("EHR", "e").
		Contains(aql.Class("COMPOSITION", "c").Predicated("  uid/value  ", aql.OpEq, aql.Param("v"))).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	const want = "SELECT x FROM EHR e CONTAINS COMPOSITION c[uid/value = $v]"
	if q.String() != want {
		t.Fatalf("padded path reached the bracket:\n got: %q\nwant: %q", q.String(), want)
	}
}
