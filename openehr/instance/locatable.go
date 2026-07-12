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
// Non-LOCATABLE RM types (DV*, EventContext) and value-form
// (non-pointer) inputs silently no-op — MutableLocatable is satisfied
// by *T only. Deliberate widening vs the previous 18-arm switch:
// every LOCATABLE concrete the template compiler can yield (FOLDER,
// EHR_STATUS, the demographic PARTY family, …) now gets its identity
// stamped rather than silently skipped; the uid policy below is
// unchanged.
func applyLocatableIdentity(rmValue any, nodeID, name string, archetypeDetails *rm.Archetyped, uidSource func() *rm.HierObjectID) {
	m, ok := rmValue.(rm.MutableLocatable)
	if !ok {
		return
	}
	m.SetArchetypeNodeID(nodeID)
	m.SetName(rm.DVText{Value: name})
	if archetypeDetails != nil {
		m.SetArchetypeDetails(archetypeDetails)
	}
	if stampsUID(rmValue) {
		// Set-only-if-unset: an explicitly provided UID (e.g. a fixture
		// replay) wins over the generator's uidSource.
		if l := rmValue.(rm.Locatable); l.GetUID() == nil {
			m.SetUID(uidSource())
		}
	}
}

// stampsUID lists the classes whose generated instances carry a fresh
// uid (REQ-107 emission policy): COMPOSITION and the ENTRY concretes —
// the openEHR entry-level, independently addressable objects.
// Structure nodes (SECTION, ITEM_*, CLUSTER, ELEMENT, HISTORY, events)
// deliberately do not get generator-minted uids. This is policy
// dispatch, not identity plumbing — it stays a hand-written closed set
// (REQ-024, no reflection).
func stampsUID(v any) bool {
	switch v.(type) {
	case *rm.Composition, *rm.Observation, *rm.Evaluation,
		*rm.Instruction, *rm.Action, *rm.AdminEntry, *rm.GenericEntry:
		return true
	}
	return false
}
