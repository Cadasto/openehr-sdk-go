package aqlprobes

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
)

// Probe020CanonicalQuery is the canonical form of the PROBE-020 reference
// query, pinned here as a LITERAL rather than read from the golden file.
// Canonicalisation is a semver contract (wire.md § REQ-055): the REQ-117
// builder additions MUST leave a program that uses none of them emitting the
// same bytes, so PROBE-088 compares the committed golden against this constant
// as well as against a fresh build. Editing both the file and this constant is
// then the explicit, reviewable act a canonical-form change ought to be.
const Probe020CanonicalQuery = "SELECT o FROM EHR e CONTAINS COMPOSITION c " +
	"CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2] " +
	"WHERE e/ehr_id/value = $ehr_id"

// probe088Constructs are the builder constructs PROBE-088 pins, in assertion
// order — REQ-117's containment algebra and in-text paging, REQ-118's
// deprecated `SELECT TOP`, and REQ-163's three write-side carriers (the VERSION
// predicate, the class standing predicate, the typed projection). Each name is
// the base name of its golden file (`<name>.aql`) under
// openehr/aql/testdata/wire/; the probe owns the builder program so a
// downstream SDK implementing those REQs reproduces the same canonical string
// from the same logical query.
var probe088Constructs = []struct {
	name  string
	build func() (aql.Query, error)
}{
	{
		// Negated containment: report compositions carrying no
		// adverse-reaction-risk evaluation.
		name: "containment_not_contains",
		build: func() (aql.Query, error) {
			return aql.Select(aql.Col("c/uid/value")).
				FromEHR("e", aql.Param("ehr_id")).
				Contains(aql.Archetype("COMPOSITION", "c", "openEHR-EHR-COMPOSITION.report.v1").
					NotContains(aql.Archetype("EVALUATION", "ev", "openEHR-EHR-EVALUATION.adverse_reaction_risk.v1"))).
				Build()
		},
	},
	{
		// Sibling AND junction below a CONTAINS keyword: one composition
		// holding BOTH a temperature observation and a diagnosis.
		name: "containment_sibling_and",
		build: func() (aql.Query, error) {
			return aql.Select(aql.Col("o"), aql.Col("ev")).
				FromEHR("e", aql.Param("ehr_id")).
				Contains(aql.Class("COMPOSITION", "c").Contains(aql.ContainsAnd(
					aql.Archetype("OBSERVATION", "o", "openEHR-EHR-OBSERVATION.body_temperature.v2"),
					aql.Archetype("EVALUATION", "ev", "openEHR-EHR-EVALUATION.problem_diagnosis.v1"),
				))).
				Build()
		},
	},
	{
		// OR nested inside AND: the grouping is load-bearing (AND binds
		// tighter), so it MUST be parenthesised.
		name: "containment_or_grouped",
		build: func() (aql.Query, error) {
			return aql.Select(aql.Col("o"), aql.Col("o2")).
				FromEHR("e", aql.Param("ehr_id")).
				Contains(aql.Class("COMPOSITION", "c").Contains(aql.ContainsAnd(
					aql.ContainsOr(
						aql.Archetype("OBSERVATION", "o", "openEHR-EHR-OBSERVATION.body_temperature.v2"),
						aql.Archetype("OBSERVATION", "o2", "openEHR-EHR-OBSERVATION.blood_pressure.v2"),
					),
					aql.Class("INSTRUCTION", "i"),
				))).
				Build()
		},
	},
	{
		// Mixed nesting in one expression: a chain operand (parenthesised
		// because `CONTAINS containsExpr` is greedy), a NOT CONTAINS inside
		// it, and an AND junction under OR (bare — precedence agrees).
		name: "containment_mixed_nesting",
		build: func() (aql.Query, error) {
			return aql.Select(aql.Col("c/uid/value")).
				FromEHR("e", aql.Param("ehr_id")).
				Contains(aql.ContainsOr(
					aql.Class("COMPOSITION", "c").NotContains(aql.Class("SECTION", "s")),
					aql.ContainsAnd(aql.Class("EVALUATION", "ev"), aql.Class("INSTRUCTION", "i")),
				)).
				Build()
		},
	},
	{
		// In-text LIMIT alone (the grammar admits OFFSET only after LIMIT).
		name:  "paging_inline_limit",
		build: func() (aql.Query, error) { return inlinePagingQuery().LimitInline(50).Build() },
	},
	{
		// In-text LIMIT + OFFSET, integer literals.
		name: "paging_inline_limit_offset",
		build: func() (aql.Query, error) {
			return inlinePagingQuery().LimitInline(50).OffsetInline(100).Build()
		},
	},
	{
		// In-text LIMIT + OFFSET, parameter-bound (`limitValue : PARAMETER`).
		name: "paging_inline_limit_offset_param",
		build: func() (aql.Query, error) {
			return inlinePagingQuery().LimitInlineParam("rows").OffsetInlineParam("skip").Build()
		},
	},
	{
		// REQ-118: the DEPRECATED `SELECT TOP n` row limit — deprecated by
		// openEHR QUERY Release-1.1.0 §4.4.3, still buildable because the SDK
		// must be able to author a shape a client may legitimately send until
		// the announced removal.
		name:  "select_top",
		build: func() (aql.Query, error) { return inlinePagingQuery().Top(5).Build() },
	},
	{
		// REQ-118: `TOP n BACKWARD` — the direction is part of the bound, so
		// dropping it would select the opposite end of the ordered result.
		name: "select_top_backward",
		build: func() (aql.Query, error) {
			return inlinePagingQuery().TopDirected(5, aql.TopBackward).Build()
		},
	},

	// --- REQ-163: the version-predicate carrier -------------------------------
	//
	// `versionPredicate : LATEST_VERSION | ALL_VERSIONS | standardPredicate`,
	// one fixture per alternative. Before REQ-163 the builder could emit exactly
	// one VERSION shape — the predicate-less `VERSION v`, which is the shape the
	// SDK's own aql_version_no_predicate advises against.
	{
		name:  "version_latest_version",
		build: func() (aql.Query, error) { return probe088VersionQuery(aql.LatestVersion()) },
	},
	{
		name:  "version_all_versions",
		build: func() (aql.Query, error) { return probe088VersionQuery(aql.AllVersions()) },
	},
	{
		// The `standardPredicate` alternative in the VERSION bracket: the
		// commit-audit comparison a "versions committed since" query needs.
		name: "version_compare",
		build: func() (aql.Query, error) {
			return probe088VersionQuery(aql.VersionCompare(
				"commit_audit/time_committed/value", aql.OpGt, aql.Param("since")))
		},
	},

	// --- REQ-163: the standing-predicate carrier ------------------------------
	{
		name: "standing_predicate_param",
		build: func() (aql.Query, error) {
			return probe088StandingQuery(aql.Class("COMPOSITION", "c").
				Predicated("uid/value", aql.OpEq, aql.Param("uid")))
		},
	},
	{
		// A string operand, so the bracket's canonical value spelling is pinned
		// beside the parameter one: both render through aql.Comparison, the
		// renderer a WHERE comparison uses.
		name: "standing_predicate_literal",
		build: func() (aql.Query, error) {
			return probe088StandingQuery(aql.Class("COMPOSITION", "c").
				Predicated("name/value", aql.OpEq, aql.String("Vital signs")))
		},
	},
	{
		// The `objectPath` alternative of `pathPredicateOperand`, which the two
		// rows above do not reach: they cover `primitive` and `PARAMETER`.
		// Comparing two paths on one class expression is ordinary AQL — here,
		// encounters whose context began and ended at the same instant — and it
		// is the row that keeps the operand guard from being written as "a
		// literal or a parameter", which would refuse legal text.
		//
		// The production's remaining alternatives, ID_CODE and AT_CODE, have no
		// [aql.Value] shape to build them from, so they are not expressible from
		// the write side and no golden can pin them.
		name: "standing_predicate_path_operand",
		build: func() (aql.Query, error) {
			return probe088StandingQuery(aql.Class("COMPOSITION", "c").
				Predicated("context/end_time/value", aql.OpEq, aql.Path("context/start_time/value")))
		},
	},
	{
		// REQ-161's own documented suppression shape — all versions of ONE
		// versioned composition — which no builder program could express before
		// REQ-163 and which the linter's aql_versioned_object_unreferenced
		// advisory exists to point at.
		name:  "standing_versioned_object",
		build: func() (aql.Query, error) { return probe088StandingQuery(probe088VersionedObjectTerm()) },
	},

	// --- REQ-163: the typed projection ----------------------------------------
	//
	// One fixture per `columnExpr` alternative plus the clause-level flags:
	// an aliased path, the star, the two COUNT shapes, a function call, a
	// literal, DISTINCT alone and DISTINCT before the deprecated TOP.
	{
		name: "select_path_alias",
		build: func() (aql.Query, error) {
			return probe088ProjectionQuery(aql.ColAs("c/uid/value", "uid"))
		},
	},
	{
		name:  "select_star",
		build: func() (aql.Query, error) { return probe088ProjectionQuery(aql.Star()) },
	},
	{
		name:  "select_count_star",
		build: func() (aql.Query, error) { return probe088ProjectionQuery(aql.CountStar().As("n")) },
	},
	{
		name: "select_count_distinct",
		build: func() (aql.Query, error) {
			return probe088ProjectionQuery(aql.CountDistinct("c/uid/value"))
		},
	},
	{
		name: "select_function_call",
		build: func() (aql.Query, error) {
			return probe088ProjectionQuery(aql.Fn("CONCAT",
				aql.Col("c/a"), aql.Lit(aql.String(" ")), aql.Col("c/b")))
		},
	},
	{
		name:  "select_literal",
		build: func() (aql.Query, error) { return probe088ProjectionQuery(aql.Lit(aql.Int(1))) },
	},
	{
		name: "select_distinct",
		build: func() (aql.Query, error) {
			return aql.Select(aql.Col("c/uid/value")).Distinct().
				FromEHR("e", aql.Param("ehr_id")).
				Contains(aql.Class("COMPOSITION", "c")).
				Build()
		},
	},
	{
		// `selectClause : SELECT DISTINCT? top? selectExpr …` — the flag
		// precedes the deprecated TOP whichever order the setters were called
		// in, which is the half of the spelling a golden can pin.
		name: "select_distinct_top",
		build: func() (aql.Query, error) {
			return aql.Select(aql.Col("c/uid/value")).Distinct().Top(5).
				FromEHR("e", aql.Param("ehr_id")).
				Contains(aql.Class("COMPOSITION", "c")).
				Build()
		},
	},
}

// inlinePagingQuery is the shared body of the paging constructs: the most
// recent compositions of one EHR, ordered so a page is well-defined.
func inlinePagingQuery() *aql.Builder {
	return aql.Select(aql.Col("c/uid/value")).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Class("COMPOSITION", "c")).
		OrderBy("c/context/start_time/value", aql.Descending)
}

// probe088VersionQuery is the shared body of the REQ-163 version-predicate
// fixtures: the compositions of one EHR reached through a VERSION containment
// carrying pred. It is the same logical query openehr/aql's own version_test.go
// builds, so the package and the probe assert one golden rather than two
// fixtures that can drift.
func probe088VersionQuery(pred aql.VersionPredicate) (aql.Query, error) {
	return aql.Select(aql.Col("c/uid/value")).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Version("v", pred).Contains(aql.Class("COMPOSITION", "c"))).
		Build()
}

// probe088StandingQuery is the shared body of the REQ-163 standing-predicate
// fixtures: the compositions of one EHR reached through term. It mirrors
// probe088VersionQuery so the two carriers' goldens differ only in the
// construct they exercise.
func probe088StandingQuery(term aql.Containment) (aql.Query, error) {
	return aql.Select(aql.Col("c/uid/value")).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(term).
		Build()
}

// probe088VersionedObjectTerm is REQ-161's motivating suppression shape as a
// builder term: all versions of ONE versioned composition, selected by the
// container's own uid. It needs BOTH new class-position carriers at once — the
// standing predicate on the VERSIONED_COMPOSITION and the ALL_VERSIONS bracket
// below it — which is why it is the fixture that proves the pair composes.
func probe088VersionedObjectTerm() aql.Containment {
	return aql.Class("VERSIONED_COMPOSITION", "vo").
		Predicated("uid/value", aql.OpEq, aql.Param("vo")).
		Contains(aql.Version("v", aql.AllVersions()).
			Contains(aql.Class("COMPOSITION", "c")))
}

// probe088ProjectionQuery is the shared body of the REQ-163 projection
// fixtures: the compositions of one EHR, projected through cols. The two
// clause-level flag fixtures spell their own body instead, because
// [aql.Builder.Distinct] is set on the builder rather than on an item.
func probe088ProjectionQuery(cols ...aql.SelectField) (aql.Query, error) {
	return aql.Select(cols...).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Class("COMPOSITION", "c")).
		Build()
}

// probe088Refusals are the build-time refusals REQ-117, REQ-118 and REQ-163
// require. Each MUST return an error wrapping [aql.ErrInvalidQuery] — never a
// silently combined, ungrammatical, or question-changing emission.
var probe088Refusals = []struct {
	name  string
	build func() (aql.Query, error)
}{
	{
		name:  "in_text_and_envelope_limit",
		build: func() (aql.Query, error) { return inlinePagingQuery().LimitInline(50).Limit(20).Build() },
	},
	{
		name:  "in_text_limit_and_envelope_offset",
		build: func() (aql.Query, error) { return inlinePagingQuery().LimitInline(50).Offset(10).Build() },
	},
	{
		name:  "in_text_offset_without_limit",
		build: func() (aql.Query, error) { return inlinePagingQuery().OffsetInline(100).Build() },
	},
	{
		// REQ-118: openEHR QUERY Release-1.1.0 §4.4.3 — "It is not allowed to
		// use TOP while also using LIMIT clause in the same query."
		name:  "top_with_in_text_limit",
		build: func() (aql.Query, error) { return inlinePagingQuery().Top(5).LimitInline(50).Build() },
	},
	{
		// REQ-118: a TOP is an in-text row bound, so it is exclusive with the
		// request envelope's bound too — two bounds on one query.
		name:  "top_with_envelope_limit",
		build: func() (aql.Query, error) { return inlinePagingQuery().Top(5).Limit(20).Build() },
	},
	{
		// REQ-118: the `top` production admits no sign.
		name:  "negative_top_count",
		build: func() (aql.Query, error) { return inlinePagingQuery().Top(-1).Build() },
	},
	{
		// REQ-163: the emitted SELECT must read back as the projection the
		// builder recorded. `Col("c/a, c/b")` re-parses as TWO items, so the
		// query asks for a projection the caller never wrote — valid AQL,
		// invisible to every golden and round-trip check downstream.
		name: "col_splits_the_projection",
		build: func() (aql.Query, error) {
			return probe088ProjectionQuery(aql.Col("c/a, c/b"))
		},
	},
	{
		// REQ-163: the same rule on the clause-level flags. `DISTINCT` is
		// consumed into the CLAUSE (`SELECT DISTINCT? top? selectExpr …`), not
		// into the item, so this text asks for distinct rows the builder never
		// recorded. A `Col` that re-parses as one flag-free item — a function
		// call, an aliased path — stays tolerated as legacy.
		name: "col_smuggles_a_clause_flag",
		build: func() (aql.Query, error) {
			return probe088ProjectionQuery(aql.Col("DISTINCT c/uid/value"))
		},
	},
	{
		// REQ-163: the third mode of the same rule, and the one the wire
		// assertion names last. A clause keyword at the top level ENDS the
		// projection, so the parser reads a FROM the builder never wrote and
		// the query it runs is not the query that was built.
		name: "col_spills_into_another_clause",
		build: func() (aql.Query, error) {
			return probe088ProjectionQuery(aql.Col("c/uid/value FROM EHR e2"))
		},
	},
	{
		// REQ-163: one carrier per grammar position. A comparison on a VERSION
		// node is legal AQL and reachable — through the VERSION bracket's own
		// carrier, `Version(alias, VersionCompare(…))`. Routing a class-position
		// call there silently would change the caller's accept set with no
		// diagnostic, so the class carrier refuses and names the constructor.
		name: "standing_predicate_on_a_version_node",
		build: func() (aql.Query, error) {
			return probe088StandingQuery(aql.Class("VERSION", "v").
				Predicated("commit_audit/time_committed/value", aql.OpGt, aql.Param("since")))
		},
	},
}

// Probe088Constructs returns the construct names PROBE-088 asserts, in
// assertion order, so a caller can read the matching
// openehr/aql/testdata/wire/<name>.aql goldens without restating the list.
func Probe088Constructs() []string {
	out := make([]string, 0, len(probe088Constructs))
	for _, c := range probe088Constructs {
		out = append(out, c.name)
	}
	return out
}

// Probe088BuilderContainmentAndPaging asserts the canonical-string stability
// of the builder constructs REQ-117 adds, plus the REQ-118 `SELECT TOP` clause
// and REQ-163's three write-side carriers (REQ-055's PROBE-020 property
// extended, PROBE-088):
//
//   - each construct in [Probe088Constructs] emits its committed golden
//     byte-for-byte;
//   - the struct-builder and the verb-functions agree on a containment-algebra
//     query, as they must for the pre-REQ-117 surface;
//   - probe020Golden — the committed PROBE-020 golden — still equals
//     [Probe020CanonicalQuery], and the unchanged builder path still
//     reproduces it: REQ-117 and REQ-163 are both semver-minor, so a program
//     using none of the new API MUST emit the same bytes as before;
//   - requesting both paging channels, or an in-text OFFSET without a LIMIT,
//     is a build-time error rather than a silent emission — and so is a
//     projection whose emitted SELECT does not read back as what the builder
//     recorded, or a standing predicate on a node whose bracket is the other
//     grammar production (REQ-163).
//
// goldens maps construct name to committed canonical form; the caller reads
// the files (probes take no filesystem dependency). Sandbox-only: no
// transport, no network (REQ-013 building block).
func Probe088BuilderContainmentAndPaging(goldens map[string]string, probe020Golden string) (Result, error) {
	r := Result{Probe: "PROBE-088"}
	if len(goldens) == 0 {
		return r, errors.New("PROBE-088: goldens required, one per Probe088Constructs entry")
	}
	if strings.TrimSpace(probe020Golden) == "" {
		return r, errors.New("PROBE-088: the PROBE-020 golden is required")
	}

	var failures []string
	fail := func(format string, args ...any) {
		failures = append(failures, fmt.Sprintf(format, args...))
	}

	// The pinned constructs against their goldens.
	for _, c := range probe088Constructs {
		golden, ok := goldens[c.name]
		if !ok {
			fail("%s: no golden supplied", c.name)
			continue
		}
		q, err := c.build()
		if err != nil {
			fail("%s: build: %v", c.name, err)
			continue
		}
		if q.String() != golden {
			fail("%s: output does not match golden:\n  built: %q\n golden: %q", c.name, q.String(), golden)
		}
	}
	for name := range goldens {
		if !slices.Contains(Probe088Constructs(), name) {
			fail("%s: golden supplied for an unknown construct", name)
		}
	}

	// Builder-style parity on a new construct (the PROBE-020 property).
	if msg := probe088StyleParity(); msg != "" {
		fail("style parity: %s", msg)
	}

	// The pre-REQ-117 canonical form is untouched — from both directions.
	if probe020Golden != Probe020CanonicalQuery {
		fail("PROBE-020 golden drifted from the pinned canonical form:\n golden: %q\n pinned: %q",
			probe020Golden, Probe020CanonicalQuery)
	}
	switch ref, err := probe088ReferenceQuery(); {
	case err != nil:
		fail("PROBE-020 reference query: build: %v", err)
	case ref.String() != Probe020CanonicalQuery:
		fail("PROBE-020 reference query no longer emits the pinned canonical form:\n  built: %q\n pinned: %q",
			ref.String(), Probe020CanonicalQuery)
	}

	// The refusals: neither channel-combining nor an ungrammatical clause
	// order may reach the wire.
	for _, rc := range probe088Refusals {
		q, err := rc.build()
		if !errors.Is(err, aql.ErrInvalidQuery) {
			fail("%s: err = %v (query %q), want aql.ErrInvalidQuery", rc.name, err, q.String())
		}
	}

	if len(failures) > 0 {
		r.Status = "fail"
		r.Detail = strings.Join(failures, "; ")
		return r, nil
	}
	r.Status = "pass"
	return r, nil
}

// probe088ReferenceQuery rebuilds the PROBE-020 reference query through the
// pre-REQ-117 API surface only (Contains with a single Archetype, envelope
// paging untouched).
func probe088ReferenceQuery() (aql.Query, error) {
	return aql.NewBuilder().
		Select(aql.Col("o")).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Archetype("COMPOSITION", "c", "")).
		Contains(aql.Archetype("OBSERVATION", "o", "openEHR-EHR-OBSERVATION.body_temperature.v2")).
		Build()
}

// probe088StyleParity builds one containment-algebra query in both builder
// styles and reports a divergence, or "" when they agree.
func probe088StyleParity() string {
	expr := func() aql.Containment {
		return aql.Class("COMPOSITION", "c").Contains(aql.ContainsOr(
			aql.Archetype("OBSERVATION", "o", "openEHR-EHR-OBSERVATION.body_temperature.v2"),
			aql.Class("EVALUATION", "ev"),
		))
	}
	structQ, err := aql.NewBuilder().Select(aql.Col("o")).
		FromEHR("e", aql.Param("ehr_id")).Contains(expr()).LimitInline(10).Build()
	if err != nil {
		return fmt.Sprintf("struct-builder: %v", err)
	}
	verbQ, err := aql.Select(aql.Col("o")).
		FromEHR("e", aql.Param("ehr_id")).Contains(expr()).LimitInline(10).Build()
	if err != nil {
		return fmt.Sprintf("verb-functions: %v", err)
	}
	if structQ.String() != verbQ.String() {
		return fmt.Sprintf("struct vs verb diverge:\n struct: %q\n   verb: %q", structQ.String(), verbQ.String())
	}
	return ""
}
