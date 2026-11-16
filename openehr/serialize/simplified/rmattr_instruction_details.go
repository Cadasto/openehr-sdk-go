package simplified

// REQ-140 — ACTION `instruction_details`, both directions.
//
// The reference spells an ACTION's INSTRUCTION_DETAILS as three suffixes of
// `<action>/_instruction_details`: `|composition_uid`, the id of the
// INSTRUCTION's composition, `|activity_id`, the archetype path of the
// activity, and `|path`, the INSTRUCTION's path in that composition. The
// suffixes spell neither the reference's namespace nor its type, nor the id's
// subtype: the reference implementation's unmarshaller always builds a
// LOCATABLE_REF to an INSTRUCTION in namespace local whose id is a
// HIER_OBJECT_ID, and so does decode here. It keeps an empty `|path`, which
// EHRbase writes on every ACTION; decode reads it as absent, since an empty
// path and none both mean the composition itself.
//
// Encode writes the three suffixes from a reference of that shape and refuses
// any other: a namespace other than local, a type other than INSTRUCTION, an id
// that is not a HIER_OBJECT_ID, or a `wf_details`. The family reaches its
// value through no datatype position, so there is no `|raw` carrier to fall
// back to.

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// instructionDetailsTails is the INSTRUCTION_DETAILS suffix set. The
// reference also reads `|instruction_uid`, `|wt_path`, `|instruction_index`
// and `|activity_index`, from which it would compute the path or the
// activity id; they are outside the set, so they are unmatched content.
var instructionDetailsTails = map[string]bool{"|composition_uid": true, "|activity_id": true, "|path": true}

// The namespace and the type of every reference the family spells.
const (
	instructionRefNamespace = "local"
	instructionRefType      = "INSTRUCTION"
)

// decodeRMAttrInstructionDetails decodes `_instruction_details` at an ACTION
// into a canonical INSTRUCTION_DETAILS. `|composition_uid` and `|activity_id`
// are RM-mandatory, so a missing or null one is refused rather than read as
// an empty string; `|path` is optional, and an empty one is absent.
func decodeRMAttrInstructionDetails(g rmattrGroup, _ string) (any, error) {
	if err := checkRMAttrTails(g, instructionDetailsTails); err != nil {
		return nil, err
	}
	uid, err := rmattrString(g, "|composition_uid")
	if err != nil {
		return nil, err
	}
	activity, err := rmattrString(g, "|activity_id")
	if err != nil {
		return nil, err
	}
	ref := map[string]any{
		"_type":     "LOCATABLE_REF",
		"id":        map[string]any{"_type": "HIER_OBJECT_ID", "value": uid},
		"namespace": instructionRefNamespace,
		"type":      instructionRefType,
	}
	if _, spelled := g.tails["|path"]; spelled {
		path, err := rmattrString(g, "|path")
		if err != nil {
			return nil, err
		}
		if path != "" {
			ref["path"] = path
		}
	}
	return map[string]any{"_type": "INSTRUCTION_DETAILS", "instruction_id": ref, "activity_id": activity}, nil
}

// instructionDetailsRMAttr writes an ACTION's instruction_details at path
// (`<action>/_instruction_details`): `|composition_uid`, `|activity_id`, and
// `|path` when the reference has a path that is not empty. A nil one writes
// nothing. What the three suffixes cannot carry is a typed error naming the
// key and not the value, since decode would read it back as another value.
func instructionDetailsRMAttr(out map[string]any, path string, d *rm.InstructionDetails) error {
	if d == nil {
		return nil
	}
	if d.WfDetails != nil && !rm.IsTypedNil(d.WfDetails) {
		return fmt.Errorf("%w: %q carries an INSTRUCTION_DETAILS.wf_details, which the reference's suffix set has no channel for",
			ErrUnsupportedDatatype, path)
	}
	ref := d.InstructionID
	if ref.Namespace != instructionRefNamespace || ref.Type != instructionRefType {
		return fmt.Errorf("%w: %q carries an instruction_id whose namespace is not %s or whose type is not %s; the suffix set implies both",
			ErrUnsupportedDatatype, path, instructionRefNamespace, instructionRefType)
	}
	var uid string
	switch id := ref.GetID().(type) {
	case nil:
		return fmt.Errorf("%w: %q is RM-mandatory on LOCATABLE_REF but absent", ErrUnsupportedDatatype, path+"|composition_uid")
	case rm.HierObjectID:
		uid = id.Value
	case *rm.HierObjectID:
		if id == nil {
			return fmt.Errorf("%w: %q is RM-mandatory on LOCATABLE_REF but absent", ErrUnsupportedDatatype, path+"|composition_uid")
		}
		uid = id.Value
	default:
		return fmt.Errorf("%w: %q cannot carry a %T; decode reads every composition_uid as a HIER_OBJECT_ID",
			ErrUnsupportedDatatype, path+"|composition_uid", id)
	}
	out[path+"|composition_uid"] = uid
	out[path+"|activity_id"] = d.ActivityID
	if ref.Path != nil && *ref.Path != "" {
		out[path+"|path"] = *ref.Path
	}
	return nil
}
