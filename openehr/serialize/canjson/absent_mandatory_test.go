package canjson_test

import (
	"encoding/json"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/expectedform"
	"github.com/cadasto/openehr-sdk-go/testkit/wireequiv"
)

// codePhraseNL is a complete CODE_PHRASE for the inputs that need one.
const codePhraseNL = `{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"},"code_string":"nl"}`

// TestREQ052_AbsentMandatoryMemberIsOmitted decodes inputs that leave out a
// mandatory member, re-encodes them, and requires the member to stay absent
// rather than come back as null. Each row is one class from issue 257, plus a
// LOCATABLE with no name. The re-encode must also be wire-equivalent to the
// input's expected form, which treats a null member as absent and keeps
// everything else.
//
// REQ-052 § An absent mandatory member: a mandatory member with no value is
// left out on encode, never written as null.
func TestREQ052_AbsentMandatoryMemberIsOmitted(t *testing.T) {
	for _, tc := range []struct {
		name   string
		in     string
		target func() any
		absent []string
	}{
		{
			name:   "RESOURCE_ANNOTATIONS documentation",
			in:     `{"_type":"RESOURCE_ANNOTATIONS"}`,
			target: func() any { return new(rm.ResourceAnnotations) },
			absent: []string{"documentation"},
		},
		{
			name:   "TRANSLATION_DETAILS author",
			in:     `{"_type":"TRANSLATION_DETAILS","language":` + codePhraseNL + `}`,
			target: func() any { return new(rm.TranslationDetails) },
			absent: []string{"author"},
		},
		{
			name:   "RESOURCE_DESCRIPTION details, original_author and parent_resource",
			in:     `{"_type":"RESOURCE_DESCRIPTION","lifecycle_state":"published"}`,
			target: func() any { return new(rm.ResourceDescription) },
			absent: []string{"details", "original_author", "parent_resource"},
		},
		{
			name:   "PERSON identities",
			in:     `{"_type":"PERSON","name":{"_type":"DV_TEXT","value":"p"},"archetype_node_id":"openEHR-DEMOGRAPHIC-PERSON.p.v1"}`,
			target: func() any { return new(rm.Person) },
			absent: []string{"identities"},
		},
		{
			name:   "CONTRIBUTION audit and versions",
			in:     `{"_type":"CONTRIBUTION","uid":{"_type":"HIER_OBJECT_ID","value":"u"}}`,
			target: func() any { return new(rm.Contribution) },
			absent: []string{"audit", "versions"},
		},
		{
			name:   "REVISION_HISTORY items",
			in:     `{"_type":"REVISION_HISTORY"}`,
			target: func() any { return new(rm.RevisionHistory) },
			absent: []string{"items"},
		},
		{
			name:   "ELEMENT, a LOCATABLE with no name",
			in:     `{"_type":"ELEMENT","archetype_node_id":"at0001"}`,
			target: func() any { return new(rm.Element) },
			absent: []string{"name"},
		},
		{
			name:   "PARTY_RELATIONSHIP source and target, PARTY_REFs held by value",
			in:     `{"_type":"PARTY_RELATIONSHIP","archetype_node_id":"openEHR-DEMOGRAPHIC-PARTY_RELATIONSHIP.r.v1","name":{"_type":"DV_TEXT","value":"r"}}`,
			target: func() any { return new(rm.PartyRelationship) },
			absent: []string{"source", "target"},
		},
		{
			name:   "COMPOSITION category, language and territory, held by value",
			in:     `{"_type":"COMPOSITION","archetype_node_id":"openEHR-EHR-COMPOSITION.report.v1","name":{"_type":"DV_TEXT","value":"r"}}`,
			target: func() any { return new(rm.Composition) },
			absent: []string{"category", "language", "territory"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := tc.target()
			if err := canjson.Unmarshal([]byte(tc.in), v); err != nil {
				t.Fatalf("canjson.Unmarshal(%s, %T): %v", tc.in, v, err)
			}
			got, err := canjson.Marshal(v)
			if err != nil {
				t.Fatalf("canjson.Marshal(%T): %v", v, err)
			}
			members := memberValues(t, got)
			for _, name := range tc.absent {
				if raw, ok := members[name]; ok {
					t.Errorf("canjson.Marshal(decode of %s) = %s\nwrites %q as %s, want the member absent", tc.in, got, name, raw)
				}
			}
			want, err := expectedform.BuildFor([]byte(tc.in), v)
			if err != nil {
				t.Fatalf("expectedform.BuildFor(%s, %T): %v", tc.in, v, err)
			}
			if ok, diff := wireequiv.Equivalent(want, got); !ok {
				t.Errorf("canjson.Marshal(decode of %s) = %s\nis not wire-equivalent to the expected form %s (expected form versus output): %s", tc.in, got, want, diff)
			}
		})
	}
}

// TestREQ052_PresentEmptyMandatoryContainerIsWritten is the control for the
// test above: a mandatory list or map that the input carries empty is a
// value, so the re-encode writes it as it is, `[]` or `{}`.
//
// REQ-052 § An absent mandatory member: a present empty mandatory list or
// map is written as it is.
func TestREQ052_PresentEmptyMandatoryContainerIsWritten(t *testing.T) {
	for _, tc := range []struct {
		name   string
		in     string
		target func() any
		member string
		want   string
	}{
		{
			name:   "PERSON identities []",
			in:     `{"_type":"PERSON","name":{"_type":"DV_TEXT","value":"p"},"archetype_node_id":"openEHR-DEMOGRAPHIC-PERSON.p.v1","identities":[]}`,
			target: func() any { return new(rm.Person) },
			member: "identities",
			want:   `[]`,
		},
		{
			name:   "RESOURCE_ANNOTATIONS documentation {}",
			in:     `{"_type":"RESOURCE_ANNOTATIONS","documentation":{}}`,
			target: func() any { return new(rm.ResourceAnnotations) },
			member: "documentation",
			want:   `{}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := tc.target()
			if err := canjson.Unmarshal([]byte(tc.in), v); err != nil {
				t.Fatalf("canjson.Unmarshal(%s, %T): %v", tc.in, v, err)
			}
			got, err := canjson.Marshal(v)
			if err != nil {
				t.Fatalf("canjson.Marshal(%T): %v", v, err)
			}
			raw, ok := memberValues(t, got)[tc.member]
			if !ok {
				t.Fatalf("canjson.Marshal(decode of %s) = %s\nleaves out %q, want it written as %s", tc.in, got, tc.member, tc.want)
			}
			if same, diff := wireequiv.Equivalent([]byte(tc.want), raw); !same {
				t.Errorf("canjson.Marshal(decode of %s) writes %q as %s, want %s: %s", tc.in, tc.member, raw, tc.want, diff)
			}
		})
	}
}

// TestREQ052_GoBuiltAbsentMandatoryMember encodes Go values whose mandatory
// field holds no value: a nil interface, a typed-nil pointer in an interface,
// a nil type-parameter field instantiated with an interface, a nil pointer, a
// nil `any`, and a struct held by value whose every field is empty, held
// directly or by a type-parameter field instantiated with a struct. Each
// member must be left out.
//
// REQ-052 § An absent mandatory member: a nil list, map, pointer or
// interface, a polymorphic RM slot holding a typed-nil pointer, or a struct
// held by value whose zero test holds, is left out on encode.
func TestREQ052_GoBuiltAbsentMandatoryMember(t *testing.T) {
	for _, tc := range []struct {
		name   string
		value  any
		member string
	}{
		{
			name:   "a nil interface in ELEMENT.name",
			value:  &rm.Element{ArchetypeNodeID: "at0001"},
			member: "name",
		},
		{
			name:   "a typed-nil pointer in ELEMENT.name",
			value:  &rm.Element{ArchetypeNodeID: "at0001", Name: (*rm.DVText)(nil)},
			member: "name",
		},
		{
			name:   "a typed-nil pointer in CONTRIBUTION.audit",
			value:  &rm.Contribution{UID: rm.HierObjectID{Value: "u"}, Audit: (*rm.AuditDetails)(nil), Versions: []rm.ObjectRefLike{}},
			member: "audit",
		},
		{
			name:   "a nil data in POINT_EVENT[ItemStructure]",
			value:  &rm.PointEvent[rm.ItemStructure]{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "e"}},
			member: "data",
		},
		{
			name:   "a zero HIER_OBJECT_ID held by value in CONTRIBUTION.uid",
			value:  &rm.Contribution{Versions: []rm.ObjectRefLike{}},
			member: "uid",
		},
		{
			name:   "a zero PARTY_REF held by value in PARTY_RELATIONSHIP.source",
			value:  &rm.PartyRelationship{ArchetypeNodeID: "openEHR-DEMOGRAPHIC-PARTY_RELATIONSHIP.r.v1", Name: rm.DVText{Value: "r"}},
			member: "source",
		},
		{
			name:   "a zero CODE_PHRASE held by value in COMPOSITION.language",
			value:  &rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.c.v1", Name: rm.DVText{Value: "c"}},
			member: "language",
		},
		{
			name:   "a zero data in POINT_EVENT[ItemTree]",
			value:  &rm.PointEvent[rm.ItemTree]{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "e"}},
			member: "data",
		},
		{
			name:   "a nil ontology in an AOM 1.4 ARCHETYPE",
			value:  &aom14.Archetype{Concept: "at0000"},
			member: "ontology",
		},
		{
			name:   "a nil item in an AOM 1.4 EXPR_LEAF",
			value:  &aom14.ExprLeaf{Type: "String", ReferenceType: "constant"},
			member: "item",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := canjson.Marshal(tc.value)
			if err != nil {
				t.Fatalf("canjson.Marshal(%T): %v", tc.value, err)
			}
			if raw, ok := memberValues(t, got)[tc.member]; ok {
				t.Errorf("canjson.Marshal(%T) = %s\nwrites %q as %s, want the member absent", tc.value, got, tc.member, raw)
			}
		})
	}
}

// TestREQ052_MandatoryMemberWithAValueIsWritten is the control for the test
// above: a mandatory field that holds a value is written, even when the value
// is empty. A concrete RM value an interface holds encodes with its `_type`,
// so a zero one is still written, and so is one a type-parameter field
// instantiated with an interface holds; a struct held by value with any
// field set is written in full; the two struct members a fixed reading
// decides, EHR_STATUS subject and ARCHETYPED archetype_id, are written
// whatever they hold; a primitive held by value has no absent form; and an
// `any` holding an empty string or a zero class value holds a value.
//
// REQ-052 § An absent mandatory member: a struct held by value is left out
// only when its zero test holds; any other mandatory member held by value is
// written.
func TestREQ052_MandatoryMemberWithAValueIsWritten(t *testing.T) {
	for _, tc := range []struct {
		name   string
		value  any
		member string
		// want is the member's expected value. When it is empty, the member
		// must be an object whose `_type` is wantType.
		want     string
		wantType string
	}{
		{
			name:     "a zero DV_TEXT in ELEMENT.name",
			value:    &rm.Element{ArchetypeNodeID: "at0001", Name: rm.DVText{}},
			member:   "name",
			wantType: "DV_TEXT",
		},
		{
			name:     "a PARTY_REF with only its type set, held by value in PARTY_RELATIONSHIP.source",
			value:    &rm.PartyRelationship{ArchetypeNodeID: "openEHR-DEMOGRAPHIC-PARTY_RELATIONSHIP.r.v1", Name: rm.DVText{Value: "r"}, Source: rm.PartyRef{Type: "PERSON"}},
			member:   "source",
			wantType: "PARTY_REF",
		},
		{
			// The floor's content rule reads this CODE_PHRASE as absent, as its
			// code and terminology are empty; the encoder reads every field.
			name:     "a CODE_PHRASE with only its preferred_term set, held by value in COMPOSITION.language",
			value:    &rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.c.v1", Name: rm.DVText{Value: "c"}, Language: rm.CodePhrase{PreferredTerm: new("Dutch")}},
			member:   "language",
			wantType: "CODE_PHRASE",
		},
		{
			name:     "an ITEM_TREE with only its archetype_node_id set in POINT_EVENT[ItemTree].data",
			value:    &rm.PointEvent[rm.ItemTree]{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "e"}, Data: rm.ItemTree{ArchetypeNodeID: "at0003"}},
			member:   "data",
			wantType: "ITEM_TREE",
		},
		{
			name:     "a zero PARTY_SELF held by value in EHR_STATUS.subject",
			value:    &rm.EHRStatus{ArchetypeNodeID: "openEHR-EHR-EHR_STATUS.generic.v1", Name: rm.DVText{Value: "s"}},
			member:   "subject",
			wantType: "PARTY_SELF",
		},
		{
			name:     "a zero ARCHETYPE_ID held by value in ARCHETYPED.archetype_id",
			value:    &rm.Archetyped{RMVersion: "1.0.4"},
			member:   "archetype_id",
			wantType: "ARCHETYPE_ID",
		},
		{
			name:   "an empty string held by value in ELEMENT.archetype_node_id",
			value:  &rm.Element{Name: rm.DVText{Value: "n"}},
			member: "archetype_node_id",
			want:   `""`,
		},
		{
			name:     "a zero ITEM_TREE an interface holds in POINT_EVENT[ItemStructure].data",
			value:    &rm.PointEvent[rm.ItemStructure]{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "e"}, Data: rm.ItemTree{}},
			member:   "data",
			wantType: "ITEM_TREE",
		},
		{
			name:   "an empty items list in REVISION_HISTORY",
			value:  &rm.RevisionHistory{Items: []rm.RevisionHistoryItem{}},
			member: "items",
			want:   `[]`,
		},
		{
			name:   "an empty string in an AOM 1.4 EXPR_LEAF.item",
			value:  &aom14.ExprLeaf{Type: "String", ReferenceType: "constant", Item: ""},
			member: "item",
			want:   `""`,
		},
		{
			name:     "a zero class value in an AOM 1.4 EXPR_LEAF.item",
			value:    &aom14.ExprLeaf{Type: "C_STRING", ReferenceType: "constant", Item: &aom14.CString{}},
			member:   "item",
			wantType: "C_STRING",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := canjson.Marshal(tc.value)
			if err != nil {
				t.Fatalf("canjson.Marshal(%T): %v", tc.value, err)
			}
			raw, ok := memberValues(t, got)[tc.member]
			if !ok {
				t.Fatalf("canjson.Marshal(%T) = %s\nleaves out %q, want it written", tc.value, got, tc.member)
			}
			if tc.want != "" {
				if same, diff := wireequiv.Equivalent([]byte(tc.want), raw); !same {
					t.Errorf("canjson.Marshal(%T) writes %q as %s, want %s: %s", tc.value, tc.member, raw, tc.want, diff)
				}
				return
			}
			var obj struct {
				Type string `json:"_type"`
			}
			if err := json.Unmarshal(raw, &obj); err != nil || obj.Type != tc.wantType {
				t.Errorf("canjson.Marshal(%T) writes %q as %s, want an object with _type %q (decode error: %v)", tc.value, tc.member, raw, tc.wantType, err)
			}
		})
	}
}

// memberValues reads data as one JSON object and returns its members by
// name. It reads with encoding/json, not the codec under test.
func memberValues(t *testing.T, data []byte) map[string]json.RawMessage {
	t.Helper()
	var members map[string]json.RawMessage
	if err := json.Unmarshal(data, &members); err != nil {
		t.Fatalf("read %s as a JSON object: %v", data, err)
	}
	return members
}
