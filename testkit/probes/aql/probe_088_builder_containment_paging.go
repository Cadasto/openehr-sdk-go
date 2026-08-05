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

// probe088Constructs are the REQ-117 builder constructs PROBE-088 pins, in
// assertion order. Each name is the base name of its golden file (`<name>.aql`)
// under openehr/aql/testdata/wire/; the probe owns the builder program so a
// downstream SDK implementing REQ-117 reproduces the same canonical string
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
}

// inlinePagingQuery is the shared body of the paging constructs: the most
// recent compositions of one EHR, ordered so a page is well-defined.
func inlinePagingQuery() *aql.Builder {
	return aql.Select(aql.Col("c/uid/value")).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Class("COMPOSITION", "c")).
		OrderBy("c/context/start_time/value", aql.Descending)
}

// probe088Refusals are the build-time refusals REQ-117 requires. Each MUST
// return an error wrapping [aql.ErrInvalidQuery] — never a silently combined
// or ungrammatical emission.
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
// (REQ-055's PROBE-020 property extended, PROBE-088):
//
//   - each construct in [Probe088Constructs] emits its committed golden
//     byte-for-byte;
//   - the struct-builder and the verb-functions agree on a containment-algebra
//     query, as they must for the pre-REQ-117 surface;
//   - probe020Golden — the committed PROBE-020 golden — still equals
//     [Probe020CanonicalQuery], and the unchanged builder path still
//     reproduces it: REQ-117 is semver-minor, so a program using none of the
//     new API MUST emit the same bytes as before;
//   - requesting both paging channels, or an in-text OFFSET without a LIMIT,
//     is a build-time error rather than a silent emission.
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

	// The REQ-117 constructs against their goldens.
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
