package rmwrite

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// NewRM constructs a fresh zero-value RM instance via the central
// typereg registry. Returns ErrUnknownRMType if the registry has no
// entry — the openehr/rm init() registers every concrete RM type
// the SDK knows about (REQ-040).
func NewRM(rmTypeName string) (any, error) {
	ctor, ok := typereg.Default.Lookup(rmTypeName)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownRMType, rmTypeName)
	}
	return ctor(), nil
}

// EnsureSingle sets the single-valued attribute named `attrName` on
// `parent` to `child`. The `parentType` string is retained for
// forward compatibility with a future string-keyed dispatch; v1
// dispatch is purely on the Go concrete type of `parent`.
//
// Returns ErrUnknownAttribute for unaddressable (parent, attrName)
// pairs and ErrTypeMismatch when child's Go type does not satisfy
// the slot.
func EnsureSingle(parent any, _ /* parentType */, attrName string, child any) error {
	switch p := parent.(type) {
	case *rm.Composition:
		return writeCompositionSingle(p, attrName, child)
	case *rm.Observation:
		return writeObservationSingle(p, attrName, child)
	case *rm.Evaluation:
		return writeEvaluationSingle(p, attrName, child)
	case *rm.Instruction:
		return writeInstructionSingle(p, attrName, child)
	case *rm.Action:
		return writeActionSingle(p, attrName, child)
	case *rm.AdminEntry:
		return writeAdminEntrySingle(p, attrName, child)
	case *rm.GenericEntry:
		return writeGenericEntrySingle(p, attrName, child)
	case *rm.Section:
		return writeSectionSingle(p, attrName, child)
	case *rm.Activity:
		return writeActivitySingle(p, attrName, child)
	case *rm.EventContext:
		return writeEventContextSingle(p, attrName, child)
	case *rm.History[rm.ItemStructure]:
		return writeHistorySingle(p, attrName, child)
	case *rm.PointEvent[rm.ItemStructure]:
		return writePointEventSingle(p, attrName, child)
	case *rm.IntervalEvent[rm.ItemStructure]:
		return writeIntervalEventSingle(p, attrName, child)
	case *rm.ItemTree:
		return writeItemTreeSingle(p, attrName, child)
	case *rm.ItemList:
		return writeItemListSingle(p, attrName, child)
	case *rm.ItemSingle:
		return writeItemSingleSingle(p, attrName, child)
	case *rm.ItemTable:
		return writeItemTableSingle(p, attrName, child)
	case *rm.Cluster:
		return writeClusterSingle(p, attrName, child)
	case *rm.Element:
		return writeElementSingle(p, attrName, child)
	case *rm.DVText:
		return writeDVTextSingle(p, attrName, child)
	case *rm.DVCodedText:
		return writeDVCodedTextSingle(p, attrName, child)
	case *rm.DVDate:
		return writeDVTemporalValueSingle("DV_DATE", attrName, child, func(s string) { p.Value = s })
	case *rm.DVTime:
		return writeDVTemporalValueSingle("DV_TIME", attrName, child, func(s string) { p.Value = s })
	case *rm.DVDateTime:
		return writeDVTemporalValueSingle("DV_DATE_TIME", attrName, child, func(s string) { p.Value = s })
	case *rm.DVDuration:
		return writeDVTemporalValueSingle("DV_DURATION", attrName, child, func(s string) { p.Value = s })
	case *rm.DVBoolean:
		return writeDVBooleanSingle(p, attrName, child)
	case *rm.DVCount:
		return writeDVCountSingle(p, attrName, child)
	case *rm.DVQuantity:
		return writeDVQuantitySingle(p, attrName, child)
	case *rm.DVMultimedia:
		return writeDVMultimediaSingle(p, attrName, child)
	case *rm.CodePhrase:
		return writeCodePhraseSingle(p, attrName, child)
	case *rm.DVInterval[rm.DVQuantity]:
		return writeDVIntervalQuantitySingle(p, attrName, child)
	case *rm.DVInterval[rm.DVCount]:
		return writeDVIntervalCountSingle(p, attrName, child)
	case *rm.DVInterval[rm.DVDateTime]:
		return writeDVIntervalDateTimeSingle(p, attrName, child)
	case *rm.DVInterval[rm.DVDate]:
		return writeDVIntervalDateSingle(p, attrName, child)
	case *rm.DVInterval[rm.DVTime]:
		return writeDVIntervalTimeSingle(p, attrName, child)
	case *rm.DVInterval[rm.DVProportion]:
		return writeDVIntervalProportionSingle(p, attrName, child)
	case *rm.DVInterval[rm.DVOrdered]:
		return writeDVIntervalOrderedSingle(p, attrName, child)
	}
	return fmt.Errorf("%w: parent %T, attr %q", ErrUnknownAttribute, parent, attrName)
}

// AppendMultiple appends `child` to the multi-valued slice
// attribute on `parent`. Same dispatch / error contract as
// EnsureSingle.
func AppendMultiple(parent any, _ /* parentType */, attrName string, child any) error {
	switch p := parent.(type) {
	case *rm.Composition:
		return writeCompositionMultiple(p, attrName, child)
	case *rm.Section:
		return writeSectionMultiple(p, attrName, child)
	case *rm.Instruction:
		return writeInstructionMultiple(p, attrName, child)
	case *rm.History[rm.ItemStructure]:
		return writeHistoryMultiple(p, attrName, child)
	case *rm.ItemTree:
		return writeItemTreeMultiple(p, attrName, child)
	case *rm.ItemList:
		return writeItemListMultiple(p, attrName, child)
	case *rm.ItemTable:
		return writeItemTableMultiple(p, attrName, child)
	case *rm.Cluster:
		return writeClusterMultiple(p, attrName, child)
	}
	return fmt.Errorf("%w: parent %T, attr %q", ErrUnknownAttribute, parent, attrName)
}

// --- COMPOSITION ---------------------------------------------------------

func writeCompositionSingle(c *rm.Composition, attr string, child any) error {
	switch attr {
	case "category":
		v, ok := coerceDVCodedText(child)
		if !ok {
			return mismatch(attr, child, "DV_CODED_TEXT")
		}
		c.Category = v
		return nil
	case "composer":
		v, ok := child.(rm.PartyProxy)
		if !ok {
			return mismatch(attr, child, "PARTY_PROXY")
		}
		c.Composer = v
		return nil
	case "language":
		v, ok := coerceCodePhrase(child)
		if !ok {
			return mismatch(attr, child, "CODE_PHRASE")
		}
		c.Language = v
		return nil
	case "territory":
		v, ok := coerceCodePhrase(child)
		if !ok {
			return mismatch(attr, child, "CODE_PHRASE")
		}
		c.Territory = v
		return nil
	case "context":
		v, ok := child.(*rm.EventContext)
		if !ok {
			return mismatch(attr, child, "EVENT_CONTEXT")
		}
		c.Context = v
		return nil
	}
	return fmt.Errorf("%w: *rm.Composition has no single attr %q", ErrUnknownAttribute, attr)
}

func writeCompositionMultiple(c *rm.Composition, attr string, child any) error {
	switch attr {
	case "content":
		v, ok := child.(rm.ContentItem)
		if !ok {
			return mismatch(attr, child, "CONTENT_ITEM")
		}
		c.Content = append(c.Content, v)
		return nil
	}
	return fmt.Errorf("%w: *rm.Composition has no multiple attr %q", ErrUnknownAttribute, attr)
}

// --- OBSERVATION ---------------------------------------------------------

func writeObservationSingle(o *rm.Observation, attr string, child any) error {
	switch attr {
	case "language":
		v, ok := coerceCodePhrase(child)
		if !ok {
			return mismatch(attr, child, "CODE_PHRASE")
		}
		o.Language = v
		return nil
	case "encoding":
		v, ok := coerceCodePhrase(child)
		if !ok {
			return mismatch(attr, child, "CODE_PHRASE")
		}
		o.Encoding = v
		return nil
	case "subject":
		v, ok := child.(rm.PartyProxy)
		if !ok {
			return mismatch(attr, child, "PARTY_PROXY")
		}
		o.Subject = v
		return nil
	case "provider":
		v, ok := child.(rm.PartyProxy)
		if !ok {
			return mismatch(attr, child, "PARTY_PROXY")
		}
		o.Provider = v
		return nil
	case "data":
		// Observation.Data is value-typed HISTORY; accept *History or History.
		return assignVia(child, func(v rm.History[rm.ItemStructure]) { o.Data = v }, attr, "HISTORY")
	case "state":
		v, ok := child.(*rm.History[rm.ItemStructure])
		if !ok {
			return mismatch(attr, child, "HISTORY")
		}
		o.State = v
		return nil
	case "protocol":
		v, ok := child.(rm.ItemStructure)
		if !ok {
			return mismatch(attr, child, "ITEM_STRUCTURE")
		}
		o.Protocol = v
		return nil
	}
	return fmt.Errorf("%w: *rm.Observation has no single attr %q", ErrUnknownAttribute, attr)
}

// --- EVALUATION ----------------------------------------------------------

func writeEvaluationSingle(e *rm.Evaluation, attr string, child any) error {
	switch attr {
	case "language":
		v, ok := coerceCodePhrase(child)
		if !ok {
			return mismatch(attr, child, "CODE_PHRASE")
		}
		e.Language = v
		return nil
	case "encoding":
		v, ok := coerceCodePhrase(child)
		if !ok {
			return mismatch(attr, child, "CODE_PHRASE")
		}
		e.Encoding = v
		return nil
	case "subject":
		v, ok := child.(rm.PartyProxy)
		if !ok {
			return mismatch(attr, child, "PARTY_PROXY")
		}
		e.Subject = v
		return nil
	case "provider":
		v, ok := child.(rm.PartyProxy)
		if !ok {
			return mismatch(attr, child, "PARTY_PROXY")
		}
		e.Provider = v
		return nil
	case "data":
		v, ok := child.(rm.ItemStructure)
		if !ok {
			return mismatch(attr, child, "ITEM_STRUCTURE")
		}
		e.Data = v
		return nil
	case "protocol":
		v, ok := child.(rm.ItemStructure)
		if !ok {
			return mismatch(attr, child, "ITEM_STRUCTURE")
		}
		e.Protocol = v
		return nil
	}
	return fmt.Errorf("%w: *rm.Evaluation has no single attr %q", ErrUnknownAttribute, attr)
}

// --- INSTRUCTION ---------------------------------------------------------

func writeInstructionSingle(i *rm.Instruction, attr string, child any) error {
	switch attr {
	case "language":
		v, ok := coerceCodePhrase(child)
		if !ok {
			return mismatch(attr, child, "CODE_PHRASE")
		}
		i.Language = v
		return nil
	case "encoding":
		v, ok := coerceCodePhrase(child)
		if !ok {
			return mismatch(attr, child, "CODE_PHRASE")
		}
		i.Encoding = v
		return nil
	case "narrative":
		v, ok := coerceDVText(child)
		if !ok {
			return mismatch(attr, child, "DV_TEXT")
		}
		i.Narrative = v
		return nil
	case "protocol":
		v, ok := child.(rm.ItemStructure)
		if !ok {
			return mismatch(attr, child, "ITEM_STRUCTURE")
		}
		i.Protocol = v
		return nil
	case "expiry_time":
		v, ok := child.(*rm.DVDateTime)
		if !ok {
			return mismatch(attr, child, "DV_DATE_TIME")
		}
		i.ExpiryTime = v
		return nil
	case "wf_definition":
		v, ok := child.(*rm.DVParsable)
		if !ok {
			return mismatch(attr, child, "DV_PARSABLE")
		}
		i.WfDefinition = v
		return nil
	case "subject":
		v, ok := child.(rm.PartyProxy)
		if !ok {
			return mismatch(attr, child, "PARTY_PROXY")
		}
		i.Subject = v
		return nil
	case "provider":
		v, ok := child.(rm.PartyProxy)
		if !ok {
			return mismatch(attr, child, "PARTY_PROXY")
		}
		i.Provider = v
		return nil
	}
	return fmt.Errorf("%w: *rm.Instruction has no single attr %q", ErrUnknownAttribute, attr)
}

func writeInstructionMultiple(i *rm.Instruction, attr string, child any) error {
	switch attr {
	case "activities":
		return assignVia(child, func(v rm.Activity) { i.Activities = append(i.Activities, v) }, attr, "ACTIVITY")
	}
	return fmt.Errorf("%w: *rm.Instruction has no multiple attr %q", ErrUnknownAttribute, attr)
}

// --- ACTION --------------------------------------------------------------

func writeActionSingle(a *rm.Action, attr string, child any) error {
	switch attr {
	case "language":
		v, ok := coerceCodePhrase(child)
		if !ok {
			return mismatch(attr, child, "CODE_PHRASE")
		}
		a.Language = v
		return nil
	case "encoding":
		v, ok := coerceCodePhrase(child)
		if !ok {
			return mismatch(attr, child, "CODE_PHRASE")
		}
		a.Encoding = v
		return nil
	case "description":
		v, ok := child.(rm.ItemStructure)
		if !ok {
			return mismatch(attr, child, "ITEM_STRUCTURE")
		}
		a.Description = v
		return nil
	case "protocol":
		v, ok := child.(rm.ItemStructure)
		if !ok {
			return mismatch(attr, child, "ITEM_STRUCTURE")
		}
		a.Protocol = v
		return nil
	case "subject":
		v, ok := child.(rm.PartyProxy)
		if !ok {
			return mismatch(attr, child, "PARTY_PROXY")
		}
		a.Subject = v
		return nil
	case "provider":
		v, ok := child.(rm.PartyProxy)
		if !ok {
			return mismatch(attr, child, "PARTY_PROXY")
		}
		a.Provider = v
		return nil
	}
	return fmt.Errorf("%w: *rm.Action has no single attr %q", ErrUnknownAttribute, attr)
}

// --- ADMIN_ENTRY ---------------------------------------------------------

func writeAdminEntrySingle(a *rm.AdminEntry, attr string, child any) error {
	switch attr {
	case "language":
		v, ok := coerceCodePhrase(child)
		if !ok {
			return mismatch(attr, child, "CODE_PHRASE")
		}
		a.Language = v
		return nil
	case "encoding":
		v, ok := coerceCodePhrase(child)
		if !ok {
			return mismatch(attr, child, "CODE_PHRASE")
		}
		a.Encoding = v
		return nil
	case "data":
		v, ok := child.(rm.ItemStructure)
		if !ok {
			return mismatch(attr, child, "ITEM_STRUCTURE")
		}
		a.Data = v
		return nil
	case "subject":
		v, ok := child.(rm.PartyProxy)
		if !ok {
			return mismatch(attr, child, "PARTY_PROXY")
		}
		a.Subject = v
		return nil
	case "provider":
		v, ok := child.(rm.PartyProxy)
		if !ok {
			return mismatch(attr, child, "PARTY_PROXY")
		}
		a.Provider = v
		return nil
	}
	return fmt.Errorf("%w: *rm.AdminEntry has no single attr %q", ErrUnknownAttribute, attr)
}

// --- GENERIC_ENTRY -------------------------------------------------------

func writeGenericEntrySingle(g *rm.GenericEntry, attr string, child any) error {
	switch attr {
	case "data":
		v, ok := child.(rm.Item)
		if !ok {
			return mismatch(attr, child, "ITEM")
		}
		g.Data = v
		return nil
	}
	return fmt.Errorf("%w: *rm.GenericEntry has no single attr %q", ErrUnknownAttribute, attr)
}

// --- SECTION -------------------------------------------------------------

func writeSectionSingle(_ *rm.Section, attr string, _ any) error {
	return fmt.Errorf("%w: *rm.Section has no single attr %q", ErrUnknownAttribute, attr)
}

func writeSectionMultiple(s *rm.Section, attr string, child any) error {
	switch attr {
	case "items":
		v, ok := child.(rm.ContentItem)
		if !ok {
			return mismatch(attr, child, "CONTENT_ITEM")
		}
		s.Items = append(s.Items, v)
		return nil
	}
	return fmt.Errorf("%w: *rm.Section has no multiple attr %q", ErrUnknownAttribute, attr)
}

// --- ACTIVITY ------------------------------------------------------------

func writeActivitySingle(a *rm.Activity, attr string, child any) error {
	switch attr {
	case "description":
		v, ok := child.(rm.ItemStructure)
		if !ok {
			return mismatch(attr, child, "ITEM_STRUCTURE")
		}
		a.Description = v
		return nil
	case "timing":
		v, ok := child.(*rm.DVParsable)
		if !ok {
			return mismatch(attr, child, "DV_PARSABLE")
		}
		a.Timing = v
		return nil
	case "action_archetype_id":
		v, ok := child.(string)
		if !ok {
			return mismatch(attr, child, "String")
		}
		a.ActionArchetypeID = v
		return nil
	}
	return fmt.Errorf("%w: *rm.Activity has no single attr %q", ErrUnknownAttribute, attr)
}

// --- EVENT_CONTEXT -------------------------------------------------------

func writeEventContextSingle(c *rm.EventContext, attr string, child any) error {
	switch attr {
	case "end_time":
		v, ok := child.(*rm.DVDateTime)
		if !ok {
			return mismatch(attr, child, "DV_DATE_TIME")
		}
		c.EndTime = v
		return nil
	case "location":
		v, ok := child.(*string)
		if !ok {
			return mismatch(attr, child, "String")
		}
		c.Location = v
		return nil
	case "health_care_facility":
		v, ok := child.(*rm.PartyIdentified)
		if !ok {
			return mismatch(attr, child, "PARTY_IDENTIFIED")
		}
		c.HealthCareFacility = v
		return nil
	case "other_context":
		v, ok := child.(rm.ItemStructure)
		if !ok {
			return mismatch(attr, child, "ITEM_STRUCTURE")
		}
		c.OtherContext = v
		return nil
	case "setting":
		v, ok := coerceDVCodedText(child)
		if !ok {
			return mismatch(attr, child, "DV_CODED_TEXT")
		}
		c.Setting = v
		return nil
	case "start_time":
		return assignVia(child, func(v rm.DVDateTime) { c.StartTime = v }, attr, "DV_DATE_TIME")
	}
	return fmt.Errorf("%w: *rm.EventContext has no single attr %q", ErrUnknownAttribute, attr)
}

// --- HISTORY -------------------------------------------------------------

func writeHistorySingle(h *rm.History[rm.ItemStructure], attr string, child any) error {
	switch attr {
	case "origin":
		return assignVia(child, func(v rm.DVDateTime) { h.Origin = v }, attr, "DV_DATE_TIME")
	case "period":
		v, ok := child.(*rm.DVDuration)
		if !ok {
			return mismatch(attr, child, "DV_DURATION")
		}
		h.Period = v
		return nil
	case "duration":
		v, ok := child.(*rm.DVDuration)
		if !ok {
			return mismatch(attr, child, "DV_DURATION")
		}
		h.Duration = v
		return nil
	case "summary":
		v, ok := child.(rm.ItemStructure)
		if !ok {
			return mismatch(attr, child, "ITEM_STRUCTURE")
		}
		h.Summary = v
		return nil
	}
	return fmt.Errorf("%w: *rm.History has no single attr %q", ErrUnknownAttribute, attr)
}

func writeHistoryMultiple(h *rm.History[rm.ItemStructure], attr string, child any) error {
	switch attr {
	case "events":
		v, ok := child.(rm.Event)
		if !ok {
			return mismatch(attr, child, "EVENT")
		}
		h.Events = append(h.Events, v)
		return nil
	}
	return fmt.Errorf("%w: *rm.History has no multiple attr %q", ErrUnknownAttribute, attr)
}

// --- POINT_EVENT / INTERVAL_EVENT ---------------------------------------

func writePointEventSingle(e *rm.PointEvent[rm.ItemStructure], attr string, child any) error {
	switch attr {
	case "time":
		return assignVia(child, func(v rm.DVDateTime) { e.Time = v }, attr, "DV_DATE_TIME")
	case "data":
		v, ok := child.(rm.ItemStructure)
		if !ok {
			return mismatch(attr, child, "ITEM_STRUCTURE")
		}
		e.Data = v
		return nil
	case "state":
		v, ok := child.(rm.ItemStructure)
		if !ok {
			return mismatch(attr, child, "ITEM_STRUCTURE")
		}
		e.State = v
		return nil
	}
	return fmt.Errorf("%w: *rm.PointEvent has no single attr %q", ErrUnknownAttribute, attr)
}

func writeIntervalEventSingle(e *rm.IntervalEvent[rm.ItemStructure], attr string, child any) error {
	switch attr {
	case "time":
		return assignVia(child, func(v rm.DVDateTime) { e.Time = v }, attr, "DV_DATE_TIME")
	case "width":
		return assignVia(child, func(v rm.DVDuration) { e.Width = v }, attr, "DV_DURATION")
	case "math_function":
		v, ok := coerceDVCodedText(child)
		if !ok {
			return mismatch(attr, child, "DV_CODED_TEXT")
		}
		e.MathFunction = v
		return nil
	case "sample_count":
		v, ok := child.(*rm.Integer)
		if !ok {
			return mismatch(attr, child, "Integer")
		}
		e.SampleCount = v
		return nil
	case "data":
		v, ok := child.(rm.ItemStructure)
		if !ok {
			return mismatch(attr, child, "ITEM_STRUCTURE")
		}
		e.Data = v
		return nil
	case "state":
		v, ok := child.(rm.ItemStructure)
		if !ok {
			return mismatch(attr, child, "ITEM_STRUCTURE")
		}
		e.State = v
		return nil
	}
	return fmt.Errorf("%w: *rm.IntervalEvent has no single attr %q", ErrUnknownAttribute, attr)
}

// --- ITEM_STRUCTURE variants --------------------------------------------

func writeItemTreeSingle(_ *rm.ItemTree, attr string, _ any) error {
	return fmt.Errorf("%w: *rm.ItemTree has no single attr %q", ErrUnknownAttribute, attr)
}

func writeItemTreeMultiple(t *rm.ItemTree, attr string, child any) error {
	switch attr {
	case "items":
		v, ok := child.(rm.Item)
		if !ok {
			return mismatch(attr, child, "ITEM")
		}
		t.Items = append(t.Items, v)
		return nil
	}
	return fmt.Errorf("%w: *rm.ItemTree has no multiple attr %q", ErrUnknownAttribute, attr)
}

func writeItemListSingle(_ *rm.ItemList, attr string, _ any) error {
	return fmt.Errorf("%w: *rm.ItemList has no single attr %q", ErrUnknownAttribute, attr)
}

func writeItemListMultiple(l *rm.ItemList, attr string, child any) error {
	switch attr {
	case "items":
		return assignVia(child, func(v rm.Element) { l.Items = append(l.Items, v) }, attr, "ELEMENT")
	}
	return fmt.Errorf("%w: *rm.ItemList has no multiple attr %q", ErrUnknownAttribute, attr)
}

func writeItemSingleSingle(s *rm.ItemSingle, attr string, child any) error {
	switch attr {
	case "item":
		return assignVia(child, func(v rm.Element) { s.Item = v }, attr, "ELEMENT")
	}
	return fmt.Errorf("%w: *rm.ItemSingle has no single attr %q", ErrUnknownAttribute, attr)
}

func writeItemTableSingle(_ *rm.ItemTable, attr string, _ any) error {
	return fmt.Errorf("%w: *rm.ItemTable has no single attr %q", ErrUnknownAttribute, attr)
}

func writeItemTableMultiple(t *rm.ItemTable, attr string, child any) error {
	switch attr {
	case "rows":
		return assignVia(child, func(v rm.Cluster) { t.Rows = append(t.Rows, v) }, attr, "CLUSTER")
	}
	return fmt.Errorf("%w: *rm.ItemTable has no multiple attr %q", ErrUnknownAttribute, attr)
}

// --- CLUSTER / ELEMENT --------------------------------------------------

func writeClusterSingle(c *rm.Cluster, attr string, child any) error {
	switch attr {
	case "name":
		if v, ok := coerceDVText(child); ok {
			c.Name = v
			return nil
		}
		if v, ok := coerceDVCodedText(child); ok {
			c.Name = v
			return nil
		}
		return mismatch(attr, child, "DV_TEXT")
	}
	return fmt.Errorf("%w: *rm.Cluster has no single attr %q", ErrUnknownAttribute, attr)
}

func writeClusterMultiple(c *rm.Cluster, attr string, child any) error {
	switch attr {
	case "items":
		v, ok := child.(rm.Item)
		if !ok {
			return mismatch(attr, child, "ITEM")
		}
		c.Items = append(c.Items, v)
		return nil
	}
	return fmt.Errorf("%w: *rm.Cluster has no multiple attr %q", ErrUnknownAttribute, attr)
}

func writeElementSingle(e *rm.Element, attr string, child any) error {
	switch attr {
	case "value":
		v, ok := child.(rm.DataValue)
		if !ok {
			return mismatch(attr, child, "DATA_VALUE")
		}
		e.Value = v
		return nil
	case "null_flavour":
		v, ok := child.(*rm.DVCodedText)
		if !ok {
			return mismatch(attr, child, "DV_CODED_TEXT")
		}
		e.NullFlavour = v
		return nil
	case "null_reason":
		v, ok := child.(*rm.DVText)
		if !ok {
			return mismatch(attr, child, "DV_TEXT")
		}
		e.NullReason = v
		return nil
	case "name":
		if v, ok := coerceDVText(child); ok {
			e.Name = v
			return nil
		}
		if v, ok := coerceDVCodedText(child); ok {
			e.Name = v
			return nil
		}
		return mismatch(attr, child, "DV_TEXT")
	}
	return fmt.Errorf("%w: *rm.Element has no single attr %q", ErrUnknownAttribute, attr)
}

// --- DataValue navigations ----------------------------------------------

// writeDVTemporalValueSingle handles `value` on DV_DATE / DV_TIME /
// DV_DATE_TIME / DV_DURATION — all carry an ISO 8601 string. The
// setter closure binds the concrete RM field; rmType names the wire
// shape for the mismatch detail. AOM 1.4 primitive short names
// (DURATION, DATE, ...) materialise as these wrappers via
// instance.concreteFor.
func writeDVTemporalValueSingle(rmType, attr string, child any, set func(string)) error {
	if attr != "value" {
		return fmt.Errorf("%w: %s has no single attr %q", ErrUnknownAttribute, rmType, attr)
	}
	v, ok := child.(string)
	if !ok {
		return mismatch(attr, child, "String")
	}
	set(v)
	return nil
}

// writeDVBooleanSingle handles `value` on DV_BOOLEAN. Parallels the
// temporal writers; primitive type is bool, not string.
func writeDVBooleanSingle(b *rm.DVBoolean, attr string, child any) error {
	if attr != "value" {
		return fmt.Errorf("%w: *rm.DVBoolean has no single attr %q", ErrUnknownAttribute, attr)
	}
	v, ok := child.(bool)
	if !ok {
		return mismatch(attr, child, "Boolean")
	}
	b.Value = v
	return nil
}

func writeDVTextSingle(t *rm.DVText, attr string, child any) error {
	switch attr {
	case "value":
		v, ok := child.(string)
		if !ok {
			return mismatch(attr, child, "String")
		}
		t.Value = v
		return nil
	}
	return fmt.Errorf("%w: *rm.DVText has no single attr %q", ErrUnknownAttribute, attr)
}

func writeDVCodedTextSingle(t *rm.DVCodedText, attr string, child any) error {
	switch attr {
	case "value":
		v, ok := child.(string)
		if !ok {
			return mismatch(attr, child, "String")
		}
		t.Value = v
		return nil
	case "defining_code":
		v, ok := coerceCodePhrase(child)
		if !ok {
			return mismatch(attr, child, "CODE_PHRASE")
		}
		if v.TerminologyID.Value == "" {
			v.TerminologyID = rm.TerminologyID{Value: "local"}
		}
		t.DefiningCode = v
		return nil
	}
	return fmt.Errorf("%w: *rm.DVCodedText has no single attr %q", ErrUnknownAttribute, attr)
}

func writeCodePhraseSingle(c *rm.CodePhrase, attr string, child any) error {
	switch attr {
	case "code_string":
		v, ok := child.(string)
		if !ok {
			return mismatch(attr, child, "String")
		}
		c.CodeString = v
		if c.TerminologyID.Value == "" {
			c.TerminologyID = rm.TerminologyID{Value: "local"}
		}
		return nil
	case "terminology_id":
		switch v := child.(type) {
		case string:
			c.TerminologyID = rm.TerminologyID{Value: v}
			return nil
		case *rm.TerminologyID:
			if v == nil {
				return mismatch(attr, child, "TERMINOLOGY_ID")
			}
			c.TerminologyID = *v
			return nil
		case rm.TerminologyID:
			c.TerminologyID = v
			return nil
		}
		return mismatch(attr, child, "TERMINOLOGY_ID")
	case "preferred_term":
		v, ok := child.(*string)
		if !ok {
			return mismatch(attr, child, "String")
		}
		c.PreferredTerm = v
		return nil
	}
	return fmt.Errorf("%w: *rm.CodePhrase has no single attr %q", ErrUnknownAttribute, attr)
}

// --- coercion helpers ---------------------------------------------------

// coerceValueOrPtr accepts child as either T or *T, dereferencing the
// pointer form and treating a nil pointer as a coercion failure — a
// nil *T fails; a typed-nil pointer satisfying an interface T (e.g.
// the rm.DVOrdered instantiation via writeIntervalSingle) still
// matches case T and coerces with ok=true. It is the single
// value-or-pointer coercion primitive behind assignVia and the
// coerce<RMType> wrappers below; writeIntervalSingle
// (interval_write.go) reaches it via assignVia for the DV_INTERVAL<T>
// bounds.
func coerceValueOrPtr[T any](child any) (T, bool) {
	var zero T
	switch v := child.(type) {
	case T:
		return v, true
	case *T:
		if v == nil {
			return zero, false
		}
		return *v, true
	}
	return zero, false
}

// assignVia coerces child to T via coerceValueOrPtr and hands the
// result to set — a caller-supplied closure that either assigns a
// scalar field or appends to a slice, so one helper covers both
// shapes uniformly. Mirrors the writeDVTemporalValueSingle closure
// idiom, generalised beyond the string primitive. On coercion
// failure it returns the same mismatch(attr, child, rmName) every
// inline coercion switch in this file used to build directly, so
// error output is unchanged.
func assignVia[T any](child any, set func(T), attr, rmName string) error {
	v, ok := coerceValueOrPtr[T](child)
	if !ok {
		return mismatch(attr, child, rmName)
	}
	set(v)
	return nil
}

// coerceDVText accepts both value- and pointer-receiver shapes; the
// typereg ctor yields *DVText but rmread / hand-built code may pass
// value-typed equivalents.
func coerceDVText(child any) (rm.DVText, bool) {
	return coerceValueOrPtr[rm.DVText](child)
}

func coerceDVCodedText(child any) (rm.DVCodedText, bool) {
	return coerceValueOrPtr[rm.DVCodedText](child)
}

func coerceCodePhrase(child any) (rm.CodePhrase, bool) {
	return coerceValueOrPtr[rm.CodePhrase](child)
}

// mismatch builds a wrapped ErrTypeMismatch with diagnostic context.
func mismatch(attr string, got any, wantRM string) error {
	return fmt.Errorf("%w: attr %q expects %s, got %T", ErrTypeMismatch, attr, wantRM, got)
}
