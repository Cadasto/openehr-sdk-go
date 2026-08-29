package lint_test

// pathshape_parseonly_test.go: REQ-164 § Path-shape checks — the group's two
// PARSE-ONLY codes, aql_paging_no_order_by and aql_select_no_alias. Neither
// consults the pinned BMM, so neither has a walk or a silent stop to pin;
// what they have instead is two channels (the paging check) and two exemptions
// (the alias check), and every arm of each carries its own named row.
//
// Every guard is mutation-detectable: the mapping from guard to the named test
// that fails when it is removed is recorded in the task report. The silence
// rows carry their own names for the reason REQ-119 § Emission verified after
// emission records — an over-firing linter must not ship green, and a
// firing-only corpus cannot tell.
//
// These rows are two of the five codes' share of PROBE-099 arm (a); the
// repeating-segment code's share is in pathshape_test.go, and the remaining two
// in pathshape_fanout_test.go and pathshape_redundant_test.go. pathshape_test.go
// also declares the code names and archetype ids these files share.

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
)

// pagedQuery is the AQL-FIT-04 audit's second verified-silent query: a row
// bound with no total order, which the shipped v0.22.0 linter answered clean
// (REQ-164 § Acceptance). The projection is aliased so the row is about the
// paging code alone.
const pagedQuery = "SELECT o/name/value AS name FROM OBSERVATION o[" + obsArch + "] " +
	"LIMIT 50 OFFSET 100"

// envelope is a request envelope carrying the given row bound — the second
// channel aql_paging_no_order_by reads (REQ-164 § Path-shape checks).
func envelope(fetch, offset int) *lint.Options {
	q := aql.NewQuery("")
	q.Fetch, q.Offset = fetch, offset
	return &lint.Options{Query: &q}
}

// --- aql_paging_no_order_by: firing rules ------------------------------------

// TestPagingNoOrderByFiresOnTheAuditQuery is REQ-164 § Acceptance's second
// named row: the audit's `LIMIT 50 OFFSET 100` query with no ORDER BY now
// warns. ONE finding over the whole result, so a second code would mean this
// group over-reaches on a query carrying exactly one defect.
func TestPagingNoOrderByFiresOnTheAuditQuery(t *testing.T) {
	t.Parallel()
	res := lint.LintString(pagedQuery, nil)
	if want := []string{pagingCode}; !slices.Equal(codes(res), want) {
		t.Fatalf("codes = %v, want %v", codes(res), want)
	}
	iss := res.Issues[0]
	if iss.Severity != lint.Warning {
		t.Errorf("severity = %v, want warning (REQ-164 § Path-shape checks)", iss.Severity)
	}
	// The channel that carried the bound is named, which is the only thing
	// distinguishing the two arms for a reader.
	if !strings.Contains(iss.Detail, "LIMIT") {
		t.Errorf("Detail %q does not name the in-text channel that carried the bound", iss.Detail)
	}
	if !res.OK() {
		t.Errorf("OK() = false on a Warning-only result: %v", codes(res))
	}
}

// TestPagingNoOrderByFiresFromTheEnvelopeAlone is the envelope arm's positive:
// the AQL text carries no row bound at all, and the bound arrives on
// [lint.Options.Query] — the same channel the parameter-binding checks read.
// Fetch and Offset each carry it on their own: REQ-164 § Path-shape checks
// reads Offset too, deliberately unlike aql_top_with_fetch's fetch-only scope,
// because an offset into an unordered result is an unstable page boundary by
// itself.
func TestPagingNoOrderByFiresFromTheEnvelopeAlone(t *testing.T) {
	t.Parallel()
	const q = "SELECT o/name/value AS name FROM OBSERVATION o[" + obsArch + "]"
	for _, tc := range []struct {
		name          string
		fetch, offset int
	}{
		{"fetch only", 20, 0},
		{"offset only", 0, 100},
		{"both", 20, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := lint.LintString(q, envelope(tc.fetch, tc.offset))
			if want := []string{pagingCode}; !slices.Equal(codes(res), want) {
				t.Fatalf("codes = %v, want %v", codes(res), want)
			}
			if d := res.Issues[0].Detail; !strings.Contains(d, "envelope") {
				t.Errorf("Detail %q does not name the envelope channel", d)
			}
			if strings.Contains(res.Issues[0].Detail, "LIMIT") {
				t.Errorf("Detail %q names an in-text LIMIT the query does not carry", res.Issues[0].Detail)
			}
		})
	}
}

// TestPagingNoOrderByFiresOncePerQueryOverBothChannels pins the at-most-once
// rule: a query bounded in TEXT and by the ENVELOPE has one missing total
// order, so it gets one finding — with both channels named, since which
// bounded the rows is what a reader needs.
func TestPagingNoOrderByFiresOncePerQueryOverBothChannels(t *testing.T) {
	t.Parallel()
	res := lint.LintString(pagedQuery, envelope(20, 0))
	if want := []string{pagingCode}; !slices.Equal(codes(res), want) {
		t.Fatalf("codes = %v, want %v — one missing order, one finding", codes(res), want)
	}
	for _, channel := range []string{"LIMIT", "envelope"} {
		if !strings.Contains(res.Issues[0].Detail, channel) {
			t.Errorf("Detail %q does not name the %s channel", res.Issues[0].Detail, channel)
		}
	}
}

// TestPagingNoOrderByReportsNoPosition pins the code's two unlocalised
// fields. Neither channel has a position to point at — [parse.Document]
// records the LIMIT clause's presence but not its place, and the envelope is
// not in the query text at all — so REQ-109 § Value-free lint diagnostics'
// zero Span is the honest answer and an invented one would be worse than
// none. Path is empty for the same reason: the finding is about the query,
// not about any path in it. (Detail is value-BEARING by contract and is
// deliberately not asserted clean; it names the channels, which needs no
// count.)
func TestPagingNoOrderByReportsNoPosition(t *testing.T) {
	t.Parallel()
	res := lint.LintString(pagedQuery, envelope(20, 0))
	if len(res.Issues) != 1 {
		t.Fatalf("codes = %v, want exactly the paging finding", codes(res))
	}
	iss := res.Issues[0]
	if !iss.Span.IsZero() {
		t.Errorf("Span = %+v, want the zero Span", iss.Span)
	}
	if iss.Path != "" {
		t.Errorf("Path = %q, want empty: the finding is about the query, not a path", iss.Path)
	}
}

// --- aql_paging_no_order_by: silence rules (one named row per near miss) -----

// TestPagingNoOrderBySilentWithOrderBy is the ORDER BY near miss: the total
// order is the remedy, so its presence silences the finding on BOTH channels —
// the in-text bound, the envelope bound, and the two together.
func TestPagingNoOrderBySilentWithOrderBy(t *testing.T) {
	t.Parallel()
	const ordered = "SELECT o/name/value AS name FROM OBSERVATION o[" + obsArch + "] " +
		"ORDER BY o/name/value LIMIT 50 OFFSET 100"
	for _, tc := range []struct {
		name string
		opts *lint.Options
	}{
		{"in-text bound", nil},
		{"envelope bound too", envelope(20, 100)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := codes(lint.LintString(ordered, tc.opts)); len(got) != 0 {
				t.Errorf("codes = %v on a query carrying ORDER BY, want none", got)
			}
		})
	}
	// The same query WITHOUT the ORDER BY does warn — without this half the
	// row would pass on a check that had simply stopped working.
	if !has(lint.LintString(pagedQuery, nil), pagingCode) {
		t.Fatalf("the unordered control stopped firing; the ORDER BY row proves nothing")
	}
}

// TestPagingNoOrderBySilentWithoutAnyRowBound is the no-bound near miss: an
// unbounded query has no page boundary to be unstable, ORDER BY or not.
func TestPagingNoOrderBySilentWithoutAnyRowBound(t *testing.T) {
	t.Parallel()
	const q = "SELECT o/name/value AS name FROM OBSERVATION o[" + obsArch + "]"
	if got := codes(lint.LintString(q, nil)); len(got) != 0 {
		t.Errorf("codes = %v on an unbounded query, want none", got)
	}
}

// TestPagingNoOrderBySilentWithoutAnEnvelope is REQ-164 § Acceptance's nil-
// Query near miss: with no envelope supplied the envelope arm cannot fire,
// exactly as it leaves the parameter-binding checks unable to. A zero-VALUED
// envelope is the same answer for the same reason — Fetch and Offset unset
// carry no bound — so a supplied-but-empty Query must not fire either.
func TestPagingNoOrderBySilentWithoutAnEnvelope(t *testing.T) {
	t.Parallel()
	const q = "SELECT o/name/value AS name FROM OBSERVATION o[" + obsArch + "]"
	for _, tc := range []struct {
		name string
		opts *lint.Options
	}{
		{"nil Options", nil},
		{"nil Query", &lint.Options{}},
		{"envelope carrying no bound", envelope(0, 0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := codes(lint.LintString(q, tc.opts)); len(got) != 0 {
				t.Errorf("codes = %v, want none: no channel carried a row bound", got)
			}
		})
	}
	// The positive control: the SAME query under an envelope that does carry a
	// bound warns, so the rows above pin the guard rather than a dead check.
	if !has(lint.LintString(q, envelope(20, 0)), pagingCode) {
		t.Fatalf("the bounded control stopped firing; the nil-envelope rows prove nothing")
	}
}

// TestPagingNoOrderBySilentOnATopOnlyBound is REQ-164 § No double-reporting:
// a query whose ONLY row bound is the deprecated TOP clause keeps
// aql_deprecated_top, which already carries the ORDER BY remedy, and does not
// also collect this code — one defect gets one finding.
func TestPagingNoOrderBySilentOnATopOnlyBound(t *testing.T) {
	t.Parallel()
	const q = "SELECT TOP 5 o/name/value AS name FROM OBSERVATION o[" + obsArch + "]"
	res := lint.LintString(q, nil)
	if want := []string{"aql_deprecated_top"}; !slices.Equal(codes(res), want) {
		t.Fatalf("codes = %v, want %v — the TOP clause owns its own ORDER BY remedy", codes(res), want)
	}
	// A TOP is not a blanket exemption: the same query gains this code the
	// moment a row bound arrives on a channel TOP does not own.
	withLimit := q + " LIMIT 10"
	if !has(lint.LintString(withLimit, nil), pagingCode) {
		t.Errorf("codes = %v on TOP with a LIMIT, want the paging code among them",
			codes(lint.LintString(withLimit, nil)))
	}
	if !has(lint.LintString(q, envelope(20, 0)), pagingCode) {
		t.Errorf("codes = %v on TOP with an envelope bound, want the paging code among them",
			codes(lint.LintString(q, envelope(20, 0))))
	}
}

// --- aql_select_no_alias: firing rules ---------------------------------------

// TestSelectNoAliasFiresOncePerUnaliasedItem pins the per-item rule and the
// span: two unaliased columns yield two findings, each spanning its own item,
// and the aliased column between them yields none.
func TestSelectNoAliasFiresOncePerUnaliasedItem(t *testing.T) {
	t.Parallel()
	const q = "SELECT o/name/value, o/uid/value AS uid, o/language/code_string " +
		"FROM OBSERVATION o[" + obsArch + "]"
	res := lint.LintString(q, nil)
	if want := []string{aliasCode, aliasCode}; !slices.Equal(codes(res), want) {
		t.Fatalf("codes = %v, want %v — one per unaliased item", codes(res), want)
	}
	for i, want := range []string{"o/name/value", "o/language/code_string"} {
		if got := spanText(t, q, res.Issues[i].Span); got != want {
			t.Errorf("finding %d spans %q, want the item %q", i, got, want)
		}
		if res.Issues[i].Path != want {
			t.Errorf("finding %d Path = %q, want %q", i, res.Issues[i].Path, want)
		}
		if res.Issues[i].Severity != lint.Warning {
			t.Errorf("finding %d severity = %v, want warning", i, res.Issues[i].Severity)
		}
	}
}

// TestSelectNoAliasFiresOnABareAlias pins REQ-164 § Path-shape checks'
// explicit non-exemption: `SELECT o` names no column either, and the engine
// picks that column's name as freely as it picks a path's.
func TestSelectNoAliasFiresOnABareAlias(t *testing.T) {
	t.Parallel()
	const q = "SELECT o FROM OBSERVATION o[" + obsArch + "]"
	res := lint.LintString(q, nil)
	if want := []string{aliasCode}; !slices.Equal(codes(res), want) {
		t.Fatalf("codes = %v, want %v", codes(res), want)
	}
	if got := spanText(t, q, res.Issues[0].Span); got != "o" {
		t.Errorf("span covers %q, want the bare alias item %q", got, "o")
	}
}

// TestSelectNoAliasFiresOnItemsWithNoPathToName covers the item shapes that
// carry no position of their own — a function call and a literal projection.
// They still fire (they name no column either), and they report the ZERO Span
// rather than a span over something INSIDE the item: REQ-109 § Value-free lint
// diagnostics has an unattributable issue report no position. Detail's ordinal
// is what locates them.
func TestSelectNoAliasFiresOnItemsWithNoPathToName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, item string }{
		{"aggregate call", "COUNT(o)"},
		{"function over a path", "MAX(o/name/value)"},
		{"literal", "'x'"},
		// COUNT(*) is NOT a `*` item: that star belongs to the aggregate call,
		// not to the projection (which is why it does not raise
		// aql_select_star either), and the column it produces has an
		// engine-defined name like any other. So the star exemption must not
		// reach it.
		{"count star", "COUNT(*)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := "SELECT " + tc.item + " FROM OBSERVATION o[" + obsArch + "]"
			res := lint.LintString(q, nil)
			if want := []string{aliasCode}; !slices.Equal(codes(res), want) {
				t.Fatalf("codes = %v, want %v (query %q)", codes(res), want, q)
			}
			iss := res.Issues[0]
			if !iss.Span.IsZero() {
				t.Errorf("Span = %+v, want the zero Span: the item carries no position, "+
					"and a span over one of its parts would point inside the item", iss.Span)
			}
			if iss.Path != "" {
				t.Errorf("Path = %q, want empty: the item projects no single path", iss.Path)
			}
			if !strings.Contains(iss.Detail, "item 1") {
				t.Errorf("Detail %q does not carry the item's ordinal, which is all that "+
					"locates an unspannable item", iss.Detail)
			}
		})
	}
}

// TestSelectNoAliasOrdinalCountsTheStar pins that Detail's ordinal is the
// SOURCE projection ordinal — the one a reader counts commas to — so the
// exempt star still occupies its place in the count.
func TestSelectNoAliasOrdinalCountsTheStar(t *testing.T) {
	t.Parallel()
	const q = "SELECT *, o/name/value FROM OBSERVATION o[" + obsArch + "]"
	res := lint.LintString(q, nil)
	if want := []string{"aql_select_star", aliasCode}; !slices.Equal(codes(res), want) {
		t.Fatalf("codes = %v, want %v", codes(res), want)
	}
	if d := res.Issues[1].Detail; !strings.Contains(d, "item 2") {
		t.Errorf("Detail %q, want the second projection: the star holds place one", d)
	}
}

// TestSelectNoAliasFiresOnTheCassetteProjections is the PROBE-028 re-baseline
// recorded at the unit level (REQ-164 § Additivity): both cassette queries
// project a column with no AS alias, so each genuinely carries this defect and
// gains exactly this code. probe028Cases carries the same fact at the probe
// level; this row is what fails first if the rule behind it moves.
func TestSelectNoAliasFiresOnTheCassetteProjections(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, query string }{
		{
			"valid.aql",
			"SELECT o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude " +
				"FROM OBSERVATION o[" + obsArch + "]",
		},
		{
			"missing_archetype.aql",
			"SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.lab_result.v1]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if want := []string{aliasCode}; !slices.Equal(codes(lint.LintString(tc.query, nil)), want) {
				t.Errorf("codes = %v, want %v", codes(lint.LintString(tc.query, nil)), want)
			}
		})
	}
}

// --- aql_select_no_alias: silence rules (one named row per near miss) --------

// TestSelectNoAliasSilentOnAliasedProjection is the aliased near miss: every
// item names its column, so nothing fires — including the shapes that carry no
// path of their own.
func TestSelectNoAliasSilentOnAliasedProjection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, projection string }{
		{"single column", "o/name/value AS name"},
		{"bare alias", "o AS obs"},
		{"aggregate", "COUNT(o) AS n"},
		{"every column of several", "o/name/value AS name, o/uid/value AS uid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := "SELECT " + tc.projection + " FROM OBSERVATION o[" + obsArch + "]"
			if got := codes(lint.LintString(q, nil)); len(got) != 0 {
				t.Errorf("codes = %v on a fully aliased projection (query %q), want none", got, q)
			}
		})
	}
}

// TestSelectNoAliasSilentOnAStarItem is the `*` near miss: there is nothing to
// alias, and REQ-164 § No double-reporting gives that shape to REQ-109's
// aql_select_star. Both spellings of a star are covered — the bare form, which
// carries no projection item at all, and the mixed form, which does.
func TestSelectNoAliasSilentOnAStarItem(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, query string }{
		{"bare star", "SELECT * FROM OBSERVATION o[" + obsArch + "]"},
		{"mixed star", "SELECT *, o/name/value AS name FROM OBSERVATION o[" + obsArch + "]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := lint.LintString(tc.query, nil)
			if want := []string{"aql_select_star"}; !slices.Equal(codes(res), want) {
				t.Errorf("codes = %v, want %v — the star item is REQ-109's, not this code's",
					codes(res), want)
			}
		})
	}
}

// --- ungated, and OK() ------------------------------------------------------

// TestParseOnlyChecksAreUngated pins REQ-164 § Always on, never gated for the
// two parse-only codes: no Options field switches either off. Options.Relation
// governs neither — neither check consults an RM fact at all — and
// Options.Query is a CHANNEL for the paging check, never a gate: supplying one
// can only add the envelope arm, and a nil one leaves the in-text arm firing.
func TestParseOnlyChecksAreUngated(t *testing.T) {
	t.Parallel()
	// Unaliased AND row-bounded with no order: both codes, on every spelling.
	const q = "SELECT o/name/value FROM OBSERVATION o[" + obsArch + "] LIMIT 50"
	want := []string{pagingCode, aliasCode}
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
		{"envelope carrying no bound", envelope(0, 0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := lint.LintString(q, tc.opts)
			if !slices.Equal(codes(res), want) {
				t.Errorf("codes = %v, want %v — no Options field gates these checks", codes(res), want)
			}
			if !res.OK() {
				t.Errorf("OK() = false with only path-shape findings (%v); both codes are Warning", codes(res))
			}
		})
	}
}
