package aql_test

// containment_test.go pins the REQ-117 builder containment algebra: the
// negated (`NOT CONTAINS`) connector, sibling AND / OR junctions, and the
// parenthesisation rule (NOT binds tightest, then AND, then OR; parentheses
// only where the grouping is load-bearing). The canonical strings asserted
// here MUST equal what openehr/aql/parse emits for the same tree — pinned
// mechanically by containment_roundtrip_test.go.
// PROBE-088

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
)

// TestContainmentEmission tables the canonical form of every containment
// shape the algebra can build. Each case emits from the FROM root `EHR e`
// so only the containment text varies.
// REQ-117
func TestContainmentEmission(t *testing.T) {
	tests := []struct {
		name string
		expr aql.Containment
		not  bool // attach via Builder.NotContains instead of Contains
		want string
	}{
		{
			name: "leaf class",
			expr: aql.Class("COMPOSITION", "c"),
			want: "CONTAINS COMPOSITION c",
		},
		{
			name: "leaf archetype unchanged",
			expr: aql.Archetype("OBSERVATION", "o", "openEHR-EHR-OBSERVATION.body_temperature.v2"),
			want: "CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2]",
		},
		{
			name: "chain",
			expr: aql.Class("COMPOSITION", "c").Contains(aql.Class("OBSERVATION", "o")),
			want: "CONTAINS COMPOSITION c CONTAINS OBSERVATION o",
		},
		{
			name: "negated child",
			expr: aql.Class("COMPOSITION", "c").NotContains(aql.Class("SECTION", "s")),
			want: "CONTAINS COMPOSITION c NOT CONTAINS SECTION s",
		},
		{
			name: "negated at the root connector",
			expr: aql.Class("SECTION", "s"),
			not:  true,
			want: "NOT CONTAINS SECTION s",
		},
		{
			name: "sibling and",
			expr: aql.ContainsAnd(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev")),
			want: "CONTAINS (OBSERVATION o AND EVALUATION ev)",
		},
		{
			name: "sibling or",
			expr: aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev")),
			want: "CONTAINS (OBSERVATION o OR EVALUATION ev)",
		},
		{
			name: "three-operand or is flat",
			expr: aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev"), aql.Class("INSTRUCTION", "i")),
			want: "CONTAINS (OBSERVATION o OR EVALUATION ev OR INSTRUCTION i)",
		},
		{
			// OR inside AND: AND binds tighter, so the grouping is
			// load-bearing and MUST be parenthesised.
			name: "or under and is grouped",
			expr: aql.ContainsAnd(
				aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev")),
				aql.Class("INSTRUCTION", "i"),
			),
			want: "CONTAINS ((OBSERVATION o OR EVALUATION ev) AND INSTRUCTION i)",
		},
		{
			// AND inside OR: precedence already agrees, so no parentheses.
			name: "and under or is bare",
			expr: aql.ContainsOr(
				aql.Class("OBSERVATION", "o"),
				aql.ContainsAnd(aql.Class("EVALUATION", "ev"), aql.Class("INSTRUCTION", "i")),
			),
			want: "CONTAINS (OBSERVATION o OR EVALUATION ev AND INSTRUCTION i)",
		},
		{
			// A CONTAINS chain used as a junction operand MUST be
			// parenthesised: the grammar's `CONTAINS containsExpr` right
			// operand is greedy, so the following AND / OR operand would
			// otherwise re-parse INTO the chain.
			name: "chain operand is grouped",
			expr: aql.ContainsOr(
				aql.Class("COMPOSITION", "c").Contains(aql.Class("OBSERVATION", "o")),
				aql.Class("EHR", "e2"),
			),
			want: "CONTAINS ((COMPOSITION c CONTAINS OBSERVATION o) OR EHR e2)",
		},
		{
			name: "negated chain operand is grouped",
			expr: aql.ContainsOr(
				aql.Class("COMPOSITION", "c").NotContains(aql.Class("SECTION", "s")),
				aql.ContainsAnd(aql.Class("EVALUATION", "ev"), aql.Class("INSTRUCTION", "i")),
			),
			want: "CONTAINS ((COMPOSITION c NOT CONTAINS SECTION s) OR EVALUATION ev AND INSTRUCTION i)",
		},
		{
			name: "junction below a chain",
			expr: aql.Class("COMPOSITION", "c").Contains(
				aql.ContainsAnd(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev")),
			),
			want: "CONTAINS COMPOSITION c CONTAINS (OBSERVATION o AND EVALUATION ev)",
		},
		{
			name: "negated junction below the root connector",
			expr: aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev")),
			not:  true,
			want: "NOT CONTAINS (OBSERVATION o OR EVALUATION ev)",
		},
		{
			name: "single-operand junction collapses",
			expr: aql.ContainsAnd(aql.Class("COMPOSITION", "c")),
			want: "CONTAINS COMPOSITION c",
		},
		{
			name: "archetype predicates survive nesting",
			expr: aql.Archetype("COMPOSITION", "c", "openEHR-EHR-COMPOSITION.report.v1").Contains(
				aql.ContainsOr(
					aql.Archetype("OBSERVATION", "o", "openEHR-EHR-OBSERVATION.body_temperature.v2"),
					aql.Class("EVALUATION", "ev"),
				),
			),
			want: "CONTAINS COMPOSITION c[openEHR-EHR-COMPOSITION.report.v1] " +
				"CONTAINS (OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2] OR EVALUATION ev)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := aql.NewBuilder().Select(aql.Col("x")).From("EHR", "e")
			if tc.not {
				b = b.NotContains(tc.expr)
			} else {
				b = b.Contains(tc.expr)
			}
			q, err := b.Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			want := "SELECT x FROM EHR e " + tc.want
			if q.String() != want {
				t.Fatalf("containment emission mismatch:\n got: %q\nwant: %q", q.String(), want)
			}
		})
	}
}

// TestContainmentValuesAreImmutable locks the construct-then-finalise
// contract: the combinators return a NEW Containment, so a shared operand
// can be reused across expressions without one derivation clobbering
// another's children (a slice-aliasing hazard).
// REQ-117
func TestContainmentValuesAreImmutable(t *testing.T) {
	base := aql.Class("COMPOSITION", "c")
	first := base.Contains(aql.Class("OBSERVATION", "o"))
	second := base.Contains(aql.Class("EVALUATION", "ev"))

	emit := func(expr aql.Containment) string {
		q, err := aql.NewBuilder().Select(aql.Col("x")).From("EHR", "e").Contains(expr).Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		return q.String()
	}
	if got, want := emit(first), "SELECT x FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o"; got != want {
		t.Errorf("first derivation mutated:\n got: %q\nwant: %q", got, want)
	}
	if got, want := emit(second), "SELECT x FROM EHR e CONTAINS COMPOSITION c CONTAINS EVALUATION ev"; got != want {
		t.Errorf("second derivation mutated:\n got: %q\nwant: %q", got, want)
	}
	if got, want := emit(base), "SELECT x FROM EHR e CONTAINS COMPOSITION c"; got != want {
		t.Errorf("base operand mutated:\n got: %q\nwant: %q", got, want)
	}
}

// TestContainmentChainedCalls verifies the flat Contains channel still
// chains across repeated calls and composes with an expression argument
// (the pre-REQ-117 behaviour, now on the widened signature).
// REQ-117
func TestContainmentChainedCalls(t *testing.T) {
	q, err := aql.NewBuilder().Select(aql.Col("x")).From("EHR", "e").
		Contains(aql.Class("COMPOSITION", "c")).
		Contains(aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev"))).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	const want = "SELECT x FROM EHR e CONTAINS COMPOSITION c CONTAINS (OBSERVATION o OR EVALUATION ev)"
	if q.String() != want {
		t.Fatalf("chained Contains mismatch:\n got: %q\nwant: %q", q.String(), want)
	}
}

// TestContainmentBuildRefusals locks the fail-loud contract: every
// structurally unusable containment tree errors at Build rather than
// emitting invalid AQL or silently dropping an operand.
// REQ-117
func TestContainmentBuildRefusals(t *testing.T) {
	tests := map[string]aql.Containment{
		"empty junction":              aql.ContainsAnd(),
		"zero-value operand":          aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Containment{}),
		"operand missing alias":       aql.ContainsAnd(aql.Class("OBSERVATION", ""), aql.Class("EVALUATION", "ev")),
		"operand missing rm type":     aql.ContainsAnd(aql.Class("", "o"), aql.Class("EVALUATION", "ev")),
		"nested child missing alias":  aql.Class("COMPOSITION", "c").Contains(aql.Class("OBSERVATION", "")),
		"duplicate alias in junction": aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "o")),
		"duplicate alias in chain":    aql.Class("COMPOSITION", "c").Contains(aql.Class("OBSERVATION", "c")),
		"alias collides with root":    aql.ContainsAnd(aql.Class("OBSERVATION", "e"), aql.Class("EVALUATION", "ev")),
	}
	for name, expr := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := aql.NewBuilder().Select(aql.Col("x")).From("EHR", "e").Contains(expr).Build()
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Fatalf("err = %v, want ErrInvalidQuery", err)
			}
		})
	}
}

// TestContainmentJunctionMustTerminateChain locks the grammar limit REQ-117
// works within: `containsExpr` admits a parenthesised group only as a WHOLE
// alternative (`'(' containsExpr ')'`), which no `CONTAINS` may follow. A
// junction therefore has to END the containment chain it sits in; a junction
// with a further term after it in the same chain is inexpressible and MUST be
// refused at Build rather than emitted as AQL the parser rejects
// (`… CONTAINS (OBSERVATION o OR EVALUATION ev) NOT CONTAINS ACTION a`).
// Re-shaping the tree instead — distributing the tail under the junction's
// operands — would silently change what the query means, so the builder fails
// closed and the caller writes the nesting they meant.
// REQ-117
func TestContainmentJunctionMustTerminateChain(t *testing.T) {
	junction := func() aql.Containment {
		return aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev"))
	}
	sel := func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("x")).From("EHR", "e")
	}

	refused := map[string]func() (aql.Query, error){
		// The chain continues with a NOT CONTAINS connector.
		"junction then negated term in one expression": func() (aql.Query, error) {
			return sel().Contains(aql.Class("COMPOSITION", "c").
				Contains(junction()).
				NotContains(aql.Class("ACTION", "a"))).Build()
		},
		// … and with a plain CONTAINS connector.
		"junction then plain term in one expression": func() (aql.Query, error) {
			return sel().Contains(aql.Class("COMPOSITION", "c").
				Contains(junction()).
				Contains(aql.Class("SECTION", "s"))).Build()
		},
		// Repeated Builder.Contains calls are one chain, so the rule spans them.
		"junction then term across builder calls": func() (aql.Query, error) {
			return sel().Contains(junction()).Contains(aql.Class("ACTION", "a")).Build()
		},
		"junction then negated term across builder calls": func() (aql.Query, error) {
			return sel().Contains(junction()).NotContains(aql.Class("ACTION", "a")).Build()
		},
		// The offending chain is itself an operand of an outer junction.
		"junction mid-chain inside an and operand": func() (aql.Query, error) {
			return sel().Contains(aql.ContainsAnd(
				aql.Class("COMPOSITION", "c").Contains(junction()).Contains(aql.Class("SECTION", "s")),
				aql.Class("ACTION", "a"),
			)).Build()
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

	// Positive controls: a junction that ENDS its chain stays buildable, in
	// every position the refusals cover, and so does the rewrite the error
	// message points the caller at (the tail written inside the operands).
	// containment_roundtrip_test.go additionally re-parses these.
	accepted := map[string]func() (aql.Query, error){
		"junction ends a longer chain": func() (aql.Query, error) {
			return sel().Contains(aql.Class("COMPOSITION", "c").
				Contains(aql.Class("SECTION", "s")).
				Contains(junction())).Build()
		},
		"junction ends the builder chain": func() (aql.Query, error) {
			return sel().Contains(aql.Class("COMPOSITION", "c")).Contains(junction()).Build()
		},
		"junction operand of a junction is unordered": func() (aql.Query, error) {
			return sel().Contains(aql.ContainsOr(
				aql.ContainsAnd(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev")),
				aql.ContainsAnd(aql.Class("INSTRUCTION", "i"), aql.Class("ACTION", "a")),
			)).Build()
		},
		"tail written inside the junction operands": func() (aql.Query, error) {
			return sel().Contains(aql.Class("COMPOSITION", "c").Contains(aql.ContainsOr(
				aql.Class("OBSERVATION", "o").NotContains(aql.Class("ACTION", "a")),
				aql.Class("EVALUATION", "ev"),
			))).Build()
		},
	}
	for name, build := range accepted {
		t.Run(name, func(t *testing.T) {
			if _, err := build(); err != nil {
				t.Fatalf("Build: %v", err)
			}
		})
	}
}

// TestContainmentNoStrayKeywords guards the whitespace rule of the
// canonical form: exactly one space around every keyword, no doubled
// spaces, no space inside the parentheses.
// REQ-117
func TestContainmentNoStrayKeywords(t *testing.T) {
	q, err := aql.NewBuilder().Select(aql.Col("x")).From("EHR", "e").
		Contains(aql.ContainsAnd(
			aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev")),
			aql.Class("COMPOSITION", "c").NotContains(aql.Class("SECTION", "s")),
		)).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := q.String()
	for _, bad := range []string{"  ", "( ", " )"} {
		if strings.Contains(got, bad) {
			t.Errorf("emission contains %q: %s", bad, got)
		}
	}
}
