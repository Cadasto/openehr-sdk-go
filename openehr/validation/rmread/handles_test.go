package rmread

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// handledTypes pins the RM types Handles must report as modelled — the same
// set ReadSingle/ReadMultiple dispatch. It is a golden checklist: when a new
// readXxxSingle/readXxxMultiple reader is added, its type MUST be added here
// AND to Handles. Removing a type from Handles without removing it here trips
// TestHandles_ModelledTypes; the reverse (a reader added but omitted from
// Handles) is caught by a reviewer noticing this list is stale.
//
// Value form only — Handles covers `*rm.T` and `rm.T` identically, and the
// pointer form is spot-checked in TestHandles_PointerForm.
var handledTypes = []any{
	// ENTRY + structural
	rm.Composition{},
	rm.Observation{},
	rm.Evaluation{},
	rm.Instruction{},
	rm.Action{},
	rm.AdminEntry{},
	rm.GenericEntry{},
	rm.Section{},
	rm.Activity{},
	rm.EventContext{},
	rm.History[rm.ItemStructure]{},
	rm.PointEvent[rm.ItemStructure]{},
	rm.IntervalEvent[rm.ItemStructure]{},
	rm.ItemTree{},
	rm.ItemList{},
	rm.ItemSingle{},
	rm.ItemTable{},
	rm.Cluster{},
	rm.Element{},
	// data values
	rm.DVText{},
	rm.DVCodedText{},
	rm.CodePhrase{},
	rm.DVDate{},
	rm.DVTime{},
	rm.DVDateTime{},
	rm.DVDuration{},
	rm.DVBoolean{},
	rm.DVIdentifier{},
	rm.DVMultimedia{},
	rm.DVCount{},
	rm.DVQuantity{},
	rm.DVProportion{},
	rm.DVURI{},
	rm.DVEHRURI{},
	rm.DVParsable{},
	// intervals
	rm.DVInterval[rm.DVQuantity]{},
	rm.DVInterval[rm.DVCount]{},
	rm.DVInterval[rm.DVDateTime]{},
	rm.DVInterval[rm.DVDate]{},
	rm.DVInterval[rm.DVTime]{},
	rm.DVInterval[rm.DVProportion]{},
	rm.DVInterval[rm.DVOrdered]{},
	// demographic
	rm.Person{},
	rm.Organisation{},
	rm.Group{},
	rm.Agent{},
	rm.Role{},
	rm.Address{},
	rm.Contact{},
	rm.PartyIdentity{},
	rm.PartyRelationship{},
	rm.Capability{},
	// EHR-IM roots
	rm.Folder{},
	rm.EHRStatus{},
}

func TestHandles_ModelledTypes(t *testing.T) {
	if got, want := len(handledTypes), 54; got != want {
		t.Errorf("handledTypes has %d entries, want %d — keep it in sync with Handles/ReadSingle", got, want)
	}
	for _, v := range handledTypes {
		if !Handles(v) {
			t.Errorf("Handles(%T) = false, want true (modelled by ReadSingle/ReadMultiple)", v)
		}
	}
}

func TestHandles_PointerForm(t *testing.T) {
	// Handles must accept the pointer form too — readers are reached via both,
	// and the walker passes whatever rmread/the caller boxes.
	ptrs := []any{
		&rm.Composition{}, &rm.DVQuantity{}, &rm.DVInterval[rm.DVQuantity]{},
		&rm.Folder{}, &rm.EHRStatus{}, &rm.Cluster{},
	}
	for _, v := range ptrs {
		if !Handles(v) {
			t.Errorf("Handles(%T) = false, want true (pointer form)", v)
		}
	}
}

func TestHandles_Unmodelled(t *testing.T) {
	// Types rmread does NOT model must report false so the floor treats them
	// as opaque leaves (validated by their own invariant evaluators) rather
	// than reading their members back as absent and fabricating `required`.
	// Includes a flattened scalar, an OBJECT_REF, and a PARTY proxy concrete
	// (recognised by rmTypeInfo but not modelled here).
	unmodelled := []any{
		"a flattened string",
		42,
		rm.ObjectRef{},
		&rm.ObjectRef{},
		rm.PartySelf{},
		nil,
	}
	for _, v := range unmodelled {
		if Handles(v) {
			t.Errorf("Handles(%T) = true, want false (not modelled by rmread)", v)
		}
	}
}
