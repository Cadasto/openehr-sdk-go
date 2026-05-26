package rmwrite

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

func TestNewRMRegistered(t *testing.T) {
	for _, name := range []string{
		"COMPOSITION", "OBSERVATION", "EVALUATION", "INSTRUCTION",
		"ACTION", "ADMIN_ENTRY", "SECTION", "GENERIC_ENTRY",
		"CLUSTER", "ELEMENT", "ITEM_LIST", "ITEM_TREE", "ITEM_SINGLE",
		"HISTORY", "POINT_EVENT", "EVENT_CONTEXT",
		"DV_TEXT", "DV_CODED_TEXT", "DV_QUANTITY", "CODE_PHRASE",
	} {
		v, err := NewRM(name)
		if err != nil {
			t.Fatalf("NewRM(%q): %v", name, err)
		}
		if v == nil {
			t.Errorf("NewRM(%q) returned nil", name)
		}
	}
}

func TestNewRMUnknown(t *testing.T) {
	_, err := NewRM("NOT_A_REAL_TYPE")
	if !errors.Is(err, ErrUnknownRMType) {
		t.Fatalf("want ErrUnknownRMType, got %v", err)
	}
}

func TestEnsureSingleVitalSignsCoverage(t *testing.T) {
	type singleCase struct {
		name       string
		parent     any
		parentType string
		attr       string
		child      any
		check      func(t *testing.T, parent any)
	}
	cases := []singleCase{
		{
			name:       "Composition.context",
			parent:     &rm.Composition{},
			parentType: "COMPOSITION",
			attr:       "context",
			child:      &rm.EventContext{},
			check: func(t *testing.T, parent any) {
				if parent.(*rm.Composition).Context == nil {
					t.Error("Context still nil after EnsureSingle")
				}
			},
		},
		{
			name:       "Observation.data",
			parent:     &rm.Observation{},
			parentType: "OBSERVATION",
			attr:       "data",
			child:      &rm.History[rm.ItemStructure]{ArchetypeNodeID: "at0001"},
			check: func(t *testing.T, parent any) {
				if got := parent.(*rm.Observation).Data.ArchetypeNodeID; got != "at0001" {
					t.Errorf("Data.ArchetypeNodeID = %q, want at0001", got)
				}
			},
		},
		{
			name:       "Element.value (DV_QUANTITY)",
			parent:     &rm.Element{},
			parentType: "ELEMENT",
			attr:       "value",
			child:      &rm.DVQuantity{Magnitude: 120, Units: "mm[Hg]"},
			check: func(t *testing.T, parent any) {
				e := parent.(*rm.Element)
				dv, ok := e.Value.(*rm.DVQuantity)
				if !ok {
					t.Fatalf("Element.Value type = %T, want *rm.DVQuantity", e.Value)
				}
				if dv.Magnitude != 120 {
					t.Errorf("Magnitude = %v, want 120", dv.Magnitude)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := EnsureSingle(tc.parent, tc.parentType, tc.attr, tc.child); err != nil {
				t.Fatalf("EnsureSingle: %v", err)
			}
			tc.check(t, tc.parent)
		})
	}
}

func TestAppendMultipleVitalSignsCoverage(t *testing.T) {
	t.Run("Composition.content", func(t *testing.T) {
		c := &rm.Composition{}
		obs := &rm.Observation{ArchetypeNodeID: "openEHR-EHR-OBSERVATION.blood_pressure.v1"}
		if err := AppendMultiple(c, "COMPOSITION", "content", obs); err != nil {
			t.Fatalf("AppendMultiple: %v", err)
		}
		if len(c.Content) != 1 {
			t.Fatalf("len(Content) = %d, want 1", len(c.Content))
		}
	})
	t.Run("History.events", func(t *testing.T) {
		h := &rm.History[rm.ItemStructure]{}
		ev := &rm.PointEvent[rm.ItemStructure]{ArchetypeNodeID: "at0006"}
		if err := AppendMultiple(h, "HISTORY", "events", ev); err != nil {
			t.Fatalf("AppendMultiple: %v", err)
		}
		if len(h.Events) != 1 {
			t.Fatalf("len(Events) = %d, want 1", len(h.Events))
		}
	})
	t.Run("ItemList.items", func(t *testing.T) {
		l := &rm.ItemList{}
		el := &rm.Element{ArchetypeNodeID: "at0004"}
		if err := AppendMultiple(l, "ITEM_LIST", "items", el); err != nil {
			t.Fatalf("AppendMultiple: %v", err)
		}
		if len(l.Items) != 1 {
			t.Fatalf("len(Items) = %d, want 1", len(l.Items))
		}
	})
	t.Run("Cluster.items", func(t *testing.T) {
		cl := &rm.Cluster{}
		el := &rm.Element{ArchetypeNodeID: "at0010"}
		if err := AppendMultiple(cl, "CLUSTER", "items", el); err != nil {
			t.Fatalf("AppendMultiple: %v", err)
		}
		if len(cl.Items) != 1 {
			t.Fatalf("len(Items) = %d, want 1", len(cl.Items))
		}
	})
}

// TestEnsureSingleDVTemporal pins the writers for the AOM 1.4
// primitive short-name path (DURATION / DATE / TIME / DATE_TIME)
// surfaced by clinical_note.opt — each materialises as the matching
// DV wrapper, and the primitive ISO 8601 string is set via .value.
func TestEnsureSingleDVTemporal(t *testing.T) {
	cases := []struct {
		name string
		ctor func() any
		want string
		get  func(any) string
	}{
		{"DV_DATE", func() any { return &rm.DVDate{} }, "2020-01-01", func(v any) string { return v.(*rm.DVDate).Value }},
		{"DV_TIME", func() any { return &rm.DVTime{} }, "12:00:00", func(v any) string { return v.(*rm.DVTime).Value }},
		{"DV_DATE_TIME", func() any { return &rm.DVDateTime{} }, "2020-01-01T12:00:00Z", func(v any) string { return v.(*rm.DVDateTime).Value }},
		{"DV_DURATION", func() any { return &rm.DVDuration{} }, "P0D", func(v any) string { return v.(*rm.DVDuration).Value }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.ctor()
			if err := EnsureSingle(p, tc.name, "value", tc.want); err != nil {
				t.Fatalf("EnsureSingle: %v", err)
			}
			if got := tc.get(p); got != tc.want {
				t.Errorf("value = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEnsureSingleDVBoolean pins the DV_BOOLEAN writer parallel to
// the temporal set — bool primitive, not string.
func TestEnsureSingleDVBoolean(t *testing.T) {
	b := &rm.DVBoolean{}
	if err := EnsureSingle(b, "DV_BOOLEAN", "value", true); err != nil {
		t.Fatalf("EnsureSingle: %v", err)
	}
	if !b.Value {
		t.Errorf("Value = %v, want true", b.Value)
	}
}

func TestEnsureSingleUnknownParent(t *testing.T) {
	err := EnsureSingle(struct{}{}, "FOO", "bar", "baz")
	if !errors.Is(err, ErrUnknownAttribute) {
		t.Fatalf("want ErrUnknownAttribute, got %v", err)
	}
}

func TestEnsureSingleTypeMismatch(t *testing.T) {
	e := &rm.Element{}
	err := EnsureSingle(e, "ELEMENT", "value", "not a DataValue")
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("want ErrTypeMismatch, got %v", err)
	}
}

func TestAppendMultipleUnknownAttr(t *testing.T) {
	c := &rm.Composition{}
	err := AppendMultiple(c, "COMPOSITION", "no_such_attr", &rm.Observation{})
	if !errors.Is(err, ErrUnknownAttribute) {
		t.Fatalf("want ErrUnknownAttribute, got %v", err)
	}
}
