package validation

import (
	"fmt"
	"slices"
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

// walkNode is the lockstep visitor: it enters the OPT node `optNode`
// bound to the RM value `rmValue`, emits structural issues at this
// node, then descends each OPT-declared attribute into its matched
// RM child(ren). The `path` argument is the issue path of `rmValue`
// (root = "/"), written by the rule the floor shares (see
// [itemPaths]).
//
// Per-node behaviour:
//   - Identity (LOCATABLE.archetype_node_id ↔ OPT pin) and RM-type
//     match (conformance in the pinned BMM's class hierarchy) fire at
//     every node.
//   - Slot leaves: no descent (slot-fit was decided by the parent
//     attribute when binding RM items to OPT children).
//   - Primitive constraint leaves (PrimitiveConstraint() != nil):
//     the REQ-103 typed validator runs against the bound RM value;
//     no descent into implicit RM-mandatory attrs of the
//     primitive's RM type. A CONSTRAINT_REF leaf is the exception:
//     its value set is external, so the attributes of the bound
//     value are still walked, as below.
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
	// before descent; the generated access reads an interface holding a
	// nil pointer as absent. This
	// belt-and-suspenders check costs one type-switch and prevents
	// any future descent path from re-introducing the panic class
	// the v2 reviewers caught twice (Element.Value, then slice
	// elements).
	if rmaccess.IsTypedNilPointer(rmValue) {
		return
	}
	// The class the walk entered, whose invariants the floor may not reach
	// when a limit stops it first ([Coverage.Unevaluated]).
	w.visited = recordClass(w.visited, rmValue)

	// Identity + RM-type checks at this node. At the composition
	// root, identity is checked inline against COMPOSITION's
	// archetype_node_id (not via a separate attribute descent).
	w.checkLocatableIdentity(optNode, rmValue, path)
	// AOM 1.4 primitive short-name leaf on a BMM scalar channel
	// (e.g. DV_COUNT.magnitude ← INTEGER, DV_DURATION.value ← string).
	if pc := optNode.PrimitiveConstraint(); pc != nil && tcimpl.IsAOMPrimitiveShortName(optNode.RMTypeName()) {
		if primitiveValueMatchesShortName(optNode.RMTypeName(), rmValue) {
			w.applyPrimitive(optNode, rmValue, path, pc)
			// A scalar has no attributes to walk, but a CONSTRAINT_REF
			// on it is still a value set checked nowhere, so the
			// coverage names it as below.
			if _, isRef := pc.(constraints.ConstraintRef); isRef {
				w.external = append(w.external, path)
			}
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
		// does not descend further: implicit RM attrs of the
		// primitive's RM type (e.g. DV_QUANTITY.magnitude / .units)
		// are the primitive validator's territory.
		w.applyPrimitive(optNode, rmValue, path, pc)
		// REQ-102: a CONSTRAINT_REF names an external value set and
		// checks nothing locally, so the bound value's RM-mandatory
		// attributes (a CODE_PHRASE's terminology_id and code_string)
		// are still walked below, as at a node the OPT leaves
		// unconstrained. The result's coverage names the node, since the
		// code there was checked against no value set.
		if _, isRef := pc.(constraints.ConstraintRef); !isRef {
			return
		}
		w.external = append(w.external, path)
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
// (an attribute the OPT prohibits that holds a value is reported as
// prohibited and not walked into, [isProhibited]) and binds the RM value to
// one of the children the OPT allows ([allowedChildren]; a child that is
// not allowed binds no value) via the AOM 1.4
// "one alternative MUST match" semantics, by [bindByRMType]: the most
// specific child wins, a child of the value's own RM type before the
// AOM 1.4 short name it wraps, and that before a child the type only
// conforms to in the pinned BMM, or a typed DV_INTERVAL<T> child for an
// interval each present bound of which conforms to T. When none match
// the walker emits a typed issue:
//   - exactly one allowed child → `rm_type_mismatch` (plain type
//     constraint, no real alternatives);
//   - two or more allowed children → `alternative_mismatch` with the
//     list of their RM types.
func (w *walker) walkSingleAttribute(
	opt *tcimpl.CompiledNode,
	attr *tcimpl.CompiledAttribute,
	parentRM any,
	parentPath string,
) {
	attrPath := joinPath(parentPath, "/"+attr.Name())
	val, ok := rmaccess.Single(parentRM, attr.Name())
	if !ok {
		if layer, rule, required := requiredBy(attr); required {
			w.emit(Issue{
				Path:     attrPath,
				Code:     "required",
				Layer:    layer,
				Rule:     rule,
				Detail:   fmt.Sprintf("required attribute %q absent on %s", attr.Name(), describeRMType(parentRM)),
				Severity: Error,
			})
		}
		return
	}
	// An attribute the OPT prohibits is reported once when it holds a
	// value (the access reads nil and typed-nil as absent), and the walk
	// goes no further into it.
	if isProhibited(attr) {
		w.emitProhibited(attr, parentRM, attrPath)
		return
	}
	// A child the OPT does not allow binds no value; the attribute is
	// prohibited, above, when it allows none.
	children := allowedChildren(attr)
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
				Layer:    LayerTemplate,
				Rule:     ruleSlot,
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
				Layer:    LayerTemplate,
				Rule:     ruleType,
				Detail:   fmt.Sprintf("RM value of type %s under %q does not satisfy template RM type %s", describeRMType(val), attr.Name(), children[0].RMTypeName()),
				Severity: Error,
			})
		} else {
			w.emit(Issue{
				Path:     attrPath,
				Code:     "alternative_mismatch",
				Layer:    LayerTemplate,
				Rule:     ruleType,
				Detail:   fmt.Sprintf("RM value of type %s under %q matches none of the OPT alternatives %s", describeRMType(val), attr.Name(), formatAllowedTypes(children)),
				Severity: Error,
			})
		}
		return
	}
	w.walkNode(child, val, attrPath)
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
// or one the RM type gotType conforms to ([rmTypeIsSubtypeOf]). Under a
// typed DV_INTERVAL<T> child each present bound has to be a T, that is,
// conform to T ([intervalConformsTo]): a DV_COUNT bound is a DV_ORDERED.
// A round-tripped bare DV_INTERVAL val with no bound present carries no T
// to check, so a typed DV_INTERVAL<T> child admits it here, after any
// bare DV_INTERVAL child has named its type ([namesRMType]).
func conformsToChild(c *tcimpl.CompiledNode, gotType string, val any) bool {
	want := c.RMTypeName()
	if want == "" || rmTypeIsSubtypeOf(gotType, want) {
		return true
	}
	return intervalConformsTo(gotType, want, val)
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
// multi-valued attribute and binds each RM item to an OPT child. An
// attribute the OPT prohibits that holds an item is reported as
// prohibited and not walked into.
// A LOCATABLE binds by archetype_node_id: exact archetype/node ids
// first, picked by the template-level name where several children share
// the id, then the parsed REQ-104 slot grammar (see [matchChildByID]);
// one that matches no child is a slot_fill. An item that is not a
// LOCATABLE has no archetype_node_id and binds by RM type instead
// (REQ-102, see [matchChildByRMType]); one that no child admits is an
// rm_type_mismatch or alternative_mismatch, never a slot_fill (see
// [unboundItemIssue]). Every bound item counts toward its child's
// occurrences.
//
// Each item is written in Issue.Path by the shared path rule
// ([itemPaths]): the binding comes first, because an item's predicate
// depends on its siblings' ids and on the names their bound OPT nodes
// pin. A child whose occurrences the items miss is written with its own
// predicate ([optChildPaths]).
func (w *walker) walkMultipleAttribute(
	opt *tcimpl.CompiledNode,
	attr *tcimpl.CompiledAttribute,
	parentRM any,
	parentPath string,
) {
	attrPath := joinPath(parentPath, "/"+attr.Name())
	items, n, isHash, ok := readItems(parentRM, attr.Name())
	if !ok {
		// The attribute is absent: a nil list, a nil hash, or one the
		// parent's RM class does not declare. If the OPT pinned existence
		// ≥ 1 (or BMM marks the attribute required), the instance does not
		// satisfy it, and it is reported as absent, as walkSingleAttribute
		// reports a single-valued one. The count of its items is a matter
		// for a present attribute only.
		if layer, rule, required := requiredBy(attr); required {
			w.emit(Issue{
				Path:     attrPath,
				Code:     "required",
				Layer:    layer,
				Rule:     rule,
				Detail:   fmt.Sprintf("required multi-valued attribute %q absent on %s", attr.Name(), describeRMType(parentRM)),
				Severity: Error,
			})
		}
		return
	}
	// An attribute the OPT prohibits is reported once when it holds at
	// least one item, and the walk goes no further into it.
	if n > 0 && isProhibited(attr) {
		w.emitProhibited(attr, parentRM, attrPath)
		return
	}
	// Existence: lower ≥ 1 with zero items is "required". When only the RM
	// makes the attribute mandatory, the list is there and empty, which the
	// RM's own rule names cardinality, as the floor does for the same list;
	// when the template's existence requires it, it is that existence rule.
	// A nil list is absent and was reported above.
	if layer, rule, required := requiredBy(attr); required && n == 0 {
		if layer == LayerRM {
			rule = ruleCardinality
		}
		w.emit(Issue{
			Path:     attrPath,
			Code:     "required",
			Layer:    layer,
			Rule:     rule,
			Detail:   fmt.Sprintf("required multi-valued attribute %q is empty on %s", attr.Name(), describeRMType(parentRM)),
			Severity: Error,
		})
	}
	// Child-count cardinality interval (when the OPT pinned one).
	if cm := attr.ChildMultiplicity(); cm != nil {
		if outOfMultiplicityInterval(n, cm) {
			w.emit(Issue{
				Path:     attrPath,
				Code:     "cardinality",
				Layer:    LayerTemplate,
				Rule:     ruleCardinality,
				Detail:   fmt.Sprintf("attribute %q has %d children; OPT cardinality %s", attr.Name(), n, formatInterval(cm)),
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
	if len(items) != n {
		// A list of a primitive type is counted, never read item by item
		// (readItems): it has no item an OPT child could bind.
		return
	}
	bound, byNodeID := bindItems(opt.RMTypeName(), attr.Name(), children, items)
	var itemPath []string
	if isHash {
		// An entry of a Hash attribute is written by its position in
		// ascending key order, never by its key (REQ-102 Issue paths).
		itemPath = make([]string, len(items))
		for idx := range items {
			itemPath[idx] = hashEntryPath(attrPath, idx)
		}
	} else {
		pathItems := boundPathItems(items, bound)
		w.recordNames(attrPath, pathItems)
		itemPath = itemPaths(attrPath, pathItems)
	}
	perChildCount := make(map[*tcimpl.CompiledNode]int, len(children))
	for idx, item := range items {
		matched := bound[idx]
		if matched == nil {
			w.emit(unboundItemIssue(attr, children, item, byNodeID[idx], itemPath[idx]))
			continue
		}
		perChildCount[matched]++
		w.walkNode(matched, item, itemPath[idx])
	}
	// AOM 1.4 occurrences upper bound on each OPT child: when the
	// OPT pins a child's `<occurrences>` interval, the count of
	// matching RM items must fall within it. A zero count + lower
	// ≥ 1 surfaces as `cardinality` at the attribute level — that
	// case is already covered by the multi-attribute existence
	// check above when the attribute itself is empty, but we still
	// fire here per-child when the attribute is non-empty and a
	// specific OPT child is missing or over-represented.
	var childPath []string
	for i, c := range children {
		occ := c.Occurrences()
		if occ == nil {
			continue
		}
		got := perChildCount[c]
		if outOfMultiplicityInterval(got, occ) {
			if childPath == nil {
				childPath = optChildPaths(attrPath, children)
			}
			w.emit(Issue{
				Path:     childPath[i],
				Code:     "cardinality",
				Layer:    LayerTemplate,
				Rule:     ruleOccurrences,
				Detail:   fmt.Sprintf("OPT child %s appears %d times under %q; occurrences %s", childIdentity(c), got, attr.Name(), formatInterval(occ)),
				Severity: Error,
			})
		}
	}
}

// readItems reads the items of the attribute attr on parent, which the
// template walk treats as multi-valued, and their number n: the items of a
// list, in order, or, where parent's RM class declares attr as a Hash, the
// entries of the hash in ascending key order (isHash). A list whose items
// the pinned BMM declares of a primitive type ([isPrimitiveType]), such as
// DV_MULTIMEDIA data, an Array of Octet, is counted ([rmaccess.Len]) and
// never read item by item: items is nil whatever n is. present is false for
// a nil list or hash and for an attribute the class does not declare; a
// present one may hold no item.
func readItems(parent any, attr string) (items []any, n int, isHash, present bool) {
	switch declared := declaredItemType(parent, attr); {
	case declared == hashType:
		_, items, present = rmaccess.Hash(parent, attr)
		return items, len(items), true, present
	case isPrimitiveType(rminfo.Default, declared):
		n, present = rmaccess.Len(parent, attr)
		return nil, n, false, present
	}
	items, present = rmaccess.Multiple(parent, attr)
	return items, len(items), false, present
}

// declaredItemType returns the RM type the pinned BMM declares for the
// attribute attr of parent's RM class, the type of its items for a list, or
// "" when parent's class is not one the walk knows or does not declare attr.
func declaredItemType(parent any, attr string) string {
	rmType, _, ok := rmTypeInfo(parent)
	if !ok {
		return ""
	}
	t, _ := rminfo.Default.AttributeRMType(bmmtype.Class(rmType), attr)
	return t
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
// archetype_node_id and the name pinned on the child it binds to. These
// names, keyed by the attribute's path and each item's index, are the
// bindings the floor takes to write the same paths ([pinnedNames]).
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

// recordNames keeps, in w.names, the pinned name of each of the items of
// the multi-valued attribute at attrPath whose id a sibling shares, keyed
// by attrPath and the item's index. The name of an item whose id is its
// own never reaches its path, so it is not kept: the floor, fed what is
// kept, writes every item as this walk does.
func (w *walker) recordNames(attrPath string, items []pathItem) {
	if !slices.ContainsFunc(items, func(it pathItem) bool { return it.name != "" }) {
		return
	}
	s := newIDSharing(items)
	for i, it := range items {
		if it.name == "" || !s.shared(it.id) {
			continue
		}
		if w.names == nil {
			w.names = pinnedNames{}
		}
		w.names[itemKey{attrPath: attrPath, index: i}] = it.name
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
			Layer:    LayerTemplate,
			Rule:     string(v.Code),
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
// least one child and allows none of them ([isAllowed]). An attribute with
// no child is open, not prohibited.
func isProhibited(attr *tcimpl.CompiledAttribute) bool {
	if e := attr.Existence(); e != nil && !e.UpperUnbounded() && e.Upper() == 0 {
		return true
	}
	children := attr.Children()
	return len(children) > 0 && !slices.ContainsFunc(children, isAllowed)
}

// isAllowed reports whether the OPT allows the child: its occurrences
// upper bound is unbounded, or bounded above 0, or it states none.
func isAllowed(child *tcimpl.CompiledNode) bool {
	occ := child.Occurrences()
	return occ == nil || occ.UpperUnbounded() || occ.Upper() != 0
}

// allowedChildren returns the children of attr the OPT allows
// ([isAllowed]), in OPT order.
func allowedChildren(attr *tcimpl.CompiledAttribute) []*tcimpl.CompiledNode {
	// Children returns a copy, so deleting from it leaves attr as it is.
	return slices.DeleteFunc(attr.Children(), func(c *tcimpl.CompiledNode) bool { return !isAllowed(c) })
}

// emitProhibited reports the attribute attr, which the OPT prohibits, as
// present on parentRM, at attrPath. The caller then leaves the attribute:
// the walk checks nothing more on it and does not descend into it. The
// Detail names the attribute, the parent's RM type and how the template
// prohibits it, never the value.
func (w *walker) emitProhibited(attr *tcimpl.CompiledAttribute, parentRM any, attrPath string) {
	how := "it allows none of the attribute's children"
	if e := attr.Existence(); e != nil && !e.UpperUnbounded() && e.Upper() == 0 {
		how = "existence " + formatInterval(e)
	}
	w.emit(Issue{
		Path:     attrPath,
		Code:     "prohibited",
		Layer:    LayerTemplate,
		Rule:     ruleExistence,
		Detail:   fmt.Sprintf("attribute %q is present on %s, but the template prohibits it (%s)", attr.Name(), describeRMType(parentRM), how),
		Severity: Error,
	})
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
				Path:     joinPath(path, "/archetype_node_id"),
				Code:     "archetype_id_mismatch",
				Layer:    LayerTemplate,
				Rule:     ruleIdentity,
				Detail:   fmt.Sprintf("archetype_node_id %q does not match template archetype id %q at %s", id, want, path),
				Severity: Error,
			})
		}
		return
	}
	if want := opt.NodeID(); want != "" && id != want {
		w.emit(Issue{
			Path:     joinPath(path, "/archetype_node_id"),
			Code:     "node_id_mismatch",
			Layer:    LayerTemplate,
			Rule:     ruleIdentity,
			Detail:   fmt.Sprintf("archetype_node_id %q does not match template node_id %q at %s", id, want, path),
			Severity: Error,
		})
	}
}

// checkRMType emits rm_type_mismatch when the concrete RM type does
// not conform to the compiled OPT node's RMTypeName. Conformance is
// the pinned BMM's class hierarchy ([rmTypeIsSubtypeOf]), for an
// abstract and a concrete ancestor alike: a node declared as
// ITEM_STRUCTURE admits ITEM_TREE, one declared as DV_TEXT admits
// DV_CODED_TEXT.
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
		Layer:    LayerTemplate,
		Rule:     ruleType,
		Detail:   fmt.Sprintf("RM type %s does not satisfy template RM type %s at %s", got, want, path),
		Severity: Error,
	})
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
// so the generator's output passes checkRMType.
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
