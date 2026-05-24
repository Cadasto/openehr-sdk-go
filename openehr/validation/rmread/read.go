package rmread

import "github.com/cadasto/openehr-sdk-go/openehr/rm"

// ReadSingle returns the RM value at `attrName` on `parent`.
// `parentType` is the OPT-declared RM class name and is accepted
// for symmetry with ReadMultiple; routing dispatches on the Go
// concrete type of `parent`.
//
// `ok` is false when the attribute is absent (nil pointer, nil
// interface, structurally-empty CodePhrase / DVText / DVCodedText,
// empty string for the locatable archetype_node_id / name primary
// channels). Pointer attrs with non-nil but zero-value payloads
// report `ok=true` — the structural walker uses pointer presence
// as the absent/present signal.
//
// Returns `(nil, false)` for unknown (parentType, attrName) pairs;
// callers SHOULD treat that as "not addressable" rather than an
// error.
func ReadSingle(parent any, parentType, attrName string) (any, bool) {
	_ = parentType
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
	}
	return nil, false
}

// ReadMultiple returns the slice at `attrName` on `parent`, each
// element boxed as `any`. `ok` is true when the parent type
// carries the attribute (the returned slice may still be empty);
// it is false for unknown (parentType, attrName) pairs. Callers
// distinguish "absent" from "empty" via `len(items) == 0` — the
// cardinality check at the call site needs both signals.
func ReadMultiple(parent any, parentType, attrName string) ([]any, bool) {
	_ = parentType
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
	case "end_time":
		return ptrPresent(c.EndTime)
	case "location":
		if c.Location == nil || *c.Location == "" {
			return c.Location, false
		}
		return c.Location, true
	case "health_care_facility":
		return ptrPresent(c.HealthCareFacility)
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
		// the element carries non-empty archetype_node_id (or any
		// non-zero content); we use the archetype_node_id signal
		// because it's the BMM-mandatory LOCATABLE field.
		if s.Item.ArchetypeNodeID == "" && s.Item.Value == nil {
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
		return ptrPresent(e.NullReason)
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

// --- presence helpers ----------------------------------------------------

func strPresent(s string) (any, bool) {
	if s == "" {
		return s, false
	}
	return s, true
}

func dvTextPresent(t rm.DVText) (any, bool) {
	if t.Value == "" {
		return t, false
	}
	return t, true
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
// carries a non-nil concrete value. The function takes `any` so a
// single helper covers every RM interface attribute regardless of
// its declared type.
func ifacePresent(v any) (any, bool) {
	if v == nil {
		return v, false
	}
	return v, true
}
