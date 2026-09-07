package terminology

import (
	"iter"
	"slices"
)

// ID is the TERMINOLOGY_ID.value every group and code set in this package is
// defined in — the openEHR Terminology's own identifier. A DV_CODED_TEXT the
// SDK builds from one of these codes carries it as its terminology id.
const ID = "openehr"

// Concept is one coded entry of a [Group]: the code and its English rubric.
type Concept struct {
	Code   string
	Rubric string
}

// Group is one closed, source-ordered openEHR terminology group — the value
// set an RM invariant such as EVENT_CONTEXT.Setting_valid names. Every group
// is a package-level variable generated from the pin (see openehr_gen.go);
// a nil pointer is inert, with every method reporting absence (REQ-025).
//
// A code's rubric is a property of the group, not of the code: the pin gives
// 532 the rubric "complete" in version lifecycle state and "completed" in
// instruction states. Look codes up on the group that governs them.
type Group struct {
	id, name string
	concepts []Concept
	byCode   map[string]int
	byRubric map[string]int
}

// newGroup builds a group from its openehr_id, its display name and its
// concepts in source order. Only the generated tables call it.
func newGroup(id, name string, concepts []Concept) *Group {
	g := &Group{
		id:       id,
		name:     name,
		concepts: concepts,
		byCode:   make(map[string]int, len(concepts)),
		byRubric: make(map[string]int, len(concepts)),
	}
	for i, c := range concepts {
		g.byCode[c.Code] = i
		g.byRubric[c.Rubric] = i
	}
	return g
}

// ID returns the group's openehr_id, e.g. "audit_change_type".
func (g *Group) ID() string {
	if g == nil {
		return ""
	}
	return g.id
}

// Name returns the group's display name, e.g. "audit change type".
func (g *Group) Name() string {
	if g == nil {
		return ""
	}
	return g.name
}

// Len returns the number of concepts in the group.
func (g *Group) Len() int {
	if g == nil {
		return 0
	}
	return len(g.concepts)
}

// All yields the group's concepts in source order — the order the pin lists
// them in. A nil group yields nothing.
func (g *Group) All() iter.Seq[Concept] {
	if g == nil {
		return func(func(Concept) bool) {}
	}
	return slices.Values(g.concepts)
}

// Has reports whether code is a member of the group. This is the membership
// verdict an RM invariant over the group asks for.
func (g *Group) Has(code string) bool {
	if g == nil {
		return false
	}
	_, ok := g.byCode[code]
	return ok
}

// Rubric returns the pinned rubric for code, and false when code is not a
// member of the group.
func (g *Group) Rubric(code string) (string, bool) {
	if g == nil {
		return "", false
	}
	i, ok := g.byCode[code]
	if !ok {
		return "", false
	}
	return g.concepts[i].Rubric, true
}

// Code returns the code whose rubric is rubric, and false when no member of
// the group carries it. Rubrics are unique within a group in the pin.
func (g *Group) Code(rubric string) (string, bool) {
	if g == nil {
		return "", false
	}
	i, ok := g.byRubric[rubric]
	if !ok {
		return "", false
	}
	return g.concepts[i].Code, true
}

// CodeSet is one closed, source-ordered openEHR code set — a value set whose
// members are bare codes with no rubric, such as the normal statuses
// DV_ORDERED.Normal_status_validity names. Every code set is a package-level
// variable generated from the pin (see openehr_gen.go); a nil pointer is
// inert, with every method reporting absence (REQ-025).
type CodeSet struct {
	id, name string
	codes    []string
	index    map[string]struct{}
}

// newCodeSet builds a code set from its openehr_id, its display name and its
// codes in source order. Only the generated tables call it.
func newCodeSet(id, name string, codes []string) *CodeSet {
	s := &CodeSet{
		id:    id,
		name:  name,
		codes: codes,
		index: make(map[string]struct{}, len(codes)),
	}
	for _, c := range codes {
		s.index[c] = struct{}{}
	}
	return s
}

// ID returns the code set's openehr_id, e.g. "normal_statuses".
func (s *CodeSet) ID() string {
	if s == nil {
		return ""
	}
	return s.id
}

// Name returns the code set's display name, e.g. "normal statuses".
func (s *CodeSet) Name() string {
	if s == nil {
		return ""
	}
	return s.name
}

// Len returns the number of codes in the code set.
func (s *CodeSet) Len() int {
	if s == nil {
		return 0
	}
	return len(s.codes)
}

// All yields the code set's codes in source order — the order the pin lists
// them in. A nil code set yields nothing.
func (s *CodeSet) All() iter.Seq[string] {
	if s == nil {
		return func(func(string) bool) {}
	}
	return slices.Values(s.codes)
}

// Has reports whether code is a member of the code set.
func (s *CodeSet) Has(code string) bool {
	if s == nil {
		return false
	}
	_, ok := s.index[code]
	return ok
}

// Groups yields every group of the pinned terminology in source order.
func Groups() iter.Seq[*Group] {
	return slices.Values(groups)
}

// CodeSets yields every code set of the pinned terminology in source order.
func CodeSets() iter.Seq[*CodeSet] {
	return slices.Values(codeSets)
}

// GroupByID returns the group whose openehr_id is id — the identifier the
// RM's has_code_for_group_id invariants name — and false when the pinned
// terminology has no such group.
func GroupByID(id string) (*Group, bool) {
	for _, g := range groups {
		if g.ID() == id {
			return g, true
		}
	}
	return nil, false
}

// CodeSetByID returns the code set whose openehr_id is id, and false when the
// pinned terminology has no such code set.
func CodeSetByID(id string) (*CodeSet, bool) {
	for _, s := range codeSets {
		if s.ID() == id {
			return s, true
		}
	}
	return nil, false
}
