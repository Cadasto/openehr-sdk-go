package terminology

import (
	"slices"
	"testing"
)

// REQ-034: a group answers code to rubric, rubric to code and membership in
// source order, and an unknown code or rubric reports absence.
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

// REQ-034: lookups on a nil group or code set report absence and never panic.
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
	if got := s.Issuer(); got != "" {
		t.Errorf("nil *CodeSet Issuer() = %q, want empty", got)
	}
	if got := s.ExternalID(); got != "" {
		t.Errorf("nil *CodeSet ExternalID() = %q, want empty", got)
	}
	if n := len(slices.Collect(s.All())); n != 0 {
		t.Errorf("nil All yields %d", n)
	}
}

func TestCodeSetLookups(t *testing.T) {
	t.Parallel()
	s := newCodeSet("demo_set", "demo set", "openehr", "openehr_demo_set", []string{"N", "H"})
	if got := s.Issuer(); got != "openehr" {
		t.Errorf("Issuer() = %q, want %q", got, "openehr")
	}
	if got := s.ExternalID(); got != "openehr_demo_set" {
		t.Errorf("ExternalID() = %q, want %q", got, "openehr_demo_set")
	}
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

// REQ-034: a code set's membership ignores letter case when its issuer is
// not openEHR, and is exact when it is. Only the ASCII letters fold, so a
// look-alike outside ASCII never matches a pinned letter.
func TestCodeSetMembershipFollowsTheIssuer(t *testing.T) {
	t.Parallel()
	external := newCodeSet("demo_ext", "demo external", "ISO", "ISO_demo", []string{"en-us", "KE", "it"})
	own := newCodeSet("demo_own", "demo own", "openehr", "openehr_demo", []string{"H", "en"})
	// The issuer attribute itself is matched ignoring case.
	ownUpper := newCodeSet("demo_own_upper", "demo own upper", "openEHR", "openehr_demo", []string{"H"})
	tests := []struct {
		name string
		set  *CodeSet
		code string
		want bool
	}{
		{name: "external, as pinned", set: external, code: "en-us", want: true},
		{name: "external, upper case", set: external, code: "EN-US", want: true},
		{name: "external, mixed case", set: external, code: "en-US", want: true},
		{name: "external, pinned upper, asked lower", set: external, code: "ke", want: true},
		{name: "external, not listed", set: external, code: "en-gb", want: false},
		{name: "external, empty", set: external, code: "", want: false},
		// The next two rows catch Unicode case folding: Go's
		// unicode.ToLower maps U+212A KELVIN SIGN to k and U+0130 LATIN
		// CAPITAL LETTER I WITH DOT ABOVE to i, so strings.ToLower would
		// admit both, as KE and it; strings.EqualFold would admit the
		// Kelvin sign only.
		{name: "external, Kelvin sign for K", set: external, code: "KE", want: false},
		{name: "external, dotted capital I for I", set: external, code: "İT", want: false},
		{name: "external, ASCII capital I", set: external, code: "IT", want: true},
		// U+FF25 FULLWIDTH LATIN CAPITAL LETTER E and U+FF53 FULLWIDTH
		// LATIN SMALL LETTER S are look-alikes no folding turns into ASCII.
		{name: "external, fullwidth E", set: external, code: "ＥN-US", want: false},
		{name: "external, fullwidth s", set: external, code: "en-uｓ", want: false},
		{name: "openEHR, as pinned", set: own, code: "H", want: true},
		{name: "openEHR, other case", set: own, code: "h", want: false},
		{name: "openEHR, other case of a lower-case code", set: own, code: "EN", want: false},
		{name: "openEHR spelled openEHR, other case", set: ownUpper, code: "h", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.set.Has(tc.code); got != tc.want {
				t.Errorf("%s (issuer %q).Has(%q) = %v, want %v", tc.set.ID(), tc.set.Issuer(), tc.code, got, tc.want)
			}
		})
	}
	// All yields the codes as the pin spells them, whatever Has accepts.
	if got := slices.Collect(external.All()); !slices.Equal(got, []string{"en-us", "KE", "it"}) {
		t.Errorf("All = %q, want the pinned spelling [en-us KE it]", got)
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
	codeSets = []*CodeSet{newCodeSet("s", "s", "openehr", "openehr_s", []string{"N"})}
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
		t.Errorf("CodeSetByID(%q) = _, true — want absence reported for an id no table carries", "zzz")
	}
	if got := len(slices.Collect(CodeSets())); got != 1 {
		t.Errorf("CodeSets yields %d, want 1", got)
	}
}
