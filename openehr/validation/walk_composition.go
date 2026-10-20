package validation

// walk_composition.go: the template's rules the one walk (engine.go) applies
// at a node and at an attribute: identity and RM type against the bound
// template node, admission of a single value and binding of a list's items
// to the template's children, the existence, cardinality and occurrences
// findings, and the template's value constraint at a primitive leaf. Each
// returns or appends its issues; the walk decides where they run and in
// which order.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/internal/rmaccess"
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/internal/rmbind"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// scalarLeaf reports whether optNode is an AOM 1.4 primitive short-name leaf
// on a BMM scalar channel (DV_COUNT.magnitude under INTEGER, DV_DURATION.value
// under DURATION) whose short name rmValue, a Go primitive, matches, and
// returns its primitive constraint. Such a node is checked by its constraint
// alone: it has no RM type to match and no attributes to walk.
func scalarLeaf(optNode *tcimpl.CompiledNode, rmValue any) (constraints.PrimitiveConstraint, bool) {
	pc := optNode.PrimitiveConstraint()
	if pc == nil || !tcimpl.IsAOMPrimitiveShortName(optNode.RMTypeName()) {
		return nil, false
	}
	return pc, rmbind.PrimitiveValueMatchesShortName(optNode.RMTypeName(), rmValue)
}

// isConstraintRef reports whether pc is a CONSTRAINT_REF: a value set bound
// outside the template, which the SDK cannot consult ([Coverage.External]).
func isConstraintRef(pc constraints.PrimitiveConstraint) bool {
	_, isRef := pc.(constraints.ConstraintRef)
	return isRef
}

// admitSingle binds val, the value of the single-valued attribute attr at
// attrPath, to one of children, the children the OPT allows, by
// [matchSingleAlternative]. It returns the child, with admitted set, or the
// one issue that refuses the value, which is then not walked: a slot child
// whose assertions the value's archetype id does not satisfy is a
// `slot_fill`; no child at all is an `rm_type_mismatch` when the attribute
// allows one child (a plain type constraint) and an `alternative_mismatch`
// when it allows more, listing their RM types.
func admitSingle(attr *tcimpl.CompiledAttribute, children []*tcimpl.CompiledNode, val any, attrPath issuePath) (child *tcimpl.CompiledNode, refusal Issue, admitted bool) {
	child = matchSingleAlternative(children, val)
	if child != nil && child.IsSlot() {
		archetypeID := locatableArchetypeNodeID(val)
		if archetypeID == "" || !child.AllowsArchetypeID(archetypeID) {
			return nil, Issue{
				Path:     attrPath.String(),
				Code:     "slot_fill",
				Layer:    LayerTemplate,
				Rule:     ruleSlot,
				Detail:   fmt.Sprintf("RM value %s under %q does not satisfy slot %s", describeLocatableID(val), attr.Name(), child.NodeID()),
				Severity: Error,
			}, false
		}
	}
	if child != nil {
		return child, refusal, true
	}
	// With multiple OPT children the OPT declared AnyOf alternatives; with
	// a single child it is a plain type constraint. The Issue.Code tells
	// "wrong type" from "didn't match any of N allowed types".
	if len(children) == 1 {
		return nil, Issue{
			Path:     attrPath.String(),
			Code:     "rm_type_mismatch",
			Layer:    LayerTemplate,
			Rule:     ruleType,
			Detail:   fmt.Sprintf("RM value of type %s under %q does not satisfy template RM type %s", describeRMType(val), attr.Name(), children[0].RMTypeName()),
			Severity: Error,
		}, false
	}
	return nil, Issue{
		Path:     attrPath.String(),
		Code:     "alternative_mismatch",
		Layer:    LayerTemplate,
		Rule:     ruleType,
		Detail:   fmt.Sprintf("RM value of type %s under %q matches none of the OPT alternatives %s", describeRMType(val), attr.Name(), formatAllowedTypes(children)),
		Severity: Error,
	}, false
}

// matchSingleAlternative picks the OPT child of a single-valued
// attribute that binds the RM value, slot children included, by the
// three binding tiers of [rmbind.Bind] (REQ-102). Returns nil when no
// child fits. With exactly one child the function is effectively "does
// the child fit?"; the alternative_mismatch case fires only when the
// OPT declared more than one alternative.
func matchSingleAlternative(children []*tcimpl.CompiledNode, val any) *tcimpl.CompiledNode {
	return rmbind.Bind(children, val, func(*tcimpl.CompiledNode) bool { return true })
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

// absentIssue returns the issue the template reports for the attribute attr
// of parentRM, at attrPath, when the instance lacks it and the template's
// existence requires it ([templateRequires]): `required`, on the template
// layer, rule existence, with required set. A multi-valued attribute is
// described as one. The RM's requirement is not read here: the walk reads it
// from the value's class ([engine.checkAbsent]).
func absentIssue(attr *tcimpl.CompiledAttribute, parentRM any, attrPath issuePath, multiValued bool) (iss Issue, required bool) {
	if !templateRequires(attr) {
		return iss, false
	}
	detail := fmt.Sprintf("required attribute %q absent on %s", attr.Name(), describeRMType(parentRM))
	if multiValued {
		detail = fmt.Sprintf("required multi-valued attribute %q absent on %s", attr.Name(), describeRMType(parentRM))
	}
	return Issue{
		Path:     attrPath.String(),
		Code:     "required",
		Layer:    LayerTemplate,
		Rule:     ruleExistence,
		Detail:   detail,
		Severity: Error,
	}, true
}

// emptyIssue returns the issue the template reports for the multi-valued
// attribute attr of parentRM, at attrPath, when it is present and holds no
// item and the template's existence requires it ([templateRequires]):
// `required`, on the template layer, rule existence, with required set. The
// RM's requirement is not read here: the walk reads it from the value's
// class ([engine.checkEmpty]).
func emptyIssue(attr *tcimpl.CompiledAttribute, parentRM any, attrPath issuePath) (iss Issue, required bool) {
	if !templateRequires(attr) {
		return iss, false
	}
	return Issue{
		Path:     attrPath.String(),
		Code:     "required",
		Layer:    LayerTemplate,
		Rule:     ruleExistence,
		Detail:   fmt.Sprintf("required multi-valued attribute %q is empty on %s", attr.Name(), describeRMType(parentRM)),
		Severity: Error,
	}, true
}

// childCountIssue returns the `cardinality` issue for the multi-valued
// attribute attr at attrPath, which holds n items, when the OPT pins a
// child-count interval n falls outside, with out set.
func childCountIssue(attr *tcimpl.CompiledAttribute, n int, attrPath issuePath) (iss Issue, out bool) {
	cm := attr.ChildMultiplicity()
	if cm == nil || !outOfMultiplicityInterval(n, cm) {
		return iss, false
	}
	return Issue{
		Path:     attrPath.String(),
		Code:     "cardinality",
		Layer:    LayerTemplate,
		Rule:     ruleCardinality,
		Detail:   fmt.Sprintf("attribute %q has %d children; OPT cardinality %s", attr.Name(), n, formatInterval(cm)),
		Severity: Error,
	}, true
}

// appendBoundPredicates appends to preds the predicate of each of the items
// of a multi-valued attribute, bound to the OPT children bound holds, and
// returns the extended slice. The items of a list are written by the shared
// path rule ([resolvePredicates]), which reads each item's id and the name
// pinned on the child it binds to: the binding comes first, because an
// item's predicate depends on its siblings' ids and on the names their bound
// OPT nodes pin. An entry of a Hash attribute (isHash) is written by its
// position in ascending key order, never by its key ([entryPredicate]).
func appendBoundPredicates(preds []predicate, items []any, bound []*tcimpl.CompiledNode, isHash bool) []predicate {
	from := len(preds)
	for idx, item := range items {
		if isHash {
			preds = append(preds, entryPredicate(idx))
			continue
		}
		p := predicate{id: locatableArchetypeNodeID(item)}
		if bound[idx] != nil {
			p.name = bound[idx].NodeName()
		}
		preds = append(preds, p)
	}
	if !isHash {
		resolvePredicates(preds[from:], true)
	}
	return preds
}

// appendOccurrenceIssues appends to issues a `cardinality` issue, rule
// occurrences, for each of children, the OPT children of the multi-valued
// attribute attr at attrPath, that pins an occurrences interval the number
// of items bound to it falls outside; bound holds the child each item binds
// to. A child whose occurrences the items miss is written with its own
// predicate ([optChildPaths]).
func appendOccurrenceIssues(issues []Issue, attr *tcimpl.CompiledAttribute, children, bound []*tcimpl.CompiledNode, attrPath issuePath) []Issue {
	var childPath []string
	for i, c := range children {
		occ := c.Occurrences()
		if occ == nil {
			continue
		}
		got := 0
		for _, b := range bound {
			if b == c {
				got++
			}
		}
		if !outOfMultiplicityInterval(got, occ) {
			continue
		}
		if childPath == nil {
			childPath = optChildPaths(attrPath.String(), children)
		}
		issues = append(issues, Issue{
			Path:     childPath[i],
			Code:     "cardinality",
			Layer:    LayerTemplate,
			Rule:     ruleOccurrences,
			Detail:   fmt.Sprintf("OPT child %s appears %d times under %q; occurrences %s", childIdentity(c), got, attr.Name(), formatInterval(occ)),
			Severity: Error,
		})
	}
	return issues
}

// bindItems binds each of the items of the multi-valued attribute
// attrName, on a node of RM type parentRMType, to one of its OPT
// children: a LOCATABLE by its archetype_node_id ([matchChildByID]),
// any other item by its RM type ([matchChildByRMType]). bound holds the
// child each item binds to, nil when none does, and byNodeID whether the
// item binds by its id.
func bindItems(parentRMType, attrName string, children []*tcimpl.CompiledNode, items []any) (bound []*tcimpl.CompiledNode, byNodeID []bool) {
	bound = make([]*tcimpl.CompiledNode, len(items))
	byNodeID = make([]bool, len(items))
	for idx, item := range items {
		byNodeID[idx] = bindsByNodeID(item, parentRMType, attrName)
		if byNodeID[idx] {
			bound[idx] = matchChildByID(children, item)
		} else {
			bound[idx] = matchChildByRMType(children, item)
		}
	}
	return bound, byNodeID
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

// appendPrimitiveIssues runs the REQ-103 primitive constraint pc against
// the RM value bound to its node, at path, and appends to issues one Issue
// per Violation; the path is written only when there is one.
// The constraint's expected input is computed via dataValueInput;
// for non-DataValue RM types that map to a primitive (e.g.
// CODE_PHRASE directly under category/defining_code → C_CODE_PHRASE),
// the value is passed through as-is.
//
// The violation's Detail is value-free, and the value it read moves
// over to Issue.Value unchanged, still redacted (REQ-168).
func appendPrimitiveIssues(issues []Issue, rmValue any, path issuePath, pc constraints.PrimitiveConstraint) []Issue {
	violations := pc.Validate(primitiveInput(rmValue))
	if len(violations) == 0 {
		return issues
	}
	at := path.String()
	for _, v := range violations {
		issues = append(issues, Issue{
			Path:     at,
			Code:     "primitive_" + string(v.Code),
			Layer:    LayerTemplate,
			Rule:     string(v.Code),
			Detail:   v.Detail,
			Severity: Error,
			Value:    v.Value,
		})
	}
	return issues
}

// primitiveInput converts an RM value into the Go value the typed
// primitive validator expects. Handles five layers:
//
//   - rm.DataValue concretes (DvQuantity, DvCodedText, DvText, …)
//     via dataValueInput — REQ-103 primitive types that bind to
//     ELEMENT.value.
//   - rm.CodePhrase directly (e.g. category/defining_code) — bind
//     to constraints.CodedTermRef.
//   - an optional primitive the RM holds by pointer (an *rm.Integer
//     precision, an *rm.Real accuracy, a *bool accuracy_is_percent):
//     the value it points to, read as that value is
//     ([rmbind.PrimitiveValue]).
//   - an optional String the RM holds by pointer (a *string
//     formatting): the text it points to, or the empty string when it
//     is not set. [rmbind.PrimitiveValue] leaves a *string as it is;
//     the *string case below reads it.
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
	value := rmbind.PrimitiveValue(rmValue)
	switch v := value.(type) {
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
	case rm.Integer64:
		// DV_COUNT.magnitude: a C_INTEGER constraint reads a bare int64,
		// and the constraints package cannot name the RM type.
		return int64(v)
	case rm.Real:
		// Normalise the named float64 to a bare float64 so a C_REAL
		// constraint validates it instead of rejecting it as
		// wrong_type (REQ-110: REAL on DV_QUANTITY.magnitude).
		return float64(v)
	}
	return value
}

// templateRequires reports whether the template's own existence requires
// the compiled attribute: an OPT existence interval with a lower bound of 1
// or more. The mandatory bit the compile carries from the lookup it was
// given ([tcimpl.CompiledAttribute.Required]) is not the template's
// requirement and is not read: the RM rules read the pinned Reference
// Model, from the value's class.
func templateRequires(attr *tcimpl.CompiledAttribute) bool {
	e := attr.Existence()
	return e != nil && !e.LowerUnbounded() && e.Lower() >= 1
}

// isProhibited reports whether the OPT prohibits the compiled attribute:
// its existence interval has a bounded upper bound of 0, or it declares at
// least one child and allows none of them ([allowedChildren]). An attribute
// with no child is open, not prohibited. The compile decides it once per
// attribute ([tcimpl.Prohibited]).
func isProhibited(attr *tcimpl.CompiledAttribute) bool {
	return tcimpl.Prohibited(attr)
}

// allowedChildren returns the children of attr the OPT allows, in OPT
// order: those whose occurrences upper bound is unbounded, or bounded above
// 0, or that state none. The compile resolves them once per attribute
// ([tcimpl.AllowedChildNodes]); the caller must not change the slice.
func allowedChildren(attr *tcimpl.CompiledAttribute) []*tcimpl.CompiledNode {
	return tcimpl.AllowedChildNodes(attr)
}

// prohibitedIssue returns the issue that reports the attribute attr, which
// the OPT prohibits, as present on parentRM, at attrPath. The caller then
// leaves the attribute: the walk checks nothing more on it and does not
// descend into it. The Detail names the attribute, the parent's RM type and
// how the template prohibits it, never the value.
func prohibitedIssue(attr *tcimpl.CompiledAttribute, parentRM any, attrPath issuePath) Issue {
	how := "it allows none of the attribute's children"
	if e := attr.Existence(); e != nil && !e.UpperUnbounded() && e.Upper() == 0 {
		how = "existence " + formatInterval(e)
	}
	return Issue{
		Path:     attrPath.String(),
		Code:     "prohibited",
		Layer:    LayerTemplate,
		Rule:     ruleExistence,
		Detail:   fmt.Sprintf("attribute %q is present on %s, but the template prohibits it (%s)", attr.Name(), describeRMType(parentRM), how),
		Severity: Error,
	}
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
	return item == nil || rmaccess.IsTypedNilPointer(item)
}

// matchChildByID picks the OPT child whose ArchetypeID (for
// archetype-root pins) or NodeID (for at-code pins) matches the RM
// item's archetype_node_id, then the first slot whose parsed
// assertions admit it. Where several non-slot children carry the id,
// as when a template reuses one archetype under distinct names, the
// item's name picks among them ([matchChildByPinnedName]). It serves
// only items that [bindsByNodeID] accepts. Returns nil when none
// match, and always for an empty or unreadable id (a typed-nil
// LOCATABLE, a nil under a LOCATABLE attribute); the caller then
// emits slot_fill.
func matchChildByID(children []*tcimpl.CompiledNode, item any) *tcimpl.CompiledNode {
	id := locatableArchetypeNodeID(item)
	if id == "" {
		return nil
	}
	var first *tcimpl.CompiledNode
	for _, c := range children {
		if c.IsSlot() || !carriesID(c, id) {
			continue
		}
		if first != nil {
			return matchChildByPinnedName(children, id, locatableName(item))
		}
		first = c
	}
	if first != nil {
		return first
	}
	for _, c := range children {
		if c.IsSlot() && slotFitsArchetypeID(c, id) {
			return c
		}
	}
	return nil
}

// carriesID reports whether the OPT child c pins id: as its archetype id
// (an archetype root) or as its at-code.
func carriesID(c *tcimpl.CompiledNode, id string) bool {
	return (c.ArchetypeID() != "" && c.ArchetypeID() == id) || (c.NodeID() != "" && c.NodeID() == id)
}

// matchChildByPinnedName picks, among the non-slot children that carry
// id, the one an item named name binds to: the child whose pinned name
// (the template-level node name) is name; failing that, the first that
// pins no name; failing that, the first, whose name constraint then
// reports the mismatch. The name only decides the binding; no issue
// path or message carries it.
func matchChildByPinnedName(children []*tcimpl.CompiledNode, id, name string) *tcimpl.CompiledNode {
	var first, unnamed *tcimpl.CompiledNode
	for _, c := range children {
		if c.IsSlot() || !carriesID(c, id) {
			continue
		}
		pinned := c.NodeName()
		if pinned != "" && pinned == name {
			return c
		}
		if first == nil {
			first = c
		}
		if pinned == "" && unnamed == nil {
			unnamed = c
		}
	}
	if unnamed != nil {
		return unnamed
	}
	return first
}

// locatableName returns the value of a LOCATABLE item's name, a DV_TEXT
// or a DV_CODED_TEXT, read as the walk reads the name it checks against
// the template, or "" when the item has none.
func locatableName(item any) string {
	v, ok := rmaccess.Single(item, "name")
	if !ok {
		return ""
	}
	like, ok := v.(rm.DVTextLike)
	if !ok {
		return ""
	}
	text, ok := rm.AsDVText(like)
	if !ok {
		return ""
	}
	return text.Value
}

// matchChildByRMType binds an item that carries no archetype_node_id
// (REQ-102) to a non-slot child whose RMTypeName admits the item's RM
// type, by the three tiers a single-valued attribute uses
// ([rmbind.Bind]); a slot child is skipped in every tier. A nil or
// typed-nil item has no value to bind, so no child takes it, not even
// an untyped one. Returns nil when no child admits the item.
func matchChildByRMType(children []*tcimpl.CompiledNode, item any) *tcimpl.CompiledNode {
	if isNilItem(item) {
		return nil
	}
	return rmbind.Bind(children, item, func(c *tcimpl.CompiledNode) bool { return !c.IsSlot() })
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
	itemPath issuePath,
) Issue {
	if byNodeID {
		return Issue{
			Path:     itemPath.String(),
			Code:     "slot_fill",
			Layer:    LayerTemplate,
			Rule:     ruleSlot,
			Detail:   fmt.Sprintf("RM item %s does not match any OPT child of %q (archetype/at-code mismatch)", describeLocatableID(item), attr.Name()),
			Severity: Error,
		}
	}
	code := "alternative_mismatch"
	if len(children) == 1 {
		code = "rm_type_mismatch"
	}
	var detail string
	switch {
	case isNilItem(item):
		detail = fmt.Sprintf("nil item under %q has no RM value for an OPT child to bind", attr.Name())
	case len(children) == 1:
		detail = fmt.Sprintf("RM value of type %s under %q does not satisfy template RM type %s", describeRMType(item), attr.Name(), children[0].RMTypeName())
	default:
		detail = fmt.Sprintf("RM value of type %s under %q matches none of the OPT alternatives %s", describeRMType(item), attr.Name(), formatAllowedTypes(children))
	}
	return Issue{
		Path:     itemPath.String(),
		Code:     code,
		Layer:    LayerTemplate,
		Rule:     ruleType,
		Detail:   detail,
		Severity: Error,
	}
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

// identityIssue returns node_id_mismatch / archetype_id_mismatch, with ok
// set, when the RM's archetype_node_id disagrees with the OPT-pinned id at
// this node. Archetype-root nodes compare against ArchetypeID(); inner nodes
// (at-code-pinned) compare against NodeID().
func identityIssue(opt *tcimpl.CompiledNode, rmValue any, path issuePath) (iss Issue, ok bool) {
	if opt.IsSlot() {
		// Slot fit is by the parsed REQ-104 archetype-id assertion
		// grammar (RM-type-prefix fallback when no includes parsed).
		// The slot's NodeID is the OPT's own at-code for the slot
		// point — it
		// is not expected to match the filling archetype's
		// archetype_node_id, so a direct identity check here would
		// false-positive on every legitimate slot fill.
		return iss, false
	}
	id := locatableArchetypeNodeID(rmValue)
	if id == "" {
		return iss, false
	}
	if want := opt.ArchetypeID(); want != "" {
		if id == want {
			return iss, false
		}
		return Issue{
			Path:     path.join("/archetype_node_id"),
			Code:     "archetype_id_mismatch",
			Layer:    LayerTemplate,
			Rule:     ruleIdentity,
			Detail:   fmt.Sprintf("archetype_node_id %q does not match template archetype id %q at %s", id, want, path.String()),
			Severity: Error,
		}, true
	}
	if want := opt.NodeID(); want != "" && id != want {
		return Issue{
			Path:     path.join("/archetype_node_id"),
			Code:     "node_id_mismatch",
			Layer:    LayerTemplate,
			Rule:     ruleIdentity,
			Detail:   fmt.Sprintf("archetype_node_id %q does not match template node_id %q at %s", id, want, path.String()),
			Severity: Error,
		}, true
	}
	return iss, false
}

// rmTypeIssue returns rm_type_mismatch, with ok set, when the concrete RM
// type does not conform to the compiled OPT node's RMTypeName. Conformance is
// the pinned BMM's class hierarchy, as the compile recorded it for the node
// ([rmbind.ConformsToNode]), for an abstract and a concrete ancestor alike: a node
// declared as ITEM_STRUCTURE admits ITEM_TREE, one declared as DV_TEXT
// admits DV_CODED_TEXT.
func rmTypeIssue(opt *tcimpl.CompiledNode, rmValue any, path issuePath) (iss Issue, ok bool) {
	want := opt.RMTypeName()
	if want == "" {
		return iss, false
	}
	got := describeRMType(rmValue)
	if got == want {
		return iss, false
	}
	if rmbind.ConformsToNode(opt, got) || intervalRMTypeMatches(got, want, rmValue) {
		return iss, false
	}
	at := path.String()
	return Issue{
		Path:     at,
		Code:     "rm_type_mismatch",
		Layer:    LayerTemplate,
		Rule:     ruleType,
		Detail:   fmt.Sprintf("RM type %s does not satisfy template RM type %s at %s", got, want, at),
		Severity: Error,
	}, true
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
// bare one, decide conformance from the bounds' runtime types via `val`
// ([rmbind.IntervalConformsTo]).
func intervalRMTypeMatches(got, want string, val any) bool {
	if got == want {
		return true
	}
	if _, gotTyped := rmbind.TypedIntervalBound(got); want == "DV_INTERVAL" && gotTyped {
		return true
	}
	return rmbind.IntervalConformsTo(got, want, val)
}
