package rmwrite

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// writeObjectRefSingle writes the attributes OBJECT_REF declares, which
// PARTY_REF and ACCESS_GROUP_REF inherit: an id of any OBJECT_ID class,
// stored as given, and the namespace and type. class names the Go type
// for the error detail.
func writeObjectRefSingle(ref *rm.ObjectRef, class, attr string, child any) error {
	switch attr {
	case "id":
		v, ok := child.(rm.ObjectID)
		if !ok || rm.IsTypedNil(v) {
			return mismatch(attr, child, "OBJECT_ID")
		}
		ref.ID = v
		return nil
	case "namespace":
		return writeStringValue(&ref.Namespace, attr, child)
	case "type":
		return writeStringValue(&ref.Type, attr, child)
	}
	return fmt.Errorf("%w: %s has no single attr %q", ErrUnknownAttribute, class, attr)
}

// writeLocatableRefSingle writes a LOCATABLE_REF: the UID_BASED_ID id it
// declares itself, stored as given in its own field rather than the
// OBJECT_REF one it embeds, its path, and the namespace and type it
// inherits.
func writeLocatableRefSingle(ref *rm.LocatableRef, attr string, child any) error {
	switch attr {
	case "id":
		v, ok := child.(rm.UIDBasedID)
		if !ok || rm.IsTypedNil(v) {
			return mismatch(attr, child, "UID_BASED_ID")
		}
		ref.ID = v
		return nil
	case "path":
		return writeOptionalString(&ref.Path, attr, child)
	}
	return writeObjectRefSingle(&ref.ObjectRef, "*rm.LocatableRef", attr, child)
}

// writeObjectIDValue writes the value of an OBJECT_ID class whose one
// attribute it is: HIER_OBJECT_ID, OBJECT_VERSION_ID, ARCHETYPE_ID,
// TEMPLATE_ID and TERMINOLOGY_ID. class names the Go type for the error
// detail.
func writeObjectIDValue(class string, value *string, attr string, child any) error {
	if attr == "value" {
		return writeStringValue(value, attr, child)
	}
	return fmt.Errorf("%w: %s has no single attr %q", ErrUnknownAttribute, class, attr)
}

// writeGenericIDSingle writes a GENERIC_ID's value and scheme.
func writeGenericIDSingle(id *rm.GenericID, attr string, child any) error {
	if attr == "scheme" {
		return writeStringValue(&id.Scheme, attr, child)
	}
	return writeObjectIDValue("*rm.GenericID", &id.Value, attr, child)
}

// assignObjectRef stores an OBJECT_REF, or any class that descends from
// it, as given: an ENTRY's workflow_id, a CARE_ENTRY's guideline_id, a
// member of a FOLDER's items. A nil child or a nil pointer is a type
// mismatch.
func assignObjectRef(child any, set func(rm.ObjectRefLike), attr string) error {
	v, ok := child.(rm.ObjectRefLike)
	if !ok || rm.IsTypedNil(v) {
		return mismatch(attr, child, "OBJECT_REF")
	}
	set(v)
	return nil
}

// writeInstructionDetailsSingle writes the INSTRUCTION_DETAILS an ACTION
// holds: the LOCATABLE_REF to the instruction, the activity_id and the
// wf_details.
func writeInstructionDetailsSingle(d *rm.InstructionDetails, attr string, child any) error {
	switch attr {
	case "instruction_id":
		// Value field. The synthesiser passes *LocatableRef.
		return assignVia(child, func(v rm.LocatableRef) { d.InstructionID = v }, attr, "LOCATABLE_REF")
	case "activity_id":
		return writeStringValue(&d.ActivityID, attr, child)
	case "wf_details":
		return assignItemStructure(child, func(v rm.ItemStructure) { d.WfDetails = v }, attr)
	}
	return fmt.Errorf("%w: *rm.InstructionDetails has no single attr %q", ErrUnknownAttribute, attr)
}
