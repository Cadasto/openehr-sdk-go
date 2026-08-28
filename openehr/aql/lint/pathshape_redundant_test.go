package lint_test

// pathshape_redundant_test.go: REQ-164 § Path-shape checks —
// aql_contains_redundant_step, the group's one code that asks the REQ-160
// relation a question. It is the general case of REQ-161's
// aql_versioned_object_unreferenced, and REQ-164 § No double-reporting has the
// general case yield to the specific; both directions of that yielding carry a
// named row here.
//
// Every guard is mutation-detectable: the mapping from guard to the named test
// that fails when it is removed is recorded in the task report. The silence
// rows carry their own names for the reason REQ-119 § Emission verified after
// emission records — an over-firing linter must not ship green, and a
// firing-only corpus cannot tell. That matters more for this code than for its
// group-mates: it advises DELETING a step, so a false finding does not merely
// annoy, it invites a wrong edit.
//
// These rows are the fifth code's share of PROBE-099 arm (a).

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
)

const redundantCode = "aql_contains_redundant_step"

// witnessRedundant is the firing witness REQ-164 § Path-shape checks names
// outright: `c` is unreferenced and predicate-less, and every EHR -> OBSERVATION
// containment route passes a COMPOSITION, so the step provably does nothing.
//
// The projection is aliased and lands on a single-valued attribute, so the
// query carries no OTHER REQ-164 defect and its whole result stays legible; the
// EHR root supplies the identifiable scope that keeps aql_from_archetype off it.
const witnessRedundant = "SELECT o/name/value AS n " +
	"FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o"

// witnessAvoidable is the same shape over an AVOIDABLE intermediate — REQ-164's
// own silence witness. Dropping `SECTION s` admits observations that sit
// directly in a composition's content, so the step narrows the result and is
// not redundant.
const witnessAvoidable = "SELECT o/name/value AS n " +
	"FROM EHR e CONTAINS SECTION s CONTAINS OBSERVATION o"

// redundantIssues returns the aql_contains_redundant_step findings, in result
// order.
func redundantIssues(r lint.Result) []lint.Issue {
	var out []lint.Issue
	for _, i := range r.Issues {
		if i.Code == redundantCode {
			out = append(out, i)
		}
	}
	return out
}

// silentOnRedundant fails unless the query raised NO finding of this code,
// naming the whole result when it did.
func silentOnRedundant(t *testing.T, q string, opts *lint.Options) {
	t.Helper()
	res := lint.LintString(q, opts)
	if got := redundantIssues(res); len(got) != 0 {
		t.Errorf("got %d %s findings, want none (codes %v, first detail %q, query %q)",
			len(got), redundantCode, codes(res), got[0].Detail, q)
	}
}

// --- Firing rules ------------------------------------------------------------

// TestRedundantStepFiresOnTheSpecWitness pins the whole finding on REQ-164's
// own example: the Span covers the offending class token, Path names it, Detail
// names the pair the proof was asked about, the severity is Warning, and the
// result carries nothing else — the query's only defect is the one step.
func TestRedundantStepFiresOnTheSpecWitness(t *testing.T) {
	t.Parallel()
	res := lint.LintString(witnessRedundant, nil)
	got := redundantIssues(res)
	if len(got) != 1 {
		t.Fatalf("got %d %s findings, want exactly 1: %v", len(got), redundantCode, codes(res))
	}
	if tok := spanText(t, witnessRedundant, got[0].Span); tok != "COMPOSITION" {
		t.Errorf("span covers %q, want the offending class token %q", tok, "COMPOSITION")
	}
	if want := "COMPOSITION c"; got[0].Path != want {
		t.Errorf("Issue.Path = %q, want %q", got[0].Path, want)
	}
	if got[0].Severity != lint.Warning {
		t.Errorf("severity = %v, want warning (REQ-164 § Path-shape checks)", got[0].Severity)
	}
	// Detail names the PAIR the relation was asked about, which is what lets a
	// reader check the claim without re-deriving the containment tree.
	for _, want := range []string{"EHR", "OBSERVATION", "COMPOSITION"} {
		if !strings.Contains(got[0].Detail, want) {
			t.Errorf("Detail %q does not name %q", got[0].Detail, want)
		}
	}
	// The whole result, named in full and in order, so a code SWAP fails here
	// too: this query's only defect is the redundant step.
	if want := []string{redundantCode}; !slices.Equal(codes(res), want) {
		t.Errorf("codes = %v, want %v", codes(res), want)
	}
	if !res.OK() {
		t.Errorf("OK() = false with only this group's finding (%v); the code is Warning", codes(res))
	}
}

// TestRedundantStepFiresOncePerRedundantStep walks a four-operand chain in
// which TWO steps are provably inert: every EHR -> INSTRUCTION route passes a
// COMPOSITION, and every COMPOSITION -> ACTIVITY route passes an INSTRUCTION.
// Each is judged against its OWN parent and child, in document order.
//
// It is also the chain-construction guard: the chain is seeded with the FROM
// root, and a chain that dropped it would shift every parent by one and report
// a different pair here.
func TestRedundantStepFiresOncePerRedundantStep(t *testing.T) {
	t.Parallel()
	const q = "SELECT a/name/value AS n " +
		"FROM EHR e CONTAINS COMPOSITION c CONTAINS INSTRUCTION i CONTAINS ACTIVITY a"
	res := lint.LintString(q, nil)
	got := redundantIssues(res)
	if len(got) != 2 {
		t.Fatalf("got %d %s findings, want exactly 2: %v", len(got), redundantCode, codes(res))
	}
	if paths := []string{got[0].Path, got[1].Path}; !slices.Equal(paths, []string{"COMPOSITION c", "INSTRUCTION i"}) {
		t.Errorf("findings name %v, want [COMPOSITION c INSTRUCTION i] in document order", paths)
	}
	// Each was asked about its own parent/child pair, not the chain's endpoints.
	if !strings.Contains(got[0].Detail, "from EHR to INSTRUCTION") {
		t.Errorf("first Detail %q does not name the EHR -> INSTRUCTION pair", got[0].Detail)
	}
	if !strings.Contains(got[1].Detail, "from COMPOSITION to ACTIVITY") {
		t.Errorf("second Detail %q does not name the COMPOSITION -> ACTIVITY pair", got[1].Detail)
	}
}

// TestRedundantStepFiresOnAnAnonymousOperand — an operand with no alias at all
// (AQL's classExprOperand makes the alias optional) is INCLUDED, not skipped,
// exactly as it is for aql_versioned_object_unreferenced: with no alias no path
// anywhere can reference it, so "roots no identified path" holds
// unconditionally and the redundancy is if anything more certain.
func TestRedundantStepFiresOnAnAnonymousOperand(t *testing.T) {
	t.Parallel()
	const q = "SELECT o/name/value AS n FROM EHR e CONTAINS COMPOSITION CONTAINS OBSERVATION o"
	res := lint.LintString(q, nil)
	got := redundantIssues(res)
	if len(got) != 1 {
		t.Fatalf("got %d %s findings, want exactly 1: %v", len(got), redundantCode, codes(res))
	}
	if want := "COMPOSITION"; got[0].Path != want {
		t.Errorf("Issue.Path = %q, want the bare class token %q", got[0].Path, want)
	}
}

// TestRedundantStepFiresOnAVersionTierStep is the version-tier shape, where the
// step is inert because the version tier is the ONLY route from a container to
// its payload (REQ-160 § Overlay edges). It also records a deliberate
// coexistence: a bare VERSION operand carries aql_version_no_predicate too
// (REQ-161, SPECPR-481). REQ-164 § No double-reporting fixes ONE owner between
// this code and aql_versioned_object_unreferenced only — these two report
// DIFFERENT defects (an unstated version tier, and a step that does nothing),
// and stating the tier silences both, since a version predicate is a
// class-position predicate.
func TestRedundantStepFiresOnAVersionTierStep(t *testing.T) {
	t.Parallel()
	const q = "SELECT c/name/value AS n " +
		"FROM VERSIONED_COMPOSITION vo[uid/value='x'] CONTAINS VERSION v CONTAINS COMPOSITION c"
	res := lint.LintString(q, nil)
	got := redundantIssues(res)
	if len(got) != 1 {
		t.Fatalf("got %d %s findings, want exactly 1: %v", len(got), redundantCode, codes(res))
	}
	if want := "VERSION v"; got[0].Path != want {
		t.Errorf("Issue.Path = %q, want %q", got[0].Path, want)
	}
	if want := []string{codeVersionNoPredicate, redundantCode}; !slices.Equal(codes(res), want) {
		t.Errorf("codes = %v, want %v", codes(res), want)
	}
	// Stating the tier is a class-position predicate, so it retires BOTH.
	stated := strings.Replace(q, "VERSION v ", "VERSION v[ALL_VERSIONS] ", 1)
	if c := codes(lint.LintString(stated, nil)); len(c) != 0 {
		t.Errorf("codes = %v with the version tier stated, want none", c)
	}
}

// --- Silence rules (one named row per near miss) -----------------------------

// TestRedundantStepSilentOnAnAvoidableIntermediate is REQ-164 § Acceptance's
// avoidable near-miss, and the substance of the whole rule: the guidance
// sentence this check comes from ("containment is minimal") would flag this
// query, and would be wrong. Dropping `SECTION s` widens the result.
func TestRedundantStepSilentOnAnAvoidableIntermediate(t *testing.T) {
	t.Parallel()
	silentOnRedundant(t, witnessAvoidable, nil)
	if c := codes(lint.LintString(witnessAvoidable, nil)); len(c) != 0 {
		t.Errorf("codes = %v, want a clean result", c)
	}
}

// TestRedundantStepSilentOnALeaf is REQ-164 § Acceptance's leaf near-miss: an
// unreferenced leaf is an existence filter and does work, so it is never
// redundant however unreferenced and predicate-less it is. Both leaf spellings
// are rows — the leaf of a two-operand FROM, and the leaf of a longer chain.
func TestRedundantStepSilentOnALeaf(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, q string }{
		{
			"leaf of a two-operand FROM",
			"SELECT e/ehr_id/value AS id FROM EHR e CONTAINS COMPOSITION c",
		},
		{
			"leaf of a longer chain",
			"SELECT e/ehr_id/value AS id " +
				"FROM EHR e CONTAINS COMPOSITION c[" + compArch + "] CONTAINS OBSERVATION o",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			silentOnRedundant(t, tc.q, nil)
		})
	}
}

// TestRedundantStepSilentOnTheFromRoot pins the not-the-root condition. The
// root anchors the containment tree; it is not a step, and it has no ancestor
// for a route to be asked about. The rule is the chain walk's lower bound, and
// an implementation that dropped it would ask the relation about a pair with no
// ancestor at all — which every firing row above would notice too.
func TestRedundantStepSilentOnTheFromRoot(t *testing.T) {
	t.Parallel()
	// COMPOSITION is unreferenced and predicate-less here, and is exactly the
	// class the spec witness fires on one position further down.
	const q = "SELECT o/name/value AS n FROM COMPOSITION c CONTAINS OBSERVATION o[" + obsArch + "]"
	silentOnRedundant(t, q, nil)
}

// TestRedundantStepSilentOnAPredicatedOperand covers each predicate kind the
// catalogue row names — archetype, standing and version — since a class-position
// predicate SELECTS, and a step that selects is not inert whatever the routes
// say. Presence is the whole test; the content is never judged.
//
// Each row is the spec witness with one predicate added, so the silence is
// attributable to the predicate and to nothing else.
func TestRedundantStepSilentOnAPredicatedOperand(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, q string }{
		{
			"archetype predicate",
			"SELECT o/name/value AS n FROM EHR e CONTAINS COMPOSITION c[" + compArch + "] " +
				"CONTAINS OBSERVATION o",
		},
		{
			"standing predicate",
			"SELECT o/name/value AS n FROM EHR e CONTAINS COMPOSITION c[name/value='Encounter'] " +
				"CONTAINS OBSERVATION o",
		},
		{
			"version predicate",
			"SELECT c/name/value AS n FROM VERSIONED_COMPOSITION vo[uid/value='x'] " +
				"CONTAINS VERSION v[LATEST_VERSION] CONTAINS COMPOSITION c",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			silentOnRedundant(t, tc.q, nil)
		})
	}
}

// TestRedundantStepSilentOnAReferencedOperand pins the "roots no identified
// path OUTSIDE FROM/CONTAINS" condition across all three clauses that can root
// one. A WHERE filter and an ORDER BY key are references as much as a
// projection is: each makes the operand do work the step alone would not.
func TestRedundantStepSilentOnAReferencedOperand(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, q string }{
		{
			"SELECT",
			"SELECT c/name/value AS m, o/name/value AS n " +
				"FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o",
		},
		{
			"WHERE",
			witnessRedundant + " WHERE c/name/value = 'Encounter'",
		},
		{
			"ORDER BY",
			witnessRedundant + " ORDER BY c/name/value",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			silentOnRedundant(t, tc.q, nil)
		})
	}
}

// TestRedundantStepSilentInsideAJunction pins the junction rule: removing an
// operand from a junction changes the junction's arity, which the relation
// alone cannot prove inert (REQ-164 § Path-shape checks). Three spellings,
// because the operand can sit on either side of the junction.
func TestRedundantStepSilentInsideAJunction(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, q string }{
		{
			// The junction is the step's CHILD: there is no single child class
			// to ask the relation about.
			"junction below the operand",
			"SELECT o/name/value AS n FROM EHR e CONTAINS COMPOSITION c " +
				"CONTAINS (OBSERVATION o OR EVALUATION v)",
		},
		{
			// The operand is INSIDE the junction, and would otherwise fire: its
			// own parent/child pair is the spec witness's.
			"operand inside the junction",
			"SELECT o/name/value AS n FROM EHR e " +
				"CONTAINS (COMPOSITION c CONTAINS OBSERVATION o OR EHR_STATUS st)",
		},
		{
			// An AND junction is no different: the arity argument does not turn
			// on the connector.
			"AND junction",
			"SELECT o/name/value AS n FROM EHR e " +
				"CONTAINS (COMPOSITION c CONTAINS OBSERVATION o AND EHR_STATUS st)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			silentOnRedundant(t, tc.q, nil)
		})
	}
}

// TestRedundantStepSilentUnderNegation pins the negation rule: removing an
// operand from a negated subtree changes the EXCLUDED set, which is not the
// same claim the relation proves. Both positions are rows — the negated
// operand itself, and the operand whose child is negated (whose own step is
// what selects what the negation then excludes).
func TestRedundantStepSilentUnderNegation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, q string }{
		{
			"the operand is negated",
			"SELECT e/ehr_id/value AS id FROM EHR e " +
				"NOT CONTAINS COMPOSITION c CONTAINS OBSERVATION o",
		},
		{
			"the operand's child is negated",
			"SELECT e/ehr_id/value AS id FROM EHR e " +
				"CONTAINS COMPOSITION c NOT CONTAINS OBSERVATION o",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			silentOnRedundant(t, tc.q, nil)
		})
	}
}

// --- No double-reporting -----------------------------------------------------

// voRedundantRelation extends the default with one consumer edge whose FROM
// endpoint the pin does not know, so SITE's only exit is that edge and the
// VERSIONED_COMPOSITION it names becomes unavoidable on the way to a
// COMPOSITION. That is what makes the VERSIONED_OBJECT skip observable at all:
// on the default relation alone no VERSIONED_* class is ever unavoidable (the
// EHR overlay reaches every payload family directly as well as through its
// container), so a skip tested only there would pass with the guard deleted.
func voRedundantRelation() *contain.TypeRelation {
	return contain.Default().WithOverlay(contain.Edge{From: "SITE", To: "VERSIONED_COMPOSITION"})
}

// TestRedundantStepYieldsToTheVersionedObjectCode is REQ-164 § No
// double-reporting, in the direction that matters: an operand whose class
// conforms to VERSIONED_OBJECT keeps REQ-161's aql_versioned_object_unreferenced
// and MUST NOT also raise this group's code. The general case yields to the
// specific.
//
// The relation is chosen so the operand WOULD otherwise fire — see
// [voRedundantRelation] — which is what makes the skip mutation-detectable
// rather than vacuous.
func TestRedundantStepYieldsToTheVersionedObjectCode(t *testing.T) {
	t.Parallel()
	const q = "SELECT c/name/value AS n FROM SITE s CONTAINS VERSIONED_COMPOSITION vo " +
		"CONTAINS COMPOSITION c[" + compArch + "]"
	opts := &lint.Options{Relation: voRedundantRelation()}

	// The premise: without the skip this operand is a firing shape. Asked of
	// the relation directly, since the point is that the RELATION proves it.
	if !opts.Relation.Unavoidable("SITE", "VERSIONED_COMPOSITION", "COMPOSITION") {
		t.Fatal("premise gone: the relation no longer proves the step inert, so the skip proves nothing")
	}
	res := lint.LintString(q, opts)
	if want := []string{codeVersionedObjectUnreferenced}; !slices.Equal(codes(res), want) {
		t.Errorf("codes = %v, want exactly %v — the VERSIONED_OBJECT operand keeps REQ-161's code alone", codes(res), want)
	}
}

// --- Relation supply ---------------------------------------------------------

// TestRedundantStepReadsTheSuppliedRelation pins the one REQ-164 code
// Options.Relation governs. A consumer overlay edge states a containment ROUTE
// fact, and a route round the step is exactly what retires the proof — so a
// dialect deployment that files observations outside compositions must not be
// told its step is redundant. Consulting contain.Default() instead of the
// supplied relation would raise that false finding, which is the class REQ-160
// § Extensibility exists to prevent.
func TestRedundantStepReadsTheSuppliedRelation(t *testing.T) {
	t.Parallel()
	if got := redundantIssues(lint.LintString(witnessRedundant, nil)); len(got) != 1 {
		t.Fatalf("premise gone: the witness raises %d findings under the default relation, want 1", len(got))
	}
	opts := &lint.Options{
		Relation: contain.Default().WithOverlay(contain.Edge{From: "EHR_STATUS", To: "OBSERVATION"}),
	}
	silentOnRedundant(t, witnessRedundant, opts)
	// A relation that says nothing new leaves the finding standing, so the
	// silence above is that EDGE's doing rather than the mere act of supplying
	// a relation. The nil and zero spellings ride the same rows: nil selects
	// the default (REQ-161 § Relation supply), it does not switch the check off.
	for _, tc := range []struct {
		name string
		opts *lint.Options
	}{
		{"nil Options", nil},
		{"zero Options", &lint.Options{}},
		{"explicit default relation", &lint.Options{Relation: contain.Default()}},
		{"unrelated overlay edge", &lint.Options{
			Relation: contain.Default().WithOverlay(contain.Edge{From: "SITE", To: "VERSIONED_COMPOSITION"}),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := redundantIssues(lint.LintString(witnessRedundant, tc.opts)); len(got) != 1 {
				t.Errorf("got %d findings, want 1 — nil selects the default relation, it does not gate the check", len(got))
			}
		})
	}
}

// TestRedundantStepValueFreeFields is REQ-109 § Value-free lint diagnostics on
// this code: the source values a disclosure boundary must never see echoed stay
// out of Code, Severity and Span. Detail and Path are value-BEARING by contract
// and are deliberately not asserted clean.
func TestRedundantStepValueFreeFields(t *testing.T) {
	t.Parallel()
	const q = "SELECT o/name/value AS n FROM EHR e[ehr_id/value='SECRET-EHR'] " +
		"CONTAINS COMPOSITION c CONTAINS OBSERVATION o"
	got := redundantIssues(lint.LintString(q, nil))
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1; the case no longer exercises the code", len(got))
	}
	rendered := fmt.Sprintf("%s %s %+v", got[0].Code, got[0].Severity, got[0].Span)
	for _, v := range []string{"SECRET-EHR", "ehr_id/value"} {
		if strings.Contains(rendered, v) {
			t.Errorf("value-free fields %q contain the source value %q", rendered, v)
		}
	}
}
