package rmwrite

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// writeEHRStatusSingle writes an EHR_STATUS's name, its PARTY_SELF subject,
// its other_details and its two Boolean flags.
func writeEHRStatusSingle(s *rm.EHRStatus, attr string, child any) error {
	switch attr {
	case "name":
		return assignDVTextLike(child, func(v rm.DVTextLike) { s.Name = v }, attr)
	case "subject":
		// Value field. The synthesiser passes *PartySelf.
		return assignVia(child, func(v rm.PartySelf) { s.Subject = v }, attr, "PARTY_SELF")
	case "other_details":
		return assignItemStructure(child, func(v rm.ItemStructure) { s.OtherDetails = v }, attr)
	case "is_queryable":
		return writeBoolean(&s.IsQueryable, attr, child)
	case "is_modifiable":
		return writeBoolean(&s.IsModifiable, attr, child)
	}
	return fmt.Errorf("%w: *rm.EHRStatus has no single attr %q", ErrUnknownAttribute, attr)
}

// writeBoolean stores a Boolean child on a mandatory Boolean attribute.
func writeBoolean(dst *bool, attr string, child any) error {
	v, ok := child.(bool)
	if !ok {
		return mismatch(attr, child, "Boolean")
	}
	*dst = v
	return nil
}

// writeFolderSingle writes a FOLDER's name and details.
func writeFolderSingle(f *rm.Folder, attr string, child any) error {
	switch attr {
	case "name":
		return assignDVTextLike(child, func(v rm.DVTextLike) { f.Name = v }, attr)
	case "details":
		return assignItemStructure(child, func(v rm.ItemStructure) { f.Details = v }, attr)
	}
	return fmt.Errorf("%w: *rm.Folder has no single attr %q", ErrUnknownAttribute, attr)
}

// writeFolderMultiple appends a sub-folder, given as a value or as a
// pointer, to a FOLDER's folders, or a reference of any OBJECT_REF class,
// as given, to its items.
func writeFolderMultiple(f *rm.Folder, attr string, child any) error {
	switch attr {
	case "folders":
		return assignVia(child, func(v rm.Folder) { f.Folders = append(f.Folders, v) }, attr, "FOLDER")
	case "items":
		return assignObjectRef(child, func(v rm.ObjectRefLike) { f.Items = append(f.Items, v) }, attr)
	}
	return fmt.Errorf("%w: *rm.Folder has no multiple attr %q", ErrUnknownAttribute, attr)
}
