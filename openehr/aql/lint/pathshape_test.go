package lint_test

// pathshape_test.go: REQ-164 § Path-shape checks — the repeating-segment
// check aql_path_repeating_unpredicated, and REQ-164 § The conservative
// segment walk's four silent stops as they are OBSERVABLE from outside the
// package (each stop's identity is pinned in pathshape_internal_test.go).
//
// Every guard is mutation-detectable: the mapping from guard to the named test
// that fails when it is removed is recorded in the task report. The silence
// rows carry their own names for the reason REQ-119 § Emission verified after
// emission records — an over-firing linter must not ship green, and a
// firing-only corpus cannot tell.
//
// These rows are the first code's share of PROBE-099 arm (a) — the per-code
// positive plus per-rule negative near-miss corpus. The other four codes carry
// their share in pathshape_parseonly_test.go (the two parse-only ones),
// pathshape_fanout_test.go and pathshape_redundant_test.go.
//
// PROBE-099 is Implemented: its corpus lives in
// testkit/probes/aql/probe_099_path_shape_lint.go and re-spells these shapes
// rather than importing them, so a change cannot move the probe and the package
// it checks together. The facts its arm (b) additivity guard rests on are pinned
// here per code: PROBE-028's valid.aql cassette predicates EVERY repeating
// segment, so it gains no aql_path_repeating_unpredicated (see the cassette row
// in TestPathRepeatingUnpredicatedSilentOnPredicatedSegments) — it does gain
// aql_select_no_alias, the recorded re-baseline pinned in
// TestSelectNoAliasFiresOnTheCassetteProjections.

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// The archetype predicates keep aql_from_archetype off these queries, so a
// count assertion over the whole result stays about the code under test.
//
// The three REQ-164 code names are declared here, together, and shared across
// the group's other test files — pathshape_parseonly_test.go takes pagingCode
// and aliasCode, pathshape_fanout_test.go takes repCode, and the two archetype
// ids reach all four files. The group's whole-result assertions name each
// other's codes, so one spelling of each is what keeps them in step. The
// remaining two codes are declared beside their own rows (fanoutPathCode,
// redundantCode), which no other file names.
const (
	obsArch    = "openEHR-EHR-OBSERVATION.blood_pressure.v1"
	compArch   = "openEHR-EHR-COMPOSITION.encounter.v1"
	repCode    = "aql_path_repeating_unpredicated"
	pagingCode = "aql_paging_no_order_by"
	aliasCode  = "aql_select_no_alias"
)

// auditQuery is the AQL-FIT-04 audit's verified-silent projection: an
// unpredicated repeating-segment path over an OBSERVATION alias, which the
// shipped v0.22.0 linter answered clean (REQ-164 § Acceptance).
const auditQuery = "SELECT o/data/events/data/items/value/magnitude " +
	"FROM EHR e CONTAINS OBSERVATION o[" + obsArch + "]"

// repeatingIssues returns the aql_path_repeating_unpredicated findings, in
// result order.
func repeatingIssues(r lint.Result) []lint.Issue {
	var out []lint.Issue
	for _, i := range r.Issues {
		if i.Code == repCode {
			out = append(out, i)
		}
	}
	return out
}

// spanText is the source text a single-line span covers. It fails the test
// when the span does not address q, which is the assertion that a span points
// at the finding rather than at an invented position.
func spanText(t *testing.T, q string, sp lint.Span) string {
	t.Helper()
	if sp.IsZero() {
		t.Fatalf("issue carries no span although the path has a position (query %q)", q)
	}
	start, end := sp.Start.Col-1, sp.End.Col-1
	if sp.Start.Line != 1 || sp.End.Line != 1 || start < 0 || end > len(q) || start >= end {
		t.Fatalf("span %+v does not address the %d-column single-line query", sp, len(q))
	}
	return q[start:end]
}

// spannedSegments is the source text of every repeating-segment finding's
// span, in result order — the segments the check actually named.
func spannedSegments(t *testing.T, q string) []string {
	t.Helper()
	var out []string
	for _, iss := range repeatingIssues(lint.LintString(q, nil)) {
		out = append(out, spanText(t, q, iss.Span))
	}
	return out
}

// --- Firing rules ------------------------------------------------------------

// TestPathRepeatingUnpredicatedFiresOnTheAuditQuery is REQ-164 § Acceptance's
// first named row: the audit's verified-silent projection now warns.
//
// ONE finding, on `events`. The path names three further repeating segments in
// its text, and the walk reaches none of them: `OBSERVATION.data` types to
// HISTORY (single-valued), `HISTORY.events` to EVENT as a container — this
// finding — and `EVENT.data` is the BMM generic parameter `T`, where the walk
// stops (REQ-164 § The conservative segment walk, and
// TestPathRepeatingUnpredicatedStopsAtTheGenericParameter below).
func TestPathRepeatingUnpredicatedFiresOnTheAuditQuery(t *testing.T) {
	t.Parallel()
	res := lint.LintString(auditQuery, nil)
	got := repeatingIssues(res)
	if len(got) != 1 {
		t.Fatalf("got %d %s findings, want exactly 1: %v", len(got), repCode, codes(res))
	}
	if seg := spanText(t, auditQuery, got[0].Span); seg != "events" {
		t.Errorf("span covers %q, want the offending segment %q", seg, "events")
	}
	if got[0].Severity != lint.Warning {
		t.Errorf("severity = %v, want warning (REQ-164 § Path-shape checks)", got[0].Severity)
	}
	if want := "o/data/events/data/items/value/magnitude"; got[0].Path != want {
		t.Errorf("Issue.Path = %q, want the offending path %q", got[0].Path, want)
	}
	// The whole result, not just this code: the audit query carries exactly
	// two REQ-164 defects — this one and an unaliased projection — so a third
	// finding would mean this group over-reaches. Named in full and in order,
	// not counted, so a code SWAP fails here too.
	if want := []string{repCode, aliasCode}; !slices.Equal(codes(res), want) {
		t.Errorf("codes = %v, want %v", codes(res), want)
	}
}

// TestPathRepeatingUnpredicatedFiresOnAWhereOnlyPath is REQ-164 § Acceptance's
// clause-scope witness: the offending path appears ONLY in WHERE. An
// implementation narrowed to the projection fails here — the SELECT path of
// this query (`o/name/value`) is entirely single-valued.
func TestPathRepeatingUnpredicatedFiresOnAWhereOnlyPath(t *testing.T) {
	t.Parallel()
	const q = "SELECT o/name/value FROM OBSERVATION o[" + obsArch + "] " +
		"WHERE o/data/events/time/value > '2020-01-01'"
	if got := spannedSegments(t, q); len(got) != 1 || got[0] != "events" {
		t.Fatalf("named segments %v, want exactly [events] — a WHERE-only path "+
			"carries the same which-occurrence ambiguity a projected one does", got)
	}
}

// TestPathRepeatingUnpredicatedFiresOnAnOrderByOnlyPath is the clause-scope
// rule's third arm: an ORDER BY key over an unconstrained repeating segment.
func TestPathRepeatingUnpredicatedFiresOnAnOrderByOnlyPath(t *testing.T) {
	t.Parallel()
	const q = "SELECT o/name/value FROM OBSERVATION o[" + obsArch + "] " +
		"ORDER BY o/data/events/time/value DESC"
	if got := spannedSegments(t, q); len(got) != 1 || got[0] != "events" {
		t.Fatalf("named segments %v, want exactly [events]", got)
	}
}

// TestPathRepeatingUnpredicatedFiresOncePerOffendingSegment pins the
// once-per-segment rule, and with it that an offending segment does not END
// the walk: `COMPOSITION.content` is a container typing to CONTENT_ITEM, whose
// inherited `links` is a container too, so one path yields two findings with
// two distinct spans.
func TestPathRepeatingUnpredicatedFiresOncePerOffendingSegment(t *testing.T) {
	t.Parallel()
	const q = "SELECT c/content/links/meaning/value FROM COMPOSITION c[" + compArch + "]"
	got := spannedSegments(t, q)
	if len(got) != 2 || got[0] != "content" || got[1] != "links" {
		t.Fatalf("named segments %v, want [content links]", got)
	}
}

// TestPathRepeatingUnpredicatedSpanSurvivesAPredicateOnAnEarlierSegment pins
// the span derivation against the spellings that make a scan which
// RE-TOKENIZES the path text place the span wrongly or lose it: a '/' inside
// an earlier segment's predicate; a ']' and a '/' inside a string literal; an
// AQL-escaped quote, which is a BACKSLASH escape and not the SQL-style
// doubling a naive literal-skipper assumes ([aql.StringValue]'s token rules);
// and a `MATCHES {/…/}` regex carrying a ']' of its own, which no bracket
// counter models. The span must still cover `events` in every one — an
// approximate span is worse than none, because a consumer cannot tell an
// approximation from a fact, and a lost one leaves a MUST-carry-a-span
// finding without one.
func TestPathRepeatingUnpredicatedSpanSurvivesAPredicateOnAnEarlierSegment(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, predicate string }{
		{"node id", "at0001"},
		{"node id and name", "at0001, 'Systolic'"},
		{"slash inside the predicate", "name/value='Systolic'"},
		{"bracket and slash inside a literal", "name/value='x]/y'"},
		{"escaped quote inside a literal", `name/value='it\'s'`},
		{"bracket and slash inside a regex", "name/value MATCHES {/a]b/}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := "SELECT o/data[" + tc.predicate + "]/events/time FROM OBSERVATION o[" + obsArch + "]"
			if got := spannedSegments(t, q); len(got) != 1 || got[0] != "events" {
				t.Fatalf("named segments %v, want exactly [events] (query %q)", got, q)
			}
		})
	}
}

// TestPathRepeatingUnpredicatedSpanSurvivesTriviaInsideThePath pins the span
// against the trivia the lexer skips but [aql.IdentifiedPath.Raw] keeps
// verbatim (REQ-119): padding, a line break, and an AQL comment — after the
// '/' and before it, the latter carrying a '/' and an apostrophe of its own,
// which are structure nowhere. The line-crossing rows also pin that a span past
// a newline reports the new line and restarts the column, rather than counting
// columns off the first line.
func TestPathRepeatingUnpredicatedSpanSurvivesTriviaInsideThePath(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, path string
		start, end parse.Position
	}{
		{
			"padding", "o/data / events/time",
			parse.Position{Line: 1, Col: 17},
			parse.Position{Line: 1, Col: 23},
		},
		{
			"line break", "o/data/\n events/time",
			parse.Position{Line: 2, Col: 2},
			parse.Position{Line: 2, Col: 8},
		},
		{
			"comment", "o/data/ -- pick one\n events/time",
			parse.Position{Line: 2, Col: 2},
			parse.Position{Line: 2, Col: 8},
		},
		// This comment sits BEFORE the separator and carries a '/' of its own,
		// so a scan that did not step over comments would count that '/' as a
		// path separator and look for the segment inside the comment text.
		{
			"slash inside a comment", "o/data -- a/b\n/events/time",
			parse.Position{Line: 2, Col: 2},
			parse.Position{Line: 2, Col: 8},
		},
		// …and an apostrophe in a comment does not open a string literal.
		{
			"apostrophe inside a comment", "o/data -- it's a/b\n/events/time",
			parse.Position{Line: 2, Col: 2},
			parse.Position{Line: 2, Col: 8},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := "SELECT " + tc.path + " FROM OBSERVATION o[" + obsArch + "]"
			got := repeatingIssues(lint.LintString(q, nil))
			if len(got) != 1 {
				t.Fatalf("got %d findings, want 1 (query %q)", len(got), q)
			}
			want := lint.Span{Start: tc.start, End: tc.end}
			if got[0].Span != want {
				t.Errorf("span = %+v, want %+v — the six columns of `events`", got[0].Span, want)
			}
		})
	}
}

// TestPathRepeatingUnpredicatedValueFreeFields is REQ-109 § Value-free lint
// diagnostics on the new code: the source values a disclosure boundary must
// never see echoed stay out of Code, Severity and Span. Detail and Path are
// value-BEARING by contract and are deliberately not asserted clean.
func TestPathRepeatingUnpredicatedValueFreeFields(t *testing.T) {
	t.Parallel()
	const q = "SELECT o/data[name/value='Systolic']/events/time FROM OBSERVATION o[" + obsArch + "]"
	got := repeatingIssues(lint.LintString(q, nil))
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1; the case no longer exercises the code", len(got))
	}
	rendered := fmt.Sprintf("%s %s %+v", got[0].Code, got[0].Severity, got[0].Span)
	for _, v := range []string{"Systolic", "name/value"} {
		if strings.Contains(rendered, v) {
			t.Errorf("value-free fields %q contain the source value %q", rendered, v)
		}
	}
}

// TestPathShapeGroupIsUngated pins REQ-164 § Always on, never gated: the group
// runs whenever Layer 2 runs, with no option to enable it. The three Options
// spellings a caller can reach must all yield the same finding — including a
// supplied Relation, which governs no ATTRIBUTE question: typing and
// multiplicity are class facts of the pinned RM, and an overlay edge states a
// containment ROUTE fact, never an attribute one (REQ-164 § The conservative
// segment walk). The group's one route-question code,
// aql_contains_redundant_step, does read the supplied relation — that is
// pathshape_redundant_test.go's TestRedundantStepReadsTheSuppliedRelation, and
// it is a different claim from gating: nil selects the default there too.
func TestPathShapeGroupIsUngated(t *testing.T) {
	t.Parallel()
	nilOpts := spannedSegments(t, auditQuery)
	if len(nilOpts) != 1 {
		t.Fatalf("named segments %v under a nil Options, want exactly one finding", nilOpts)
	}
	for _, tc := range []struct {
		name string
		opts *lint.Options
	}{
		{"zero Options", &lint.Options{}},
		{"default relation", &lint.Options{Relation: contain.Default()}},
		// The overlay carries a containment edge, which is the only thing a
		// relation can say — and it says nothing about HISTORY.events.
		{"overlay relation", &lint.Options{
			Relation: contain.Default().WithOverlay(contain.Edge{From: "OBSERVATION", To: "COMPOSITION"}),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got []string
			for _, iss := range repeatingIssues(lint.LintString(auditQuery, tc.opts)) {
				got = append(got, spanText(t, auditQuery, iss.Span))
			}
			if !slices.Equal(got, nilOpts) {
				t.Errorf("named segments %v, want %v — no Options field gates this group", got, nilOpts)
			}
		})
	}
}

// TestPathShapeFindingsKeepResultOK pins REQ-164 § Acceptance's severity row:
// a Result carrying only this group's findings reports OK() true, so a
// consumer that gated on OK() before this group existed sees no change.
func TestPathShapeFindingsKeepResultOK(t *testing.T) {
	t.Parallel()
	res := lint.LintString(auditQuery, nil)
	if len(repeatingIssues(res)) == 0 {
		t.Fatalf("no %s finding; the case stopped exercising the code", repCode)
	}
	if !res.OK() {
		t.Errorf("OK() = false with only path-shape findings (%v); every code in "+
			"the group is Warning", codes(res))
	}
}

// --- Silence rules (one named row per near miss) -----------------------------

// TestPathRepeatingUnpredicatedSilentOnPredicatedSegments is REQ-164
// § Acceptance's predicated-segment near miss. ANY predicate suppresses the
// finding: presence suffices and content is never judged — whether `[at0006]`
// is the RIGHT node id is Layer 3's question
// (aql_path_not_in_template), not this check's.
//
// The first row is PROBE-028's valid.aql cassette verbatim, which is why that
// probe's baseline gains no repeating-segment code (REQ-164 § Additivity; the
// unaliased-projection code it does gain is pinned in
// TestSelectNoAliasFiresOnTheCassetteProjections).
func TestPathRepeatingUnpredicatedSilentOnPredicatedSegments(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, path string }{
		{
			"cassette spelling (node ids throughout)",
			"o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude",
		},
		{"node id", "o/data/events[at0006]/time"},
		{"node id and name", "o/data/events[at0006, 'Systolic']/time"},
		{"standing comparison", "o/data/events[name/value='Systolic']/time"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := "SELECT " + tc.path + " FROM OBSERVATION o[" + obsArch + "]"
			if got := spannedSegments(t, q); got != nil {
				t.Errorf("named segments %v on a predicated path (query %q); a "+
					"predicate suppresses the finding", got, q)
			}
		})
	}
}

// TestPathRepeatingUnpredicatedSilentOnSingleValuedPath is the multiplicity
// guard's own row: every segment types, none is a container, so nothing fires.
// An implementation that dropped the IsContainer test would warn on all of
// these.
func TestPathRepeatingUnpredicatedSilentOnSingleValuedPath(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"o/name/value",
		"o/data/origin/value",
		"o/uid/value",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			q := "SELECT " + path + " FROM OBSERVATION o[" + obsArch + "]"
			if got := spannedSegments(t, q); got != nil {
				t.Errorf("named segments %v on a wholly single-valued path (query %q)", got, q)
			}
		})
	}
}

// TestPathRepeatingUnpredicatedSilentOnUnknownAliasClass is the first named
// silent stop: the walk cannot start, so the path goes unjudged rather than
// judged against a guess. Both spellings reach it — a class the pin does not
// know, and an alias that binds nothing at all (each has its own code:
// aql_unknown_rm_class and aql_unknown_alias).
func TestPathRepeatingUnpredicatedSilentOnUnknownAliasClass(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, query string }{
		{"class outside the pin", "SELECT x/data/events/time FROM NOT_AN_RM_CLASS x"},
		{"alias bound to nothing", "SELECT zz/data/events/time FROM OBSERVATION o[" + obsArch + "]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := spannedSegments(t, tc.query); got != nil {
				t.Errorf("named segments %v (query %q); an unknown alias class ends "+
					"the walk silently", got, tc.query)
			}
		})
	}
}

// TestPathRepeatingUnpredicatedSilentOnParamArchetypeScope is the second named
// silent stop: a `$param` archetype scope, whose extent the CDR resolves at
// execution — the skip Layer 3 and the REQ-161 checks already apply for the
// same reason.
func TestPathRepeatingUnpredicatedSilentOnParamArchetypeScope(t *testing.T) {
	t.Parallel()
	const q = "SELECT o/data/events/time FROM OBSERVATION o[$arch]"
	if got := spannedSegments(t, q); got != nil {
		t.Errorf("named segments %v on a $param archetype scope (query %q)", got, q)
	}
	// The same path under a LITERAL archetype does warn — without this half the
	// row would pass on a check that had simply stopped working.
	lit := "SELECT o/data/events/time FROM OBSERVATION o[" + obsArch + "]"
	if got := spannedSegments(t, lit); len(got) != 1 || got[0] != "events" {
		t.Fatalf("named segments %v under a literal archetype, want [events]", got)
	}
}

// TestPathRepeatingUnpredicatedSilentOnUndeclaredAttribute is the third named
// silent stop: the pin declares no such attribute on the current class, so
// neither that segment nor anything below it is judged — `items` is a
// container on SECTION and on ITEM_TREE, and on OBSERVATION it is not an
// attribute at all.
func TestPathRepeatingUnpredicatedSilentOnUndeclaredAttribute(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"o/items/value",              // stops at the first segment
		"o/data/nosuch/events/value", // stops mid-path, above a real container
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			q := "SELECT " + path + " FROM OBSERVATION o[" + obsArch + "]"
			if got := spannedSegments(t, q); got != nil {
				t.Errorf("named segments %v (query %q); an attribute the pin does not "+
					"declare ends the walk silently", got, q)
			}
		})
	}
}

// TestPathRepeatingUnpredicatedStopsAtTheGenericParameter is the fourth named
// silent stop, and REQ-164 § Acceptance names it explicitly: `EVENT.data` is
// literally typed `T` on the pinned tables, and REQ-048 leaves generic-
// parameter resolution out of scope, so the walk stops there.
//
// The consequence is observable and is the point: the audit query's `items` is
// a container on every class that declares it, and the check MUST NOT name it
// — reaching it would mean the walk had guessed which class `T` stands for.
func TestPathRepeatingUnpredicatedStopsAtTheGenericParameter(t *testing.T) {
	t.Parallel()
	got := spannedSegments(t, auditQuery)
	if len(got) != 1 || got[0] != "events" {
		t.Fatalf("named segments %v, want exactly [events]: everything below "+
			"EVENT.data's generic `T` is beyond the walk's reach", got)
	}
	// Named directly, not merely absent from a count: the segment below the
	// stop is a container whose spelling carries no predicate, so it is the one
	// a walk that descended through `T` would report.
	for _, iss := range repeatingIssues(lint.LintString(auditQuery, nil)) {
		if strings.Contains(iss.Detail, "items") {
			t.Errorf("Detail %q names a segment below the generic-parameter stop", iss.Detail)
		}
	}
}

// TestPathRepeatingUnpredicatedSilentOnBareAliasAndStar covers the two paths
// with nothing to walk: a bare alias projection (PROBE-028's
// missing_archetype.aql cassette shape) and `SELECT *`, which roots no
// identified path at all.
func TestPathRepeatingUnpredicatedSilentOnBareAliasAndStar(t *testing.T) {
	t.Parallel()
	for _, q := range []string{
		"SELECT o FROM OBSERVATION o[" + obsArch + "]",
		"SELECT * FROM OBSERVATION o[" + obsArch + "]",
	} {
		t.Run(q, func(t *testing.T) {
			t.Parallel()
			if got := spannedSegments(t, q); got != nil {
				t.Errorf("named segments %v; the query roots no walkable path", got)
			}
		})
	}
}
