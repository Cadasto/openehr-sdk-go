package template

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ParseOPT parses one ADL 1.4 operational template from r. It accepts
// an optional UTF-8 BOM and the standard openEHR OPT XSD element
// shape (root <template> in namespace http://schemas.openehr.org/v1).
// REQ-100.
//
// Returns ErrInvalidOPT (wrapped) for malformed XML or missing
// required wrapper fields (template_id, definition). In default
// (lenient) mode, unknown <children> xsi:type values are admitted as
// forward-compatible leaf *ComplexObject nodes; use ParseOPTStrict to
// reject unknown xsi:type values that carry nested attributes (i.e.
// values the lenient mode would silently flatten).
func ParseOPT(r io.Reader) (*OperationalTemplate, error) {
	return parseOPT(r, false)
}

// ParseOPTStrict is like ParseOPT but rejects any <children>
// xsi:type value the parser does not recognise when it carries
// nested <attributes> — those are values the lenient mode would
// admit as a leaf and silently drop the subtree. Use for production
// validators that need to fail loudly on shapes outside the v1
// taxonomy (e.g. AOM 2 / ADL 2 inputs, primitive constraint trees).
// Returns ErrUnsupportedNode (wrapped) on first such occurrence.
func ParseOPTStrict(r io.Reader) (*OperationalTemplate, error) {
	return parseOPT(r, true)
}

// ParseFile reads an .opt file from disk. The path suffix MUST be
// .opt (case-insensitive) per REQ-100; other extensions return
// ErrNotOPTFile without opening the file.
func ParseFile(path string) (*OperationalTemplate, error) {
	return parseFile(path, false)
}

// ParseFileStrict is the strict-mode counterpart to ParseFile.
// Equivalent to ParseFile + ParseOPTStrict — see ParseOPTStrict for
// the strict-mode contract.
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

func parseOPT(r io.Reader, strict bool) (*OperationalTemplate, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: nil reader", ErrInvalidOPT)
	}
	br := bufio.NewReader(r)
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
		return nil, fmt.Errorf("%w: %w", ErrInvalidOPT, err)
	}
	if wire.TemplateID == nil || strings.TrimSpace(wire.TemplateID.Value) == "" {
		return nil, fmt.Errorf("%w: missing or empty template_id", ErrInvalidOPT)
	}
	if wire.Definition == nil {
		return nil, fmt.Errorf("%w: missing definition", ErrInvalidOPT)
	}

	root, err := buildNode(wire.Definition, strict)
	if err != nil {
		// Use %w for the inner error so errors.Is reaches the
		// builder sentinel (e.g. ErrUnsupportedNode) through the
		// outer ErrInvalidOPT wrap.
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
				return fmt.Errorf("unexpected trailing text after root element")
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
	// <value> child in the openEHR OPT shape.
	ArchetypeID string `xml:"archetype_id>value"`
	// ARCHETYPE_SLOT extras (raw text — assertion grammar not
	// interpreted in v1)
	Includes []xmlAssertion `xml:"includes"`
	Excludes []xmlAssertion `xml:"excludes"`
}

type xmlCAttribute struct {
	Type      string        `xml:"http://www.w3.org/2001/XMLSchema-instance type,attr"`
	Name      string        `xml:"rm_attribute_name"`
	Existence *xmlInterval  `xml:"existence"`
	Children  []*xmlCObject `xml:"children"`
}

type xmlInterval struct {
	Lower          int  `xml:"lower"`
	Upper          int  `xml:"upper"`
	LowerUnbounded bool `xml:"lower_unbounded"`
	UpperUnbounded bool `xml:"upper_unbounded"`
}

type xmlAssertion struct {
	InnerXML string `xml:",innerxml"`
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

func buildNode(o *xmlCObject, strict bool) (Node, error) {
	if o == nil {
		return nil, fmt.Errorf("%w: nil node", ErrInvalidOPT)
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
		return buildComplexObject(o, strict)
	case "C_ARCHETYPE_ROOT":
		co, err := buildComplexObject(o, strict)
		if err != nil {
			return nil, err
		}
		return &ArchetypeRoot{archetypeID: strings.TrimSpace(o.ArchetypeID), ComplexObject: *co}, nil
	case "ARCHETYPE_SLOT":
		return &Slot{
			rmTypeName: o.RMTypeName,
			nodeID:     o.NodeID,
			includes:   collectAssertions(o.Includes),
			excludes:   collectAssertions(o.Excludes),
		}, nil
	default:
		// Forward-compatible: unknown xsi:type values (e.g.
		// C_PRIMITIVE_OBJECT, C_CODE_PHRASE, C_DV_QUANTITY) are
		// surfaced as leaf ComplexObject nodes carrying the RM
		// type name. Primitive constraint introspection is
		// deferred to a later REQ (REQ-103).
		//
		// In strict mode, an unknown xsi:type that carries nested
		// <attributes> means lenient mode would silently drop a
		// non-trivial subtree — that's a forward-compat hazard worth
		// surfacing for production validators.
		if strict && len(o.Attributes) > 0 {
			return nil, fmt.Errorf("%w: unknown xsi:type=%q on %q with %d nested attributes (strict mode)",
				ErrUnsupportedNode, o.Type, o.RMTypeName, len(o.Attributes))
		}
		return &ComplexObject{
			rmTypeName:  o.RMTypeName,
			nodeID:      o.NodeID,
			occurrences: intervalToMultiplicity(o.Occurrences),
		}, nil
	}
}

func buildComplexObject(o *xmlCObject, strict bool) (*ComplexObject, error) {
	co := &ComplexObject{
		rmTypeName:  o.RMTypeName,
		nodeID:      o.NodeID,
		occurrences: intervalToMultiplicity(o.Occurrences),
	}
	for _, a := range o.Attributes {
		attr, err := buildAttribute(a, strict)
		if err != nil {
			return nil, err
		}
		co.attributes = append(co.attributes, attr)
	}
	return co, nil
}

func buildAttribute(a *xmlCAttribute, strict bool) (*Attribute, error) {
	attr := &Attribute{
		name:      a.Name,
		existence: intervalToMultiplicity(a.Existence),
	}
	switch a.Type {
	case "C_SINGLE_ATTRIBUTE", "":
		attr.cardinality = Single
	case "C_MULTIPLE_ATTRIBUTE":
		attr.cardinality = Multiple
	default:
		return nil, fmt.Errorf("%w: attribute xsi:type=%q", ErrUnsupportedNode, a.Type)
	}
	for _, c := range a.Children {
		node, err := buildNode(c, strict)
		if err != nil {
			return nil, err
		}
		attr.children = append(attr.children, node)
	}
	return attr, nil
}

func intervalToMultiplicity(i *xmlInterval) *Multiplicity {
	if i == nil {
		return nil
	}
	return &Multiplicity{
		lower:          i.Lower,
		upper:          i.Upper,
		lowerUnbounded: i.LowerUnbounded,
		upperUnbounded: i.UpperUnbounded,
	}
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
