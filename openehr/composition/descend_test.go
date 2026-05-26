package composition

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// TestDescendOne_PredicateMismatchErrors pins the PR #19 review fix:
// when a multi-attribute holds siblings whose archetype_node_id
// values are all known but NONE matches the path's predicate,
// descendOne now surfaces ErrInvalidPath instead of silently falling
// through to items[0]. Tests the unexported function directly because
// the failure scenario requires a runtime tree whose stamped ids
// diverge from the OPT-declared predicate — a state Builder.Set
// cannot reach end-to-end (NodeAt rejects OPT-side predicates that
// don't match any compiled child).
func TestDescendOne_PredicateMismatchErrors(t *testing.T) {
	// Hand-built multi-attribute: a Composition.content with two
	// observation children whose archetype-ids are present but
	// neither matches the requested predicate.
	parent := &rm.Composition{
		Content: []rm.ContentItem{
			&rm.Observation{ArchetypeNodeID: "openEHR-EHR-OBSERVATION.heart_rate.v1"},
			&rm.Observation{ArchetypeNodeID: "openEHR-EHR-OBSERVATION.respiration.v1"},
		},
	}
	seg := pathSegment{
		attrName:    "content",
		cardinality: template.Multiple,
		matchID:     "openEHR-EHR-OBSERVATION.blood_pressure.v1",
	}
	_, err := descendOne(parent, seg)
	if !errors.Is(err, ErrInvalidPath) {
		t.Errorf("expected ErrInvalidPath on predicate mismatch, got %v", err)
	}
	// PR #19 review optional nit: assert the diagnostic carries the
	// requested predicate AND the actual sibling ids so callers can
	// debug from a single error line. Exercises siblingIDs output.
	msg := err.Error()
	if !descendContains(msg, "openEHR-EHR-OBSERVATION.blood_pressure.v1") {
		t.Errorf("error missing requested predicate: %v", err)
	}
	if !descendContains(msg, "openEHR-EHR-OBSERVATION.heart_rate.v1") || !descendContains(msg, "openEHR-EHR-OBSERVATION.respiration.v1") {
		t.Errorf("error missing one of the sibling ids: %v", err)
	}
}

// descendContains is a local micro-helper to keep this internal
// test self-contained (no strings.Contains import).
func descendContains(haystack, needle string) bool {
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// TestDescendOne_PredicateMatchSucceeds is the positive twin — when
// a sibling's archetype_node_id matches the predicate, descendOne
// returns that sibling without scanning others.
func TestDescendOne_PredicateMatchSucceeds(t *testing.T) {
	target := &rm.Observation{ArchetypeNodeID: "openEHR-EHR-OBSERVATION.blood_pressure.v1"}
	parent := &rm.Composition{
		Content: []rm.ContentItem{
			&rm.Observation{ArchetypeNodeID: "openEHR-EHR-OBSERVATION.heart_rate.v1"},
			target,
		},
	}
	seg := pathSegment{
		attrName:    "content",
		cardinality: template.Multiple,
		matchID:     "openEHR-EHR-OBSERVATION.blood_pressure.v1",
	}
	got, err := descendOne(parent, seg)
	if err != nil {
		t.Fatalf("descendOne: %v", err)
	}
	if got != target {
		t.Errorf("descendOne returned wrong sibling (got %p, want %p)", got, target)
	}
}

// TestDescendOne_NoPredicateReturnsFirst confirms the convenience
// path — when the segment carries no predicate (matchID == ""),
// descendOne picks items[0] (NodeAt narrows by structure when no
// predicate is supplied; the runtime walker does the same).
func TestDescendOne_NoPredicateReturnsFirst(t *testing.T) {
	first := &rm.Observation{ArchetypeNodeID: "openEHR-EHR-OBSERVATION.heart_rate.v1"}
	parent := &rm.Composition{
		Content: []rm.ContentItem{
			first,
			&rm.Observation{ArchetypeNodeID: "openEHR-EHR-OBSERVATION.respiration.v1"},
		},
	}
	seg := pathSegment{
		attrName:    "content",
		cardinality: template.Multiple,
	}
	got, err := descendOne(parent, seg)
	if err != nil {
		t.Fatalf("descendOne: %v", err)
	}
	if got != first {
		t.Errorf("descendOne returned wrong sibling without predicate (got %p, want %p)", got, first)
	}
}
