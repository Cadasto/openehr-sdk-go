package parse_test

// REQ-119 · PROBE-090 — issue #99.
//
// predicate_guard_test.go holds the EMISSION side of the bracket positions:
// [parse.ClassExpr.Predicate] is spliced verbatim, so text that terminates the
// bracket early re-parses as a DIFFERENT query with no error — REQ-119's
// silent-substitution class, the only failure mode that justifies refusing an
// operand which may itself be valid AQL.
//
// The extraction side (whitespace fidelity, round-trip identity) lives in
// predicate_parity_test.go.
//
// One struct field carries TWO grammar positions, and they do not have the same
// accept set:
//
//	classExprOperand : IDENTIFIER variable=IDENTIFIER? pathPredicate?
//	                 | VERSION variable=IDENTIFIER? ('[' versionPredicate ']')?
//	pathPredicate    : '[' (standardPredicate | archetypePredicate | nodePredicate) ']'
//	versionPredicate : LATEST_VERSION | ALL_VERSIONS | standardPredicate
//
// `versionPredicate` admits NO node predicate, so a guard that treats the field
// uniformly is wrong in both directions at once: it lets `VERSION v[at0001]`
// through (the parser rejects it) and it refuses `VERSION v[LATEST_VERSION]`
// (which the extractor itself produces). Both directions are asserted here.

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// emitWithPredicate parses base, overwrites the FROM root's standing predicate
// with text, and re-emits — the direct-construction path a consumer reaches by
// assembling or rewriting an AST by hand, which is what these guards bind.
func emitWithPredicate(t *testing.T, base, text string) (string, error) {
	t.Helper()
	q, err := parse.ParseQuery(base)
	if err != nil {
		t.Fatalf("ParseQuery(%q) = %v", base, err)
	}
	q.From.Root.Predicate = text
	q.From.Root.PredicateComparison = nil
	return q.Emit()
}

// TestEmitVersionPredicateHoldsItsOwnSubGrammar is the position split: the
// VERSION bracket is `versionPredicate`, not `pathPredicate`.
func TestEmitVersionPredicateHoldsItsOwnSubGrammar(t *testing.T) {
	const base = "SELECT v/data FROM VERSION v[LATEST_VERSION]"

	t.Run("accepts every versionPredicate alternative", func(t *testing.T) {
		// The POSITIVE CONTROL REQ-119 requires: this fails the moment the
		// class-position rule is applied here, which is the tightening
		// failure the REQ guards against as squarely as the splice.
		for _, ok := range []string{
			"LATEST_VERSION",
			"ALL_VERSIONS",
			// Case-insensitive: the lexer builds both keywords out of
			// case-insensitive letter fragments (L A T E S T '_' …).
			"latest_version",
			"all_versions",
			"Latest_Version",
			// standardPredicate : objectPath COMPARISON_OPERATOR pathPredicateOperand
			"commit_audit/time_committed > '2020'",
			"commit_audit/time_committed>'2020'",
			"commit_audit/committer/name = $who",
		} {
			out, err := emitWithPredicate(t, base, ok)
			if err != nil {
				t.Errorf("Emit with VERSION predicate %q = %v; want it accepted", ok, err)
				continue
			}
			if _, err := parse.ParseQuery(out); err != nil {
				t.Errorf("emitted %q does not re-parse: %v", out, err)
			}
		}
	})

	t.Run("refuses a node predicate the position cannot carry", func(t *testing.T) {
		// `versionPredicate` has no nodePredicate alternative, so these emit
		// text the SDK's own parser rejects — REQ-119's LOUD class, which the
		// closure property still forbids ("ParseQuery of the emitted text
		// MUST succeed").
		for _, bad := range []string{
			"at0001",
			"at0001,'name'",
			"$p",
			"openEHR-EHR-COMPOSITION.encounter.v1",
		} {
			out, err := emitWithPredicate(t, base, bad)
			if err == nil {
				t.Errorf("Emit with VERSION predicate %q = %q, want an error; "+
					"versionPredicate admits only LATEST_VERSION, ALL_VERSIONS and standardPredicate", bad, out)
				continue
			}
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("Emit with VERSION predicate %q: error %v does not wrap ErrInvalidQuery", bad, err)
			}
		}
	})
}

// TestEmitVersionPredicateRefusalNamesThePosition keeps the diagnostic useful:
// a caller who wrote a node predicate needs to be told the VERSION bracket is a
// different position, not merely that something was invalid.
func TestEmitVersionPredicateRefusalNamesThePosition(t *testing.T) {
	_, err := emitWithPredicate(t, "SELECT v/data FROM VERSION v[LATEST_VERSION]", "at0001")
	if err == nil {
		t.Fatal("want an error for a node predicate in the VERSION position")
	}
	if !strings.Contains(err.Error(), "VERSION") {
		t.Errorf("error %q does not mention the VERSION position", err)
	}
}
