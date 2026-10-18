package composition

import (
	"errors"
	"strings"
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
	if !strings.Contains(msg, "openEHR-EHR-OBSERVATION.blood_pressure.v1") {
		t.Errorf("error missing requested predicate: %v", err)
	}
	if !strings.Contains(msg, "openEHR-EHR-OBSERVATION.heart_rate.v1") || !strings.Contains(msg, "openEHR-EHR-OBSERVATION.respiration.v1") {
		t.Errorf("error missing one of the sibling ids: %v", err)
	}
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

// TestDescendOne_NamePredicatePicksTheNamedSibling checks that a segment
// carrying a name returns, of the siblings that share its id, the one
// whose name value is that name, skipping one without a name; and that a
// name no sibling carries fails with ErrInvalidPath naming the predicate
// with its name, rather than routing to a sibling of another name.
func TestDescendOne_NamePredicatePicksTheNamedSibling(t *testing.T) {
	target := &rm.Element{ArchetypeNodeID: "at0002", Name: &rm.DVCodedText{Value: "B"}}
	parent := &rm.ItemTree{
		Items: []rm.Item{
			&rm.Element{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "A"}},
			&rm.Element{ArchetypeNodeID: "at0002"},
			target,
		},
	}
	seg := pathSegment{attrName: "items", cardinality: template.Multiple, matchID: "at0002", matchName: "B"}
	got, err := descendOne(parent, seg)
	if err != nil {
		t.Fatalf("descendOne(%+v): %v", seg, err)
	}
	if got != target {
		t.Errorf("descendOne(%+v) returned %+v, want the sibling named B", seg, got)
	}

	seg.matchName = "C"
	if _, err := descendOne(parent, seg); !errors.Is(err, ErrInvalidPath) {
		t.Errorf("descendOne(%+v) = %v, want ErrInvalidPath", seg, err)
	} else if !strings.Contains(err.Error(), `"at0002,'C'"`) {
		t.Errorf("descendOne(%+v) error %q does not name the predicate at0002,'C'", seg, err)
	}
}

// TestLocatableName checks that locatableName reads the name value of a
// LOCATABLE, plain or coded, and answers "" without panicking for a value
// that is no LOCATABLE, a nil pointer to one, and one whose name is
// absent or a nil pointer.
func TestLocatableName(t *testing.T) {
	cases := []struct {
		name string
		v    any
		want string
	}{
		{name: "DV_TEXT name", v: &rm.Element{Name: rm.DVText{Value: "A"}}, want: "A"},
		{name: "DV_CODED_TEXT name", v: &rm.Cluster{Name: &rm.DVCodedText{Value: "B"}}, want: "B"},
		{name: "not a LOCATABLE", v: &rm.DVText{Value: "A"}},
		{name: "nil pointer to a LOCATABLE", v: (*rm.Element)(nil)},
		{name: "absent name", v: &rm.Element{}},
		{name: "nil pointer name", v: &rm.Element{Name: (*rm.DVText)(nil)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := locatableName(tc.v); got != tc.want {
				t.Errorf("locatableName(%#v) = %q, want %q", tc.v, got, tc.want)
			}
		})
	}
}

// TestArchetypeNodeID checks that archetypeNodeID reads the
// archetype_node_id of every LOCATABLE a composition path can descend
// through, and answers "" without panicking for a value that is no
// LOCATABLE and for a nil pointer to one.
func TestArchetypeNodeID(t *testing.T) {
	const id = "at0001"
	cases := []struct {
		name string
		v    any
		want string
	}{
		{name: "COMPOSITION", v: &rm.Composition{ArchetypeNodeID: id}, want: id},
		{name: "OBSERVATION", v: &rm.Observation{ArchetypeNodeID: id}, want: id},
		{name: "EVALUATION", v: &rm.Evaluation{ArchetypeNodeID: id}, want: id},
		{name: "INSTRUCTION", v: &rm.Instruction{ArchetypeNodeID: id}, want: id},
		{name: "ACTION", v: &rm.Action{ArchetypeNodeID: id}, want: id},
		{name: "ADMIN_ENTRY", v: &rm.AdminEntry{ArchetypeNodeID: id}, want: id},
		{name: "GENERIC_ENTRY", v: &rm.GenericEntry{ArchetypeNodeID: id}, want: id},
		{name: "SECTION", v: &rm.Section{ArchetypeNodeID: id}, want: id},
		{name: "ACTIVITY", v: &rm.Activity{ArchetypeNodeID: id}, want: id},
		{name: "HISTORY", v: &rm.History[rm.ItemStructure]{ArchetypeNodeID: id}, want: id},
		{name: "POINT_EVENT", v: &rm.PointEvent[rm.ItemStructure]{ArchetypeNodeID: id}, want: id},
		{name: "INTERVAL_EVENT", v: &rm.IntervalEvent[rm.ItemStructure]{ArchetypeNodeID: id}, want: id},
		{name: "ITEM_TREE", v: &rm.ItemTree{ArchetypeNodeID: id}, want: id},
		{name: "ITEM_LIST", v: &rm.ItemList{ArchetypeNodeID: id}, want: id},
		{name: "ITEM_SINGLE", v: &rm.ItemSingle{ArchetypeNodeID: id}, want: id},
		{name: "ITEM_TABLE", v: &rm.ItemTable{ArchetypeNodeID: id}, want: id},
		{name: "CLUSTER", v: &rm.Cluster{ArchetypeNodeID: id}, want: id},
		{name: "ELEMENT", v: &rm.Element{ArchetypeNodeID: id}, want: id},
		{name: "not a LOCATABLE", v: &rm.DVText{Value: id}},
		{name: "nil", v: nil},
		{name: "nil pointer to a LOCATABLE", v: (*rm.Observation)(nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := archetypeNodeID(tc.v); got != tc.want {
				t.Errorf("archetypeNodeID(%#v) = %q, want %q", tc.v, got, tc.want)
			}
		})
	}
}

// TestDescendOne_SkipsANilSibling checks that a nil pointer among the
// siblings neither panics nor matches: descendOne returns the sibling the
// predicate names, and a predicate no sibling matches fails with
// ErrInvalidPath, listing the nil sibling as having no id.
func TestDescendOne_SkipsANilSibling(t *testing.T) {
	target := &rm.Observation{ArchetypeNodeID: "openEHR-EHR-OBSERVATION.blood_pressure.v1"}
	parent := &rm.Composition{
		Content: []rm.ContentItem{(*rm.Observation)(nil), target},
	}
	seg := pathSegment{
		attrName:    "content",
		cardinality: template.Multiple,
		matchID:     "openEHR-EHR-OBSERVATION.blood_pressure.v1",
	}
	got, err := descendOne(parent, seg)
	if err != nil {
		t.Fatalf("descendOne(%+v): %v", seg, err)
	}
	if got != target {
		t.Errorf("descendOne(%+v) returned %#v, want the blood pressure observation", seg, got)
	}

	seg.matchID = "openEHR-EHR-OBSERVATION.heart_rate.v1"
	if _, err := descendOne(parent, seg); !errors.Is(err, ErrInvalidPath) {
		t.Errorf("descendOne(%+v) = %v, want ErrInvalidPath", seg, err)
	} else if !strings.Contains(err.Error(), "<empty>, openEHR-EHR-OBSERVATION.blood_pressure.v1") {
		t.Errorf("descendOne(%+v) error %q does not list the siblings as <empty>, then the blood pressure id", seg, err)
	}
}
