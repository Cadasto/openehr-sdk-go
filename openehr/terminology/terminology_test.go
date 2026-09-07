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
	if !g.Has("1") || g.Has("3") || g.Has("") {
		t.Error("Has misreports membership")
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
	if !s.Has("N") || s.Has("X") || s.Len() != 2 || s.ID() != "demo_set" || s.Name() != "demo set" {
		t.Error("code-set surface")
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
	if got := slices.Collect(Groups()); len(got) != 2 || got[0] != a || got[1] != b {
		t.Error("Groups order")
	}
	if g, ok := GroupByID("b"); !ok || g != b {
		t.Error("GroupByID")
	}
	if _, ok := GroupByID("zzz"); ok {
		t.Error("GroupByID unknown")
	}
	if s, ok := CodeSetByID("s"); !ok || s.ID() != "s" {
		t.Error("CodeSetByID")
	}
	if _, ok := CodeSetByID("zzz"); ok {
		t.Error("CodeSetByID unknown")
	}
	if got := len(slices.Collect(CodeSets())); got != 1 {
		t.Errorf("CodeSets yields %d, want 1", got)
	}
}
