package rmpath

import "github.com/cadasto/openehr-sdk-go/openehr/rm"

// childrenAt returns the child object(s) at attribute attr on parent,
// flattened to a slice (0, 1, or many). Dispatch is on the concrete RM
// type — reflection-free (REQ-024). An unknown (type, attr) pair yields
// no children, so the path simply fails to resolve rather than panicking.
func childrenAt(parent any, attr string) []any {
	switch p := parent.(type) {
	case *rm.Composition:
		return compositionChildren(p, attr)
	case rm.Composition:
		return compositionChildren(&p, attr)

	case *rm.Section:
		return sectionChildren(p, attr)
	case rm.Section:
		return sectionChildren(&p, attr)

	case *rm.Observation:
		return observationChildren(p, attr)
	case rm.Observation:
		return observationChildren(&p, attr)

	case *rm.Evaluation:
		return evaluationChildren(p, attr)
	case rm.Evaluation:
		return evaluationChildren(&p, attr)

	case *rm.Instruction:
		return instructionChildren(p, attr)
	case rm.Instruction:
		return instructionChildren(&p, attr)

	case *rm.Action:
		return actionChildren(p, attr)
	case rm.Action:
		return actionChildren(&p, attr)

	case *rm.AdminEntry:
		return adminEntryChildren(p, attr)
	case rm.AdminEntry:
		return adminEntryChildren(&p, attr)

	case *rm.GenericEntry:
		return genericEntryChildren(p, attr)
	case rm.GenericEntry:
		return genericEntryChildren(&p, attr)

	case *rm.Activity:
		return activityChildren(p, attr)
	case rm.Activity:
		return activityChildren(&p, attr)

	case *rm.History[rm.ItemStructure]:
		return historyChildren(p, attr)
	case rm.History[rm.ItemStructure]:
		return historyChildren(&p, attr)

	case *rm.PointEvent[rm.ItemStructure]:
		return pointEventChildren(p, attr)
	case rm.PointEvent[rm.ItemStructure]:
		return pointEventChildren(&p, attr)

	case *rm.IntervalEvent[rm.ItemStructure]:
		return intervalEventChildren(p, attr)
	case rm.IntervalEvent[rm.ItemStructure]:
		return intervalEventChildren(&p, attr)

	case *rm.ItemTree:
		return itemTreeChildren(p, attr)
	case rm.ItemTree:
		return itemTreeChildren(&p, attr)

	case *rm.ItemList:
		return itemListChildren(p, attr)
	case rm.ItemList:
		return itemListChildren(&p, attr)

	case *rm.ItemSingle:
		return itemSingleChildren(p, attr)
	case rm.ItemSingle:
		return itemSingleChildren(&p, attr)

	case *rm.ItemTable:
		return itemTableChildren(p, attr)
	case rm.ItemTable:
		return itemTableChildren(&p, attr)

	case *rm.Cluster:
		return clusterChildren(p, attr)
	case rm.Cluster:
		return clusterChildren(&p, attr)

	case *rm.Element:
		return elementChildren(p, attr)
	case rm.Element:
		return elementChildren(&p, attr)
	}
	return nil
}

// iface wraps a non-nil interface value as a single child.
func iface(v any) []any {
	if v == nil {
		return nil
	}
	return []any{v}
}

func compositionChildren(c *rm.Composition, attr string) []any {
	switch attr {
	case "content":
		return ifaceSlice(c.Content)
	case "context":
		if c.Context == nil {
			return nil
		}
		return []any{c.Context}
	case "category":
		return []any{c.Category}
	case "name":
		return iface(c.Name)
	case "composer":
		return iface(c.Composer)
	}
	return nil
}

func sectionChildren(s *rm.Section, attr string) []any {
	switch attr {
	case "items":
		return ifaceSlice(s.Items)
	case "name":
		return iface(s.Name)
	}
	return nil
}

func observationChildren(o *rm.Observation, attr string) []any {
	switch attr {
	case "data":
		return []any{&o.Data}
	case "state":
		if o.State == nil {
			return nil
		}
		return []any{o.State}
	case "protocol":
		return iface(o.Protocol)
	case "name":
		return iface(o.Name)
	}
	return nil
}

func evaluationChildren(e *rm.Evaluation, attr string) []any {
	switch attr {
	case "data":
		return iface(e.Data)
	case "protocol":
		return iface(e.Protocol)
	case "name":
		return iface(e.Name)
	}
	return nil
}

func instructionChildren(i *rm.Instruction, attr string) []any {
	switch attr {
	case "activities":
		out := make([]any, 0, len(i.Activities))
		for k := range i.Activities {
			out = append(out, &i.Activities[k])
		}
		return out
	case "protocol":
		return iface(i.Protocol)
	case "name":
		return iface(i.Name)
	}
	return nil
}

func actionChildren(a *rm.Action, attr string) []any {
	switch attr {
	case "description":
		return iface(a.Description)
	case "protocol":
		return iface(a.Protocol)
	case "name":
		return iface(a.Name)
	}
	return nil
}

func adminEntryChildren(a *rm.AdminEntry, attr string) []any {
	switch attr {
	case "data":
		return iface(a.Data)
	case "name":
		return iface(a.Name)
	}
	return nil
}

func genericEntryChildren(g *rm.GenericEntry, attr string) []any {
	switch attr {
	case "data":
		return iface(g.Data)
	case "name":
		return iface(g.Name)
	}
	return nil
}

func activityChildren(a *rm.Activity, attr string) []any {
	switch attr {
	case "description":
		return iface(a.Description)
	case "name":
		return iface(a.Name)
	}
	return nil
}

func historyChildren(h *rm.History[rm.ItemStructure], attr string) []any {
	switch attr {
	case "events":
		out := make([]any, 0, len(h.Events))
		for _, e := range h.Events {
			out = append(out, e)
		}
		return out
	case "summary":
		return iface(h.Summary)
	case "name":
		return iface(h.Name)
	}
	return nil
}

func pointEventChildren(e *rm.PointEvent[rm.ItemStructure], attr string) []any {
	switch attr {
	case "data":
		return iface(e.Data)
	case "state":
		return iface(e.State)
	case "name":
		return iface(e.Name)
	}
	return nil
}

func intervalEventChildren(e *rm.IntervalEvent[rm.ItemStructure], attr string) []any {
	switch attr {
	case "data":
		return iface(e.Data)
	case "state":
		return iface(e.State)
	case "name":
		return iface(e.Name)
	}
	return nil
}

func itemTreeChildren(t *rm.ItemTree, attr string) []any {
	switch attr {
	case "items":
		return ifaceSlice(t.Items)
	case "name":
		return iface(t.Name)
	}
	return nil
}

func itemListChildren(l *rm.ItemList, attr string) []any {
	switch attr {
	case "items":
		out := make([]any, 0, len(l.Items))
		for k := range l.Items {
			out = append(out, &l.Items[k])
		}
		return out
	case "name":
		return iface(l.Name)
	}
	return nil
}

func itemSingleChildren(s *rm.ItemSingle, attr string) []any {
	switch attr {
	case "item":
		return []any{&s.Item}
	case "name":
		return iface(s.Name)
	}
	return nil
}

func itemTableChildren(t *rm.ItemTable, attr string) []any {
	switch attr {
	case "rows":
		out := make([]any, 0, len(t.Rows))
		for k := range t.Rows {
			out = append(out, &t.Rows[k])
		}
		return out
	case "name":
		return iface(t.Name)
	}
	return nil
}

func clusterChildren(c *rm.Cluster, attr string) []any {
	switch attr {
	case "items":
		return ifaceSlice(c.Items)
	case "name":
		return iface(c.Name)
	}
	return nil
}

func elementChildren(e *rm.Element, attr string) []any {
	switch attr {
	case "value":
		return iface(e.Value)
	case "null_flavour":
		if e.NullFlavour == nil {
			return nil
		}
		return []any{e.NullFlavour}
	case "name":
		return iface(e.Name)
	}
	return nil
}

// ifaceSlice flattens a slice of an interface element type to []any,
// skipping nil entries.
func ifaceSlice[T any](s []T) []any {
	out := make([]any, 0, len(s))
	for _, v := range s {
		if av := any(v); av != nil {
			out = append(out, av)
		}
	}
	return out
}

// nodeIDOf returns the archetype_node_id of a LOCATABLE child, or "".
func nodeIDOf(o any) string {
	switch v := o.(type) {
	case *rm.Composition:
		return v.ArchetypeNodeID
	case rm.Composition:
		return v.ArchetypeNodeID
	case *rm.Section:
		return v.ArchetypeNodeID
	case rm.Section:
		return v.ArchetypeNodeID
	case *rm.Observation:
		return v.ArchetypeNodeID
	case rm.Observation:
		return v.ArchetypeNodeID
	case *rm.Evaluation:
		return v.ArchetypeNodeID
	case rm.Evaluation:
		return v.ArchetypeNodeID
	case *rm.Instruction:
		return v.ArchetypeNodeID
	case rm.Instruction:
		return v.ArchetypeNodeID
	case *rm.Action:
		return v.ArchetypeNodeID
	case rm.Action:
		return v.ArchetypeNodeID
	case *rm.AdminEntry:
		return v.ArchetypeNodeID
	case rm.AdminEntry:
		return v.ArchetypeNodeID
	case *rm.GenericEntry:
		return v.ArchetypeNodeID
	case rm.GenericEntry:
		return v.ArchetypeNodeID
	case *rm.Activity:
		return v.ArchetypeNodeID
	case rm.Activity:
		return v.ArchetypeNodeID
	case *rm.History[rm.ItemStructure]:
		return v.ArchetypeNodeID
	case rm.History[rm.ItemStructure]:
		return v.ArchetypeNodeID
	case *rm.PointEvent[rm.ItemStructure]:
		return v.ArchetypeNodeID
	case rm.PointEvent[rm.ItemStructure]:
		return v.ArchetypeNodeID
	case *rm.IntervalEvent[rm.ItemStructure]:
		return v.ArchetypeNodeID
	case rm.IntervalEvent[rm.ItemStructure]:
		return v.ArchetypeNodeID
	case *rm.ItemTree:
		return v.ArchetypeNodeID
	case rm.ItemTree:
		return v.ArchetypeNodeID
	case *rm.ItemList:
		return v.ArchetypeNodeID
	case rm.ItemList:
		return v.ArchetypeNodeID
	case *rm.ItemSingle:
		return v.ArchetypeNodeID
	case rm.ItemSingle:
		return v.ArchetypeNodeID
	case *rm.ItemTable:
		return v.ArchetypeNodeID
	case rm.ItemTable:
		return v.ArchetypeNodeID
	case *rm.Cluster:
		return v.ArchetypeNodeID
	case rm.Cluster:
		return v.ArchetypeNodeID
	case *rm.Element:
		return v.ArchetypeNodeID
	case rm.Element:
		return v.ArchetypeNodeID
	}
	return ""
}

// nameValueOf returns the name/value string of a LOCATABLE child, or "".
func nameValueOf(o any) string {
	switch v := o.(type) {
	case *rm.Composition:
		return rm.DVTextValueOf(v.Name)
	case rm.Composition:
		return rm.DVTextValueOf(v.Name)
	case *rm.Section:
		return rm.DVTextValueOf(v.Name)
	case rm.Section:
		return rm.DVTextValueOf(v.Name)
	case *rm.Observation:
		return rm.DVTextValueOf(v.Name)
	case rm.Observation:
		return rm.DVTextValueOf(v.Name)
	case *rm.Evaluation:
		return rm.DVTextValueOf(v.Name)
	case rm.Evaluation:
		return rm.DVTextValueOf(v.Name)
	case *rm.Instruction:
		return rm.DVTextValueOf(v.Name)
	case rm.Instruction:
		return rm.DVTextValueOf(v.Name)
	case *rm.Action:
		return rm.DVTextValueOf(v.Name)
	case rm.Action:
		return rm.DVTextValueOf(v.Name)
	case *rm.AdminEntry:
		return rm.DVTextValueOf(v.Name)
	case rm.AdminEntry:
		return rm.DVTextValueOf(v.Name)
	case *rm.GenericEntry:
		return rm.DVTextValueOf(v.Name)
	case rm.GenericEntry:
		return rm.DVTextValueOf(v.Name)
	case *rm.Activity:
		return rm.DVTextValueOf(v.Name)
	case rm.Activity:
		return rm.DVTextValueOf(v.Name)
	case *rm.History[rm.ItemStructure]:
		return rm.DVTextValueOf(v.Name)
	case rm.History[rm.ItemStructure]:
		return rm.DVTextValueOf(v.Name)
	case *rm.PointEvent[rm.ItemStructure]:
		return rm.DVTextValueOf(v.Name)
	case rm.PointEvent[rm.ItemStructure]:
		return rm.DVTextValueOf(v.Name)
	case *rm.IntervalEvent[rm.ItemStructure]:
		return rm.DVTextValueOf(v.Name)
	case rm.IntervalEvent[rm.ItemStructure]:
		return rm.DVTextValueOf(v.Name)
	case *rm.ItemTree:
		return rm.DVTextValueOf(v.Name)
	case rm.ItemTree:
		return rm.DVTextValueOf(v.Name)
	case *rm.ItemList:
		return rm.DVTextValueOf(v.Name)
	case rm.ItemList:
		return rm.DVTextValueOf(v.Name)
	case *rm.ItemSingle:
		return rm.DVTextValueOf(v.Name)
	case rm.ItemSingle:
		return rm.DVTextValueOf(v.Name)
	case *rm.ItemTable:
		return rm.DVTextValueOf(v.Name)
	case rm.ItemTable:
		return rm.DVTextValueOf(v.Name)
	case *rm.Cluster:
		return rm.DVTextValueOf(v.Name)
	case rm.Cluster:
		return rm.DVTextValueOf(v.Name)
	case *rm.Element:
		return rm.DVTextValueOf(v.Name)
	case rm.Element:
		return rm.DVTextValueOf(v.Name)
	}
	return ""
}
