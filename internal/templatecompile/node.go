package templatecompile

import (
	"maps"
	"slices"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// IsAOMPrimitiveShortName reports whether s is an AOM 1.4 primitive
// short name (BOOLEAN, DATE, TIME, DATE_TIME, DURATION, INTEGER, REAL,
// STRING). These appear as the rm_type_name of C_PRIMITIVE_OBJECT
// children pinned under BMM-typed primitive attributes (e.g.
// DV_DURATION.value, DV_TEXT.value). Shared by the validator and the
// instance synthesiser so both agree on which leaves carry a primitive
// constraint rather than an RM wrapper. It is a closed switch and
// uses no reflection.
func IsAOMPrimitiveShortName(s string) bool {
	switch s {
	case "BOOLEAN", "DATE", "TIME", "DATE_TIME", "DURATION", "INTEGER", "REAL", "STRING":
		return true
	default:
		return false
	}
}

// CompiledNode is one node in the compiled OPT tree. Mirrors the
// OPT's [template.Node] taxonomy (ComplexObject / ArchetypeRoot /
// Slot) collapsed into a single struct because walker code rarely
// cares about the wire-side discrimination; it asks
// "what's the AQL path of this thing, what does it constrain, and
// can I descend".
//
// IsSlot distinguishes leaf slot-fill points from descendable
// objects; ArchetypeID is non-empty exactly when the OPT pinned an
// archetype id on this node (i.e. it was a *ArchetypeRoot on the
// wire side).
type CompiledNode struct {
	aqlPath     string
	rmTypeName  string
	nodeID      string
	nodeName    string
	archetypeID string // empty unless this node was an *ArchetypeRoot on the wire
	occurrences *template.Multiplicity
	attributes  []*CompiledAttribute
	parent      *CompiledNode

	// slot holds what only a slot carries; nil exactly when the node is
	// not a slot.
	slot *slotInfo

	// primitive carries the typed REQ-103 constraint value when the
	// wire xsi:type was a primitive. Nil for non-primitive nodes
	// (composition root, archetype roots, slots, plain complex objects).
	primitive constraints.PrimitiveConstraint

	// terms is populated only on *ArchetypeRoot-derived nodes. At-codes
	// are scoped to their enclosing archetype root; the same at-code
	// can have different meanings under sibling roots. See [Term] for
	// the parent-walk lookup that respects scope.
	terms map[string]template.ArchetypeTerm

	// conforming and conformingResolved hold what a validation walk asks
	// of the node's type, set by [Compile] once its attributes are
	// compiled and never changed after (walkfacts.go): the classes that
	// conform to rmTypeName, nil when the class hierarchy does not know
	// it.
	conforming         classBits
	conformingResolved bool
}

// slotInfo is what a slot node carries beyond the fields every node has:
// its raw include and exclude assertion strings and its parsed rules. It
// sits behind a pointer so that the many nodes that are not slots do not
// carry room for it.
type slotInfo struct {
	includes []string
	excludes []string
	rules    constraints.SlotRules
}

// AQLPath returns the canonical openEHR path string of this node.
// Computed once at compile time; stable across calls.
func (n *CompiledNode) AQLPath() string { return n.aqlPath }

// RMTypeName returns the BMM RM class name this node constrains
// (e.g. "COMPOSITION", "DV_QUANTITY").
func (n *CompiledNode) RMTypeName() string { return n.rmTypeName }

// NodeID returns the archetype node id (at-code) of this node, or
// "" when none is set on the wire.
func (n *CompiledNode) NodeID() string { return n.nodeID }

// NodeName returns the template-level node name (the fixed C_STRING
// the OPT pins on this node's name attribute), or "" when the node
// pins no fixed name. The archetype concept term is never
// substituted: distinct sibling names are what disambiguate a reused
// archetype under one slot, and the reference WebTemplate derives its
// node ids and name-predicated paths from this value. Carried
// verbatim from [template.ObjectNode.NodeName].
func (n *CompiledNode) NodeName() string { return n.nodeName }

// ArchetypeID returns the slot-fill archetype id when this node was
// a *ArchetypeRoot on the OPT wire side; otherwise "".
func (n *CompiledNode) ArchetypeID() string { return n.archetypeID }

// Occurrences returns the parsed occurrences interval, or nil when
// the OPT did not declare one for this node.
func (n *CompiledNode) Occurrences() *template.Multiplicity {
	return n.occurrences
}

// Attributes returns a defensive copy of the compiled attributes
// (OPT-declared + implicit-RM-injected) in stable order. See
// [CompiledAttribute.Implicit] to distinguish the two sources.
func (n *CompiledNode) Attributes() []*CompiledAttribute {
	return slices.Clone(n.attributes)
}

// Attribute returns the named child attribute (OPT-declared or
// implicit), or nil when no attribute by that name exists on this
// node.
func (n *CompiledNode) Attribute(name string) *CompiledAttribute {
	for _, a := range n.attributes {
		if a.name == name {
			return a
		}
	}
	return nil
}

// Parent returns the immediate parent node, or nil for the root.
func (n *CompiledNode) Parent() *CompiledNode { return n.parent }

// IsSlot reports whether the wire-side node was an *ARCHETYPE_SLOT
// (i.e. an opaque slot-fill point with no descendable structure).
func (n *CompiledNode) IsSlot() bool { return n.slot != nil }

// SlotIncludes returns a defensive copy of the slot's raw
// archetype-id include assertion strings. Empty for non-slot nodes.
func (n *CompiledNode) SlotIncludes() []string {
	if n.slot == nil {
		return nil
	}
	return slices.Clone(n.slot.includes)
}

// SlotExcludes returns a defensive copy of the slot's raw
// archetype-id exclude assertion strings. Empty for non-slot nodes.
func (n *CompiledNode) SlotExcludes() []string {
	if n.slot == nil {
		return nil
	}
	return slices.Clone(n.slot.excludes)
}

// SlotRules returns the parsed archetype-slot assertion rules for this
// slot. Zero value for non-slot nodes. The returned rule slices are
// defensive copies.
func (n *CompiledNode) SlotRules() constraints.SlotRules {
	if n.slot == nil {
		return constraints.SlotRules{}
	}
	return n.slot.rules.Clone()
}

// AllowsArchetypeID reports whether archetypeID satisfies this
// slot's include / exclude rules, including the
// RM-type-prefix fallback when no includes were parsed. False for
// non-slot nodes.
func (n *CompiledNode) AllowsArchetypeID(archetypeID string) bool {
	if n.slot == nil {
		return false
	}
	return n.slot.rules.AllowsArchetypeID(archetypeID)
}

// ExampleSlotFillArchetypeID returns a synthetic archetype id for
// instance generation when one can be derived from this slot's rules.
// Returns "" for non-slot nodes, or when parsed includes are too
// complex to synthesize safely.
func (n *CompiledNode) ExampleSlotFillArchetypeID() string {
	if n.slot == nil {
		return ""
	}
	return n.slot.rules.ExampleArchetypeID()
}

// PrimitiveConstraint returns the typed primitive constraint value for
// this node, or nil when the wire xsi:type was not a primitive in
// the closed set. Mirrors [template.ComplexObject.PrimitiveConstraint];
// the compile step copies the value through without modification.
func (n *CompiledNode) PrimitiveConstraint() constraints.PrimitiveConstraint {
	return n.primitive
}

// Term returns the term definition for an at-code, scoped to the
// nearest enclosing archetype root. Walks the parent chain so a
// descendant of `/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]`
// sees that root's terminology rather than a sibling root's. Returns
// (zero, false) when no enclosing root defines the code.
//
// The lang parameter is accepted for forward compatibility but is
// currently ignored: an ADL 1.4 OPT carries a single document
// language ([Compiled.Language]), so there is only one set of term
// definitions to return. A future multi-language OPT
// would select lang and fall back to the document language when the
// requested translation is absent; until then every lang resolves
// to the document-language term.
//
// The returned [template.ArchetypeTerm.Items] map is a defensive copy.
func (n *CompiledNode) Term(code, lang string) (template.ArchetypeTerm, bool) {
	_ = lang // single-language OPT (REQ-105); see doc comment.
	for cur := n; cur != nil; cur = cur.parent {
		if t, ok := cur.terms[code]; ok {
			return template.ArchetypeTerm{Code: t.Code, Items: maps.Clone(t.Items)}, true
		}
	}
	return template.ArchetypeTerm{}, false
}

// RootTerm returns the term definition for an at-code that the nearest
// enclosing archetype root of n defines, n itself included: the first
// node up the parent chain that has an archetype id. Unlike [Term], it
// asks that root alone, so a code the root does not define finds nothing,
// even where a root further up defines the same code: a local code means
// what its own archetype says. lang is read as Term reads it. Returns
// (zero, false) when n has no enclosing archetype root or that root does
// not define code.
//
// The returned [template.ArchetypeTerm.Items] map is a defensive copy.
func (n *CompiledNode) RootTerm(code, lang string) (template.ArchetypeTerm, bool) {
	_ = lang // one document language per OPT; see Term.
	for cur := n; cur != nil; cur = cur.parent {
		if cur.archetypeID == "" {
			continue
		}
		t, ok := cur.terms[code]
		if !ok {
			return template.ArchetypeTerm{}, false
		}
		return template.ArchetypeTerm{Code: t.Code, Items: maps.Clone(t.Items)}, true
	}
	return template.ArchetypeTerm{}, false
}

// CompiledAttribute is one attribute on a CompiledNode. Carries the
// OPT-declared cardinality + existence when the OPT had an
// <attributes> entry for it, and the RM-derived type when implicit.
type CompiledAttribute struct {
	name              string
	cardinality       template.Cardinality
	existence         *template.Multiplicity
	childMultiplicity *template.Multiplicity
	rmTypeName        string // RM type from rminfo; empty when not resolved
	implicit          bool   // true when injected from rminfo, not declared by OPT
	required          bool   // BMM is_mandatory (true even when OPT silent)
	children          []*CompiledNode

	// allowed and prohibited hold what a validation walk asks of the
	// attribute, set by [Compile] and never changed after (walkfacts.go):
	// the children the template allows, and whether the template
	// prohibits it.
	allowed    []*CompiledNode
	prohibited bool
}

// Name returns the RM attribute name (e.g. "content", "data").
func (a *CompiledAttribute) Name() string { return a.name }

// Cardinality returns Single or Multiple. For implicit attributes
// the value is derived from the RM (containers → Multiple,
// otherwise Single).
func (a *CompiledAttribute) Cardinality() template.Cardinality { return a.cardinality }

// Existence returns the OPT-declared existence interval, or nil
// when the OPT was silent (and for implicit attributes). Existence
// answers "must this attribute carry at least one value?". For the
// min/max child count on a multi-valued attribute see
// [CompiledAttribute.ChildMultiplicity].
func (a *CompiledAttribute) Existence() *template.Multiplicity { return a.existence }

// ChildMultiplicity returns the AOM 1.4 CARDINALITY interval, the
// min/max number of child objects under a C_MULTIPLE_ATTRIBUTE.
// Returns nil for single attributes (no such block) and for
// multi-valued attributes whose OPT omitted <cardinality>. Walkers
// that need to flag too-few / too-many children should consult this
// alongside [CompiledAttribute.Existence].
func (a *CompiledAttribute) ChildMultiplicity() *template.Multiplicity {
	return a.childMultiplicity
}

// RMTypeName returns the RM type the BMM declares for this attribute
// on its parent (the element type for containers). When the BMM types
// the attribute with a formal generic parameter and the parent's RM
// type name supplies the actual one, the actual type is returned: the
// lower attribute of a DV_INTERVAL<DV_QUANTITY> node gives
// "DV_QUANTITY". On a parent named by its bare generic class no actual
// type is known, so the formal name comes back as the BMM writes it
// ("T" for the data attribute of an EVENT). Empty when the parent's
// class is outside the known RM universe or does not declare the
// attribute.
func (a *CompiledAttribute) RMTypeName() string { return a.rmTypeName }

// Implicit reports whether this attribute was injected by the
// compile step because the BMM declares it mandatory and the OPT
// omitted it. Implicit attributes carry no children; downstream
// composition builders fill them with RM defaults. One implicit
// attribute value is shared by every node of the same RM type that
// lacks it, so its pointer does not identify a node.
func (a *CompiledAttribute) Implicit() bool { return a.implicit }

// Required reports whether the BMM declares this attribute as
// mandatory on the parent type. True implies the composition
// builder must emit a value at this attribute's RM path.
func (a *CompiledAttribute) Required() bool { return a.required }

// Children returns a defensive copy of the child nodes. Empty for
// implicit attributes (no OPT-declared children) and for the
// trivial-leaf case where the OPT named the attribute but pinned no
// value.
func (a *CompiledAttribute) Children() []*CompiledNode { return slices.Clone(a.children) }
