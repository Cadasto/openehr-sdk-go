package canxml_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// relationshipXML is a PARTY_RELATIONSHIP in canonical XML whose source, a
// PARTY_REF the SDK holds by value, is spelled source: "" leaves the element
// out. Its target is set.
func relationshipXML(source string) string {
	return `<party_relationship xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"` +
		` archetype_node_id="openEHR-DEMOGRAPHIC-PARTY_RELATIONSHIP.r.v1"><name xsi:type="DV_TEXT"><value>r</value></name>` +
		source + `<target><namespace>local</namespace><type>PERSON</type></target></party_relationship>`
}

// TestREQ056_EmptyMandatoryStructMemberWritesNoElement decodes
// PARTY_RELATIONSHIP documents whose mandatory source, held by value, is
// missing, empty, or partly filled, re-encodes them, and holds the output to
// the rule canonical JSON follows: a member left out of the input stays out,
// an empty one is left out, and one with a part set is written in full.
//
// REQ-056: canonical XML writes no element for a mandatory member held by
// value as a struct that is its type's zero value (REQ-052 § An absent
// mandatory member).
func TestREQ056_EmptyMandatoryStructMemberWritesNoElement(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{name: "missing", in: relationshipXML(""), want: relationshipXML("")},
		{name: "empty, its children empty", in: relationshipXML(`<source><namespace></namespace><type></type></source>`), want: relationshipXML("")},
		{name: "empty, self-closing", in: relationshipXML(`<source/>`), want: relationshipXML("")},
		{
			name: "namespace only",
			in:   relationshipXML(`<source><namespace>local</namespace><type></type></source>`),
			want: relationshipXML(`<source><namespace>local</namespace><type></type></source>`),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var rel rm.PartyRelationship
			if err := canxml.Unmarshal([]byte(tc.in), &rel); err != nil {
				t.Fatalf("canxml.Unmarshal(%s): %v", tc.in, err)
			}
			got, err := canxml.Marshal(&rel)
			if err != nil {
				t.Fatalf("canxml.Marshal: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("canxml.Marshal(decode of %s)\n got %s\nwant %s", tc.in, got, tc.want)
			}
		})
	}
}

// TestREQ056_EmptyTypeParameterMemberWritesNoElement holds the rule at a
// member typed by its class's own type parameter: POINT_EVENT data bound to
// ITEM_TREE, held by value, writes no element when empty, and writes one
// when the empty ITEM_TREE sits behind the ITEM_STRUCTURE interface, which
// holds a value.
func TestREQ056_EmptyTypeParameterMemberWritesNoElement(t *testing.T) {
	name := rm.DVText{Value: "e"}
	for _, tc := range []struct {
		name  string
		v     any
		wrote bool
	}{
		{name: "POINT_EVENT[ItemTree], data empty", v: &rm.PointEvent[rm.ItemTree]{ArchetypeNodeID: "at0001", Name: name}},
		{name: "POINT_EVENT[ItemTree], data named", v: &rm.PointEvent[rm.ItemTree]{ArchetypeNodeID: "at0001", Name: name, Data: rm.ItemTree{ArchetypeNodeID: "at0002"}}, wrote: true},
		{name: "POINT_EVENT[ItemStructure] holding an empty ITEM_TREE", v: &rm.PointEvent[rm.ItemStructure]{ArchetypeNodeID: "at0001", Name: name, Data: rm.ItemTree{}}, wrote: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := canxml.Marshal(tc.v)
			if err != nil {
				t.Fatalf("canxml.Marshal(%T): %v", tc.v, err)
			}
			if wrote := strings.Contains(string(got), "<data"); wrote != tc.wrote {
				t.Errorf("canxml.Marshal(%T) = %s\nwrites a data element = %v, want %v", tc.v, got, wrote, tc.wrote)
			}
		})
	}
}
