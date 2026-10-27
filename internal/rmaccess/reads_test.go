package rmaccess

// reads_test.go: what the generated access reads for the attributes the
// validators walk (REQ-102, REQ-107, REQ-112): each attribute reads
// present when set and absent when empty, by the readings of REQ-171
// Structural access, and a set one gives back the value the parent holds, in
// the pointer and the value form of the parent. These rows carry over the
// reader tests of the hand-written readers the access replaced.

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// bothForms returns v and a pointer to a copy of it, so a case written once
// is read in the value and the pointer form the access both accepts. A
// pointer comes back as itself and as the value it points to.
func bothForms(v any) []any {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		return []any{v, rv.Elem().Interface()}
	}
	ptr := reflect.New(rv.Type())
	ptr.Elem().Set(rv)
	return []any{v, ptr.Interface()}
}

// readCase is one attribute read through Single: whether it is present and,
// when want is set, the value it gives back, compared with == when the value
// is comparable and with reflect.DeepEqual otherwise. wantType, when set, is
// the Go type of the value. byAddress marks an attribute that holds a class
// by value, which comes back as a pointer into the parent: want is then the
// value that pointer points to.
type readCase struct {
	name      string
	parent    any
	attr      string
	present   bool
	want      any
	wantType  string
	byAddress bool
}

// checkSingle reads tc through Single on each form of its parent.
func checkSingle(t *testing.T, tc readCase, forms []any) {
	t.Helper()
	for _, form := range forms {
		got, present := Single(form, tc.attr)
		if present != tc.present {
			t.Errorf("Single(%T, %q) present = %t, want %t (value %#v)", form, tc.attr, present, tc.present, got)
			continue
		}
		if !present {
			continue
		}
		if tc.byAddress {
			ptr := reflect.ValueOf(got)
			if ptr.Kind() != reflect.Pointer || ptr.IsNil() {
				t.Errorf("Single(%T, %q) = %#v, want a pointer to the value the parent holds", form, tc.attr, got)
				continue
			}
			got = ptr.Elem().Interface()
		}
		if tc.wantType != "" && fmt.Sprintf("%T", got) != tc.wantType {
			t.Errorf("Single(%T, %q) = %T, want a %s", form, tc.attr, got, tc.wantType)
		}
		if tc.want != nil && !sameValue(got, tc.want) {
			t.Errorf("Single(%T, %q) = %#v, want %#v", form, tc.attr, got, tc.want)
		}
	}
}

// sameValue reports whether got is want: the same pointer, or an equal
// value.
func sameValue(got, want any) bool {
	if reflect.TypeOf(want).Comparable() && reflect.TypeOf(got) == reflect.TypeOf(want) {
		return got == want
	}
	return reflect.DeepEqual(got, want)
}

// TestAccess_REQ102_REQ110_ReadsTheTemplateWalkAttributes pins the
// attributes the template walk descends through (REQ-102): those of
// COMPOSITION, EVENT_CONTEXT, OBSERVATION, ACTION and ISM_TRANSITION, of the
// item structures, of DV_CODED_TEXT, of the data values whose scalar the
// template constrains, and of the demographic PARTY hierarchy, FOLDER and
// EHR_STATUS. Each reads present when set and absent when empty, in the
// pointer and the value form of its parent.
func TestAccess_REQ102_REQ110_ReadsTheTemplateWalkAttributes(t *testing.T) {
	coded := func(value, code string) rm.DVCodedText {
		return rm.DVCodedText{
			Value:        value,
			DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: code},
		}
	}
	tree := &rm.ItemTree{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "tree"}}
	comp := rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.encounter.v1",
		Name:            rm.DVText{Value: "Encounter"},
		Category:        coded("event", "433"),
		Composer:        rm.PartySelf{},
		Language:        rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: "en"},
		Territory:       rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"}, CodeString: "NL"},
		Context:         &rm.EventContext{},
	}
	step := coded("step", "245")
	termMapping := rm.TermMapping{
		Match:   rm.Character("="),
		Purpose: &rm.DVCodedText{Value: "billing"},
		Target:  rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "73211009"},
	}
	issuer := "issuer"
	cases := []readCase{
		// COMPOSITION: every attribute the walk reads, set and empty.
		{name: "COMPOSITION archetype_node_id", parent: comp, attr: "archetype_node_id", present: true, want: comp.ArchetypeNodeID},
		{name: "COMPOSITION name", parent: comp, attr: "name", present: true},
		{name: "COMPOSITION category", parent: comp, attr: "category", present: true, want: comp.Category, byAddress: true},
		{name: "COMPOSITION composer", parent: comp, attr: "composer", present: true, want: rm.PartySelf{}},
		{name: "COMPOSITION language", parent: comp, attr: "language", present: true, want: comp.Language, byAddress: true},
		{name: "COMPOSITION territory", parent: comp, attr: "territory", present: true, want: comp.Territory, byAddress: true},
		{name: "COMPOSITION context", parent: comp, attr: "context", present: true, want: comp.Context},
		{name: "empty COMPOSITION archetype_node_id", parent: rm.Composition{}, attr: "archetype_node_id"},
		{name: "empty COMPOSITION name", parent: rm.Composition{}, attr: "name"},
		{name: "empty COMPOSITION category", parent: rm.Composition{}, attr: "category"},
		{name: "empty COMPOSITION composer", parent: rm.Composition{}, attr: "composer"},
		{name: "empty COMPOSITION language", parent: rm.Composition{}, attr: "language"},
		{name: "empty COMPOSITION territory", parent: rm.Composition{}, attr: "territory"},
		{name: "empty COMPOSITION context", parent: rm.Composition{}, attr: "context"},

		// EVENT_CONTEXT's RM-mandatory start_time and setting.
		{name: "EVENT_CONTEXT start_time", parent: rm.EventContext{StartTime: rm.DVDateTime{Value: "2026-05-26T12:00:00Z"}}, attr: "start_time", present: true, want: rm.DVDateTime{Value: "2026-05-26T12:00:00Z"}, byAddress: true},
		{name: "EVENT_CONTEXT setting", parent: rm.EventContext{Setting: coded("other care", "238")}, attr: "setting", present: true, want: coded("other care", "238"), byAddress: true},
		{name: "empty EVENT_CONTEXT start_time", parent: rm.EventContext{}, attr: "start_time"},
		{name: "empty EVENT_CONTEXT setting", parent: rm.EventContext{}, attr: "setting"},

		// OBSERVATION: data present once its HISTORY is not empty; state and
		// protocol absent until set.
		{name: "OBSERVATION data", parent: rm.Observation{Data: rm.History[rm.ItemStructure]{ArchetypeNodeID: "at0001"}}, attr: "data", present: true, wantType: "rm.History[github.com/cadasto/openehr-sdk-go/openehr/rm.ItemStructure]", byAddress: true},
		{name: "OBSERVATION state unset", parent: rm.Observation{}, attr: "state"},
		{name: "OBSERVATION protocol unset", parent: rm.Observation{}, attr: "protocol"},
		{name: "OBSERVATION protocol set", parent: rm.Observation{Protocol: tree}, attr: "protocol", present: true, want: tree},

		// ACTION: time, ism_transition and instruction_details.
		{name: "ACTION time", parent: rm.Action{Time: rm.DVDateTime{Value: "2026-01-01T00:00:00Z"}}, attr: "time", present: true},
		{name: "ACTION ism_transition", parent: rm.Action{IsmTransition: rm.IsmTransition{CurrentState: coded("planned", "526")}}, attr: "ism_transition", present: true},
		{name: "ACTION instruction_details", parent: rm.Action{InstructionDetails: &rm.InstructionDetails{}}, attr: "instruction_details", present: true},
		{name: "empty ACTION time", parent: rm.Action{}, attr: "time"},
		{name: "empty ACTION ism_transition", parent: rm.Action{}, attr: "ism_transition"},
		{name: "empty ACTION instruction_details", parent: rm.Action{}, attr: "instruction_details"},

		// ISM_TRANSITION: current_state once it carries a value, careflow_step
		// and transition once set.
		{name: "ISM_TRANSITION current_state", parent: rm.IsmTransition{CurrentState: step}, attr: "current_state", present: true, want: step, byAddress: true},
		{name: "ISM_TRANSITION careflow_step", parent: rm.IsmTransition{CareflowStep: &step}, attr: "careflow_step", present: true, want: step},
		{name: "ISM_TRANSITION transition", parent: rm.IsmTransition{Transition: &step}, attr: "transition", present: true, want: step},
		{name: "empty ISM_TRANSITION current_state", parent: rm.IsmTransition{}, attr: "current_state"},
		{name: "empty ISM_TRANSITION careflow_step", parent: rm.IsmTransition{}, attr: "careflow_step"},
		{name: "empty ISM_TRANSITION transition", parent: rm.IsmTransition{}, attr: "transition"},

		// ITEM_SINGLE item, a pointer into the parent.
		{name: "ITEM_SINGLE item", parent: rm.ItemSingle{Item: rm.Element{ArchetypeNodeID: "at0010"}}, attr: "item", present: true, wantType: "*rm.Element"},

		// DV_CODED_TEXT defining_code, the CODE_PHRASE leaf.
		{name: "DV_CODED_TEXT defining_code", parent: coded("event", "433"), attr: "defining_code", present: true, want: coded("event", "433").DefiningCode, byAddress: true},
		{name: "empty DV_CODED_TEXT defining_code", parent: rm.DVCodedText{}, attr: "defining_code"},

		// TERM_MAPPING: match, purpose and target, set and at their zero
		// value (REQ-112).
		{name: "TERM_MAPPING match", parent: termMapping, attr: "match", present: true, want: rm.Character("=")},
		{name: "TERM_MAPPING purpose", parent: termMapping, attr: "purpose", present: true, want: termMapping.Purpose},
		{name: "TERM_MAPPING target", parent: termMapping, attr: "target", present: true, want: termMapping.Target, byAddress: true},
		{name: "empty TERM_MAPPING match", parent: rm.TermMapping{}, attr: "match"},
		{name: "empty TERM_MAPPING purpose", parent: rm.TermMapping{}, attr: "purpose"},
		{name: "empty TERM_MAPPING target", parent: rm.TermMapping{}, attr: "target"},

		// The data-value scalars a template constrains (REQ-102).
		{name: "DV_COUNT magnitude", parent: rm.DVCount{Magnitude: 5}, attr: "magnitude", present: true, want: rm.Integer64(5)},
		{name: "DV_QUANTITY magnitude", parent: rm.DVQuantity{Magnitude: 1.5}, attr: "magnitude", present: true, want: rm.Real(1.5)},
		{name: "DV_QUANTITY units", parent: rm.DVQuantity{Units: "kg"}, attr: "units", present: true, want: "kg"},
		{name: "DV_PROPORTION numerator", parent: rm.DVProportion{Numerator: 1, Denominator: 2}, attr: "numerator", present: true, want: rm.Real(1)},
		{name: "DV_PROPORTION denominator", parent: rm.DVProportion{Numerator: 1, Denominator: 2}, attr: "denominator", present: true, want: rm.Real(2)},
		{name: "DV_URI value", parent: rm.DVURI{Value: "http://example.com"}, attr: "value", present: true, want: "http://example.com"},
		{name: "DV_PARSABLE value", parent: rm.DVParsable{Value: "x", Formalism: "text/plain"}, attr: "value", present: true, want: "x"},
		{name: "DV_PARSABLE formalism", parent: rm.DVParsable{Value: "x", Formalism: "text/plain"}, attr: "formalism", present: true, want: "text/plain"},
		{name: "DV_INTERVAL<DV_QUANTITY> lower_unbounded", parent: rm.DVInterval[rm.DVQuantity]{}, attr: "lower_unbounded", present: true, want: false},
		{name: "DV_INTERVAL<DV_DATE> upper_included", parent: rm.DVInterval[rm.DVDate]{UpperIncluded: true}, attr: "upper_included", present: true, want: true},
		{name: "DV_DATE value", parent: rm.DVDate{Value: "1980-01-01"}, attr: "value", present: true, want: "1980-01-01"},
		{name: "DV_TIME value", parent: rm.DVTime{Value: "10:00:00"}, attr: "value", present: true, want: "10:00:00"},
		{name: "DV_DATE_TIME value", parent: rm.DVDateTime{Value: "1980-01-01T10:00:00Z"}, attr: "value", present: true, want: "1980-01-01T10:00:00Z"},
		{name: "DV_DURATION value", parent: rm.DVDuration{Value: "P1Y"}, attr: "value", present: true, want: "P1Y"},
		{name: "DV_BOOLEAN false value", parent: rm.DVBoolean{}, attr: "value", present: true, want: false},
		{name: "DV_IDENTIFIER id", parent: rm.DVIdentifier{ID: "abc"}, attr: "id", present: true, want: "abc"},
		{name: "DV_IDENTIFIER issuer", parent: rm.DVIdentifier{ID: "abc", Issuer: &issuer}, attr: "issuer", present: true, want: &issuer},
		{name: "DV_MULTIMEDIA size", parent: rm.DVMultimedia{}, attr: "size", present: true},
		{name: "DV_MULTIMEDIA media_type", parent: rm.DVMultimedia{MediaType: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "IANA_media-types"}, CodeString: "application/pdf"}}, attr: "media_type", present: true},
		{name: "empty DV_DATE value", parent: rm.DVDate{}, attr: "value"},
		{name: "empty DV_IDENTIFIER id", parent: rm.DVIdentifier{}, attr: "id"},
		{name: "DV_IDENTIFIER issuer unset", parent: rm.DVIdentifier{ID: "abc"}, attr: "issuer"},
		{name: "empty DV_MULTIMEDIA media_type", parent: rm.DVMultimedia{}, attr: "media_type"},

		// The demographic PARTY hierarchy, its parts, FOLDER, EHR_STATUS and
		// EHR_ACCESS (REQ-102, REQ-112).
		{name: "PERSON details", parent: rm.Person{Details: tree}, attr: "details", present: true, want: tree},
		{name: "PERSON details unset", parent: rm.Person{}, attr: "details"},
		{name: "ORGANISATION details", parent: rm.Organisation{Details: tree}, attr: "details", present: true, want: tree},
		{name: "GROUP details", parent: rm.Group{Details: tree}, attr: "details", present: true, want: tree},
		{name: "AGENT details", parent: rm.Agent{Details: tree}, attr: "details", present: true, want: tree},
		{name: "ROLE details", parent: rm.Role{Details: tree}, attr: "details", present: true, want: tree},
		{name: "ADDRESS details", parent: rm.Address{Details: tree}, attr: "details", present: true, want: tree},
		{name: "FOLDER name", parent: rm.Folder{Name: rm.DVText{Value: "root"}}, attr: "name", present: true},
		{name: "EHR_STATUS other_details", parent: rm.EHRStatus{OtherDetails: tree}, attr: "other_details", present: true, want: tree},
		{name: "EHR_STATUS is_modifiable", parent: rm.EHRStatus{IsModifiable: true}, attr: "is_modifiable", present: true, want: true},
		{name: "EHR_STATUS is_queryable", parent: rm.EHRStatus{}, attr: "is_queryable", present: true, want: false},
		{name: "EHR_ACCESS archetype_node_id", parent: rm.EHRAccess{ArchetypeNodeID: "openEHR-EHR-EHR_ACCESS.generic.v1"}, attr: "archetype_node_id", present: true},
		{name: "EHR_ACCESS name", parent: rm.EHRAccess{Name: rm.DVText{Value: "EHR Access"}}, attr: "name", present: true},
		{name: "empty EHR_ACCESS archetype_node_id", parent: rm.EHRAccess{}, attr: "archetype_node_id"},
		{name: "empty EHR_ACCESS name", parent: rm.EHRAccess{}, attr: "name"},
		{name: "EHR_ACCESS settings unset", parent: rm.EHRAccess{}, attr: "settings"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkSingle(t, tc, bothForms(tc.parent))
		})
	}
}

// listCase is one container read through Multiple: whether it is present,
// how many items it holds and the Go type of its first item.
type listCase struct {
	name     string
	parent   any
	attr     string
	present  bool
	items    int
	itemType string
}

// TestAccess_REQ102_REQ110_REQ112_ReadsTheContainers pins the containers the
// walks descend through (REQ-102, REQ-112): a set list reads
// present with its items, a list of values boxed as pointers into its
// backing array so a walk enters the live item, a list of interfaces as it
// is; a nil list reads absent (REQ-171 Structural access), in the pointer
// and the value form of its parent.
func TestAccess_REQ102_REQ110_REQ112_ReadsTheContainers(t *testing.T) {
	mapping := rm.TermMapping{Match: rm.Character("="), Target: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "73211009"}}
	identity := rm.PartyIdentity{ArchetypeNodeID: "openEHR-DEMOGRAPHIC-PARTY_IDENTITY.x.v1"}
	cases := []listCase{
		{name: "ITEM_TREE items", parent: rm.ItemTree{Items: []rm.Item{&rm.Element{ArchetypeNodeID: "at0004"}}}, attr: "items", present: true, items: 1, itemType: "*rm.Element"},
		{name: "ITEM_LIST items", parent: rm.ItemList{Items: []rm.Element{{ArchetypeNodeID: "at0006"}}}, attr: "items", present: true, items: 1, itemType: "*rm.Element"},
		{name: "ITEM_TABLE rows", parent: rm.ItemTable{Rows: []rm.Cluster{{ArchetypeNodeID: "at0008"}}}, attr: "rows", present: true, items: 1, itemType: "*rm.Cluster"},
		{name: "DV_TEXT mappings", parent: rm.DVText{Value: "x", Mappings: []rm.TermMapping{mapping}}, attr: "mappings", present: true, items: 1, itemType: "*rm.TermMapping"},
		{name: "DV_CODED_TEXT mappings", parent: rm.DVCodedText{DVText: rm.DVText{Value: "x", Mappings: []rm.TermMapping{mapping}}}, attr: "mappings", present: true, items: 1, itemType: "*rm.TermMapping"},
		{name: "DV_TEXT mappings unset", parent: rm.DVText{Value: "x"}, attr: "mappings"},
		{name: "PERSON identities", parent: rm.Person{Identities: []rm.PartyIdentity{identity}}, attr: "identities", present: true, items: 1, itemType: "*rm.PartyIdentity"},
		{name: "PERSON contacts", parent: rm.Person{Contacts: []rm.Contact{{ArchetypeNodeID: "openEHR-DEMOGRAPHIC-CONTACT.person.v1"}}}, attr: "contacts", present: true, items: 1, itemType: "*rm.Contact"},
		{name: "PERSON relationships", parent: rm.Person{Relationships: []rm.PartyRelationship{{}}}, attr: "relationships", present: true, items: 1, itemType: "*rm.PartyRelationship"},
		{name: "PERSON languages unset", parent: rm.Person{}, attr: "languages"},
		{name: "PERSON roles unset", parent: rm.Person{}, attr: "roles"},
		{name: "PERSON languages, interfaces as they are", parent: rm.Person{Languages: []rm.DVTextLike{rm.DVText{Value: "nl"}}}, attr: "languages", present: true, items: 1, itemType: "rm.DVText"},
		{name: "ORGANISATION identities", parent: rm.Organisation{Identities: []rm.PartyIdentity{identity}}, attr: "identities", present: true, items: 1, itemType: "*rm.PartyIdentity"},
		{name: "GROUP identities", parent: rm.Group{Identities: []rm.PartyIdentity{identity}}, attr: "identities", present: true, items: 1, itemType: "*rm.PartyIdentity"},
		{name: "AGENT identities", parent: rm.Agent{Identities: []rm.PartyIdentity{identity}}, attr: "identities", present: true, items: 1, itemType: "*rm.PartyIdentity"},
		{name: "ROLE capabilities", parent: rm.Role{Capabilities: []rm.Capability{{ArchetypeNodeID: "openEHR-DEMOGRAPHIC-CAPABILITY.c.v1"}}}, attr: "capabilities", present: true, items: 1, itemType: "*rm.Capability"},
		{name: "CONTACT addresses", parent: rm.Contact{Addresses: []rm.Address{{ArchetypeNodeID: "openEHR-DEMOGRAPHIC-ADDRESS.address.v1"}}}, attr: "addresses", present: true, items: 1, itemType: "*rm.Address"},
		{name: "FOLDER folders", parent: rm.Folder{Folders: []rm.Folder{{ArchetypeNodeID: "openEHR-EHR-FOLDER.generic.v1"}}}, attr: "folders", present: true, items: 1, itemType: "*rm.Folder"},
		{name: "FOLDER items, interfaces as they are", parent: rm.Folder{Items: []rm.ObjectRefLike{rm.ObjectRef{Namespace: "local", Type: "PERSON"}}}, attr: "items", present: true, items: 1, itemType: "rm.ObjectRef"},
		{name: "INSTRUCTION activities", parent: rm.Instruction{Activities: []rm.Activity{{ArchetypeNodeID: "at0001"}}}, attr: "activities", present: true, items: 1, itemType: "*rm.Activity"},
		{name: "PARTY_IDENTIFIED identifiers", parent: rm.PartyIdentified{Identifiers: []rm.DVIdentifier{{ID: "123"}}}, attr: "identifiers", present: true, items: 1, itemType: "*rm.DVIdentifier"},
		{name: "PARTY_RELATED identifiers", parent: rm.PartyRelated{Identifiers: []rm.DVIdentifier{{ID: "123"}}}, attr: "identifiers", present: true, items: 1, itemType: "*rm.DVIdentifier"},
		{name: "PARTY_IDENTIFIED identifiers unset", parent: rm.PartyIdentified{}, attr: "identifiers"},
		{name: "PARTY_RELATED identifiers unset", parent: rm.PartyRelated{}, attr: "identifiers"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, form := range bothForms(tc.parent) {
				items, present := Multiple(form, tc.attr)
				if present != tc.present || len(items) != tc.items {
					t.Fatalf("Multiple(%T, %q) = (%d items, %t), want (%d, %t)", form, tc.attr, len(items), present, tc.items, tc.present)
				}
				if tc.items > 0 && fmt.Sprintf("%T", items[0]) != tc.itemType {
					t.Errorf("Multiple(%T, %q)[0] = %T, want a %s", form, tc.attr, items[0], tc.itemType)
				}
			}
		})
	}

	// A list of values is boxed as pointers into the parent's own backing
	// array, so a walk enters the live item.
	ids := []rm.DVIdentifier{{ID: "123"}}
	ps := []rm.Participation{{Function: rm.DVText{Value: "assistant"}}}
	for _, holder := range []struct {
		name   string
		parent any
		attr   string
		first  any
	}{
		{name: "PARTY_IDENTIFIED identifiers", parent: rm.PartyIdentified{Identifiers: ids}, attr: "identifiers", first: &ids[0]},
		{name: "PARTY_RELATED identifiers", parent: rm.PartyRelated{Identifiers: ids}, attr: "identifiers", first: &ids[0]},
		{name: "EVENT_CONTEXT participations", parent: rm.EventContext{Participations: ps}, attr: "participations", first: &ps[0]},
		{name: "OBSERVATION other_participations", parent: rm.Observation{OtherParticipations: ps}, attr: "other_participations", first: &ps[0]},
		{name: "EVALUATION other_participations", parent: rm.Evaluation{OtherParticipations: ps}, attr: "other_participations", first: &ps[0]},
		{name: "INSTRUCTION other_participations", parent: rm.Instruction{OtherParticipations: ps}, attr: "other_participations", first: &ps[0]},
		{name: "ACTION other_participations", parent: rm.Action{OtherParticipations: ps}, attr: "other_participations", first: &ps[0]},
		{name: "ADMIN_ENTRY other_participations", parent: rm.AdminEntry{OtherParticipations: ps}, attr: "other_participations", first: &ps[0]},
	} {
		for _, form := range bothForms(holder.parent) {
			items, present := Multiple(form, holder.attr)
			if !present || len(items) != 1 || items[0] != holder.first {
				t.Errorf("Multiple(%T, %q) = (%v, %t), want one item, a pointer to the slice element", form, holder.attr, items, present)
			}
		}
	}
}

// TestAccess_REQ102_REQ112_ReadsReferenceParts pins the references the walks
// read (REQ-102, REQ-112). The parts of an OBJECT_REF, an ACCESS_GROUP_REF, a
// LOCATABLE_REF and a PARTY_REF read present when set, as the value the
// reference holds, and absent when empty or nil; a LOCATABLE_REF's id is its
// own UID_BASED_ID field, not the OBJECT_REF id it embeds, and its optional
// path reads present only when set. The value of every OBJECT_ID a
// reference's id can hold reads present when set and absent when empty. A
// ROLE's performer and a PARTY_RELATIONSHIP's source and target read as the
// PARTY_REF they hold, present unless it is the zero PARTY_REF, so a walk
// enters one with a part missing and the reference entry reports the part.
func TestAccess_REQ102_REQ112_ReadsReferenceParts(t *testing.T) {
	id := &rm.HierObjectID{Value: "6f1d3a52-9c0e-4b7a-8f21-3d5e7a9b1c04"}
	path := "/content[at0001]"
	refs := []struct {
		name  string
		full  any
		empty any
		attrs map[string]any
	}{
		{name: "OBJECT_REF", full: rm.ObjectRef{ID: id, Namespace: "local", Type: "COMPOSITION"}, empty: rm.ObjectRef{}, attrs: map[string]any{"id": id, "namespace": "local", "type": "COMPOSITION"}},
		{name: "ACCESS_GROUP_REF", full: rm.AccessGroupRef{ObjectRef: rm.ObjectRef{ID: id, Namespace: "local", Type: "ACCESS_GROUP"}}, empty: rm.AccessGroupRef{}, attrs: map[string]any{"id": id, "namespace": "local", "type": "ACCESS_GROUP"}},
		{name: "PARTY_REF", full: rm.PartyRef{ObjectRef: rm.ObjectRef{ID: id, Namespace: "demographic", Type: "ORGANISATION"}}, empty: rm.PartyRef{}, attrs: map[string]any{"id": id, "namespace": "demographic", "type": "ORGANISATION"}},
		// The embedded OBJECT_REF id stays nil: access that took it instead
		// of the LOCATABLE_REF's own id would read id as absent.
		{name: "LOCATABLE_REF", full: rm.LocatableRef{ID: id, ObjectRef: rm.ObjectRef{Namespace: "local", Type: "COMPOSITION"}, Path: &path}, empty: rm.LocatableRef{}, attrs: map[string]any{"id": id, "namespace": "local", "type": "COMPOSITION", "path": &path}},
	}
	for _, ref := range refs {
		for attr, want := range ref.attrs {
			t.Run(ref.name+" "+attr, func(t *testing.T) {
				checkSingle(t, readCase{attr: attr, present: true, want: want}, bothForms(ref.full))
				checkSingle(t, readCase{attr: attr}, bothForms(ref.empty))
			})
		}
	}

	ids := []struct {
		name        string
		attr        string
		full, empty any
	}{
		{name: "HIER_OBJECT_ID", attr: "value", full: rm.HierObjectID{Value: "1.2.3"}, empty: rm.HierObjectID{}},
		{name: "OBJECT_VERSION_ID", attr: "value", full: rm.ObjectVersionID{Value: "a::b::1"}, empty: rm.ObjectVersionID{}},
		{name: "GENERIC_ID", attr: "value", full: rm.GenericID{Value: "123", Scheme: "BSN"}, empty: rm.GenericID{}},
		{name: "GENERIC_ID", attr: "scheme", full: rm.GenericID{Value: "123", Scheme: "BSN"}, empty: rm.GenericID{}},
		{name: "ARCHETYPE_ID", attr: "value", full: rm.ArchetypeID{Value: "openEHR-EHR-OBSERVATION.x.v1"}, empty: rm.ArchetypeID{}},
		{name: "TEMPLATE_ID", attr: "value", full: rm.TemplateID{Value: "t"}, empty: rm.TemplateID{}},
		{name: "TERMINOLOGY_ID", attr: "value", full: rm.TerminologyID{Value: "SNOMED-CT"}, empty: rm.TerminologyID{}},
	}
	for _, tc := range ids {
		t.Run(tc.name+" "+tc.attr, func(t *testing.T) {
			checkSingle(t, readCase{attr: tc.attr, present: true}, bothForms(tc.full))
			checkSingle(t, readCase{attr: tc.attr}, bothForms(tc.empty))
		})
	}

	full := rm.PartyRef{ID: id, Namespace: "local", Type: "PERSON"}
	partial := []rm.PartyRef{{Namespace: "local", Type: "PERSON"}, {ID: id, Type: "PERSON"}, {ID: id, Namespace: "local"}}
	holders := []struct {
		name    string
		attr    string
		holding func(rm.PartyRef) any
	}{
		{name: "ROLE", attr: "performer", holding: func(ref rm.PartyRef) any { return rm.Role{Performer: ref} }},
		{name: "PARTY_RELATIONSHIP", attr: "source", holding: func(ref rm.PartyRef) any { return rm.PartyRelationship{Source: ref, Target: full} }},
		{name: "PARTY_RELATIONSHIP", attr: "target", holding: func(ref rm.PartyRef) any { return rm.PartyRelationship{Source: full, Target: ref} }},
	}
	for _, h := range holders {
		t.Run(h.name+" "+h.attr, func(t *testing.T) {
			checkSingle(t, readCase{attr: h.attr, present: true, want: full, byAddress: true}, bothForms(h.holding(full)))
			for _, ref := range partial {
				checkSingle(t, readCase{attr: h.attr, present: true, want: ref, byAddress: true}, bothForms(h.holding(ref)))
			}
			checkSingle(t, readCase{attr: h.attr}, bothForms(h.holding(rm.PartyRef{})))
		})
	}
}

// TestAccess_REQ112_ReadsPartyProxiesAndParticipation pins the party proxies
// and PARTICIPATION (REQ-112). An optional attribute held by pointer reads
// present once set, whatever it holds, and gives back that pointer, so an
// external_ref with an empty namespace reads present and the floor walks it
// to the reference check, and a name set to the empty string reads present
// and the floor reports it under Name_valid. A DV_TEXT function or a
// DV_CODED_TEXT relationship with no value reads absent, and a nil or
// typed-nil performer reads absent. A ROLE's and a CAPABILITY's
// time_validity reads present as the interval it points to once set.
func TestAccess_REQ112_ReadsPartyProxiesAndParticipation(t *testing.T) {
	name, empty := "Dr Jones", ""
	ref := &rm.PartyRef{ID: &rm.HierObjectID{Value: "9fcc1c70"}, Type: "PERSON"}
	bare := &rm.PartyRef{}
	relationship := rm.DVCodedText{Value: "mother", DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "10"}}
	mode := &rm.DVCodedText{Value: "face-to-face communication"}
	span := &rm.DVInterval[rm.DVDateTime]{}
	validity := &rm.DVInterval[rm.DVDate]{LowerUnbounded: true, UpperUnbounded: true}
	performer := &rm.PartyIdentified{}
	cases := []readCase{
		{name: "PARTY_SELF external_ref set", parent: rm.PartySelf{ExternalRef: ref}, attr: "external_ref", present: true, want: ref},
		{name: "PARTY_SELF external_ref without namespace", parent: rm.PartySelf{ExternalRef: bare}, attr: "external_ref", present: true, want: bare},
		{name: "PARTY_SELF external_ref unset", parent: rm.PartySelf{}, attr: "external_ref"},
		{name: "PARTY_IDENTIFIED name set", parent: rm.PartyIdentified{Name: &name}, attr: "name", present: true, want: &name},
		{name: "PARTY_IDENTIFIED name set but empty", parent: rm.PartyIdentified{Name: &empty}, attr: "name", present: true, want: &empty},
		{name: "PARTY_IDENTIFIED name unset", parent: rm.PartyIdentified{}, attr: "name"},
		{name: "PARTY_IDENTIFIED external_ref set", parent: rm.PartyIdentified{ExternalRef: ref}, attr: "external_ref", present: true, want: ref},
		{name: "PARTY_IDENTIFIED external_ref without namespace", parent: rm.PartyIdentified{ExternalRef: bare}, attr: "external_ref", present: true, want: bare},
		{name: "PARTY_IDENTIFIED external_ref unset", parent: rm.PartyIdentified{}, attr: "external_ref"},
		{name: "PARTY_RELATED relationship set", parent: rm.PartyRelated{Relationship: relationship}, attr: "relationship", present: true, want: relationship, byAddress: true},
		{name: "PARTY_RELATED relationship unset", parent: rm.PartyRelated{}, attr: "relationship"},
		{name: "PARTY_RELATED name set", parent: rm.PartyRelated{PartyIdentified: rm.PartyIdentified{Name: &name}}, attr: "name", present: true, want: &name},
		{name: "PARTY_RELATED name unset", parent: rm.PartyRelated{}, attr: "name"},
		{name: "PARTY_RELATED external_ref set", parent: rm.PartyRelated{PartyIdentified: rm.PartyIdentified{ExternalRef: ref}}, attr: "external_ref", present: true, want: ref},
		{name: "PARTY_RELATED external_ref unset", parent: rm.PartyRelated{}, attr: "external_ref"},
		{name: "PARTICIPATION function set", parent: rm.Participation{Function: rm.DVText{Value: "assistant"}}, attr: "function", present: true},
		{name: "PARTICIPATION function with an empty value", parent: rm.Participation{Function: rm.DVText{}}, attr: "function"},
		{name: "PARTICIPATION function unset", parent: rm.Participation{}, attr: "function"},
		{name: "PARTICIPATION performer set", parent: rm.Participation{Performer: performer}, attr: "performer", present: true, want: performer},
		{name: "PARTICIPATION performer typed nil", parent: rm.Participation{Performer: (*rm.PartyIdentified)(nil)}, attr: "performer"},
		{name: "PARTICIPATION performer unset", parent: rm.Participation{}, attr: "performer"},
		{name: "PARTICIPATION mode set", parent: rm.Participation{Mode: mode}, attr: "mode", present: true, want: mode},
		{name: "PARTICIPATION mode unset", parent: rm.Participation{}, attr: "mode"},
		{name: "PARTICIPATION time set", parent: rm.Participation{Time: span}, attr: "time", present: true, want: span},
		{name: "PARTICIPATION time unset", parent: rm.Participation{}, attr: "time"},
		{name: "ROLE time_validity set", parent: rm.Role{TimeValidity: validity}, attr: "time_validity", present: true, want: validity},
		{name: "ROLE time_validity unset", parent: rm.Role{}, attr: "time_validity"},
		{name: "CAPABILITY time_validity set", parent: rm.Capability{TimeValidity: validity}, attr: "time_validity", present: true, want: validity},
		{name: "CAPABILITY time_validity unset", parent: rm.Capability{}, attr: "time_validity"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkSingle(t, tc, bothForms(tc.parent))
		})
	}
	for _, v := range []any{rm.PartySelf{}, rm.PartyIdentified{}, rm.PartyRelated{}, rm.Participation{}} {
		for _, form := range bothForms(v) {
			if !Covers(form) {
				t.Errorf("Covers(%T) = false, want true in the pointer and the value form", form)
			}
		}
	}
}

// TestAccess_REQ107_ReadsOptionalStrings pins the optional String attributes
// the REQ-107 generator can fill: set, each reads present and gives back the
// pointer; unset, it reads absent.
func TestAccess_REQ107_ReadsOptionalStrings(t *testing.T) {
	s := "plain"
	cases := []struct {
		name       string
		set, unset any
		attr       string
	}{
		{name: "DV_TEXT formatting", set: rm.DVText{Formatting: &s}, unset: rm.DVText{}, attr: "formatting"},
		{name: "DV_CODED_TEXT formatting", set: rm.DVCodedText{DVText: rm.DVText{Formatting: &s}}, unset: rm.DVCodedText{}, attr: "formatting"},
		{name: "DV_MULTIMEDIA alternate_text", set: rm.DVMultimedia{AlternateText: &s}, unset: rm.DVMultimedia{}, attr: "alternate_text"},
		{name: "DV_QUANTITY magnitude_status", set: rm.DVQuantity{MagnitudeStatus: &s}, unset: rm.DVQuantity{}, attr: "magnitude_status"},
		{name: "DV_IDENTIFIER issuer", set: rm.DVIdentifier{Issuer: &s}, unset: rm.DVIdentifier{}, attr: "issuer"},
		{name: "DV_IDENTIFIER assigner", set: rm.DVIdentifier{Assigner: &s}, unset: rm.DVIdentifier{}, attr: "assigner"},
		{name: "DV_IDENTIFIER type", set: rm.DVIdentifier{Type: &s}, unset: rm.DVIdentifier{}, attr: "type"},
		{name: "CODE_PHRASE preferred_term", set: rm.CodePhrase{PreferredTerm: &s}, unset: rm.CodePhrase{}, attr: "preferred_term"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkSingle(t, readCase{attr: tc.attr, present: true, want: &s}, bothForms(tc.set))
			checkSingle(t, readCase{attr: tc.attr}, bothForms(tc.unset))
		})
	}
}

// TestAccess_REQ112_ReadsOrderedValues pins the DV_ORDERED attributes the
// floor walks (REQ-112): on every registered DV_ORDERED, decoded with all
// three DV_ORDERED attributes, normal_status and normal_range read present,
// the interval as a value the access covers, and other_reference_ranges holds
// one REFERENCE_RANGE the access covers, whose meaning reads present; on the
// zero value the two single attributes read absent and the nil list absent.
// accuracy, on every registered DV_ORDERED rminfo declares it for, reads
// present once decoded, a DV_DURATION the access covers on the temporal
// types, and absent on the zero value.
func TestAccess_REQ112_ReadsOrderedValues(t *testing.T) {
	const openInterval = `{"_type":"DV_INTERVAL","lower_unbounded":true,"upper_unbounded":true,"lower_included":false,"upper_included":false}`
	lister, ok := rminfo.Default.(rminfo.AttributeLister)
	if !ok {
		t.Fatal("rminfo.Default does not list attributes")
	}
	var ordered, accurate int
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		if _, ok := ctor().(rm.DVOrdered); !ok {
			continue
		}
		ordered++
		t.Run(name, func(t *testing.T) {
			v, err := typereg.Default.Decode([]byte(`{"_type":"` + name + `",` +
				`"normal_status":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr_normal_statuses"},"code_string":"N"},` +
				`"normal_range":` + openInterval + `,` +
				`"other_reference_ranges":[{"_type":"REFERENCE_RANGE","meaning":{"_type":"DV_TEXT","value":"critical"},"range":` + openInterval + `}]}`))
			if err != nil {
				t.Fatalf("typereg.Decode(%s with DV_ORDERED attributes): %v", name, err)
			}
			if got, ok := Single(v, "normal_status"); !ok {
				t.Errorf("Single(%T, normal_status) = (%v, false), want present", v, got)
			}
			if got, ok := Single(v, "normal_range"); !ok || !Covers(got) {
				t.Errorf("Single(%T, normal_range) = (%T, %t), want an interval the access covers", v, got, ok)
			}
			ranges, ok := Multiple(v, "other_reference_ranges")
			if !ok || len(ranges) != 1 {
				t.Fatalf("Multiple(%T, other_reference_ranges) = (%d items, %t), want (1, true)", v, len(ranges), ok)
			}
			if gotName, _ := rm.RMTypeName(ranges[0]); gotName != "REFERENCE_RANGE" || !Covers(ranges[0]) {
				t.Errorf("other_reference_ranges[0] = %T (RM type %q, covered %t), want a REFERENCE_RANGE the access covers", ranges[0], gotName, Covers(ranges[0]))
			}
			if got, ok := Single(ranges[0], "meaning"); !ok {
				t.Errorf("Single(%T, meaning) = (%v, false), want present", ranges[0], got)
			}
			zero := ctor()
			for _, attr := range []string{"normal_status", "normal_range"} {
				if got, ok := Single(zero, attr); ok {
					t.Errorf("Single(zero %T, %q) = (%v, true), want absent", zero, attr, got)
				}
			}
			if got, ok := Multiple(zero, "other_reference_ranges"); ok || got != nil {
				t.Errorf("Multiple(zero %T, other_reference_ranges) = (%v, %t), want (nil, false): a nil list is absent", zero, got, ok)
			}
			if !slices.Contains(lister.AttributeNames(name), "accuracy") {
				return
			}
			accurate++
			attrType, _ := rminfo.Default.AttributeRMType(name, "accuracy")
			accuracy := `0.5`
			if attrType == "DV_DURATION" {
				accuracy = `{"_type":"DV_DURATION","value":"PT1H"}`
			}
			withAccuracy, err := typereg.Default.Decode([]byte(`{"_type":"` + name + `","accuracy":` + accuracy + `}`))
			if err != nil {
				t.Fatalf("typereg.Decode(%s with accuracy): %v", name, err)
			}
			got, ok := Single(withAccuracy, "accuracy")
			if !ok {
				t.Fatalf("Single(%T, accuracy) = (%v, false), want present", withAccuracy, got)
			}
			if gotName, _ := rm.RMTypeName(got); attrType == "DV_DURATION" && (gotName != "DV_DURATION" || !Covers(got)) {
				t.Errorf("Single(%T, accuracy) = %T, want a DV_DURATION the access covers", withAccuracy, got)
			}
			if got, ok := Single(ctor(), "accuracy"); ok {
				t.Errorf("Single(zero %s, accuracy) = (%v, true), want absent", name, got)
			}
		})
	}
	if ordered == 0 || accurate == 0 {
		t.Fatalf("checked %d DV_ORDERED and %d with accuracy; the registry or rminfo has gone blind", ordered, accurate)
	}

	symbol := rm.DVCodedText{Value: "mild", DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at1"}}
	meaning := rm.DVText{Value: "critical"}
	open := rm.DVInterval[rm.DVOrdered]{LowerUnbounded: true, UpperUnbounded: true}
	bounded := rm.DVInterval[rm.DVOrdered]{Lower: rm.DVCount{Magnitude: 1}, UpperUnbounded: true}
	const rangeType = "*rm.DVInterval[github.com/cadasto/openehr-sdk-go/openehr/rm.DVOrdered]"
	cases := []readCase{
		// DV_ORDINAL and DV_SCALE: symbol reads absent with neither a value
		// nor a code; value, a number, always reads present.
		{name: "DV_ORDINAL symbol set", parent: rm.DVOrdinal{Value: 1, Symbol: symbol}, attr: "symbol", present: true, want: symbol, byAddress: true},
		{name: "DV_ORDINAL symbol absent", parent: rm.DVOrdinal{Value: 1}, attr: "symbol"},
		{name: "DV_ORDINAL value zero", parent: rm.DVOrdinal{Symbol: symbol}, attr: "value", present: true},
		{name: "DV_SCALE symbol set", parent: rm.DVScale{Value: 0.5, Symbol: symbol}, attr: "symbol", present: true, want: symbol, byAddress: true},
		{name: "DV_SCALE symbol absent", parent: rm.DVScale{Value: 0.5}, attr: "symbol"},
		{name: "DV_SCALE value zero", parent: rm.DVScale{Symbol: symbol}, attr: "value", present: true},
		{name: "DV_ORDINAL magnitude, no attribute of the class", parent: rm.DVOrdinal{}, attr: "magnitude"},
		// REFERENCE_RANGE: meaning and range, range as a pointer into the
		// parent, absent at its zero value, present once a bound is set or a
		// side opened; in every instantiation the RM types carry.
		{name: "REFERENCE_RANGE meaning set", parent: rm.ReferenceRange[rm.DVOrdered]{Meaning: meaning}, attr: "meaning", present: true},
		{name: "REFERENCE_RANGE meaning absent", parent: rm.ReferenceRange[rm.DVOrdered]{}, attr: "meaning"},
		{name: "REFERENCE_RANGE meaning empty", parent: rm.ReferenceRange[rm.DVOrdered]{Meaning: rm.DVText{}}, attr: "meaning"},
		{name: "REFERENCE_RANGE range zero", parent: rm.ReferenceRange[rm.DVOrdered]{Meaning: meaning}, attr: "range"},
		{name: "REFERENCE_RANGE range open on both sides", parent: rm.ReferenceRange[rm.DVOrdered]{Range: open}, attr: "range", present: true, wantType: rangeType},
		{name: "REFERENCE_RANGE range with a bound", parent: rm.ReferenceRange[rm.DVOrdered]{Range: bounded}, attr: "range", present: true, wantType: rangeType},
		{name: "REFERENCE_RANGE value, no attribute of the class", parent: rm.ReferenceRange[rm.DVOrdered]{}, attr: "value"},
		{name: "REFERENCE_RANGE<DV_COUNT> meaning", parent: rm.ReferenceRange[rm.DVCount]{Meaning: meaning}, attr: "meaning", present: true},
		{name: "REFERENCE_RANGE<DV_COUNT> range", parent: rm.ReferenceRange[rm.DVCount]{Range: open}, attr: "range", present: true, wantType: rangeType},
		{name: "REFERENCE_RANGE<DV_QUANTITY> range", parent: rm.ReferenceRange[rm.DVQuantity]{Range: bounded}, attr: "range", present: true, wantType: rangeType},
		{name: "REFERENCE_RANGE<DV_QUANTITY> meaning", parent: rm.ReferenceRange[rm.DVQuantity]{Meaning: meaning}, attr: "meaning", present: true},
		{name: "REFERENCE_RANGE<DV_PROPORTION> meaning", parent: rm.ReferenceRange[rm.DVProportion]{Meaning: meaning}, attr: "meaning", present: true},
		{name: "REFERENCE_RANGE<DV_PROPORTION> range", parent: rm.ReferenceRange[rm.DVProportion]{Range: open}, attr: "range", present: true, wantType: rangeType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkSingle(t, tc, bothForms(tc.parent))
		})
	}
}

// TestAccess_REQ112_ArchetypeDetailsOnEveryLocatable pins the inherited
// LOCATABLE archetype_details on every registered LOCATABLE, the only way the
// floor reaches an ARCHETYPED node (REQ-112): nil reads absent, and a set one
// reads present as the same pointer, in the pointer and the value form.
// ARCHETYPED's own archetype_id and rm_version give back the value they hold,
// and template_id the pointer once set.
func TestAccess_REQ112_ArchetypeDetailsOnEveryLocatable(t *testing.T) {
	want := &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-CLUSTER.x.v1"}, RMVersion: "1.1.0"}
	var checked int
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		v := ctor()
		loc, ok := v.(rm.MutableLocatable)
		if !ok {
			continue
		}
		checked++
		checkSingle(t, readCase{attr: "archetype_details"}, bothForms(v))
		loc.SetArchetypeDetails(want)
		checkSingle(t, readCase{attr: "archetype_details", present: true, want: want}, bothForms(v))
	}
	if checked == 0 {
		t.Fatal("the registry yields no LOCATABLE concretes; registrations missing?")
	}

	tid := &rm.TemplateID{Value: "ehr_status.v1"}
	full := rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-EHR_STATUS.generic.v1"}, RMVersion: "1.1.0", TemplateID: tid}
	for _, tc := range []readCase{
		{name: "complete archetype_id", parent: full, attr: "archetype_id", present: true, want: full.ArchetypeID, byAddress: true},
		{name: "complete rm_version", parent: full, attr: "rm_version", present: true, want: "1.1.0"},
		{name: "complete template_id", parent: full, attr: "template_id", present: true, want: tid},
		{name: "zero archetype_id", parent: rm.Archetyped{}, attr: "archetype_id", present: true, want: rm.ArchetypeID{}, byAddress: true},
		{name: "zero rm_version", parent: rm.Archetyped{}, attr: "rm_version", present: true, want: ""},
		{name: "archetype_node_id, no attribute of the class", parent: full, attr: "archetype_node_id"},
	} {
		t.Run("ARCHETYPED "+tc.name, func(t *testing.T) {
			checkSingle(t, tc, bothForms(tc.parent))
		})
	}
}

// TestAccess_REQ112_TypedNilParent pins that a typed-nil parent reads every
// attribute absent rather than panicking inside a reader (REQ-112): a panic
// there would cross into the caller's walk.
func TestAccess_REQ112_TypedNilParent(t *testing.T) {
	cases := []struct {
		parent any
		attr   string
		multi  bool
	}{
		{parent: (*rm.DVQuantity)(nil), attr: "units"},
		{parent: (*rm.EHRAccess)(nil), attr: "archetype_details"},
		{parent: (*rm.ReferenceRange[rm.DVOrdered])(nil), attr: "meaning"},
		{parent: (*rm.DVQuantity)(nil), attr: "bogus", multi: true},
		{parent: (*rm.DVDate)(nil), attr: "other_reference_ranges", multi: true},
		{parent: (*rm.Cluster)(nil), attr: "items", multi: true},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%T %s", tc.parent, tc.attr), func(t *testing.T) {
			if tc.multi {
				if items, present := Multiple(tc.parent, tc.attr); items != nil || present {
					t.Errorf("Multiple(typed-nil %T, %q) = (%v, %t), want (nil, false)", tc.parent, tc.attr, items, present)
				}
				return
			}
			if v, present := Single(tc.parent, tc.attr); v != nil || present {
				t.Errorf("Single(typed-nil %T, %q) = (%v, %t), want (nil, false)", tc.parent, tc.attr, v, present)
			}
		})
	}
}
