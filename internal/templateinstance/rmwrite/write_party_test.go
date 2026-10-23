package rmwrite

import (
	"errors"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// partyWrite is one attribute write the party tests make: the RM
// attribute, whether it is multi-valued, the child, and the Go field of
// the parent the write must land in.
type partyWrite struct {
	attr  string
	multi bool
	child any
	field string
}

// partyCommonWrites are the PARTY attributes every party class carries.
func partyCommonWrites() []partyWrite {
	return []partyWrite{
		{attr: "name", child: &rm.DVText{Value: "party"}, field: "Name"},
		{attr: "details", child: &rm.ItemTree{ArchetypeNodeID: "at0001"}, field: "Details"},
		{attr: "identities", multi: true, child: &rm.PartyIdentity{ArchetypeNodeID: "at0002"}, field: "Identities"},
		{attr: "contacts", multi: true, child: &rm.Contact{ArchetypeNodeID: "at0003"}, field: "Contacts"},
		{attr: "relationships", multi: true, child: &rm.PartyRelationship{ArchetypeNodeID: "at0004"}, field: "Relationships"},
	}
}

// actorWrites adds the attributes ACTOR declares beyond PARTY: languages,
// a DV_TEXT or a DV_CODED_TEXT each, and roles, a PARTY_REF each. The
// template validator binds a member of either by its RM type, since
// neither is a LOCATABLE with a node id.
func actorWrites() []partyWrite {
	return append(partyCommonWrites(),
		partyWrite{attr: "languages", multi: true, child: &rm.DVText{Value: "nl"}, field: "Languages"},
		partyWrite{attr: "languages", multi: true, child: &rm.DVCodedText{
			Value: "Dutch", DefiningCode: rm.CodePhrase{CodeString: "nl", TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}},
		}, field: "Languages"},
		partyWrite{attr: "roles", multi: true, child: &rm.PartyRef{
			ID: &rm.HierObjectID{Value: "00000000-0000-0000-0000-000000000001"}, Namespace: "local", Type: "ROLE",
		}, field: "Roles"})
}

// actorRefused are children of the wrong type for the ACTOR attributes.
func actorRefused() []partyWrite {
	return []partyWrite{
		{attr: "languages", multi: true, child: &rm.CodePhrase{CodeString: "nl"}},
		{attr: "roles", multi: true, child: &rm.ObjectRef{Namespace: "local", Type: "ROLE"}},
	}
}

// roleRefused are the ACTOR attributes a ROLE, which is no ACTOR, does not
// have.
func roleRefused() []partyWrite {
	return []partyWrite{
		{attr: "languages", multi: true, child: &rm.DVText{Value: "nl"}},
		{attr: "roles", multi: true, child: &rm.PartyRef{Namespace: "local", Type: "ROLE"}},
	}
}

// roleWrites adds the attributes ROLE declares beyond PARTY.
func roleWrites() []partyWrite {
	return append(partyCommonWrites(),
		partyWrite{attr: "performer", child: &rm.PartyRef{
			ID: &rm.HierObjectID{Value: "00000000-0000-0000-0000-000000000001"}, Namespace: "local", Type: "PERSON",
		}, field: "Performer"},
		partyWrite{attr: "capabilities", multi: true, child: &rm.Capability{ArchetypeNodeID: "at0005"}, field: "Capabilities"},
		partyWrite{attr: "time_validity", child: &rm.DVInterval[rm.DVDate]{}, field: "TimeValidity"})
}

// TestREQ107_PartyParentsAddressTheirAttributes is the REQ-107 check that
// rmwrite writes the attributes an OPT can name on every PARTY class, so
// the generator can give a party template root its RM-mandatory
// identities and a ROLE its performer, the languages and roles of every
// ACTOR, and on CAPABILITY, so a ROLE's capability can carry its
// RM-mandatory credentials. An attribute the class does not have stays
// ErrUnknownAttribute, a ROLE's languages and roles included, and a child
// of the wrong type for languages or roles is ErrTypeMismatch.
func TestREQ107_PartyParentsAddressTheirAttributes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		class   string
		parent  func() any
		writes  []partyWrite
		refused []partyWrite
		// unknown are writes of attributes the class does not have.
		unknown []partyWrite
	}{
		{class: "PERSON", parent: func() any { return &rm.Person{} }, writes: actorWrites(), refused: actorRefused()},
		{class: "AGENT", parent: func() any { return &rm.Agent{} }, writes: actorWrites(), refused: actorRefused()},
		{class: "GROUP", parent: func() any { return &rm.Group{} }, writes: actorWrites(), refused: actorRefused()},
		{class: "ORGANISATION", parent: func() any { return &rm.Organisation{} }, writes: actorWrites(), refused: actorRefused()},
		{class: "ROLE", parent: func() any { return &rm.Role{} }, writes: roleWrites(), unknown: roleRefused()},
		{class: "CAPABILITY", parent: func() any { return &rm.Capability{} }, writes: []partyWrite{
			{attr: "name", child: &rm.DVText{Value: "capability"}, field: "Name"},
			{attr: "credentials", child: &rm.ItemTree{ArchetypeNodeID: "at0001"}, field: "Credentials"},
			{attr: "time_validity", child: &rm.DVInterval[rm.DVDate]{}, field: "TimeValidity"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.class, func(t *testing.T) {
			t.Parallel()
			for _, w := range tc.writes {
				parent := tc.parent()
				write, call := EnsureSingle, "EnsureSingle"
				if w.multi {
					write, call = AppendMultiple, "AppendMultiple"
				}
				if err := write(parent, w.attr, w.child); err != nil {
					t.Errorf("%s(%s, %q) = %v, want nil", call, tc.class, w.attr, err)
					continue
				}
				field := reflect.ValueOf(parent).Elem().FieldByName(w.field)
				if w.multi && field.Len() != 1 {
					t.Errorf("%s(%s, %q): %s has %d members, want 1", call, tc.class, w.attr, w.field, field.Len())
				}
				if !w.multi && field.IsZero() {
					t.Errorf("%s(%s, %q): %s is empty, want the child", call, tc.class, w.attr, w.field)
				}
			}
			for _, write := range []func(any, string, any) error{EnsureSingle, AppendMultiple} {
				if err := write(tc.parent(), "not_an_attribute", &rm.DVText{Value: "x"}); !errors.Is(err, ErrUnknownAttribute) {
					t.Errorf("write(%s, not_an_attribute) = %v, want ErrUnknownAttribute", tc.class, err)
				}
			}
			for _, w := range tc.refused {
				if err := AppendMultiple(tc.parent(), w.attr, w.child); !errors.Is(err, ErrTypeMismatch) {
					t.Errorf("AppendMultiple(%s, %q, %T) = %v, want ErrTypeMismatch", tc.class, w.attr, w.child, err)
				}
			}
			for _, w := range tc.unknown {
				if err := AppendMultiple(tc.parent(), w.attr, w.child); !errors.Is(err, ErrUnknownAttribute) {
					t.Errorf("AppendMultiple(%s, %q) = %v, want ErrUnknownAttribute", tc.class, w.attr, err)
				}
			}
		})
	}
}

// TestREQ107_PartyRelatedWritesItsRelationship is the REQ-107 check that
// rmwrite writes a PARTY_RELATED's relationship, so the generator keeps
// the coded text an OPT constrains there. The DV_CODED_TEXT is taken as a
// pointer, the shape the walk builds, or as a value. Another child is
// ErrTypeMismatch, and an attribute PARTY_RELATED does not have, single or
// multi-valued, is ErrUnknownAttribute. The attributes it inherits from
// PARTY_IDENTIFIED are written too
// (TestREQ107_PartyProxyParentsAddressTheirAttributes).
func TestREQ107_PartyRelatedWritesItsRelationship(t *testing.T) {
	t.Parallel()
	mother := rm.DVCodedText{
		Value:        "mother",
		DefiningCode: rm.CodePhrase{CodeString: "10", TerminologyID: rm.TerminologyID{Value: "openehr"}},
	}
	for _, child := range []any{&mother, mother} {
		p := &rm.PartyRelated{}
		if err := EnsureSingle(p, "relationship", child); err != nil {
			t.Errorf("EnsureSingle(PARTY_RELATED, relationship, %T) = %v, want nil", child, err)
			continue
		}
		if got := p.Relationship; got.DefiningCode != mother.DefiningCode || got.Value != mother.Value {
			t.Errorf("EnsureSingle(PARTY_RELATED, relationship, %T): relationship = %q %+v, want %q %+v",
				child, got.Value, got.DefiningCode, mother.Value, mother.DefiningCode)
		}
	}
	for _, child := range []any{&rm.DVText{Value: "mother"}, (*rm.DVCodedText)(nil), "mother"} {
		if err := EnsureSingle(&rm.PartyRelated{}, "relationship", child); !errors.Is(err, ErrTypeMismatch) {
			t.Errorf("EnsureSingle(PARTY_RELATED, relationship, %T) = %v, want ErrTypeMismatch", child, err)
		}
	}
	for _, attr := range []string{"not_an_attribute", "relationships"} {
		for _, write := range []func(any, string, any) error{EnsureSingle, AppendMultiple} {
			if err := write(&rm.PartyRelated{}, attr, &mother); !errors.Is(err, ErrUnknownAttribute) {
				t.Errorf("write(PARTY_RELATED, %q) = %v, want ErrUnknownAttribute", attr, err)
			}
		}
	}
}

// partyIdentifiedWrites are the writes PARTY_IDENTIFIED takes, which
// PARTY_RELATED inherits: the String name, DV_IDENTIFIER identifiers and a
// PARTY_REF external_ref.
func partyIdentifiedWrites() []fieldWrite {
	name := "Jane"
	ref := &rm.PartyRef{ID: &rm.HierObjectID{Value: "p-1"}, Namespace: "demographic", Type: "PERSON"}
	identifier := &rm.DVIdentifier{ID: "123456"}
	return []fieldWrite{
		{attr: "name", child: name, field: "Name", want: &name},
		{attr: "identifiers", multi: true, child: identifier, field: "Identifiers", want: []rm.DVIdentifier{*identifier}},
		{attr: "external_ref", child: ref, field: "ExternalRef", want: ref},
	}
}

// partyIdentifiedMismatches are children no PARTY_IDENTIFIED attribute
// holds.
func partyIdentifiedMismatches() []fieldWrite {
	return []fieldWrite{
		{attr: "name", child: &rm.DVText{Value: "Jane"}},
		{attr: "identifiers", multi: true, child: &rm.DVText{Value: "123456"}},
		{attr: "external_ref", child: &rm.ObjectRef{Namespace: "local", Type: "PERSON"}},
		{attr: "external_ref", child: (*rm.PartyRef)(nil)},
	}
}

// TestREQ107_PartyProxyParentsAddressTheirAttributes is the REQ-107 check
// that rmwrite writes the attributes an OPT can name on a party proxy, a
// PARTY_RELATIONSHIP and a CONTACT, so the generator keeps what an OPT
// constrains there: a PARTY_IDENTIFIED's name, identifiers and
// external_ref, which PARTY_RELATED inherits; a PARTY_SELF's external_ref;
// a PARTY_RELATIONSHIP's name, details, source, target and time_validity;
// and a CONTACT's name and time_validity beside its addresses.
func TestREQ107_PartyProxyParentsAddressTheirAttributes(t *testing.T) {
	t.Parallel()
	ref := &rm.PartyRef{ID: &rm.HierObjectID{Value: "p-1"}, Namespace: "demographic", Type: "ORGANISATION"}
	validity := &rm.DVInterval[rm.DVDate]{}
	tree := &rm.ItemTree{ArchetypeNodeID: "at0001"}
	runWriterCases(t, []writerCase{
		{
			class: "PARTY_IDENTIFIED", parent: func() any { return &rm.PartyIdentified{} },
			writes: partyIdentifiedWrites(), mismatched: partyIdentifiedMismatches(), unknown: []string{"relationship"},
		},
		{
			class: "PARTY_RELATED", parent: func() any { return &rm.PartyRelated{} },
			writes: partyIdentifiedWrites(), mismatched: partyIdentifiedMismatches(),
		},
		{
			class: "PARTY_SELF", parent: func() any { return &rm.PartySelf{} },
			writes:     []fieldWrite{{attr: "external_ref", child: ref, field: "ExternalRef", want: ref}},
			mismatched: []fieldWrite{{attr: "external_ref", child: &rm.ObjectRef{Namespace: "local", Type: "PERSON"}}},
			unknown:    []string{"name", "identifiers"},
		},
		{
			class: "PARTY_RELATIONSHIP", parent: func() any { return &rm.PartyRelationship{} },
			writes: []fieldWrite{
				{attr: "name", child: &rm.DVText{Value: "employment"}, field: "Name", want: rm.DVText{Value: "employment"}},
				{attr: "details", child: tree, field: "Details", want: tree},
				{attr: "source", child: ref, field: "Source", want: *ref},
				{attr: "target", child: ref, field: "Target", want: *ref},
				{attr: "time_validity", child: validity, field: "TimeValidity", want: validity},
			},
			mismatched: []fieldWrite{
				{attr: "source", child: &rm.ObjectRef{Namespace: "local", Type: "PERSON"}},
				{attr: "target", child: (*rm.PartyRef)(nil)},
				{attr: "details", child: &rm.Cluster{}},
				{attr: "time_validity", child: &rm.DVInterval[rm.DVDateTime]{}},
			},
			unknown: []string{"relationship"},
		},
		{
			class: "CONTACT", parent: func() any { return &rm.Contact{} },
			writes: []fieldWrite{
				{attr: "name", child: &rm.DVText{Value: "home"}, field: "Name", want: rm.DVText{Value: "home"}},
				{attr: "time_validity", child: validity, field: "TimeValidity", want: validity},
				{attr: "addresses", multi: true, child: &rm.Address{ArchetypeNodeID: "at0002"}, field: "Addresses", want: []rm.Address{{ArchetypeNodeID: "at0002"}}},
			},
			mismatched: []fieldWrite{
				{attr: "name", child: "home"},
				{attr: "time_validity", child: &rm.DVInterval[rm.DVDateTime]{}},
			},
			unknown: []string{"details"},
		},
	})
}
