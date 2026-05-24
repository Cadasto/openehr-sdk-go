package rmread_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
)

// REQ-102 v2 Phase 1 — ReadSingle covers every (RMType, attr) the
// template walker descends through, and reports `ok=true` when the
// value is structurally present.
func TestReadSingle_PresentValues(t *testing.T) {
	comp := &rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.encounter.v1",
		Name:            rm.DVText{Value: "Encounter"},
		Category: rm.DVCodedText{
			DVText: rm.DVText{Value: "event"},
			DefiningCode: rm.CodePhrase{
				TerminologyID: rm.TerminologyID{Value: "openehr"},
				CodeString:    "433",
			},
		},
		Composer: rm.PartySelf{},
		Language: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "ISO_639-1"},
			CodeString:    "en",
		},
		Territory: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"},
			CodeString:    "NL",
		},
		Context: &rm.EventContext{},
	}

	cases := []struct {
		attr string
	}{
		{"archetype_node_id"},
		{"name"},
		{"category"},
		{"composer"},
		{"language"},
		{"territory"},
		{"context"},
	}
	for _, tc := range cases {
		t.Run(tc.attr, func(t *testing.T) {
			_, ok := rmread.ReadSingle(comp, "COMPOSITION", tc.attr)
			if !ok {
				t.Errorf("ReadSingle(COMPOSITION, %q) ok=false, want true", tc.attr)
			}
		})
	}
}

// REQ-102 v2 Phase 1 — empty / nil attributes report `ok=false` so
// the structural walker can flag a `required` issue.
func TestReadSingle_AbsentValues(t *testing.T) {
	empty := &rm.Composition{}

	for _, attr := range []string{"archetype_node_id", "name", "category", "composer", "language", "territory", "context"} {
		_, ok := rmread.ReadSingle(empty, "COMPOSITION", attr)
		if ok {
			t.Errorf("ReadSingle(empty, %q) ok=true, want false", attr)
		}
	}
}

// REQ-102 v2 Phase 1 — ReadMultiple returns the typed slice boxed
// as []any with `ok=true` even on empty slices; an unknown attr
// yields `ok=false`.
func TestReadMultiple(t *testing.T) {
	comp := &rm.Composition{
		Content: []rm.ContentItem{
			&rm.Observation{ArchetypeNodeID: "openEHR-EHR-OBSERVATION.blood_pressure.v1"},
		},
	}
	items, ok := rmread.ReadMultiple(comp, "COMPOSITION", "content")
	if !ok {
		t.Fatal("ReadMultiple(/content) ok=false, want true")
	}
	if len(items) != 1 {
		t.Errorf("ReadMultiple(/content) returned %d items, want 1", len(items))
	}
	if _, ok := items[0].(*rm.Observation); !ok {
		t.Errorf("ReadMultiple(/content)[0] type = %T, want *rm.Observation", items[0])
	}

	if _, ok := rmread.ReadMultiple(comp, "COMPOSITION", "no_such_attr"); ok {
		t.Error("ReadMultiple(/no_such_attr) ok=true, want false")
	}

	empty := &rm.Composition{}
	items, ok = rmread.ReadMultiple(empty, "COMPOSITION", "content")
	if !ok {
		t.Error("ReadMultiple(empty content) ok=false, want true (attr addressable even when empty)")
	}
	if len(items) != 0 {
		t.Errorf("ReadMultiple(empty content) returned %d items, want 0", len(items))
	}
}

// REQ-102 v2 Phase 1 — Observation traversal: data is present iff
// the History is non-zero; state/protocol are absent until set.
func TestReadSingle_Observation(t *testing.T) {
	obs := &rm.Observation{
		ArchetypeNodeID: "openEHR-EHR-OBSERVATION.blood_pressure.v1",
		Data: rm.History[rm.ItemStructure]{
			ArchetypeNodeID: "at0001",
			Events: []rm.Event{
				&rm.PointEvent[rm.ItemStructure]{
					ArchetypeNodeID: "at0002",
				},
			},
		},
	}
	if _, ok := rmread.ReadSingle(obs, "OBSERVATION", "data"); !ok {
		t.Error("Observation.data ok=false, want true (non-zero history)")
	}
	if _, ok := rmread.ReadSingle(obs, "OBSERVATION", "state"); ok {
		t.Error("Observation.state ok=true, want false (nil pointer)")
	}
	if _, ok := rmread.ReadSingle(obs, "OBSERVATION", "protocol"); ok {
		t.Error("Observation.protocol ok=true, want false (nil interface)")
	}
}

// REQ-102 v2 Phase 1 — ItemStructure variants each carry a
// different child-attribute name; ReadMultiple routes correctly.
func TestReadMultiple_ItemStructureVariants(t *testing.T) {
	tree := &rm.ItemTree{
		ArchetypeNodeID: "at0003",
		Items: []rm.Item{
			&rm.Element{ArchetypeNodeID: "at0004"},
		},
	}
	if items, ok := rmread.ReadMultiple(tree, "ITEM_TREE", "items"); !ok || len(items) != 1 {
		t.Errorf("ItemTree.items ok=%v len=%d, want true, 1", ok, len(items))
	}

	list := &rm.ItemList{
		ArchetypeNodeID: "at0005",
		Items:           []rm.Element{{ArchetypeNodeID: "at0006"}},
	}
	if items, ok := rmread.ReadMultiple(list, "ITEM_LIST", "items"); !ok || len(items) != 1 {
		t.Errorf("ItemList.items ok=%v len=%d, want true, 1", ok, len(items))
	} else if _, isEl := items[0].(*rm.Element); !isEl {
		t.Errorf("ItemList.items[0] type = %T, want *rm.Element", items[0])
	}

	table := &rm.ItemTable{
		ArchetypeNodeID: "at0007",
		Rows:            []rm.Cluster{{ArchetypeNodeID: "at0008"}},
	}
	if items, ok := rmread.ReadMultiple(table, "ITEM_TABLE", "rows"); !ok || len(items) != 1 {
		t.Errorf("ItemTable.rows ok=%v len=%d, want true, 1", ok, len(items))
	}

	single := &rm.ItemSingle{
		ArchetypeNodeID: "at0009",
		Item:            rm.Element{ArchetypeNodeID: "at0010"},
	}
	if v, ok := rmread.ReadSingle(single, "ITEM_SINGLE", "item"); !ok {
		t.Errorf("ItemSingle.item ok=false, want true")
	} else if _, isEl := v.(*rm.Element); !isEl {
		t.Errorf("ItemSingle.item type = %T, want *rm.Element", v)
	}
}

// REQ-102 v2 Phase 1 — DataValue inner navigations: DVCodedText →
// defining_code (CODE_PHRASE leaf). The OPT validator descends
// here when a C_CODE_PHRASE is declared under /defining_code.
func TestReadSingle_DataValueNavigation(t *testing.T) {
	cat := &rm.DVCodedText{
		DVText: rm.DVText{Value: "event"},
		DefiningCode: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "openehr"},
			CodeString:    "433",
		},
	}
	if v, ok := rmread.ReadSingle(cat, "DV_CODED_TEXT", "defining_code"); !ok {
		t.Error("DVCodedText.defining_code ok=false")
	} else if cp, isCP := v.(rm.CodePhrase); !isCP || cp.CodeString != "433" {
		t.Errorf("defining_code unexpected: %#v", v)
	}

	empty := &rm.DVCodedText{}
	if _, ok := rmread.ReadSingle(empty, "DV_CODED_TEXT", "defining_code"); ok {
		t.Error("empty DVCodedText.defining_code ok=true, want false")
	}
}

// REQ-102 v2 Phase 1 — unknown (parent type, attr) pair returns
// (nil, false) without panic.
func TestReadSingle_UnknownPair(t *testing.T) {
	if _, ok := rmread.ReadSingle(struct{}{}, "WHATEVER", "x"); ok {
		t.Error("unknown parent type ok=true, want false")
	}
	comp := &rm.Composition{}
	if _, ok := rmread.ReadSingle(comp, "COMPOSITION", "no_such_attr"); ok {
		t.Error("unknown attr ok=true, want false")
	}
}

// REQ-102 v2 Phase 1 — passing a nil parent must not panic. The
// concrete-type switch lands on `*rm.Composition(nil)` which the
// handler should treat as unrecognised.
func TestReadSingle_NilParent(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ReadSingle panicked on nil parent: %v", r)
		}
	}()
	// Untyped nil — falls through the switch to (nil, false).
	if _, ok := rmread.ReadSingle(nil, "COMPOSITION", "category"); ok {
		t.Error("ReadSingle(nil) ok=true, want false")
	}
}
