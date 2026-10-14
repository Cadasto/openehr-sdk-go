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
// a nil type-parameter field instantiated with an interface, a nil pointer
// and a nil `any`. Each member must be left out.
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
// is empty. A concrete RM value encodes with its `_type`, so a zero one is
// still written; a struct or primitive held by value, and a type-parameter
// field instantiated by value, have no absent form; and an `any` holding an
// empty string or object holds a value.
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
			name:     "a zero HIER_OBJECT_ID held by value in CONTRIBUTION.uid",
			value:    &rm.Contribution{Versions: []rm.ObjectRefLike{}},
			member:   "uid",
			wantType: "HIER_OBJECT_ID",
		},
		{
			name:   "an empty string held by value in ELEMENT.archetype_node_id",
			value:  &rm.Element{Name: rm.DVText{Value: "n"}},
			member: "archetype_node_id",
			want:   `""`,
		},
		{
			name:     "a zero data in POINT_EVENT[ItemTree]",
			value:    &rm.PointEvent[rm.ItemTree]{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "e"}},
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
			name:   "an empty object in an AOM 1.4 EXPR_LEAF.item",
			value:  &aom14.ExprLeaf{Type: "Object", ReferenceType: "constant", Item: map[string]any{}},
			member: "item",
			want:   `{}`,
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
