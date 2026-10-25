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
// nil uidSource stamps none, and neither does one that returns nil. The
// caller decides: a locatable gets one only where its OPT requires it
// (setLocatableIdentity), whatever its class.
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
		// replay) wins over the generator's uidSource. A source that gives
		// nil leaves the uid unset, with no nil pointer stored in its place.
		if l := rmValue.(rm.Locatable); l.GetUID() == nil {
			if uid := uidSource(); uid != nil {
				m.SetUID(uid)
			}
		}
	}
}
