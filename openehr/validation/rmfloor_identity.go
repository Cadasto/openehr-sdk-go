package validation

// rmfloor_identity.go: REQ-112 § Driver — which values the RM-floor walk can
// reach twice. A pointer to an RM value has an identity: two attributes can
// hold the same one, and a value built in Go can hold itself. The walk keys
// what it has reached ([rmFloorWalker.walk]) by that pointer.

import "github.com/cadasto/openehr-sdk-go/openehr/rm"

// hasIdentity reports whether v is a pointer to an RM value, the only kind of
// value the walk can reach again: by two attributes that share it, or from
// inside itself. A value held by value has no identity the walk can see. The
// cases are the pointer types [rm.IsTypedNil] lists, and a test keeps the two
// lists equal; a type switch keeps the check free of reflection.
func hasIdentity(v any) bool {
	switch v.(type) {
	case *rm.AccessGroupRef, *rm.Action, *rm.Activity, *rm.Address, *rm.AdminEntry, *rm.Agent,
		*rm.Archetyped, *rm.ArchetypeID, *rm.Attestation, *rm.AuditDetails,
		*rm.BasicDefinitions, *rm.Capability, *rm.Cluster, *rm.CodePhrase, *rm.Composition,
		*rm.Contact, *rm.Contribution, *rm.DVBoolean, *rm.DVCodedText, *rm.DVCount, *rm.DVDate,
		*rm.DVDateTime, *rm.DVDuration, *rm.DVEHRURI, *rm.DVGeneralTimeSpecification,
		*rm.DVIdentifier, *rm.DVInterval[rm.DVOrdered], *rm.DVInterval[rm.DVCount],
		*rm.DVInterval[rm.DVDate], *rm.DVInterval[rm.DVDateTime],
		*rm.DVInterval[rm.DVDuration], *rm.DVInterval[rm.DVOrdinal],
		*rm.DVInterval[rm.DVProportion], *rm.DVInterval[rm.DVQuantity],
		*rm.DVInterval[rm.DVScale], *rm.DVInterval[rm.DVTime], *rm.DVMultimedia, *rm.DVOrdinal,
		*rm.DVParagraph, *rm.DVParsable, *rm.DVPeriodicTimeSpecification, *rm.DVProportion,
		*rm.DVQuantity, *rm.DVScale, *rm.DVState, *rm.DVText, *rm.DVTime, *rm.DVURI, *rm.EHR,
		*rm.EHRAccess, *rm.EHRStatus, *rm.Element, *rm.Evaluation, *rm.EventContext,
		*rm.FeederAudit, *rm.FeederAuditDetails, *rm.Folder, *rm.GenericEntry, *rm.GenericID,
		*rm.Group, *rm.HierObjectID, *rm.History[rm.ItemStructure], *rm.History[rm.ItemList],
		*rm.History[rm.ItemSingle], *rm.History[rm.ItemTable], *rm.History[rm.ItemTree],
		*rm.ImportedVersion[any], *rm.Instruction, *rm.InstructionDetails, *rm.InternetID,
		*rm.IntervalEvent[rm.ItemStructure], *rm.IntervalEvent[rm.ItemList],
		*rm.IntervalEvent[rm.ItemSingle], *rm.IntervalEvent[rm.ItemTable],
		*rm.IntervalEvent[rm.ItemTree], *rm.IsmTransition, *rm.ISOOID, *rm.ItemList,
		*rm.ItemSingle, *rm.ItemTable, *rm.ItemTag, *rm.ItemTree, *rm.ISO8601Timezone,
		*rm.Link, *rm.LocatableRef, *rm.MeasurementService, *rm.ObjectRef, *rm.ObjectVersionID,
		*rm.Observation, *rm.OpenehrCodeSetIdentifiers, *rm.OpenehrDefinitions,
		*rm.OpenehrTerminologyGroupIdentifiers, *rm.Organisation, *rm.OriginalVersion[any],
		*rm.Participation, *rm.PartyIdentified, *rm.PartyIdentity, *rm.PartyRef,
		*rm.PartyRelated, *rm.PartyRelationship, *rm.PartySelf, *rm.Person,
		*rm.PointEvent[rm.ItemStructure], *rm.PointEvent[rm.ItemList],
		*rm.PointEvent[rm.ItemSingle], *rm.PointEvent[rm.ItemTable],
		*rm.PointEvent[rm.ItemTree], *rm.PointInterval[any], *rm.ProperInterval[any],
		*rm.ReferenceRange[rm.DVOrdered], *rm.ReferenceRange[rm.DVCount],
		*rm.ReferenceRange[rm.DVDate], *rm.ReferenceRange[rm.DVDateTime],
		*rm.ReferenceRange[rm.DVDuration], *rm.ReferenceRange[rm.DVOrdinal],
		*rm.ReferenceRange[rm.DVProportion], *rm.ReferenceRange[rm.DVQuantity],
		*rm.ReferenceRange[rm.DVScale], *rm.ReferenceRange[rm.DVTime], *rm.ResourceAnnotations,
		*rm.ResourceDescription, *rm.ResourceDescriptionItem, *rm.RevisionHistory,
		*rm.RevisionHistoryItem, *rm.Role, *rm.Section, *rm.TemplateID, *rm.TerminologyID,
		*rm.TerminologyService, *rm.TermMapping, *rm.TranslationDetails, *rm.TimeDefinitions,
		*rm.Uuid, *rm.VersionedComposition, *rm.VersionedEHRAccess, *rm.VersionedEHRStatus,
		*rm.VersionedFolder, *rm.VersionedObject[any], *rm.VersionedParty, *rm.VersionTreeID:
		return true
	}
	return false
}
