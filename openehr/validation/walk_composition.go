package validation

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/internal/rmaccess"
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
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
	return pc, primitiveValueMatchesShortName(optNode.RMTypeName(), rmValue)
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
func admitSingle(attr *tcimpl.CompiledAttribute, children []*tcimpl.CompiledNode, val any, attrPath string) (child *tcimpl.CompiledNode, refusal Issue, admitted bool) {
	child = matchSingleAlternative(children, val)
	if child != nil && child.IsSlot() {
		archetypeID := locatableArchetypeNodeID(val)
		if archetypeID == "" || !child.AllowsArchetypeID(archetypeID) {
			return nil, Issue{
				Path:     attrPath,
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
			Path:     attrPath,
			Code:     "rm_type_mismatch",
			Layer:    LayerTemplate,
			Rule:     ruleType,
			Detail:   fmt.Sprintf("RM value of type %s under %q does not satisfy template RM type %s", describeRMType(val), attr.Name(), children[0].RMTypeName()),
			Severity: Error,
		}, false
	}
	return nil, Issue{
		Path:     attrPath,
		Code:     "alternative_mismatch",
		Layer:    LayerTemplate,
		Rule:     ruleType,
		Detail:   fmt.Sprintf("RM value of type %s under %q matches none of the OPT alternatives %s", describeRMType(val), attr.Name(), formatAllowedTypes(children)),
		Severity: Error,
	}, false
}

// matchSingleAlternative picks the OPT child of a single-valued
// attribute that binds the RM value, slot children included, by
// [bindByRMType]. Returns nil when no child fits. With exactly one
// child the function is effectively "does the child fit?"; the
// alternative_mismatch case fires only when the OPT declared more
// than one alternative.
func matchSingleAlternative(children []*tcimpl.CompiledNode, val any) *tcimpl.CompiledNode {
	return bindByRMType(children, val, func(*tcimpl.CompiledNode) bool { return true })
}

// bindByRMType picks, among the children eligible accepts, the one
// that binds the RM value val: the most specific child, in three
// tiers (REQ-102). Within a tier the first child in OPT order wins.
//
//  1. A child that names val's own RM type ([namesRMType]).
//  2. A child that names the AOM 1.4 primitive short name whose
//     canonical DV wrapper val is ([wrapsShortName]).
//  3. A child val's type conforms to by BMM descent, or an untyped
//     child, or, for an interval each present bound of which conforms
//     to T, a typed DV_INTERVAL<T> child ([conformsToChild]).
//
// A value therefore meets the constraints of the tightest child that
// admits it, not those of a looser one listed first: under
// [DV_TEXT, DV_CODED_TEXT] a DV_CODED_TEXT meets the DV_CODED_TEXT
// child's code list, and under [DATA_VALUE, DURATION] a DV_DURATION
// meets the DURATION child's C_DURATION. Both the single-valued
// alternatives and the multi-valued items without an
// archetype_node_id bind by this rule. Returns nil when no child
// admits val.
func bindByRMType(children []*tcimpl.CompiledNode, val any, eligible func(*tcimpl.CompiledNode) bool) *tcimpl.CompiledNode {
	gotType := describeRMType(val)
	for _, c := range children {
		if eligible(c) && namesRMType(c, gotType, val) {
			return c
		}
	}
	for _, c := range children {
		if eligible(c) && wrapsShortName(c, gotType) {
			return c
		}
	}
	for _, c := range children {
		if eligible(c) && conformsToChild(c, gotType, val) {
			return c
		}
	}
	return nil
}

// namesRMType reports whether the OPT child c names the RM value val's
// own type, gotType. That is the same name, with one refinement for an
// interval that went through a canonical-JSON round-trip and decoded
// as the bare DV_INTERVAL: its type is the parameterised
// DV_INTERVAL<T> its bounds give (REQ-102, REQ-110), so a typed child
// whose T every present bound is names it. One with no bound present
// has no parameter, so only a bare DV_INTERVAL child names its type,
// and a typed child admits it in the third tier ([conformsToChild]).
// An AOM 1.4 primitive short name over a Go primitive val (an int64
// under INTEGER, a string under DURATION) also names val's type. An
// untyped child names no type.
func namesRMType(c *tcimpl.CompiledNode, gotType string, val any) bool {
	want := c.RMTypeName()
	if want == "" {
		return false
	}
	if gotType == "DV_INTERVAL" {
		bounded := collapsedIntervalBounded(val)
		if bound, typed := typedIntervalBound(want); typed {
			return bounded && intervalBoundsSatisfy(val, bound)
		}
		if want == "DV_INTERVAL" {
			return !bounded
		}
	}
	if gotType == want {
		return true
	}
	// AOM 1.4 primitive short name (DURATION, DATE, INTEGER, …)
	// pinned under a BMM-typed attribute channel: the RM value
	// may be a Go string, integer, real, or bool rather than an
	// RM wrapper type.
	return tcimpl.IsAOMPrimitiveShortName(want) && c.PrimitiveConstraint() != nil &&
		primitiveValueMatchesShortName(want, val)
}

// wrapsShortName reports whether the OPT child c names an AOM 1.4
// primitive short name whose canonical DV wrapper
// ([aomPrimitiveWrappers]) is the RM type gotType.
func wrapsShortName(c *tcimpl.CompiledNode, gotType string) bool {
	wrapper, ok := aomPrimitiveWrappers[c.RMTypeName()]
	return ok && wrapper == gotType
}

// conformsToChild reports whether the OPT child c declares no RM type,
// or one the RM type gotType conforms to ([conformsToNode]). Under a
// typed DV_INTERVAL<T> child each present bound has to be a T, that is,
// conform to T ([intervalConformsTo]): a DV_COUNT bound is a DV_ORDERED.
// A round-tripped bare DV_INTERVAL val with no bound present carries no T
// to check, so a typed DV_INTERVAL<T> child admits it here, after any
// bare DV_INTERVAL child has named its type ([namesRMType]).
func conformsToChild(c *tcimpl.CompiledNode, gotType string, val any) bool {
	want := c.RMTypeName()
	if want == "" || conformsToNode(c, gotType) {
		return true
	}
	return intervalConformsTo(gotType, want, val)
}

// conformsToNode reports whether the RM type concrete conforms to the RM
// type the compiled node c declares, as [rmTypeIsSubtypeOf] decides it, from
// the classes the compile found to conform to that type
// ([tcimpl.ConformsTo]) rather than from the class hierarchy at each call.
func conformsToNode(c *tcimpl.CompiledNode, concrete string) bool {
	class := bmmtype.Class(concrete)
	if conforms, known := tcimpl.ConformsTo(c, class); known {
		return conforms
	}
	wrapper, ok := aomPrimitiveWrappers[c.RMTypeName()]
	return ok && wrapper == class
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
// of parentRM, at attrPath, when the instance lacks it: `required`, on the
// layer and by the rule [requiredBy] names, with required set. A
// multi-valued attribute is described as one. required is false when
// neither the template nor the BMM bit the template carries requires attr.
func absentIssue(attr *tcimpl.CompiledAttribute, parentRM any, attrPath string, multiValued bool) (iss Issue, required bool) {
	layer, rule, required := requiredBy(attr)
	if !required {
		return iss, false
	}
	detail := fmt.Sprintf("required attribute %q absent on %s", attr.Name(), describeRMType(parentRM))
	if multiValued {
		detail = fmt.Sprintf("required multi-valued attribute %q absent on %s", attr.Name(), describeRMType(parentRM))
	}
	return Issue{
		Path:     attrPath,
		Code:     "required",
		Layer:    layer,
		Rule:     rule,
		Detail:   detail,
		Severity: Error,
	}, true
}

// emptyIssue returns the issue the template walk reports for the
// multi-valued attribute attr of parentRM, at attrPath, when it is present
// and holds no item: `required`, with required set, when [requiredBy] says
// attr is required. When the template's existence requires it, the rule is
// that existence; when only the RM makes it mandatory, the list is there and
// empty, which the RM's own rule names cardinality.
func emptyIssue(attr *tcimpl.CompiledAttribute, parentRM any, attrPath string) (iss Issue, required bool) {
	layer, rule, required := requiredBy(attr)
	if !required {
		return iss, false
	}
	if layer == LayerRM {
		rule = ruleCardinality
	}
	return Issue{
		Path:     attrPath,
		Code:     "required",
		Layer:    layer,
		Rule:     rule,
		Detail:   fmt.Sprintf("required multi-valued attribute %q is empty on %s", attr.Name(), describeRMType(parentRM)),
		Severity: Error,
	}, true
}

// childCountIssue returns the `cardinality` issue for the multi-valued
// attribute attr at attrPath, which holds n items, when the OPT pins a
// child-count interval n falls outside, with out set.
func childCountIssue(attr *tcimpl.CompiledAttribute, n int, attrPath string) (iss Issue, out bool) {
	cm := attr.ChildMultiplicity()
	if cm == nil || !outOfMultiplicityInterval(n, cm) {
		return iss, false
	}
	return Issue{
		Path:     attrPath,
		Code:     "cardinality",
		Layer:    LayerTemplate,
		Rule:     ruleCardinality,
		Detail:   fmt.Sprintf("attribute %q has %d children; OPT cardinality %s", attr.Name(), n, formatInterval(cm)),
		Severity: Error,
	}, true
}

// boundItemPaths returns the issue path of each of the items of the
// multi-valued attribute at attrPath, bound to the OPT children bound holds.
// The items of a list are written by the shared path rule ([itemPaths]),
// which reads each item's id and the name pinned on the child it binds to
// ([boundPathItems]): the binding comes first, because an item's predicate
// depends on its siblings' ids and on the names their bound OPT nodes pin.
// An entry of a Hash attribute (isHash) is written by its position in
// ascending key order, never by its key (REQ-102 Issue paths).
func boundItemPaths(attrPath string, items []any, bound []*tcimpl.CompiledNode, isHash bool) []string {
	if isHash {
		paths := make([]string, len(items))
		for idx := range items {
			paths[idx] = hashEntryPath(attrPath, idx)
		}
		return paths
	}
	return itemPaths(attrPath, boundPathItems(items, bound))
}

// appendOccurrenceIssues appends to issues a `cardinality` issue, rule
// occurrences, for each of children, the OPT children of the multi-valued
// attribute attr at attrPath, that pins an occurrences interval the number
// of items bound to it falls outside; bound holds the child each item binds
// to. A child whose occurrences the items miss is written with its own
// predicate ([optChildPaths]).
func appendOccurrenceIssues(issues []Issue, attr *tcimpl.CompiledAttribute, children, bound []*tcimpl.CompiledNode, attrPath string) []Issue {
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
			childPath = optChildPaths(attrPath, children)
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

// boundPathItems returns what the path rule reads of each item: its
// archetype_node_id and the name pinned on the child it binds to.
func boundPathItems(items []any, bound []*tcimpl.CompiledNode) []pathItem {
	out := make([]pathItem, len(items))
	for idx, item := range items {
		out[idx].id = locatableArchetypeNodeID(item)
		if bound[idx] != nil {
			out[idx].name = bound[idx].NodeName()
		}
	}
	return out
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
// per Violation.
// The constraint's expected input is computed via dataValueInput;
// for non-DataValue RM types that map to a primitive (e.g.
// CODE_PHRASE directly under category/defining_code → C_CODE_PHRASE),
// the value is passed through as-is.
//
// The violation's Detail is value-free, and the value it read moves
// over to Issue.Value unchanged, still redacted (REQ-168).
func appendPrimitiveIssues(issues []Issue, rmValue any, path string, pc constraints.PrimitiveConstraint) []Issue {
	input := primitiveInput(rmValue)
	for _, v := range pc.Validate(input) {
		issues = append(issues, Issue{
			Path:     path,
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

// requiredBy reports whether the compiled attribute must be present, and
// which layer and rule require it. An OPT existence interval with lower
// bound ≥ 1 is the template's own requirement: the template layer, rule
// existence. Failing that, the BMM-mandatory bit (Required()) makes some
// attributes mandatory even when the OPT existence element is silent (e.g.
// COMPOSITION.category): the RM layer, rule mandatory. When both hold, the
// template's own requirement is the one reported. required is false, with
// no layer or rule, when neither holds.
func requiredBy(attr *tcimpl.CompiledAttribute) (layer Layer, rule string, required bool) {
	if e := attr.Existence(); e != nil && !e.LowerUnbounded() && e.Lower() >= 1 {
		return LayerTemplate, ruleExistence, true
	}
	if attr.Required() {
		return LayerRM, ruleMandatory, true
	}
	return LayerNone, "", false
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
func prohibitedIssue(attr *tcimpl.CompiledAttribute, parentRM any, attrPath string) Issue {
	how := "it allows none of the attribute's children"
	if e := attr.Existence(); e != nil && !e.UpperUnbounded() && e.Upper() == 0 {
		how = "existence " + formatInterval(e)
	}
	return Issue{
		Path:     attrPath,
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
// ([bindByRMType]); a slot child is skipped in every tier. A nil or
// typed-nil item has no value to bind, so no child takes it, not even
// an untyped one. Returns nil when no child admits the item.
func matchChildByRMType(children []*tcimpl.CompiledNode, item any) *tcimpl.CompiledNode {
	if isNilItem(item) {
		return nil
	}
	return bindByRMType(children, item, func(c *tcimpl.CompiledNode) bool { return !c.IsSlot() })
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
		Path:     itemPath,
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
func identityIssue(opt *tcimpl.CompiledNode, rmValue any, path string) (iss Issue, ok bool) {
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
			Path:     joinPath(path, "/archetype_node_id"),
			Code:     "archetype_id_mismatch",
			Layer:    LayerTemplate,
			Rule:     ruleIdentity,
			Detail:   fmt.Sprintf("archetype_node_id %q does not match template archetype id %q at %s", id, want, path),
			Severity: Error,
		}, true
	}
	if want := opt.NodeID(); want != "" && id != want {
		return Issue{
			Path:     joinPath(path, "/archetype_node_id"),
			Code:     "node_id_mismatch",
			Layer:    LayerTemplate,
			Rule:     ruleIdentity,
			Detail:   fmt.Sprintf("archetype_node_id %q does not match template node_id %q at %s", id, want, path),
			Severity: Error,
		}, true
	}
	return iss, false
}

// rmTypeIssue returns rm_type_mismatch, with ok set, when the concrete RM
// type does not conform to the compiled OPT node's RMTypeName. Conformance is
// the pinned BMM's class hierarchy ([rmTypeIsSubtypeOf], read through
// [conformsToNode]), for an abstract and a concrete ancestor alike: a node
// declared as ITEM_STRUCTURE admits ITEM_TREE, one declared as DV_TEXT
// admits DV_CODED_TEXT.
func rmTypeIssue(opt *tcimpl.CompiledNode, rmValue any, path string) (iss Issue, ok bool) {
	want := opt.RMTypeName()
	if want == "" {
		return iss, false
	}
	got := describeRMType(rmValue)
	if got == want {
		return iss, false
	}
	if conformsToNode(opt, got) || intervalRMTypeMatches(got, want, rmValue) {
		return iss, false
	}
	return Issue{
		Path:     path,
		Code:     "rm_type_mismatch",
		Layer:    LayerTemplate,
		Rule:     ruleType,
		Detail:   fmt.Sprintf("RM type %s does not satisfy template RM type %s at %s", got, want, path),
		Severity: Error,
	}, true
}

// rmTypeIsSubtypeOf reports whether the RM type concrete conforms to
// the OPT-declared RM type declared: equal to it, or a descendant of
// it in the pinned BMM's class hierarchy (rminfo), whether declared is
// abstract or concrete (REQ-102, REQ-110). A newly covered RM class
// therefore needs no admission row here, and the generated access reads
// its attributes below the node.
//
// A parameterised value type such as "DV_INTERVAL<DV_COUNT>" is looked
// up by its class, "DV_INTERVAL", because the generic parameter does
// not change which classes it descends from. The declared type is
// looked up as written: a parameterised DV_INTERVAL<T> is no BMM class
// name, so the hierarchy does not know it, and [intervalRMTypeMatches]
// decides it instead.
//
// When the hierarchy does not know one of the two names, an AOM 1.4
// primitive short name in declared still admits its canonical DV
// wrapper ([aomPrimitiveWrappers]).
func rmTypeIsSubtypeOf(concrete, declared string) bool {
	class := bmmtype.Class(concrete)
	if h, ok := rminfo.Default.(rminfo.Hierarchy); ok {
		if conforms, known := h.ConformsTo(class, declared); known {
			return conforms
		}
	}
	wrapper, ok := aomPrimitiveWrappers[declared]
	return ok && wrapper == class
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
// ([intervalConformsTo]).
func intervalRMTypeMatches(got, want string, val any) bool {
	if got == want {
		return true
	}
	if _, gotTyped := typedIntervalBound(got); want == "DV_INTERVAL" && gotTyped {
		return true
	}
	return intervalConformsTo(got, want, val)
}

// intervalConformsTo reports whether an interval value of RM type got, val,
// conforms to the typed interval want, DV_INTERVAL<T>: each present bound
// has to be a T (REQ-110), which a bound of T's own type or of one that
// descends from T in the pinned BMM is ([rmTypeIsSubtypeOf]). A typed
// value, DV_INTERVAL<U>, conforms when U does; a round-tripped bare
// DV_INTERVAL is read by its bounds' runtime types, and one with no bound
// present conforms to every T. An abstract T such as DV_ORDERED therefore
// admits DV_COUNT bounds. Any other pair of types does not conform here.
func intervalConformsTo(got, want string, val any) bool {
	wantBound, wantTyped := typedIntervalBound(want)
	if !wantTyped {
		return false
	}
	if got == "DV_INTERVAL" {
		return intervalBoundsConform(val, wantBound)
	}
	gotBound, gotTyped := typedIntervalBound(got)
	return gotTyped && rmTypeIsSubtypeOf(gotBound, wantBound)
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
// DVInterval[DVOrdered] names DV_INTERVAL<wantInner> exactly, by the
// runtime types of its bounds (which survive the round-trip via their own
// `_type`). Every *present* bound must be of the wanted element type
// itself; a fully unbounded interval carries no bound to key off and
// trivially satisfies any element type. (Requiring agreement rather than
// accepting on either bound rejects a malformed interval whose lower and
// upper disagree — well-formed DV_INTERVAL<T> bounds are both T.) The
// first binding tier reads it ([namesRMType]); a bound that only conforms
// to wantInner is read by [intervalBoundsConform].
func intervalBoundsSatisfy(val any, wantInner string) bool {
	bounds, ok := collapsedIntervalBounds(val)
	if !ok {
		return false
	}
	for _, b := range bounds {
		if describeRMType(b) != wantInner {
			return false
		}
	}
	// Every present bound matched (or the interval is fully unbounded):
	// an unbounded interval satisfies any DV_INTERVAL<T>.
	return true
}

// intervalBoundsConform reports whether val is a round-trip-collapsed
// DVInterval[DVOrdered] each of whose present bounds conforms to wantInner:
// its type is wantInner or descends from it in the pinned BMM
// ([rmTypeIsSubtypeOf]). An interval with no bound present conforms. The
// binding tiers name a type exactly ([intervalBoundsSatisfy]); this is the
// conformance the third tier and the RM type match read.
func intervalBoundsConform(val any, wantInner string) bool {
	bounds, ok := collapsedIntervalBounds(val)
	if !ok {
		return false
	}
	for _, b := range bounds {
		if !rmTypeIsSubtypeOf(describeRMType(b), wantInner) {
			return false
		}
	}
	return true
}

// collapsedIntervalBounded reports whether val is a round-trip-collapsed
// DVInterval[DVOrdered] with at least one present bound, a bound that
// gives the interval its parameterised type ([namesRMType]).
func collapsedIntervalBounded(val any) bool {
	bounds, ok := collapsedIntervalBounds(val)
	return ok && len(bounds) > 0
}

// collapsedIntervalBounds returns the present bounds of a
// round-trip-collapsed DVInterval[DVOrdered], lower first; a side that
// is unbounded or carries no bound is absent. ok is false when val is
// no such interval.
func collapsedIntervalBounds(val any) (bounds []rm.DVOrdered, ok bool) {
	var iv *rm.DVInterval[rm.DVOrdered]
	switch v := val.(type) {
	case *rm.DVInterval[rm.DVOrdered]:
		iv = v
	case rm.DVInterval[rm.DVOrdered]:
		iv = &v
	default:
		return nil, false
	}
	if !iv.LowerUnbounded && iv.Lower != nil {
		bounds = append(bounds, iv.Lower)
	}
	if !iv.UpperUnbounded && iv.Upper != nil {
		bounds = append(bounds, iv.Upper)
	}
	return bounds, true
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

// aomPrimitiveWrappers maps each AOM 1.4 primitive short name an OPT
// may declare as an RM type, typically under C_PRIMITIVE_OBJECT, to the
// canonical DV wrapper that carries its value. The short names are no
// BMM classes, so the class hierarchy cannot admit the wrapper; this
// table does (REQ-102, Validation dimensions: RM type match). Each
// wrapper is the type instance.concreteFor builds for the short name,
// so the generator's output passes the RM type match ([rmTypeIssue]).
//
// A Go primitive under a short name (an int64 under INTEGER, a string
// under DURATION) is admitted apart from this table, by
// primitiveValueMatchesShortName.
var aomPrimitiveWrappers = map[string]string{
	"DURATION":  "DV_DURATION",
	"DATE":      "DV_DATE",
	"TIME":      "DV_TIME",
	"DATE_TIME": "DV_DATE_TIME",
	"BOOLEAN":   "DV_BOOLEAN",
	"INTEGER":   "DV_COUNT",
}
