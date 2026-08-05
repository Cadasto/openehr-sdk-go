package simplified

// REQ-140 — underscore-prefixed RM attributes, encode side.
//
// [rmattrEncode] is the post-node hook: after a node's own emission, it
// inspects the owner's RM type and writes every populated in-scope `_` key
// under that node's FLAT path. It is called for the composition root, for every
// resolved LOCATABLE node the Web Template walk reaches, for the ELEMENT a
// collapsed leaf hides, and for the composition's EVENT_CONTEXT.
//
// The resolution-first / leaf-second architecture is untouched: this hook runs
// *beside* the leaf mapping, never in front of the rmpath resolution that feeds
// it. What it adds is the one thing the walk cannot express — attributes with no
// Web Template node of their own.
//
// Nothing populated is ever dropped (REQ-053 fail-loud, REQ-140 § Behavioural
// rules). Where the grammar has no channel for a value — a coded LINK.meaning, a
// decorated `_end_time`, an OBJECT_REF whose id is neither GENERIC_ID nor
// HIER_OBJECT_ID — the result is a typed error, because none of these families
// has a `|raw` carrier to fall back to.

import (
	"fmt"
	"strconv"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// rmattrEncode writes owner's underscore-prefixed RM attributes under the FLAT
// path base. A nil or typed-nil owner writes nothing.
//
// An owner the underscore grammar does not model is a typed error rather than a
// silent no-op: every call site passes a composition-tree LOCATABLE or the
// EVENT_CONTEXT, so a miss means a new RM shape reached the walk and its
// attributes would go unwritten.
func rmattrEncode(owner any, base string, out map[string]any) error {
	if owner == nil || rm.IsTypedNil(owner) {
		return nil
	}
	if ec, ok := as[rm.EventContext](owner); ok {
		return eventContextRMAttrs(out, base, ec)
	}
	links, modelled := rmattrLinksOf(owner)
	if !modelled {
		return fmt.Errorf("%w: %q owns a %T, which the underscore RM attribute grammar does not model (REQ-140)",
			ErrUnsupportedDatatype, base, owner)
	}
	if loc, ok := owner.(rm.Locatable); ok {
		if err := uidRMAttr(out, base, loc.GetUID()); err != nil {
			return err
		}
	}
	if err := linksRMAttr(out, base, links); err != nil {
		return err
	}
	if err := objectRefRMAttr(out, base+"/_work_flow_id", entryWorkflowIDOf(owner)); err != nil {
		return err
	}
	return objectRefRMAttr(out, base+"/_guideline_id", careEntryGuidelineIDOf(owner))
}

// rmattrLinksOf returns the LINK list carried by a LOCATABLE, and whether owner
// is a LOCATABLE the underscore grammar models at all.
//
// `uid` rides the generated rm.Locatable.GetUID accessor (ADR 0013), but
// LOCATABLE.links has none, and reflection is forbidden (REQ-024) — so the
// composition-tree LOCATABLE set is enumerated here. Demographic LOCATABLEs
// (PERSON, ROLE, PARTY_IDENTITY, …) are deliberately absent: no COMPOSITION
// attribute reaches one, and admitting a type the walk cannot produce would
// weaken the fail-loud report [rmattrEncode] makes of a miss.
func rmattrLinksOf(owner any) ([]rm.Link, bool) {
	if v, ok := as[rm.Composition](owner); ok {
		return v.Links, true
	}
	if v, ok := as[rm.Section](owner); ok {
		return v.Links, true
	}
	if v, ok := as[rm.Observation](owner); ok {
		return v.Links, true
	}
	if v, ok := as[rm.Evaluation](owner); ok {
		return v.Links, true
	}
	if v, ok := as[rm.Instruction](owner); ok {
		return v.Links, true
	}
	if v, ok := as[rm.Action](owner); ok {
		return v.Links, true
	}
	if v, ok := as[rm.AdminEntry](owner); ok {
		return v.Links, true
	}
	if v, ok := as[rm.GenericEntry](owner); ok {
		return v.Links, true
	}
	if v, ok := as[rm.Activity](owner); ok {
		return v.Links, true
	}
	if v, ok := as[rm.Cluster](owner); ok {
		return v.Links, true
	}
	if v, ok := as[rm.Element](owner); ok {
		return v.Links, true
	}
	if v, ok := as[rm.ItemTree](owner); ok {
		return v.Links, true
	}
	if v, ok := as[rm.ItemList](owner); ok {
		return v.Links, true
	}
	if v, ok := as[rm.ItemTable](owner); ok {
		return v.Links, true
	}
	if v, ok := as[rm.ItemSingle](owner); ok {
		return v.Links, true
	}
	if v, ok := as[rm.History[rm.ItemStructure]](owner); ok {
		return v.Links, true
	}
	if v, ok := as[rm.PointEvent[rm.ItemStructure]](owner); ok {
		return v.Links, true
	}
	if v, ok := as[rm.IntervalEvent[rm.ItemStructure]](owner); ok {
		return v.Links, true
	}
	return nil, false
}

// entryWorkflowIDOf returns ENTRY.workflow_id, or nil when owner is not an
// ENTRY. All five ENTRY subtypes declare it; GENERIC_ENTRY does not (it is a
// CONTENT_ITEM, not an ENTRY), and nothing above ENTRY does — which is what
// makes the decode-side rminfo check refuse `_work_flow_id` on a SECTION, a
// CLUSTER or an ELEMENT.
func entryWorkflowIDOf(owner any) rm.ObjectRefLike {
	if v, ok := as[rm.Observation](owner); ok {
		return v.WorkflowID
	}
	if v, ok := as[rm.Evaluation](owner); ok {
		return v.WorkflowID
	}
	if v, ok := as[rm.Instruction](owner); ok {
		return v.WorkflowID
	}
	if v, ok := as[rm.Action](owner); ok {
		return v.WorkflowID
	}
	if v, ok := as[rm.AdminEntry](owner); ok {
		return v.WorkflowID
	}
	return nil
}

// careEntryGuidelineIDOf returns CARE_ENTRY.guideline_id, or nil when owner is
// not a CARE_ENTRY.
//
// The owner set is **narrower** than `workflow_id`'s: `guideline_id` is declared
// on CARE_ENTRY, and ADMIN_ENTRY is an ENTRY but not a CARE_ENTRY, so it has no
// `_guideline_id` at all. wire.md § REQ-140's table says "ENTRY subtypes" for
// both, which is the coarse reading; the RM is the authority for which class
// declares what, and the corpus agrees —
// `ehrbase_conformance_admin_entry.json` carries `_work_flow_id|*` and no
// `_guideline_id`, while `action` / `evaluation` / `instruction` / `observation`
// carry both.
func careEntryGuidelineIDOf(owner any) rm.ObjectRefLike {
	if v, ok := as[rm.Observation](owner); ok {
		return v.GuidelineID
	}
	if v, ok := as[rm.Evaluation](owner); ok {
		return v.GuidelineID
	}
	if v, ok := as[rm.Instruction](owner); ok {
		return v.GuidelineID
	}
	if v, ok := as[rm.Action](owner); ok {
		return v.GuidelineID
	}
	return nil
}

// uidRMAttr writes `<base>/_uid`.
//
// The key is a bare string, so the concrete UID_BASED_ID subtype has no channel
// of its own and decode re-derives it from the lexical form ([uidBasedIDType]).
// A value whose form implies the *other* subtype would therefore come back
// retyped — the codec refuses rather than emitting a payload that does not
// round-trip. An all-zero id writes nothing, the same reason
// [codePhraseToFlat] skips an empty code_string: there is no value to carry, and
// an unconditional emit would put a blank `_uid` on every composition.
func uidRMAttr(out map[string]any, base string, uid rm.UIDBasedID) error {
	var value, kind string
	switch u := uid.(type) {
	case nil:
		return nil
	case rm.HierObjectID:
		value, kind = u.Value, "HIER_OBJECT_ID"
	case *rm.HierObjectID:
		if u == nil {
			return nil
		}
		value, kind = u.Value, "HIER_OBJECT_ID"
	case rm.ObjectVersionID:
		value, kind = u.Value, "OBJECT_VERSION_ID"
	case *rm.ObjectVersionID:
		if u == nil {
			return nil
		}
		value, kind = u.Value, "OBJECT_VERSION_ID"
	default:
		return fmt.Errorf("%w: %q cannot carry a %T", ErrUnsupportedDatatype, base+"/_uid", uid)
	}
	if value == "" {
		return nil
	}
	if implied := uidBasedIDType(value); implied != kind {
		return fmt.Errorf("%w: %q carries a %s whose value %q is the lexical form of a %s; the bare `_uid` key cannot distinguish them, so decode would retype it",
			ErrUnsupportedDatatype, base+"/_uid", kind, value, implied)
	}
	out[base+"/_uid"] = value
	return nil
}

// linksRMAttr writes `<base>/_link:N|meaning`, `|type` and `|target` for each
// LINK, indexed by list position. All three attributes are RM-mandatory, so a
// populated list always contributes the full triple.
func linksRMAttr(out map[string]any, base string, links []rm.Link) error {
	for i, l := range links {
		path := base + "/_link:" + strconv.Itoa(i)
		meaning, err := linkTextRMAttr(path+"|meaning", l.Meaning)
		if err != nil {
			return err
		}
		typ, err := linkTextRMAttr(path+"|type", l.Type)
		if err != nil {
			return err
		}
		out[path+"|meaning"] = meaning
		out[path+"|type"] = typ
		out[path+"|target"] = l.Target.Value
	}
	return nil
}

// linkTextRMAttr reduces a LINK.meaning / .type to the plain string its suffix
// carries. The suffix has no channel for a coded term or a DV_TEXT decoration
// (`formatting`, `hyperlink`, `language`, `encoding`, `mappings`) and LINK has no
// `|raw` carrier anywhere in the grammar, so such a value is a typed error
// rather than a silent narrowing to its rubric.
func linkTextRMAttr(key string, v rm.DVTextLike) (string, error) {
	if v == nil || rm.IsTypedNil(v) {
		return "", fmt.Errorf("%w: %q is RM-mandatory on LINK but absent", ErrUnsupportedDatatype, key)
	}
	dv, plain := as[rm.DVText](v)
	if !plain {
		return "", fmt.Errorf("%w: %q cannot carry a %T; the LINK suffixes take a plain DV_TEXT", ErrUnsupportedDatatype, key, v)
	}
	if dv.Formatting != nil || dv.Hyperlink != nil || dv.Language != nil || dv.Encoding != nil || len(dv.Mappings) > 0 {
		return "", fmt.Errorf("%w: %q cannot carry a decorated DV_TEXT (formatting, hyperlink, language, encoding or mappings)", ErrUnsupportedDatatype, key)
	}
	return dv.Value, nil
}

// objectRefRMAttr writes an OBJECT_REF family (`_work_flow_id`,
// `_guideline_id`) as `|id`, the optional `|id_scheme`, `|namespace` and
// `|type` — the corpus's spelling, which uses `|namespace` where the party
// grammar uses `|id_namespace`.
//
// `|id_scheme` is the OBJECT_ID discriminator both ways: present for a
// GENERIC_ID (it carries the scheme), absent for a HIER_OBJECT_ID. Every other
// OBJECT_ID subtype is scheme-less and would therefore be indistinguishable
// from HIER_OBJECT_ID on the wire, so it is refused instead of being retyped by
// the round-trip. An OBJECT_REF *subtype* (PARTY_REF, LOCATABLE_REF,
// ACCESS_GROUP_REF) is refused on the same grounds — the suffix set carries no
// `_type`.
func objectRefRMAttr(out map[string]any, path string, ref rm.ObjectRefLike) error {
	if ref == nil || rm.IsTypedNil(ref) {
		return nil
	}
	o, plain := as[rm.ObjectRef](ref)
	if !plain {
		return fmt.Errorf("%w: %q cannot carry a %T; the suffix set carries no subtype discriminator", ErrUnsupportedDatatype, path, ref)
	}
	switch id := o.ID.(type) {
	case nil:
		return fmt.Errorf("%w: %q is RM-mandatory on OBJECT_REF but absent", ErrUnsupportedDatatype, path+"|id")
	case rm.GenericID:
		out[path+"|id"] = id.Value
		out[path+"|id_scheme"] = id.Scheme
	case *rm.GenericID:
		out[path+"|id"] = id.Value
		out[path+"|id_scheme"] = id.Scheme
	case rm.HierObjectID:
		out[path+"|id"] = id.Value
	case *rm.HierObjectID:
		out[path+"|id"] = id.Value
	default:
		return fmt.Errorf("%w: %q cannot carry a %T; only GENERIC_ID (with `|id_scheme`) and HIER_OBJECT_ID (without) are distinguishable in this suffix set",
			ErrUnsupportedDatatype, path+"|id", o.ID)
	}
	out[path+"|namespace"] = o.Namespace
	out[path+"|type"] = o.Type
	return nil
}

// eventContextRMAttrs writes the EVENT_CONTEXT optionals this phase carries —
// `_end_time` and `_location` — under the real `context` segment (ADR 0016; the
// ITS `ctx/` sketches for them are deliberately not implemented). Both fields
// are pointers, so nil is genuine absence and there is no zero/absent ambiguity
// of the kind EVENT_CONTEXT.setting has.
func eventContextRMAttrs(out map[string]any, base string, ec rm.EventContext) error {
	if ec.EndTime != nil {
		if err := endTimeRMAttr(out, base, *ec.EndTime); err != nil {
			return err
		}
	}
	if ec.Location != nil {
		out[base+"/_location"] = *ec.Location
	}
	return nil
}

// endTimeRMAttr writes `<base>/_end_time`. The key is a bare value, so a
// decorated DV_DATE_TIME (a magnitude_status, an accuracy, a normal status or
// range) has no carrier — and unlike a template-constrained leaf there is no
// `|raw` to fall back on — so it is a typed error. An empty value writes
// nothing, matching [uidRMAttr].
func endTimeRMAttr(out map[string]any, base string, t rm.DVDateTime) error {
	if t.MagnitudeStatus != nil || t.Accuracy != nil || t.NormalStatus != nil ||
		t.NormalRange != nil || len(t.OtherReferenceRanges) > 0 {
		return fmt.Errorf("%w: %q is a bare value and cannot carry a decorated DV_DATE_TIME (magnitude_status, accuracy, normal status or range)",
			ErrUnsupportedDatatype, base+"/_end_time")
	}
	if t.Value == "" {
		return nil
	}
	out[base+"/_end_time"] = t.Value
	return nil
}
