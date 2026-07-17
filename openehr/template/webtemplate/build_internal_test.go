package webtemplate

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// rangeValidation must treat the zero NumericRange — what childConstraint
// returns when the magnitude/numerator/denominator child carries no
// constraint — as "no validation", not as the impossible interval 0<x<0
// (REQ-106).
func TestRangeValidationUnconstrained(t *testing.T) {
	if v := rangeValidation(constraints.NumericRange{}); v != nil {
		t.Errorf("rangeValidation(zero) = %+v, want nil", v.Range)
	}
	if v := rangeValidation(constraints.NumericRange{LowerUnbounded: true, UpperUnbounded: true}); v != nil {
		t.Errorf("rangeValidation(both unbounded) = %+v, want nil", v.Range)
	}
	// A genuine one-sided bound must still be emitted.
	v := rangeValidation(constraints.NumericRange{Lower: 0, LowerInclusive: true, UpperUnbounded: true})
	if v == nil || v.Range.Min == nil || *v.Range.Min != 0 || v.Range.MinOp != ">=" || v.Range.Max != nil {
		t.Errorf("rangeValidation(>=0) = %+v, want min 0 >=", v)
	}
}

// Build returns a mutable tree for post-processing, so the percent
// denominator's Min and Max must be independent allocations — mutating
// one bound must not move the other (REQ-106).
func TestKindDenominatorValidationBoundsNotAliased(t *testing.T) {
	v := kindDenominatorValidation([]int64{2})
	if v == nil || v.Range.Min == nil || v.Range.Max == nil {
		t.Fatalf("kindDenominatorValidation([2]) = %+v, want a bounded range", v)
	}
	if *v.Range.Min != 100 || *v.Range.Max != 100 {
		t.Fatalf("bounds = [%v,%v], want [100,100]", *v.Range.Min, *v.Range.Max)
	}
	*v.Range.Min = 0
	if *v.Range.Max != 100 {
		t.Errorf("mutating Min changed Max to %v — bounds alias the same pointer", *v.Range.Max)
	}
	// Non-percent kinds derive no bound.
	if got := kindDenominatorValidation([]int64{0}); got != nil {
		t.Errorf("kindDenominatorValidation([0]) = %+v, want nil", got)
	}
}

// The reference normalises exclusive INTEGER bounds to inclusive
// (>10 → >=11, <15 → <=14) for DV_COUNT ranges (REQ-106).
func TestIntRangeValidationNormalisesExclusiveBounds(t *testing.T) {
	v := intRangeValidation(constraints.NumericRange{Lower: 10, Upper: 15, UpperInclusive: true})
	if v == nil || v.Range.Min == nil || *v.Range.Min != 11 || v.Range.MinOp != ">=" {
		t.Errorf("min = %+v, want >=11", v)
	}
	if v.Range.Max == nil || *v.Range.Max != 15 || v.Range.MaxOp != "<=" {
		t.Errorf("max = %+v, want <=15", v)
	}
	v = intRangeValidation(constraints.NumericRange{Lower: 0, LowerInclusive: true, Upper: 5})
	if v == nil || v.Range.Max == nil || *v.Range.Max != 4 || v.Range.MaxOp != "<=" {
		t.Errorf("max = %+v, want <=4", v)
	}
	if v := intRangeValidation(constraints.NumericRange{}); v != nil {
		t.Errorf("intRangeValidation(zero) = %+v, want nil", v)
	}
}

// The archetype root's internal at0000 — including specialized forms like
// at0000.1 — is not a nodeId; the archetype id takes its place (REQ-106).
func TestNodeIDOfExcludesSpecializedRoot(t *testing.T) {
	for _, id := range []string{"at0000", "at0000.1", "at0000.1.1"} {
		if !isArchetypeRootCode(id) {
			t.Errorf("isArchetypeRootCode(%q) = false, want true", id)
		}
	}
	for _, id := range []string{"at0001", "at00001", "at0000x"} {
		if isArchetypeRootCode(id) {
			t.Errorf("isArchetypeRootCode(%q) = true, want false", id)
		}
	}
}

// A punctuation-only display name sanitises to "" and must fall through
// to the attribute-name / RM-type fallbacks instead of emitting an empty
// FLAT-path id (REQ-106).
func TestFirstNonEmptyID(t *testing.T) {
	cases := []struct {
		candidates []string
		want       string
	}{
		{[]string{"Blood pressure", "value", "DV_TEXT"}, "blood_pressure"},
		{[]string{"!!!", "value", "DV_TEXT"}, "value"},         // name sanitises to ""
		{[]string{"", "???", "DV_TEXT"}, "dv_text"},            // attr sanitises to "" too
		{[]string{"  ", "", "DV_CODED_TEXT"}, "dv_coded_text"}, // blanks skipped
		{[]string{"!!!", "  ", ""}, ""},                        // nothing usable
	}
	for _, tc := range cases {
		if got := firstNonEmptyID(tc.candidates...); got != tc.want {
			t.Errorf("firstNonEmptyID(%q) = %q, want %q", tc.candidates, got, tc.want)
		}
	}
}

// Duplicate sibling ids would corrupt FLAT-path binding, the format's
// load-bearing property; until the reference's disambiguation rule is
// implemented, Build must fail loudly instead of emitting duplicates
// (REQ-106, ADR-0014).
func TestCheckIDCollisions(t *testing.T) {
	ok := &Node{ID: "root", Children: []*Node{
		{ID: "comment", Children: []*Node{{ID: "comment"}}}, // same id at different depth is fine
		{ID: "other"},
	}}
	if err := checkIDCollisions(ok); err != nil {
		t.Errorf("no sibling collision, got %v", err)
	}

	dup := &Node{ID: "root", Children: []*Node{
		{ID: "comment", AQLPath: "/content[at0001]"},
		{ID: "comment", AQLPath: "/content[at0002]"},
	}}
	err := checkIDCollisions(dup)
	if !errors.Is(err, ErrIDCollision) {
		t.Fatalf("sibling collision: err = %v, want ErrIDCollision", err)
	}
}
