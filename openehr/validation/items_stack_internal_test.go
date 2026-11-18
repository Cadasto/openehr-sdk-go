package validation

// items_stack_internal_test.go: the engine reads the items of a list onto
// its items stack, as a run that ends at the list's last item, and drops the
// run when the visit of the attribute returns.

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// A list's run ends at its own last item, so an append to it cannot write
// into a run read above it; a second list is read above the first; an
// empty list and an absent one hold no run. A walk drops every run it read,
// so it ends with the stack empty.
func TestEngineItemsStack_RunsCappedAndDropped(t *testing.T) {
	t.Parallel()
	element := func(name string) *rm.Element {
		return &rm.Element{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: name}, Value: &rm.DVText{Value: "text"}}
	}
	nameOf := func(item any) string {
		if l, ok := item.(rm.Locatable); ok && l.GetName() != nil {
			return l.GetName().GetValue()
		}
		return fmt.Sprintf("%T", item)
	}
	outer := &rm.Cluster{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "outer"}, Items: []rm.Item{element("a"), element("b")}}
	inner := &rm.Cluster{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "inner"}, Items: []rm.Item{element("c")}}
	items := &engineAttr{name: "items", rmType: "ITEM", shape: shapeList}

	e := newEngine(Limits{})
	first := e.readAttribute(outer, items)
	if first.count != 2 || len(first.items) != 2 {
		t.Fatalf("readAttribute(outer, items): count %d, len %d, want 2, 2", first.count, len(first.items))
	}
	if cap(first.items) != 2 {
		t.Errorf("readAttribute(outer, items): cap %d, want 2, the run's own length", cap(first.items))
	}
	second := e.readAttribute(inner, items)
	if len(second.items) != 1 || nameOf(second.items[0]) != "c" {
		t.Fatalf("readAttribute(inner, items): %d items, want the one named c", len(second.items))
	}
	appended := append(first.items, element("appended"))
	if got := nameOf(second.items[0]); got != "c" {
		t.Errorf("appending %s to the outer run wrote into the inner one: its item is named %s, want c", nameOf(appended[2]), got)
	}
	if len(e.items) != 3 {
		t.Errorf("items stack holds %d items after two reads, want 3", len(e.items))
	}
	for _, c := range []*rm.Cluster{{Items: []rm.Item{}}, {}} {
		if r := e.readAttribute(c, items); r.items != nil || r.count != 0 {
			t.Errorf("readAttribute(CLUSTER with items %#v, items) = %d items, count %d, want nil, 0", c.Items, len(r.items), r.count)
		}
	}

	root := &rm.Cluster{ArchetypeNodeID: "at0000", Name: rm.DVText{Value: "root"}, Items: []rm.Item{outer, inner, element("d")}}
	if _, e := engineWalk(root, nil, Limits{}); len(e.items) != 0 {
		t.Errorf("items stack holds %d items after the walk, want 0", len(e.items))
	}
}
