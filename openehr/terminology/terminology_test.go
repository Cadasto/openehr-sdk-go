package terminology

import (
	"slices"
	"testing"
)

func TestGroupLookups(t *testing.T) {
	t.Parallel()
	g := newGroup("demo", "demo group", []Concept{{"1", "one"}, {"2", "two"}})
	if got := g.ID(); got != "demo" {
		t.Errorf("ID = %q", got)
	}
	if got := g.Name(); got != "demo group" {
		t.Errorf("Name = %q", got)
	}
	if got := g.Len(); got != 2 {
		t.Errorf("Len = %d", got)
	}
	if r, ok := g.Rubric("2"); !ok || r != "two" {
		t.Errorf("Rubric(2) = %q,%v", r, ok)
	}
	if c, ok := g.Code("one"); !ok || c != "1" {
		t.Errorf("Code(one) = %q,%v", c, ok)
	}
	if got := g.Has("1"); !got {
		t.Errorf("Has(%q) = %v, want true — a member of the group", "1", got)
	}
	if got := g.Has("3"); got {
		t.Errorf("Has(%q) = %v, want false — not a member", "3", got)
	}
	if got := g.Has(""); got {
		t.Errorf("Has(%q) = %v, want false — the empty code is never a member", "", got)
	}
	if _, ok := g.Rubric("3"); ok {
		t.Error("Rubric on an unknown code must report absence")
	}
	if _, ok := g.Code("three"); ok {
		t.Error("Code on an unknown rubric must report absence")
	}
	got := slices.Collect(g.All())
	if !slices.Equal(got, []Concept{{"1", "one"}, {"2", "two"}}) {
		t.Errorf("All = %v (source order required)", got)
	}
}

func TestNilGroupAndCodeSetAreInert(t *testing.T) { // REQ-025
	t.Parallel()
	var g *Group
	if g.Has("1") || g.Len() != 0 || g.ID() != "" || g.Name() != "" {
		t.Error("nil *Group must be inert")
	}
	if _, ok := g.Rubric("1"); ok {
		t.Error("nil Rubric")
	}
	if _, ok := g.Code("x"); ok {
		t.Error("nil Code")
	}
	if n := len(slices.Collect(g.All())); n != 0 {
		t.Errorf("nil All yields %d", n)
	}
	var s *CodeSet
	if s.Has("N") || s.Len() != 0 || s.ID() != "" || s.Name() != "" {
		t.Error("nil *CodeSet must be inert")
	}
	if n := len(slices.Collect(s.All())); n != 0 {
		t.Errorf("nil All yields %d", n)
	}
}

func TestCodeSetLookups(t *testing.T) {
	t.Parallel()
	s := newCodeSet("demo_set", "demo set", []string{"N", "H"})
	if got := s.Has("N"); !got {
		t.Errorf("Has(%q) = %v, want true — a member of the code set", "N", got)
	}
	if got := s.Has("X"); got {
		t.Errorf("Has(%q) = %v, want false — not a member", "X", got)
	}
	if got := s.Len(); got != 2 {
		t.Errorf("Len() = %d, want 2", got)
	}
	if got := s.ID(); got != "demo_set" {
		t.Errorf("ID() = %q, want %q", got, "demo_set")
	}
	if got := s.Name(); got != "demo set" {
		t.Errorf("Name() = %q, want %q", got, "demo set")
	}
	if got := slices.Collect(s.All()); !slices.Equal(got, []string{"N", "H"}) {
		t.Errorf("All = %v", got)
	}
}

func TestRegistryAccessorsUseTheTables(t *testing.T) {
	// swap the package tables for the test's own, restore after — so this
	// test cannot run in parallel with any other.
	saveG, saveS := groups, codeSets
	t.Cleanup(func() { groups, codeSets = saveG, saveS })
	a := newGroup("a", "a", []Concept{{"1", "one"}})
	b := newGroup("b", "b", []Concept{{"2", "two"}})
	groups = []*Group{a, b}
	codeSets = []*CodeSet{newCodeSet("s", "s", []string{"N"})}
	got := slices.Collect(Groups())
	if len(got) != 2 {
		t.Fatalf("len(slices.Collect(Groups())) = %d, want 2 (the two tables just installed)", len(got))
	}
	if !slices.Equal(got, []*Group{a, b}) {
		t.Errorf("Groups() = [%q %q], want source order [%q %q]", got[0].ID(), got[1].ID(), a.ID(), b.ID())
	}
	g, ok := GroupByID("b")
	if !ok {
		t.Errorf("GroupByID(%q) reported absence, want the installed group", "b")
	} else if g != b {
		t.Errorf("GroupByID(%q) = %q, want %q", "b", g.ID(), b.ID())
	}
	if _, ok := GroupByID("zzz"); ok {
		t.Errorf("GroupByID(%q) = _, true — want absence reported for an id no table carries", "zzz")
	}
	s, ok := CodeSetByID("s")
	if !ok {
		t.Errorf("CodeSetByID(%q) reported absence, want the installed code set", "s")
	} else if s.ID() != "s" {
		t.Errorf("CodeSetByID(%q).ID() = %q, want %q", "s", s.ID(), "s")
	}
	if _, ok := CodeSetByID("zzz"); ok {
		t.Error("CodeSetByID unknown")
	}
	if got := len(slices.Collect(CodeSets())); got != 1 {
		t.Errorf("CodeSets yields %d, want 1", got)
	}
}
