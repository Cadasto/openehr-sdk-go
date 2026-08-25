package aql_test

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
)

// This file is the read/write parity gate of REQ-162 § Contract:
//
//	for a query expressible through the builder, the verification's code
//	multiset MUST equal the containment-check subset of lint.LintString over
//	the emitted text — one rule engine, two adapters, no drift.
//
// It lives in openehr/aql (package aql_test) rather than in openehr/aql/lint
// because the import arrow runs lint → aql: package aql itself cannot import
// lint, but an EXTERNAL test package is compiled separately and may import both
// with no cycle. openehr/aql already uses external test packages throughout
// (aql_test.go, builder_test.go, containment_roundtrip_test.go), so this is the
// house pattern, and it keeps the whole change inside openehr/aql.
//
// It is also the reason the shared rule engine sits in
// openehr/aql/internal/semcheck: reachable from both adapters without either
// importing the other.

// containmentCodes is the five-code subset REQ-162 § Contract scopes parity to.
// The three REQ-161 portability advisories are read-side-only (PROBE-097
// § parity), so they are filtered out of the lint side rather than expected from
// the write side.
var containmentCodes = []string{
	codeImpossible,
	codeNotContainable,
	codeArchetype,
	codeUnknownClass,
	codeByReference,
}

// lintContainmentCodes is the sorted containment-code multiset lint reports for
// q — every other lint code (syntax, shape, param, template, portability) is
// outside the parity scope and dropped.
func lintContainmentCodes(q string, rel *contain.Relation) []string {
	var out []string
	for _, i := range lint.LintString(q, &lint.Options{Relation: rel}).Issues {
		if slices.Contains(containmentCodes, i.Code) {
			out = append(out, i.Code)
		}
	}
	slices.Sort(out)
	return out
}

// parityCases are the queries both adapters are held to. Every containment shape
// the builder can express is represented: a plain chain, a flattened multi-entry
// chain, each of the five defects, an OR junction, an AND junction, mixed AND/OR
// nesting, NOT CONTAINS (plain, nested, and over a defect), an archetype
// predicate (conforming and mismatched), a $param archetype predicate, and the
// EHR/VERSION tier.
var parityCases = []struct {
	name  string
	build func() *aql.Builder
}{
	{"clean pair", func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("o")).From("COMPOSITION", "c").
			Contains(aql.Class("OBSERVATION", "o"))
	}},
	{"impossible pair", func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("o")).From("OBSERVATION", "o").
			Contains(aql.Class("COMPOSITION", "c"))
	}},
	{"non-containable operand", func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
			Contains(aql.Class("DV_TEXT", "t"))
	}},
	{"unknown class", func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
			Contains(aql.Class("FOO_BAR", "f"))
	}},
	{
		// The role-assignment row (semcheck.Role.Next): a root position raises
		// no aql_contains_not_containable in ANY spelling, and DV_TEXT still
		// suppresses the pair below it. Both sides must answer silence — a write
		// side that labelled its FROM root "contained" would report an Error the
		// read side never reports, which is the parity break the shared engine
		// exists to prevent.
		"non-containable FROM root", func() *aql.Builder {
			return aql.NewBuilder().Select(aql.Col("t")).From("DV_TEXT", "t").
				Contains(aql.Class("ELEMENT", "el"))
		},
	},
	{"unknown FROM root", func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("c")).From("FOO_BAR", "f").
			Contains(aql.Class("COMPOSITION", "c"))
	}},
	{"by-reference hop", func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("c")).From("FOLDER", "f").
			Contains(aql.Class("COMPOSITION", "c"))
	}},
	{"archetype/class mismatch", func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("ev")).From("COMPOSITION", "c").
			Contains(aql.Archetype("EVALUATION", "ev", "openEHR-EHR-OBSERVATION.body_temperature.v2"))
	}},
	{"conforming archetype predicate", func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("o")).From("COMPOSITION", "c").
			Contains(aql.Archetype("OBSERVATION", "o", "openEHR-EHR-OBSERVATION.body_temperature.v2"))
	}},
	{"$param archetype predicate", func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("o")).From("COMPOSITION", "c").
			Contains(aql.Archetype("OBSERVATION", "o", "$arch")).Bind("arch", "openEHR-EHR-OBSERVATION.body_temperature.v2")
	}},
	{"$param archetype on an unknown class", func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("x")).From("COMPOSITION", "c").
			Contains(aql.Archetype("FOO_BAR", "x", "$arch"))
	}},
	{"OR junction, one defective operand", func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
			Contains(aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("DV_TEXT", "t")))
	}},
	{"AND junction under an impossible root", func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("o")).From("OBSERVATION", "o").
			Contains(aql.ContainsAnd(aql.Class("COMPOSITION", "c1"), aql.Class("COMPOSITION", "c2")))
	}},
	{"nested mixed AND/OR junction", func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
			Contains(aql.ContainsAnd(
				aql.Class("OBSERVATION", "o"),
				aql.ContainsOr(aql.Class("EVALUATION", "ev"), aql.Class("DV_TEXT", "t")),
			))
	}},
	{"junction whose operand carries its own chain", func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("c")).FromEHR("e", aql.Param("ehr_id")).
			Contains(aql.Class("COMPOSITION", "c").Contains(aql.ContainsOr(
				aql.Archetype("OBSERVATION", "o", "openEHR-EHR-OBSERVATION.body_temperature.v2").
					NotContains(aql.Class("CLUSTER", "cl")),
				aql.Class("EVALUATION", "ev"),
			)))
	}},
	{"NOT CONTAINS over an impossible pair", func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("o")).From("OBSERVATION", "o").
			NotContains(aql.Class("COMPOSITION", "c"))
	}},
	{"NOT CONTAINS over a non-containable operand", func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
			NotContains(aql.Class("DV_TEXT", "t"))
	}},
	{"NOT CONTAINS nested in a chain", func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
			Contains(aql.Class("OBSERVATION", "o").NotContains(aql.Class("DV_TEXT", "t")))
	}},
	// The next three rows pin the two DISTINCT flattening sites the chain-tail
	// decision governs — see [containVerifier.chain]. They are separate rows on
	// purpose: a class node's children flatten through Containment.emit's child
	// loop, while consecutive Builder.Contains entries flatten through
	// ast.build's entry loop, so one row cannot cover both. If a future change
	// ever parenthesised sibling children, only the sibling-children rows would
	// catch it.
	{"flattened multi-entry chain (ast.build's entry loop)", func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("o")).From("EHR", "e").
			Contains(aql.Class("FOLDER", "f").Contains(aql.Class("COMPOSITION", "c"))).
			Contains(aql.Class("OBSERVATION", "o"))
	}},
	{
		// One class node with THREE children (Containment.emit's child loop).
		// The pairs are EHR→FOLDER, FOLDER→COMPOSITION, COMPOSITION→OBSERVATION
		// and OBSERVATION→DV_TEXT: adjacency walks the flattened chain, so each
		// child follows the PREVIOUS one, not the node they all hang off.
		"class node with three sibling children", func() *aql.Builder {
			return aql.NewBuilder().Select(aql.Col("o")).From("EHR", "e").
				Contains(aql.Class("FOLDER", "f").
					Contains(aql.Class("COMPOSITION", "c")).
					Contains(aql.Class("OBSERVATION", "o")).
					Contains(aql.Class("DV_TEXT", "t")))
		},
	},
	{
		// A junction whose enclosing parent is the flattened TAIL of the chain
		// before it (COMPOSITION c), not the chain's head and not the FROM root.
		"junction enclosed by a flattened chain tail", func() *aql.Builder {
			return aql.NewBuilder().Select(aql.Col("o")).From("EHR", "e").
				Contains(aql.Class("FOLDER", "f")).
				Contains(aql.Class("COMPOSITION", "c")).
				Contains(aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("COMPOSITION", "c2")))
		},
	},
	{"EHR/VERSION tier", func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("c")).From("EHR", "e").
			Contains(aql.Class("VERSION", "v").Contains(aql.Class("COMPOSITION", "c")))
	}},
	{"every defect at once", func() *aql.Builder {
		return aql.NewBuilder().
			Select(aql.Col("o"), aql.Col("c")).
			From("OBSERVATION", "o").
			NotContains(aql.Class("FOO_BAR", "f")).
			Contains(aql.Class("COMPOSITION", "c").Contains(aql.ContainsOr(
				aql.Archetype("EVALUATION", "ev", "openEHR-EHR-OBSERVATION.body_temperature.v2"),
				aql.Class("DV_TEXT", "t"),
			)))
	}},
}

// TestReadWriteParity is the REQ-162 § Contract MUST: build → emit →
// [lint.LintString], and the two code multisets agree over the five containment
// codes.
//
// It is a property, not a spot check: the read side walks a parse tree whose
// SHAPE differs from the builder's (the extractor nests one level per CONTAINS
// and pre-flattens same-operator junctions; the builder flattens chains at
// emission instead), so the two walks reach the same operands by different
// routes. If a row here disagrees, the divergence is the defect — one adapter is
// asking the wrong pair or assigning the wrong role.
func TestReadWriteParity(t *testing.T) {
	t.Parallel()
	for _, tc := range parityCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := tc.build()
			q, err := b.Build()
			if err != nil {
				t.Fatalf("Build() = %v — every parity case must be buildable", err)
			}
			write := findingCodes(b.VerifyContainment(nil))
			read := lintContainmentCodes(q.Q, nil)
			if !slices.Equal(write, read) {
				t.Errorf("code multisets diverge for %q:\n  write side (VerifyContainment) = %v\n  read side  (LintString)         = %v",
					q.Q, write, read)
			}
		})
	}
}

// TestReadWriteParityUnderOverlay pins that parity survives the relation
// parameter: both adapters take the SAME [*contain.Relation], so an overlay that
// retires a finding must retire it on both sides. A read side that ignored the
// relation, or a write side that resolved the default itself, would show up here
// and nowhere else.
func TestReadWriteParityUnderOverlay(t *testing.T) {
	t.Parallel()
	rel := contain.Default().WithOverlay(
		contain.Edge{From: "OBSERVATION", To: "COMPOSITION"},
		contain.Edge{From: "FOO_BAR", To: "COMPOSITION"},
	)
	for _, tc := range parityCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := tc.build()
			q, err := b.Build()
			if err != nil {
				t.Fatalf("Build() = %v", err)
			}
			write := findingCodes(b.VerifyContainment(rel))
			read := lintContainmentCodes(q.Q, rel)
			if !slices.Equal(write, read) {
				t.Errorf("overlay code multisets diverge for %q:\n  write side = %v\n  read side  = %v", q.Q, write, read)
			}
		})
	}
}

// TestReadWriteParityIsNotVacuous guards the parity test against passing for the
// wrong reason: the table must actually EXERCISE every one of the five codes, or
// an adapter could go silent on one and both sides would agree on nothing.
func TestReadWriteParityIsNotVacuous(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	clean := 0
	for _, tc := range parityCases {
		fs := tc.build().VerifyContainment(nil)
		if len(fs) == 0 {
			clean++
		}
		for _, f := range fs {
			seen[f.Code] = true
		}
	}
	for _, code := range containmentCodes {
		if !seen[code] {
			t.Errorf("no parity case raises %s; the table does not cover it", code)
		}
	}
	if clean == 0 {
		t.Error("no parity case is clean; the table cannot show a false positive")
	}
}
