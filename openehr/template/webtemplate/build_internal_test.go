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
