package validation

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
)

// walkNode is the lockstep visitor: it enters the OPT node `optNode`
// bound to the RM value `rmValue`, emits structural issues at this
// node, then descends each OPT-declared attribute into its matched
// RM child(ren). The `path` argument is the OPT-authoritative AQL
// path of `optNode` (root = "/").
//
// Per-node behaviour:
//   - Identity (LOCATABLE.archetype_node_id ↔ OPT pin) and RM-type
//     match (with BMM abstract-supertype admission) fire at every
//     node.
//   - Slot leaves: no descent (slot-fit was decided by the parent
//     attribute when binding RM items to OPT children).
//   - Primitive constraint leaves (PrimitiveConstraint() != nil):
//     the REQ-103 typed validator runs against the bound RM value;
//     no descent into implicit RM-mandatory attrs of the
//     primitive's RM type.
//   - Otherwise: iterate every attribute (explicit OPT-declared
//     and BMM-mandatory implicits), enforce existence + cardinality,
//     match RM child(ren), recurse.
func (w *walker) walkNode(optNode *templatecompile.CompiledNode, rmValue any, path string) {
	if optNode == nil || rmValue == nil {
		return
	}
	// Defence-in-depth typed-nil guard. The matchers (matchChildByID
	// for multi-valued attrs, matchSingleAlternative for single)
	// already reject typed-nil before descent; ifacePresent /
	// readItemSingleSingle reject it at the rmread layer. This
	// belt-and-suspenders check costs one type-switch and prevents
	// any future descent path from re-introducing the panic class
	// the v2 reviewers caught twice (Element.Value, then slice
	// elements).
	if rmread.IsTypedNilPointer(rmValue) {
		return
	}

	// Identity + RM-type checks at this node. At the composition
	// root, identity is checked inline against COMPOSITION's
	// archetype_node_id (not via a separate attribute descent).
	w.checkLocatableIdentity(optNode, rmValue, path)
	// AOM 1.4 primitive short-name leaf reached via a DV wrapper's
	// .value string channel (clinical_note.opt shape). Validate the
	// string against the REQ-103 constraint without an RM-type check.
	if pc := optNode.PrimitiveConstraint(); pc != nil && templatecompile.IsAOMPrimitiveShortName(optNode.RMTypeName()) {
		if _, ok := rmValue.(string); ok {
			w.applyPrimitive(optNode, rmValue, path, pc)
			return
		}
	}
	w.checkRMType(optNode, rmValue, path)

	if optNode.IsSlot() {
		// Slot leaves carry no descendable structure — slot-fill
		// matching is the parent attribute's responsibility (see
		// walkMultipleAttribute).
		return
	}
	if pc := optNode.PrimitiveConstraint(); pc != nil {
		// REQ-103 primitive constraint leaf. Convert the RM
		// DataValue (or RM-typed primitive Go value) to the
		// constraint's expected input via dataValueInput, then
		// fan Violations out as Issues. The structural walker
		// MUST NOT descend further: implicit RM attrs of the
		// primitive's RM type (e.g. DV_QUANTITY.magnitude / .units)
		// are the primitive validator's territory.
		w.applyPrimitive(optNode, rmValue, path, pc)
		return
	}

	for _, attr := range optNode.Attributes() {
		// Implicit (BMM-mandatory, OPT-silent) attributes have no
		// children — the OPT did not pin a structural constraint
		// for them. We still run the existence check so a
		// BMM-mandatory attribute (e.g. COMPOSITION.composer,
		// /language, /territory) that the composition leaves nil
		// or zero-valued surfaces as `required`. No descent
		// happens because Children() is empty for implicit attrs.
		switch attr.Cardinality() {
		case template.Single:
			w.walkSingleAttribute(optNode, attr, rmValue, path)
		case template.Multiple:
			w.walkMultipleAttribute(optNode, attr, rmValue, path)
		}
	}
}

// walkSingleAttribute enforces existence on a C_SINGLE_ATTRIBUTE
// and binds the RM value to one of attr.Children() via the AOM 1.4
// "one alternative MUST match" semantics. Tries each OPT child in
// order; first that fits the RM value's concrete type (with BMM
// abstract-supertype admission) wins. When none match the walker
// emits a typed issue:
//   - exactly one child → `rm_type_mismatch` (plain type constraint,
//     no real alternatives);
//   - two or more children → `alternative_mismatch` with the list
//     of allowed RM types.
func (w *walker) walkSingleAttribute(
	opt *templatecompile.CompiledNode,
	attr *templatecompile.CompiledAttribute,
	parentRM any,
	parentPath string,
) {
	attrPath := joinPath(parentPath, "/"+attr.Name())
	val, ok := rmread.ReadSingle(parentRM, opt.RMTypeName(), attr.Name())
	if !ok {
		if isRequired(attr) {
			w.emit(Issue{
				Path:     attrPath,
				Code:     "required",
				Detail:   fmt.Sprintf("required attribute %q absent on %s", attr.Name(), describeRMType(parentRM)),
				Severity: Error,
			})
		}
		return
	}
	children := attr.Children()
	if len(children) == 0 {
		// No OPT children → no structural constraint; primitive
		// constraints fire when walkNode reaches a primitive leaf.
		return
	}
	child := matchSingleAlternative(children, val)
	if child == nil {
		// With multiple OPT children the OPT declared AnyOf
		// alternatives; with a single child it is a plain type
		// constraint. Disambiguate the Issue.Code so consumers can
		// distinguish "wrong type" from "didn't match any of N
		// allowed types".
		if len(children) == 1 {
			w.emit(Issue{
				Path:     attrPath,
				Code:     "rm_type_mismatch",
				Detail:   fmt.Sprintf("RM value of type %s under %q does not satisfy template RM type %s", describeRMType(val), attr.Name(), children[0].RMTypeName()),
				Severity: Error,
			})
		} else {
			w.emit(Issue{
				Path:     attrPath,
				Code:     "alternative_mismatch",
				Detail:   fmt.Sprintf("RM value of type %s under %q matches none of the OPT alternatives %s", describeRMType(val), attr.Name(), formatAllowedTypes(children)),
				Severity: Error,
			})
		}
		return
	}
	w.walkNode(child, val, joinPath(parentPath, segmentForChild(attr, child, 0)))
}

// matchSingleAlternative picks the OPT child whose declared
// RMTypeName admits the RM value (concrete equality + BMM
// supertype expansion). Returns nil when no child fits. With
// exactly one child the function is effectively "does the child
// fit?"; the alternative_mismatch case fires only when the OPT
// declared more than one alternative.
func matchSingleAlternative(children []*templatecompile.CompiledNode, val any) *templatecompile.CompiledNode {
	gotType := describeRMType(val)
	for _, c := range children {
		want := c.RMTypeName()
		if want == "" {
			// Wildcard / not-typed OPT child — accept.
			return c
		}
		if gotType == want || rmTypeIsSubtypeOf(gotType, want) {
			return c
		}
		// AOM 1.4 primitive short name (DURATION, DATE, …) pinned
		// under a DV wrapper's .value string channel — the RM value
		// is a Go string while the OPT child rm_type_name is the
		// primitive short name.
		if templatecompile.IsAOMPrimitiveShortName(want) && c.PrimitiveConstraint() != nil {
			if _, ok := val.(string); ok {
				return c
			}
		}
	}
	return nil
}

// formatAllowedTypes renders the OPT child RM types for inclusion
// in alternative_mismatch Detail messages.
func formatAllowedTypes(children []*templatecompile.CompiledNode) string {
	names := make([]string, len(children))
	for i, c := range children {
		names[i] = c.RMTypeName()
	}
	return "[" + strings.Join(names, ", ") + "]"
}

// walkMultipleAttribute enforces existence + cardinality on a
// multi-valued attribute and binds each RM item to the OPT child
// whose archetype_node_id matches. Items without a matching OPT
// child surface as slot_fill issues; matching pins exact
// archetype/node ids first, then evaluates the parsed REQ-104 slot
// grammar (see [matchChildByID]).
func (w *walker) walkMultipleAttribute(
	opt *templatecompile.CompiledNode,
	attr *templatecompile.CompiledAttribute,
	parentRM any,
	parentPath string,
) {
	attrPath := joinPath(parentPath, "/"+attr.Name())
	items, ok := rmread.ReadMultiple(parentRM, opt.RMTypeName(), attr.Name())
	if !ok {
		// rmread cannot address the attribute on this RM type. If
		// the OPT pinned existence ≥ 1 (or BMM marks the attribute
		// required), the composition has no way to satisfy the
		// constraint — mirror walkSingleAttribute's `required` emit.
		// Silent skip would let a missing rmread row (or a
		// composition with an unhandled parent RM type) hide a real
		// structural failure.
		if isRequired(attr) {
			w.emit(Issue{
				Path:     attrPath,
				Code:     "required",
				Detail:   fmt.Sprintf("required multi-valued attribute %q absent on %s", attr.Name(), describeRMType(parentRM)),
				Severity: Error,
			})
		}
		return
	}
	// Existence: lower ≥ 1 with zero items is "required".
	if isRequired(attr) && len(items) == 0 {
		w.emit(Issue{
			Path:     attrPath,
			Code:     "required",
			Detail:   fmt.Sprintf("required multi-valued attribute %q is empty on %s", attr.Name(), describeRMType(parentRM)),
			Severity: Error,
		})
	}
	// Child-count cardinality interval (when the OPT pinned one).
	if cm := attr.ChildMultiplicity(); cm != nil {
		if outOfMultiplicityInterval(len(items), cm) {
			w.emit(Issue{
				Path:     attrPath,
				Code:     "cardinality",
				Detail:   fmt.Sprintf("attribute %q has %d children; OPT cardinality %s", attr.Name(), len(items), formatInterval(cm)),
				Severity: Error,
			})
		}
	}
	// Recurse into each matched item. Items without a matching OPT
	// child contribute one slot_fill issue — UNLESS the OPT declared
	// no children for this attribute, in which case the attribute is
	// "open" (any RM item passes; the OPT pinned only existence /
	// cardinality, not membership). Tally per-child occurrences for
	// the AOM 1.4 occurrences upper-bound check.
	children := attr.Children()
	if len(children) == 0 {
		return
	}
	perChildCount := make(map[*templatecompile.CompiledNode]int, len(children))
	for idx, item := range items {
		matched := matchChildByID(children, item)
		segment := segmentForRMItem(attr, item, idx)
		itemPath := joinPath(parentPath, segment)
		if matched == nil {
			w.emit(Issue{
				Path:     itemPath,
				Code:     "slot_fill",
				Detail:   fmt.Sprintf("RM item %s does not match any OPT child of %q (archetype/at-code mismatch)", describeLocatableID(item), attr.Name()),
				Severity: Error,
			})
			continue
		}
		perChildCount[matched]++
		w.walkNode(matched, item, itemPath)
	}
	// AOM 1.4 occurrences upper bound on each OPT child: when the
	// OPT pins a child's `<occurrences>` interval, the count of
	// matching RM items must fall within it. A zero count + lower
	// ≥ 1 surfaces as `cardinality` at the attribute level — that
	// case is already covered by the multi-attribute existence
	// check above when the attribute itself is empty, but we still
	// fire here per-child when the attribute is non-empty and a
	// specific OPT child is missing or over-represented.
	for _, c := range children {
		occ := c.Occurrences()
		if occ == nil {
			continue
		}
		got := perChildCount[c]
		if outOfMultiplicityInterval(got, occ) {
			w.emit(Issue{
				Path:     joinPath(parentPath, segmentForChild(attr, c, 0)),
				Code:     "cardinality",
				Detail:   fmt.Sprintf("OPT child %s appears %d times under %q; occurrences %s", childIdentity(c), got, attr.Name(), formatInterval(occ)),
				Severity: Error,
			})
		}
	}
}

// childIdentity describes an OPT child for inclusion in cardinality
// diagnostics. Prefers the archetype id (for *ArchetypeRoot children)
// then the at-code then the RM type name.
func childIdentity(c *templatecompile.CompiledNode) string {
	if id := c.ArchetypeID(); id != "" {
		return id
	}
	if id := c.NodeID(); id != "" {
		return id
	}
	return c.RMTypeName()
}

// applyPrimitive runs the REQ-103 primitive constraint against the
// RM value bound to `optNode` and emits one Issue per Violation.
// The constraint's expected input is computed via dataValueInput;
// for non-DataValue RM types that map to a primitive (e.g.
// CODE_PHRASE directly under category/defining_code → C_CODE_PHRASE),
// the value is passed through as-is.
func (w *walker) applyPrimitive(
	optNode *templatecompile.CompiledNode,
	rmValue any,
	path string,
	pc constraints.PrimitiveConstraint,
) {
	input := primitiveInput(rmValue)
	for _, v := range pc.Validate(input) {
		w.emit(Issue{
			Path:     path,
			Code:     "primitive_" + string(v.Code),
			Detail:   v.Detail,
			Severity: Error,
		})
	}
}

// primitiveInput converts an RM value into the Go value the typed
// primitive validator expects. Handles three layers:
//
//   - rm.DataValue concretes (DvQuantity, DvCodedText, DvText, …)
//     via dataValueInput — REQ-103 primitive types that bind to
//     ELEMENT.value.
//   - rm.CodePhrase directly (e.g. category/defining_code) — bind
//     to constraints.CodedTermRef.
//   - everything else: pass through unchanged. The typed primitive
//     validator returns CodeWrongType when the input shape does not
//     fit; that is a contract failure on the caller side, not a
//     constraint failure.
func primitiveInput(rmValue any) any {
	if dv, ok := rmValue.(rm.DataValue); ok {
		if input, ok := dataValueInput(dv); ok {
			return input
		}
		return dv
	}
	switch v := rmValue.(type) {
	case rm.CodePhrase:
		return constraints.CodedTermRef{
			Terminology: v.TerminologyID.Value,
			CodeString:  v.CodeString,
		}
	case *rm.CodePhrase:
		if v == nil {
			return constraints.CodedTermRef{}
		}
		return constraints.CodedTermRef{
			Terminology: v.TerminologyID.Value,
			CodeString:  v.CodeString,
		}
	}
	return rmValue
}

// isRequired reports whether the compiled attribute carries an
// existence interval with lower bound ≥ 1 — i.e. the OPT mandates
// presence. Also honours the BMM-mandatory bit (Required()): some
// attributes are mandatory by RM even when the OPT existence
// element is silent (e.g. COMPOSITION.category).
func isRequired(attr *templatecompile.CompiledAttribute) bool {
	if attr.Required() {
		return true
	}
	e := attr.Existence()
	if e == nil {
		return false
	}
	if e.LowerUnbounded() {
		return false
	}
	return e.Lower() >= 1
}

// outOfMultiplicityInterval reports whether `count` falls outside the
// closed interval encoded in `m`. Honours the unbounded flags.
func outOfMultiplicityInterval(count int, m *template.Multiplicity) bool {
	if !m.LowerUnbounded() && count < m.Lower() {
		return true
	}
	if !m.UpperUnbounded() && count > m.Upper() {
		return true
	}
	return false
}

// formatInterval renders a Multiplicity for inclusion in human-
// readable Issue.Detail strings.
func formatInterval(m *template.Multiplicity) string {
	lo := strconv.Itoa(m.Lower())
	if m.LowerUnbounded() {
		lo = "*"
	}
	hi := strconv.Itoa(m.Upper())
	if m.UpperUnbounded() {
		hi = "*"
	}
	return "[" + lo + ".." + hi + "]"
}

// segmentForChild computes the path delta from a single-attribute
// descent. For Single attrs the delta is "/attr"; for Multiple
// attrs (rare on this code path — walkMultipleAttribute uses
// segmentForRMItem instead) we fall back to the child's
// id-or-index segment.
func segmentForChild(attr *templatecompile.CompiledAttribute, child *templatecompile.CompiledNode, idx int) string {
	seg := "/" + attr.Name()
	if attr.Cardinality() != template.Multiple {
		return seg
	}
	if id := child.ArchetypeID(); id != "" {
		return seg + "[" + id + "]"
	}
	if id := child.NodeID(); id != "" {
		return seg + "[" + id + "]"
	}
	return seg + fmt.Sprintf("[@%d]", idx+1)
}

// segmentForRMItem computes the path delta for an RM item under a
// Multiple attribute. Predicate is the RM item's
// archetype_node_id when available; otherwise a 1-based sibling
// index ("@1", "@2", ...).
func segmentForRMItem(attr *templatecompile.CompiledAttribute, item any, idx int) string {
	seg := "/" + attr.Name()
	if id := locatableArchetypeNodeID(item); id != "" {
		return seg + "[" + id + "]"
	}
	return seg + fmt.Sprintf("[@%d]", idx+1)
}

// matchChildByID picks the OPT child whose ArchetypeID (for
// archetype-root pins) or NodeID (for at-code pins) matches the RM
// item's archetype_node_id. Returns nil when none match — caller
// emits slot_fill in that case.
func matchChildByID(children []*templatecompile.CompiledNode, item any) *templatecompile.CompiledNode {
	id := locatableArchetypeNodeID(item)
	if id == "" {
		return nil
	}
	for _, c := range children {
		if c.IsSlot() {
			continue
		}
		if c.ArchetypeID() != "" && c.ArchetypeID() == id {
			return c
		}
		if c.NodeID() != "" && c.NodeID() == id {
			return c
		}
	}
	for _, c := range children {
		if c.IsSlot() && slotFitsArchetypeID(c, id) {
			return c
		}
	}
	return nil
}

// slotFitsArchetypeID checks whether archetypeID satisfies the
// slot's REQ-104 include / exclude rules, including the RM-type-
// prefix fallback when no includes were parsed.
func slotFitsArchetypeID(slot *templatecompile.CompiledNode, archetypeID string) bool {
	return slot.AllowsArchetypeID(archetypeID)
}

// locatableArchetypeNodeID extracts archetype_node_id from any RM
// LOCATABLE value. Returns "" for non-LOCATABLE values (DataValue
// subtypes, PartyProxy, EventContext). Thin wrapper over
// [rmTypeInfo] — adding a new RM type means editing one switch.
func locatableArchetypeNodeID(v any) string {
	_, id, _ := rmTypeInfo(v)
	return id
}

// describeLocatableID renders an RM item identity for diagnostic
// messages. Falls back to the Go type when the value carries no
// archetype_node_id (e.g. DataValue subtypes).
func describeLocatableID(v any) string {
	if id := locatableArchetypeNodeID(v); id != "" {
		return fmt.Sprintf("%s[%s]", describeRMType(v), id)
	}
	return describeRMType(v)
}

// checkLocatableIdentity emits node_id_mismatch / archetype_id_mismatch
// when the RM's archetype_node_id disagrees with the OPT-pinned id
// at this node. Archetype-root nodes compare against ArchetypeID();
// inner nodes (at-code-pinned) compare against NodeID().
func (w *walker) checkLocatableIdentity(opt *templatecompile.CompiledNode, rmValue any, path string) {
	if opt.IsSlot() {
		// Slot fit is by the parsed REQ-104 archetype-id assertion
		// grammar (RM-type-prefix fallback when no includes parsed).
		// The slot's NodeID is the OPT's own at-code for the slot
		// point — it
		// is not expected to match the filling archetype's
		// archetype_node_id, so a direct identity check here would
		// false-positive on every legitimate slot fill.
		return
	}
	id := locatableArchetypeNodeID(rmValue)
	if id == "" {
		return
	}
	if want := opt.ArchetypeID(); want != "" {
		if id != want {
			w.emit(Issue{
				Path:     path + "/archetype_node_id",
				Code:     "archetype_id_mismatch",
				Detail:   fmt.Sprintf("archetype_node_id %q does not match template archetype id %q at %s", id, want, path),
				Severity: Error,
			})
		}
		return
	}
	if want := opt.NodeID(); want != "" && id != want {
		w.emit(Issue{
			Path:     path + "/archetype_node_id",
			Code:     "node_id_mismatch",
			Detail:   fmt.Sprintf("archetype_node_id %q does not match template node_id %q at %s", id, want, path),
			Severity: Error,
		})
	}
}

// checkRMType emits rm_type_mismatch when the concrete RM Go type
// disagrees with the compiled OPT node's RMTypeName. Honours BMM
// abstract supertypes: an OPT slot constrained to ITEM_STRUCTURE
// admits ITEM_TREE / ITEM_LIST / ITEM_SINGLE / ITEM_TABLE, etc.
func (w *walker) checkRMType(opt *templatecompile.CompiledNode, rmValue any, path string) {
	want := opt.RMTypeName()
	if want == "" {
		return
	}
	got := describeRMType(rmValue)
	if got == want {
		return
	}
	if rmTypeIsSubtypeOf(got, want) {
		return
	}
	w.emit(Issue{
		Path:     path,
		Code:     "rm_type_mismatch",
		Detail:   fmt.Sprintf("RM type %s does not satisfy template RM type %s at %s", got, want, path),
		Severity: Error,
	})
}

// rmTypeIsSubtypeOf encodes the BMM supertype relations the
// validator exercises. Restricted to the abstract slots the OPT
// can name (LOCATABLE, ITEM, ITEM_STRUCTURE, DATA_VALUE, EVENT,
// CONTENT_ITEM, ENTRY, CARE_ENTRY, PARTY_PROXY); concrete
// subtypes admitted under each.
func rmTypeIsSubtypeOf(concrete, abstract string) bool {
	subtypes := bmmSubtypes[abstract]
	return slices.Contains(subtypes, concrete)
}

// bmmSubtypes is the closed lookup of abstract → concrete RM type
// admission rules used by checkRMType. Sourced from
// openehr_rm_1.2.0.bmm: concrete classes that satisfy each
// abstract slot. Entries are limited to the RM types the rest of
// the validator routes — describeRMType, locatableArchetypeNodeID,
// and the rmread table. Adding an abstract→concrete row here
// without the corresponding routing rows would surface as a false
// rm_type_mismatch (the walker would not recognise the concrete);
// the inverse — concretes whose abstract slot is missing — would
// surface as the same false positive on a polymorphic OPT slot.
// Extend in lock-step.
//
// Out of scope for v2: DV_INTERVAL / DV_PARSABLE / DV_MULTIMEDIA /
// DV_PROPORTION / DV_SCALE / DV_STATE / time-specifications (DataValue
// subtypes outside the closed REQ-103 primitive set). Add when an OPT
// surfaces a real consumer for them.
//
// REQ-110 added the demographic PARTY hierarchy (+ sub-components) and
// the EHR-IM roots FOLDER / EHR_STATUS so non-COMPOSITION OPTs validate
// through the same walker.
var bmmSubtypes = map[string][]string{
	"LOCATABLE": {
		"COMPOSITION", "OBSERVATION", "EVALUATION", "INSTRUCTION", "ACTION",
		"ADMIN_ENTRY", "GENERIC_ENTRY", "SECTION", "ACTIVITY",
		"HISTORY", "POINT_EVENT", "INTERVAL_EVENT",
		"ITEM_TREE", "ITEM_LIST", "ITEM_SINGLE", "ITEM_TABLE",
		"CLUSTER", "ELEMENT",
		// REQ-110: demographic + EHR-IM LOCATABLE concretes.
		"PERSON", "ORGANISATION", "GROUP", "AGENT", "ROLE",
		"ADDRESS", "CONTACT", "PARTY_IDENTITY", "PARTY_RELATIONSHIP", "CAPABILITY",
		"FOLDER", "EHR_STATUS",
	},
	// PARTY hierarchy (org.openehr.rm.demographic): PARTY is the common
	// ancestor; ACTOR adds the real-world-entity subtypes (PERSON,
	// ORGANISATION, GROUP, AGENT). ROLE is a PARTY but not an ACTOR.
	"PARTY": {
		"PERSON", "ORGANISATION", "GROUP", "AGENT", "ROLE",
	},
	"ACTOR": {
		"PERSON", "ORGANISATION", "GROUP", "AGENT",
	},
	"CONTENT_ITEM": {
		"OBSERVATION", "EVALUATION", "INSTRUCTION", "ACTION",
		"ADMIN_ENTRY", "GENERIC_ENTRY", "SECTION",
	},
	"ENTRY": {
		"OBSERVATION", "EVALUATION", "INSTRUCTION", "ACTION", "ADMIN_ENTRY",
	},
	"CARE_ENTRY": {
		"OBSERVATION", "EVALUATION", "INSTRUCTION", "ACTION",
	},
	"ITEM_STRUCTURE": {
		"ITEM_TREE", "ITEM_LIST", "ITEM_SINGLE", "ITEM_TABLE",
	},
	"ITEM": {
		"CLUSTER", "ELEMENT",
	},
	"EVENT": {
		"POINT_EVENT", "INTERVAL_EVENT",
	},
	"PARTY_PROXY": {
		"PARTY_SELF", "PARTY_IDENTIFIED", "PARTY_RELATED",
	},
	"DATA_VALUE": {
		"DV_TEXT", "DV_CODED_TEXT", "DV_QUANTITY", "DV_COUNT",
		"DV_BOOLEAN", "DV_ORDINAL", "DV_DATE", "DV_TIME",
		"DV_DATE_TIME", "DV_DURATION",
		"DV_IDENTIFIER", "DV_URI", "DV_EHR_URI",
	},
	// AOM 1.4 primitive short names (used under C_PRIMITIVE_OBJECT)
	// admit the canonical DV wrapper carrying the primitive value.
	// Lockstep with instance.concreteFor — surfaced by clinical_note.opt
	// where DURATION appears as the rm_type_name of a primitive-
	// constrained ELEMENT.value child.
	"DURATION":  {"DV_DURATION"},
	"DATE":      {"DV_DATE"},
	"TIME":      {"DV_TIME"},
	"DATE_TIME": {"DV_DATE_TIME"},
	"BOOLEAN":   {"DV_BOOLEAN"},
}
