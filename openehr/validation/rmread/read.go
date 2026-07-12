package rmread

import "github.com/cadasto/openehr-sdk-go/openehr/rm"

// ReadSingle returns the RM value at `attrName` on `parent`.
// The second (named-blank) parameter is the OPT-declared RM
// class name; v1 dispatch is purely on the Go concrete type of
// `parent`, but the parameter is retained so callers boxing an
// RM value through an interface (e.g. a future `any`-typed
// builder harness) can pass through the compiled RM type
// without re-flattening — the contract stays open for a future
// dispatch table that wants the string key.
//
// `ok` is false when the attribute is absent (nil pointer, nil
// interface, typed-nil pointer behind an interface — see
// [IsTypedNilPointer] — structurally-empty CodePhrase / DVText /
// DVCodedText, empty string for the locatable archetype_node_id /
// name primary channels). Pointer attrs with non-nil but zero-value
// report `ok=true` — the structural walker uses pointer presence
// as the absent/present signal.
//
// Returns `(nil, false)` for unknown (parent, attrName) pairs;
// callers SHOULD treat that as "not addressable" rather than an
// error.
func ReadSingle(parent any, _ /* parentType */, attrName string) (any, bool) {
	switch p := parent.(type) {
	case *rm.Composition:
		return readCompositionSingle(p, attrName)
	case rm.Composition:
		return readCompositionSingle(&p, attrName)

	case *rm.Observation:
		return readObservationSingle(p, attrName)
	case rm.Observation:
		return readObservationSingle(&p, attrName)

	case *rm.Evaluation:
		return readEvaluationSingle(p, attrName)
	case rm.Evaluation:
		return readEvaluationSingle(&p, attrName)

	case *rm.Instruction:
		return readInstructionSingle(p, attrName)
	case rm.Instruction:
		return readInstructionSingle(&p, attrName)

	case *rm.Action:
		return readActionSingle(p, attrName)
	case rm.Action:
		return readActionSingle(&p, attrName)

	case *rm.AdminEntry:
		return readAdminEntrySingle(p, attrName)
	case rm.AdminEntry:
		return readAdminEntrySingle(&p, attrName)

	case *rm.GenericEntry:
		return readGenericEntrySingle(p, attrName)
	case rm.GenericEntry:
		return readGenericEntrySingle(&p, attrName)

	case *rm.Section:
		return readSectionSingle(p, attrName)
	case rm.Section:
		return readSectionSingle(&p, attrName)

	case *rm.Activity:
		return readActivitySingle(p, attrName)
	case rm.Activity:
		return readActivitySingle(&p, attrName)

	case *rm.EventContext:
		return readEventContextSingle(p, attrName)
	case rm.EventContext:
		return readEventContextSingle(&p, attrName)

	case *rm.History[rm.ItemStructure]:
		return readHistorySingle(p, attrName)
	case rm.History[rm.ItemStructure]:
		return readHistorySingle(&p, attrName)

	case *rm.PointEvent[rm.ItemStructure]:
		return readPointEventSingle(p, attrName)
	case rm.PointEvent[rm.ItemStructure]:
		return readPointEventSingle(&p, attrName)

	case *rm.IntervalEvent[rm.ItemStructure]:
		return readIntervalEventSingle(p, attrName)
	case rm.IntervalEvent[rm.ItemStructure]:
		return readIntervalEventSingle(&p, attrName)

	case *rm.ItemTree:
		return readItemTreeSingle(p, attrName)
	case rm.ItemTree:
		return readItemTreeSingle(&p, attrName)

	case *rm.ItemList:
		return readItemListSingle(p, attrName)
	case rm.ItemList:
		return readItemListSingle(&p, attrName)

	case *rm.ItemSingle:
		return readItemSingleSingle(p, attrName)
	case rm.ItemSingle:
		return readItemSingleSingle(&p, attrName)

	case *rm.ItemTable:
		return readItemTableSingle(p, attrName)
	case rm.ItemTable:
		return readItemTableSingle(&p, attrName)

	case *rm.Cluster:
		return readClusterSingle(p, attrName)
	case rm.Cluster:
		return readClusterSingle(&p, attrName)

	case *rm.Element:
		return readElementSingle(p, attrName)
	case rm.Element:
		return readElementSingle(&p, attrName)

	case *rm.DVText:
		return readDVTextSingle(p, attrName)
	case rm.DVText:
		return readDVTextSingle(&p, attrName)

	case *rm.DVCodedText:
		return readDVCodedTextSingle(p, attrName)
	case rm.DVCodedText:
		return readDVCodedTextSingle(&p, attrName)

	case *rm.CodePhrase:
		return readCodePhraseSingle(p, attrName)
	case rm.CodePhrase:
		return readCodePhraseSingle(&p, attrName)

	// --- primitive-bearing DataValue leaves (explicit `value` C_PRIMITIVE child) ---
	case *rm.DVDate:
		return readDVDateSingle(p, attrName)
	case rm.DVDate:
		return readDVDateSingle(&p, attrName)
	case *rm.DVTime:
		return readDVTimeSingle(p, attrName)
	case rm.DVTime:
		return readDVTimeSingle(&p, attrName)
	case *rm.DVDateTime:
		return readDVDateTimeSingle(p, attrName)
	case rm.DVDateTime:
		return readDVDateTimeSingle(&p, attrName)
	case *rm.DVDuration:
		return readDVDurationSingle(p, attrName)
	case rm.DVDuration:
		return readDVDurationSingle(&p, attrName)
	case *rm.DVBoolean:
		return readDVBooleanSingle(p, attrName)
	case rm.DVBoolean:
		return readDVBooleanSingle(&p, attrName)
	case *rm.DVIdentifier:
		return readDVIdentifierSingle(p, attrName)
	case rm.DVIdentifier:
		return readDVIdentifierSingle(&p, attrName)
	case *rm.DVMultimedia:
		return readDVMultimediaSingle(p, attrName)
	case rm.DVMultimedia:
		return readDVMultimediaSingle(&p, attrName)

	case *rm.DVCount:
		return readDVCountSingle(p, attrName)
	case rm.DVCount:
		return readDVCountSingle(&p, attrName)

	case *rm.DVQuantity:
		return readDVQuantitySingle(p, attrName)
	case rm.DVQuantity:
		return readDVQuantitySingle(&p, attrName)

	case *rm.DVProportion:
		return readDVProportionSingle(p, attrName)
	case rm.DVProportion:
		return readDVProportionSingle(&p, attrName)

	case *rm.DVURI:
		return readDVURISingle(p, attrName)
	case rm.DVURI:
		return readDVURISingle(&p, attrName)

	case *rm.DVEHRURI:
		return readDVEHRURISingle(p, attrName)
	case rm.DVEHRURI:
		return readDVEHRURISingle(&p, attrName)

	case *rm.DVParsable:
		return readDVParsableSingle(p, attrName)
	case rm.DVParsable:
		return readDVParsableSingle(&p, attrName)

	case *rm.DVInterval[rm.DVQuantity]:
		return readDVIntervalQuantitySingle(p, attrName)
	case rm.DVInterval[rm.DVQuantity]:
		return readDVIntervalQuantitySingle(&p, attrName)

	case *rm.DVInterval[rm.DVCount]:
		return readDVIntervalCountSingle(p, attrName)
	case rm.DVInterval[rm.DVCount]:
		return readDVIntervalCountSingle(&p, attrName)

	case *rm.DVInterval[rm.DVDateTime]:
		return readDVIntervalDateTimeSingle(p, attrName)
	case rm.DVInterval[rm.DVDateTime]:
		return readDVIntervalDateTimeSingle(&p, attrName)

	case *rm.DVInterval[rm.DVDate]:
		return readDVIntervalDateSingle(p, attrName)
	case rm.DVInterval[rm.DVDate]:
		return readDVIntervalDateSingle(&p, attrName)

	case *rm.DVInterval[rm.DVTime]:
		return readDVIntervalTimeSingle(p, attrName)
	case rm.DVInterval[rm.DVTime]:
		return readDVIntervalTimeSingle(&p, attrName)

	case *rm.DVInterval[rm.DVProportion]:
		return readDVIntervalProportionSingle(p, attrName)
	case rm.DVInterval[rm.DVProportion]:
		return readDVIntervalProportionSingle(&p, attrName)

	case *rm.DVInterval[rm.DVOrdered]:
		return readDVIntervalOrderedSingle(p, attrName)
	case rm.DVInterval[rm.DVOrdered]:
		return readDVIntervalOrderedSingle(&p, attrName)

	// --- demographic: PARTY hierarchy + archetypeable sub-components ---
	case *rm.Person:
		return readPersonSingle(p, attrName)
	case rm.Person:
		return readPersonSingle(&p, attrName)

	case *rm.Organisation:
		return readOrganisationSingle(p, attrName)
	case rm.Organisation:
		return readOrganisationSingle(&p, attrName)

	case *rm.Group:
		return readGroupSingle(p, attrName)
	case rm.Group:
		return readGroupSingle(&p, attrName)

	case *rm.Agent:
		return readAgentSingle(p, attrName)
	case rm.Agent:
		return readAgentSingle(&p, attrName)

	case *rm.Role:
		return readRoleSingle(p, attrName)
	case rm.Role:
		return readRoleSingle(&p, attrName)

	case *rm.Address:
		return readAddressSingle(p, attrName)
	case rm.Address:
		return readAddressSingle(&p, attrName)

	case *rm.Contact:
		return readContactSingle(p, attrName)
	case rm.Contact:
		return readContactSingle(&p, attrName)

	case *rm.PartyIdentity:
		return readPartyIdentitySingle(p, attrName)
	case rm.PartyIdentity:
		return readPartyIdentitySingle(&p, attrName)

	case *rm.PartyRelationship:
		return readPartyRelationshipSingle(p, attrName)
	case rm.PartyRelationship:
		return readPartyRelationshipSingle(&p, attrName)

	case *rm.Capability:
		return readCapabilitySingle(p, attrName)
	case rm.Capability:
		return readCapabilitySingle(&p, attrName)

	// --- EHR-IM roots ---
	case *rm.Folder:
		return readFolderSingle(p, attrName)
	case rm.Folder:
		return readFolderSingle(&p, attrName)

	case *rm.EHRStatus:
		return readEHRStatusSingle(p, attrName)
	case rm.EHRStatus:
		return readEHRStatusSingle(&p, attrName)
	}
	return nil, false
}

// Handles reports whether rmread models parent's RM type for attribute
// reading — i.e. whether [ReadSingle] / [ReadMultiple] dispatch to a typed
// reader rather than falling through to (nil, false). A BMM-driven walker
// (e.g. the REQ-112 RM-floor validator) uses this to avoid descending into
// or required-checking the attributes of a type rmread does not model
// (OBJECT_REF, PARTICIPATION, LINK, …): those are opaque leaves here and
// must be validated by their own evaluators, not by reading their members
// (which would all read back as absent and fabricate `required`).
//
// MUST track the type set of ReadSingle/ReadMultiple above. A type added
// there but omitted here is treated as a leaf — its RM-mandatory attributes
// go unchecked (a missed check, never a false positive), so erring toward
// omission is the safe failure mode.
func Handles(parent any) bool {
	switch parent.(type) {
	case *rm.Composition, rm.Composition,
		*rm.Observation, rm.Observation,
		*rm.Evaluation, rm.Evaluation,
		*rm.Instruction, rm.Instruction,
		*rm.Action, rm.Action,
		*rm.AdminEntry, rm.AdminEntry,
		*rm.GenericEntry, rm.GenericEntry,
		*rm.Section, rm.Section,
		*rm.Activity, rm.Activity,
		*rm.EventContext, rm.EventContext,
		*rm.History[rm.ItemStructure], rm.History[rm.ItemStructure],
		*rm.PointEvent[rm.ItemStructure], rm.PointEvent[rm.ItemStructure],
		*rm.IntervalEvent[rm.ItemStructure], rm.IntervalEvent[rm.ItemStructure],
		*rm.ItemTree, rm.ItemTree,
		*rm.ItemList, rm.ItemList,
		*rm.ItemSingle, rm.ItemSingle,
		*rm.ItemTable, rm.ItemTable,
		*rm.Cluster, rm.Cluster,
		*rm.Element, rm.Element,
		*rm.DVText, rm.DVText,
		*rm.DVCodedText, rm.DVCodedText,
		*rm.CodePhrase, rm.CodePhrase,
		*rm.DVDate, rm.DVDate,
		*rm.DVTime, rm.DVTime,
		*rm.DVDateTime, rm.DVDateTime,
		*rm.DVDuration, rm.DVDuration,
		*rm.DVBoolean, rm.DVBoolean,
		*rm.DVIdentifier, rm.DVIdentifier,
		*rm.DVMultimedia, rm.DVMultimedia,
		*rm.DVCount, rm.DVCount,
		*rm.DVQuantity, rm.DVQuantity,
		*rm.DVProportion, rm.DVProportion,
		*rm.DVURI, rm.DVURI,
		*rm.DVEHRURI, rm.DVEHRURI,
		*rm.DVParsable, rm.DVParsable,
		*rm.DVInterval[rm.DVQuantity], rm.DVInterval[rm.DVQuantity],
		*rm.DVInterval[rm.DVCount], rm.DVInterval[rm.DVCount],
		*rm.DVInterval[rm.DVDateTime], rm.DVInterval[rm.DVDateTime],
		*rm.DVInterval[rm.DVDate], rm.DVInterval[rm.DVDate],
		*rm.DVInterval[rm.DVTime], rm.DVInterval[rm.DVTime],
		*rm.DVInterval[rm.DVProportion], rm.DVInterval[rm.DVProportion],
		*rm.DVInterval[rm.DVOrdered], rm.DVInterval[rm.DVOrdered],
		*rm.Person, rm.Person,
		*rm.Organisation, rm.Organisation,
		*rm.Group, rm.Group,
		*rm.Agent, rm.Agent,
		*rm.Role, rm.Role,
		*rm.Address, rm.Address,
		*rm.Contact, rm.Contact,
		*rm.PartyIdentity, rm.PartyIdentity,
		*rm.PartyRelationship, rm.PartyRelationship,
		*rm.Capability, rm.Capability,
		*rm.Folder, rm.Folder,
		*rm.EHRStatus, rm.EHRStatus:
		return true
	}
	return false
}

// ReadMultiple returns the slice at `attrName` on `parent`, each
// element boxed as `any`. `ok` is true when the parent type
// carries the attribute (the returned slice may still be empty);
// it is false for unknown (parentType, attrName) pairs. Callers
// distinguish "absent" from "empty" via `len(items) == 0` — the
// cardinality check at the call site needs both signals.
func ReadMultiple(parent any, _ /* parentType */, attrName string) ([]any, bool) {
	switch p := parent.(type) {
	case *rm.Composition:
		return readCompositionMultiple(p, attrName)
	case rm.Composition:
		return readCompositionMultiple(&p, attrName)

	case *rm.Section:
		return readSectionMultiple(p, attrName)
	case rm.Section:
		return readSectionMultiple(&p, attrName)

	case *rm.Instruction:
		return readInstructionMultiple(p, attrName)
	case rm.Instruction:
		return readInstructionMultiple(&p, attrName)

	case *rm.History[rm.ItemStructure]:
		return readHistoryMultiple(p, attrName)
	case rm.History[rm.ItemStructure]:
		return readHistoryMultiple(&p, attrName)

	case *rm.ItemTree:
		return readItemTreeMultiple(p, attrName)
	case rm.ItemTree:
		return readItemTreeMultiple(&p, attrName)

	case *rm.ItemList:
		return readItemListMultiple(p, attrName)
	case rm.ItemList:
		return readItemListMultiple(&p, attrName)

	case *rm.ItemTable:
		return readItemTableMultiple(p, attrName)
	case rm.ItemTable:
		return readItemTableMultiple(&p, attrName)

	case *rm.Cluster:
		return readClusterMultiple(p, attrName)
	case rm.Cluster:
		return readClusterMultiple(&p, attrName)

	// --- demographic: PARTY hierarchy + sub-components ---
	case *rm.Person:
		return readPersonMultiple(p, attrName)
	case rm.Person:
		return readPersonMultiple(&p, attrName)

	case *rm.Organisation:
		return readOrganisationMultiple(p, attrName)
	case rm.Organisation:
		return readOrganisationMultiple(&p, attrName)

	case *rm.Group:
		return readGroupMultiple(p, attrName)
	case rm.Group:
		return readGroupMultiple(&p, attrName)

	case *rm.Agent:
		return readAgentMultiple(p, attrName)
	case rm.Agent:
		return readAgentMultiple(&p, attrName)

	case *rm.Role:
		return readRoleMultiple(p, attrName)
	case rm.Role:
		return readRoleMultiple(&p, attrName)

	case *rm.Contact:
		return readContactMultiple(p, attrName)
	case rm.Contact:
		return readContactMultiple(&p, attrName)

	// --- EHR-IM roots ---
	case *rm.Folder:
		return readFolderMultiple(p, attrName)
	case rm.Folder:
		return readFolderMultiple(&p, attrName)
	}
	return nil, false
}

// --- COMPOSITION ----------------------------------------------------------

func readCompositionSingle(c *rm.Composition, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(c.ArchetypeNodeID)
	case "name":
		return dvTextPresent(c.Name)
	case "category":
		return dvCodedTextPresent(c.Category)
	case "composer":
		return ifacePresent(c.Composer)
	case "language":
		return codePhrasePresent(c.Language)
	case "territory":
		return codePhrasePresent(c.Territory)
	case "context":
		return ptrPresent(c.Context)
	}
	return nil, false
}

func readCompositionMultiple(c *rm.Composition, attr string) ([]any, bool) {
	switch attr {
	case "content":
		out := make([]any, 0, len(c.Content))
		for _, it := range c.Content {
			out = append(out, it)
		}
		return out, true
	}
	return nil, false
}

// --- OBSERVATION ----------------------------------------------------------

func readObservationSingle(o *rm.Observation, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(o.ArchetypeNodeID)
	case "name":
		return dvTextPresent(o.Name)
	case "language":
		return codePhrasePresent(o.Language)
	case "encoding":
		return codePhrasePresent(o.Encoding)
	case "subject":
		return ifacePresent(o.Subject)
	case "provider":
		return ifacePresent(o.Provider)
	case "data":
		// HISTORY value-typed; absent iff zero-valued (no events,
		// no origin). The structural walker descends regardless;
		// `ok=false` only when the underlying struct is wholly
		// zero — match by checking the Origin's underlying string
		// + len(events) as a cheap heuristic.
		if len(o.Data.Events) == 0 && o.Data.Origin.Value == "" && o.Data.ArchetypeNodeID == "" {
			return o.Data, false
		}
		return o.Data, true
	case "state":
		return ptrPresent(o.State)
	case "protocol":
		return ifacePresent(o.Protocol)
	}
	return nil, false
}

// --- EVALUATION -----------------------------------------------------------

func readEvaluationSingle(e *rm.Evaluation, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(e.ArchetypeNodeID)
	case "name":
		return dvTextPresent(e.Name)
	case "language":
		return codePhrasePresent(e.Language)
	case "encoding":
		return codePhrasePresent(e.Encoding)
	case "subject":
		return ifacePresent(e.Subject)
	case "provider":
		return ifacePresent(e.Provider)
	case "data":
		return ifacePresent(e.Data)
	case "protocol":
		return ifacePresent(e.Protocol)
	}
	return nil, false
}

// --- INSTRUCTION ----------------------------------------------------------

func readInstructionSingle(i *rm.Instruction, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(i.ArchetypeNodeID)
	case "name":
		return dvTextPresent(i.Name)
	case "language":
		return codePhrasePresent(i.Language)
	case "encoding":
		return codePhrasePresent(i.Encoding)
	case "narrative":
		return dvTextPresent(i.Narrative)
	case "protocol":
		return ifacePresent(i.Protocol)
	case "expiry_time":
		return ptrPresent(i.ExpiryTime)
	case "wf_definition":
		return ptrPresent(i.WfDefinition)
	case "subject":
		return ifacePresent(i.Subject)
	case "provider":
		return ifacePresent(i.Provider)
	}
	return nil, false
}

func readInstructionMultiple(i *rm.Instruction, attr string) ([]any, bool) {
	switch attr {
	case "activities":
		out := make([]any, 0, len(i.Activities))
		for k := range i.Activities {
			out = append(out, &i.Activities[k])
		}
		return out, true
	}
	return nil, false
}

// --- ACTION ---------------------------------------------------------------

func readActionSingle(a *rm.Action, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(a.ArchetypeNodeID)
	case "name":
		return dvTextPresent(a.Name)
	case "language":
		return codePhrasePresent(a.Language)
	case "encoding":
		return codePhrasePresent(a.Encoding)
	case "description":
		return ifacePresent(a.Description)
	case "protocol":
		return ifacePresent(a.Protocol)
	case "subject":
		return ifacePresent(a.Subject)
	case "provider":
		return ifacePresent(a.Provider)
	}
	return nil, false
}

// --- ADMIN_ENTRY ----------------------------------------------------------

func readAdminEntrySingle(a *rm.AdminEntry, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(a.ArchetypeNodeID)
	case "name":
		return dvTextPresent(a.Name)
	case "language":
		return codePhrasePresent(a.Language)
	case "encoding":
		return codePhrasePresent(a.Encoding)
	case "data":
		return ifacePresent(a.Data)
	case "subject":
		return ifacePresent(a.Subject)
	case "provider":
		return ifacePresent(a.Provider)
	}
	return nil, false
}

// --- GENERIC_ENTRY --------------------------------------------------------

func readGenericEntrySingle(g *rm.GenericEntry, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(g.ArchetypeNodeID)
	case "name":
		return dvTextPresent(g.Name)
	case "data":
		return ifacePresent(g.Data)
	}
	return nil, false
}

// --- SECTION --------------------------------------------------------------

func readSectionSingle(s *rm.Section, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(s.ArchetypeNodeID)
	case "name":
		return dvTextPresent(s.Name)
	}
	return nil, false
}

func readSectionMultiple(s *rm.Section, attr string) ([]any, bool) {
	switch attr {
	case "items":
		out := make([]any, 0, len(s.Items))
		for _, it := range s.Items {
			out = append(out, it)
		}
		return out, true
	}
	return nil, false
}

// --- ACTIVITY -------------------------------------------------------------

func readActivitySingle(a *rm.Activity, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(a.ArchetypeNodeID)
	case "name":
		return dvTextPresent(a.Name)
	case "description":
		return ifacePresent(a.Description)
	case "timing":
		return ptrPresent(a.Timing)
	case "action_archetype_id":
		return strPresent(a.ActionArchetypeID)
	}
	return nil, false
}

// --- EVENT_CONTEXT --------------------------------------------------------

func readEventContextSingle(c *rm.EventContext, attr string) (any, bool) {
	switch attr {
	case "start_time":
		// DV_DATE_TIME — BMM-mandatory; present iff non-empty value.
		if c.StartTime.Value == "" {
			return c.StartTime, false
		}
		return c.StartTime, true
	case "setting":
		// DV_CODED_TEXT — BMM-mandatory; present iff defining_code
		// carries a code_string OR the surface value is non-empty.
		if c.Setting.DefiningCode.CodeString == "" && c.Setting.Value == "" {
			return c.Setting, false
		}
		return c.Setting, true
	case "end_time":
		return ptrPresent(c.EndTime)
	case "location":
		if c.Location == nil || *c.Location == "" {
			return c.Location, false
		}
		return c.Location, true
	case "health_care_facility":
		// REQ-052: rm.PartyIdentifiedLike is an interface — nilable
		// on its own; ifacePresent is the right predicate.
		return ifacePresent(c.HealthCareFacility)
	case "other_context":
		return ifacePresent(c.OtherContext)
	}
	return nil, false
}

// --- HISTORY --------------------------------------------------------------

func readHistorySingle(h *rm.History[rm.ItemStructure], attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(h.ArchetypeNodeID)
	case "name":
		return dvTextPresent(h.Name)
	case "origin":
		// DV_DATE_TIME — present iff non-empty value.
		if h.Origin.Value == "" {
			return h.Origin, false
		}
		return h.Origin, true
	case "period":
		return ptrPresent(h.Period)
	case "duration":
		return ptrPresent(h.Duration)
	case "summary":
		return ifacePresent(h.Summary)
	}
	return nil, false
}

func readHistoryMultiple(h *rm.History[rm.ItemStructure], attr string) ([]any, bool) {
	switch attr {
	case "events":
		out := make([]any, 0, len(h.Events))
		for _, e := range h.Events {
			out = append(out, e)
		}
		return out, true
	}
	return nil, false
}

// --- POINT_EVENT / INTERVAL_EVENT ----------------------------------------

func readPointEventSingle(e *rm.PointEvent[rm.ItemStructure], attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(e.ArchetypeNodeID)
	case "name":
		return dvTextPresent(e.Name)
	case "time":
		if e.Time.Value == "" {
			return e.Time, false
		}
		return e.Time, true
	case "data":
		return ifacePresent(e.Data)
	case "state":
		return ifacePresent(e.State)
	}
	return nil, false
}

func readIntervalEventSingle(e *rm.IntervalEvent[rm.ItemStructure], attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(e.ArchetypeNodeID)
	case "name":
		return dvTextPresent(e.Name)
	case "time":
		if e.Time.Value == "" {
			return e.Time, false
		}
		return e.Time, true
	case "width":
		if e.Width.Value == "" {
			return e.Width, false
		}
		return e.Width, true
	case "math_function":
		return dvCodedTextPresent(e.MathFunction)
	case "sample_count":
		return ptrPresent(e.SampleCount)
	case "data":
		return ifacePresent(e.Data)
	case "state":
		return ifacePresent(e.State)
	}
	return nil, false
}

// --- ITEM_STRUCTURE variants ---------------------------------------------

func readItemTreeSingle(t *rm.ItemTree, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(t.ArchetypeNodeID)
	case "name":
		return dvTextPresent(t.Name)
	}
	return nil, false
}

func readItemTreeMultiple(t *rm.ItemTree, attr string) ([]any, bool) {
	switch attr {
	case "items":
		out := make([]any, 0, len(t.Items))
		for _, it := range t.Items {
			out = append(out, it)
		}
		return out, true
	}
	return nil, false
}

func readItemListSingle(l *rm.ItemList, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(l.ArchetypeNodeID)
	case "name":
		return dvTextPresent(l.Name)
	}
	return nil, false
}

func readItemListMultiple(l *rm.ItemList, attr string) ([]any, bool) {
	switch attr {
	case "items":
		out := make([]any, 0, len(l.Items))
		for k := range l.Items {
			out = append(out, &l.Items[k])
		}
		return out, true
	}
	return nil, false
}

func readItemSingleSingle(s *rm.ItemSingle, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(s.ArchetypeNodeID)
	case "name":
		return dvTextPresent(s.Name)
	case "item":
		// ITEM_SINGLE.item is a value-typed Element. Present iff
		// the element carries non-empty archetype_node_id OR a
		// concrete (non-typed-nil) value. A typed-nil DataValue
		// (e.g. `(*rm.DVQuantity)(nil)` stored in
		// `Element.Value`) is treated as absent — without the
		// IsTypedNilPointer check the != nil arm would
		// false-positive on an interface that carries a type but
		// no value.
		if s.Item.ArchetypeNodeID == "" &&
			(s.Item.Value == nil || IsTypedNilPointer(s.Item.Value)) {
			return &s.Item, false
		}
		return &s.Item, true
	}
	return nil, false
}

func readItemTableSingle(t *rm.ItemTable, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(t.ArchetypeNodeID)
	case "name":
		return dvTextPresent(t.Name)
	}
	return nil, false
}

func readItemTableMultiple(t *rm.ItemTable, attr string) ([]any, bool) {
	switch attr {
	case "rows":
		out := make([]any, 0, len(t.Rows))
		for k := range t.Rows {
			out = append(out, &t.Rows[k])
		}
		return out, true
	}
	return nil, false
}

// --- CLUSTER / ELEMENT ---------------------------------------------------

func readClusterSingle(c *rm.Cluster, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(c.ArchetypeNodeID)
	case "name":
		return dvTextPresent(c.Name)
	}
	return nil, false
}

func readClusterMultiple(c *rm.Cluster, attr string) ([]any, bool) {
	switch attr {
	case "items":
		out := make([]any, 0, len(c.Items))
		for _, it := range c.Items {
			out = append(out, it)
		}
		return out, true
	}
	return nil, false
}

func readElementSingle(e *rm.Element, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(e.ArchetypeNodeID)
	case "name":
		return dvTextPresent(e.Name)
	case "value":
		return ifacePresent(e.Value)
	case "null_flavour":
		return ptrPresent(e.NullFlavour)
	case "null_reason":
		// REQ-052: rm.DVTextLike is an interface — nilable on its
		// own; ifacePresent is the right predicate.
		return ifacePresent(e.NullReason)
	}
	return nil, false
}

// --- DataValue navigations -----------------------------------------------

func readDVTextSingle(t *rm.DVText, attr string) (any, bool) {
	switch attr {
	case "value":
		return strPresent(t.Value)
	}
	return nil, false
}

func readDVCodedTextSingle(t *rm.DVCodedText, attr string) (any, bool) {
	switch attr {
	case "value":
		return strPresent(t.Value)
	case "defining_code":
		return codePhrasePresent(t.DefiningCode)
	}
	return nil, false
}

// Primitive-bearing DataValue leaves. When an OPT encodes a DV value as
// a C_COMPLEX_OBJECT (DV_DATE / DV_BOOLEAN / …) carrying an explicit
// `value` C_PRIMITIVE_OBJECT child — rather than a folded REQ-103
// primitive leaf — the walker descends into the value attribute and
// needs to bind the primitive. Without these readers a populated value
// reports absent, a false `required`. The bound primitive is then
// validated by the C_PRIMITIVE child (REQ-103).

func readDVDateSingle(d *rm.DVDate, attr string) (any, bool) {
	if attr == "value" {
		return strPresent(d.Value)
	}
	return nil, false
}

func readDVTimeSingle(t *rm.DVTime, attr string) (any, bool) {
	if attr == "value" {
		return strPresent(t.Value)
	}
	return nil, false
}

func readDVDateTimeSingle(d *rm.DVDateTime, attr string) (any, bool) {
	if attr == "value" {
		return strPresent(d.Value)
	}
	return nil, false
}

func readDVDurationSingle(d *rm.DVDuration, attr string) (any, bool) {
	if attr == "value" {
		return strPresent(d.Value)
	}
	return nil, false
}

func readDVBooleanSingle(b *rm.DVBoolean, attr string) (any, bool) {
	if attr == "value" {
		// Boolean value-typed field — always structurally present.
		return b.Value, true
	}
	return nil, false
}

func readDVIdentifierSingle(i *rm.DVIdentifier, attr string) (any, bool) {
	switch attr {
	case "id":
		return strPresent(i.ID)
	case "issuer":
		return ptrPresent(i.Issuer)
	case "assigner":
		return ptrPresent(i.Assigner)
	case "type":
		return ptrPresent(i.Type)
	}
	return nil, false
}

func readDVMultimediaSingle(m *rm.DVMultimedia, attr string) (any, bool) {
	switch attr {
	case "media_type":
		return codePhrasePresent(m.MediaType)
	case "size":
		// Integer value-typed field — always structurally present.
		return m.Size, true
	}
	return nil, false
}

func readCodePhraseSingle(c *rm.CodePhrase, attr string) (any, bool) {
	switch attr {
	case "code_string":
		return strPresent(c.CodeString)
	case "terminology_id":
		return strPresent(c.TerminologyID.Value)
	case "preferred_term":
		if c.PreferredTerm == nil || *c.PreferredTerm == "" {
			return c.PreferredTerm, false
		}
		return c.PreferredTerm, true
	}
	return nil, false
}

// --- demographic: PARTY hierarchy + sub-components -----------------------
//
// The five PARTY concretes (PERSON, ORGANISATION, GROUP, AGENT and the
// ACTOR-less ROLE) share the LOCATABLE channels (archetype_node_id /
// name) plus the PARTY `details` ITEM_STRUCTURE; the four ACTOR
// subtypes additionally share identities / contacts / relationships /
// languages / roles. The shared shapes route through actorLike* /
// partyLike* helpers so the per-type readers stay one line each.

// readActorLikeSingle serves the single-valued LOCATABLE + PARTY
// channels common to PERSON / ORGANISATION / GROUP / AGENT / ROLE.
func readActorLikeSingle(archetypeNodeID string, name rm.DVTextLike, details rm.ItemStructure, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(archetypeNodeID)
	case "name":
		return dvTextPresent(name)
	case "details":
		return ifacePresent(details)
	}
	return nil, false
}

// readActorMultiple serves the multi-valued PARTY/ACTOR channels shared
// by the four ACTOR subtypes (PERSON / ORGANISATION / GROUP / AGENT).
func readActorMultiple(
	identities []rm.PartyIdentity,
	contacts []rm.Contact,
	relationships []rm.PartyRelationship,
	languages []rm.DVTextLike,
	roles []rm.PartyRef,
	attr string,
) ([]any, bool) {
	switch attr {
	case "identities":
		return boxPtrs(identities), true
	case "contacts":
		return boxPtrs(contacts), true
	case "relationships":
		return boxPtrs(relationships), true
	case "languages":
		return boxIfaces(languages), true
	case "roles":
		return boxPtrs(roles), true
	}
	return nil, false
}

func readPersonSingle(p *rm.Person, attr string) (any, bool) {
	return readActorLikeSingle(p.ArchetypeNodeID, p.Name, p.Details, attr)
}

func readPersonMultiple(p *rm.Person, attr string) ([]any, bool) {
	return readActorMultiple(p.Identities, p.Contacts, p.Relationships, p.Languages, p.Roles, attr)
}

func readOrganisationSingle(o *rm.Organisation, attr string) (any, bool) {
	return readActorLikeSingle(o.ArchetypeNodeID, o.Name, o.Details, attr)
}

func readOrganisationMultiple(o *rm.Organisation, attr string) ([]any, bool) {
	return readActorMultiple(o.Identities, o.Contacts, o.Relationships, o.Languages, o.Roles, attr)
}

func readGroupSingle(g *rm.Group, attr string) (any, bool) {
	return readActorLikeSingle(g.ArchetypeNodeID, g.Name, g.Details, attr)
}

func readGroupMultiple(g *rm.Group, attr string) ([]any, bool) {
	return readActorMultiple(g.Identities, g.Contacts, g.Relationships, g.Languages, g.Roles, attr)
}

func readAgentSingle(a *rm.Agent, attr string) (any, bool) {
	return readActorLikeSingle(a.ArchetypeNodeID, a.Name, a.Details, attr)
}

func readAgentMultiple(a *rm.Agent, attr string) ([]any, bool) {
	return readActorMultiple(a.Identities, a.Contacts, a.Relationships, a.Languages, a.Roles, attr)
}

// ROLE is a PARTY but not an ACTOR — it carries capabilities and a
// performer reference rather than identities-as-ACTOR; it still has
// identities / contacts / relationships.
func readRoleSingle(r *rm.Role, attr string) (any, bool) {
	return readActorLikeSingle(r.ArchetypeNodeID, r.Name, r.Details, attr)
}

func readRoleMultiple(r *rm.Role, attr string) ([]any, bool) {
	switch attr {
	case "capabilities":
		return boxPtrs(r.Capabilities), true
	case "contacts":
		return boxPtrs(r.Contacts), true
	case "identities":
		return boxPtrs(r.Identities), true
	case "relationships":
		return boxPtrs(r.Relationships), true
	}
	return nil, false
}

// ADDRESS / PARTY_IDENTITY / PARTY_RELATIONSHIP are archetypeable
// LOCATABLEs whose only descendable channel is `details`
// (ITEM_STRUCTURE). source/target on PARTY_RELATIONSHIP are PARTY_REF
// references, not archetypeable structure — not surfaced.
func readAddressSingle(a *rm.Address, attr string) (any, bool) {
	return readActorLikeSingle(a.ArchetypeNodeID, a.Name, a.Details, attr)
}

func readPartyIdentitySingle(p *rm.PartyIdentity, attr string) (any, bool) {
	return readActorLikeSingle(p.ArchetypeNodeID, p.Name, p.Details, attr)
}

func readPartyRelationshipSingle(p *rm.PartyRelationship, attr string) (any, bool) {
	return readActorLikeSingle(p.ArchetypeNodeID, p.Name, p.Details, attr)
}

// CONTACT holds a set of ADDRESS alternatives; its archetypeable
// structure is the addresses list (no `details`).
func readContactSingle(c *rm.Contact, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(c.ArchetypeNodeID)
	case "name":
		return dvTextPresent(c.Name)
	}
	return nil, false
}

func readContactMultiple(c *rm.Contact, attr string) ([]any, bool) {
	switch attr {
	case "addresses":
		return boxPtrs(c.Addresses), true
	}
	return nil, false
}

// CAPABILITY (under ROLE) carries `credentials` (ITEM_STRUCTURE).
func readCapabilitySingle(c *rm.Capability, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(c.ArchetypeNodeID)
	case "name":
		return dvTextPresent(c.Name)
	case "credentials":
		return ifacePresent(c.Credentials)
	}
	return nil, false
}

// --- EHR-IM roots: FOLDER, EHR_STATUS ------------------------------------

func readFolderSingle(f *rm.Folder, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(f.ArchetypeNodeID)
	case "name":
		return dvTextPresent(f.Name)
	case "details":
		return ifacePresent(f.Details)
	}
	return nil, false
}

func readFolderMultiple(f *rm.Folder, attr string) ([]any, bool) {
	switch attr {
	case "folders":
		return boxPtrs(f.Folders), true
	case "items":
		// OBJECT_REF references, not archetypeable structure; surfaced
		// so an OPT pinning existence/cardinality on `items` can be
		// satisfied (the walker does not descend reference targets).
		return boxIfaces(f.Items), true
	}
	return nil, false
}

func readEHRStatusSingle(s *rm.EHRStatus, attr string) (any, bool) {
	switch attr {
	case "archetype_node_id":
		return strPresent(s.ArchetypeNodeID)
	case "name":
		return dvTextPresent(s.Name)
	case "subject":
		// PARTY_SELF — value-typed, always structurally present.
		return s.Subject, true
	case "other_details":
		return ifacePresent(s.OtherDetails)
	case "is_modifiable":
		// BMM-mandatory Boolean — a value-typed bool is always present.
		return s.IsModifiable, true
	case "is_queryable":
		return s.IsQueryable, true
	}
	return nil, false
}

// --- slice boxing helpers ------------------------------------------------

// boxPtrs boxes each element of a value-typed RM slice as a pointer
// (`*T`) into the backing array, mirroring readInstructionMultiple's
// `&i.Activities[k]`. Pointer boxing lets the parent validator's
// rmTypeInfo switch (which enumerates `*rm.T`) recognise the element
// and lets the walker descend into the live struct. REQ-024 — generic,
// no reflection.
func boxPtrs[T any](xs []T) []any {
	out := make([]any, 0, len(xs))
	for k := range xs {
		out = append(out, &xs[k])
	}
	return out
}

// boxIfaces boxes each element of an interface-typed RM slice (e.g.
// []DVTextLike, []ObjectRefLike) as-is — the element already carries a
// concrete behind the interface, mirroring readClusterMultiple's
// `append(out, it)`.
func boxIfaces[T any](xs []T) []any {
	out := make([]any, 0, len(xs))
	for _, x := range xs {
		out = append(out, x)
	}
	return out
}

// --- presence helpers ----------------------------------------------------

func strPresent(s string) (any, bool) {
	if s == "" {
		return s, false
	}
	return s, true
}

func dvTextPresent(v rm.DVTextLike) (any, bool) {
	// REQ-052: name / narrative slots are typed as rm.DVTextLike
	// (DVText OR DVCodedText subtype). Unwrap to the parent DVText
	// payload — absence means nil interface or empty .Value.
	if v == nil {
		return nil, false
	}
	t, ok := rm.AsDVText(v)
	if !ok || t.Value == "" {
		return v, false
	}
	return v, true
}

func dvCodedTextPresent(t rm.DVCodedText) (any, bool) {
	zeroCode := t.DefiningCode.CodeString == "" && t.DefiningCode.TerminologyID.Value == ""
	if t.Value == "" && zeroCode {
		return t, false
	}
	return t, true
}

func codePhrasePresent(cp rm.CodePhrase) (any, bool) {
	if cp.CodeString == "" && cp.TerminologyID.Value == "" {
		return cp, false
	}
	return cp, true
}

func ptrPresent[T any](p *T) (any, bool) {
	if p == nil {
		return p, false
	}
	return p, true
}

// ifacePresent reports whether an interface-typed RM attribute
// carries a non-nil concrete value. Catches BOTH the bare nil case
// (`var p DataValue; ifacePresent(p)` — interface itself is nil)
// AND the typed-nil case (`Element.Value = (*rm.DVQuantity)(nil)` —
// the interface is non-nil because it carries a type, but the
// underlying pointer is). Without the typed-nil check the walker
// would treat the slot as present and downstream dispatchers
// (rmread.ReadSingle on pointer cases, dataValueInput) would
// dereference and panic.
//
// REQ-024 compliant: the typed-nil test delegates to [IsTypedNilPointer],
// a closed type switch over the RM pointer concretes that can appear
// behind a Go interface in the v2 content-type closed set. No reflection.
func ifacePresent(v any) (any, bool) {
	if v == nil {
		return v, false
	}
	if IsTypedNilPointer(v) {
		return v, false
	}
	return v, true
}

// IsTypedNilPointer reports whether v is an interface value carrying
// a typed-nil pointer (e.g. Element.Value = (*rm.DVQuantity)(nil)).
// Bare nil interfaces and value-typed structs return false.
//
// Delegates to the generated rm.IsTypedNil (ADR 0013), which covers
// every registered RM concrete. The previous hand-written switch was
// deliberately narrow (only types pointer-stored behind an RM
// interface); the generated predicate is a strict superset — the
// additional types either never occur as typed-nils behind the
// interfaces this package descends, or, where they can (e.g. a
// typed-nil root handed to the validator), were latent panics that
// now report correctly. Exported so openehr/validation's rmTypeInfo
// shares the same guard. REQ-024 compliant — no reflection.
func IsTypedNilPointer(v any) bool {
	return rm.IsTypedNil(v)
}
