package rmpath

import "github.com/cadasto/openehr-sdk-go/openehr/rm"

// childrenAt returns the child object(s) at attribute attr on parent,
// flattened to a slice (0, 1, or many). Dispatch is on the concrete RM
// type — reflection-free (REQ-024). An unknown (type, attr) pair yields
// no children, so the path simply fails to resolve rather than panicking.
//
// A typed-nil pointer parent (including a typed-nil root, e.g. a
// (*rm.Composition)(nil) passed to ItemAtPath) yields no children rather
// than a nil-receiver dereference — upholding the no-panic contract.
func childrenAt(parent any, attr string) []any {
	if isNilPointer(parent) {
		return nil
	}
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

// iface wraps a present interface value as a single child. A genuinely
// nil interface and a typed-nil pointer (e.g. a (*rm.Observation)(nil)
// boxed in a ContentItem) both yield no child, so the walker never
// dereferences a nil receiver — upholding the no-panic contract.
func iface(v any) []any {
	if v == nil || isNilPointer(v) {
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
		return ifaceSlice(h.Events)
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
// skipping genuinely-nil entries and typed-nil pointers (so the walker
// never dereferences a nil receiver).
func ifaceSlice[T any](s []T) []any {
	out := make([]any, 0, len(s))
	for _, v := range s {
		if av := any(v); av != nil && !isNilPointer(av) {
			out = append(out, av)
		}
	}
	return out
}

// isNilPointer reports whether v is a typed-nil pointer boxed in an
// interface (e.g. (*rm.Observation)(nil) stored as a ContentItem, or a
// typed-nil root) — itself non-nil, so without this guard childrenAt /
// nodeIDOf / nameValueOf would dereference it. Delegates to the
// generated rm.IsTypedNil (ADR 0013), which covers every registered RM
// concrete; the walker's previous hand-written switch covered only the
// 18 types it dispatches on, so this is a strict superset with
// identical semantics on all reachable values. Reflection-free
// (REQ-024).
func isNilPointer(v any) bool {
	return rm.IsTypedNil(v)
}

// nodeIDOf returns the archetype_node_id of a LOCATABLE child, or "".
// Reads polymorphically through the generated rm.Locatable identity
// surface (ADR 0013); the isNilPointer guard MUST stay ahead of the
// assertion — a getter invoked on a typed-nil pointer panics.
func nodeIDOf(o any) string {
	if isNilPointer(o) {
		return ""
	}
	if l, ok := o.(rm.Locatable); ok {
		return l.GetArchetypeNodeID()
	}
	return ""
}

// nameValueOf returns the name/value string of a LOCATABLE child, or "".
// Same guard-then-assert shape as nodeIDOf; rm.DVTextValueOf handles a
// nil or typed-nil name (partially built nodes).
func nameValueOf(o any) string {
	if isNilPointer(o) {
		return ""
	}
	if l, ok := o.(rm.Locatable); ok {
		return rm.DVTextValueOf(l.GetName())
	}
	return ""
}
