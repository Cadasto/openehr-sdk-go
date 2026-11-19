package rmaccess_test

// append_test.go: AppendMultiple appends the items of a list to a buffer the
// caller owns, so a walk that reads a list through a buffer it reuses
// allocates nothing for it; Multiple is AppendMultiple into no buffer.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/rmaccess"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// prefixed returns a buffer holding two items, with room for six more, so a
// test can tell an append that keeps them from one that drops them.
func prefixed() []any {
	dst := make([]any, 2, 8)
	dst[0], dst[1] = "first", "second"
	return dst
}

// checkPrefix fails unless got starts with the two items prefixed holds, in
// dst's own backing array.
func checkPrefix(t *testing.T, call string, got, dst []any) {
	t.Helper()
	if len(got) < 2 || got[0] != "first" || got[1] != "second" {
		t.Fatalf("%s = %v, want it to start with the buffer's items [first second]", call, got)
	}
	if &got[0] != &dst[0] {
		t.Errorf("%s returned a new backing array, want the buffer's own: it had room for every item", call)
	}
}

func TestAppendMultipleAppendsAfterTheBuffer(t *testing.T) {
	section := &rm.Section{ArchetypeNodeID: "at0001"}
	cluster := &rm.Cluster{ArchetypeNodeID: "at0002"}
	list := &rm.ItemList{Items: []rm.Element{{ArchetypeNodeID: "at0003"}, {ArchetypeNodeID: "at0004"}}}
	composition := &rm.Composition{Content: []rm.ContentItem{section}}
	tree := &rm.Cluster{Items: []rm.Item{cluster, &list.Items[0]}}

	t.Run("list of values", func(t *testing.T) {
		dst := prefixed()
		got, present := rmaccess.AppendMultiple(dst, list, "items")
		const call = "AppendMultiple(prefix, ITEM_LIST, items)"
		if !present || len(got) != 4 {
			t.Fatalf("%s = (%d items, %t), want (4, true)", call, len(got), present)
		}
		checkPrefix(t, call, got, dst)
		for i := range list.Items {
			if got[2+i] != &list.Items[i] {
				t.Errorf("%s[%d] = %v, want a pointer to items[%d] in the parent's backing array", call, 2+i, got[2+i], i)
			}
		}
	})
	t.Run("list of interfaces", func(t *testing.T) {
		dst := prefixed()
		got, present := rmaccess.AppendMultiple(dst, composition, "content")
		const call = "AppendMultiple(prefix, COMPOSITION, content)"
		if !present || len(got) != 3 {
			t.Fatalf("%s = (%d items, %t), want (3, true)", call, len(got), present)
		}
		checkPrefix(t, call, got, dst)
		if got[2] != any(section) {
			t.Errorf("%s[2] = %v, want the SECTION as the list holds it", call, got[2])
		}
	})
	t.Run("parent held by value", func(t *testing.T) {
		dst := prefixed()
		got, present := rmaccess.AppendMultiple(dst, *tree, "items")
		const call = "AppendMultiple(prefix, CLUSTER value, items)"
		if !present || len(got) != 4 {
			t.Fatalf("%s = (%d items, %t), want (4, true)", call, len(got), present)
		}
		checkPrefix(t, call, got, dst)
		if got[2] != any(cluster) || got[3] != any(&list.Items[0]) {
			t.Errorf("%s = %v, want the items as the list holds them", call, got)
		}
	})
	t.Run("buffer too small", func(t *testing.T) {
		dst := []any{"first", "second"}
		got, present := rmaccess.AppendMultiple(dst[:2:2], list, "items")
		if !present || len(got) != 4 || got[0] != "first" || got[1] != "second" || got[2] != &list.Items[0] || got[3] != &list.Items[1] {
			t.Errorf("AppendMultiple(full buffer, ITEM_LIST, items) = (%v, %t), want the buffer's items then pointers to the two items", got, present)
		}
	})
}

func TestAppendMultipleWritesReachTheParent(t *testing.T) {
	list := &rm.ItemList{Items: []rm.Element{{ArchetypeNodeID: "at0001"}}}
	var buf [4]any
	got, present := rmaccess.AppendMultiple(buf[:0], list, "items")
	if !present || len(got) != 1 {
		t.Fatalf("AppendMultiple(buffer, ITEM_LIST, items) = (%d items, %t), want (1, true)", len(got), present)
	}
	element, ok := got[0].(*rm.Element)
	if !ok {
		t.Fatalf("AppendMultiple(buffer, ITEM_LIST, items)[0] = %T, want *rm.Element", got[0])
	}
	element.ArchetypeNodeID = "at0002"
	if list.Items[0].ArchetypeNodeID != "at0002" {
		t.Errorf("after a write through item 0, the parent's items[0] holds %q, want %q: the item is a copy", list.Items[0].ArchetypeNodeID, "at0002")
	}
}

func TestAppendMultipleKeepsTheBufferWhenNothingIsAppended(t *testing.T) {
	for _, tc := range []struct {
		name        string
		parent      any
		attr        string
		wantPresent bool
	}{
		{name: "absent list", parent: &rm.ItemList{}, attr: "items"},
		{name: "empty list", parent: &rm.ItemList{Items: []rm.Element{}}, attr: "items", wantPresent: true},
		{name: "undeclared attribute", parent: &rm.ItemList{Items: []rm.Element{{}}}, attr: "no_such_attribute"},
		{name: "single-valued attribute", parent: &rm.ItemList{Items: []rm.Element{{}}}, attr: "name"},
		{name: "nil parent", parent: nil, attr: "items"},
		{name: "typed-nil parent", parent: (*rm.ItemList)(nil), attr: "items"},
		{name: "foreign parent", parent: "not an RM value", attr: "items"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dst := prefixed()
			got, present := rmaccess.AppendMultiple(dst, tc.parent, tc.attr)
			if present != tc.wantPresent || len(got) != 2 {
				t.Fatalf("AppendMultiple(prefix, %T, %q) = (%v, %t), want the buffer unchanged and %t", tc.parent, tc.attr, got, present, tc.wantPresent)
			}
			checkPrefix(t, "AppendMultiple(prefix, "+tc.name+")", got, dst)
		})
	}
}

func TestMultipleReadsIntoNoBuffer(t *testing.T) {
	list := &rm.ItemList{Items: []rm.Element{{}, {}}}
	if items, present := rmaccess.Multiple(list, "items"); !present || len(items) != 2 || items[1] != &list.Items[1] {
		t.Errorf("Multiple(ITEM_LIST with two items, items) = (%v, %t), want pointers to the two items and true", items, present)
	}
	if items, present := rmaccess.Multiple(&rm.ItemList{Items: []rm.Element{}}, "items"); items != nil || !present {
		t.Errorf("Multiple(ITEM_LIST with an empty list, items) = (%#v, %t), want (nil, true)", items, present)
	}
	if items, present := rmaccess.Multiple(&rm.ItemList{}, "items"); items != nil || present {
		t.Errorf("Multiple(ITEM_LIST with no list, items) = (%#v, %t), want (nil, false)", items, present)
	}
}

// TestAppendMultipleAllocatesNothingIntoARoomyBuffer pins the reason
// AppendMultiple exists: read into a buffer with room for every item, a list
// of values and a list of interfaces cost no allocation. It runs serially, as
// testing.AllocsPerRun counts every allocation the process makes.
func TestAppendMultipleAllocatesNothingIntoARoomyBuffer(t *testing.T) {
	element := rm.Element{ArchetypeNodeID: "at0001"}
	link := rm.Link{Type: rm.DVText{Value: "issue"}}
	for _, tc := range []struct {
		name   string
		parent any
		attr   string
		want   int
	}{
		{name: "ITEM_LIST items, a list of values", parent: &rm.ItemList{Items: []rm.Element{element, element, element}}, attr: "items", want: 3},
		{name: "CLUSTER links, a list of values", parent: &rm.Cluster{Links: []rm.Link{link, link}}, attr: "links", want: 2},
		{name: "CLUSTER items, a list of interfaces", parent: &rm.Cluster{Items: []rm.Item{&element, &rm.Cluster{}}}, attr: "items", want: 2},
		{name: "COMPOSITION content, a list of interfaces", parent: &rm.Composition{Content: []rm.ContentItem{&rm.Section{}, &rm.Observation{}, &rm.Section{}}}, attr: "content", want: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var n int
			var present bool
			allocs := testing.AllocsPerRun(100, func() {
				var buf [8]any
				items, ok := rmaccess.AppendMultiple(buf[:0], tc.parent, tc.attr)
				n, present = len(items), ok
			})
			if n != tc.want || !present {
				t.Fatalf("AppendMultiple(buffer, %T, %q) = (%d items, %t), want (%d, true)", tc.parent, tc.attr, n, present, tc.want)
			}
			if allocs != 0 {
				t.Errorf("AppendMultiple(buffer of 8, %T, %q) made %v allocations per call, want 0: it boxes the list into a new slice", tc.parent, tc.attr, allocs)
			}
		})
	}
}
