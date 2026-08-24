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

// semanticCodes is the full REQ-161 catalogue — arm (a)'s universe, and what
// the arm (b) additivity guard checks stays absent from a pre-REQ-161-clean
// query.
func semanticCodes() []string {
	return []string{
		codeImpossibleContainment, codeContainsNotContainable, codeArchetypeClassMismatch,
		codeUnknownRMClass, codeContainmentByReference,
		codeVersionNoPredicate, codeVersionedObjectUnreferenced, codeFanoutRowGrain,
	}
}

// containmentCodes is the REQ-162 § Contract five-code subset arm (c) scopes
// parity to. The three portability advisories above are read-side only and
// never appear from [aql.Builder.VerifyContainment] — REQ-162 § Contract is
// explicit that parity is scoped to this subset, not the full catalogue.
func containmentCodes() []string {
	return []string{
		codeImpossibleContainment, codeContainsNotContainable, codeArchetypeClassMismatch,
		codeUnknownRMClass, codeContainmentByReference,
	}
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

// SemanticSilentCase is one PROBE-097 arm-(a) silence row: Query MUST raise
// exactly the REQ-161 code multiset in Want. A plain near miss leaves Want
// nil; a suppression negative (the unknown-class suppression, the
// archetype-mismatch suppression, and the fan-out advisory's own conservative
// near miss — all three named explicitly by the PROBE-097 wire assertion)
// sets Want to the specific code the suppression rule still permits, e.g.
// []string{"aql_unknown_rm_class"} for a pair the unknown operand suppresses.
type SemanticSilentCase struct {
	// Name labels the case for diagnostic output.
	Name string
	// Query is the AQL string under test.
	Query string
	// Want is the exact REQ-161 code multiset Query MUST raise (order
	// irrelevant; nil means none).
	Want []string
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

// SemanticCorpus is the whole PROBE-097 corpus, one field per wire-assertion
// arm.
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
	for _, tc := range c.Fire {
		if msg := runSemanticFire(tc); msg != "" {
			failures = append(failures, fmt.Sprintf("fire/%s: %s", tc.Name, msg))
		}
	}
	for _, tc := range c.Silent {
		if msg := runSemanticSilent(tc); msg != "" {
			failures = append(failures, fmt.Sprintf("silent/%s: %s", tc.Name, msg))
		}
	}
	for _, tc := range c.Additivity {
		// runLintCase (probe_028_aql_lint.go) is the additivity guard itself:
		// it asserts the FULL pre-REQ-161 code multiset is unchanged, and any
		// REQ-161 code appearing on one of these three cassettes — which carry
		// no REQ-161 defect — already breaks that equality (REQ-161
		// § Additivity's whole point). The task-brief ruling that a gained
		// code is a blocker, never a re-baseline, is a reporting instruction
		// for whoever reads this failure, not a runtime branch: WantCodes
		// below is the pre-REQ-161 baseline, and this loop never moves it.
		if msg := runLintCase(tc); msg != "" {
			failures = append(failures, fmt.Sprintf("additivity/%s: %s", tc.Name, msg))
		}
	}
	for _, tc := range c.Parity {
		if msg := runParity(tc); msg != "" {
			failures = append(failures, fmt.Sprintf("parity/%s: %s", tc.Name, msg))
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
		return fmt.Sprintf("%s carries no Detail", tc.Code)
	}
	return ""
}

// runSemanticSilent runs one arm-(a) silence row.
func runSemanticSilent(tc SemanticSilentCase) string {
	got := filterCodes(lint.LintString(tc.Query, nil), semanticCodes())
	if !slices.Equal(got, sortedCopy(tc.Want)) {
		return fmt.Sprintf("semantic codes = %v, want %v (query %q)", got, tc.Want, tc.Query)
	}
	return ""
}

// runParity runs one arm-(c) row (REQ-162 § Contract): build → emit →
// compare [aql.Builder.VerifyContainment] against [lint.LintString]'s
// [containmentCodes] subset, mirroring the comparison discipline of
// openehr/aql/verify_parity_test.go's own TestReadWriteParity.
func runParity(tc ParityCase) string {
	b := tc.Build()
	q, err := b.Build()
	if err != nil {
		return fmt.Sprintf("Build() = %v — every parity case must be buildable", err)
	}
	write := findingCodes(b.VerifyContainment(nil))
	read := filterCodes(lint.LintString(q.Q, nil), containmentCodes())
	if !slices.Equal(write, read) {
		return fmt.Sprintf("code multisets diverge for %q:\n  write (VerifyContainment) = %v\n  read  (LintString)        = %v",
			q.Q, write, read)
	}
	return ""
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
func classSpan(query, rmType string, nth int) (lint.Span, error) {
	if strings.Contains(query, "\n") {
		return lint.Span{}, fmt.Errorf("classSpan assumes a single-line query, got %q", query)
	}
	if nth < 1 {
		return lint.Span{}, fmt.Errorf("classSpan: nth is 1-based, got %d", nth)
	}
	idx, from := 0, 0
	for range nth {
		i := strings.Index(query[from:], rmType)
		if i < 0 {
			return lint.Span{}, fmt.Errorf("query %q has fewer than %d occurrences of %q", query, nth, rmType)
		}
		idx = from + i
		from = idx + len(rmType)
	}
	col := len([]rune(query[:idx])) + 1
	return lint.Span{
		Start: parse.Position{Line: 1, Col: col},
		End:   parse.Position{Line: 1, Col: col + len([]rune(rmType))},
	}, nil
}
