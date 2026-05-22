package template

import (
	"maps"
	"slices"
	"strconv"
)

// OperationalTemplate is a parsed ADL 1.4 operational template (OPT).
// Construct via ParseOPT or ParseFile; the zero value is not useful.
type OperationalTemplate struct {
	templateID  string
	concept     string
	uid         string
	language    string
	root        Node
	annotations map[string][]Annotation
	description *Description
}

// TemplateID returns the value of <template_id>/<value> from the OPT
// (e.g. "vital_signs"). Required by REQ-100; non-empty after a
// successful parse.
func (t *OperationalTemplate) TemplateID() string { return t.templateID }

// Concept returns the value of <concept> from the OPT (the
// machine-readable concept slug). Empty when the OPT omits it.
func (t *OperationalTemplate) Concept() string { return t.concept }

// UID returns the value of <uid>/<value> from the OPT, or an empty
// string when the OPT does not carry a UID.
func (t *OperationalTemplate) UID() string { return t.uid }

// Language returns the ISO 639-1 code from <language>/<code_string>,
// or an empty string when absent.
func (t *OperationalTemplate) Language() string { return t.language }

// Description returns the parsed top-level <description> block, or
// nil when the OPT omits it (or carries only empty sub-elements).
// The returned pointer is owned by the OperationalTemplate — callers
// MUST NOT mutate the map values it exposes.
func (t *OperationalTemplate) Description() *Description { return t.description }

// Annotations returns the parsed <annotations path="..."> blocks,
// keyed by the path attribute (empty string when the annotation has
// no path). Returns nil when the OPT carries no annotations or only
// empty ones. The returned map is a defensive copy: map mutation
// does not affect the underlying template (the Annotation slice
// headers and their entries are still shared with the OPT).
func (t *OperationalTemplate) Annotations() map[string][]Annotation {
	return maps.Clone(t.annotations)
}

// Annotation is one <items id="..."> entry inside an <annotations>
// block. Annotations carry UI / editor hints in the OPT and are
// addressable by path (an AQL-style locator string). The format is
// open-ended in the OPT XSD — consumers interpret IDs by convention.
type Annotation struct {
	// ID is the items/@id attribute (e.g. "name", "comment", "ui-hint").
	ID string
	// Value is the raw character data of the <items> element, trimmed
	// of surrounding whitespace.
	Value string
}

// Description is the parsed top-level <description> block. The OPT
// XSD models it as a RESOURCE_DESCRIPTION, of which v1 captures the
// most frequently consumed fields. Translations and per-language
// details are deferred to a later REQ.
type Description struct {
	lifecycleState  string
	originalAuthors map[string]string
	otherDetails    map[string]string
}

// LifecycleState returns the OPT's <lifecycle_state> value (e.g.
// "initial", "in_review", "published", "unmanaged"). Empty when
// the OPT omits the element.
func (d *Description) LifecycleState() string {
	if d == nil {
		return ""
	}
	return d.lifecycleState
}

// OriginalAuthors returns the parsed <original_author id="...">
// attribute map (e.g. {"name": "Alice", "organisation": "Acme"}).
// Returns nil when the OPT omits the element. The returned map is
// a defensive copy: caller mutation does not affect the underlying
// Description.
func (d *Description) OriginalAuthors() map[string]string {
	if d == nil {
		return nil
	}
	return maps.Clone(d.originalAuthors)
}

// OtherDetails returns the parsed <other_details id="..."> attribute
// map. These are open-ended provenance fields (e.g. "licence",
// "sem_ver", "build_uid"). Returns nil when absent. The returned
// map is a defensive copy.
func (d *Description) OtherDetails() map[string]string {
	if d == nil {
		return nil
	}
	return maps.Clone(d.otherDetails)
}

// Root returns the root definition node. Its RMTypeName is the
// composition RM class (conventionally "COMPOSITION"). The concrete
// type is *ArchetypeRoot for OPTs whose <definition> declares an
// archetype id (the typical Ocean Template Designer shape), and
// *ComplexObject for OPTs without an explicit root archetype id.
// Callers that need to descend into attributes should type-switch
// on both *ArchetypeRoot and *ComplexObject, or use OperationalTemplate.NodeAt.
func (t *OperationalTemplate) Root() Node { return t.root }

// Node is the sealed root interface for OPT definition-tree nodes.
// Implementations are *ComplexObject, *ArchetypeRoot, *Attribute, and
// *Slot. The interface is closed; new concrete types may appear in a
// future REQ but only within this package.
//
// Callers that walk the tree and need to distinguish descendable
// objects from attribute carriers should match against ObjectNode
// (covers *ComplexObject + *ArchetypeRoot) rather than re-listing
// the two concrete types.
type Node interface {
	// RMTypeName returns the openEHR Reference Model class name this
	// node constrains (e.g. "COMPOSITION", "DV_QUANTITY"). For an
	// *Attribute node it returns the empty string — attributes are
	// not RM-typed.
	RMTypeName() string

	// NodeID returns the archetype node id (e.g. "at0001") when one
	// is set on this node; otherwise the empty string. *Attribute
	// nodes always return the empty string.
	NodeID() string

	isNode()
}

// ObjectNode is the supertype of the two descendable OPT node kinds
// — *ComplexObject and *ArchetypeRoot. Walker code that does not
// need to discriminate between archetype-root and bare complex-object
// should type-switch on ObjectNode instead of listing the two
// concrete types separately. *Slot and *Attribute are NOT
// ObjectNodes (a slot is a leaf with opaque slot-fill semantics; an
// attribute holds an RM attribute name and its children rather than
// being a typed object).
type ObjectNode interface {
	Node
	// Attributes returns the OPT-declared child attributes in
	// document order. The returned slice is a defensive copy; see
	// ComplexObject.Attributes.
	Attributes() []*Attribute
	// Occurrences returns the parsed occurrences interval, or nil
	// when the OPT did not declare one for this node.
	Occurrences() *Multiplicity
}

// Multiplicity is the min/max interval that OPT uses for both
// existence and occurrences blocks. Fields are unexported to keep
// parsed intervals immutable from outside the package — construct
// values only via the parser (no public constructor exists in v1).
type Multiplicity struct {
	lower          int
	upper          int
	lowerUnbounded bool
	upperUnbounded bool
}

// Lower returns the lower bound of the interval.
func (m Multiplicity) Lower() int { return m.lower }

// Upper returns the upper bound of the interval.
func (m Multiplicity) Upper() int { return m.upper }

// LowerUnbounded reports whether the lower bound is "*" / unbounded
// in the OPT XML.
func (m Multiplicity) LowerUnbounded() bool { return m.lowerUnbounded }

// UpperUnbounded reports whether the upper bound is "*" / unbounded
// in the OPT XML.
func (m Multiplicity) UpperUnbounded() bool { return m.upperUnbounded }

// Cardinality tags an Attribute as single-valued or multi-valued.
type Cardinality int

const (
	// Single corresponds to xsi:type="C_SINGLE_ATTRIBUTE".
	Single Cardinality = iota
	// Multiple corresponds to xsi:type="C_MULTIPLE_ATTRIBUTE".
	Multiple
)

// String returns "single" or "multiple"; out-of-range values render
// as "cardinality(N)" for diagnostic readability.
func (c Cardinality) String() string {
	switch c {
	case Single:
		return "single"
	case Multiple:
		return "multiple"
	default:
		return "cardinality(" + strconv.Itoa(int(c)) + ")"
	}
}

// IsValid reports whether c is one of the recognised Cardinality
// constants. Useful for guard assertions in walker code that build
// Cardinality values from external input.
func (c Cardinality) IsValid() bool {
	return c == Single || c == Multiple
}

// ComplexObject is xsi:type="C_COMPLEX_OBJECT" in the OPT XML and
// also the embedded payload of *ArchetypeRoot. It is used for both
// internal nodes (with child attributes) and leaf primitive
// constraints (e.g. CODE_PHRASE, DV_QUANTITY) which appear without
// child attributes in v1.
type ComplexObject struct {
	rmTypeName  string
	nodeID      string
	occurrences *Multiplicity
	attributes  []*Attribute
}

// RMTypeName implements Node.
func (c *ComplexObject) RMTypeName() string { return c.rmTypeName }

// NodeID implements Node.
func (c *ComplexObject) NodeID() string { return c.nodeID }

// Occurrences returns the parsed occurrences block, or nil when the
// OPT did not declare one for this node.
func (c *ComplexObject) Occurrences() *Multiplicity { return c.occurrences }

// Attributes returns a defensive copy of the child attributes in OPT
// document order. Slice mutation does not affect the underlying tree;
// the *Attribute pointers themselves are still shared with the OPT.
func (c *ComplexObject) Attributes() []*Attribute { return slices.Clone(c.attributes) }

func (c *ComplexObject) isNode() {}

// ArchetypeRoot is xsi:type="C_ARCHETYPE_ROOT" in the OPT XML — a
// ComplexObject decorated with an archetype id. The archetype id is
// the slot fill within the template (e.g.
// "openEHR-EHR-OBSERVATION.blood_pressure.v1").
type ArchetypeRoot struct {
	archetypeID string
	ComplexObject
}

// ArchetypeID returns the slot-fill archetype identifier.
func (a *ArchetypeRoot) ArchetypeID() string { return a.archetypeID }

func (a *ArchetypeRoot) isNode() {}

// Attribute is xsi:type="C_SINGLE_ATTRIBUTE" or "C_MULTIPLE_ATTRIBUTE".
// Implements Node; an Attribute's RMTypeName and NodeID are always
// empty (attributes are not RM-typed and do not carry archetype
// node ids).
type Attribute struct {
	name        string
	cardinality Cardinality
	existence   *Multiplicity
	children    []Node
}

// Name returns the RM attribute name (e.g. "content", "data").
func (a *Attribute) Name() string { return a.name }

// Cardinality returns Single or Multiple per the OPT xsi:type.
func (a *Attribute) Cardinality() Cardinality { return a.cardinality }

// Existence returns the parsed existence block, or nil when the OPT
// did not declare one.
func (a *Attribute) Existence() *Multiplicity { return a.existence }

// Children returns a defensive copy of the child nodes in OPT
// document order. The Node pointers themselves are still shared with
// the OPT.
//
// Children are constrained by the OPT tree shape to be one of
// *ComplexObject, *ArchetypeRoot, or *Slot — never another
// *Attribute. Walker code that needs to descend may type-switch on
// ObjectNode (covers ComplexObject + ArchetypeRoot) and treat *Slot
// as a leaf.
func (a *Attribute) Children() []Node { return slices.Clone(a.children) }

// RMTypeName implements Node and always returns the empty string —
// attributes are not RM-typed.
func (*Attribute) RMTypeName() string { return "" }

// NodeID implements Node and always returns the empty string.
func (*Attribute) NodeID() string { return "" }

func (a *Attribute) isNode() {}

// Slot is xsi:type="ARCHETYPE_SLOT". Includes and Excludes carry the
// archetype-id assertion strings as raw text from the OPT; v1 does
// not interpret the assertion grammar.
type Slot struct {
	rmTypeName string
	nodeID     string
	includes   []string
	excludes   []string
}

// RMTypeName implements Node — typically the slot-constrained RM
// class name (e.g. "OBSERVATION", "SECTION").
func (s *Slot) RMTypeName() string { return s.rmTypeName }

// NodeID implements Node.
func (s *Slot) NodeID() string { return s.nodeID }

// Includes returns a defensive copy of the raw archetype-id include
// assertion strings.
func (s *Slot) Includes() []string { return slices.Clone(s.includes) }

// Excludes returns a defensive copy of the raw archetype-id exclude
// assertion strings.
func (s *Slot) Excludes() []string { return slices.Clone(s.excludes) }

func (s *Slot) isNode() {}
