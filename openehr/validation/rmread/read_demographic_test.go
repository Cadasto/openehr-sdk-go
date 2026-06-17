package rmread_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
)

// REQ-110 — the demographic PARTY hierarchy and its archetypeable
// sub-components are addressable by the same template walker that drives
// COMPOSITION validation. ReadSingle/ReadMultiple route every (RMType,
// attr) the walker descends through.

func TestReadSingle_Person(t *testing.T) {
	p := &rm.Person{
		ArchetypeNodeID: "openEHR-DEMOGRAPHIC-PERSON.person.v2",
		Name:            rm.DVText{Value: "Persoon"},
		Details:         &rm.ItemTree{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "tree"}},
	}
	for _, attr := range []string{"archetype_node_id", "name", "details"} {
		if _, ok := rmread.ReadSingle(p, "PERSON", attr); !ok {
			t.Errorf("ReadSingle(PERSON, %q) ok=false, want true", attr)
		}
	}
	if _, ok := rmread.ReadSingle(p, "PERSON", "no_such_attr"); ok {
		t.Error("ReadSingle(PERSON, no_such_attr) ok=true, want false")
	}
}

func TestReadSingle_PersonAbsentDetails(t *testing.T) {
	// details is an interface (ITEM_STRUCTURE); a nil interface reports
	// absent so the walker can flag a `required` issue.
	p := &rm.Person{ArchetypeNodeID: "openEHR-DEMOGRAPHIC-PERSON.person.v2", Name: rm.DVText{Value: "P"}}
	if _, ok := rmread.ReadSingle(p, "PERSON", "details"); ok {
		t.Error("ReadSingle(PERSON, details) ok=true on nil Details, want false")
	}
}

func TestReadMultiple_Person(t *testing.T) {
	p := &rm.Person{
		Identities: []rm.PartyIdentity{
			{ArchetypeNodeID: "openEHR-DEMOGRAPHIC-PARTY_IDENTITY.person_name.v2"},
		},
		Contacts:      []rm.Contact{{ArchetypeNodeID: "openEHR-DEMOGRAPHIC-CONTACT.person.v1"}},
		Relationships: []rm.PartyRelationship{{}},
	}
	cases := map[string]int{"identities": 1, "contacts": 1, "relationships": 1, "languages": 0, "roles": 0}
	for attr, want := range cases {
		items, ok := rmread.ReadMultiple(p, "PERSON", attr)
		if !ok {
			t.Errorf("ReadMultiple(PERSON, %q) ok=false, want true", attr)
			continue
		}
		if len(items) != want {
			t.Errorf("ReadMultiple(PERSON, %q) len=%d, want %d", attr, len(items), want)
		}
	}
	// Value-typed slices box as *rm.T so rmTypeInfo recognises the
	// element and the walker can descend into the live struct.
	items, _ := rmread.ReadMultiple(p, "PERSON", "identities")
	if _, ok := items[0].(*rm.PartyIdentity); !ok {
		t.Errorf("ReadMultiple(PERSON, identities)[0] type = %T, want *rm.PartyIdentity", items[0])
	}
	if _, ok := rmread.ReadMultiple(p, "PERSON", "no_such_attr"); ok {
		t.Error("ReadMultiple(PERSON, no_such_attr) ok=true, want false")
	}
}

func TestReadRole(t *testing.T) {
	r := &rm.Role{
		ArchetypeNodeID: "openEHR-DEMOGRAPHIC-ROLE.role.v1",
		Name:            rm.DVText{Value: "GP"},
		Details:         &rm.ItemTree{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "t"}},
		Capabilities:    []rm.Capability{{ArchetypeNodeID: "openEHR-DEMOGRAPHIC-CAPABILITY.c.v1"}},
	}
	if _, ok := rmread.ReadSingle(r, "ROLE", "details"); !ok {
		t.Error("ReadSingle(ROLE, details) ok=false, want true")
	}
	caps, ok := rmread.ReadMultiple(r, "ROLE", "capabilities")
	if !ok || len(caps) != 1 {
		t.Fatalf("ReadMultiple(ROLE, capabilities) = %d,%v want 1,true", len(caps), ok)
	}
	if _, ok := caps[0].(*rm.Capability); !ok {
		t.Errorf("ReadMultiple(ROLE, capabilities)[0] type = %T, want *rm.Capability", caps[0])
	}
}

func TestReadContactAndAddress(t *testing.T) {
	c := &rm.Contact{
		ArchetypeNodeID: "openEHR-DEMOGRAPHIC-CONTACT.person.v1",
		Name:            rm.DVText{Value: "home"},
		Addresses:       []rm.Address{{ArchetypeNodeID: "openEHR-DEMOGRAPHIC-ADDRESS.address.v1"}},
	}
	addrs, ok := rmread.ReadMultiple(c, "CONTACT", "addresses")
	if !ok || len(addrs) != 1 {
		t.Fatalf("ReadMultiple(CONTACT, addresses) = %d,%v want 1,true", len(addrs), ok)
	}
	if _, ok := addrs[0].(*rm.Address); !ok {
		t.Errorf("ReadMultiple(CONTACT, addresses)[0] type = %T, want *rm.Address", addrs[0])
	}
	a := &rm.Address{
		ArchetypeNodeID: "openEHR-DEMOGRAPHIC-ADDRESS.address.v1",
		Name:            rm.DVText{Value: "addr"},
		Details:         &rm.ItemTree{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "t"}},
	}
	if _, ok := rmread.ReadSingle(a, "ADDRESS", "details"); !ok {
		t.Error("ReadSingle(ADDRESS, details) ok=false, want true")
	}
}

func TestReadFolder(t *testing.T) {
	f := &rm.Folder{
		ArchetypeNodeID: "openEHR-EHR-FOLDER.generic.v1",
		Name:            rm.DVText{Value: "root"},
		Folders:         []rm.Folder{{ArchetypeNodeID: "openEHR-EHR-FOLDER.generic.v1", Name: rm.DVText{Value: "sub"}}},
	}
	if _, ok := rmread.ReadSingle(f, "FOLDER", "name"); !ok {
		t.Error("ReadSingle(FOLDER, name) ok=false, want true")
	}
	subs, ok := rmread.ReadMultiple(f, "FOLDER", "folders")
	if !ok || len(subs) != 1 {
		t.Fatalf("ReadMultiple(FOLDER, folders) = %d,%v want 1,true", len(subs), ok)
	}
	if _, ok := subs[0].(*rm.Folder); !ok {
		t.Errorf("ReadMultiple(FOLDER, folders)[0] type = %T, want *rm.Folder", subs[0])
	}
}

func TestReadEHRStatus(t *testing.T) {
	s := &rm.EHRStatus{
		ArchetypeNodeID: "openEHR-EHR-EHR_STATUS.generic.v1",
		Name:            rm.DVText{Value: "status"},
		OtherDetails:    &rm.ItemTree{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "t"}},
		IsModifiable:    true,
		IsQueryable:     true,
	}
	for _, attr := range []string{"archetype_node_id", "name", "subject", "other_details", "is_modifiable", "is_queryable"} {
		if _, ok := rmread.ReadSingle(s, "EHR_STATUS", attr); !ok {
			t.Errorf("ReadSingle(EHR_STATUS, %q) ok=false, want true", attr)
		}
	}
}
