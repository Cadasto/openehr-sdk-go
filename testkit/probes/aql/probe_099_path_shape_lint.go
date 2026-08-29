package aqlprobes

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
)

// PROBE-099 — the REQ-164 path-shape lint corpus.
//
// PROBE-097's structure MINUS its read/write-parity arm: every REQ-164 code is
// read-side only, so there is no builder analogue for a parity arm to compare
// against. What is left is arm (a), the per-code firing rows and the negative
// near misses, and arm (b), the additivity guard that re-runs PROBE-028's
// cassette corpus.
//
// The five REQ-164 codes are spelled here independently of openehr/aql/lint's
// own strings, for the reason PROBE-097 spells the REQ-161 codes independently:
// a probe that imported the constants it is pinning would pass through a rename
// that broke every consumer.
const (
	codePathRepeatingUnpredicated = "aql_path_repeating_unpredicated"
	codePagingNoOrderBy           = "aql_paging_no_order_by"
	codeSelectNoAlias             = "aql_select_no_alias"
	codeFanoutPathGrain           = "aql_fanout_path_grain"
	codeContainsRedundantStep     = "aql_contains_redundant_step"
)

// pathShapeCodesAll is the full REQ-164 catalogue — arm (a)'s universe. Both
// arm-(a) runners filter a [lint.Result] down to this set, and both arm-(a)
// completeness guards enumerate it, so a code dropped from this list is a code
// PROBE-099 stops demanding corpus rows for. A package-level var for the reason
// [semanticCodesAll] is one: every caller only reads it (via [pathShapeCodes]).
//
// Arm (b) does NOT consult it: runLintCase (probe_028_aql_lint.go) compares the
// FULL issue-code multiset against the case's WantCodes, with no filter and no
// catalogue — which is what makes a gained REQ-164 code visible there as a
// baseline break rather than as a silently tolerated addition.
var pathShapeCodesAll = []string{
	codePathRepeatingUnpredicated, codePagingNoOrderBy, codeSelectNoAlias,
	codeFanoutPathGrain, codeContainsRedundantStep,
}

// pathShapeCodes returns [pathShapeCodesAll]; that var's doc carries the
// contract.
func pathShapeCodes() []string { return pathShapeCodesAll }

// PathShapeFireRow names one firing row the PROBE-099 wire assertion requires
// BY NAME (conformance.md § PROBE-099, arm (a)) — the two queries the AQL-FIT-04
// audit verified silent on the shipped v0.22.0 linter and which MUST now warn,
// and the clause-scope witness REQ-164 § Acceptance names beside them. A
// [PathShapeFireCase] tagged with one counts towards that requirement, and
// [Probe099PathShapeLint] fails a corpus in which any is unclaimed.
//
// A firing row that merely exercises a code carries no tag: the per-code
// coverage guard already demands one row per code, and these three are the rows
// the wire assertion names on top of that.
type PathShapeFireRow string

const (
	// FireAuditRepeatingSegment — the audit's unpredicated repeating-segment
	// projection.
	FireAuditRepeatingSegment PathShapeFireRow = "audit: the unpredicated repeating-segment projection"
	// FireAuditRowBound — the audit's LIMIT-without-ORDER BY query.
	FireAuditRowBound PathShapeFireRow = "audit: the LIMIT-without-ORDER BY query"
	// FireClauseScopeWhereOnly — a repeating segment reached only through
	// WHERE, which an implementation narrowed to the projection would miss.
	FireClauseScopeWhereOnly PathShapeFireRow = "clause scope: a WHERE-only offending path"
)

// mandatoryFireRowsAll is the set arm (a)'s firing completeness guard enumerates
// beyond one-row-per-code. A package-level var for the same reason as
// [pathShapeCodesAll].
var mandatoryFireRowsAll = []PathShapeFireRow{
	FireAuditRepeatingSegment, FireAuditRowBound, FireClauseScopeWhereOnly,
}

// PathShapeNegative names one of the negative near misses the PROBE-099 wire
// assertion requires BY NAME (conformance.md § PROBE-099, arm (a)) — the eleven
// per-rule near misses and the conservative walk's four silent stops. A
// [PathShapeSilentCase] tagged with one counts towards that requirement, and
// [Probe099PathShapeLint] fails a corpus in which any is unclaimed.
type PathShapeNegative string

const (
	// NegPredicatedSegment — any predicate on a multi-valued segment
	// suppresses aql_path_repeating_unpredicated.
	NegPredicatedSegment PathShapeNegative = "predicated repeating segment"
	// NegOrderByPresent — a total order beside a row bound silences
	// aql_paging_no_order_by.
	NegOrderByPresent PathShapeNegative = "ORDER BY beside a row bound"
	// NegNilEnvelope — with no Options.Query the envelope arm cannot fire.
	NegNilEnvelope PathShapeNegative = "nil Options.Query"
	// NegTopOnlyBound — a query whose only row bound is the deprecated TOP
	// keeps aql_deprecated_top and does not also collect the paging code.
	NegTopOnlyBound PathShapeNegative = "TOP-only row bound"
	// NegAliasedProjection — an item that names its column raises nothing.
	NegAliasedProjection PathShapeNegative = "aliased projection item"
	// NegStarItem — a `*` item has nothing to alias; that shape is REQ-109's
	// aql_select_star.
	NegStarItem PathShapeNegative = "star projection item"
	// NegSharedRepeatingPrefix — two projected paths whose multi-valued
	// segments are all in their common prefix are one repeating scope, not a
	// product.
	NegSharedRepeatingPrefix PathShapeNegative = "repeating segments in the common prefix"
	// NegDifferentAliases — two projected paths on different aliases are the
	// junction question, aql_fanout_row_grain (REQ-161).
	NegDifferentAliases PathShapeNegative = "projected paths on different aliases"
	// NegAvoidableIntermediate — an avoidable unreferenced intermediate
	// narrows the result, so it is not redundant.
	NegAvoidableIntermediate PathShapeNegative = "avoidable unreferenced intermediate"
	// NegUnreferencedLeaf — an unreferenced leaf is an existence filter and
	// does work.
	NegUnreferencedLeaf PathShapeNegative = "unreferenced leaf"
	// NegVersionedObjectOperand — a VERSIONED_OBJECT-conforming operand keeps
	// REQ-161's aql_versioned_object_unreferenced (REQ-164 § No
	// double-reporting).
	NegVersionedObjectOperand PathShapeNegative = "VERSIONED_OBJECT-conforming operand"
	// NegWalkStopUnknownClass — the walk cannot start on a class the pin does
	// not know.
	NegWalkStopUnknownClass PathShapeNegative = "walk stop: unknown class"
	// NegWalkStopUndeclaredAttribute — the pin declares no such attribute on
	// the current class.
	NegWalkStopUndeclaredAttribute PathShapeNegative = "walk stop: undeclared attribute"
	// NegWalkStopGenericParameter — the segment types to a BMM generic
	// parameter (`EVENT.data` is literally typed `T`).
	NegWalkStopGenericParameter PathShapeNegative = "walk stop: generic-parameter type"
	// NegWalkStopParamArchetype — a `$param` archetype scope, whose extent the
	// CDR resolves at execution.
	NegWalkStopParamArchetype PathShapeNegative = "walk stop: $param archetype scope"
)

// pathShapeNegativesAll is the set arm (a)'s silence completeness guard
// enumerates. A package-level var for the same reason as [pathShapeCodesAll].
var pathShapeNegativesAll = []PathShapeNegative{
	NegPredicatedSegment, NegOrderByPresent, NegNilEnvelope, NegTopOnlyBound,
	NegAliasedProjection, NegStarItem, NegSharedRepeatingPrefix, NegDifferentAliases,
	NegAvoidableIntermediate, NegUnreferencedLeaf, NegVersionedObjectOperand,
	NegWalkStopUnknownClass, NegWalkStopUndeclaredAttribute,
	NegWalkStopGenericParameter, NegWalkStopParamArchetype,
}

// PathShapeFireCase is one PROBE-099 arm-(a) firing row: Query MUST raise the
// REQ-164 code multiset Want — which MUST include Code, the code the row exists
// for — with Code carried at Severity and spanned as SpanText / SpanNth say.
//
// Want is the WHOLE REQ-164 multiset rather than the single code PROBE-097's
// firing rows demand, because several REQ-164 codes legitimately co-occur: the
// fan-out advisory's PREMISE is two unpredicated repeating segments, so its
// witness carries aql_path_repeating_unpredicated twice by construction (REQ-164
// § No double-reporting gives neither code ownership of the other). Naming the
// multiset in full keeps the row an assertion about the whole result rather than
// about the code it happens to look at.
type PathShapeFireCase struct {
	// Name labels the case for diagnostic output.
	Name string
	// Query is the AQL string under test; assumed single-line (span
	// computation does not handle multi-line queries).
	Query string
	// Fetch and Offset, when either is non-zero, supply the request envelope
	// on [lint.Options.Query] — the second channel aql_paging_no_order_by
	// reads. Both zero means no envelope is supplied at all.
	Fetch, Offset int
	// Code is the REQ-164 issue code this row exists for.
	Code string
	// Want is the exact REQ-164 code multiset Query MUST raise (order
	// irrelevant; duplicates count). It MUST contain Code.
	Want []string
	// Severity is Code's REQ-164 catalogue severity — Warning for all five,
	// asserted per row rather than assumed so a promotion cannot ship quietly.
	Severity lint.Severity
	// SpanText is the source text the issue's Span MUST cover — a segment
	// name, a whole identified path, a projection item or a class token,
	// depending on the code. EMPTY requires the ZERO Span instead, which is
	// aql_paging_no_order_by's honest answer: neither of its channels has a
	// position in the query text to point at.
	SpanText string
	// SpanNth is the 1-based occurrence of SpanText in Query the Span MUST
	// land on. Ignored when SpanText is empty.
	SpanNth int
	// Mandatory, when set, declares this row as one of the firing rows the
	// PROBE-099 wire assertion names explicitly. Each of [PathShapeFireRow]'s
	// constants MUST be claimed by at least one row.
	Mandatory PathShapeFireRow
}

// PathShapeSilentCase is one PROBE-099 arm-(a) silence row: Query MUST reach the
// REQ-164 checks at all, and MUST then raise exactly the REQ-164 code multiset
// in Want — nil for a plain near miss.
//
// Keeps is what makes a YIELDING near miss non-vacuous. Three of the negatives
// are not "nothing fires" but "another code owns this shape" (REQ-164 § No
// double-reporting): the TOP-only bound keeps aql_deprecated_top, the star item
// keeps aql_select_star, and the VERSIONED_OBJECT operand keeps REQ-161's
// aql_versioned_object_unreferenced. Without Keeps those rows would pass just as
// well on a query that had stopped being linted at all.
type PathShapeSilentCase struct {
	// Name labels the case for diagnostic output.
	Name string
	// Query is the AQL string under test.
	Query string
	// Fetch and Offset supply the request envelope, as on
	// [PathShapeFireCase].
	Fetch, Offset int
	// Relation, when non-nil, is the REQ-160 relation the run supplies on
	// [lint.Options.Relation]. Only the VERSIONED_OBJECT negative needs one:
	// on the default relation no VERSIONED_* class is ever unavoidable, so a
	// skip tested there alone would pass with the guard deleted. Nil selects
	// the default relation, which is not the same as switching a check off
	// (REQ-161 § Relation supply).
	Relation *contain.TypeRelation
	// Want is the exact REQ-164 code multiset Query MUST raise (order
	// irrelevant; nil means none).
	Want []string
	// Keeps are codes OUTSIDE the REQ-164 group that Query MUST still carry —
	// the code this near miss yields the shape to.
	Keeps []string
	// ForCode, when set, declares which REQ-164 code's silence this row
	// guards. Every code in [pathShapeCodes] MUST be claimed by at least one
	// row — conformance.md § PROBE-099 arm (a) requires, per code, a firing row
	// AND a negative near miss that stays silent.
	ForCode string
	// Negative, when set, declares this row as one of the near misses the
	// PROBE-099 wire assertion names explicitly. Each of [PathShapeNegative]'s
	// constants MUST be claimed by at least one row.
	Negative PathShapeNegative
}

// PathShapeCorpus is the whole PROBE-099 corpus: three fields across two
// wire-assertion arms — Fire and Silent are the two halves of arm (a), the
// firing rows and the near misses that must stay silent. All three are required.
//
// Additivity reuses [LintCase] (probe_028_aql_lint.go, same package) exactly as
// PROBE-097 arm (b) does: arm (b) is PROBE-028's own corpus re-run under the
// completed REQ-164 linter, not a corpus of its own.
type PathShapeCorpus struct {
	// Fire is arm (a)'s firing rows — one per REQ-164 code, minimum, plus the
	// three the wire assertion names.
	Fire []PathShapeFireCase
	// Silent is arm (a)'s near-miss rows.
	Silent []PathShapeSilentCase
	// Additivity is arm (b): the PROBE-028 corpus, re-run.
	Additivity []LintCase
}

// Probe099PathShapeLint runs every row of both arms and aggregates all failures
// into one [Result] (collect-all, like [Probe097SemanticLint] — a single early
// failure would hide the rest of the corpus from the report).
func Probe099PathShapeLint(c PathShapeCorpus) (Result, error) {
	r := Result{Probe: "PROBE-099"}
	if len(c.Fire) == 0 || len(c.Silent) == 0 || len(c.Additivity) == 0 {
		return r, errors.New("PROBE-099: all three corpus fields (Fire, Silent, Additivity) are required")
	}

	var failures []string
	fireCodes := map[string]bool{}
	fireRows := map[PathShapeFireRow]bool{}
	groupOnlyRows := 0
	for _, tc := range c.Fire {
		msg, groupOnly := runPathShapeFire(tc)
		if msg != "" {
			failures = append(failures, fmt.Sprintf("fire/%s: %s", tc.Name, msg))
		}
		if groupOnly {
			groupOnlyRows++
		}
		fireCodes[tc.Code] = true
		if tc.Mandatory != "" {
			fireRows[tc.Mandatory] = true
		}
	}
	// The per-code coverage guard, mirroring PROBE-097's: a Fire row deleted
	// from the corpus (or never added for a new REQ-164 code) leaves this probe
	// green as long as the surviving rows still pass on their own — the
	// len(c.Fire) == 0 check above only catches an EMPTY corpus.
	for _, code := range pathShapeCodes() {
		if !fireCodes[code] {
			failures = append(failures, fmt.Sprintf("fire: no fire case raises %s; the corpus does not exercise it", code))
		}
	}
	for _, row := range mandatoryFireRowsAll {
		if !fireRows[row] {
			failures = append(failures, fmt.Sprintf("fire: no fire case claims the %q row; the PROBE-099 wire assertion names it explicitly", row))
		}
	}
	// REQ-164 § Acceptance's severity row, enforced rather than assumed: a
	// Result carrying ONLY this group's findings reports OK() true. Each fire
	// row asserts it for itself when its whole result is group-only
	// ([runPathShapeFire]); this is the guard that at least one row IS, so the
	// claim cannot go untested on a corpus whose every row happens to carry a
	// REQ-109 or REQ-161 bystander code too.
	if groupOnlyRows == 0 {
		failures = append(failures, "fire: no fire case yields a group-only result; the OK() claim goes untested")
	}

	silentCodes := map[string]bool{}
	silentNegatives := map[PathShapeNegative]bool{}
	for _, tc := range c.Silent {
		if msg := runPathShapeSilent(tc); msg != "" {
			failures = append(failures, fmt.Sprintf("silent/%s: %s", tc.Name, msg))
		}
		if tc.ForCode != "" {
			silentCodes[tc.ForCode] = true
		}
		if tc.Negative != "" {
			silentNegatives[tc.Negative] = true
		}
	}
	// The mirror of the fire coverage guard, for the half of arm (a) that
	// asserts SILENCE — the half that can go dark quietly, since a deleted near
	// miss leaves every surviving row passing on its own. An over-firing linter
	// MUST NOT ship green (REQ-164 § Acceptance, and the REQ-119 lesson it
	// cites), which a firing-only corpus cannot tell.
	for _, code := range pathShapeCodes() {
		if !silentCodes[code] {
			failures = append(failures, fmt.Sprintf("silent: no silence case guards %s; the corpus does not pin its near miss", code))
		}
	}
	for _, neg := range pathShapeNegativesAll {
		if !silentNegatives[neg] {
			failures = append(failures, fmt.Sprintf("silent: no silence case pins the %q negative; the PROBE-099 wire assertion names it explicitly", neg))
		}
	}

	for _, tc := range c.Additivity {
		// runLintCase (probe_028_aql_lint.go) is the additivity guard itself: it
		// asserts the FULL issue-code multiset, so a REQ-164 code gained on a
		// cassette that carries no REQ-164 defect breaks the baseline here. The
		// codes two of these cassettes DO gain are a deliberate, recorded
		// re-baseline (REQ-164 § Additivity, recorded in conformance.md's
		// PROBE-099 entry) — a change to what the caller passes in, not to what
		// this guard does.
		if msg := runLintCase(tc); msg != "" {
			failures = append(failures, fmt.Sprintf("additivity/%s: %s", tc.Name, msg))
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

// pathShapeOptions builds the [lint.Options] a corpus row asks for: a request
// envelope when either bound is non-zero, and a supplied relation when one is
// given. It returns nil when the row asks for neither, so the commonest row runs
// through the same nil-Options entry point a consumer with no options reaches.
func pathShapeOptions(fetch, offset int, rel *contain.TypeRelation) *lint.Options {
	if fetch == 0 && offset == 0 && rel == nil {
		return nil
	}
	opts := &lint.Options{Relation: rel}
	if fetch != 0 || offset != 0 {
		q := aql.NewQuery("")
		q.Fetch, q.Offset = fetch, offset
		opts.Query = &q
	}
	return opts
}

// runPathShapeFire runs one arm-(a) firing row.
//
// groupOnly reports whether the row's WHOLE result was REQ-164 findings — which
// is the condition under which this row could assert REQ-164 § Acceptance's
// OK() claim, and which the caller counts so the claim cannot go untested
// corpus-wide.
func runPathShapeFire(tc PathShapeFireCase) (msg string, groupOnly bool) {
	// A row whose Want omits its own Code would assert nothing about the code
	// it exists for, and would still satisfy the per-code coverage guard — so
	// the corpus itself is checked before the linter is.
	if !slices.Contains(tc.Want, tc.Code) {
		return fmt.Sprintf("corpus row error: Want %v does not contain the row's own Code %s", tc.Want, tc.Code), false
	}
	res := lint.LintString(tc.Query, pathShapeOptions(tc.Fetch, tc.Offset, nil))
	got := filterCodes(res, pathShapeCodes())
	if !slices.Equal(got, sortedCopy(tc.Want)) {
		return fmt.Sprintf("path-shape codes = %v, want %v (query %q)", got, sortedCopy(tc.Want), tc.Query), false
	}
	idx := slices.IndexFunc(res.Issues, func(i lint.Issue) bool { return i.Code == tc.Code })
	iss := res.Issues[idx]
	if iss.Severity != tc.Severity {
		return fmt.Sprintf("%s severity = %v, want %v", tc.Code, iss.Severity, tc.Severity), false
	}
	if iss.Detail == "" {
		return tc.Code + " carries no Detail", false
	}
	if tc.SpanText == "" {
		// The zero Span is a POSITIVE assertion here, not a skipped one: an
		// unattributable finding reports no position rather than an invented
		// one (REQ-109 § Value-free lint diagnostics), and a span that appeared
		// on aql_paging_no_order_by would be exactly that invention.
		if !iss.Span.IsZero() {
			return fmt.Sprintf("%s span = %+v, want the zero Span — neither paging channel has a position in the query text", tc.Code, iss.Span), false
		}
	} else {
		want, err := tokenSpan(tc.Query, tc.SpanText, tc.SpanNth)
		if err != nil {
			return err.Error(), false
		}
		if iss.Span != want {
			return fmt.Sprintf("%s span = %+v, want %+v (on %q)", tc.Code, iss.Span, want, tc.SpanText), false
		}
	}
	// Group-only means every finding in the result belongs to REQ-164, which is
	// the shape REQ-164 § Acceptance's OK() claim is about. Where a REQ-109 or
	// REQ-161 code rides along, OK() is that code's business and this row says
	// nothing about it.
	groupOnly = len(res.Issues) == len(got)
	if groupOnly && !res.OK() {
		return "OK() = false on a result carrying only REQ-164 findings; every code in the group is Warning", true
	}
	return "", groupOnly
}

// runPathShapeSilent runs one arm-(a) silence row.
//
// Silence is proved, not assumed: a Layer-1 failure is rejected BEFORE the
// comparison, because [filterCodes] drops aql_syntax / aql_empty (neither is a
// path-shape code) and a Want-nil row over an unparseable query would otherwise
// compare nil against nil and pass vacuously — the same hole [runSemanticSilent]
// closes for PROBE-097.
func runPathShapeSilent(tc PathShapeSilentCase) string {
	res := lint.LintString(tc.Query, pathShapeOptions(tc.Fetch, tc.Offset, tc.Relation))
	if code := layer1Code(res); code != "" {
		return fmt.Sprintf("query never reached the REQ-164 checks (%s): a silence row MUST assert silence on a query that actually linted, not on one Layer 1 rejected (query %q)",
			code, tc.Query)
	}
	got := filterCodes(res, pathShapeCodes())
	if !slices.Equal(got, sortedCopy(tc.Want)) {
		return fmt.Sprintf("path-shape codes = %v, want %v (query %q)", got, sortedCopy(tc.Want), tc.Query)
	}
	for _, code := range tc.Keeps {
		if !slices.ContainsFunc(res.Issues, func(i lint.Issue) bool { return i.Code == code }) {
			return fmt.Sprintf("the shape yields to %s, but the result does not carry it (codes %v, query %q)",
				code, allCodes(res), tc.Query)
		}
	}
	return ""
}

// allCodes is the sorted multiset of every issue code in r, for a diagnostic
// that must show what the result DID carry rather than the filtered view the
// comparison used.
func allCodes(r lint.Result) []string {
	out := make([]string, 0, len(r.Issues))
	for _, i := range r.Issues {
		out = append(out, i.Code)
	}
	slices.Sort(out)
	return out
}
