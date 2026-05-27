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
	}
	return nil, false
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
		// SDK-GAP-11: rm.PartyIdentifiedLike is an interface — nilable
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
		// SDK-GAP-11: rm.DVTextLike is an interface — nilable on its
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

func dvTextPresent(v rm.DVTextLike) (any, bool) {
	// SDK-GAP-11: name / narrative slots are typed as rm.DVTextLike
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
// REQ-024 compliant: a closed type switch over the RM pointer
// concretes that can appear behind a Go interface in the v2
// content-type closed set. No reflection.
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
// Exported so openehr/validation's rmTypeInfo switch stays in lock-
// step with ifacePresent without duplicating the closed type set.
// REQ-024 compliant — no reflection.
func IsTypedNilPointer(v any) bool {
	return isTypedNilPointer(v)
}

// isTypedNilPointer detects the "interface carrying a typed-nil
// pointer" case for every RM concrete the validator descends into.
// The set is intentionally narrow: RM types pointer-stored behind
// an interface (DataValue, ItemStructure, Item, Event, ContentItem,
// PartyProxy). Value-typed concretes never trigger the typed-nil
// problem (a struct stored by value cannot be nil).
//
// Adding a new RM type means adding one switch case here, in
// rmread.ReadSingle/ReadMultiple, and in the parent validator's
// rmTypeInfo — the three switches are kept in lock-step.
func isTypedNilPointer(v any) bool {
	switch p := v.(type) {
	// DataValue concretes (Element.Value, etc.).
	case *rm.DVQuantity:
		return p == nil
	case *rm.DVText:
		return p == nil
	case *rm.DVCodedText:
		return p == nil
	case *rm.DVBoolean:
		return p == nil
	case *rm.DVCount:
		return p == nil
	case *rm.DVOrdinal:
		return p == nil
	case *rm.DVDate:
		return p == nil
	case *rm.DVTime:
		return p == nil
	case *rm.DVDateTime:
		return p == nil
	case *rm.DVDuration:
		return p == nil
	case *rm.DVURI:
		return p == nil
	case *rm.DVEHRURI:
		return p == nil
	case *rm.DVIdentifier:
		return p == nil
	case *rm.DVMultimedia:
		return p == nil
	case *rm.DVParsable:
		return p == nil
	case *rm.DVProportion:
		return p == nil

	// ItemStructure concretes (Observation.Protocol, Activity.Description, …).
	case *rm.ItemTree:
		return p == nil
	case *rm.ItemList:
		return p == nil
	case *rm.ItemSingle:
		return p == nil
	case *rm.ItemTable:
		return p == nil

	// Item concretes (Cluster.Items, Section items walked into Cluster/Element).
	case *rm.Cluster:
		return p == nil
	case *rm.Element:
		return p == nil

	// Event concretes (History.Events).
	case *rm.PointEvent[rm.ItemStructure]:
		return p == nil
	case *rm.IntervalEvent[rm.ItemStructure]:
		return p == nil

	// ContentItem concretes (Composition.Content, Section.Items).
	case *rm.Observation:
		return p == nil
	case *rm.Evaluation:
		return p == nil
	case *rm.Instruction:
		return p == nil
	case *rm.Action:
		return p == nil
	case *rm.AdminEntry:
		return p == nil
	case *rm.GenericEntry:
		return p == nil
	case *rm.Section:
		return p == nil

	// PartyProxy concretes (Composition.Composer, Entry.Subject).
	case *rm.PartySelf:
		return p == nil
	case *rm.PartyIdentified:
		return p == nil
	case *rm.PartyRelated:
		return p == nil
	}
	return false
}
