package parse_test

// roundtrip_test.go pins the SDK-GAP-17 Tier 2 round-trip property
// (REQ-113): for any AQL query the parser accepts and the v1 emitter
// catalogue supports, `Emit(ParseQuery(Emit(ParseQuery(x)))) ==
// Emit(ParseQuery(x))` — parse → emit is idempotent on the second
// pass.
//
// The first emit normalises whitespace, keyword casing, optional
// keywords (e.g. ASC default) and clause ordering against the
// canonical write form; the second parse-emit MUST be a fixed point.
// This is the buildable-grammar equivalent of PROBE-020.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

func TestRoundTripIdempotent(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
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

// TestEmitNilQuery covers the nil-query guard.
func TestEmitNilQuery(t *testing.T) {
	var q *parse.Query
	if _, err := q.Emit(); err == nil {
		t.Error("Emit on nil *Query: expected error, got nil")
	}
}
