package rmaccess

import (
	"maps"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// TestPresence_REQ171_EachReading pins REQ-171 Structural access,
// Acceptance: a test pins each presence reading Structural access names and
// fails when one changes. Each row reads one attribute through Single and
// names the reading it pins. The list and hash readings are pinned by
// TestPresence_REQ171_ListReadings and TestPresence_REQ171_HashReadings.
func TestPresence_REQ171_EachReading(t *testing.T) {
	empty := ""
	text := "text"
	code := rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "433"}
	hierID := &rm.HierObjectID{Value: "9fcf1d0e-0000-4000-8000-000000000001"}
	partyRef := func(id rm.ObjectID, namespace, typ string) rm.PartyRef {
		return rm.PartyRef{ID: id, Namespace: namespace, Type: typ}
	}
	versionID := &rm.ObjectVersionID{Value: "9fcf1d0e-0000-4000-8000-000000000002::local::1"}
	locatableRef := func(id rm.UIDBasedID, namespace, typ string) rm.LocatableRef {
		return rm.LocatableRef{ID: id, Namespace: namespace, Type: typ}
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

		// At an attribute the RM marks mandatory (COMPOSITION name), a DV_TEXT,
		// held directly or through an interface, is empty when its value is.
		{reading: "DV_TEXT nil", parent: &rm.Composition{}, attr: "name", want: false},
		{reading: "DV_TEXT nil pointer", parent: &rm.Composition{Name: (*rm.DVText)(nil)}, attr: "name", want: false},
		{reading: "DV_TEXT pointer with an empty value", parent: &rm.Composition{Name: &rm.DVText{}}, attr: "name", want: false},
		{reading: "DV_TEXT pointer with a value", parent: &rm.Composition{Name: &rm.DVText{Value: text}}, attr: "name", want: true},
		{reading: "DV_TEXT held as a value with an empty value", parent: &rm.Composition{Name: rm.DVText{}}, attr: "name", want: false},
		{reading: "DV_TEXT held as a value with a value", parent: &rm.Composition{Name: rm.DVText{Value: text}}, attr: "name", want: true},
		{reading: "DV_CODED_TEXT as a DV_TEXT, a code but an empty value", parent: &rm.Composition{Name: &rm.DVCodedText{DefiningCode: code}}, attr: "name", want: false},
		{reading: "DV_CODED_TEXT as a DV_TEXT with a value", parent: &rm.Composition{Name: &rm.DVCodedText{Value: text}}, attr: "name", want: true},

		// At an optional attribute (ELEMENT null_reason, AUDIT_DETAILS
		// description), a DV_TEXT takes the reading of an interface: present
		// whatever it holds, unless nil or a nil pointer, so the walk enters it.
		{reading: "optional DV_TEXT nil", parent: &rm.Element{}, attr: "null_reason", want: false},
		{reading: "optional DV_TEXT nil pointer", parent: &rm.Element{NullReason: (*rm.DVText)(nil)}, attr: "null_reason", want: false},
		{reading: "optional DV_TEXT pointer with an empty value", parent: &rm.Element{NullReason: &rm.DVText{}}, attr: "null_reason", want: true},
		{reading: "optional DV_TEXT held as a value with an empty value", parent: &rm.AuditDetails{Description: rm.DVText{}}, attr: "description", want: true},
		{reading: "optional DV_CODED_TEXT as a DV_TEXT, a code but an empty value", parent: &rm.Element{NullReason: &rm.DVCodedText{DefiningCode: code}}, attr: "null_reason", want: true},

		// At an attribute the RM marks mandatory (COMPOSITION category), a
		// DV_CODED_TEXT held by value is empty when its value and its defining
		// code are.
		{reading: "DV_CODED_TEXT empty", parent: &rm.Composition{}, attr: "category", want: false},
		{reading: "DV_CODED_TEXT empty but not zero", parent: &rm.Composition{Category: rm.DVCodedText{Formatting: &text}}, attr: "category", want: false},
		{reading: "DV_CODED_TEXT with a value only", parent: &rm.Composition{Category: rm.DVCodedText{Value: "event"}}, attr: "category", want: true},
		{reading: "DV_CODED_TEXT with a code_string only", parent: &rm.Composition{Category: rm.DVCodedText{DefiningCode: rm.CodePhrase{CodeString: "433"}}}, attr: "category", want: true},
		{reading: "DV_CODED_TEXT with a terminology only", parent: &rm.Composition{Category: rm.DVCodedText{DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}}}}, attr: "category", want: true},

		// At an attribute the RM marks mandatory (COMPOSITION language), a
		// CODE_PHRASE held by value is empty when its code_string and its
		// terminology are.
		{reading: "CODE_PHRASE empty", parent: &rm.Composition{}, attr: "language", want: false},
		{reading: "CODE_PHRASE empty but not zero", parent: &rm.Composition{Language: rm.CodePhrase{PreferredTerm: &text}}, attr: "language", want: false},
		{reading: "CODE_PHRASE with a code_string only", parent: &rm.Composition{Language: rm.CodePhrase{CodeString: "en"}}, attr: "language", want: true},
		{reading: "CODE_PHRASE with a terminology only", parent: &rm.Composition{Language: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}}}, attr: "language", want: true},

		// An object reference held by value is present unless it is its zero
		// value, so the walk enters one with a part missing and the reference
		// entry reports the part. PARTY_REF (ROLE performer):
		{reading: "PARTY_REF complete", parent: &rm.Role{Performer: partyRef(hierID, "local", "PERSON")}, attr: "performer", want: true},
		{reading: "PARTY_REF zero", parent: &rm.Role{}, attr: "performer", want: false},
		{reading: "PARTY_REF holding only a nil-pointer id", parent: &rm.Role{Performer: partyRef((*rm.HierObjectID)(nil), "", "")}, attr: "performer", want: false},
		{reading: "PARTY_REF without an id", parent: &rm.Role{Performer: partyRef(nil, "local", "PERSON")}, attr: "performer", want: true},
		{reading: "PARTY_REF with a nil-pointer id", parent: &rm.Role{Performer: partyRef((*rm.HierObjectID)(nil), "local", "PERSON")}, attr: "performer", want: true},
		{reading: "PARTY_REF without a namespace", parent: &rm.Role{Performer: partyRef(hierID, "", "PERSON")}, attr: "performer", want: true},
		{reading: "PARTY_REF without a type", parent: &rm.Role{Performer: partyRef(hierID, "local", "")}, attr: "performer", want: true},
		// LOCATABLE_REF (INSTRUCTION_DETAILS instruction_id), which redefines id
		// as a UID_BASED_ID beside the OBJECT_REF id it embeds:
		{reading: "LOCATABLE_REF complete", parent: &rm.InstructionDetails{InstructionID: locatableRef(versionID, "local", "INSTRUCTION")}, attr: "instruction_id", want: true},
		{reading: "LOCATABLE_REF zero", parent: &rm.InstructionDetails{}, attr: "instruction_id", want: false},
		{reading: "LOCATABLE_REF without an id", parent: &rm.InstructionDetails{InstructionID: locatableRef(nil, "local", "INSTRUCTION")}, attr: "instruction_id", want: true},
		{reading: "LOCATABLE_REF with a nil-pointer id", parent: &rm.InstructionDetails{InstructionID: locatableRef((*rm.ObjectVersionID)(nil), "local", "INSTRUCTION")}, attr: "instruction_id", want: true},
		{reading: "LOCATABLE_REF without a namespace", parent: &rm.InstructionDetails{InstructionID: locatableRef(versionID, "", "INSTRUCTION")}, attr: "instruction_id", want: true},
		{reading: "LOCATABLE_REF without a type", parent: &rm.InstructionDetails{InstructionID: locatableRef(versionID, "local", "")}, attr: "instruction_id", want: true},
		{reading: "LOCATABLE_REF with only a path", parent: &rm.InstructionDetails{InstructionID: rm.LocatableRef{Path: &text}}, attr: "instruction_id", want: true},
		{reading: "LOCATABLE_REF with only the embedded OBJECT_REF id", parent: &rm.InstructionDetails{InstructionID: rm.LocatableRef{ObjectRef: rm.ObjectRef{ID: hierID}}}, attr: "instruction_id", want: true},

		// Any other class held by value: empty when it is its zero value.
		{reading: "DV_DATE_TIME zero", parent: &rm.EventContext{}, attr: "start_time", want: false},
		{reading: "DV_DATE_TIME with a value", parent: &rm.EventContext{StartTime: rm.DVDateTime{Value: "2026-10-13T10:00:00Z"}}, attr: "start_time", want: true},
		{reading: "DV_DATE_TIME with an empty value but not zero", parent: &rm.EventContext{StartTime: rm.DVDateTime{MagnitudeStatus: &text}}, attr: "start_time", want: true},
		// An interface field holding a nil pointer counts as zero: an
		// ELEMENT whose only content is a nil-pointer value, and an interval
		// whose bounds are nil pointers, are zero values.
		{reading: "ELEMENT held by value holding only a nil-pointer value", parent: &rm.ItemSingle{Item: rm.Element{Value: (*rm.DVQuantity)(nil)}}, attr: "item", want: false},
		{reading: "ELEMENT held by value holding a value", parent: &rm.ItemSingle{Item: rm.Element{Value: &rm.DVQuantity{}}}, attr: "item", want: true},
		{reading: "interval held by value with nil-pointer bounds", parent: &rm.ReferenceRange[rm.DVCount]{Range: rm.DVInterval[rm.DVOrdered]{Lower: (*rm.DVCount)(nil), Upper: (*rm.DVCount)(nil)}}, attr: "range", want: false},
		{reading: "interval held by value with a bound", parent: &rm.ReferenceRange[rm.DVCount]{Range: rm.DVInterval[rm.DVOrdered]{Lower: &rm.DVCount{}, Upper: (*rm.DVCount)(nil)}}, attr: "range", want: true},
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

// TestPresence_REQ171_ListReadings pins the list readings through Multiple
// and Len: a list is present when it is not nil, and empty when it is present
// and holds no item. Len counts what Multiple reads.
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
			items, present := Multiple(tc.parent, "content")
			if present != tc.wantPresent || len(items) != tc.wantItems {
				t.Errorf("Multiple(COMPOSITION, content) = (%d items, %t), want (%d, %t)", len(items), present, tc.wantItems, tc.wantPresent)
			}
			if !present && items != nil {
				t.Errorf("Multiple(COMPOSITION, content) = (%v, false), want nil items when absent", items)
			}
			if n, present := Len(tc.parent, "content"); n != tc.wantItems || present != tc.wantPresent {
				t.Errorf("Len(COMPOSITION, content) = (%d, %t), want (%d, %t)", n, present, tc.wantItems, tc.wantPresent)
			}
		})
	}
	if items, present := Multiple(&rm.Composition{Content: []rm.ContentItem{}}, "name"); items != nil || present {
		t.Errorf("Multiple(COMPOSITION, name) = (%v, %t), want (nil, false) for a single-valued attribute", items, present)
	}
	if n, present := Len(&rm.Composition{Content: []rm.ContentItem{}}, "name"); n != 0 || present {
		t.Errorf("Len(COMPOSITION, name) = (%d, %t), want (0, false) for a single-valued attribute", n, present)
	}
}

// TestLen_REQ171_CountsAListOfAPrimitiveTypeWithoutBoxing pins that Len
// reads a list of a primitive type, DV_MULTIMEDIA data, an Array of Octet,
// as Multiple does, absent, empty or holding its bytes, and counts a
// mebibyte of it while allocating less than the list's size: it boxes no
// byte, where Multiple hands each one back as an item.
func TestLen_REQ171_CountsAListOfAPrimitiveTypeWithoutBoxing(t *testing.T) {
	for _, tc := range []struct {
		name        string
		data        []byte
		wantN       int
		wantPresent bool
	}{
		{name: "absent", data: nil, wantN: 0, wantPresent: false},
		{name: "empty", data: []byte{}, wantN: 0, wantPresent: true},
		{name: "three bytes", data: []byte{1, 2, 3}, wantN: 3, wantPresent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := &rm.DVMultimedia{Data: tc.data}
			n, present := Len(parent, "data")
			items, mPresent := Multiple(parent, "data")
			if n != tc.wantN || present != tc.wantPresent {
				t.Errorf("Len(DV_MULTIMEDIA, data) = (%d, %t), want (%d, %t)", n, present, tc.wantN, tc.wantPresent)
			}
			if len(items) != n || mPresent != present {
				t.Errorf("Multiple(DV_MULTIMEDIA, data) = (%d items, %t), want what Len reads, (%d, %t)", len(items), mPresent, n, present)
			}
		})
	}
	const size = 1 << 20
	parent := &rm.DVMultimedia{Data: make([]byte, size)}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	n, present := Len(parent, "data")
	runtime.ReadMemStats(&after)
	if n != size || !present {
		t.Errorf("Len(DV_MULTIMEDIA with %d bytes of data, data) = (%d, %t), want (%d, true)", size, n, present, size)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated >= size {
		t.Errorf("Len(DV_MULTIMEDIA with %d bytes of data, data) allocated %d bytes, want fewer than %d: it boxes the bytes", size, allocated, size)
	}
}

// TestPresence_REQ171_HashReadings pins the hash readings through Hash: a
// hash is present when it is not nil, held directly or through a pointer that
// is not nil, and empty when it is present and holds no entry. Its entries
// come back in ascending key order, a value of a class or a primitive as a
// pointer to a copy that the access covers, a value held as an interface as
// it is. Single serves the same attribute as the whole map, with the same
// presence.
func TestPresence_REQ171_HashReadings(t *testing.T) {
	var nilMap map[string]string
	type want struct {
		present bool
		keys    []string
	}
	cases := []struct {
		reading string
		parent  any
		attr    string
		want    want
	}{
		{reading: "nil map", parent: &rm.ResourceDescription{}, attr: "details", want: want{present: false}},
		{reading: "nil pointer to a map", parent: &rm.ResourceDescription{}, attr: "other_details", want: want{present: false}},
		{reading: "empty map", parent: &rm.ResourceDescription{Details: map[string]rm.ResourceDescriptionItem{}}, attr: "details", want: want{present: true}},
		{reading: "pointer to an empty map", parent: &rm.ResourceDescriptionItem{OtherDetails: &map[string]string{}}, attr: "other_details", want: want{present: true}},
		{reading: "pointer to a nil map", parent: &rm.TranslationDetails{OtherDetails: &nilMap}, attr: "other_details", want: want{present: true}},
		{
			reading: "map with entries",
			parent:  &rm.TranslationDetails{Author: map[string]string{"organisation": "o", "name": "n", "email": "e"}},
			attr:    "author",
			want:    want{present: true, keys: []string{"email", "name", "organisation"}},
		},
		{
			reading: "pointer to a map with entries",
			parent:  &rm.ResourceDescription{OtherDetails: &map[string]string{"b": "2", "a": "1"}},
			attr:    "other_details",
			want:    want{present: true, keys: []string{"a", "b"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.reading, func(t *testing.T) {
			keys, values, present := Hash(tc.parent, tc.attr)
			if present != tc.want.present || !slices.Equal(keys, tc.want.keys) || len(values) != len(keys) {
				t.Errorf("Hash(%T, %q) = (%q, %d values, %t), want (%q, %d values, %t)",
					tc.parent, tc.attr, keys, len(values), present, tc.want.keys, len(tc.want.keys), tc.want.present)
			}
			if _, singlePresent := Single(tc.parent, tc.attr); singlePresent != present {
				t.Errorf("Single(%T, %q) present = %t, Hash present = %t; one attribute has one presence", tc.parent, tc.attr, singlePresent, present)
			}
		})
	}

	// Entries of an RM class: pointers to copies, in key order, that a walk
	// can enter.
	desc := &rm.ResourceDescription{Details: map[string]rm.ResourceDescriptionItem{
		"nl": {Purpose: "doel"},
		"en": {Purpose: "purpose"},
	}}
	keys, values, present := Hash(desc, "details")
	if !present || !slices.Equal(keys, []string{"en", "nl"}) || len(values) != 2 {
		t.Fatalf("Hash(RESOURCE_DESCRIPTION, details) = (%q, %d values, %t), want ([en nl], 2 values, true)", keys, len(values), present)
	}
	for i, v := range values {
		item, ok := v.(*rm.ResourceDescriptionItem)
		if !ok || item.Purpose != desc.Details[keys[i]].Purpose || !Covers(v) {
			t.Errorf("Hash(RESOURCE_DESCRIPTION, details) value %d = %#v (covered %t), want a *rm.ResourceDescriptionItem for key %q", i, v, Covers(v), keys[i])
			continue
		}
		item.Purpose = "changed"
		if desc.Details[keys[i]].Purpose == "changed" {
			t.Errorf("a change through Hash value %q reached the map; a value is a copy", keys[i])
		}
	}

	// Entries of a primitive: pointers to copies.
	_, values, _ = Hash(&rm.TranslationDetails{Author: map[string]string{"name": "n"}}, "author")
	if len(values) != 1 {
		t.Fatalf("Hash(TRANSLATION_DETAILS, author) has %d values, want 1", len(values))
	}
	if s, ok := values[0].(*string); !ok || *s != "n" {
		t.Errorf("Hash(TRANSLATION_DETAILS, author) value = %#v, want a *string holding %q", values[0], "n")
	}

	// Entries held as an interface: as they are.
	nested := map[string]any{"ui": "passthrough"}
	_, values, _ = Hash(&rm.ResourceAnnotations{Documentation: map[string]any{"en": nested}}, "documentation")
	if len(values) != 1 {
		t.Fatalf("Hash(RESOURCE_ANNOTATIONS, documentation) has %d values, want 1", len(values))
	}
	if m, ok := values[0].(map[string]any); !ok || m["ui"] != "passthrough" {
		t.Errorf("Hash(RESOURCE_ANNOTATIONS, documentation) value = %#v, want the map[string]any as it is", values[0])
	}

	// A pair the class does not declare as a Hash.
	for _, attr := range []string{"lifecycle_state", "other_contributors", "no_such_attribute"} {
		if keys, values, present := Hash(&rm.ResourceDescription{LifecycleState: "draft", OtherContributors: []string{"x"}}, attr); keys != nil || values != nil || present {
			t.Errorf("Hash(RESOURCE_DESCRIPTION, %q) = (%q, %v, %t), want (nil, nil, false)", attr, keys, values, present)
		}
	}
}

// fieldByAttr returns the field of the struct v that holds the attribute
// attr, found by its JSON name among the fields a selector reaches, so a
// field the class declares shadows one an embedded class declares under the
// same name (LOCATABLE_REF id). v must be addressable for the field to be
// set.
func fieldByAttr(v reflect.Value, attr string) (reflect.Value, bool) {
	for _, f := range reflect.VisibleFields(v.Type()) {
		if f.Anonymous || !f.IsExported() {
			continue
		}
		if name, _, _ := strings.Cut(f.Tag.Get("json"), ","); name == attr {
			return v.FieldByIndex(f.Index), true
		}
	}
	return reflect.Value{}, false
}

// TestPresence_REQ171_ContentReadingsOnlyAtMandatoryAttributes pins REQ-171
// Structural access on every registered class: the content readings apply at
// an attribute the RM marks mandatory, and only there. Each single-valued
// attribute declared DV_TEXT, DV_CODED_TEXT or CODE_PHRASE is set to a value
// of its type that is not its zero value but whose content is missing: a
// DV_TEXT with an empty value and a formatting, a DV_CODED_TEXT with that and
// an empty defining code, a CODE_PHRASE with an empty code_string and
// terminology and a preferred term. Held by value or through an interface,
// it reads absent where rminfo marks the attribute required and present where
// it does not, so the walk enters it there; held by pointer, it reads present
// everywhere.
func TestPresence_REQ171_ContentReadingsOnlyAtMandatoryAttributes(t *testing.T) {
	formatting := "plain"
	contentless := map[string]any{
		"DV_TEXT":       rm.DVText{Formatting: &formatting},
		"DV_CODED_TEXT": rm.DVCodedText{Formatting: &formatting},
		"CODE_PHRASE":   rm.CodePhrase{PreferredTerm: &formatting},
	}
	lister, ok := rminfo.Default.(rminfo.AttributeLister)
	if !ok {
		t.Fatal("rminfo.Default does not list attribute names")
	}
	var mandatory, optional, pointers []string
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		required := rminfo.Default.RequiredAttributes(name)
		for _, attr := range lister.AttributeNames(name) {
			rmType, _ := rminfo.Default.AttributeRMType(name, attr)
			value, ok := contentless[rmType]
			if container, _ := rminfo.Default.IsContainer(name, attr); !ok || container {
				continue
			}
			parent := ctor()
			field, ok := fieldByAttr(reflect.ValueOf(parent).Elem(), attr)
			if !ok {
				t.Fatalf("%T has no field for %s.%s", parent, name, attr)
			}
			pair := name + "." + attr
			want := !slices.Contains(required, attr)
			switch field.Kind() {
			case reflect.Pointer:
				// A pointer takes the reading of its Go kind wherever it is.
				p := reflect.New(reflect.TypeOf(value))
				p.Elem().Set(reflect.ValueOf(value))
				field.Set(p)
				want = true
				pointers = append(pointers, pair)
			case reflect.Interface:
				p := reflect.New(reflect.TypeOf(value))
				p.Elem().Set(reflect.ValueOf(value))
				field.Set(p)
			default:
				field.Set(reflect.ValueOf(value))
			}
			if field.Kind() != reflect.Pointer {
				if want {
					optional = append(optional, pair)
				} else {
					mandatory = append(mandatory, pair)
				}
			}
			if _, present := Single(parent, attr); present != want {
				t.Errorf("Single(%T with a %s whose content is missing, %q) present = %t, want %t (required: %t)", parent, rmType, attr, present, want, !want)
			}
		}
	}
	if len(mandatory) < 60 {
		t.Errorf("checked %d mandatory attributes, want the full RM set; rminfo has gone blind", len(mandatory))
	}
	for _, pair := range []string{"ELEMENT.null_reason", "AUDIT_DETAILS.description", "ATTESTATION.description"} {
		if !slices.Contains(optional, pair) {
			t.Errorf("the optional attributes checked, %q, lack %s", optional, pair)
		}
	}
	if !slices.Contains(pointers, "ELEMENT.null_flavour") {
		t.Errorf("the attributes held by pointer checked, %q, lack ELEMENT.null_flavour", pointers)
	}
}

// TestPresence_REQ171_ReferencesHeldByValue pins REQ-171 Structural access on
// every registered class: an object reference held by value is present unless
// it is its zero value, so the walk enters one with a part missing and
// REQ-112's reference entry reports the part. Each single-valued attribute
// whose declared type is OBJECT_REF, or a class rminfo places below it, held
// by value, reads absent when it is its zero value and when it holds only a
// nil-pointer id, and present with any one of its id, namespace and type set.
func TestPresence_REQ171_ReferencesHeldByValue(t *testing.T) {
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		t.Fatal("rminfo.Default answers no class hierarchy")
	}
	lister, ok := rminfo.Default.(rminfo.AttributeLister)
	if !ok {
		t.Fatal("rminfo.Default does not list attribute names")
	}
	id := &rm.HierObjectID{Value: "9fcf1d0e-0000-4000-8000-000000000001"}
	parts := map[string]any{"id": id, "namespace": "local", "type": "PERSON"}
	var checked []string
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		for _, attr := range lister.AttributeNames(name) {
			rmType, _ := rminfo.Default.AttributeRMType(name, attr)
			isRef, _ := h.ConformsTo(rmType, "OBJECT_REF")
			if container, _ := rminfo.Default.IsContainer(name, attr); !isRef || container {
				continue
			}
			// holding returns a fresh parent and its reference field.
			holding := func() (any, reflect.Value) {
				parent := ctor()
				field, ok := fieldByAttr(reflect.ValueOf(parent).Elem(), attr)
				if !ok {
					t.Fatalf("%T has no field for %s.%s", parent, name, attr)
				}
				return parent, field
			}
			if _, field := holding(); field.Kind() != reflect.Struct {
				continue // a pointer or an interface takes the reading of its Go kind
			}
			checked = append(checked, name+"."+attr)
			parent, _ := holding()
			if _, present := Single(parent, attr); present {
				t.Errorf("Single(%T holding a zero %s, %q) present = true, want false", parent, rmType, attr)
			}
			parent, field := holding()
			nilID, _ := fieldByAttr(field, "id")
			nilID.Set(reflect.Zero(reflect.TypeOf(id)))
			if _, present := Single(parent, attr); present {
				t.Errorf("Single(%T holding a %s with only a nil-pointer id, %q) present = true, want false", parent, rmType, attr)
			}
			for _, part := range slices.Sorted(maps.Keys(parts)) {
				parent, field := holding()
				f, ok := fieldByAttr(field, part)
				if !ok {
					t.Fatalf("%s has no field for its %s", rmType, part)
				}
				f.Set(reflect.ValueOf(parts[part]))
				if _, present := Single(parent, attr); !present {
					t.Errorf("Single(%T holding a %s with only its %s, %q) present = false, want true", parent, rmType, part, attr)
				}
			}
		}
	}
	for _, pair := range []string{"ROLE.performer", "PARTY_RELATIONSHIP.source", "PARTY_RELATIONSHIP.target", "INSTRUCTION_DETAILS.instruction_id"} {
		if !slices.Contains(checked, pair) {
			t.Errorf("the references held by value checked, %q, lack %s", checked, pair)
		}
	}
}

// TestAccess_REQ171_Shapes pins the six attributes, of five classes, whose
// value comes back in another shape than the field's own, and the boxing of list items: a list of
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
