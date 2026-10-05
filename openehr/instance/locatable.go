package instance

import (
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// applyLocatableIdentity stamps archetype_node_id, name, and (where
// the RM mandates) uid + archetype_details on a freshly-constructed
// RM value. Identity writes go through the generated
// rm.MutableLocatable surface (ADR 0013) — REQ-024, no reflection.
//
// archetypeDetails is non-nil only for archetype-root pins and the
// template root; event nodes are never archetype roots, so the
// details branch is inert for them (the previous per-type switch
// omitted it on the event arms — same observable behaviour).
// Non-LOCATABLE RM types (DV*, EventContext), value-form
// (non-pointer) inputs, and typed-nil pointers silently no-op —
// MutableLocatable is satisfied by *T only, and a typed-nil would
// panic in the setters (and in the GetUID read below), so the write
// path guards like every read path does (ADR 0013 guard-before-read).
// Deliberate widening vs the previous 18-arm switch:
// every LOCATABLE concrete the template compiler can yield (FOLDER,
// EHR_STATUS, the demographic PARTY family, …) now gets its identity
// stamped rather than silently skipped. The uid comes from uidSource; a
// nil uidSource stamps none. The caller decides: the classes stampsUID
// names get one unless their OPT prohibits it, and any other locatable
// only where its OPT requires one (setLocatableIdentity, uidFor).
func applyLocatableIdentity(rmValue any, nodeID, name string, archetypeDetails *rm.Archetyped, uidSource func() *rm.HierObjectID) {
	m, ok := rmValue.(rm.MutableLocatable)
	if !ok || rm.IsTypedNil(rmValue) {
		return
	}
	m.SetArchetypeNodeID(nodeID)
	m.SetName(rm.DVText{Value: name})
	if archetypeDetails != nil {
		m.SetArchetypeDetails(archetypeDetails)
	}
	if uidSource != nil {
		// Set-only-if-unset: an explicitly provided UID (e.g. a fixture
		// replay) wins over the generator's uidSource.
		if l := rmValue.(rm.Locatable); l.GetUID() == nil {
			m.SetUID(uidSource())
		}
	}
}

// partyNeedsUID reports whether v is a PARTY, whose uid the RM needs
// (PARTY Uid_mandatory), so the generator stamps it even where the OPT
// prohibits uid.
func partyNeedsUID(v any) bool {
	switch v.(type) {
	case *rm.Person, *rm.Organisation, *rm.Group, *rm.Agent, *rm.Role:
		return true
	}
	return false
}

// stampsUID lists the classes whose generated instances carry a fresh
// uid (REQ-107 emission policy): COMPOSITION, the ENTRY concretes and
// GENERIC_ENTRY, the independently addressable clinical objects, and the
// PARTY concretes, whose uid the RM requires (PARTY Uid_mandatory). A
// PARTY_RELATIONSHIP takes its uid in fillPartyRelationship instead.
// Structure nodes (SECTION, ITEM_*, CLUSTER, ELEMENT, HISTORY, events)
// and the other demographic locatables (PARTY_IDENTITY, CONTACT,
// ADDRESS, CAPABILITY) deliberately get no generator-minted uid. This is
// policy dispatch, not identity plumbing — it stays a hand-written
// closed set (REQ-024, no reflection).
func stampsUID(v any) bool {
	switch v.(type) {
	case *rm.Composition, *rm.Observation, *rm.Evaluation,
		*rm.Instruction, *rm.Action, *rm.AdminEntry, *rm.GenericEntry,
		*rm.Person, *rm.Organisation, *rm.Group, *rm.Agent, *rm.Role:
		return true
	}
	return false
}
