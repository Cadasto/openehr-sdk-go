package template

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ParseOPT parses one ADL 1.4 operational template from r. It accepts
// an optional UTF-8 BOM and the standard openEHR OPT XSD element
// shape (root <template> in namespace http://schemas.openehr.org/v1).
//
// Returns ErrInvalidOPT (wrapped) for malformed XML or missing
// required wrapper fields (template_id, definition). In default
// (lenient) mode, unknown <children> xsi:type values are admitted as
// forward-compatible leaf *ComplexObject nodes; use ParseOPTStrict to
// reject unknown xsi:type values that carry nested attributes (i.e.
// values the lenient mode would silently flatten).
//
// An archetype root is read from xsi:type="C_ARCHETYPE_ROOT" and also
// from xsi:type="T_ARCHETYPE_ROOT", because some exporters write the
// second spelling. openEHR Template.xsd defines only C_ARCHETYPE_ROOT.
// Both spellings give the same *ArchetypeRoot, with its archetype id,
// optional template id, term definitions and full subtree, so neither
// is an unknown type.
//
// Nor is xsi:type="CONSTRAINT_REF": it is read as a leaf
// *ComplexObject whose PrimitiveConstraint is a
// [constraints.ConstraintRef] carrying its <reference>. A missing or
// blank reference leaves that constraint with an empty Reference.
//
// An attribute named null_flavor on an ELEMENT node, the US spelling some
// OPTs write, is read as null_flavour, the Reference Model attribute, so
// its constraint applies there, unless the node also declares
// null_flavour: null_flavor is then kept as written, and each attribute
// keeps its own constraint. ParseOPTStrict reads it the same way. Every
// other attribute name is kept as written, whether or not the RM class
// has it.
func ParseOPT(r io.Reader) (*OperationalTemplate, error) {
	return parseOPT(r, false)
}

// ParseOPTStrict is like ParseOPT but rejects any <children>
// xsi:type value the parser does not recognise when it carries
// nested <attributes>. Lenient mode would admit such a value as a
// leaf and silently drop the subtree. Use it for production
// validators that need to fail loudly on shapes outside the supported
// taxonomy (e.g. AOM 2 / ADL 2 inputs, primitive constraint trees).
// Returns ErrUnsupportedNode (wrapped) on the first such occurrence.
// It also refuses an unparseable C_INTEGER or C_REAL bound, an
// unparseable C_INTEGER or C_REAL list item, a C_PRIMITIVE_OBJECT
// with no <item> child, a CONSTRAINT_REF with no <reference> or
// a blank one, and an <attributes> or <children> inside <definition>,
// or an <item> of a C_PRIMITIVE_OBJECT, that has no xsi:type in the
// XML Schema instance namespace
// (http://www.w3.org/2001/XMLSchema-instance), returning an error that
// wraps ErrInvalidOPT. That last case covers a missing or empty type
// and an xsi prefix bound to another namespace: openEHR ITS-XML makes
// C_ATTRIBUTE, C_OBJECT and C_PRIMITIVE abstract, so each such element
// must name a concrete type. <definition> itself needs none. ParseOPT
// reads such an <attributes> as C_SINGLE_ATTRIBUTE, such a <children>
// as C_ARCHETYPE_ROOT when it carries an archetype id and as
// C_COMPLEX_OBJECT otherwise, and such an <item> as no constraint, so
// any constraint its type would have carried is lost.
//
// T_ARCHETYPE_ROOT and CONSTRAINT_REF are not unknown values, so
// strict mode never refuses them as unsupported. It reads
// T_ARCHETYPE_ROOT exactly as ParseOPT does. It reads a
// CONSTRAINT_REF as ParseOPT does, as a leaf carrying a
// [constraints.ConstraintRef], with one difference: where the
// reference is missing or blank, ParseOPT keeps the leaf with an
// empty Reference and strict mode refuses the template.
func ParseOPTStrict(r io.Reader) (*OperationalTemplate, error) {
	return parseOPT(r, true)
}

// ParseFile reads an .opt file from disk. The path suffix must be
// .opt (case-insensitive); other extensions return
// ErrNotOPTFile without opening the file.
func ParseFile(path string) (*OperationalTemplate, error) {
	return parseFile(path, false)
}

// ParseFileStrict is the strict-mode counterpart to ParseFile.
// It is equivalent to ParseFile + ParseOPTStrict; see ParseOPTStrict
// for the strict-mode contract.
func ParseFileStrict(path string) (*OperationalTemplate, error) {
	return parseFile(path, true)
}

func parseFile(path string, strict bool) (*OperationalTemplate, error) {
	if !strings.EqualFold(filepath.Ext(path), ".opt") {
		return nil, fmt.Errorf("%w: %s", ErrNotOPTFile, path)
	}
	f, err := os.Open(path) //nolint:gosec // callers control the path
	if err != nil {
		// Preserve fs.ErrNotExist (and peers) on the chain so
		// callers can errors.Is-classify, and attach the path for
		// debuggability when reports surface deep in a stack.
		return nil, fmt.Errorf("template: open %q: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read-only file
	return parseOPT(f, strict)
}

// maxOPTBytes is the maximum number of bytes parseOPT will read from
// an io.Reader before returning an "input too large" error. Default is
// 32 MiB. Unexported so tests in package template can lower it
// temporarily via t.Cleanup.
var maxOPTBytes int64 = 32 << 20

// maxOPTDepth is the maximum OPT node-tree nesting depth. Real OPTs stay
// well under 64 levels (COMPOSITION > SECTION > … > ELEMENT ≈ 5–20); the
// 128 cap gives generous headroom while bounding recursion against a
// crafted document. A var (not const) so tests can lower it.
var maxOPTDepth = 128

func parseOPT(r io.Reader, strict bool) (*OperationalTemplate, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: nil reader", ErrInvalidOPT)
	}
	// Cap the read with a one-byte margin: when the source holds more
	// than maxOPTBytes, the decoder drains the extra byte and lr.N
	// reaches 0, which we treat as "too large" rather than letting a
	// truncated read surface as a misleading malformed-XML error.
	lr := &io.LimitedReader{R: r, N: maxOPTBytes + 1}
	br := bufio.NewReader(lr)
	peek, peekErr := br.Peek(3)
	if peekErr != nil && !errors.Is(peekErr, io.EOF) {
		return nil, fmt.Errorf("%w: read header: %w", ErrInvalidOPT, peekErr)
	}
	if len(peek) == 3 && bytes.Equal(peek, []byte{0xEF, 0xBB, 0xBF}) {
		if _, err := br.Discard(3); err != nil {
			return nil, fmt.Errorf("%w: discard BOM: %w", ErrInvalidOPT, err)
		}
	}

	dec := xml.NewDecoder(br)
	var wire xmlTemplate
	if err := dec.Decode(&wire); err != nil {
		if lr.N == 0 {
			return nil, fmt.Errorf("%w: input exceeds %d bytes", ErrInvalidOPT, maxOPTBytes)
		}
		return nil, fmt.Errorf("%w: %w", ErrInvalidOPT, err)
	}
	// Forward-compat: defend against non-<template> documents that
	// somehow decoded (e.g. when the OPT XSD wrapper is renamed by a
	// downstream tool). xml.Decoder is permissive about root names
	// when the target struct does not pin a namespace.
	if wire.XMLName.Local != "" && wire.XMLName.Local != "template" {
		return nil, fmt.Errorf("%w: root element <%s>, expected <template>", ErrInvalidOPT, wire.XMLName.Local)
	}
	// Reject trailing non-whitespace tokens — a well-formed OPT has
	// exactly one root element; anything else after </template> means
	// the document is either malformed or carries multiple roots.
	if err := requireEOF(dec); err != nil {
		if lr.N == 0 {
			return nil, fmt.Errorf("%w: input exceeds %d bytes", ErrInvalidOPT, maxOPTBytes)
		}
		return nil, fmt.Errorf("%w: %w", ErrInvalidOPT, err)
	}
	// Authoritative size check: lr.N == 0 means the full maxOPTBytes+1
	// budget was consumed, i.e. the source is larger than the cap.
	if lr.N == 0 {
		return nil, fmt.Errorf("%w: input exceeds %d bytes", ErrInvalidOPT, maxOPTBytes)
	}
	if wire.TemplateID == nil || strings.TrimSpace(wire.TemplateID.Value) == "" {
		return nil, fmt.Errorf("%w: missing or empty template_id", ErrInvalidOPT)
	}
	if wire.Definition == nil {
		return nil, fmt.Errorf("%w: missing definition", ErrInvalidOPT)
	}

	root, err := buildNode(wire.Definition, strict, 0, &optPath{})
	if err != nil {
		// A builder error that already wraps ErrInvalidOPT carries its
		// text; wrapping it again would print the prefix twice. Any
		// other gains it here, with %w on both so errors.Is reaches the
		// builder sentinel (e.g. ErrUnsupportedNode) as well.
		if errors.Is(err, ErrInvalidOPT) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %w", ErrInvalidOPT, err)
	}

	tmpl := &OperationalTemplate{
		templateID:  strings.TrimSpace(wire.TemplateID.Value),
		concept:     strings.TrimSpace(wire.Concept),
		root:        root,
		annotations: collectAnnotations(wire.Annotations),
		description: descriptionFromWire(wire.Description),
	}
	if wire.UID != nil {
		tmpl.uid = strings.TrimSpace(wire.UID.Value)
	}
	if wire.Language != nil {
		tmpl.language = strings.TrimSpace(wire.Language.CodeString)
	}
	return tmpl, nil
}

// requireEOF advances the decoder past trailing whitespace and
// confirms io.EOF — i.e. there are no further root elements after
// </template>. Returns a wrapped error positioned at the offending
// token when extra content is present.
func requireEOF(dec *xml.Decoder) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("trailing content: %w", err)
		}
		switch t := tok.(type) {
		case xml.CharData:
			if strings.TrimSpace(string(t)) != "" {
				return errors.New("unexpected trailing text after root element")
			}
		case xml.Comment, xml.ProcInst, xml.Directive:
			// Permitted as trailing trivia.
		case xml.StartElement:
			return fmt.Errorf("unexpected trailing element <%s>", t.Name.Local)
		case xml.EndElement:
			// xml.Decoder closes the root element with an EndElement
			// before EOF — treat as benign.
		}
	}
}

// --- internal wire structs ----------------------------------------------

// xsi:type struct tags carry the full XSI namespace explicitly so
// that the decoder matches the attribute even when downstream tools
// rebind the `xsi` prefix or omit the declaration on inner elements.

type xmlTemplate struct {
	XMLName     xml.Name         `xml:"template"`
	Language    *xmlCodePhrase   `xml:"language"`
	UID         *xmlValueWrapper `xml:"uid"`
	TemplateID  *xmlValueWrapper `xml:"template_id"`
	Concept     string           `xml:"concept"`
	Description *xmlDescription  `xml:"description"`
	Definition  *xmlCObject      `xml:"definition"`
	Annotations []xmlAnnotation  `xml:"annotations"`
}

type xmlValueWrapper struct {
	Value string `xml:"value"`
}

type xmlCodePhrase struct {
	TerminologyID *xmlValueWrapper `xml:"terminology_id"`
	CodeString    string           `xml:"code_string"`
}

// xmlCObject is the union shape for any <definition> or <children>
// element. The Type attribute (xsi:type local name) discriminates;
// the buildNode dispatch interprets which fields are meaningful.
type xmlCObject struct {
	Type        string           `xml:"http://www.w3.org/2001/XMLSchema-instance type,attr"`
	RMTypeName  string           `xml:"rm_type_name"`
	NodeID      string           `xml:"node_id"`
	Occurrences *xmlInterval     `xml:"occurrences"`
	Attributes  []*xmlCAttribute `xml:"attributes"`
	// C_ARCHETYPE_ROOT extras — the archetype_id element wraps a
	// <value> child in the openEHR OPT shape, and so does the optional
	// template_id Template.xsd allows after it.
	ArchetypeID     string               `xml:"archetype_id>value"`
	TemplateID      string               `xml:"template_id>value"`
	TermDefinitions []xmlTermDefSection  `xml:"term_definitions"`
	TermBindings    []xmlTermBindSection `xml:"term_bindings"`
	// ARCHETYPE_SLOT extras (raw text — assertion grammar not
	// interpreted in v1)
	Includes []xmlAssertion `xml:"includes"`
	Excludes []xmlAssertion `xml:"excludes"`

	// REQ-103 primitive constraint payload — only some are
	// meaningful per xsi:type. The dispatch in buildPrimitive reads
	// only the fields relevant to the parent xsi:type.
	TrueValid        xmlBoolFlag            `xml:"true_valid"`
	FalseValid       xmlBoolFlag            `xml:"false_valid"`
	Range            *xmlNumericInterval    `xml:"range"`
	PrimitivePattern string                 `xml:"pattern"`
	PrimitiveList    []xmlPrimitiveListItem `xml:"list"`
	ListOpen         xmlBoolFlag            `xml:"list_open"`
	AssumedValue     string                 `xml:"assumed_value"`
	Property         *xmlCodePhraseRef      `xml:"property"`
	TerminologyID    *xmlValueWrapper       `xml:"terminology_id"`
	CodeList         []string               `xml:"code_list"`
	// TimezoneValidity is the timezone_validity of a C_DATE, C_TIME or
	// C_DATE_TIME, a VALIDITY_KIND, as written; "" when the OPT omits it.
	TimezoneValidity string `xml:"timezone_validity"`
	// Item is the inner constraint of a C_PRIMITIVE_OBJECT wrapper.
	// Populated only when Type == "C_PRIMITIVE_OBJECT"; the inner
	// element carries its own xsi:type (`C_BOOLEAN`, `C_INTEGER`,
	// `C_DURATION`, …). buildPrimitive recurses into Item when the
	// wrapper is named. Empty for all other shapes.
	Item *xmlCObject `xml:"item"`
	// Reference is the constraint code (e.g. "ac0001") of a
	// CONSTRAINT_REF, which names a value set outside the archetype.
	// Empty for all other shapes.
	Reference string `xml:"reference"`
}

// xmlTermDefSection is one <term_definitions code="..."> block on a
// C_ARCHETYPE_ROOT. Each block carries the term definition for a
// single at-code in the OPT's primary language; the ADL 1.4 OPT
// shape does not interleave language per block (the root OPT element
// pins one language for the whole document).
type xmlTermDefSection struct {
	Code  string        `xml:"code,attr"`
	Items []xmlTermItem `xml:"items"`
}

// xmlTermItem is one <items id="text|description|...">value</items>
// entry inside a term_definitions or term_bindings block.
type xmlTermItem struct {
	ID    string            `xml:"id,attr"`
	Code  string            `xml:"code,attr"`
	Value string            `xml:",chardata"`
	Coded *xmlCodePhraseRef `xml:"value"`
}

// xmlCodePhraseRef captures the <value><terminology_id><value>X</value></terminology_id><code_string>Y</code_string></value>
// shape used inside term_bindings items.
type xmlCodePhraseRef struct {
	TerminologyID *xmlValueWrapper `xml:"terminology_id"`
	CodeString    string           `xml:"code_string"`
}

// xmlTermBindSection is one <term_bindings terminology="..."> block
// on a C_ARCHETYPE_ROOT. Items inside bind an at-code (or path) to
// an external terminology code.
type xmlTermBindSection struct {
	Terminology string        `xml:"terminology,attr"`
	Items       []xmlTermItem `xml:"items"`
}

type xmlCAttribute struct {
	Type      string       `xml:"http://www.w3.org/2001/XMLSchema-instance type,attr"`
	Name      string       `xml:"rm_attribute_name"`
	Existence *xmlInterval `xml:"existence"`
	// Cardinality is the AOM 1.4 CARDINALITY block on
	// C_MULTIPLE_ATTRIBUTE. The is_ordered / is_unique flags are
	// declared but not retained by v1; only the interval (min/max
	// child count) is consumed downstream by the validator.
	Cardinality *xmlCardinality `xml:"cardinality"`
	Children    []*xmlCObject   `xml:"children"`
}

// xmlInterval is the whole-number interval of an occurrences, existence
// or cardinality block. All four flags are xmlBoolFlag values, so an
// omitted, empty or whitespace-only element stays unset and is never a
// decode error. The bounds are pointers, nil when the OPT omits them, so
// unboundedFlag can read an unset unbounded flag by its side's bound and
// includedFlag gives an unset included flag the default for its side.
type xmlInterval struct {
	Lower          *int        `xml:"lower"`
	Upper          *int        `xml:"upper"`
	LowerIncluded  xmlBoolFlag `xml:"lower_included"`
	UpperIncluded  xmlBoolFlag `xml:"upper_included"`
	LowerUnbounded xmlBoolFlag `xml:"lower_unbounded"`
	UpperUnbounded xmlBoolFlag `xml:"upper_unbounded"`
}

// xmlCardinality maps the AOM 1.4 CARDINALITY type — the
// child-count interval that decorates C_MULTIPLE_ATTRIBUTE alongside
// existence. Only the interval is captured in v1; is_ordered /
// is_unique are parsed-but-ignored (no validator dimension consumes
// them).
type xmlCardinality struct {
	IsOrdered bool         `xml:"is_ordered"`
	IsUnique  bool         `xml:"is_unique"`
	Interval  *xmlInterval `xml:"interval"`
}

// xmlAssertion is one <includes> or <excludes> blob of an
// ARCHETYPE_SLOT. InnerXML keeps the raw text the slot reports;
// Expressions holds the <expression> children the same decode reads,
// so the slot patterns need no second XML pass over InnerXML.
type xmlAssertion struct {
	InnerXML    string           `xml:",innerxml"`
	Expressions []slotExpression `xml:"expression"`
}

// xmlAnnotation captures one <annotations path="..."><items
// id="...">text</items>...</annotations> block. The Path attribute
// MAY be empty (the schema permits annotations without a path,
// targeting the OPT as a whole).
type xmlAnnotation struct {
	Path  string              `xml:"path,attr"`
	Items []xmlAnnotationItem `xml:"items"`
}

type xmlAnnotationItem struct {
	ID    string `xml:"id,attr"`
	Value string `xml:",chardata"`
}

// xmlDescription captures the top-level <description> block. Schema
// is RESOURCE_DESCRIPTION (cAM); we keep the most commonly queried
// fields plus other_details / details (passthrough). Richer access
// (translations, multi-language metadata) is deferred to a later
// REQ when a consumer surfaces.
type xmlDescription struct {
	OriginalAuthors []xmlIdentifiedValue `xml:"original_author"`
	LifecycleState  string               `xml:"lifecycle_state"`
	OtherDetails    []xmlIdentifiedValue `xml:"other_details"`
}

type xmlIdentifiedValue struct {
	ID    string `xml:"id,attr"`
	Value string `xml:",chardata"`
}

// --- wire → public node tree --------------------------------------------

// optPath is the place in the OPT tree of the node being built: one step
// per level from the root down to it. A parse keeps one, pushing a step as
// it descends into a child and popping it on the way back, so that a
// refusal can name where it is without the parse building a path for
// every node. The path it writes holds structural names only: attribute
// names and node or archetype ids, never a constraint value.
type optPath struct {
	steps []optStep
}

// optStep is one level of an [optPath].
type optStep struct {
	attr string // the attribute the node is a child of
	id   string // the node's archetype id, else its node id; "" for neither
}

// push descends into the child c of the attribute attr. A nil c, which
// buildNode refuses, has no id.
func (p *optPath) push(attr string, c *xmlCObject) {
	var id string
	if c != nil {
		id = strings.TrimSpace(c.ArchetypeID)
		if id == "" {
			id = strings.TrimSpace(c.NodeID)
		}
	}
	p.steps = append(p.steps, optStep{attr: attr, id: id})
}

// pop returns to the parent of the node push descended into.
func (p *optPath) pop() {
	p.steps = p.steps[:len(p.steps)-1]
}

// String writes the path of the node being built, "/" for the root, with
// each node's id as a predicate where it has one:
// /content[openEHR-EHR-ACTION.medication.v1]/ism_transition[at0002].
func (p *optPath) String() string {
	if len(p.steps) == 0 {
		return "/"
	}
	var b strings.Builder
	for _, s := range p.steps {
		b.WriteString("/" + s.attr)
		if s.id != "" {
			b.WriteString("[" + s.id + "]")
		}
	}
	return b.String()
}

// attribute returns the path of the attribute name of the node being
// built.
func (p *optPath) attribute(name string) string {
	if len(p.steps) == 0 {
		return "/" + name
	}
	return p.String() + "/" + name
}

// buildNode builds the node o, whose place in the OPT tree path holds.
func buildNode(o *xmlCObject, strict bool, depth int, path *optPath) (Node, error) {
	if o == nil {
		return nil, fmt.Errorf("%w: nil node", ErrInvalidOPT)
	}
	if depth > maxOPTDepth {
		return nil, fmt.Errorf("%w: node nesting exceeds %d levels", ErrInvalidOPT, maxOPTDepth)
	}
	// The OPT root <definition> element carries no xsi:type but
	// behaves as a C_ARCHETYPE_ROOT when an archetype_id is present
	// (the XSD declares the slot as the concrete type). Detect that
	// shape before the generic complex-object path.
	if (o.Type == "" || o.Type == "C_COMPLEX_OBJECT") && strings.TrimSpace(o.ArchetypeID) != "" {
		o.Type = "C_ARCHETYPE_ROOT"
	}
	switch o.Type {
	case "C_COMPLEX_OBJECT", "":
		return buildComplexObject(o, strict, depth, path)
	case "C_ARCHETYPE_ROOT", "T_ARCHETYPE_ROOT":
		// REQ-100: some exporters write T_ARCHETYPE_ROOT for the node
		// openEHR Template.xsd calls C_ARCHETYPE_ROOT. It is a second
		// spelling of the same shape, not an unknown type, so it builds
		// the same *ArchetypeRoot in both parse modes. Only this named
		// spelling is added: other unknown types still reach the
		// default branch below. Under either spelling, a child element
		// the parser does not map, such as one Template.xsd does not
		// define for C_ARCHETYPE_ROOT, is skipped in both modes, not
		// refused, so strict mode does not report it.
		co, err := buildComplexObject(o, strict, depth, path)
		if err != nil {
			return nil, err
		}
		return &ArchetypeRoot{
			archetypeID:   strings.TrimSpace(o.ArchetypeID),
			templateID:    strings.TrimSpace(o.TemplateID),
			ComplexObject: *co,
			terms:         collectTermDefs(o.TermDefinitions),
			termBindings:  collectTermBindings(o.TermBindings),
		}, nil
	case "ARCHETYPE_SLOT":
		includes := collectAssertions(o.Includes)
		excludes := collectAssertions(o.Excludes)
		return &Slot{
			rmTypeName:     o.RMTypeName,
			nodeID:         o.NodeID,
			includes:       includes,
			excludes:       excludes,
			parsedIncludes: parseSlotAssertions(o.Includes),
			parsedExcludes: parseSlotAssertions(o.Excludes),
		}, nil
	default:
		// REQ-103 primitive constraint types, CONSTRAINT_REF among
		// them, map to leaf ComplexObject nodes carrying a typed
		// PrimitiveConstraint. Unknown xsi:type values (outside the
		// closed set) still fall through as bare leaf ComplexObject —
		// the forward-compatibility escape hatch from REQ-100.
		//
		// In strict mode, an unknown xsi:type that carries nested
		// <attributes> means lenient mode would silently drop a
		// non-trivial subtree — that's a forward-compat hazard worth
		// surfacing for production validators. Primitive types never
		// carry <attributes>, so the strict check only fires on
		// genuinely unknown shapes.
		primitive, err := buildPrimitive(o, strict)
		if err != nil {
			return nil, fmt.Errorf("%s (%s): %w", o.NodeID, o.RMTypeName, err)
		}
		if strict && primitive == nil && len(o.Attributes) > 0 {
			return nil, fmt.Errorf("%w: unknown xsi:type=%q on %q with %d nested attributes (strict mode)",
				ErrUnsupportedNode, o.Type, o.RMTypeName, len(o.Attributes))
		}
		occ, err := intervalToMultiplicity(o.Occurrences)
		if err != nil {
			return nil, fmt.Errorf("occurrences on %q (%s): %w", o.NodeID, o.RMTypeName, err)
		}
		return &ComplexObject{
			rmTypeName:  o.RMTypeName,
			nodeID:      o.NodeID,
			occurrences: occ,
			primitive:   primitive,
		}, nil
	}
}

// buildComplexObject builds the node o, whose place in the OPT tree path
// holds.
func buildComplexObject(o *xmlCObject, strict bool, depth int, path *optPath) (*ComplexObject, error) {
	occ, err := intervalToMultiplicity(o.Occurrences)
	if err != nil {
		return nil, fmt.Errorf("occurrences on %q (%s): %w", o.NodeID, o.RMTypeName, err)
	}
	co := &ComplexObject{
		rmTypeName:  o.RMTypeName,
		nodeID:      o.NodeID,
		occurrences: occ,
	}
	for _, a := range o.Attributes {
		// REQ-100: C_ATTRIBUTE is abstract in ITS-XML, so an <attributes>
		// must name its concrete type in the XSI namespace. Strict mode
		// refuses one that does not; the default mode reads it as a
		// C_SINGLE_ATTRIBUTE (buildAttribute). The refusal names the
		// attribute's OPT path, structural names only.
		if strict && a.Type == "" {
			return nil, fmt.Errorf("%w: <attributes> at %q, of %s %q, has no xsi:type in the XSI namespace (strict mode)",
				ErrInvalidOPT, path.attribute(a.Name), o.RMTypeName, o.NodeID)
		}
		attr, err := buildAttribute(a, strict, depth, path)
		if err != nil {
			return nil, err
		}
		attr.name = attributeName(o, attr.name)
		co.attributes = append(co.attributes, attr)
	}
	co.nodeName = deriveNodeName(o.Attributes)
	return co, nil
}

// attributeName returns the name the parser gives an attribute the OPT
// names name on the node o. REQ-100: an ELEMENT's null_flavor, the US
// spelling some OPTs write, is read as the RM attribute null_flavour, so
// its constraint applies; that is the only name read otherwise than as
// written. Where the ELEMENT also declares null_flavour, null_flavor is
// kept as written, so the node does not declare null_flavour twice and
// each attribute keeps its own constraint.
func attributeName(o *xmlCObject, name string) string {
	if o.RMTypeName != "ELEMENT" || name != "null_flavor" {
		return name
	}
	if slices.ContainsFunc(o.Attributes, func(a *xmlCAttribute) bool { return a.Name == "null_flavour" }) {
		return name
	}
	return "null_flavour"
}

// deriveNodeName extracts the template-level node name (REQ-116): the
// fixed C_STRING the OPT pins on the node's name attribute — name →
// a DV_TEXT/DV_CODED_TEXT child → value → C_STRING whose list holds
// exactly one non-blank entry, taken trimmed, and is closed. Anything
// looser (no name attribute, an open pattern, a multi-entry list, an
// open list, a lone blank entry — a whitespace-only name is useless
// as a disambiguator) is not a *fixed* name and yields "" — the
// archetype concept term is never substituted. When the name attribute carries alternatives,
// the first child that pins a fixed value wins (a non-pinning or
// blank-pinning alternative falls through to the next), matching the
// first-alternative convention used across the parse and compile
// layers.
//
// The walk reads the *wire* structs, not the built tree: buildString
// drops blank <list> entries, so a built single-entry list can also
// be the remnant of a multi-entry choice (<list>A</list><list> </list>).
// Fixedness is a property of what the OPT declared — exactly one raw
// <list> element — so it is decided before that filtering.
func deriveNodeName(attrs []*xmlCAttribute) string {
	for _, a := range attrs {
		if a.Name != "name" {
			continue
		}
		for _, child := range a.Children {
			if name := fixedValueString(child); name != "" {
				return name
			}
		}
	}
	return ""
}

// fixedValueString returns the single fixed string pinned on the
// value attribute of a wire name-constraint node, or "" when the
// constraint does not pin exactly one raw non-blank <list> entry in a
// closed list. A list whose list_open is true admits other strings, so
// it pins nothing; an omitted or empty list_open leaves it closed.
// Handles both wire spellings: a C_PRIMITIVE_OBJECT wrapper carrying
// <item xsi:type="C_STRING">, and a direct C_STRING child.
func fixedValueString(o *xmlCObject) string {
	if o == nil {
		return ""
	}
	for _, a := range o.Attributes {
		if a.Name != "value" {
			continue
		}
		for _, child := range a.Children {
			leaf := child
			if leaf.Type == "C_PRIMITIVE_OBJECT" && leaf.Item != nil {
				leaf = leaf.Item
			}
			if leaf.Type != "C_STRING" || len(leaf.PrimitiveList) != 1 || leaf.ListOpen.orFalse() {
				continue
			}
			if s := strings.TrimSpace(leaf.PrimitiveList[0].Text); s != "" {
				return s
			}
		}
	}
	return ""
}

// buildAttribute builds the attribute a of the node whose place in the OPT
// tree path holds, which it leaves as it found it.
func buildAttribute(a *xmlCAttribute, strict bool, depth int, path *optPath) (*Attribute, error) {
	existence, err := intervalToMultiplicity(a.Existence)
	if err != nil {
		return nil, fmt.Errorf("existence on attribute %q: %w", a.Name, err)
	}
	attr := &Attribute{
		name:      a.Name,
		existence: existence,
	}
	switch a.Type {
	case "C_SINGLE_ATTRIBUTE", "":
		attr.cardinality = Single
		// C_SINGLE_ATTRIBUTE has no <cardinality> block per the AOM
		// 1.4 schema. Silently ignore the field if a non-conformant
		// wire payload supplies one.
	case "C_MULTIPLE_ATTRIBUTE":
		attr.cardinality = Multiple
		if a.Cardinality != nil {
			cm, err := intervalToMultiplicity(a.Cardinality.Interval)
			if err != nil {
				return nil, fmt.Errorf("cardinality on attribute %q: %w", a.Name, err)
			}
			attr.childMultiplicity = cm
		}
	default:
		return nil, fmt.Errorf("%w: attribute xsi:type=%q", ErrUnsupportedNode, a.Type)
	}
	for i, c := range a.Children {
		// REQ-100: C_OBJECT is abstract in ITS-XML, so a <children> must
		// name its concrete type in the XSI namespace. Strict mode refuses
		// one that does not; the default mode reads it as a
		// C_ARCHETYPE_ROOT or a C_COMPLEX_OBJECT (buildNode). The refusal
		// names the attribute's OPT path, structural names only.
		if strict && c != nil && c.Type == "" {
			return nil, fmt.Errorf("%w: <children> %d of the attribute at %q (%s %q) has no xsi:type in the XSI namespace (strict mode)",
				ErrInvalidOPT, i+1, path.attribute(a.Name), c.RMTypeName, c.NodeID)
		}
		path.push(a.Name, c)
		node, err := buildNode(c, strict, depth+1, path)
		path.pop()
		if err != nil {
			return nil, err
		}
		attr.children = append(attr.children, node)
	}
	return attr, nil
}

// unboundedFlag reads the lower_unbounded or upper_unbounded flag of the
// interval side named side, whose bound the OPT gives when hasBound
// (REQ-169). A flag the OPT writes is read as written. An omitted flag reads
// as false beside a bound and as true where the bound is omitted too, so an
// interval that gives only an upper bound is open below. A side the OPT
// writes bounded but gives no bound is refused: the parser holds every bound
// by value, so it has no value to give that side.
func unboundedFlag(flag xmlBoolFlag, hasBound bool, side string) (bool, error) {
	if !flag.set {
		return !hasBound, nil
	}
	if !flag.value && !hasBound {
		return false, fmt.Errorf("%s_unbounded is false but the interval gives no %s bound", side, side)
	}
	return flag.value, nil
}

// includedFlag reads the lower_included or upper_included flag of an
// interval. A flag the OPT writes is read as written. An omitted flag is
// a closed end on a bounded side and an open end on an unbounded side,
// the only value the BASE interval invariants allow there.
func includedFlag(flag *bool, unbounded bool) bool {
	if flag != nil {
		return *flag
	}
	return !unbounded
}

// intervalToMultiplicity returns the interval with each bounded side
// closed, or nil when the OPT omits the interval.
func intervalToMultiplicity(i *xmlInterval) (*Multiplicity, error) {
	if i == nil {
		return nil, nil
	}
	lowerUnbounded, err := unboundedFlag(i.LowerUnbounded, i.Lower != nil, "lower")
	if err != nil {
		return nil, err
	}
	upperUnbounded, err := unboundedFlag(i.UpperUnbounded, i.Upper != nil, "upper")
	if err != nil {
		return nil, err
	}
	lowerIncluded := includedFlag(i.LowerIncluded.ptr(), lowerUnbounded)
	upperIncluded := includedFlag(i.UpperIncluded.ptr(), upperUnbounded)
	lower, upper := intOrZero(i.Lower), intOrZero(i.Upper)
	m := &Multiplicity{
		lower:          lower,
		upper:          upper,
		lowerUnbounded: lowerUnbounded,
		upperUnbounded: upperUnbounded,
	}
	// A Multiplicity counts whole items, so an excluded end on a bounded
	// side becomes the closed end it stands for: ">0" is 1 and "<3" is 2.
	// An unbounded side has no bound to move, whatever its included flag
	// says. An int cannot hold the closed end of "> the largest int" or
	// "< the smallest int", so such an end is refused instead of being
	// left to wrap around to the other end of the int range.
	if !lowerUnbounded && !lowerIncluded {
		if m.lower == math.MaxInt {
			return nil, fmt.Errorf("closed lower bound overflows int: excluded lower=%d", lower)
		}
		m.lower++
	}
	if !upperUnbounded && !upperIncluded {
		if m.upper == math.MinInt {
			return nil, fmt.Errorf("closed upper bound overflows int: excluded upper=%d", upper)
		}
		m.upper--
	}
	// Reject inverted intervals at parse time so downstream walkers
	// can assume `Lower <= Upper` whenever both bounds are present.
	// The check runs on the closed ends, so an interval that closing
	// leaves empty, such as ">2..<3", is refused here too; the message
	// adds the interval as written so the reader recognises it.
	// Unbounded sides are skipped: they have no concrete value to compare.
	if !m.lowerUnbounded && !m.upperUnbounded && m.lower > m.upper {
		return nil, fmt.Errorf("inverted interval lower=%d > upper=%d (written %s)",
			m.lower, m.upper, writtenBounds(lower, upper, lowerIncluded, upperIncluded))
	}
	return m, nil
}

// intOrZero returns the bound p points to, or 0 for a bound the OPT omits.
func intOrZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// writtenBounds spells a bounded interval in ADL notation, with the
// bounds the OPT wrote and each end marked by its included flag:
// ">2..<3" when both ends are excluded, "2..3" when both are closed.
func writtenBounds(lower, upper int, lowerIncluded, upperIncluded bool) string {
	lowerMark, upperMark := "", ""
	if !lowerIncluded {
		lowerMark = ">"
	}
	if !upperIncluded {
		upperMark = "<"
	}
	return fmt.Sprintf("%s%d..%s%d", lowerMark, lower, upperMark, upper)
}

func collectAssertions(xs []xmlAssertion) []string {
	if len(xs) == 0 {
		return nil
	}
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if s := strings.TrimSpace(x.InnerXML); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// collectAnnotations groups the raw OPT <annotations> blocks by their
// path attribute. Annotations without a path attribute key under the
// empty string (template-wide). Empty <items> are dropped so callers
// do not see meaningless zero-value annotations.
func collectAnnotations(xs []xmlAnnotation) map[string][]Annotation {
	if len(xs) == 0 {
		return nil
	}
	out := make(map[string][]Annotation, len(xs))
	for _, x := range xs {
		for _, item := range x.Items {
			value := strings.TrimSpace(item.Value)
			if value == "" && strings.TrimSpace(item.ID) == "" {
				continue
			}
			out[x.Path] = append(out[x.Path], Annotation{
				ID:    strings.TrimSpace(item.ID),
				Value: value,
			})
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func descriptionFromWire(d *xmlDescription) *Description {
	if d == nil {
		return nil
	}
	desc := &Description{
		lifecycleState:  strings.TrimSpace(d.LifecycleState),
		originalAuthors: identifiedValuesToMap(d.OriginalAuthors),
		otherDetails:    identifiedValuesToMap(d.OtherDetails),
	}
	if desc.lifecycleState == "" && len(desc.originalAuthors) == 0 && len(desc.otherDetails) == 0 {
		return nil
	}
	return desc
}

func identifiedValuesToMap(vs []xmlIdentifiedValue) map[string]string {
	if len(vs) == 0 {
		return nil
	}
	out := make(map[string]string, len(vs))
	for _, v := range vs {
		id := strings.TrimSpace(v.ID)
		if id == "" {
			continue
		}
		out[id] = strings.TrimSpace(v.Value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// collectTermDefs flattens wire <term_definitions code="..."><items
// id="..."/></term_definitions> blocks into a per-at-code map keyed
// by at-code, value = map[itemID]itemValue (e.g. "at0000" →
// {"text": "Blood Pressure", "description": "..."}).
//
// The ADL 1.4 OPT shape ships one term_definitions block per
// at-code in the OPT's primary language; multi-language ontologies
// belong to the AOM 1.4 ARCHETYPE_ONTOLOGY block which v1 does not
// surface (the OPT carries a single canonical-language view).
func collectTermDefs(xs []xmlTermDefSection) map[string]ArchetypeTerm {
	if len(xs) == 0 {
		return nil
	}
	out := make(map[string]ArchetypeTerm, len(xs))
	for _, x := range xs {
		code := strings.TrimSpace(x.Code)
		if code == "" {
			continue
		}
		items := make(map[string]string, len(x.Items))
		for _, item := range x.Items {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				continue
			}
			items[id] = strings.TrimSpace(item.Value)
		}
		if len(items) == 0 {
			continue
		}
		out[code] = ArchetypeTerm{Code: code, Items: items}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// collectTermBindings flattens wire <term_bindings
// terminology="..."><items code="..."><value>...</value></items>...
// blocks into a flat slice of TermBinding records. The "code"
// attribute may be either a bare at-code (e.g. "at0013") or an
// AQL-like path (e.g. "/data[at0002]/events[at0005]/..."); callers
// MUST treat it opaquely until compile-time resolution lands.
func collectTermBindings(xs []xmlTermBindSection) []TermBinding {
	if len(xs) == 0 {
		return nil
	}
	var out []TermBinding
	for _, x := range xs {
		terminology := strings.TrimSpace(x.Terminology)
		for _, item := range x.Items {
			code := strings.TrimSpace(item.Code)
			if code == "" {
				continue
			}
			b := TermBinding{
				Terminology: terminology,
				NodeOrPath:  code,
			}
			if item.Coded != nil {
				if item.Coded.TerminologyID != nil {
					b.Target.TerminologyID = strings.TrimSpace(item.Coded.TerminologyID.Value)
				}
				b.Target.CodeString = strings.TrimSpace(item.Coded.CodeString)
			}
			out = append(out, b)
		}
	}
	return out
}
