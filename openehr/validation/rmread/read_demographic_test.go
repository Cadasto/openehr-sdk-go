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

// REQ-110 — the four ACTOR subtypes share readActorLike* helpers but have
// SEPARATE dispatch arms in ReadSingle/ReadMultiple. One representative
// case per type guards those arms against a copy-paste slip (an arm wired
// to the wrong reader, or a missing value-case) that PERSON coverage alone
// would not catch.
func TestReadActorDispatch(t *testing.T) {
	tree := &rm.ItemTree{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "t"}}
	id := rm.PartyIdentity{ArchetypeNodeID: "openEHR-DEMOGRAPHIC-PARTY_IDENTITY.x.v1"}
	org := &rm.Organisation{ArchetypeNodeID: "org", Name: rm.DVText{Value: "O"}, Details: tree, Identities: []rm.PartyIdentity{id}}
	grp := &rm.Group{ArchetypeNodeID: "grp", Name: rm.DVText{Value: "G"}, Details: tree, Identities: []rm.PartyIdentity{id}}
	agt := &rm.Agent{ArchetypeNodeID: "agt", Name: rm.DVText{Value: "A"}, Details: tree, Identities: []rm.PartyIdentity{id}}

	cases := []struct {
		typ  string
		root any
	}{
		{"ORGANISATION", org},
		{"GROUP", grp},
		{"AGENT", agt},
	}
	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			if _, ok := rmread.ReadSingle(tc.root, tc.typ, "details"); !ok {
				t.Errorf("ReadSingle(%s, details) ok=false, want true", tc.typ)
			}
			items, ok := rmread.ReadMultiple(tc.root, tc.typ, "identities")
			if !ok || len(items) != 1 {
				t.Fatalf("ReadMultiple(%s, identities) = %d,%v want 1,true", tc.typ, len(items), ok)
			}
			if _, ok := items[0].(*rm.PartyIdentity); !ok {
				t.Errorf("ReadMultiple(%s, identities)[0] type = %T, want *rm.PartyIdentity", tc.typ, items[0])
			}
		})
	}
}

// REQ-110 — boxIfaces boxes interface-slice elements as-is (not pointer-
// boxed). Exercise the non-empty path (languages, FOLDER.items) so the
// boxPtrs-vs-boxIfaces distinction the readers rely on is asserted, not
// just observed empty.
func TestReadActorLanguagesBoxIfaces(t *testing.T) {
	p := &rm.Person{Languages: []rm.DVTextLike{rm.DVText{Value: "nl"}}}
	langs, ok := rmread.ReadMultiple(p, "PERSON", "languages")
	if !ok || len(langs) != 1 {
		t.Fatalf("ReadMultiple(PERSON, languages) = %d,%v want 1,true", len(langs), ok)
	}
	// Element is the interface value (DVText), not a pointer-to-element.
	if _, ok := langs[0].(rm.DVText); !ok {
		t.Errorf("languages[0] type = %T, want rm.DVText (interface value, not *T)", langs[0])
	}

	f := &rm.Folder{Items: []rm.ObjectRefLike{rm.ObjectRef{Namespace: "local", Type: "PERSON"}}}
	items, ok := rmread.ReadMultiple(f, "FOLDER", "items")
	if !ok || len(items) != 1 {
		t.Fatalf("ReadMultiple(FOLDER, items) = %d,%v want 1,true", len(items), ok)
	}
}

// REQ-110 — the primitive-bearing DataValue leaf readers. Each populated
// value reports present (so the C_PRIMITIVE child validates); each empty
// optional value reports absent (so a mandated leaf surfaces `required`).
func TestReadDataValueLeaves(t *testing.T) {
	strptr := func(s string) *string { return &s }

	present := []struct {
		name string
		dv   any
		typ  string
		attr string
	}{
		{"DV_DATE.value", &rm.DVDate{Value: "1980-01-01"}, "DV_DATE", "value"},
		{"DV_TIME.value", &rm.DVTime{Value: "10:00:00"}, "DV_TIME", "value"},
		{"DV_DATE_TIME.value", &rm.DVDateTime{Value: "1980-01-01T10:00:00Z"}, "DV_DATE_TIME", "value"},
		{"DV_DURATION.value", &rm.DVDuration{Value: "P1Y"}, "DV_DURATION", "value"},
		{"DV_BOOLEAN.value(false)", &rm.DVBoolean{Value: false}, "DV_BOOLEAN", "value"}, // value-typed: always present
		{"DV_IDENTIFIER.id", &rm.DVIdentifier{ID: "abc"}, "DV_IDENTIFIER", "id"},
		{"DV_IDENTIFIER.issuer", &rm.DVIdentifier{ID: "abc", Issuer: strptr("X")}, "DV_IDENTIFIER", "issuer"},
		{"DV_MULTIMEDIA.size", &rm.DVMultimedia{Size: 10}, "DV_MULTIMEDIA", "size"}, // value-typed: always present
		{"DV_MULTIMEDIA.media_type", &rm.DVMultimedia{MediaType: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "IANA_media-types"}, CodeString: "application/pdf"}}, "DV_MULTIMEDIA", "media_type"},
	}
	for _, tc := range present {
		t.Run("present/"+tc.name, func(t *testing.T) {
			if _, ok := rmread.ReadSingle(tc.dv, tc.typ, tc.attr); !ok {
				t.Errorf("ReadSingle(%s, %s) ok=false, want true (populated)", tc.typ, tc.attr)
			}
		})
	}

	absent := []struct {
		name string
		dv   any
		typ  string
		attr string
	}{
		{"DV_DATE.value", &rm.DVDate{}, "DV_DATE", "value"},
		{"DV_IDENTIFIER.id", &rm.DVIdentifier{}, "DV_IDENTIFIER", "id"},
		{"DV_IDENTIFIER.issuer", &rm.DVIdentifier{ID: "abc"}, "DV_IDENTIFIER", "issuer"}, // nil *string
		{"DV_MULTIMEDIA.media_type", &rm.DVMultimedia{}, "DV_MULTIMEDIA", "media_type"},
	}
	for _, tc := range absent {
		t.Run("absent/"+tc.name, func(t *testing.T) {
			if _, ok := rmread.ReadSingle(tc.dv, tc.typ, tc.attr); ok {
				t.Errorf("ReadSingle(%s, %s) ok=true, want false (empty)", tc.typ, tc.attr)
			}
		})
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
