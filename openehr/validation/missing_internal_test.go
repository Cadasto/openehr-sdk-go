package validation

// missing_internal_test.go: REQ-112 § Value-typed mandatory presence,
// REQ-171 § Surface: the trie a call's report of missing members is read
// from. A pointer is read as RFC 6901 writes it, the options of one call
// are read as the union of their lists, and a call with no member named
// builds no trie, so the walk pays nothing for the option it was not given.

import (
	"slices"
	"testing"
)

// marks returns every pointer the trie under n marks, each written back in
// unescaped segments joined by "/", sorted.
func marks(n *missingNode, prefix string) []string {
	var out []string
	if n.marked() {
		out = append(out, prefix)
	}
	for key, below := range n.below {
		out = append(out, marks(below, prefix+"/"+key)...)
	}
	slices.Sort(out)
	return out
}

// TestWithMissing_REQ112_ReadsPointers pins how WithMissing reads its
// pointers: the segments of each, unescaped "~1" before "~0", so that "~01"
// names a key "~1"; the union of every list the call gives, a pointer given
// twice marking one member; and no mark for the empty pointer, which names
// the root, nor for a text that is no JSON Pointer: one that does not start
// with "/", and one with a "~" that opens no escape, which would otherwise
// name a Hash key spelled with that "~".
func TestWithMissing_REQ112_ReadsPointers(t *testing.T) {
	t.Parallel()
	c := newCallConfig([]ValidateOption{
		WithMissing([]string{"/subject", "/details/de~1x/purpose", "/details/~01/purpose"}),
		nil,
		WithMissing([]string{"/content/0/data/events/0/time", "/subject", "", "name", "/details/de~2x/purpose", "/details/x~"}),
	})
	want := []string{
		"/content/0/data/events/0/time",
		"/details/de/x/purpose",
		"/details/~1/purpose",
		"/subject",
	}
	if got := marks(c.missing, ""); !slices.Equal(got, want) {
		t.Errorf("WithMissing marks %q, want %q", got, want)
	}
	entry := c.missing.child("details").child("de/x")
	if entry == nil || entry.marked() || !entry.child("purpose").marked() {
		t.Errorf("WithMissing(/details/de~1x/purpose): the entry de/x is %+v, want an unmarked node with purpose marked below it", entry)
	}
}

// TestWithMissing_REQ171_NoMemberBuildsNoTrie pins the cost of a call that
// names no member: no option, a nil option, a nil list, an empty list and a
// list of texts that name no member, malformed escapes among them, each
// leave the call with no trie, so every node of the walk carries a nil one
// and child lookups stop at once.
func TestWithMissing_REQ171_NoMemberBuildsNoTrie(t *testing.T) {
	t.Parallel()
	for name, opts := range map[string][]ValidateOption{
		"no option":      nil,
		"a nil option":   {nil},
		"a nil list":     {WithMissing(nil)},
		"an empty list":  {WithMissing([]string{})},
		"no member text": {WithMissing([]string{"", "subject", "/de~2x/purpose", "/x~", "/a/~"})},
	} {
		if c := newCallConfig(opts); c.missing != nil {
			t.Errorf("%s: newCallConfig built a trie %+v, want none", name, c.missing)
		}
	}
	var none *missingNode
	if got := none.child("subject"); got != nil || none.marked() {
		t.Errorf("a nil trie: child(subject) = %+v, marked = %t; want nil and false", got, none.marked())
	}
}

// TestUnescapePointer_REQ112 pins RFC 6901's order: "~1" is read as "/"
// first and "~0" as "~" after it.
func TestUnescapePointer_REQ112(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"subject": "subject",
		"de~1x":   "de/x",
		"a~0b":    "a~b",
		"~01":     "~1",
		"~10":     "/0",
		"~0~1":    "~/",
	} {
		if got := unescapePointer(in); got != want {
			t.Errorf("unescapePointer(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestEngine_REQ171_AWithdrawnTrialRecordsNoAbsence pins the walk's record
// of the members it read as absent (REQ-171 § One walk, Trials: nothing a
// trial under a child the value does not bind to found reaches the result):
// an item of a list whose rank holds two children is tried before it is
// walked under the child it binds to, and a member the report names inside
// it is recorded once, by that walk, not again by each trial, whose records
// are withdrawn with it.
func TestEngine_REQ171_AWithdrawnTrialRecordsNoAbsence(t *testing.T) {
	t.Parallel()
	c := compileOPTText(t, clusterChainOPT(1, false))
	r, e := engineWalk(clusterChain(1, false), c, Limits{}, WithMissing([]string{"/items/0/name"}))
	if e == nil {
		t.Fatal("engineWalk(CLUSTER chain) returned no engine: a guard refused the call")
	}
	const name = "/items[at0001]/name"
	if want := []string{name}; !slices.Equal(e.absent, want) {
		t.Errorf("engineWalk(CLUSTER chain, WithMissing(/items/0/name)) recorded %q as absent, want %q: once, by the walk the item binds to", e.absent, want)
	}
	if n := len(slices.DeleteFunc(slices.Clone(r.Issues), func(i Issue) bool { return i.Path != name || i.Code != "required" })); n != 1 {
		t.Errorf("engineWalk(CLUSTER chain, WithMissing(/items/0/name)) reports required at %s %d times, want once; issues %+v", name, n, r.Issues)
	}
}
