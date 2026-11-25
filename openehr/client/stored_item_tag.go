package client

import "github.com/cadasto/openehr-sdk-go/openehr/rm"

// StoredItemTag is an ITEM_TAG as a server stores it and returns it from a
// tags route: the tag's key, value and target path, the object the tag is
// attached to, and the object that owns it. Tags on one target are told
// apart by their key and target path together.
//
// It decodes from the JSON list the openEHR REST tags routes return, where
// target and owner_id are OBJECT_REFs. [StoredItemTag.ItemTag] gives the part
// a tags write sends.
type StoredItemTag struct {
	// Key is the tag's key.
	Key string `json:"key"`
	// Value is the tag's value; empty when the tag has none.
	Value string `json:"value,omitempty"`
	// TargetPath is the path, within the target, of the element the tag
	// marks; empty when the tag marks the whole target.
	TargetPath string `json:"target_path,omitempty"`
	// Target is the tagged object. Its ID is an OBJECT_VERSION_ID for a tag
	// on one version and a HIER_OBJECT_ID for a tag on the versioned
	// object; its Type names the object's class, such as PERSON.
	Target rm.ObjectRef `json:"target"`
	// OwnerID is the object that owns the tag, as the server reports it.
	OwnerID rm.ObjectRef `json:"owner_id"`
}

// ItemTag returns the tag's key, value and target path: the part a tags
// write sends, so a list read from a server can be edited and written back.
func (t StoredItemTag) ItemTag() ItemTag {
	return ItemTag{Key: t.Key, Value: t.Value, TargetPath: t.TargetPath}
}
