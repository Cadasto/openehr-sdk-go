package template

// OperationalTemplate is a parsed ADL 1.4 operational template (OPT).
// Construct via ParseOPT or ParseFile; the zero value is not useful.
type OperationalTemplate struct {
	templateID string
	concept    string
	uid        string
	language   string
	root       Node
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

// Multiplicity is the min/max interval that OPT uses for both
// existence and occurrences blocks.
type Multiplicity struct {
	Lower          int
	Upper          int
	LowerUnbounded bool
	UpperUnbounded bool
}

// Cardinality tags an Attribute as single-valued or multi-valued.
type Cardinality int

const (
	// Single corresponds to xsi:type="C_SINGLE_ATTRIBUTE".
	Single Cardinality = iota
	// Multiple corresponds to xsi:type="C_MULTIPLE_ATTRIBUTE".
	Multiple
)

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

// Attributes returns the child attributes in OPT document order.
// The returned slice MUST NOT be mutated by callers.
func (c *ComplexObject) Attributes() []*Attribute { return c.attributes }

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

// Children returns the child nodes in OPT document order.
func (a *Attribute) Children() []Node { return a.children }

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

// Includes returns the raw archetype-id include assertion strings.
func (s *Slot) Includes() []string { return s.includes }

// Excludes returns the raw archetype-id exclude assertion strings.
func (s *Slot) Excludes() []string { return s.excludes }

func (s *Slot) isNode() {}
