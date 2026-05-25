package templatecompile

import (
	"maps"
	"slices"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// CompiledNode is one node in the compiled OPT tree. Mirrors the
// OPT's [template.Node] taxonomy (ComplexObject / ArchetypeRoot /
// Slot) collapsed into a single struct because walker code rarely
// cares about the wire-side discrimination — it cares about
// "what's the AQL path of this thing, what does it constrain, and
// can I descend".
//
// IsSlot distinguishes leaf slot-fill points from descendable
// objects; ArchetypeID is non-empty exactly when the OPT pinned an
// archetype id on this node (i.e. it was a *ArchetypeRoot on the
// wire side).
type CompiledNode struct {
	aqlPath      string
	rmTypeName   string
	nodeID       string
	archetypeID  string // empty unless this node was an *ArchetypeRoot on the wire
	occurrences  *template.Multiplicity
	attributes   []*CompiledAttribute
	parent       *CompiledNode
	isSlot       bool
	slotIncludes []string
	slotExcludes []string

	// primitive carries the typed REQ-103 constraint value when the
	// wire xsi:type was a primitive. Nil for non-primitive nodes
	// (composition root, archetype roots, slots, plain complex objects).
	primitive constraints.PrimitiveConstraint

	// terms is populated only on *ArchetypeRoot-derived nodes. At-codes
	// are scoped to their enclosing archetype root; the same at-code
	// can have different meanings under sibling roots. See [Term] for
	// the parent-walk lookup that respects scope.
	terms map[string]template.ArchetypeTerm
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
func (n *CompiledNode) IsSlot() bool { return n.isSlot }

// SlotIncludes returns a defensive copy of the slot's raw
// archetype-id include assertion strings. Empty for non-slot nodes.
func (n *CompiledNode) SlotIncludes() []string { return slices.Clone(n.slotIncludes) }

// SlotExcludes returns a defensive copy of the slot's raw
// archetype-id exclude assertion strings. Empty for non-slot nodes.
func (n *CompiledNode) SlotExcludes() []string { return slices.Clone(n.slotExcludes) }

// PrimitiveConstraint returns the typed REQ-103 constraint value for
// this node, or nil when the wire xsi:type was not a primitive in
// the closed set. Mirrors [template.ComplexObject.PrimitiveConstraint]
// — the compile step copies the value through without modification.
func (n *CompiledNode) PrimitiveConstraint() constraints.PrimitiveConstraint {
	return n.primitive
}

// Term returns the term definition for an at-code, scoped to the
// nearest enclosing archetype root. Walks the parent chain so a
// descendant of `/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]`
// sees that root's terminology rather than a sibling root's. Returns
// (zero, false) when no enclosing root defines the code.
//
// The returned [template.ArchetypeTerm.Items] map is a defensive copy.
func (n *CompiledNode) Term(code string) (template.ArchetypeTerm, bool) {
	for cur := n; cur != nil; cur = cur.parent {
		if t, ok := cur.terms[code]; ok {
			return template.ArchetypeTerm{Code: t.Code, Items: maps.Clone(t.Items)}, true
		}
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

// ChildMultiplicity returns the AOM 1.4 CARDINALITY interval — the
// min/max number of child objects under a C_MULTIPLE_ATTRIBUTE.
// Returns nil for single attributes (no such block) and for
// multi-valued attributes whose OPT omitted <cardinality>. Walkers
// that need to flag too-few / too-many children should consult this
// alongside [CompiledAttribute.Existence].
func (a *CompiledAttribute) ChildMultiplicity() *template.Multiplicity {
	return a.childMultiplicity
}

// RMTypeName returns the BMM-declared RM type of this attribute
// (the element type for containers). Empty when rminfo did not
// resolve the parent class (rare — only when the parent type is
// outside the known RM universe).
func (a *CompiledAttribute) RMTypeName() string { return a.rmTypeName }

// Implicit reports whether this attribute was injected by the
// compile step because the BMM declares it mandatory and the OPT
// omitted it. Implicit attributes carry no children — downstream
// composition builders fill them with RM defaults.
func (a *CompiledAttribute) Implicit() bool { return a.implicit }

// Required reports whether the BMM declares this attribute as
// mandatory on the parent type. True implies the composition
// builder MUST emit a value at this attribute's RM path.
func (a *CompiledAttribute) Required() bool { return a.required }

// Children returns a defensive copy of the child nodes. Empty for
// implicit attributes (no OPT-declared children) and for the
// trivial-leaf case where the OPT named the attribute but pinned no
// value.
func (a *CompiledAttribute) Children() []*CompiledNode { return slices.Clone(a.children) }
