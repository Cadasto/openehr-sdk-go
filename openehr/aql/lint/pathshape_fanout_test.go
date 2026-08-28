package lint_test

// pathshape_fanout_test.go: REQ-164 § Path-shape checks — aql_fanout_path_grain,
// the PATH source of engine-defined row multiplicity (SPECQUERY-9). Its sibling
// aql_fanout_row_grain (REQ-161) owns the JUNCTION source, and the two are
// disjoint by construction: this one needs two projected paths under ONE alias,
// that one two projected aliases under one AND junction. Both directions of that
// disjointness carry a named row here.
//
// Every guard is mutation-detectable: the mapping from guard to the named test
// that fails when it is removed is recorded in the task report. The silence rows
// carry their own names for the reason REQ-119 § Emission verified after
// emission records — an over-firing linter must not ship green, and a
// firing-only corpus cannot tell.
//
// These rows are the fourth code's share of PROBE-099 arm (a); the other three
// codes' shares are in pathshape_test.go (which declares the code names shared
// across the group's files) and pathshape_parseonly_test.go.

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
)

const fanoutPathCode = "aql_fanout_path_grain"

// obsFrom binds one OBSERVATION alias under a literal archetype, so a
// whole-result assertion over a query using it stays about the codes under test
// (no aql_from_archetype, no containment finding).
const obsFrom = " FROM OBSERVATION o[" + obsArch + "]"

// twoAliasFrom binds two walkable aliases in a plain CONTAINS CHAIN — no
// junction, so aql_fanout_row_grain has nothing to say about it and a silence
// row over it is about this code alone.
const twoAliasFrom = " FROM EHR e CONTAINS COMPOSITION c[" + compArch + "] " +
	"CONTAINS OBSERVATION o[" + obsArch + "]"

// witness is REQ-164 § The conservative segment walk's named firing witness,
// quoted there verbatim as `SELECT o/data/events/time, o/links/meaning/value`:
// the two paths diverge AT the alias, with an unpredicated container on each
// branch (HISTORY.events and LOCATABLE.links, the latter inherited by
// OBSERVATION). The columns are aliased so the result carries no
// aql_select_no_alias and the whole-result assertions stay legible.
const witness = "SELECT o/data/events/time AS t, o/links/meaning/value AS m" + obsFrom

// fanoutPathIssues returns the aql_fanout_path_grain findings, in result order.
func fanoutPathIssues(r lint.Result) []lint.Issue {
	var out []lint.Issue
	for _, i := range r.Issues {
		if i.Code == fanoutPathCode {
			out = append(out, i)
		}
	}
	return out
}

// oneFanoutPath fails unless the query raised exactly one finding of this code,
// and returns it.
func oneFanoutPath(t *testing.T, q string, opts *lint.Options) lint.Issue {
	t.Helper()
	res := lint.LintString(q, opts)
	got := fanoutPathIssues(res)
	if len(got) != 1 {
		t.Fatalf("got %d %s findings, want exactly 1 (codes %v, query %q)",
			len(got), fanoutPathCode, codes(res), q)
	}
	return got[0]
}

// --- Firing rules ------------------------------------------------------------

// TestFanoutPathGrainFiresOnTheSpecWitness is the firing witness REQ-164 § The
// conservative segment walk names outright. It also pins the whole finding: the
// Span covers the LATER path of the pair, Detail names BOTH paths and cites
// SPECQUERY-9 as its sibling does, the severity is Warning, and the result is
// still OK().
//
// The whole-result code list is asserted in full and in order, not counted, so
// a code swap fails here too — and so does any leak of the JUNCTION code, which
// this shape must never raise (one alias, no junction).
func TestFanoutPathGrainFiresOnTheSpecWitness(t *testing.T) {
	t.Parallel()
	res := lint.LintString(witness, nil)
	// Two unpredicated repeating segments (one per branch) are the PREMISE of
	// the fan-out, so the repeating-segment code accompanies it by construction:
	// the two report different defects — which occurrence, and how many rows —
	// and REQ-164 § No double-reporting gives neither ownership of the other.
	if want := []string{repCode, repCode, fanoutPathCode}; !slices.Equal(codes(res), want) {
		t.Fatalf("codes = %v, want %v", codes(res), want)
	}
	iss := fanoutPathIssues(res)[0]
	if got := spanText(t, witness, iss.Span); got != "o/links/meaning/value" {
		t.Errorf("span covers %q, want the LATER path of the pair", got)
	}
	if iss.Path != "o/links/meaning/value" {
		t.Errorf("Issue.Path = %q, want the path the span covers", iss.Path)
	}
	if iss.Severity != lint.Warning {
		t.Errorf("severity = %v, want warning (REQ-164 § Path-shape checks)", iss.Severity)
	}
	for _, want := range []string{"o/data/events/time", "o/links/meaning/value", "SPECQUERY-9"} {
		if !strings.Contains(iss.Detail, want) {
			t.Errorf("Detail %q does not name %q", iss.Detail, want)
		}
	}
	if !res.OK() {
		t.Errorf("OK() = false on a Warning-only result: %v", codes(res))
	}
}

// TestFanoutPathGrainCountsSegmentsTypedBeforeAWalkStop pins REQ-164 § The
// conservative segment walk's bounded-reach ruling: a walk that stopped still
// contributes the segments it typed BEFORE the stop. The first path here is the
// AQL-FIT-04 audit projection, whose walk ends at `EVENT.data`'s generic `T` —
// and its already-typed `HISTORY.events` is what makes the pair.
//
// The `items` below that stop is a container on every class that declares it,
// and the repeating SCOPE Detail names must not be it: reaching it would mean
// the walk had guessed what `T` stands for. (The path SPELLING Detail quotes
// does contain `/items/` — that is the query's own text, which is why the
// assertion is on the `Class.attribute` rendering of the scope.)
func TestFanoutPathGrainCountsSegmentsTypedBeforeAWalkStop(t *testing.T) {
	t.Parallel()
	const q = "SELECT o/data/events/data/items/value/magnitude AS t, " +
		"o/links/meaning/value AS m" + obsFrom
	iss := oneFanoutPath(t, q, nil)
	if !strings.Contains(iss.Detail, "HISTORY.events") {
		t.Errorf("Detail %q does not name the repeating scope typed above the stop", iss.Detail)
	}
	if strings.Contains(iss.Detail, ".items") {
		t.Errorf("Detail %q names a repeating scope below the generic-parameter stop", iss.Detail)
	}
}

// TestFanoutPathGrainFiresOncePerAlias pins the once-per-ALIAS rule: three
// mutually diverging projected paths on one alias make three offending pairs
// and yield ONE advisory, not a quadratic report. The first pair in document
// order — paths one and two — fixes the Span and is named in Detail; the third
// path appears in neither.
func TestFanoutPathGrainFiresOncePerAlias(t *testing.T) {
	t.Parallel()
	const q = "SELECT o/data/events/time AS t, o/links/meaning/value AS m, " +
		"o/other_participations/function/value AS p" + obsFrom
	iss := oneFanoutPath(t, q, nil)
	if got := spanText(t, q, iss.Span); got != "o/links/meaning/value" {
		t.Errorf("span covers %q, want the later path of the FIRST offending pair", got)
	}
	if !strings.Contains(iss.Detail, "o/data/events/time") {
		t.Errorf("Detail %q does not name the earlier path of the first pair", iss.Detail)
	}
	if strings.Contains(iss.Detail, "other_participations") {
		t.Errorf("Detail %q names the third path; only the first offending pair is reported", iss.Detail)
	}
}

// TestFanoutPathGrainStopsAfterOnePairUnderOneLaterPath is the once-per-alias
// budget's SECOND guard, and it needs a shape of its own: the `reported` map is
// consulted when a later path is CHOSEN, so within a single later path nothing
// but the loop break keeps a second pair from being emitted for the same alias.
//
// This projection reaches that state. Its first two paths pair INERTLY with each
// other — their only repeating segment, `events`, sits in their common prefix —
// while BOTH of them offend against the third. So the third path forms two
// offending pairs, and only the break stops the second from being reported.
//
// Mutation check recorded in the task report: removing the break makes exactly
// this test fail, with two aql_fanout_path_grain findings for alias `o`.
func TestFanoutPathGrainStopsAfterOnePairUnderOneLaterPath(t *testing.T) {
	t.Parallel()
	const q = "SELECT o/data/events/time AS a, o/data/events/name/value AS b, " +
		"o/links/meaning/value AS c" + obsFrom
	res := lint.LintString(q, nil)
	// The whole result, in order: each of the three paths carries its own
	// unpredicated repeating segment, and the pair adds ONE advisory — not one
	// per offending pair.
	want := []string{repCode, repCode, repCode, fanoutPathCode}
	if !slices.Equal(codes(res), want) {
		t.Fatalf("codes = %v, want %v — one advisory per alias, not per pair", codes(res), want)
	}
	iss := fanoutPathIssues(res)[0]
	if got := spanText(t, q, iss.Span); got != "o/links/meaning/value" {
		t.Errorf("span covers %q, want the later path the pair was found under", got)
	}
	if !strings.Contains(iss.Detail, "o/data/events/time") {
		t.Errorf("Detail %q does not name the earlier path of the first pair", iss.Detail)
	}
	if strings.Contains(iss.Detail, "o/data/events/name/value") {
		t.Errorf("Detail %q names the second offending pair's earlier path; the search "+
			"stops at the first pair under a later path", iss.Detail)
	}
}

// TestFanoutPathGrainReportsTheFirstOFFENDINGPair pins that "first pair" means
// the first pair that OFFENDS, not the first pair of paths. The leading column
// here is wholly single-valued, so every pair it forms is inert and the finding
// belongs to the two paths after it.
func TestFanoutPathGrainReportsTheFirstOFFENDINGPair(t *testing.T) {
	t.Parallel()
	const q = "SELECT o/name/value AS n, o/data/events/time AS t, " +
		"o/links/meaning/value AS m" + obsFrom
	iss := oneFanoutPath(t, q, nil)
	if got := spanText(t, q, iss.Span); got != "o/links/meaning/value" {
		t.Errorf("span covers %q, want the later path of the first OFFENDING pair", got)
	}
	if !strings.Contains(iss.Detail, "o/data/events/time") {
		t.Errorf("Detail %q does not name the earlier path of that pair", iss.Detail)
	}
	if strings.Contains(iss.Detail, "o/name/value") {
		t.Errorf("Detail %q names the inert leading column", iss.Detail)
	}
}

// TestFanoutPathGrainFiresPerAliasNotPerQuery pins that "once per alias" is a
// per-ALIAS budget, not a per-query one: two aliases, each carrying its own
// diverging pair, yield two findings.
//
// The two rows are the SAME four paths INTERLEAVED differently, which is what
// makes them pin the search order. A finding is reported AT the later path of
// its pair, and the search is ordered by that path — so the findings come out
// in the document order of the paths they span, and the ALIASES swap places
// between the rows. An implementation that grouped by alias first (say, in
// first-seen order) would emit `c` before `o` in both rows and fail the second.
func TestFanoutPathGrainFiresPerAliasNotPerQuery(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		projection string
		wantSpans  []string
	}{
		{
			"alias pairs nested",
			"c/content/links/meaning/value AS w, o/data/events/time AS x, " +
				"o/links/meaning/value AS y, c/context/participations/function/value AS z",
			[]string{"o/links/meaning/value", "c/context/participations/function/value"},
		},
		{
			"alias pairs interleaved",
			"c/content/links/meaning/value AS w, o/data/events/time AS x, " +
				"c/context/participations/function/value AS y, o/links/meaning/value AS z",
			[]string{"c/context/participations/function/value", "o/links/meaning/value"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := "SELECT " + tc.projection + twoAliasFrom
			got := fanoutPathIssues(lint.LintString(q, nil))
			if len(got) != len(tc.wantSpans) {
				t.Fatalf("got %d findings, want %d — one per alias", len(got), len(tc.wantSpans))
			}
			for i, want := range tc.wantSpans {
				if s := spanText(t, q, got[i].Span); s != want {
					t.Errorf("finding %d spans %q, want %q", i, s, want)
				}
			}
		})
	}
}

// TestFanoutPathGrainValueFreeFields is REQ-109 § Value-free lint diagnostics on
// this code: the source values a disclosure boundary must never see echoed stay
// out of Code, Severity and Span. Detail and Path are value-BEARING by contract
// and are deliberately not asserted clean — Detail names the two paths, which is
// the whole of the finding.
func TestFanoutPathGrainValueFreeFields(t *testing.T) {
	t.Parallel()
	const q = "SELECT o/data[name/value='Systolic']/events/time AS t, " +
		"o/links/meaning/value AS m" + obsFrom
	iss := oneFanoutPath(t, q, nil)
	rendered := fmt.Sprintf("%s %s %+v", iss.Code, iss.Severity, iss.Span)
	for _, v := range []string{"Systolic", "name/value"} {
		if strings.Contains(rendered, v) {
			t.Errorf("value-free fields %q contain the source value %q", rendered, v)
		}
	}
}

// TestFanoutPathGrainIsUngated pins REQ-164 § Always on, never gated for this
// code: no Options field switches it off. Options.Relation governs it least of
// all — an overlay edge states a containment ROUTE fact, and this check asks
// only about attribute typing and the projection's shape.
func TestFanoutPathGrainIsUngated(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		opts *lint.Options
	}{
		{"nil Options", nil},
		{"zero Options", &lint.Options{}},
		{"default relation", &lint.Options{Relation: contain.Default()}},
		{"overlay relation", &lint.Options{
			Relation: contain.Default().WithOverlay(contain.Edge{From: "OBSERVATION", To: "COMPOSITION"}),
		}},
		{"request envelope", envelope(20, 0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			iss := oneFanoutPath(t, witness, tc.opts)
			if got := spanText(t, witness, iss.Span); got != "o/links/meaning/value" {
				t.Errorf("span covers %q, want the later path — no Options field gates this check", got)
			}
			if !lint.LintString(witness, tc.opts).OK() {
				t.Errorf("OK() = false; every code in this group is Warning")
			}
		})
	}
}

// --- Silence rules (one named row per near miss) -----------------------------

// TestFanoutPathGrainSilentWhenRepeatingScopesAreInTheCommonPrefix is the
// catalogue's first MUST NOT: two paths whose multi-valued segments all sit in
// their COMMON PREFIX descend one repeating scope, and one scope is no product.
// The prefix and duplicate rows are the degenerate cases of the same rule — a
// path with nothing beyond the shared part cannot be half of a product.
func TestFanoutPathGrainSilentWhenRepeatingScopesAreInTheCommonPrefix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, projection string }{
		{
			"diverging below the shared repeating segment",
			"o/data/events/time AS a, o/data/events/name/value AS b",
		},
		{
			"one path a prefix of the other",
			"o/data/events/time AS a, o/data/events/time/value AS b",
		},
		{
			"the same path projected twice",
			"o/data/events/time AS a, o/data/events/time AS b",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := "SELECT " + tc.projection + obsFrom
			if got := fanoutPathIssues(lint.LintString(q, nil)); got != nil {
				t.Errorf("got %d findings on one repeating scope (query %q), want none", len(got), q)
			}
		})
	}
	// The positive control: the same alias, diverging ABOVE its repeating
	// segments, does warn — without it these rows would pass on a dead check.
	if !has(lint.LintString(witness, nil), fanoutPathCode) {
		t.Fatalf("the diverging control stopped firing; the common-prefix rows prove nothing")
	}
}

// TestFanoutPathGrainSilentWhenEitherPathIsPredicatedAtTheDivergence is the
// catalogue's second MUST NOT: a pair in which EITHER path carries a predicate
// on every multi-valued segment at or after the divergence. Both directions
// carry their own row, because a rule that only looked at one half would pass
// on one of them.
//
// Presence is all that is read, never content — the third row's predicate is a
// standing comparison, which suppresses exactly as a node id does.
func TestFanoutPathGrainSilentWhenEitherPathIsPredicatedAtTheDivergence(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, projection string }{
		{
			"the earlier path is predicated",
			"o/data/events[at0006]/time AS t, o/links/meaning/value AS m",
		},
		{
			"the later path is predicated",
			"o/data/events/time AS t, o/links[at0001]/meaning/value AS m",
		},
		{
			"a standing comparison predicates it",
			"o/data/events[name/value='Systolic']/time AS t, o/links/meaning/value AS m",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := "SELECT " + tc.projection + obsFrom
			if got := fanoutPathIssues(lint.LintString(q, nil)); got != nil {
				t.Errorf("got %d findings although a branch is constrained (query %q), want none",
					len(got), q)
			}
		})
	}
	if !has(lint.LintString(witness, nil), fanoutPathCode) {
		t.Fatalf("the unpredicated control stopped firing; the predicated rows prove nothing")
	}
}

// TestFanoutPathGrainSilentOnPathsRootedOnDifferentAliases is the catalogue's
// third MUST NOT and the DISJOINTNESS pin with REQ-161's aql_fanout_row_grain,
// in both directions:
//
//   - a two-alias shape raises the JUNCTION code and never this one, whether or
//     not the two aliases sit under an AND junction;
//   - the one-alias control raises THIS code and never the junction code.
//
// Both queries give each alias a path with an unpredicated repeating segment, so
// only the alias test keeps this code off them.
func TestFanoutPathGrainSilentOnPathsRootedOnDifferentAliases(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, query  string
		wantJunction bool
	}{
		{
			name: "plain CONTAINS chain",
			query: "SELECT c/content/links/meaning/value AS a, o/data/events/time AS b" +
				twoAliasFrom,
		},
		{
			name: "AND junction — the junction code's own shape",
			query: "SELECT o/links/meaning/value AS a, v/links/meaning/value AS b " +
				"FROM COMPOSITION c[" + compArch + "] CONTAINS " +
				"(OBSERVATION o[" + obsArch + "] AND EVALUATION v)",
			wantJunction: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := lint.LintString(tc.query, nil)
			if got := fanoutPathIssues(res); got != nil {
				t.Errorf("got %d %s findings across two aliases (codes %v), want none — "+
					"that is the junction question", len(got), fanoutPathCode, codes(res))
			}
			if got := has(res, codeFanoutRowGrain); got != tc.wantJunction {
				t.Errorf("%s present = %v, want %v (codes %v)",
					codeFanoutRowGrain, got, tc.wantJunction, codes(res))
			}
		})
	}
	// The other direction: this code's own shape never re-fires the junction
	// code. One alias, no junction, so the junction rule has nothing to read.
	if has(lint.LintString(witness, nil), codeFanoutRowGrain) {
		t.Errorf("codes = %v on a one-alias shape, want no %v",
			codes(lint.LintString(witness, nil)), codeFanoutRowGrain)
	}
}

// TestFanoutPathGrainSilentOnBareSelectStar is the catalogue's fourth MUST NOT:
// a bare `SELECT *` roots no identified path at all, so there is nothing to
// pair — its row-grain question belongs to the junction code, whose rule reads a
// star as projecting every alias.
//
// The MIXED star is deliberately NOT exempt, and carries its own row: the
// exemption is scoped to the bare form by the rule's own words ("projects no
// identified path at all"), and a mixed star projects named columns beside it,
// which pair by the ordinary rule.
func TestFanoutPathGrainSilentOnBareSelectStar(t *testing.T) {
	t.Parallel()
	const bare = "SELECT *" + obsFrom
	res := lint.LintString(bare, nil)
	if want := []string{"aql_select_star"}; !slices.Equal(codes(res), want) {
		t.Errorf("codes = %v, want %v — a bare star projects no path to pair", codes(res), want)
	}
	const mixed = "SELECT *, o/data/events/time AS t, o/links/meaning/value AS m" + obsFrom
	if got := spanText(t, mixed, oneFanoutPath(t, mixed, nil).Span); got != "o/links/meaning/value" {
		t.Errorf("span covers %q, want the later of the two NAMED columns beside the star", got)
	}
}

// TestFanoutPathGrainSilentOnPathsOutsideTheProjection pins the check's clause
// scope, which is deliberately the OPPOSITE of its group-mate
// aql_path_repeating_unpredicated's: a row PRODUCT needs two columns to
// multiply, so a WHERE filter or an ORDER BY key over a second repeating scope
// returns no column and multiplies nothing. An implementation that read every
// clause — as the repeating-segment check MUST — fails these rows.
func TestFanoutPathGrainSilentOnPathsOutsideTheProjection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, query string }{
		{
			"second path in WHERE",
			"SELECT o/data/events/time AS t" + obsFrom +
				" WHERE o/links/meaning/value = 'x'",
		},
		{
			"second path in ORDER BY",
			"SELECT o/data/events/time AS t" + obsFrom +
				" ORDER BY o/links/meaning/value",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := lint.LintString(tc.query, nil)
			if got := fanoutPathIssues(res); got != nil {
				t.Errorf("got %d findings although only one path is projected (codes %v)",
					len(got), codes(res))
			}
			// The unprojected path is still judged by the repeating-segment
			// check, which is what makes this a scope rule rather than a
			// blind spot: BOTH paths carry that code here.
			if n := len(repeatingIssues(res)); n != 2 {
				t.Errorf("got %d %s findings, want 2 — the clause scope differs per code", n, repCode)
			}
		})
	}
	// Moving the same second path INTO the projection warns, so these rows pin
	// the projection filter rather than a dead check.
	if !has(lint.LintString(witness, nil), fanoutPathCode) {
		t.Fatalf("the projected control stopped firing; the clause-scope rows prove nothing")
	}
}

// TestFanoutPathGrainSilentWhenAPathDoesNotType is the walk's silent stops seen
// through this code: a path the pin cannot type contributes no segment, so it
// can never be half of a pair. One row per stop that can reach here — the
// unknown alias class, the `$param` archetype scope, and an attribute the pin
// does not declare (which leaves the other path fully typed and still silent,
// because a pair needs both halves).
func TestFanoutPathGrainSilentWhenAPathDoesNotType(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, query string }{
		{
			"unknown alias class",
			"SELECT x/data/events/time AS t, x/links/meaning/value AS m FROM NOT_AN_RM_CLASS x",
		},
		{
			"$param archetype scope",
			"SELECT o/data/events/time AS t, o/links/meaning/value AS m FROM OBSERVATION o[$arch]",
		},
		{
			"one branch stops at an undeclared attribute",
			"SELECT o/data/events/time AS t, o/nosuch/links/meaning AS m" + obsFrom,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := lint.LintString(tc.query, nil)
			if got := fanoutPathIssues(res); got != nil {
				t.Errorf("got %d findings on an untypable branch (codes %v), want none — "+
					"the walk stops silently", len(got), codes(res))
			}
		})
	}
	if !has(lint.LintString(witness, nil), fanoutPathCode) {
		t.Fatalf("the typable control stopped firing; the silent-stop rows prove nothing")
	}
}
