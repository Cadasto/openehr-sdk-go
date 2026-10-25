package rminfo_test

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// TestNonEmptyLists_REQ112_MatchThePinnedBMM pins the table NonEmptyLists
// returns against the pinned RM BMM: the six entries the REQ-112 Non-empty
// lists entry names, each an invariant the BMM declares on its class whose
// text reads its list, and its alternative where it has one, as the entry
// says: `x /= Void implies not x.is_empty` for a list x alone, and
// `(x /= Void and then not x.is_empty) or a /= Void` for a list x that an
// attribute a also keeps. The list is a container attribute of the class
// and the alternative a single-valued one. A BMM bump that rewrites one of
// these invariants fails here until the table follows.
func TestNonEmptyLists_REQ112_MatchThePinnedBMM(t *testing.T) {
	t.Parallel()
	schema, err := bmm.LoadAll("openehr_rm_1.2.0", bmm.FSResolver{Root: probeResourcesDir})
	if err != nil {
		t.Fatalf("LoadAll(openehr_rm_1.2.0): %v", err)
	}
	lists := rminfo.NonEmptyLists()
	var keys []string
	for _, l := range lists {
		keys = append(keys, l.Class+"."+l.Invariant)
	}
	want := []string{
		"ATTESTATION.Items_valid",
		"COMPOSITION.Content_valid",
		"EVENT_CONTEXT.Participations_validity",
		"HISTORY.Events_valid",
		"INSTRUCTION.Activities_valid",
		"SECTION.Items_valid",
	}
	if !slices.Equal(keys, want) {
		t.Fatalf("NonEmptyLists names %q, want %q", keys, want)
	}
	for _, l := range lists {
		cls, ok := schema.ClassDefinitions[l.Class].(*bmm.SimpleClass)
		if !ok {
			t.Errorf("the pinned BMM defines no class %s as a simple class", l.Class)
			continue
		}
		text := l.List + " /= Void implies not " + l.List + ".is_empty"
		if l.Alternative != "" {
			text = "(" + l.List + " /= Void and then not " + l.List + ".is_empty) or " + l.Alternative + " /= Void"
		}
		if got := cls.Invariants[l.Invariant]; got != text {
			t.Errorf("BMM %s.%s = %q, want %q, the invariant the entry reads", l.Class, l.Invariant, got, text)
		}
		if isList, known := rminfo.Default.IsContainer(l.Class, l.List); !known || !isList {
			t.Errorf("IsContainer(%q, %q) = %t, %t, want a list", l.Class, l.List, isList, known)
		}
		if l.Alternative != "" {
			if isList, known := rminfo.Default.IsContainer(l.Class, l.Alternative); !known || isList {
				t.Errorf("IsContainer(%q, %q) = %t, %t, want a single-valued attribute", l.Class, l.Alternative, isList, known)
			}
		}
	}
}

// TestNonEmptyListOf finds each entry of NonEmptyLists by its class and
// list, and no entry for a class or an attribute the table does not pair,
// nor for a class that only conforms to one it names.
func TestNonEmptyListOf(t *testing.T) {
	t.Parallel()
	for _, l := range rminfo.NonEmptyLists() {
		if got, ok := rminfo.NonEmptyListOf(l.Class, l.List); !ok || got != l {
			t.Errorf("NonEmptyListOf(%q, %q) = %+v, %t, want %+v, true", l.Class, l.List, got, ok, l)
		}
	}
	for _, tc := range []struct{ class, attr string }{
		{class: "SECTION", attr: "content"},
		{class: "HISTORY", attr: "summary"},
		{class: "CLUSTER", attr: "items"},
		{class: "ITEM_TREE", attr: "items"},
		{class: "CONTENT_ITEM", attr: "items"},
	} {
		if got, ok := rminfo.NonEmptyListOf(tc.class, tc.attr); ok {
			t.Errorf("NonEmptyListOf(%q, %q) = %+v, true, want no entry", tc.class, tc.attr, got)
		}
	}
}

// TestNonEmptyLists_ReturnsACopy checks that a caller who changes the slice
// NonEmptyLists returns changes neither a later call's answer nor
// NonEmptyListOf's.
func TestNonEmptyLists_ReturnsACopy(t *testing.T) {
	t.Parallel()
	first := rminfo.NonEmptyLists()
	want := slices.Clone(first)
	for i := range first {
		first[i].List = "changed"
	}
	if got := rminfo.NonEmptyLists(); !slices.Equal(got, want) {
		t.Errorf("NonEmptyLists after a caller's change = %+v, want %+v", got, want)
	}
	if _, ok := rminfo.NonEmptyListOf("COMPOSITION", "content"); !ok {
		t.Error(`NonEmptyListOf("COMPOSITION", "content") found no entry after a caller changed a copy`)
	}
}
