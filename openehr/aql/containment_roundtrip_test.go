package aql_test

// containment_roundtrip_test.go is the equivalence pin between the two
// canonicalisers of REQ-117: the builder's (openehr/aql, this package's
// subject) and the parser's (openehr/aql/parse). openehr/aql cannot import
// openehr/aql/parse — the dependency runs the other way — so the paren and
// precedence rules are stated twice; this test proves the two statements
// agree by construction:
//
//	build → parse.Parse → QueryErr() == nil → Emit() == build
//
// A divergence in either direction fails here: an emitter drift shows up as a
// byte mismatch, an out-of-catalogue shape as a non-nil QueryErr
// (aql.ErrIncompleteAST), and an ungrammatical emission as a syntax error.
// The external test package (aql_test) is what makes the parse import legal.
// REQ-117 · PROBE-088

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// builderRoundTripCases are the builder programs whose output must survive a
// parse → emit round trip byte-for-byte. Every construct the REQ-117 builder
// API adds appears here.
func builderRoundTripCases() []struct {
	name  string
	build func() (aql.Query, error)
} {
	sel := func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("e/ehr_id/value")).From("EHR", "e")
	}
	return []struct {
		name  string
		build func() (aql.Query, error)
	}{
		{"leaf_class", func() (aql.Query, error) {
			return sel().Contains(aql.Class("COMPOSITION", "c")).Build()
		}},
		{"leaf_archetype", func() (aql.Query, error) {
			return sel().Contains(aql.Archetype("OBSERVATION", "o", "openEHR-EHR-OBSERVATION.body_temperature.v2")).Build()
		}},
		{"chain", func() (aql.Query, error) {
			return sel().Contains(aql.Class("COMPOSITION", "c").Contains(aql.Class("OBSERVATION", "o"))).Build()
		}},
		{"chained_calls", func() (aql.Query, error) {
			return sel().Contains(aql.Class("COMPOSITION", "c")).Contains(aql.Class("OBSERVATION", "o")).Build()
		}},
		{"not_contains_child", func() (aql.Query, error) {
			return sel().Contains(aql.Class("COMPOSITION", "c").NotContains(aql.Class("SECTION", "s"))).Build()
		}},
		{"not_contains_root", func() (aql.Query, error) {
			return sel().NotContains(aql.Class("COMPOSITION", "c")).Build()
		}},
		{"sibling_and", func() (aql.Query, error) {
			return sel().Contains(aql.ContainsAnd(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev"))).Build()
		}},
		{"sibling_or", func() (aql.Query, error) {
			return sel().Contains(aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev"))).Build()
		}},
		{"or_three_operands", func() (aql.Query, error) {
			return sel().Contains(aql.ContainsOr(
				aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev"), aql.Class("INSTRUCTION", "i"),
			)).Build()
		}},
		{"or_under_and_grouped", func() (aql.Query, error) {
			return sel().Contains(aql.ContainsAnd(
				aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev")),
				aql.Class("INSTRUCTION", "i"),
			)).Build()
		}},
		{"and_under_or_bare", func() (aql.Query, error) {
			return sel().Contains(aql.ContainsOr(
				aql.Class("OBSERVATION", "o"),
				aql.ContainsAnd(aql.Class("EVALUATION", "ev"), aql.Class("INSTRUCTION", "i")),
			)).Build()
		}},
		{"chain_as_junction_operand", func() (aql.Query, error) {
			return sel().Contains(aql.ContainsOr(
				aql.Class("COMPOSITION", "c").Contains(aql.Class("OBSERVATION", "o")),
				aql.Class("SECTION", "s"),
			)).Build()
		}},
		{"negated_chain_as_junction_operand", func() (aql.Query, error) {
			return sel().Contains(aql.ContainsOr(
				aql.Class("COMPOSITION", "c").NotContains(aql.Class("SECTION", "s")),
				aql.ContainsAnd(aql.Class("EVALUATION", "ev"), aql.Class("INSTRUCTION", "i")),
			)).Build()
		}},
		{"junction_below_chain", func() (aql.Query, error) {
			return sel().Contains(aql.Class("COMPOSITION", "c").Contains(
				aql.ContainsAnd(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev")),
			)).Build()
		}},
		// A junction may only END a containment chain (the grammar cannot
		// chain CONTAINS after a parenthesised group), so the three legal
		// neighbours of that boundary are pinned here: the junction closing a
		// longer chain, closing the Builder-level chain, and the rewrite the
		// Build refusal points the caller at — the tail written INSIDE the
		// junction's operands. The refusals themselves are in
		// containment_test.go; this corpus is the "every accepted build
		// re-parses" half of the same invariant.
		{"junction_ends_longer_chain", func() (aql.Query, error) {
			return sel().Contains(aql.Class("COMPOSITION", "c").
				Contains(aql.Class("SECTION", "s")).
				Contains(aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev")))).Build()
		}},
		{"junction_ends_builder_chain", func() (aql.Query, error) {
			return sel().Contains(aql.Class("COMPOSITION", "c")).
				Contains(aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev"))).Build()
		}},
		{"tail_inside_junction_operands", func() (aql.Query, error) {
			return sel().Contains(aql.Class("COMPOSITION", "c").Contains(aql.ContainsOr(
				aql.Class("OBSERVATION", "o").NotContains(aql.Class("ACTION", "a")),
				aql.Class("EVALUATION", "ev"),
			))).Build()
		}},
		{"negated_junction_at_root", func() (aql.Query, error) {
			return sel().NotContains(aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev"))).Build()
		}},
		{"nested_junctions_both_ways", func() (aql.Query, error) {
			return sel().Contains(aql.ContainsOr(
				aql.ContainsAnd(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev")),
				aql.ContainsAnd(aql.Class("INSTRUCTION", "i"), aql.Class("ACTION", "ac")),
			)).Build()
		}},
		{"archetype_predicates_in_junction", func() (aql.Query, error) {
			return sel().Contains(
				aql.Archetype("COMPOSITION", "c", "openEHR-EHR-COMPOSITION.report.v1").Contains(
					aql.ContainsOr(
						aql.Archetype("OBSERVATION", "o", "openEHR-EHR-OBSERVATION.body_temperature.v2"),
						aql.Class("EVALUATION", "ev"),
					),
				),
			).Build()
		}},
		{"inline_limit", func() (aql.Query, error) {
			return sel().LimitInline(50).Build()
		}},
		{"inline_limit_offset", func() (aql.Query, error) {
			return sel().LimitInline(50).OffsetInline(100).Build()
		}},
		{"inline_limit_offset_param", func() (aql.Query, error) {
			return sel().LimitInlineParam("rows").OffsetInlineParam("skip").Build()
		}},
		{"inline_limit_zero", func() (aql.Query, error) {
			return sel().LimitInline(0).Build()
		}},
		{"inline_paging_after_order_by", func() (aql.Query, error) {
			return sel().OrderBy("e/time_created/value", aql.Descending).
				LimitInline(10).OffsetInline(20).Build()
		}},
		{"containment_with_where_and_order_by", func() (aql.Query, error) {
			return aql.Select(aql.Col("o")).
				FromEHR("e", aql.Param("ehr_id")).
				Contains(aql.Class("COMPOSITION", "c").Contains(
					aql.ContainsAnd(
						aql.Archetype("OBSERVATION", "o", "openEHR-EHR-OBSERVATION.body_temperature.v2"),
						aql.Class("EVALUATION", "ev"),
					),
				)).
				Where(aql.Gt("o/data/events/data/items/value/magnitude", aql.Real(37.5))).
				OrderBy("o/name/value", aql.Descending).
				Build()
		}},
		{"all_clauses_with_inline_paging", func() (aql.Query, error) {
			return aql.Select(aql.Col("o"), aql.Col("ev")).
				FromEHR("e", aql.Param("ehr_id")).
				NotContains(aql.ContainsOr(
					aql.Class("COMPOSITION", "c").Contains(aql.Class("OBSERVATION", "o")),
					aql.Class("EVALUATION", "ev"),
				)).
				Where(aql.Eq("o/name/value", aql.String("Temperature"))).
				OrderBy("o/name/value", aql.Ascending).
				LimitInline(25).OffsetInlineParam("skip").
				Build()
		}},
	}
}

// TestBuilderOutputRoundTripsThroughParse is the REQ-117 definition-of-done
// tie-in: every builder output parses with an EMPTY catalogue-gap diagnostic
// (no aql.ErrIncompleteAST) and re-emits to the same bytes.
// PROBE-088
func TestBuilderOutputRoundTripsThroughParse(t *testing.T) {
	for _, tc := range builderRoundTripCases() {
		t.Run(tc.name, func(t *testing.T) {
			q, err := tc.build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			built := q.String()

			doc, err := parse.Parse(built)
			if err != nil {
				t.Fatalf("Parse(%q): %v", built, err)
			}
			if qerr := doc.QueryErr(); qerr != nil {
				t.Fatalf("QueryErr(%q) = %v (incomplete AST: %t), want nil",
					built, qerr, errors.Is(qerr, aql.ErrIncompleteAST))
			}
			emitted, err := doc.Query().Emit()
			if err != nil {
				t.Fatalf("Emit(%q): %v", built, err)
			}
			if emitted != built {
				t.Fatalf("builder and parse canonicalisers diverge:\n builder: %s\n   parse: %s", built, emitted)
			}
		})
	}
}
