package rmread_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
)

// bothForms returns v and a pointer to a copy of it, so a case written once
// is read in the value and the pointer form the readers both accept.
func bothForms(v any) []any {
	ptr := reflect.New(reflect.TypeOf(v))
	ptr.Elem().Set(reflect.ValueOf(v))
	return []any{v, ptr.Interface()}
}

// TestREQ112_Handles_PartyProxyAndParticipation checks that Handles accepts
// the three party proxies and PARTICIPATION in value and pointer form, so the
// RM floor reads their attributes instead of stopping at them (REQ-112).
func TestREQ112_Handles_PartyProxyAndParticipation(t *testing.T) {
	for _, v := range []any{rm.PartySelf{}, rm.PartyIdentified{}, rm.PartyRelated{}, rm.Participation{}} {
		for _, form := range bothForms(v) {
			if !rmread.Handles(form) {
				t.Errorf("Handles(%T) = false, want true", form)
			}
		}
	}
}

// TestREQ112_ReadSingle_PartyProxyAndParticipation checks the presence each
// party-proxy and PARTICIPATION reader gives (REQ-112). An optional
// attribute held by pointer reads present once set, whatever it holds, so
// an external_ref with an empty namespace reads present and the floor walks
// it to the PARTY_REF check. A DV_TEXT function or a DV_CODED_TEXT
// relationship with no value reads absent, and a nil or typed-nil performer
// reads absent.
func TestREQ112_ReadSingle_PartyProxyAndParticipation(t *testing.T) {
	name := "Dr Jones"
	empty := ""
	ref := &rm.PartyRef{ID: &rm.HierObjectID{Value: "9fcc1c70"}, Type: "PERSON"}
	relationship := rm.DVCodedText{
		Value:        "mother",
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "10"},
	}
	mode := &rm.DVCodedText{Value: "face-to-face communication"}
	span := &rm.DVInterval[rm.DVDateTime]{}
	cases := []struct {
		name   string
		parent any
		attr   string
		want   bool
	}{
		{"PARTY_SELF external_ref set", rm.PartySelf{ExternalRef: ref}, "external_ref", true},
		{"PARTY_SELF external_ref without namespace", rm.PartySelf{ExternalRef: &rm.PartyRef{}}, "external_ref", true},
		{"PARTY_SELF external_ref unset", rm.PartySelf{}, "external_ref", false},

		{"PARTY_IDENTIFIED name set", rm.PartyIdentified{Name: &name}, "name", true},
		{"PARTY_IDENTIFIED name set but empty", rm.PartyIdentified{Name: &empty}, "name", true},
		{"PARTY_IDENTIFIED name unset", rm.PartyIdentified{}, "name", false},
		{"PARTY_IDENTIFIED external_ref set", rm.PartyIdentified{ExternalRef: ref}, "external_ref", true},
		{"PARTY_IDENTIFIED external_ref without namespace", rm.PartyIdentified{ExternalRef: &rm.PartyRef{}}, "external_ref", true},
		{"PARTY_IDENTIFIED external_ref unset", rm.PartyIdentified{}, "external_ref", false},

		{"PARTY_RELATED relationship set", rm.PartyRelated{Relationship: relationship}, "relationship", true},
		{"PARTY_RELATED relationship unset", rm.PartyRelated{}, "relationship", false},
		{"PARTY_RELATED name set", rm.PartyRelated{Name: &name}, "name", true},
		{"PARTY_RELATED name unset", rm.PartyRelated{}, "name", false},
		{"PARTY_RELATED external_ref set", rm.PartyRelated{ExternalRef: ref}, "external_ref", true},
		{"PARTY_RELATED external_ref unset", rm.PartyRelated{}, "external_ref", false},

		{"PARTICIPATION function set", rm.Participation{Function: rm.DVText{Value: "assistant"}}, "function", true},
		{"PARTICIPATION function with an empty value", rm.Participation{Function: rm.DVText{}}, "function", false},
		{"PARTICIPATION function unset", rm.Participation{}, "function", false},
		{"PARTICIPATION performer set", rm.Participation{Performer: &rm.PartyIdentified{}}, "performer", true},
		{"PARTICIPATION performer typed nil", rm.Participation{Performer: (*rm.PartyIdentified)(nil)}, "performer", false},
		{"PARTICIPATION performer unset", rm.Participation{}, "performer", false},
		{"PARTICIPATION mode set", rm.Participation{Mode: mode}, "mode", true},
		{"PARTICIPATION mode unset", rm.Participation{}, "mode", false},
		{"PARTICIPATION time set", rm.Participation{Time: span}, "time", true},
		{"PARTICIPATION time unset", rm.Participation{}, "time", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, form := range bothForms(tc.parent) {
				if _, ok := rmread.ReadSingle(form, "", tc.attr); ok != tc.want {
					t.Errorf("ReadSingle(%T, %q) ok = %v, want %v", form, tc.attr, ok, tc.want)
				}
			}
		})
	}
}

// TestREQ112_ReadSingle_PartyProxyGivesBackTheValue checks that a set
// optional attribute is read back as the value the proxy holds: the name as
// its *string, as the optional String attributes are, and the external_ref
// as its *rm.PartyRef, so the floor walks the reference itself (REQ-112).
func TestREQ112_ReadSingle_PartyProxyGivesBackTheValue(t *testing.T) {
	name := "Dr Jones"
	ref := &rm.PartyRef{}
	cases := []struct {
		name   string
		parent any
		attr   string
		want   any
	}{
		{"PARTY_SELF external_ref", &rm.PartySelf{ExternalRef: ref}, "external_ref", ref},
		{"PARTY_IDENTIFIED name", &rm.PartyIdentified{Name: &name}, "name", &name},
		{"PARTY_IDENTIFIED external_ref", &rm.PartyIdentified{ExternalRef: ref}, "external_ref", ref},
		{"PARTY_RELATED name", &rm.PartyRelated{Name: &name}, "name", &name},
		{"PARTY_RELATED external_ref", &rm.PartyRelated{ExternalRef: ref}, "external_ref", ref},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := rmread.ReadSingle(tc.parent, "", tc.attr)
			if !ok || got != tc.want {
				t.Errorf("ReadSingle(%T, %q) = (%v, %v), want (%v, true)", tc.parent, tc.attr, got, ok, tc.want)
			}
		})
	}
}

// TestREQ112_ReadMultiple_PartyIdentifiedIdentifiers checks that
// PARTY_IDENTIFIED and PARTY_RELATED serve identifiers, each element boxed
// as a pointer into the slice so the floor walks it as a DV_IDENTIFIER
// node, and that unset identifiers is served as an empty list (REQ-112).
func TestREQ112_ReadMultiple_PartyIdentifiedIdentifiers(t *testing.T) {
	ids := []rm.DVIdentifier{{ID: "123"}}
	cases := []struct {
		name   string
		parent any
		want   int
	}{
		{"PARTY_IDENTIFIED", &rm.PartyIdentified{Identifiers: ids}, 1},
		{"PARTY_IDENTIFIED value", rm.PartyIdentified{Identifiers: ids}, 1},
		{"PARTY_IDENTIFIED unset", &rm.PartyIdentified{}, 0},
		{"PARTY_RELATED", &rm.PartyRelated{Identifiers: ids}, 1},
		{"PARTY_RELATED value", rm.PartyRelated{Identifiers: ids}, 1},
		{"PARTY_RELATED unset", &rm.PartyRelated{}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items, ok := rmread.ReadMultiple(tc.parent, "", "identifiers")
			if !ok || len(items) != tc.want {
				t.Fatalf("ReadMultiple(%T, identifiers) = (%d items, %v), want (%d items, true)", tc.parent, len(items), ok, tc.want)
			}
			if tc.want == 1 && items[0] != &ids[0] {
				t.Errorf("ReadMultiple(%T, identifiers)[0] = %v (%T), want a pointer to the slice element", tc.parent, items[0], items[0])
			}
		})
	}
}

// TestREQ112_ReadMultiple_Participations checks that EVENT_CONTEXT serves
// participations and each ENTRY concrete serves other_participations, in
// value and pointer form, each element boxed as a pointer into the slice so
// the floor walks it as a PARTICIPATION node (REQ-112).
func TestREQ112_ReadMultiple_Participations(t *testing.T) {
	ps := []rm.Participation{{Function: rm.DVText{Value: "assistant"}}}
	cases := []struct {
		parent any
		attr   string
	}{
		{rm.EventContext{Participations: ps}, "participations"},
		{rm.Observation{OtherParticipations: ps}, "other_participations"},
		{rm.Evaluation{OtherParticipations: ps}, "other_participations"},
		{rm.Instruction{OtherParticipations: ps}, "other_participations"},
		{rm.Action{OtherParticipations: ps}, "other_participations"},
		{rm.AdminEntry{OtherParticipations: ps}, "other_participations"},
	}
	for _, tc := range cases {
		for _, form := range bothForms(tc.parent) {
			t.Run(fmt.Sprintf("%T.%s", form, tc.attr), func(t *testing.T) {
				items, ok := rmread.ReadMultiple(form, "", tc.attr)
				if !ok || len(items) != 1 {
					t.Fatalf("ReadMultiple(%T, %q) = (%d items, %v), want (1 item, true)", form, tc.attr, len(items), ok)
				}
				if items[0] != &ps[0] {
					t.Errorf("ReadMultiple(%T, %q)[0] = %v (%T), want a pointer to the slice element", form, tc.attr, items[0], items[0])
				}
			})
		}
	}
}

// TestREQ112_ReadMultiple_InstructionKeepsActivities checks that INSTRUCTION
// still serves activities beside other_participations (REQ-112).
func TestREQ112_ReadMultiple_InstructionKeepsActivities(t *testing.T) {
	in := &rm.Instruction{Activities: []rm.Activity{{ArchetypeNodeID: "at0001"}}}
	if items, ok := rmread.ReadMultiple(in, "INSTRUCTION", "activities"); !ok || len(items) != 1 {
		t.Errorf("ReadMultiple(INSTRUCTION, activities) = (%d items, %v), want (1 item, true)", len(items), ok)
	}
}
