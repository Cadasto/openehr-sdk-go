package rmwrite

import (
	"errors"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/rmlists"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// TestREQ107_EnsureListMakesTheListPresent checks that EnsureList turns a
// nil item-structure list into a present empty one, and leaves a list that
// is already empty, or that holds members, as it is.
func TestREQ107_EnsureListMakesTheListPresent(t *testing.T) {
	t.Parallel()
	element := rm.Element{}
	cluster := rm.Cluster{}
	cases := []struct {
		name   string
		parent any
		attr   string
		want   int // the members the list holds after EnsureList
	}{
		{name: "nil ITEM_TREE items", parent: &rm.ItemTree{}, attr: "items"},
		{name: "nil ITEM_LIST items", parent: &rm.ItemList{}, attr: "items"},
		{name: "nil ITEM_TABLE rows", parent: &rm.ItemTable{}, attr: "rows"},
		{name: "empty ITEM_TREE items", parent: &rm.ItemTree{Items: []rm.Item{}}, attr: "items"},
		{name: "ITEM_TREE items with a member", parent: &rm.ItemTree{Items: []rm.Item{&element}}, attr: "items", want: 1},
		{name: "ITEM_LIST items with a member", parent: &rm.ItemList{Items: []rm.Element{element}}, attr: "items", want: 1},
		{name: "ITEM_TABLE rows with a member", parent: &rm.ItemTable{Rows: []rm.Cluster{cluster}}, attr: "rows", want: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := EnsureList(tc.parent, tc.attr); err != nil {
				t.Fatalf("EnsureList(%T, %q) = %v, want nil", tc.parent, tc.attr, err)
			}
			var present bool
			var members int
			switch p := tc.parent.(type) {
			case *rm.ItemTree:
				present, members = p.Items != nil, len(p.Items)
			case *rm.ItemList:
				present, members = p.Items != nil, len(p.Items)
			case *rm.ItemTable:
				present, members = p.Rows != nil, len(p.Rows)
			}
			if !present || members != tc.want {
				t.Errorf("after EnsureList(%T, %q) the list is present=%t with %d members, want present with %d",
					tc.parent, tc.attr, present, members, tc.want)
			}
		})
	}
}

// TestREQ107_EnsureListWritesTheKeepsEmptyLists checks EnsureList against
// rmlists.KeepsEmpty, the lists canonical JSON carries as [] when present
// and empty. Asked about every attribute name the RM declares on a fresh
// value of every class the type registry knows, it writes exactly the pairs
// KeepsEmpty names and refuses every other with ErrUnknownAttribute, so a
// pair added to one and not the other fails here.
func TestREQ107_EnsureListWritesTheKeepsEmptyLists(t *testing.T) {
	t.Parallel()
	attrs := probeAttrNames(modelOf(t, rminfo.Default))
	var wrote []string
	for class, newParent := range registryParents() {
		for _, attr := range attrs {
			err := EnsureList(newParent(), attr)
			if err != nil && !errors.Is(err, ErrUnknownAttribute) {
				t.Errorf("EnsureList(%s, %q) = %v, want nil or ErrUnknownAttribute", class, attr, err)
			}
			if got, want := err == nil, rmlists.KeepsEmpty(class, attr); got != want {
				t.Errorf("EnsureList(%s, %q) writes = %t, want %t as rmlists.KeepsEmpty says", class, attr, got, want)
			}
			if err == nil {
				wrote = append(wrote, class+"."+attr)
			}
		}
	}
	slices.Sort(wrote)
	if want := []string{"ITEM_LIST.items", "ITEM_TABLE.rows", "ITEM_TREE.items"}; !slices.Equal(wrote, want) {
		t.Errorf("EnsureList writes %q, want %q", wrote, want)
	}
}
