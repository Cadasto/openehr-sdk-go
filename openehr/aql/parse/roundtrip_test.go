package parse_test

// roundtrip_test.go pins the PROBE-080 Tier 2 round-trip property
// (REQ-113): for any AQL query the parser accepts and the v1 emitter
// catalogue supports, `Emit(ParseQuery(Emit(ParseQuery(x)))) ==
// Emit(ParseQuery(x))` — parse → emit is idempotent on the second
// pass — AND, where the input is already in canonical form, the
// stronger `Emit(ParseQuery(x)) == x` semantic-preservation property
// holds.
//
// The first emit normalises whitespace, keyword casing, optional
// keywords (e.g. ASC default) and clause ordering against the
// canonical write form; the second parse-emit MUST be a fixed point.
// This is the buildable-grammar equivalent of PROBE-020.

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// TestRoundTripIdempotent runs the corpus through parse → emit → parse
// → emit and asserts byte equality across the two emits. Inputs in
// canonical form ALSO satisfy emit==input — see
// [TestRoundTripPreservesCanonicalInput].
func TestRoundTripIdempotent(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		// Baseline shapes
		{"select_from", "SELECT e/ehr_id/value FROM EHR e"},
		{"select_star", "SELECT * FROM EHR e"},
		{"select_distinct", "SELECT DISTINCT e/ehr_id/value FROM EHR e"},
		{"contains_chain", "SELECT c FROM EHR e CONTAINS COMPOSITION c"},
		{"contains_nested", "SELECT o FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o"},
		{"where_eq_param", "SELECT e/ehr_id/value FROM EHR e WHERE e/ehr_id/value = $id"},
		{"where_int_literal", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/data > 100"},
		{"where_and_or", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/x = $a AND o/y = $b"},
		{"where_exists", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE EXISTS o/data"},
		{"where_like", "SELECT p FROM EHR e CONTAINS PERSON p WHERE p/name LIKE 'Dr%'"},
		{"where_matches", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/status MATCHES {'active', 'archived'}"},
		{"order_by_desc", "SELECT e FROM EHR e ORDER BY e/time_created DESC"},
		{"order_by_asc", "SELECT e FROM EHR e ORDER BY e/time_created ASC"},
		{"order_by_multi", "SELECT e FROM EHR e ORDER BY e/x DESC, e/y ASC"},
		{"limit_offset", "SELECT e FROM EHR e LIMIT 50 OFFSET 100"},
		{"limit_only", "SELECT e FROM EHR e LIMIT 10"},

		// Critical-fix shapes (REQ-113 review feedback): each was a
		// silent-drop or invalid-emit case before REQ-113 review.
		{"where_bool_true", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/active = true"},
		{"where_bool_false", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/active = false"},
		{"where_null", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/data = NULL"},
		{"where_datetime", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/time > '2026-01-01T00:00:00'"},
		{"where_date", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/time > '2026-01-01'"},
		{"where_negative_int", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/data > -100"},
		{"where_real", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/data > 1.5"},
		{"where_not", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE NOT o/x = $a"},
		{"not_contains", "SELECT c FROM EHR e CONTAINS COMPOSITION c NOT CONTAINS SECTION s"},
		{"count_star", "SELECT COUNT(*) FROM EHR e"},
		{"count_distinct", "SELECT COUNT(DISTINCT o/data) FROM EHR e CONTAINS OBSERVATION o"},
		{"standing_predicate", "SELECT e/ehr_id/value FROM EHR e[ehr_id/value=$id]"},
		{"archetype_hrid", "SELECT o FROM EHR e CONTAINS COMPOSITION c[openEHR-EHR-COMPOSITION.report.v1] CONTAINS OBSERVATION o"},
		{"param_archetype", "SELECT c FROM EHR e CONTAINS COMPOSITION c[$template]"},
		{"version_predicate", "SELECT v FROM EHR e CONTAINS VERSION v[all_versions]"},
		{"version_latest", "SELECT v FROM EHR e CONTAINS VERSION v[latest_version]"},
		{"limit_param", "SELECT e FROM EHR e LIMIT $rows"},
		{"limit_offset_param", "SELECT e FROM EHR e LIMIT $rows OFFSET $skip"},

		// REQ-117 catalogue closures (PROBE-087): shapes that used to
		// surface aql.ErrIncompleteAST now round-trip.
		{"select_literal_int", "SELECT 1, e/ehr_id/value FROM EHR e"},
		{"select_literal_string_alias", "SELECT 'urgent' AS label FROM EHR e"},
		{"select_literal_real", "SELECT 1.5 FROM EHR e"},
		{"select_literal_bool", "SELECT true FROM EHR e"},
		{"select_star_mixed", "SELECT *, c/uid/value FROM EHR e CONTAINS COMPOSITION c"},
		{"select_star_after_column", "SELECT c/uid/value, * FROM EHR e CONTAINS COMPOSITION c"},
		{"select_count_star_literal_alias", "SELECT COUNT(*), 1, e/x AS a FROM EHR e"},
		{"select_function_args", "SELECT CONCAT('hello', $p, LENGTH(p/name)) FROM EHR e CONTAINS PERSON p"},
		{"select_terminology_lowercase", "SELECT terminology('SNOMED-CT','near','12345') FROM EHR e"},
		{"where_function_lhs", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE LENGTH(o/name/value) > 5"},
		{"where_function_lhs_lowercase", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE length(o/name/value) > 5"},
		{"where_function_rhs", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/x = LENGTH(o/y)"},
		{"where_path_vs_path", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/x = o/data[at0001]/value"},
		{"where_function_args", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE CONCAT('a', $p, LENGTH(o/y)) = 'abc'"},
		{"where_junction_new_operands", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/x = $a AND LENGTH(o/name) > 5 AND o/p = o/q"},
		{"where_or_junction_new_operands", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/p = o/q OR LENGTH(o/name) > 5"},
		{"where_not_over_function_lhs", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE NOT LENGTH(o/name) > 5"},
		{"where_mixed_junction_precedence", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/a = 1 AND (o/b = o/c OR LENGTH(o/d) > 2)"},
		{"where_terminology_rhs", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/code = terminology('SNOMED-CT','near','12345')"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// First pass: parse → emit produces the canonical form.
			q1, err := parse.ParseQuery(tc.in)
			if err != nil {
				t.Fatalf("ParseQuery(%q): %v", tc.in, err)
			}
			emit1, err := q1.Emit()
			if err != nil {
				t.Fatalf("first Emit: %v", err)
			}
			// Second pass: parse the canonical form, emit again. The
			// idempotence property requires byte-equality with emit1.
			q2, err := parse.ParseQuery(emit1)
			if err != nil {
				t.Fatalf("ParseQuery(canonical %q): %v", emit1, err)
			}
			emit2, err := q2.Emit()
			if err != nil {
				t.Fatalf("second Emit: %v", err)
			}
			if emit1 != emit2 {
				t.Errorf("round-trip not idempotent\n  input:    %s\n  emit1:    %s\n  emit2:    %s",
					tc.in, emit1, emit2)
			}
		})
	}
}

// TestRoundTripPreservesCanonicalInput pins the stronger
// semantic-preservation property: when the input is ALREADY in
// canonical form, the first emit equals the input — not just emit1
// equals emit2 (REQ-113 review feedback: idempotence ≠ preservation).
func TestRoundTripPreservesCanonicalInput(t *testing.T) {
	canonical := []string{
		"SELECT e/ehr_id/value FROM EHR e",
		"SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/active = true",
		"SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/data = NULL",
		"SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/time > '2026-01-01T00:00:00'",
		"SELECT c FROM EHR e CONTAINS COMPOSITION c NOT CONTAINS SECTION s",
		"SELECT COUNT(*) FROM EHR e",
		"SELECT COUNT(DISTINCT o/data) FROM EHR e CONTAINS OBSERVATION o",
		"SELECT e/ehr_id/value FROM EHR e[ehr_id/value=$id]",
		"SELECT c FROM EHR e CONTAINS COMPOSITION c[$template]",
		"SELECT v FROM EHR e CONTAINS VERSION v[all_versions]",
		"SELECT e FROM EHR e LIMIT $rows OFFSET $skip",

		// REQ-117 catalogue closures (PROBE-087).
		"SELECT 1, e/ehr_id/value FROM EHR e",
		"SELECT 'urgent' AS label FROM EHR e",
		"SELECT *, c/uid/value FROM EHR e CONTAINS COMPOSITION c",
		"SELECT COUNT(*), 1, e/x AS a FROM EHR e",
		"SELECT CONCAT('hello', $p, LENGTH(p/name)) FROM EHR e CONTAINS PERSON p",
		"SELECT TERMINOLOGY('SNOMED-CT', 'near', '12345') FROM EHR e",
		"SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE LENGTH(o/name/value) > 5",
		"SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/x = o/data[at0001]/value",
		"SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE CONCAT('a', $p, LENGTH(o/y)) = 'abc'",
		"SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/x = $a AND LENGTH(o/name) > 5 AND o/p = o/q",
		"SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/a = 1 AND (o/b = o/c OR LENGTH(o/d) > 2)",
	}
	for _, in := range canonical {
		t.Run(in, func(t *testing.T) {
			q, err := parse.ParseQuery(in)
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}
			out, err := q.Emit()
			if err != nil {
				t.Fatalf("Emit: %v", err)
			}
			if out != in {
				t.Errorf("canonical input not preserved\n  in:  %s\n  out: %s", in, out)
			}
		})
	}
}

// TestFormerCatalogueGapsModelled pins the REQ-117 catalogue closures:
// every input below used to surface aql.ErrIncompleteAST from the v1
// extractor. ParseQuery MUST now model it without a gap, Emit MUST render
// it, and the emission MUST be a fixed point (the PROBE-080 property
// extended to the closed shapes).
// PROBE-087
func TestFormerCatalogueGapsModelled(t *testing.T) {
	cases := []struct {
		name, in string
	}{
		{"primitive_in_select", "SELECT 1 FROM EHR e"},
		{"select_star_mix", "SELECT *, c/uid/value FROM EHR e CONTAINS COMPOSITION c"},
		{"concat_primitive_arg", "SELECT CONCAT('hello', p/name) FROM EHR e CONTAINS PERSON p"},
		{"function_call_where_lhs", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE LENGTH(o/name) > 5"},
		{"path_vs_path", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/x = o/y"},
		{"and_junction_with_function_operand", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/x = $a AND LENGTH(o/name) > 5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := parse.Parse(tc.in)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.in, err)
			}
			if qerr := doc.QueryErr(); qerr != nil {
				t.Fatalf("QueryErr(%q) = %v, want nil (shape is in catalogue per REQ-117)", tc.in, qerr)
			}
			emit1, err := doc.Query().Emit()
			if err != nil {
				t.Fatalf("Emit(%q): %v", tc.in, err)
			}
			q2, err := parse.ParseQuery(emit1)
			if err != nil {
				t.Fatalf("ParseQuery(canonical %q): %v", emit1, err)
			}
			emit2, err := q2.Emit()
			if err != nil {
				t.Fatalf("second Emit: %v", err)
			}
			if emit1 != emit2 {
				t.Errorf("emission not a fixed point\n  input: %s\n  emit1: %s\n  emit2: %s", tc.in, emit1, emit2)
			}
		})
	}
}

// TestParseQuerySurfacesIncompleteAST pins the RESIDUAL catalogue gap after
// REQ-117: an integer literal that cannot be represented in the AST
// (LIMIT / OFFSET beyond Go `int`) still surfaces aql.ErrIncompleteAST on
// ParseQuery and still refuses to render on Emit, rather than silently
// dropping the clause.
// PROBE-087
func TestParseQuerySurfacesIncompleteAST(t *testing.T) {
	cases := []struct {
		name, in, reason string
	}{
		{"limit_overflow", "SELECT e FROM EHR e LIMIT 9223372036854775808", "out of range"},
		{"offset_overflow", "SELECT e FROM EHR e LIMIT 10 OFFSET 9223372036854775808", "out of range"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := parse.ParseQuery(tc.in)
			if err == nil {
				t.Fatalf("ParseQuery(%q): expected ErrIncompleteAST, got nil", tc.in)
			}
			if !errors.Is(err, aql.ErrIncompleteAST) {
				t.Fatalf("ParseQuery error does not wrap ErrIncompleteAST: %v", err)
			}
			if !strings.Contains(err.Error(), tc.reason) {
				t.Errorf("error message does not mention %q: %v", tc.reason, err)
			}
			// Even on a gap, the partial AST should be non-nil so the
			// caller can inspect what survived.
			if q == nil {
				t.Errorf("ParseQuery returned nil *Query on catalogue gap; want best-effort partial AST")
			}
			// Emit on an incomplete AST MUST refuse with the same
			// ErrIncompleteAST so a caller who ignored the parse
			// return cannot accidentally emit semantically wrong
			// AQL (the structural recommendation from the PR #58
			// re-review).
			if _, eerr := q.Emit(); !errors.Is(eerr, aql.ErrIncompleteAST) {
				t.Errorf("Emit on incomplete AST: want ErrIncompleteAST, got %v", eerr)
			}
		})
	}
}

// TestEmitOffsetWithoutLimit guards the emitter against producing AQL
// the grammar rejects on re-parse: `OFFSET n` with no preceding LIMIT.
func TestEmitOffsetWithoutLimit(t *testing.T) {
	q := &parse.Query{
		Select: parse.SelectClause{Items: []parse.SelectItem{{Expr: parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "e"}}}}}},
		From:   parse.FromClause{Root: parse.ClassExpr{RMType: "EHR", Alias: "e"}},
		Offset: parse.IntLimit{N: 100},
	}
	_, err := q.Emit()
	if !errors.Is(err, aql.ErrInvalidQuery) {
		t.Fatalf("Emit OFFSET-without-LIMIT: want ErrInvalidQuery, got %v", err)
	}
	if !strings.Contains(err.Error(), "OFFSET without LIMIT") {
		t.Errorf("error message should mention OFFSET without LIMIT: %v", err)
	}
}

// TestEmitDuplicateAlias guards the emitter against producing AQL with
// duplicate aliases — the symmetric mirror of [aql.Builder.Build]'s
// alias-uniqueness check.
func TestEmitDuplicateAlias(t *testing.T) {
	q := &parse.Query{
		Select: parse.SelectClause{Items: []parse.SelectItem{{Expr: parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "c"}}}}}},
		From: parse.FromClause{
			Root: parse.ClassExpr{RMType: "EHR", Alias: "c"},
			Contains: &parse.Containment{
				Class: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"},
			},
		},
	}
	_, err := q.Emit()
	if !errors.Is(err, aql.ErrInvalidQuery) {
		t.Fatalf("Emit duplicate-alias: want ErrInvalidQuery, got %v", err)
	}
	if !strings.Contains(err.Error(), "duplicate alias") {
		t.Errorf("error message should mention duplicate alias: %v", err)
	}
}

// TestEmitNilQuery covers the nil-query guard.
func TestEmitNilQuery(t *testing.T) {
	var q *parse.Query
	if _, err := q.Emit(); err == nil {
		t.Error("Emit on nil *Query: expected error, got nil")
	}
}
