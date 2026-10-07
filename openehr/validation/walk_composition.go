package validation

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
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
func (w *walker) walkNode(optNode *tcimpl.CompiledNode, rmValue any, path string) {
	if optNode == nil || rmValue == nil {
		return
	}
	// Defence-in-depth typed-nil guard. The matchers (matchChildByID
	// and matchChildByRMType for multi-valued attrs,
	// matchSingleAlternative for single) already reject typed-nil
	// before descent; ifacePresent /
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
	// AOM 1.4 primitive short-name leaf on a BMM scalar channel
	// (e.g. DV_COUNT.magnitude ← INTEGER, DV_DURATION.value ← string).
	if pc := optNode.PrimitiveConstraint(); pc != nil && tcimpl.IsAOMPrimitiveShortName(optNode.RMTypeName()) {
		if primitiveValueMatchesShortName(optNode.RMTypeName(), rmValue) {
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
		if rminfo.IsNonStorableAttr(bmmtype.Class(optNode.RMTypeName()), attr.Name()) {
			continue
		}
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
	opt *tcimpl.CompiledNode,
	attr *tcimpl.CompiledAttribute,
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
	if child != nil && child.IsSlot() {
		archetypeID := locatableArchetypeNodeID(val)
		if archetypeID == "" || !child.AllowsArchetypeID(archetypeID) {
			w.emit(Issue{
				Path:     attrPath,
				Code:     "slot_fill",
				Detail:   fmt.Sprintf("RM value %s under %q does not satisfy slot %s", describeLocatableID(val), attr.Name(), child.NodeID()),
				Severity: Error,
			})
			return
		}
	}
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
func matchSingleAlternative(children []*tcimpl.CompiledNode, val any) *tcimpl.CompiledNode {
	gotType := describeRMType(val)
	for _, c := range children {
		if admitsRMValue(c, gotType, val) {
			return c
		}
	}
	return nil
}

// admitsRMValue reports whether the OPT child c admits the RM value
// val, whose RM type name is gotType: an untyped child, the exact
// type, a BMM subtype, a collapsed DV_INTERVAL whose bounds fit, or a
// Go primitive under an AOM 1.4 primitive short name. Both the
// single-valued alternatives and the multi-valued items without an
// archetype_node_id (REQ-102) bind by this rule.
func admitsRMValue(c *tcimpl.CompiledNode, gotType string, val any) bool {
	want := c.RMTypeName()
	if want == "" {
		// Wildcard / not-typed OPT child: accept.
		return true
	}
	if gotType == want || rmTypeIsSubtypeOf(gotType, want) || intervalRMTypeMatches(gotType, want, val) {
		return true
	}
	// AOM 1.4 primitive short name (DURATION, DATE, INTEGER, …)
	// pinned under a BMM-typed attribute channel: the RM value
	// may be a Go string, integer, real, or bool rather than an
	// RM wrapper type.
	return tcimpl.IsAOMPrimitiveShortName(want) && c.PrimitiveConstraint() != nil &&
		primitiveValueMatchesShortName(want, val)
}

// formatAllowedTypes renders the OPT child RM types for inclusion
// in alternative_mismatch Detail messages.
func formatAllowedTypes(children []*tcimpl.CompiledNode) string {
	names := make([]string, len(children))
	for i, c := range children {
		names[i] = c.RMTypeName()
	}
	return "[" + strings.Join(names, ", ") + "]"
}

// walkMultipleAttribute enforces existence + cardinality on a
// multi-valued attribute and binds each RM item to an OPT child.
// A LOCATABLE binds by archetype_node_id: exact archetype/node ids
// first, then the parsed REQ-104 slot grammar (see [matchChildByID]);
// one that matches no child is a slot_fill. An item that is not a
// LOCATABLE has no archetype_node_id and binds by RM type instead
// (REQ-102, see [matchChildByRMType]); one that no child admits is an
// rm_type_mismatch or alternative_mismatch, never a slot_fill (see
// [unboundItemIssue]). Every bound item counts toward its child's
// occurrences.
func (w *walker) walkMultipleAttribute(
	opt *tcimpl.CompiledNode,
	attr *tcimpl.CompiledAttribute,
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
	// Recurse into each matched item. An item no OPT child binds
	// contributes one issue, unless the OPT declared no children for
	// this attribute, in which case the attribute is "open" (any RM
	// item passes; the OPT pinned only existence / cardinality, not
	// membership). Tally per-child occurrences for the AOM 1.4
	// occurrences upper-bound check.
	children := attr.Children()
	if len(children) == 0 {
		return
	}
	perChildCount := make(map[*tcimpl.CompiledNode]int, len(children))
	for idx, item := range items {
		itemPath := joinPath(parentPath, segmentForRMItem(attr, item, idx))
		byNodeID := bindsByNodeID(item, opt.RMTypeName(), attr.Name())
		var matched *tcimpl.CompiledNode
		if byNodeID {
			matched = matchChildByID(children, item)
		} else {
			matched = matchChildByRMType(children, item)
		}
		if matched == nil {
			w.emit(unboundItemIssue(attr, children, item, byNodeID, itemPath))
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
func childIdentity(c *tcimpl.CompiledNode) string {
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
//
// The violation's Detail is value-free, and the value it read moves
// over to Issue.Value unchanged, still redacted (REQ-168).
func (w *walker) applyPrimitive(
	optNode *tcimpl.CompiledNode,
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
			Value:    v.Value,
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
	case *string:
		// An optional String attribute: validate the text, not the pointer.
		if v == nil {
			return ""
		}
		return *v
	case rm.Integer:
		return int64(v)
	case rm.Real:
		// Normalise the named float64 to a bare float64 so a C_REAL
		// constraint validates it instead of rejecting it as
		// wrong_type (REQ-110: REAL on DV_QUANTITY.magnitude).
		return float64(v)
	}
	return rmValue
}

// isRequired reports whether the compiled attribute carries an
// existence interval with lower bound ≥ 1 — i.e. the OPT mandates
// presence. Also honours the BMM-mandatory bit (Required()): some
// attributes are mandatory by RM even when the OPT existence
// element is silent (e.g. COMPOSITION.category).
func isRequired(attr *tcimpl.CompiledAttribute) bool {
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
func segmentForChild(attr *tcimpl.CompiledAttribute, child *tcimpl.CompiledNode, idx int) string {
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
func segmentForRMItem(attr *tcimpl.CompiledAttribute, item any, idx int) string {
	seg := "/" + attr.Name()
	if id := locatableArchetypeNodeID(item); id != "" {
		return seg + "[" + id + "]"
	}
	return seg + fmt.Sprintf("[@%d]", idx+1)
}

// bindsByNodeID reports whether an item of the multi-valued attribute
// attrName, on a node of type parentRMType, is bound by its
// archetype_node_id. It holds for every LOCATABLE, a typed-nil one
// included. A nil item has no type of its own, so it follows the
// attribute's declared item type: node id where that type is a
// LOCATABLE (COMPOSITION.content), RM type where it is not
// (ACTOR.languages, FOLDER.items). Every other item carries no
// archetype_node_id and is bound by RM type (REQ-102).
func bindsByNodeID(item any, parentRMType, attrName string) bool {
	if item == nil {
		return declaresLocatableItems(parentRMType, attrName)
	}
	_, isLocatable := item.(rm.Locatable)
	return isLocatable
}

// declaresLocatableItems reports whether the pinned BMM declares the
// items of attrName on parentRMType as LOCATABLE. An attribute or type
// the BMM does not know keeps the node-id path, as before REQ-102 bound
// items by RM type.
func declaresLocatableItems(parentRMType, attrName string) bool {
	itemType, ok := rminfo.Default.AttributeRMType(bmmtype.Class(parentRMType), attrName)
	if !ok {
		return true
	}
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		return true
	}
	conforms, known := h.ConformsTo(bmmtype.Class(itemType), "LOCATABLE")
	return conforms || !known
}

// isNilItem reports whether item carries no RM value: nil, or a
// typed-nil pointer.
func isNilItem(item any) bool {
	return item == nil || rmread.IsTypedNilPointer(item)
}

// matchChildByID picks the OPT child whose ArchetypeID (for
// archetype-root pins) or NodeID (for at-code pins) matches the RM
// item's archetype_node_id, then the first slot whose parsed
// assertions admit it. It serves only items that [bindsByNodeID]
// accepts. Returns nil when none match, and always for an empty or
// unreadable id (a typed-nil LOCATABLE, a nil under a LOCATABLE
// attribute); the caller then emits slot_fill.
func matchChildByID(children []*tcimpl.CompiledNode, item any) *tcimpl.CompiledNode {
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

// matchChildByRMType binds an item that carries no archetype_node_id
// (REQ-102) to the first non-slot child whose RMTypeName admits the
// item's RM type, by the rule a single-valued attribute uses
// ([admitsRMValue]). A nil or typed-nil item has no value to bind, so
// no child takes it, not even an untyped one. Returns nil when no
// child admits the item.
func matchChildByRMType(children []*tcimpl.CompiledNode, item any) *tcimpl.CompiledNode {
	if isNilItem(item) {
		return nil
	}
	gotType := describeRMType(item)
	for _, c := range children {
		if !c.IsSlot() && admitsRMValue(c, gotType, item) {
			return c
		}
	}
	return nil
}

// unboundItemIssue reports an item of a multi-valued attribute that
// no OPT child binds, at the item's own path. An item that binds by
// node id is a slot_fill. An item without an archetype_node_id is
// never a slot_fill (REQ-102): it is an rm_type_mismatch when the
// attribute has one child and an alternative_mismatch when it has
// more, as on a single-valued attribute. The Detail names the item's
// RM type and the children's, never a value; a nil or typed-nil item
// has no RM type to name, so its Detail says it is a nil item.
func unboundItemIssue(
	attr *tcimpl.CompiledAttribute,
	children []*tcimpl.CompiledNode,
	item any,
	byNodeID bool,
	itemPath string,
) Issue {
	if byNodeID {
		return Issue{
			Path:     itemPath,
			Code:     "slot_fill",
			Detail:   fmt.Sprintf("RM item %s does not match any OPT child of %q (archetype/at-code mismatch)", describeLocatableID(item), attr.Name()),
			Severity: Error,
		}
	}
	iss := Issue{Path: itemPath, Code: "alternative_mismatch", Severity: Error}
	if len(children) == 1 {
		iss.Code = "rm_type_mismatch"
	}
	switch {
	case isNilItem(item):
		iss.Detail = fmt.Sprintf("nil item under %q has no RM value for an OPT child to bind", attr.Name())
	case len(children) == 1:
		iss.Detail = fmt.Sprintf("RM value of type %s under %q does not satisfy template RM type %s", describeRMType(item), attr.Name(), children[0].RMTypeName())
	default:
		iss.Detail = fmt.Sprintf("RM value of type %s under %q matches none of the OPT alternatives %s", describeRMType(item), attr.Name(), formatAllowedTypes(children))
	}
	return iss
}

// slotFitsArchetypeID checks whether archetypeID satisfies the
// slot's REQ-104 include / exclude rules, including the RM-type-
// prefix fallback when no includes were parsed.
func slotFitsArchetypeID(slot *tcimpl.CompiledNode, archetypeID string) bool {
	return slot.AllowsArchetypeID(archetypeID)
}

// locatableArchetypeNodeID extracts archetype_node_id from any RM
// LOCATABLE value. Returns "" for non-LOCATABLE values (DataValue
// subtypes, PartyProxy, EventContext). Thin wrapper over
// [rmTypeInfo], which reads the generated identity surface (ADR 0013).
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
func (w *walker) checkLocatableIdentity(opt *tcimpl.CompiledNode, rmValue any, path string) {
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
func (w *walker) checkRMType(opt *tcimpl.CompiledNode, rmValue any, path string) {
	want := opt.RMTypeName()
	if want == "" {
		return
	}
	got := describeRMType(rmValue)
	if got == want {
		return
	}
	if rmTypeIsSubtypeOf(got, want) || intervalRMTypeMatches(got, want, rmValue) {
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
//
// A parameterised name such as "DV_INTERVAL<DV_COUNT>" is looked up by
// its class, "DV_INTERVAL", because the generic parameter does not
// change which abstract types the class conforms to.
func rmTypeIsSubtypeOf(concrete, abstract string) bool {
	subtypes := bmmSubtypes[abstract]
	return slices.Contains(subtypes, bmmtype.Class(concrete))
}

// intervalRMTypeMatches reports whether a concrete interval RM type
// name satisfies an OPT-declared generic interval type (REQ-052 / REQ-110).
//
// REQ-052 sub-gap B: a DV_INTERVAL<T> value that has been through a
// canonical-JSON round-trip re-decodes as the bare DVInterval[DVOrdered]
// (typereg has a single "DV_INTERVAL" registration), so describeRMType
// reports "DV_INTERVAL" and the OPT's "DV_INTERVAL<DV_QUANTITY>" would
// spuriously fail. The wire form and the bounds are correct, so when the
// declared type is a parameterised interval and the value collapsed to a
// bare one, decide conformance from the bounds' runtime types via `val`.
func intervalRMTypeMatches(got, want string, val any) bool {
	if got == want {
		return true
	}
	if _, gotTyped := typedIntervalBound(got); want == "DV_INTERVAL" && gotTyped {
		return true
	}
	if wantBound, wantTyped := typedIntervalBound(want); got == "DV_INTERVAL" && wantTyped {
		return intervalBoundsSatisfy(val, wantBound)
	}
	return false
}

// typedIntervalBound returns the bound type of a parameterised interval name,
// "DV_QUANTITY" for "DV_INTERVAL<DV_QUANTITY>". ok is false for any other
// name, the bare "DV_INTERVAL" and a malformed spelling included.
func typedIntervalBound(name string) (bound string, ok bool) {
	class, params, ok := bmmtype.Split(name)
	if !ok || class != "DV_INTERVAL" || len(params) != 1 {
		return "", false
	}
	return params[0], true
}

// intervalBoundsSatisfy reports whether a round-trip-collapsed
// DVInterval[DVOrdered] satisfies DV_INTERVAL<wantInner> by inspecting
// the runtime types of its bounds (which survive the round-trip via
// their own `_type`). Every *present* bound must be the wanted element
// type; a fully unbounded interval carries no bound to key off and
// trivially satisfies any element type. (Requiring agreement rather than
// accepting on either bound rejects a malformed interval whose lower and
// upper disagree — well-formed DV_INTERVAL<T> bounds are both T.) See
// [intervalRMTypeMatches].
func intervalBoundsSatisfy(val any, wantInner string) bool {
	var iv *rm.DVInterval[rm.DVOrdered]
	switch v := val.(type) {
	case *rm.DVInterval[rm.DVOrdered]:
		iv = v
	case rm.DVInterval[rm.DVOrdered]:
		iv = &v
	default:
		return false
	}
	lowerPresent := !iv.LowerUnbounded && iv.Lower != nil
	upperPresent := !iv.UpperUnbounded && iv.Upper != nil
	if lowerPresent && describeRMType(iv.Lower) != wantInner {
		return false
	}
	if upperPresent && describeRMType(iv.Upper) != wantInner {
		return false
	}
	// Every present bound matched (or the interval is fully unbounded):
	// an unbounded interval satisfies any DV_INTERVAL<T>.
	return true
}

// primitiveValueMatchesShortName reports whether an RM-side Go value
// satisfies an AOM 1.4 primitive short-name OPT child (INTEGER,
// REAL, DURATION, STRING, …) bound to a BMM scalar attribute slot.
func primitiveValueMatchesShortName(shortName string, val any) bool {
	switch shortName {
	case "BOOLEAN":
		_, ok := val.(bool)
		return ok
	case "INTEGER":
		return rm.IsInt64(val)
	case "REAL":
		return rm.IsReal(val)
	case "DATE", "TIME", "DATE_TIME", "DURATION":
		_, ok := val.(string)
		return ok
	case "STRING":
		// An optional String attribute (DV_TEXT.formatting,
		// DV_IDENTIFIER.issuer, …) is a *string in the RM; the reader
		// returns it only when set, so a nil pointer is absent.
		if p, isPtr := val.(*string); isPtr {
			return p != nil
		}
		_, ok := val.(string)
		return ok
	default:
		return false
	}
}

// bmmSubtypes is the closed lookup of abstract → concrete RM type
// admission rules used by checkRMType. Sourced from
// openehr_rm_1.2.0.bmm: concrete classes that satisfy each
// abstract slot. Every row is written by hand.
//
// A concrete class missing from its abstract row is refused with a
// false rm_type_mismatch wherever an OPT node declares that abstract
// type. Adding a class to a row needs no naming table elsewhere:
// describeRMType names every class the generated type registry holds
// (rm.RMTypeName), and locatableArchetypeNodeID reads
// archetype_node_id through rm.Locatable. What the walker reads below
// a node still comes from rmread, so an OPT that constrains attributes
// of a newly admitted class also needs rmread readers for them. Extend
// the rows in lock-step with the BMM and with rmread.
//
// The DATA_VALUE row holds every concrete DATA_VALUE descendant of the
// pinned BMM. TestDataValueSubtypesMatchBMM compares it with rminfo, so
// a BMM bump that adds or drops a data value type fails that test until
// the row follows it.
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
		"DV_BOOLEAN", "DV_CODED_TEXT", "DV_COUNT", "DV_DATE",
		"DV_DATE_TIME", "DV_DURATION", "DV_EHR_URI",
		"DV_GENERAL_TIME_SPECIFICATION", "DV_IDENTIFIER", "DV_INTERVAL",
		"DV_MULTIMEDIA", "DV_ORDINAL", "DV_PARAGRAPH", "DV_PARSABLE",
		"DV_PERIODIC_TIME_SPECIFICATION", "DV_PROPORTION", "DV_QUANTITY",
		"DV_SCALE", "DV_STATE", "DV_TEXT", "DV_TIME", "DV_URI",
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
