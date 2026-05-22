package template

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ParseOPT parses one ADL 1.4 operational template from r. It accepts
// an optional UTF-8 BOM and the standard Ocean Template Designer XSD
// element shape (root <template> in namespace
// http://schemas.openehr.org/v1). REQ-100.
//
// Returns ErrInvalidOPT (wrapped) for malformed XML or missing
// required wrapper fields (template_id, definition).
func ParseOPT(r io.Reader) (*OperationalTemplate, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: nil reader", ErrInvalidOPT)
	}
	br := bufio.NewReader(r)
	if peek, _ := br.Peek(3); bytes.Equal(peek, []byte{0xEF, 0xBB, 0xBF}) {
		_, _ = br.Discard(3)
	}

	dec := xml.NewDecoder(br)
	var wire xmlTemplate
	if err := dec.Decode(&wire); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidOPT, err)
	}
	if wire.TemplateID == nil || strings.TrimSpace(wire.TemplateID.Value) == "" {
		return nil, fmt.Errorf("%w: missing or empty template_id", ErrInvalidOPT)
	}
	if wire.Definition == nil {
		return nil, fmt.Errorf("%w: missing definition", ErrInvalidOPT)
	}

	root, err := buildNode(wire.Definition)
	if err != nil {
		// Use %w for the inner error so errors.Is reaches the
		// builder sentinel (e.g. ErrUnsupportedNode) through the
		// outer ErrInvalidOPT wrap.
		return nil, fmt.Errorf("%w: %w", ErrInvalidOPT, err)
	}

	tmpl := &OperationalTemplate{
		templateID: strings.TrimSpace(wire.TemplateID.Value),
		concept:    strings.TrimSpace(wire.Concept),
		root:       root,
	}
	if wire.UID != nil {
		tmpl.uid = strings.TrimSpace(wire.UID.Value)
	}
	if wire.Language != nil {
		tmpl.language = strings.TrimSpace(wire.Language.CodeString)
	}
	return tmpl, nil
}

// ParseFile reads an .opt file from disk. The path suffix MUST be
// .opt (case-insensitive) per REQ-100; other extensions return
// ErrNotOPTFile without opening the file.
func ParseFile(path string) (*OperationalTemplate, error) {
	if !strings.EqualFold(filepath.Ext(path), ".opt") {
		return nil, fmt.Errorf("%w: %s", ErrNotOPTFile, path)
	}
	f, err := os.Open(path) //nolint:gosec // callers control the path
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only file
	return ParseOPT(f)
}

// --- internal wire structs ----------------------------------------------

type xmlTemplate struct {
	XMLName    xml.Name         `xml:"template"`
	Language   *xmlCodePhrase   `xml:"language"`
	UID        *xmlValueWrapper `xml:"uid"`
	TemplateID *xmlValueWrapper `xml:"template_id"`
	Concept    string           `xml:"concept"`
	Definition *xmlCObject      `xml:"definition"`
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
	Type        string           `xml:"type,attr"`
	RMTypeName  string           `xml:"rm_type_name"`
	NodeID      string           `xml:"node_id"`
	Occurrences *xmlInterval     `xml:"occurrences"`
	Attributes  []*xmlCAttribute `xml:"attributes"`
	// C_ARCHETYPE_ROOT extras — the archetype_id element wraps a
	// <value> child in the Ocean OPT shape.
	ArchetypeID string `xml:"archetype_id>value"`
	// ARCHETYPE_SLOT extras (raw text — assertion grammar not
	// interpreted in v1)
	Includes []xmlAssertion `xml:"includes"`
	Excludes []xmlAssertion `xml:"excludes"`
}

type xmlCAttribute struct {
	Type      string        `xml:"type,attr"`
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

// --- wire → public node tree --------------------------------------------

func buildNode(o *xmlCObject) (Node, error) {
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
		return buildComplexObject(o)
	case "C_ARCHETYPE_ROOT":
		co, err := buildComplexObject(o)
		if err != nil {
			return nil, err
		}
		return &ArchetypeRoot{archetypeID: o.ArchetypeID, ComplexObject: *co}, nil
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
		// deferred to a later REQ.
		return &ComplexObject{
			rmTypeName:  o.RMTypeName,
			nodeID:      o.NodeID,
			occurrences: intervalToMultiplicity(o.Occurrences),
		}, nil
	}
}

func buildComplexObject(o *xmlCObject) (*ComplexObject, error) {
	co := &ComplexObject{
		rmTypeName:  o.RMTypeName,
		nodeID:      o.NodeID,
		occurrences: intervalToMultiplicity(o.Occurrences),
	}
	for _, a := range o.Attributes {
		attr, err := buildAttribute(a)
		if err != nil {
			return nil, err
		}
		co.attributes = append(co.attributes, attr)
	}
	return co, nil
}

func buildAttribute(a *xmlCAttribute) (*Attribute, error) {
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
		node, err := buildNode(c)
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
		Lower:          i.Lower,
		Upper:          i.Upper,
		LowerUnbounded: i.LowerUnbounded,
		UpperUnbounded: i.UpperUnbounded,
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
