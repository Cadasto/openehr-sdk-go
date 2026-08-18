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
		// The keyword-literal SELECT rows pin the EMITTED bytes only; the AST
		// shape behind them (a LiteralExpr, never a PathExpr rooted at a
		// pseudo-alias) is pinned structurally by
		// TestParseQuerySelectKeywordLiteral in query_test.go.
		{"select_literal_bool", "SELECT true FROM EHR e"},
		{"select_literal_bool_false", "SELECT false FROM EHR e"},
		{"select_literal_bool_uppercase", "SELECT TRUE FROM EHR e"},
		{"select_literal_bool_aliased", "SELECT true AS flag FROM EHR e"},
		{"select_literal_bool_mixed", "SELECT true, e/ehr_id/value FROM EHR e"},
		{"select_keyword_path_tail", "SELECT true/nested FROM EHR e"},
		{"order_by_select_alias", "SELECT e/time_created AS score FROM EHR e ORDER BY score DESC"},
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
		{"matches_terminology_operand", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/code MATCHES terminology('SNOMED-CT','near','12345')"},
		{"matches_uri_operand", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/code MATCHES {uri://terminology.hl7.org/CodeSystem/v3-ActCode}"},
		{"matches_value_list_terminology", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/code MATCHES {terminology('SNOMED-CT','near','12345'), 'other'}"},
		{"matches_param_and_literal", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/code MATCHES {$code, 'other'}"},
		{"from_root_or_junction", "SELECT c1 FROM COMPOSITION c1 OR COMPOSITION c2"},
		{"from_root_and_junction", "SELECT c1 FROM COMPOSITION c1 AND COMPOSITION c2"},
		{"from_root_or_chain", "SELECT c1 FROM COMPOSITION c1 OR COMPOSITION c2 OR EHR e"},
		{"from_root_grouped_junction", "SELECT c1 FROM (COMPOSITION c1 OR COMPOSITION c2) AND EHR e"},
		{"from_root_junction_with_contains", "SELECT o FROM COMPOSITION c1 CONTAINS OBSERVATION o OR EHR e"},
		{"from_root_junction_with_predicates", "SELECT c1 FROM COMPOSITION c1[openEHR-EHR-COMPOSITION.report.v1] OR EHR e[ehr_id/value=$id]"},
		{"contains_nested_junction", "SELECT c FROM EHR e CONTAINS (COMPOSITION c OR SECTION s)"},
		{"from_root_junction_where", "SELECT c1 FROM COMPOSITION c1 OR COMPOSITION c2 WHERE c1/uid/value = $id"},
		{"from_root_junction_chain_operand", "SELECT o FROM (COMPOSITION c1 CONTAINS OBSERVATION o) OR EHR e"},
		{"from_root_junction_two_chains", "SELECT o FROM (COMPOSITION c1 CONTAINS OBSERVATION o) AND (EHR e CONTAINS SECTION s)"},
		{"from_root_junction_and_under_or", "SELECT c1 FROM COMPOSITION c1 OR COMPOSITION c2 AND EHR e"},
		{"from_root_junction_grouped_and_under_or", "SELECT c1 FROM (COMPOSITION c1 AND COMPOSITION c2) OR EHR e"},
		{"from_root_junction_negated_chain", "SELECT c1 FROM COMPOSITION c1 OR (COMPOSITION c2 NOT CONTAINS SECTION s)"},
		{"contains_nested_junction_grouped", "SELECT c FROM EHR e CONTAINS ((COMPOSITION c OR SECTION s) AND OBSERVATION o)"},

		// REQ-118: the deprecated `SELECT TOP` row limit, in every shape the
		// `top : TOP INTEGER direction=(FORWARD|BACKWARD)?` production admits,
		// including the §4.4.3-forbidden TOP+LIMIT combination — which the
		// parser and emitter carry faithfully and the lint gate diagnoses.
		{"select_top", "SELECT TOP 5 c/uid/value FROM COMPOSITION c"},
		{"select_top_forward", "SELECT TOP 5 FORWARD c/uid/value FROM COMPOSITION c"},
		{"select_top_backward", "SELECT TOP 5 BACKWARD c/uid/value FROM COMPOSITION c"},
		{"select_top_lower_case", "SELECT top 5 backward c/uid/value FROM COMPOSITION c"},
		{"select_distinct_top", "SELECT DISTINCT TOP 3 c/uid/value FROM COMPOSITION c"},
		{"select_top_star", "SELECT TOP 3 * FROM COMPOSITION c"},
		{"select_top_zero", "SELECT TOP 0 c/uid/value FROM COMPOSITION c"},
		{"select_top_with_limit", "SELECT TOP 5 c/uid/value FROM COMPOSITION c LIMIT 10 OFFSET 2"},
		{"select_top_order_by", "SELECT TOP 5 BACKWARD c/uid/value FROM COMPOSITION c ORDER BY c/uid/value DESC"},
		// REQ-118: a non-canonical literal — the AST carries the canonical
		// value AND the source text, and emission renders the canonical form,
		// so the SECOND emit is the fixed point (emit1 != input here).
		{"select_literal_non_canonical", `SELECT 1.50, "dq", 001 FROM EHR e`},
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
		"SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/code MATCHES TERMINOLOGY('SNOMED-CT', 'near', '12345')",
		"SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/code MATCHES {uri://terminology.hl7.org/CodeSystem/v3-ActCode}",
		"SELECT c1 FROM COMPOSITION c1 OR COMPOSITION c2",
		"SELECT c1 FROM COMPOSITION c1 OR COMPOSITION c2 OR EHR e",
		"SELECT c1 FROM (COMPOSITION c1 OR COMPOSITION c2) AND EHR e",
		"SELECT c FROM EHR e CONTAINS (COMPOSITION c OR SECTION s)",
		"SELECT o FROM (COMPOSITION c1 CONTAINS OBSERVATION o) OR EHR e",
		"SELECT c1 FROM COMPOSITION c1 OR COMPOSITION c2 AND EHR e",
		"SELECT c FROM EHR e CONTAINS ((COMPOSITION c OR SECTION s) AND OBSERVATION o)",
		// A bare boolean keyword projected from SELECT: canonical bytes are the
		// lower-case literal, so a canonical input is preserved verbatim even
		// though the AST now carries an aql.BoolValue rather than a path.
		"SELECT true FROM EHR e",
		"SELECT false FROM EHR e",
		"SELECT true, e/ehr_id/value FROM EHR e",
		// ORDER BY resolving against a SELECT AS alias — the parse→emit tie to
		// the REQ-109/REQ-117 lint acceptance (an AS alias is a legal ORDER BY
		// key and survives the round trip unchanged).
		"SELECT e/time_created AS score FROM EHR e ORDER BY score DESC",
		"SELECT o/data[at0001]/value/magnitude AS score FROM EHR e CONTAINS OBSERVATION o ORDER BY score ASC",

		// REQ-118: canonical `TOP` forms — upper-cased keywords, direction
		// present only when written, emitted between DISTINCT and the
		// projection list.
		"SELECT TOP 5 c/uid/value FROM COMPOSITION c",
		"SELECT TOP 5 FORWARD c/uid/value FROM COMPOSITION c",
		"SELECT TOP 5 BACKWARD c/uid/value FROM COMPOSITION c",
		"SELECT DISTINCT TOP 3 c/uid/value FROM COMPOSITION c",
		"SELECT TOP 3 * FROM COMPOSITION c",
		"SELECT TOP 5 c/uid/value FROM COMPOSITION c LIMIT 10 OFFSET 2",
		"SELECT TOP 5 BACKWARD c/uid/value FROM COMPOSITION c ORDER BY c/uid/value DESC",
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
		{"matches_terminology", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/code MATCHES terminology('SNOMED-CT','near','12345')"},
		{"matches_uri", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/code MATCHES {uri://terminology.hl7.org/CodeSystem/v3-ActCode}"},
		{"from_junction", "SELECT e FROM EHR e OR EHR f"},
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

// TestTopClauseGapClosed is the REPLACEMENT for the retired
// TestParseQuerySurfacesTopClauseGap: REQ-118 moved the deprecated `top`
// production INTO the catalogue, so each input below — every one of which used
// to surface aql.ErrIncompleteAST — must now parse clean, carry the bound, and
// emit as a fixed point. The retired test asserted the opposite; the row limit
// is still never dropped, it is now carried instead of refused.
// REQ-118 · PROBE-087
func TestTopClauseGapClosed(t *testing.T) {
	for _, in := range []string{
		"SELECT TOP 5 c/uid/value FROM COMPOSITION c",
		"SELECT TOP 5 FORWARD c/uid/value FROM COMPOSITION c",
		"SELECT TOP 5 BACKWARD c/uid/value FROM COMPOSITION c",
		"SELECT DISTINCT TOP 5 c/uid/value FROM COMPOSITION c",
	} {
		t.Run(in, func(t *testing.T) {
			doc, err := parse.Parse(in)
			if err != nil {
				t.Fatalf("Parse(%q): %v", in, err)
			}
			if qerr := doc.QueryErr(); qerr != nil {
				t.Fatalf("QueryErr(%q) = %v, want nil (the top clause is in catalogue per REQ-118)", in, qerr)
			}
			q := doc.Query()
			if q.Select.Top == nil {
				t.Fatal("Select.Top = nil; the bound must be carried, never dropped")
			}
			emit1, err := q.Emit()
			if err != nil {
				t.Fatalf("Emit: %v", err)
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
				t.Errorf("emission not a fixed point\n  input: %s\n  emit1: %s\n  emit2: %s", in, emit1, emit2)
			}
		})
	}
}

// TestParseQueryTopClauseOutOfRange pins a TOP count outside Go `int` as the
// RESIDUAL unrepresentable-numeric refusal rather than a top-specific one
// (REQ-118): the bound is refused loudly at parse and on Emit, never truncated
// — a truncated row limit is silent data loss whichever clause carries it.
// REQ-118 · PROBE-087
func TestParseQueryTopClauseOutOfRange(t *testing.T) {
	const in = "SELECT TOP 99999999999999999999 c/uid/value FROM COMPOSITION c"
	q, err := parse.ParseQuery(in)
	if !errors.Is(err, aql.ErrIncompleteAST) {
		t.Fatalf("ParseQuery(%q) error = %v, want ErrIncompleteAST", in, err)
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Errorf("error message does not mention %q: %v", "out of range", err)
	}
	if q == nil {
		t.Fatal("partial AST should be non-nil on a gap")
	}
	if q.Select.Top != nil {
		t.Errorf("Select.Top = %+v, want nil — a refused count must not leave a truncated bound", *q.Select.Top)
	}
	if _, eerr := q.Emit(); !errors.Is(eerr, aql.ErrIncompleteAST) {
		t.Fatalf("Emit error = %v, want ErrIncompleteAST", eerr)
	}
}

// TestEmitUnrenderableTopClause pins the emitter's guards on a hand-built AST
// (REQ-118): the `top` production admits no sign, and a direction outside the
// vocabulary renders as nothing at all — which would emit an UNDIRECTED bound,
// a silent loss. Both refuse, mirroring [aql.Builder]'s build-time guards.
// REQ-118
func TestEmitUnrenderableTopClause(t *testing.T) {
	for name, top := range map[string]*aql.TopClause{
		"negative count":    {N: -1},
		"unknown direction": {N: 5, Dir: aql.TopDir(99)},
	} {
		t.Run(name, func(t *testing.T) {
			q := &parse.Query{
				Select: parse.SelectClause{
					Items: []parse.SelectItem{{Expr: parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "e/ehr_id/value"}}}}},
					Top:   top,
				},
				From: parse.FromClause{Root: parse.ClassExpr{RMType: "EHR", Alias: "e"}},
			}
			if _, err := q.Emit(); !errors.Is(err, aql.ErrInvalidQuery) {
				t.Fatalf("Emit error = %v, want ErrInvalidQuery", err)
			}
		})
	}
}

// TestParseQuerySurfacesIncompleteAST pins the RESIDUAL catalogue gap after
// REQ-117: a numeric literal that cannot be represented in the AST
// (LIMIT / OFFSET beyond Go `int`, integer VALUE beyond int64, REAL /
// scientific REAL beyond float64) still surfaces aql.ErrIncompleteAST on
// ParseQuery and still refuses to render on Emit, rather than silently
// dropping the clause or degrading to +Inf.
// PROBE-087
func TestParseQuerySurfacesIncompleteAST(t *testing.T) {
	cases := []struct {
		name, in, reason string
	}{
		{"limit_overflow", "SELECT e FROM EHR e LIMIT 9223372036854775808", "out of range"},
		{"offset_overflow", "SELECT e FROM EHR e LIMIT 10 OFFSET 9223372036854775808", "out of range"},
		// The same unrepresentable-integer class in a VALUE position: the
		// shared vocabulary's IntValue is an int64, so a wider literal is
		// refused rather than degraded to a float.
		{"select_literal_overflow", "SELECT 99999999999999999999 FROM EHR e", "out of range"},
		{"comparison_literal_overflow", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/x > 99999999999999999999", "out of range"},
		{"matches_literal_overflow", "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/x MATCHES {99999999999999999999}", "out of range"},
		// REAL / scientific REAL beyond float64: same residual 1 class as
		// integer overflow — refuse, never degrade to +Inf (REQ-117 / PROBE-087).
		{"real_overflow", "SELECT e FROM EHR e WHERE e/x > 1e400", "out of range"},
		{"sci_real_overflow", "SELECT 1e400 FROM EHR e", "out of range"},
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

// TestEmitFromRootAndJunction guards a hand-built AST that sets BOTH a
// single FROM root and a root junction: the grammar has no
// `(A OR B) CONTAINS C` form, so the emitter refuses rather than produce
// text the parser rejects (REQ-117).
// PROBE-087
func TestEmitFromRootAndJunction(t *testing.T) {
	q := &parse.Query{
		Select: parse.SelectClause{Items: []parse.SelectItem{{Expr: parse.PathExpr{IdentifiedPath: parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Raw: "c"}}}}}},
		From: parse.FromClause{
			Root: parse.ClassExpr{RMType: "EHR", Alias: "e"},
			Junction: &parse.Containment{
				ChildJoin: parse.ContainsOr,
				Children: []parse.Containment{
					{Class: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c1"}},
					{Class: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c2"}},
				},
			},
		},
	}
	_, err := q.Emit()
	if !errors.Is(err, aql.ErrInvalidQuery) {
		t.Fatalf("Emit root+junction: want ErrInvalidQuery, got %v", err)
	}
	if !strings.Contains(err.Error(), "root junction") {
		t.Errorf("error message should mention the root junction: %v", err)
	}
}

// TestEmitNilQuery covers the nil-query guard.
func TestEmitNilQuery(t *testing.T) {
	var q *parse.Query
	if _, err := q.Emit(); err == nil {
		t.Error("Emit on nil *Query: expected error, got nil")
	}
}
