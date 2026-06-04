package instance

import (
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// applyLocatableIdentity stamps archetype_node_id, name, and (where
// the RM mandates) uid + archetype_details on a freshly-constructed
// RM value. Dispatch is a closed type switch on the Go concrete
// type — REQ-024, no reflection.
//
// archetypeDetails is non-nil only for archetype-root pins and the
// template root. Non-LOCATABLE RM types (DV*, EventContext) silently
// no-op — they carry neither archetype_node_id nor name.
func applyLocatableIdentity(rmValue any, nodeID, name string, archetypeDetails *rm.Archetyped, uidSource func() *rm.HierObjectID) {
	switch v := rmValue.(type) {
	case *rm.Composition:
		v.ArchetypeNodeID = nodeID
		v.Name = rm.DVText{Value: name}
		if archetypeDetails != nil {
			v.ArchetypeDetails = archetypeDetails
		}
		if v.UID == nil {
			id := uidSource()
			v.UID = id
		}
	case *rm.Observation:
		v.ArchetypeNodeID = nodeID
		v.Name = rm.DVText{Value: name}
		if archetypeDetails != nil {
			v.ArchetypeDetails = archetypeDetails
		}
		if v.UID == nil {
			id := uidSource()
			v.UID = id
		}
	case *rm.Evaluation:
		v.ArchetypeNodeID = nodeID
		v.Name = rm.DVText{Value: name}
		if archetypeDetails != nil {
			v.ArchetypeDetails = archetypeDetails
		}
		if v.UID == nil {
			id := uidSource()
			v.UID = id
		}
	case *rm.Instruction:
		v.ArchetypeNodeID = nodeID
		v.Name = rm.DVText{Value: name}
		if archetypeDetails != nil {
			v.ArchetypeDetails = archetypeDetails
		}
		if v.UID == nil {
			id := uidSource()
			v.UID = id
		}
	case *rm.Action:
		v.ArchetypeNodeID = nodeID
		v.Name = rm.DVText{Value: name}
		if archetypeDetails != nil {
			v.ArchetypeDetails = archetypeDetails
		}
		if v.UID == nil {
			id := uidSource()
			v.UID = id
		}
	case *rm.AdminEntry:
		v.ArchetypeNodeID = nodeID
		v.Name = rm.DVText{Value: name}
		if archetypeDetails != nil {
			v.ArchetypeDetails = archetypeDetails
		}
		if v.UID == nil {
			id := uidSource()
			v.UID = id
		}
	case *rm.GenericEntry:
		v.ArchetypeNodeID = nodeID
		v.Name = rm.DVText{Value: name}
		if archetypeDetails != nil {
			v.ArchetypeDetails = archetypeDetails
		}
		if v.UID == nil {
			id := uidSource()
			v.UID = id
		}
	case *rm.Section:
		v.ArchetypeNodeID = nodeID
		v.Name = rm.DVText{Value: name}
		if archetypeDetails != nil {
			v.ArchetypeDetails = archetypeDetails
		}
	case *rm.Activity:
		v.ArchetypeNodeID = nodeID
		v.Name = rm.DVText{Value: name}
		if archetypeDetails != nil {
			v.ArchetypeDetails = archetypeDetails
		}
	case *rm.History[rm.ItemStructure]:
		v.ArchetypeNodeID = nodeID
		v.Name = rm.DVText{Value: name}
		if archetypeDetails != nil {
			v.ArchetypeDetails = archetypeDetails
		}
	case *rm.PointEvent[rm.ItemStructure]:
		v.ArchetypeNodeID = nodeID
		v.Name = rm.DVText{Value: name}
	case *rm.IntervalEvent[rm.ItemStructure]:
		v.ArchetypeNodeID = nodeID
		v.Name = rm.DVText{Value: name}
	case *rm.ItemTree:
		v.ArchetypeNodeID = nodeID
		v.Name = rm.DVText{Value: name}
		if archetypeDetails != nil {
			v.ArchetypeDetails = archetypeDetails
		}
	case *rm.ItemList:
		v.ArchetypeNodeID = nodeID
		v.Name = rm.DVText{Value: name}
		if archetypeDetails != nil {
			v.ArchetypeDetails = archetypeDetails
		}
	case *rm.ItemSingle:
		v.ArchetypeNodeID = nodeID
		v.Name = rm.DVText{Value: name}
		if archetypeDetails != nil {
			v.ArchetypeDetails = archetypeDetails
		}
	case *rm.ItemTable:
		v.ArchetypeNodeID = nodeID
		v.Name = rm.DVText{Value: name}
		if archetypeDetails != nil {
			v.ArchetypeDetails = archetypeDetails
		}
	case *rm.Cluster:
		v.ArchetypeNodeID = nodeID
		v.Name = rm.DVText{Value: name}
		if archetypeDetails != nil {
			v.ArchetypeDetails = archetypeDetails
		}
	case *rm.Element:
		v.ArchetypeNodeID = nodeID
		v.Name = rm.DVText{Value: name}
		if archetypeDetails != nil {
			v.ArchetypeDetails = archetypeDetails
		}
		// Non-LOCATABLE / DataValue concretes: nothing to stamp.
	}
}
