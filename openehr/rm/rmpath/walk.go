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
//
// # Coverage and the silent-drop hazard
//
// "Unknown attribute" and "attribute present but empty" are indistinguishable
// to callers: both yield no children, and [ItemAtPath] reports
// [ErrPathNotFound] for each. A caller that treats that as "absent optional"
// — as the FLAT encoder's skipNotFound does — will therefore *silently drop*
// data for any attribute missing from these switches.
//
// PROBE-086 caught exactly that: EVENT `time`, INSTRUCTION `narrative` /
// `expiry_time`, and — worse, because it carries template-constrained clinical
// data — every EVENT_CONTEXT attribute were absent here, so upstream
// compositions round-tripped through the FLAT codec with those values
// discarded and no error. `webtemplate.TestInContextLeavesResolveViaRmpath`
// guards the class by asserting that every synthesized in-context leaf the
// WebTemplate can emit is resolvable here unless deliberately exempted.
//
// Resolving an attribute here does not by itself make the FLAT encoder fail:
// leafToFlat skips the leaf datatypes it does not map (PARTY_PROXY, STRING — a
// documented skip, see openehr/serialize/simplified/deviations.md) and emits
// unmapped DV_* datatypes as |raw. "The codec cannot write it" is therefore a
// reason for a leaf to stay unemitted, never a reason for an attribute to stay
// unresolvable. CODE_PHRASE was in that skip list until the PROBE-086 coverage
// ratchet gave it a |code + |terminology leaf mapping, which is why ENTRY
// `language` / `encoding` resolve here now.
//
// The attributes deliberately still absent are EVENT_CONTEXT `start_time` /
// `setting` and COMPOSITION `language` / `territory`. All four are owned by the
// ctx/ short-form spelling on encode — ctx/time, ctx/setting, ctx/language,
// ctx/territory — so resolving them here would double-spell the value, and
// ADR 0015 made that permanent: encode emits the ctx/ short forms only (decode
// accepts both spellings). ADR 0015 settled the *spelling* without clearing
// `ctx/setting` emission, which is still deferred, so a non-default
// EVENT_CONTEXT `setting` is dropped on encode — recorded in
// simplified/deviations.md. Resolving `setting` on a conformant WithTemplate
// decode would emit the decoder's synthesized default (`238|other care`, see
// flat_decode.go completeRequired / defaultAttr) rather than zeros; a
// zero-valued leaf is what a template-less decode or a hand-built RM value
// would yield.
//
// COMPOSITION `composer` does resolve (compositionChildren below) — the FLAT
// encoder simply declines to write a PARTY_PROXY. Of the REQ-121 completeness
// set, only ENTRY `subject` and ACTIVITY `action_archetype_id` are not written
// yet.
func childrenAt(parent any, attr string) []any {
	if isNilPointer(parent) {
		return nil
	}
	switch p := parent.(type) {
	case *rm.Composition:
		return compositionChildren(p, attr)
	case rm.Composition:
		return compositionChildren(&p, attr)

	case *rm.EventContext:
		return eventContextChildren(p, attr)
	case rm.EventContext:
		return eventContextChildren(&p, attr)

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

// eventContextChildren resolves the EVENT_CONTEXT attributes reachable from
// COMPOSITION `context`. `other_context` matters most: it is the only
// template-constrained one, so before this existed every
// /context/other_context/… path was unresolvable and the FLAT encoder dropped
// the data. `start_time` and `setting` are deliberately absent — see the
// hazard note on childrenAt.
func eventContextChildren(c *rm.EventContext, attr string) []any {
	switch attr {
	case "other_context":
		return iface(c.OtherContext)
	case "end_time":
		return iface(c.EndTime)
	case "health_care_facility":
		return iface(c.HealthCareFacility)
	case "location":
		if c.Location == nil {
			return nil
		}
		return []any{c.Location}
	case "participations":
		out := make([]any, 0, len(c.Participations))
		for k := range c.Participations {
			out = append(out, &c.Participations[k])
		}
		return out
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

// entryChildren resolves the two in-context attributes this helper covers of
// those the five ENTRY subtypes share: `language` and `encoding` (REQ-121).
// `subject` is a third such shared attribute the Web Template emits, but it
// stays unresolved — see the hazard note on childrenAt. Delegated to from
// OBSERVATION / EVALUATION / INSTRUCTION / ACTION / ADMIN_ENTRY; GENERIC_ENTRY
// is deliberately not among them — it descends from CONTENT_ITEM, not ENTRY,
// and carries neither attribute.
//
// Both are non-pointer CODE_PHRASE fields, so they are resolved *by value* —
// the convention here for non-pointer attributes (cf. ACTION `time`,
// COMPOSITION `category`) — a child is therefore always returned, and "unset"
// reaches the caller as a zero CODE_PHRASE. The FLAT encoder's leaf mapping is
// what declines to write an empty code (see simplified.codePhraseToFlat).
// Resolution and writability are separate concerns; conflating them is what
// previously hid an encode-side drop.
func entryChildren(language, encoding rm.CodePhrase, attr string) []any {
	switch attr {
	case "language":
		return []any{language}
	case "encoding":
		return []any{encoding}
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
	return entryChildren(o.Language, o.Encoding, attr)
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
	return entryChildren(e.Language, e.Encoding, attr)
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
	case "narrative":
		return iface(i.Narrative)
	case "expiry_time":
		if i.ExpiryTime == nil {
			return nil
		}
		return []any{i.ExpiryTime}
	}
	return entryChildren(i.Language, i.Encoding, attr)
}

func actionChildren(a *rm.Action, attr string) []any {
	switch attr {
	case "description":
		return iface(a.Description)
	case "protocol":
		return iface(a.Protocol)
	case "name":
		return iface(a.Name)
	case "time":
		// Inert on encode today — the WebTemplate builder synthesizes no
		// ACTION `time` leaf — but ItemAtPath is a public reader (REQ-121).
		return []any{a.Time}
	}
	return entryChildren(a.Language, a.Encoding, attr)
}

func adminEntryChildren(a *rm.AdminEntry, attr string) []any {
	switch attr {
	case "data":
		return iface(a.Data)
	case "name":
		return iface(a.Name)
	}
	return entryChildren(a.Language, a.Encoding, attr)
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
	case "timing":
		if a.Timing == nil {
			return nil
		}
		return []any{a.Timing}
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
	case "time":
		return []any{e.Time}
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
	case "time":
		return []any{e.Time}
	case "math_function":
		return []any{e.MathFunction}
	case "width":
		return []any{e.Width}
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
