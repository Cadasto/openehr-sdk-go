package aqlprobes

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// PROBE-097 — the REQ-160/161/162 semantic and portability lint corpus.
//
// Two adapters share one rule engine (openehr/aql/internal/semcheck): the
// read-side linter (openehr/aql/lint, REQ-161) and the write-side builder
// verification ((*aql.Builder).VerifyContainment, REQ-162). This probe pins
// the OBSERVABLE behaviour of both without importing semcheck itself — it is
// internal to openehr/aql, unreachable from testkit's import position (Go's
// internal-visibility rule) — mirroring how PROBE-028 pins openehr/aql/lint
// without importing its own internals.
//
// The REQ-161 issue codes are spelled here independently of semcheck's
// constants, for the same reason openehr/aql/verify_parity_test.go and
// openehr/aql/lint/semantic_test.go spell them independently: a probe that
// imported the strings it is pinning would pass through a rename that broke
// every consumer.
const (
	codeImpossibleContainment       = "aql_impossible_containment"
	codeContainsNotContainable      = "aql_contains_not_containable"
	codeArchetypeClassMismatch      = "aql_archetype_class_mismatch"
	codeUnknownRMClass              = "aql_unknown_rm_class"
	codeContainmentByReference      = "aql_containment_by_reference"
	codeVersionNoPredicate          = "aql_version_no_predicate"
	codeVersionedObjectUnreferenced = "aql_versioned_object_unreferenced"
	codeFanoutRowGrain              = "aql_fanout_row_grain"
)

// semanticCodesAll is the full REQ-161 catalogue — arm (a)'s universe. Both
// arm-(a) runners filter a [lint.Result] down to this set, and both arm-(a)
// completeness guards enumerate it, so a code dropped from this list is a code
// PROBE-097 stops demanding corpus rows for. A package-level var rather than a
// literal built fresh per call: every caller only reads it (via
// [semanticCodes]).
//
// Arm (b) does NOT consult it: runLintCase (probe_028_aql_lint.go) compares the
// FULL issue-code multiset against the case's WantCodes, with no filter and no
// catalogue — which is precisely what makes a gained REQ-161 code break the
// additivity baseline there.
var semanticCodesAll = []string{
	codeImpossibleContainment, codeContainsNotContainable, codeArchetypeClassMismatch,
	codeUnknownRMClass, codeContainmentByReference,
	codeVersionNoPredicate, codeVersionedObjectUnreferenced, codeFanoutRowGrain,
}

// semanticCodes returns [semanticCodesAll]; that var's doc carries the contract.
func semanticCodes() []string { return semanticCodesAll }

// containmentCodesAll is the REQ-162 § Contract five-code subset arm (c) scopes
// parity to. The three portability advisories in [semanticCodesAll] are
// read-side only and never appear from [aql.Builder.VerifyContainment] —
// REQ-162 § Contract is explicit that parity is scoped to this subset, not the
// full catalogue. A package-level var for the same reason as
// [semanticCodesAll].
var containmentCodesAll = []string{
	codeImpossibleContainment, codeContainsNotContainable, codeArchetypeClassMismatch,
	codeUnknownRMClass, codeContainmentByReference,
}

// containmentCodes returns [containmentCodesAll]; that var's doc carries the
// contract.
func containmentCodes() []string { return containmentCodesAll }

// layer1Codes are the two Layer-1 outcomes [lint.LintString] returns INSTEAD of
// linting: an empty query and a parse failure each short-circuit to a single
// issue, and no REQ-161 check ever runs.
var layer1Codes = []string{"aql_empty", "aql_syntax"}

// layer1Code is the Layer-1 failure code carried by r, or "" when r came from a
// query that actually parsed. Neither Layer-1 code is a semantic code, so
// neither survives [filterCodes] — which is why a silence row MUST consult this
// before comparing: otherwise a row whose query never parsed compares nil
// against nil and passes without a single semantic check having run.
func layer1Code(r lint.Result) string {
	for _, i := range r.Issues {
		if slices.Contains(layer1Codes, i.Code) {
			return i.Code
		}
	}
	return ""
}

// MandatoryNegative names one of the three negative cases the PROBE-097 wire
// assertion requires BY NAME (conformance.md § PROBE-097, arm (a)). A
// [SemanticSilentCase] tagged with one counts towards that requirement, and
// [Probe097SemanticLint] fails a corpus in which any of the three is unclaimed.
type MandatoryNegative string

const (
	// NegUnknownClassSuppression — an unknown operand suppresses the pair
	// checks on both sides of it.
	NegUnknownClassSuppression MandatoryNegative = "unknown-class suppression"
	// NegArchetypeMismatchSuppression — a literal archetype predicate whose
	// declared class or HRID type segment is unknown yields
	// aql_unknown_rm_class only, never aql_archetype_class_mismatch.
	NegArchetypeMismatchSuppression MandatoryNegative = "archetype-mismatch suppression"
	// NegFanoutConservativeFiring — the aql_fanout_row_grain advisory's own
	// conservative firing rule (it needs >= 2 projected leaves).
	NegFanoutConservativeFiring MandatoryNegative = "fan-out conservative firing rule"
)

// mandatoryNegativesAll is the set arm (a)'s silence completeness guard
// enumerates. A package-level var for the same reason as [semanticCodesAll].
var mandatoryNegativesAll = []MandatoryNegative{
	NegUnknownClassSuppression, NegArchetypeMismatchSuppression, NegFanoutConservativeFiring,
}

// SemanticFireCase is one PROBE-097 arm-(a) firing row: Query MUST raise
// exactly one REQ-161 code — Code, at Severity — spanned on the SpanNth
// (1-based) occurrence of SpanClass in Query. One row per REQ-161 code is
// the wire assertion's minimum ("at least one corpus query yields it with
// the specified severity at the span of the offending construct").
type SemanticFireCase struct {
	// Name labels the case for diagnostic output.
	Name string
	// Query is the AQL string under test; assumed single-line (span
	// computation does not handle multi-line queries).
	Query string
	// Code is the REQ-161 issue code Query MUST raise, and no other
	// semantic code besides it.
	Code string
	// Severity is Code's REQ-161 catalogue severity.
	Severity lint.Severity
	// SpanClass is the RM class token the issue's Span MUST cover.
	SpanClass string
	// SpanNth is the 1-based occurrence of SpanClass in Query the Span
	// MUST land on.
	SpanNth int
}

// SemanticSilentCase is one PROBE-097 arm-(a) silence row: Query MUST reach
// the REQ-161 checks at all, and MUST then raise exactly the REQ-161 code
// multiset in Want.
//
// A plain near miss leaves Want nil. The two SUPPRESSION negatives — the
// unknown-class suppression and the archetype-mismatch suppression — set Want
// to the specific code the suppression rule still permits, e.g.
// []string{"aql_unknown_rm_class"} for a pair the unknown operand suppresses.
// The third mandatory negative, the fan-out advisory's own conservative near
// miss, is not a suppression and leaves Want nil like a plain near miss:
// nothing survives that near miss for the row to assert.
type SemanticSilentCase struct {
	// Name labels the case for diagnostic output.
	Name string
	// Query is the AQL string under test.
	Query string
	// Want is the exact REQ-161 code multiset Query MUST raise (order
	// irrelevant; nil means none).
	Want []string
	// ForCode, when set, declares which REQ-161 code's silence this row
	// guards. Every code in [semanticCodes] MUST be claimed by at least one
	// row — conformance.md § PROBE-097 arm (a) requires, per code, a firing
	// row AND "at least one near-miss query [that] stays silent". A row that
	// exists for some other reason leaves it empty.
	ForCode string
	// Mandatory, when set, declares this row as one of the three negatives
	// the PROBE-097 wire assertion names explicitly. Each of
	// [MandatoryNegative]'s three constants MUST be claimed by at least one
	// row. A row that is not one of the three leaves it empty.
	Mandatory MandatoryNegative
}

// ParityCase is one PROBE-097 arm-(c) row (REQ-162 § Contract): Build MUST
// produce a *aql.Builder whose emitted query, run through
// [aql.Builder.VerifyContainment] and through [lint.LintString], agrees on
// the [containmentCodes] subset.
type ParityCase struct {
	// Name labels the case for diagnostic output.
	Name string
	// Build constructs the builder tree under test; called once per run.
	Build func() *aql.Builder
}

// SemanticCorpus is the whole PROBE-097 corpus: four fields across three
// wire-assertion arms — Fire and Silent are the two halves of arm (a), the
// firing rows and the near misses that must stay silent. All four are
// required.
//
// Additivity reuses [LintCase] (probe_028_aql_lint.go, same package): arm (b)
// is PROBE-028's own corpus re-run under the completed REQ-161 linter, not a
// corpus of its own, so the case shape it needs — an optional OPT, a query,
// the full expected code multiset — is already exactly LintCase's.
type SemanticCorpus struct {
	// Fire is arm (a)'s firing rows — one per REQ-161 code, minimum.
	Fire []SemanticFireCase
	// Silent is arm (a)'s near-miss and suppression-negative rows.
	Silent []SemanticSilentCase
	// Additivity is arm (b): the PROBE-028 corpus, re-run.
	Additivity []LintCase
	// Parity is arm (c): the REQ-162 § Contract read/write agreement.
	Parity []ParityCase
}

// Probe097SemanticLint runs every row of every arm and aggregates all
// failures into one [Result] (collect-all, like [Probe028AQLLint] — a single
// early failure would hide the rest of the corpus from the report).
func Probe097SemanticLint(c SemanticCorpus) (Result, error) {
	r := Result{Probe: "PROBE-097"}
	if len(c.Fire) == 0 || len(c.Silent) == 0 || len(c.Additivity) == 0 || len(c.Parity) == 0 {
		return r, errors.New("PROBE-097: all four corpus fields (Fire, Silent, Additivity, Parity) are required")
	}

	var failures []string
	fireCodes := map[string]bool{}
	for _, tc := range c.Fire {
		if msg := runSemanticFire(tc); msg != "" {
			failures = append(failures, fmt.Sprintf("fire/%s: %s", tc.Name, msg))
		}
		fireCodes[tc.Code] = true
	}
	// Completeness guard, mirroring the arm-(c) parity guard below: a Fire
	// row deleted from the corpus (or never added for a new REQ-161 code)
	// leaves this probe green as long as the surviving rows still pass on
	// their own — len(c.Fire) == 0 above only catches an EMPTY corpus, not
	// one missing a member of [semanticCodes]. SemanticFireCase's own doc
	// calls "one row per REQ-161 code" the wire assertion's minimum, so this
	// is that minimum, enforced.
	for _, code := range semanticCodes() {
		if !fireCodes[code] {
			failures = append(failures, fmt.Sprintf("fire: no fire case raises %s; the corpus does not exercise it", code))
		}
	}
	silentCodes := map[string]bool{}
	silentNegatives := map[MandatoryNegative]bool{}
	for _, tc := range c.Silent {
		if msg := runSemanticSilent(tc); msg != "" {
			failures = append(failures, fmt.Sprintf("silent/%s: %s", tc.Name, msg))
		}
		if tc.ForCode != "" {
			silentCodes[tc.ForCode] = true
		}
		if tc.Mandatory != "" {
			silentNegatives[tc.Mandatory] = true
		}
	}
	// The mirror of the fire completeness guard above, for the half of arm (a)
	// that asserts SILENCE — the half that can go dark quietly, since a deleted
	// near miss leaves every surviving row passing on its own. conformance.md
	// § PROBE-097 arm (a) requires BOTH halves per code ("at least one corpus
	// query yields it … and at least one near-miss query stays silent") and
	// names three negatives explicitly, so both are enforced rather than
	// assumed. len(c.Silent) == 0 above only catches an EMPTY arm.
	for _, code := range semanticCodes() {
		if !silentCodes[code] {
			failures = append(failures, fmt.Sprintf("silent: no silence case guards %s; the corpus does not pin its near miss", code))
		}
	}
	for _, neg := range mandatoryNegativesAll {
		if !silentNegatives[neg] {
			failures = append(failures, fmt.Sprintf("silent: no silence case pins the %s negative; the PROBE-097 wire assertion names it explicitly", neg))
		}
	}
	for _, tc := range c.Additivity {
		// runLintCase (probe_028_aql_lint.go) is the additivity guard itself:
		// it asserts the FULL pre-REQ-161 code multiset is unchanged, and any
		// REQ-161 code appearing on one of these three cassettes — which carry
		// no REQ-161 defect — already breaks that equality (REQ-161
		// § Additivity's whole point). The task-brief ruling that a gained
		// REQ-161 code is a blocker, never a re-baseline, is a reporting
		// instruction for whoever reads this failure, not a runtime branch:
		// WantCodes is the caller's baseline, and this loop never moves it.
		// (A LATER requirement may re-baseline that table deliberately — see
		// probe028Cases, which REQ-164 § Additivity moved — which is a change
		// to what the caller passes in, not to what this guard does.)
		if msg := runLintCase(tc); msg != "" {
			failures = append(failures, fmt.Sprintf("additivity/%s: %s", tc.Name, msg))
		}
	}
	// The parity loop also feeds the non-vacuity guard below: it is not enough
	// for every row to compare equal if both sides can be equal by being
	// empty. TestReadWriteParityIsNotVacuous (openehr/aql/verify_parity_test.go)
	// is the unit-level precedent for this exact guard; arm (c) had no
	// analogue, so its clean rows — the "near miss" ones — were free to be
	// vacuous: green under a mutation that deleted the write-side check
	// entirely, for the wrong reason (both sides empty, not both sides
	// agreeing on a real finding). No row tally is quoted here on purpose:
	// Parity is caller-supplied, so any count would describe one particular
	// corpus rather than the contract, and rot the day that corpus grew.
	writeUnion := map[string]bool{}
	cleanRows := 0
	for _, tc := range c.Parity {
		msg, write, measured := runParity(tc)
		if msg != "" {
			failures = append(failures, fmt.Sprintf("parity/%s: %s", tc.Name, msg))
		}
		if !measured {
			continue
		}
		if len(write) == 0 {
			cleanRows++
		}
		for _, code := range write {
			writeUnion[code] = true
		}
	}
	for _, code := range containmentCodes() {
		if !writeUnion[code] {
			failures = append(failures, fmt.Sprintf("parity: no parity case raises %s; the corpus does not exercise it", code))
		}
	}
	if cleanRows == 0 {
		failures = append(failures, "parity: no parity case is clean; the corpus cannot show a false positive")
	}

	if len(failures) > 0 {
		r.Status = "fail"
		r.Detail = strings.Join(failures, "; ")
		return r, nil
	}
	r.Status = "pass"
	return r, nil
}

// runSemanticFire runs one arm-(a) firing row.
func runSemanticFire(tc SemanticFireCase) string {
	res := lint.LintString(tc.Query, nil)
	got := filterCodes(res, semanticCodes())
	if !slices.Equal(got, []string{tc.Code}) {
		return fmt.Sprintf("semantic codes = %v, want exactly [%s] (query %q)", got, tc.Code, tc.Query)
	}
	idx := slices.IndexFunc(res.Issues, func(i lint.Issue) bool { return i.Code == tc.Code })
	iss := res.Issues[idx]
	if iss.Severity != tc.Severity {
		return fmt.Sprintf("%s severity = %v, want %v", tc.Code, iss.Severity, tc.Severity)
	}
	want, err := classSpan(tc.Query, tc.SpanClass, tc.SpanNth)
	if err != nil {
		return err.Error()
	}
	if iss.Span != want {
		return fmt.Sprintf("%s span = %+v, want %+v (on %s)", tc.Code, iss.Span, want, tc.SpanClass)
	}
	if iss.Detail == "" {
		return tc.Code + " carries no Detail"
	}
	return ""
}

// runSemanticSilent runs one arm-(a) silence row.
//
// Silence is proved, not assumed: a Layer-1 failure is rejected BEFORE the
// comparison, because [filterCodes] drops aql_syntax / aql_empty (neither is a
// semantic code) and a Want-nil row over an unparseable query would otherwise
// compare nil against nil and pass vacuously. The firing arm needs no such
// check — it demands a code, which a Layer-1 result cannot supply — and arm (b)
// needs none either, since runLintCase asserts the full multiset, Layer-1 codes
// included.
func runSemanticSilent(tc SemanticSilentCase) string {
	res := lint.LintString(tc.Query, nil)
	if code := layer1Code(res); code != "" {
		return fmt.Sprintf("query never reached the REQ-161 checks (%s): a silence row MUST assert silence on a query that actually linted, not on one Layer 1 rejected (query %q)",
			code, tc.Query)
	}
	got := filterCodes(res, semanticCodes())
	if !slices.Equal(got, sortedCopy(tc.Want)) {
		return fmt.Sprintf("semantic codes = %v, want %v (query %q)", got, tc.Want, tc.Query)
	}
	return ""
}

// runParity runs one arm-(c) row (REQ-162 § Contract): build → emit →
// compare [aql.Builder.VerifyContainment] against [lint.LintString]'s
// [containmentCodes] subset, mirroring the comparison discipline of
// openehr/aql/verify_parity_test.go's own TestReadWriteParity.
//
// It fails closed rather than panicking (REQ-025) on a caller-supplied nil
// Build field or a Build that returns a nil *aql.Builder: ParityCase is
// exported API, and unlike [aql.Builder.VerifyContainment],
// [aql.Builder.Build] has no nil-receiver guard of its own.
//
// measured reports whether write is a real observation (Build ran and
// produced a query) rather than the zero value of a guard failure — the
// caller's non-vacuity accounting must not count an unmeasured row as
// "clean".
func runParity(tc ParityCase) (msg string, write []string, measured bool) {
	if tc.Build == nil {
		return "ParityCase.Build is nil", nil, false
	}
	b := tc.Build()
	if b == nil {
		return "Build() returned a nil *aql.Builder", nil, false
	}
	q, err := b.Build()
	if err != nil {
		return fmt.Sprintf("Build() = %v — every parity case must be buildable", err), nil, false
	}
	write = findingCodes(b.VerifyContainment(nil))
	read := filterCodes(lint.LintString(q.Q, nil), containmentCodes())
	if !slices.Equal(write, read) {
		return fmt.Sprintf("code multisets diverge for %q:\n  write (VerifyContainment) = %v\n  read  (LintString)        = %v",
			q.Q, write, read), write, true
	}
	return "", write, true
}

// filterCodes is the sorted multiset of r's issue codes restricted to set.
func filterCodes(r lint.Result, set []string) []string {
	var out []string
	for _, i := range r.Issues {
		if slices.Contains(set, i.Code) {
			out = append(out, i.Code)
		}
	}
	slices.Sort(out)
	return out
}

// findingCodes is the sorted code multiset of fs — order-irrelevant,
// duplicates count, mirroring openehr/aql/verify_test.go's helper of the same
// name in its own package.
func findingCodes(fs []contain.Finding) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Code)
	}
	slices.Sort(out)
	return out
}

// classSpan is the span the nth (1-based) occurrence of an RM class token
// occupies in a single-line query — mirrors openehr/aql/lint's own
// semantic_test.go helper of the same purpose. Computed from the source text
// rather than from the implementation under test, so a span that silently
// widened to the whole clause fails.
//
// The occurrence search is boundary-checked (see [boundaryOK]), not a
// token-aware parse: it skips a candidate match embedded inside a longer
// class-token or archetype-HRID segment (e.g. "OBSERVATION" occurring inside
// "openEHR-EHR-OBSERVATION.blood_pressure.v1" earlier in the query), so a
// future corpus row naming a short class token cannot silently anchor on an
// HRID fragment rather than the bare token it means to pin. SpanNth remains
// the corpus author's own guard against two GENUINE class-token occurrences
// of the same name; boundary checking only rules out a partial match.
//
// The final column is computed via a rune conversion (not a byte offset), so
// a query carrying non-ASCII text ahead of the match would still land on the
// right column — every corpus query today is ASCII, so that path is correct
// but unexercised.
func classSpan(query, rmType string, nth int) (lint.Span, error) {
	if strings.Contains(query, "\n") {
		return lint.Span{}, fmt.Errorf("classSpan assumes a single-line query, got %q", query)
	}
	if nth < 1 {
		return lint.Span{}, fmt.Errorf("classSpan: nth is 1-based, got %d", nth)
	}
	idx := -1
	count, from := 0, 0
	for {
		i := strings.Index(query[from:], rmType)
		if i < 0 {
			break
		}
		start := from + i
		end := start + len(rmType)
		from = start + 1 // advance by one byte, not len(rmType), so an overlapping candidate is still considered
		if !boundaryOK(query, start, end) {
			continue
		}
		count++
		if count == nth {
			idx = start
			break
		}
	}
	if idx < 0 {
		return lint.Span{}, fmt.Errorf("query %q has fewer than %d boundary-matched occurrences of %q", query, nth, rmType)
	}
	col := len([]rune(query[:idx])) + 1
	return lint.Span{
		Start: parse.Position{Line: 1, Col: col},
		End:   parse.Position{Line: 1, Col: col + len([]rune(rmType))},
	}, nil
}

// boundaryOK reports whether the candidate occurrence s[start:end] is a
// standalone token rather than a fragment embedded in a longer one: neither
// the byte immediately before start nor the byte immediately at end may be
// an [identifierByte] — a class-token or archetype-HRID constituent.
func boundaryOK(s string, start, end int) bool {
	if start > 0 && identifierByte(s[start-1]) {
		return false
	}
	if end < len(s) && identifierByte(s[end]) {
		return false
	}
	return true
}

// identifierByte reports whether b can appear inside an RM class token or an
// archetype HRID segment: letters, digits, underscore, and the HRID
// separators hyphen and period.
func identifierByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	case b == '_' || b == '-' || b == '.':
		return true
	}
	return false
}
