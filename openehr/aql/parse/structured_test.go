package parse_test

// structured_test.go: PROBE-082 — REQ-113 / SDK-GAP-19. The parser must
// expose the two path-bearing sub-structures as parsed structure, not only
// raw text: a class standing predicate as a {path, op, value} comparison,
// and a WHERE comparison's alias-qualified path as alias + segments — so a
// consumer reads them without re-tokenizing AQL grammar. Round-trip/emit is
// unaffected (the verbatim predicate/path text remains).

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// gap19Query exercises both asks in one parse: a standing class predicate on
// the EHR root (`ehr_id/value=$ehr`) and a WHERE comparison over an
// alias-qualified path (`o/data[at0001]/events[at0006]/value/magnitude`).
const gap19Query = "SELECT o/data[at0001]/events[at0006]/value/magnitude " +
	"FROM EHR e[ehr_id/value=$ehr] " +
	"CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1] " +
	"WHERE o/data[at0001]/events[at0006]/value/magnitude > $threshold"

// TestStandingPredicateStructured is the SDK-GAP-19 Ask #1 case: a class
// standing predicate is readable as a structured comparison, with the
// verbatim text retained for round-trip.
func TestStandingPredicateStructured(t *testing.T) {
	q, err := parse.ParseQuery(gap19Query)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	root := q.From.Root
	if root.PredicateComparison == nil {
		t.Fatalf("EHR standing predicate not structured; Predicate=%q", root.Predicate)
	}
	pc := root.PredicateComparison
	if pc.Path != "ehr_id/value" {
		t.Errorf("PredicateComparison.Path = %q, want ehr_id/value", pc.Path)
	}
	if pc.Op != aql.OpEq {
		t.Errorf("PredicateComparison.Op = %q, want =", pc.Op)
	}
	pv, ok := pc.Val.(aql.ParamValue)
	if !ok || pv.Name != "ehr" {
		t.Errorf("PredicateComparison.Val = %#v, want ParamValue{ehr}", pc.Val)
	}
	if root.Predicate == "" {
		t.Errorf("verbatim Predicate text must remain for round-trip, got empty")
	}
}

// TestWhereComparisonStructuredPath is the Ask #2 case: a WHERE comparison
// carries the structured alias + segments alongside the raw path string.
func TestWhereComparisonStructuredPath(t *testing.T) {
	q, err := parse.ParseQuery(gap19Query)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	cmp, ok := q.Where.(aql.Comparison)
	if !ok {
		t.Fatalf("Where = %T, want aql.Comparison", q.Where)
	}
	if cmp.ParsedPath == nil {
		t.Fatalf("WHERE comparison ParsedPath nil; Path=%q", cmp.Path)
	}
	pp := cmp.ParsedPath
	if pp.Alias != "o" {
		t.Errorf("ParsedPath.Alias = %q, want o", pp.Alias)
	}
	if len(pp.Segments) != 4 {
		t.Fatalf("ParsedPath.Segments len = %d, want 4 (%+v)", len(pp.Segments), pp.Segments)
	}
	if got := pp.Segments[0]; got.Name != "data" || got.Predicate != "at0001" {
		t.Errorf("Segments[0] = %+v, want {data at0001}", got)
	}
	if last := pp.Segments[3]; last.Name != "magnitude" {
		t.Errorf("Segments[3].Name = %q, want magnitude", last.Name)
	}
	if cmp.Path != pp.Raw {
		t.Errorf("Comparison.Path %q != ParsedPath.Raw %q (must agree)", cmp.Path, pp.Raw)
	}
}

// TestClassArchetypePredicateNotComparison confirms a non-comparison class
// predicate (an archetype HRID) is distinguishable: PredicateComparison is
// nil and the HRID lives on Archetype.
func TestClassArchetypePredicateNotComparison(t *testing.T) {
	q, err := parse.ParseQuery("SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	root := q.From.Root
	if root.Archetype != "openEHR-EHR-OBSERVATION.blood_pressure.v1" {
		t.Errorf("Archetype = %q, want the HRID", root.Archetype)
	}
	if root.PredicateComparison != nil {
		t.Errorf("archetype predicate must not yield a comparison; got %#v", root.PredicateComparison)
	}
}

// TestStandingPredicateRoundTrip confirms Ask #1/#2 are additive: emit still
// round-trips, the standing predicate survives a parse→emit→parse cycle, and
// emission is idempotent.
func TestStandingPredicateRoundTrip(t *testing.T) {
	q, err := parse.ParseQuery(gap19Query)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	emitted, err := q.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	q2, err := parse.ParseQuery(emitted)
	if err != nil {
		t.Fatalf("re-parse emitted %q: %v", emitted, err)
	}
	if q2.From.Root.PredicateComparison == nil {
		t.Errorf("standing predicate lost on round-trip; emitted=%q", emitted)
	}
	emitted2, err := q2.Emit()
	if err != nil {
		t.Fatalf("re-emit: %v", err)
	}
	if emitted != emitted2 {
		t.Errorf("emit not idempotent:\n  %q\n  %q", emitted, emitted2)
	}
}
