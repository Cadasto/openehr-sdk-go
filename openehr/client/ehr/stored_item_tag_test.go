package ehr_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// objectIDValue returns the value of an OBJECT_REF's id and its RM class.
// The type registry decodes an identifier to a pointer.
func objectIDValue(id rm.ObjectID) (value, class string) {
	switch v := id.(type) {
	case *rm.HierObjectID:
		return v.Value, "HIER_OBJECT_ID"
	case *rm.ObjectVersionID:
		return v.Value, "OBJECT_VERSION_ID"
	default:
		return "", ""
	}
}

// TestStoredItemTagDecodesTheTagsListBody holds the shared tag type to the
// ITEM_TAG list the pinned tags routes return (REQ-050, REQ-059): key, value
// and target path, and target and owner_id as OBJECT_REFs whose id keeps its
// concrete class, so a version target and a versioned-object target stay
// apart, and whose type names the tagged party's class.
func TestStoredItemTagDecodesTheTagsListBody(t *testing.T) {
	var tags []ehr.StoredItemTag
	if err := canjson.Unmarshal(readFixture(t, "demographic", "demographic_tags.json"), &tags); err != nil {
		t.Fatalf("decode demographic_tags.json: %v", err)
	}
	type want struct {
		key, value, targetPath string
		targetID, targetClass  string
		targetType             string
	}
	wants := []want{
		{"flag", "follow-up", "", "8849182c-82ad-4088-a07f-48ead4180515", "HIER_OBJECT_ID", "PERSON"},
		{"flag", "review", "", "0fd8e2c4-1b7a-4c55-9f3e-5a2d6b8c9e01::cdr.example.com::2", "OBJECT_VERSION_ID", "ORGANISATION"},
		{"flag", "follow-up", "/identities", "3c5e7a91-2d4f-4b6a-8e0c-1f9d7b3a5c22", "HIER_OBJECT_ID", "AGENT"},
	}
	if len(tags) != len(wants) {
		t.Fatalf("decoded %d tags, want %d", len(tags), len(wants))
	}
	for i, w := range wants {
		got := tags[i]
		if got.Key != w.key || got.Value != w.value || got.TargetPath != w.targetPath {
			t.Errorf("tag[%d] = key %q value %q target_path %q, want %q %q %q",
				i, got.Key, got.Value, got.TargetPath, w.key, w.value, w.targetPath)
		}
		id, class := objectIDValue(got.Target.ID)
		if id != w.targetID || class != w.targetClass {
			t.Errorf("tag[%d].Target.ID = %q (%s), want %q (%s)", i, id, class, w.targetID, w.targetClass)
		}
		if got.Target.Type != w.targetType || got.Target.Namespace != "local" {
			t.Errorf("tag[%d].Target = type %q namespace %q, want %q %q", i, got.Target.Type, got.Target.Namespace, w.targetType, "local")
		}
		owner, ownerClass := objectIDValue(got.OwnerID.ID)
		if owner != "7d44b88c-4199-4bad-97dc-d78268e01398" || ownerClass != "HIER_OBJECT_ID" || got.OwnerID.Type != "SYSTEM" {
			t.Errorf("tag[%d].OwnerID = %q (%s) type %q, want the fixture's SYSTEM owner", i, owner, ownerClass, got.OwnerID.Type)
		}
	}
}

// TestStoredItemTagItemTagKeepsTheWritablePart: the tag a tags write sends
// back is the stored tag's key, value and target path, so a read can be
// edited and written without copying fields by hand.
func TestStoredItemTagItemTagKeepsTheWritablePart(t *testing.T) {
	st := ehr.StoredItemTag{
		Key:        "flag",
		Value:      "follow-up",
		TargetPath: "/identities",
		Target:     rm.ObjectRef{ID: &rm.HierObjectID{Value: "8849182c-82ad-4088-a07f-48ead4180515"}, Namespace: "local", Type: "PERSON"},
		OwnerID:    rm.ObjectRef{ID: &rm.HierObjectID{Value: "7d44b88c-4199-4bad-97dc-d78268e01398"}, Namespace: "local", Type: "SYSTEM"},
	}
	want := ehr.ItemTag{Key: "flag", Value: "follow-up", TargetPath: "/identities"}
	if got := st.ItemTag(); got != want {
		t.Errorf("ItemTag() = %+v, want %+v", got, want)
	}
}
