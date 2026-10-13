package rmaccess

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestPresence_REQ171_EachReading pins REQ-171 Structural access,
// Acceptance: a test pins each presence reading Structural access names and
// fails when one changes. Each row reads one attribute through Single and
// names the reading it pins; the list readings, which Multiple does not
// report, are read through multiple's presence answer.
func TestPresence_REQ171_EachReading(t *testing.T) {
	empty := ""
	text := "text"
	code := rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "433"}
	hierID := &rm.HierObjectID{Value: "9fcf1d0e-0000-4000-8000-000000000001"}
	partyRef := func(id rm.ObjectID, namespace, typ string) rm.PartyRef {
		return rm.PartyRef{ID: id, Namespace: namespace, Type: typ}
	}
	quantityInterval := func(unbounded bool, lower rm.DVQuantity) *rm.DVInterval[rm.DVQuantity] {
		return &rm.DVInterval[rm.DVQuantity]{Lower: lower, LowerUnbounded: unbounded, UpperUnbounded: true}
	}
	orderedInterval := func(unbounded bool, lower rm.DVOrdered) *rm.DVInterval[rm.DVOrdered] {
		return &rm.DVInterval[rm.DVOrdered]{Lower: lower, LowerUnbounded: unbounded, UpperUnbounded: true}
	}

	cases := []struct {
		reading string
		parent  any
		attr    string
		want    bool
	}{
		// A pointer is present when it is not nil.
		{reading: "nil pointer", parent: &rm.Composition{}, attr: "context", want: false},
		{reading: "pointer to an empty value", parent: &rm.Composition{Context: &rm.EventContext{}}, attr: "context", want: true},
		{reading: "pointer to an empty string (PARTY_IDENTIFIED name)", parent: &rm.PartyIdentified{Name: &empty}, attr: "name", want: true},
		{reading: "pointer to an empty string (EVENT_CONTEXT location)", parent: &rm.EventContext{Location: &empty}, attr: "location", want: true},
		{reading: "pointer to an empty string (CODE_PHRASE preferred_term)", parent: &rm.CodePhrase{PreferredTerm: &empty}, attr: "preferred_term", want: true},
		{reading: "pointer to a string", parent: &rm.PartyIdentified{Name: &text}, attr: "name", want: true},

		// An interface is present when it holds a value that is not a nil pointer.
		{reading: "nil interface", parent: &rm.Element{}, attr: "value", want: false},
		{reading: "interface holding a nil pointer", parent: &rm.Element{Value: (*rm.DVQuantity)(nil)}, attr: "value", want: false},
		{reading: "interface holding a pointer to an empty value", parent: &rm.Element{Value: &rm.DVQuantity{}}, attr: "value", want: true},
		{reading: "interface holding an empty value", parent: &rm.Element{Value: rm.DVBoolean{}}, attr: "value", want: true},

		// A string is present when it is not empty.
		{reading: "empty string", parent: &rm.Composition{}, attr: "archetype_node_id", want: false},
		{reading: "string", parent: &rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}, attr: "archetype_node_id", want: true},

		// A number or a boolean is always present.
		{reading: "number zero", parent: &rm.DVCount{}, attr: "magnitude", want: true},
		{reading: "boolean false", parent: &rm.DVBoolean{}, attr: "value", want: true},

		// DV_TEXT, held directly or through an interface: empty when its value is.
		{reading: "DV_TEXT nil", parent: &rm.Composition{}, attr: "name", want: false},
		{reading: "DV_TEXT nil pointer", parent: &rm.Composition{Name: (*rm.DVText)(nil)}, attr: "name", want: false},
		{reading: "DV_TEXT pointer with an empty value", parent: &rm.Composition{Name: &rm.DVText{}}, attr: "name", want: false},
		{reading: "DV_TEXT pointer with a value", parent: &rm.Composition{Name: &rm.DVText{Value: text}}, attr: "name", want: true},
		{reading: "DV_TEXT held as a value with an empty value", parent: &rm.Composition{Name: rm.DVText{}}, attr: "name", want: false},
		{reading: "DV_TEXT held as a value with a value", parent: &rm.Composition{Name: rm.DVText{Value: text}}, attr: "name", want: true},
		{reading: "DV_CODED_TEXT as a DV_TEXT, a code but an empty value", parent: &rm.Composition{Name: &rm.DVCodedText{DefiningCode: code}}, attr: "name", want: false},
		{reading: "DV_CODED_TEXT as a DV_TEXT with a value", parent: &rm.Composition{Name: &rm.DVCodedText{Value: text}}, attr: "name", want: true},

		// DV_CODED_TEXT held by value: empty when its value and its defining code are.
		{reading: "DV_CODED_TEXT empty", parent: &rm.Composition{}, attr: "category", want: false},
		{reading: "DV_CODED_TEXT with a value only", parent: &rm.Composition{Category: rm.DVCodedText{Value: "event"}}, attr: "category", want: true},
		{reading: "DV_CODED_TEXT with a code_string only", parent: &rm.Composition{Category: rm.DVCodedText{DefiningCode: rm.CodePhrase{CodeString: "433"}}}, attr: "category", want: true},
		{reading: "DV_CODED_TEXT with a terminology only", parent: &rm.Composition{Category: rm.DVCodedText{DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}}}}, attr: "category", want: true},

		// CODE_PHRASE held by value: empty when its code_string and its terminology are.
		{reading: "CODE_PHRASE empty", parent: &rm.Composition{}, attr: "language", want: false},
		{reading: "CODE_PHRASE with a code_string only", parent: &rm.Composition{Language: rm.CodePhrase{CodeString: "en"}}, attr: "language", want: true},
		{reading: "CODE_PHRASE with a terminology only", parent: &rm.Composition{Language: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}}}, attr: "language", want: true},

		// PARTY_REF held by value: empty when its id, namespace or type is missing.
		{reading: "PARTY_REF complete", parent: &rm.Role{Performer: partyRef(hierID, "local", "PERSON")}, attr: "performer", want: true},
		{reading: "PARTY_REF without an id", parent: &rm.Role{Performer: partyRef(nil, "local", "PERSON")}, attr: "performer", want: false},
		{reading: "PARTY_REF with a nil-pointer id", parent: &rm.Role{Performer: partyRef((*rm.HierObjectID)(nil), "local", "PERSON")}, attr: "performer", want: false},
		{reading: "PARTY_REF without a namespace", parent: &rm.Role{Performer: partyRef(hierID, "", "PERSON")}, attr: "performer", want: false},
		{reading: "PARTY_REF without a type", parent: &rm.Role{Performer: partyRef(hierID, "local", "")}, attr: "performer", want: false},

		// Any other class held by value: empty when it is its zero value.
		{reading: "DV_DATE_TIME zero", parent: &rm.EventContext{}, attr: "start_time", want: false},
		{reading: "DV_DATE_TIME with a value", parent: &rm.EventContext{StartTime: rm.DVDateTime{Value: "2026-10-13T10:00:00Z"}}, attr: "start_time", want: true},
		{reading: "DV_DATE_TIME with an empty value but not zero", parent: &rm.EventContext{StartTime: rm.DVDateTime{MagnitudeStatus: &text}}, attr: "start_time", want: true},
		{reading: "HISTORY zero", parent: &rm.Observation{}, attr: "data", want: false},
		{reading: "HISTORY with a name only", parent: &rm.Observation{Data: rm.History[rm.ItemStructure]{Name: &rm.DVText{Value: text}}}, attr: "data", want: true},

		// Fixed: an interval bound is absent only when its side is marked
		// unbounded and the bound is empty.
		{reading: "interval bound on an open side, empty", parent: quantityInterval(true, rm.DVQuantity{}), attr: "lower", want: false},
		{reading: "interval bound on an open side, not empty", parent: quantityInterval(true, rm.DVQuantity{Units: "kg"}), attr: "lower", want: true},
		{reading: "interval bound on a closed side, empty", parent: quantityInterval(false, rm.DVQuantity{}), attr: "lower", want: true},
		{reading: "interface interval bound on an open side, nil", parent: orderedInterval(true, nil), attr: "lower", want: false},
		{reading: "interface interval bound on an open side, nil pointer", parent: orderedInterval(true, (*rm.DVCount)(nil)), attr: "lower", want: false},
		{reading: "interface interval bound on an open side, a value", parent: orderedInterval(true, &rm.DVCount{}), attr: "lower", want: true},
		{reading: "interface interval bound on a closed side, nil", parent: orderedInterval(false, nil), attr: "lower", want: true},

		// Fixed: EHR_STATUS subject is always present.
		{reading: "EHR_STATUS subject zero", parent: &rm.EHRStatus{}, attr: "subject", want: true},

		// Fixed: ARCHETYPED archetype_id and rm_version are always present.
		{reading: "ARCHETYPED archetype_id empty", parent: &rm.Archetyped{}, attr: "archetype_id", want: true},
		{reading: "ARCHETYPED rm_version empty", parent: &rm.Archetyped{}, attr: "rm_version", want: true},
		{reading: "ARCHETYPED template_id nil", parent: &rm.Archetyped{}, attr: "template_id", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.reading, func(t *testing.T) {
			value, present := Single(tc.parent, tc.attr)
			if present != tc.want {
				t.Errorf("Single(%T, %q) present = %t, want %t", tc.parent, tc.attr, present, tc.want)
			}
			if !present && value != nil {
				t.Errorf("Single(%T, %q) = (%v, false), want a nil value when absent", tc.parent, tc.attr, value)
			}
		})
	}
}

// TestPresence_REQ171_ListReadings pins the list readings: a list is present
// when it is not nil, and empty when it is present and holds no item. Multiple
// answers ok for all three, as the RM floor reads it.
func TestPresence_REQ171_ListReadings(t *testing.T) {
	cases := []struct {
		reading     string
		parent      *rm.Composition
		wantPresent bool
		wantItems   int
	}{
		{reading: "nil list", parent: &rm.Composition{}, wantPresent: false, wantItems: 0},
		{reading: "empty list", parent: &rm.Composition{Content: []rm.ContentItem{}}, wantPresent: true, wantItems: 0},
		{reading: "list with an item", parent: &rm.Composition{Content: []rm.ContentItem{&rm.Section{}}}, wantPresent: true, wantItems: 1},
	}
	for _, tc := range cases {
		t.Run(tc.reading, func(t *testing.T) {
			items, present, served := multiple(tc.parent, "content")
			if !served || present != tc.wantPresent || len(items) != tc.wantItems {
				t.Errorf("multiple(COMPOSITION, content) = (%d items, present %t, served %t), want (%d, %t, true)",
					len(items), present, served, tc.wantItems, tc.wantPresent)
			}
			if items, ok := Multiple(tc.parent, "content"); !ok || len(items) != tc.wantItems {
				t.Errorf("Multiple(COMPOSITION, content) = (%d items, %t), want (%d, true)", len(items), ok, tc.wantItems)
			}
		})
	}
}

// TestPresence_REQ171_ObjectRef pins the OBJECT_REF content rule, which no
// attribute of the pinned RM holds by value, so no Single row reaches it.
func TestPresence_REQ171_ObjectRef(t *testing.T) {
	id := &rm.HierObjectID{Value: "9fcf1d0e-0000-4000-8000-000000000001"}
	cases := []struct {
		reading string
		ref     rm.ObjectRef
		want    bool
	}{
		{reading: "complete", ref: rm.ObjectRef{ID: id, Namespace: "local", Type: "PERSON"}, want: true},
		{reading: "without an id", ref: rm.ObjectRef{Namespace: "local", Type: "PERSON"}, want: false},
		{reading: "with a nil-pointer id", ref: rm.ObjectRef{ID: (*rm.HierObjectID)(nil), Namespace: "local", Type: "PERSON"}, want: false},
		{reading: "without a namespace", ref: rm.ObjectRef{ID: id, Type: "PERSON"}, want: false},
		{reading: "without a type", ref: rm.ObjectRef{ID: id, Namespace: "local"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.reading, func(t *testing.T) {
			if got := presentObjectRef(tc.ref); got != tc.want {
				t.Errorf("presentObjectRef(%+v) = %t, want %t", tc.ref, got, tc.want)
			}
		})
	}
}

// TestAccess_REQ171_Shapes pins the five attributes whose value comes back in
// another shape than the field's own, and the boxing of list items: a list of
// values as pointers into its backing array, a list of interfaces as it is.
func TestAccess_REQ171_Shapes(t *testing.T) {
	precision := rm.Integer(2)
	step := rm.DVCodedText{Value: "step"}
	single := &rm.ItemSingle{Item: rm.Element{ArchetypeNodeID: "at1"}}
	ranged := &rm.ReferenceRange[rm.DVCount]{Range: rm.DVInterval[rm.DVOrdered]{LowerUnbounded: true, UpperUnbounded: true}}

	if v, _ := Single(single, "item"); v != &single.Item {
		t.Errorf("Single(ITEM_SINGLE, item) = %T %p, want a pointer to the parent's item %p", v, v, &single.Item)
	}
	if v, _ := Single(ranged, "range"); v != &ranged.Range {
		t.Errorf("Single(REFERENCE_RANGE, range) = %T, want a pointer to the parent's range", v)
	}
	for _, attr := range []string{"careflow_step", "transition"} {
		v, _ := Single(&rm.IsmTransition{CareflowStep: &step, Transition: &step}, attr)
		if got, ok := v.(rm.DVCodedText); !ok || got.Value != step.Value {
			t.Errorf("Single(ISM_TRANSITION, %s) = %#v, want the DV_CODED_TEXT it points to", attr, v)
		}
	}
	if v, _ := Single(&rm.DVProportion{Precision: &precision}, "precision"); v != any(precision) {
		t.Errorf("Single(DV_PROPORTION, precision) = %#v, want the Integer it points to", v)
	}
	if v, _ := Single(&rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}}, "terminology_id"); v != any("openehr") {
		t.Errorf("Single(CODE_PHRASE, terminology_id) = %#v, want the string its TERMINOLOGY_ID holds", v)
	}

	tree := &rm.ItemList{Items: []rm.Element{{ArchetypeNodeID: "at1"}}}
	if items, _ := Multiple(tree, "items"); len(items) != 1 || items[0] != &tree.Items[0] {
		t.Errorf("Multiple(ITEM_LIST, items) = %v, want a pointer into the backing array", items)
	}
	section := &rm.Section{}
	if items, _ := Multiple(&rm.Composition{Content: []rm.ContentItem{section}}, "content"); len(items) != 1 || items[0] != any(section) {
		t.Errorf("Multiple(COMPOSITION, content) = %v, want the item as it is", items)
	}
}
