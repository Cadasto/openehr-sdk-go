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

// actorWrites adds the ACTOR attribute rmwrite builds a member for.
func actorWrites() []partyWrite {
	return append(partyCommonWrites(),
		partyWrite{attr: "languages", multi: true, child: &rm.DVText{Value: "nl"}, field: "Languages"})
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
// identities and a ROLE its performer, and on CAPABILITY, so a ROLE's
// capability can carry its RM-mandatory credentials. An attribute the
// class does not have stays ErrUnknownAttribute.
func TestREQ107_PartyParentsAddressTheirAttributes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		class  string
		parent func() any
		writes []partyWrite
	}{
		{class: "PERSON", parent: func() any { return &rm.Person{} }, writes: actorWrites()},
		{class: "AGENT", parent: func() any { return &rm.Agent{} }, writes: actorWrites()},
		{class: "GROUP", parent: func() any { return &rm.Group{} }, writes: actorWrites()},
		{class: "ORGANISATION", parent: func() any { return &rm.Organisation{} }, writes: actorWrites()},
		{class: "ROLE", parent: func() any { return &rm.Role{} }, writes: roleWrites()},
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
				if err := write(parent, tc.class, w.attr, w.child); err != nil {
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
			for _, write := range []func(any, string, string, any) error{EnsureSingle, AppendMultiple} {
				if err := write(tc.parent(), tc.class, "not_an_attribute", &rm.DVText{Value: "x"}); !errors.Is(err, ErrUnknownAttribute) {
					t.Errorf("write(%s, not_an_attribute) = %v, want ErrUnknownAttribute", tc.class, err)
				}
			}
		})
	}
}
