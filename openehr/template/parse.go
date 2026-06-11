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

	root, err := buildNode(wire.Definition, strict, 0)
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
	ArchetypeID     string               `xml:"archetype_id>value"`
	TermDefinitions []xmlTermDefSection  `xml:"term_definitions"`
	TermBindings    []xmlTermBindSection `xml:"term_bindings"`
	// ARCHETYPE_SLOT extras (raw text — assertion grammar not
	// interpreted in v1)
	Includes []xmlAssertion `xml:"includes"`
	Excludes []xmlAssertion `xml:"excludes"`

	// REQ-103 primitive constraint payload — only some are
	// meaningful per xsi:type. The dispatch in buildPrimitive reads
	// only the fields relevant to the parent xsi:type.
	TrueValid        *bool                  `xml:"true_valid"`
	FalseValid       *bool                  `xml:"false_valid"`
	Range            *xmlNumericInterval    `xml:"range"`
	PrimitivePattern string                 `xml:"pattern"`
	PrimitiveList    []xmlPrimitiveListItem `xml:"list"`
	AssumedValue     string                 `xml:"assumed_value"`
	Property         *xmlCodePhraseRef      `xml:"property"`
	TerminologyID    *xmlValueWrapper       `xml:"terminology_id"`
	CodeList         []string               `xml:"code_list"`
	// Item is the inner constraint of a C_PRIMITIVE_OBJECT wrapper.
	// Populated only when Type == "C_PRIMITIVE_OBJECT"; the inner
	// element carries its own xsi:type (`C_BOOLEAN`, `C_INTEGER`,
	// `C_DURATION`, …). buildPrimitive recurses into Item when the
	// wrapper is named. Empty for all other shapes.
	Item *xmlCObject `xml:"item"`
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

type xmlInterval struct {
	Lower          int  `xml:"lower"`
	Upper          int  `xml:"upper"`
	LowerUnbounded bool `xml:"lower_unbounded"`
	UpperUnbounded bool `xml:"upper_unbounded"`
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

func buildNode(o *xmlCObject, strict bool, depth int) (Node, error) {
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
		return buildComplexObject(o, strict, depth)
	case "C_ARCHETYPE_ROOT":
		co, err := buildComplexObject(o, strict, depth)
		if err != nil {
			return nil, err
		}
		return &ArchetypeRoot{
			archetypeID:   strings.TrimSpace(o.ArchetypeID),
			ComplexObject: *co,
			terms:         collectTermDefs(o.TermDefinitions),
			termBindings:  collectTermBindings(o.TermBindings),
		}, nil
	case "ARCHETYPE_SLOT":
		return &Slot{
			rmTypeName: o.RMTypeName,
			nodeID:     o.NodeID,
			includes:   collectAssertions(o.Includes),
			excludes:   collectAssertions(o.Excludes),
		}, nil
	default:
		// REQ-103 primitive constraint types map to leaf
		// ComplexObject nodes carrying a typed PrimitiveConstraint.
		// Unknown xsi:type values (outside the v1 closed set) still
		// fall through as bare leaf ComplexObject — the
		// forward-compatibility escape hatch from REQ-100.
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

func buildComplexObject(o *xmlCObject, strict bool, depth int) (*ComplexObject, error) {
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
		attr, err := buildAttribute(a, strict, depth)
		if err != nil {
			return nil, err
		}
		co.attributes = append(co.attributes, attr)
	}
	return co, nil
}

func buildAttribute(a *xmlCAttribute, strict bool, depth int) (*Attribute, error) {
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
	for _, c := range a.Children {
		node, err := buildNode(c, strict, depth+1)
		if err != nil {
			return nil, err
		}
		attr.children = append(attr.children, node)
	}
	return attr, nil
}

func intervalToMultiplicity(i *xmlInterval) (*Multiplicity, error) {
	if i == nil {
		return nil, nil
	}
	// Reject inverted intervals at parse time so downstream walkers
	// can assume `Lower <= Upper` whenever both bounds are present.
	// Unbounded sides are skipped — by definition they have no
	// concrete value to compare.
	if !i.LowerUnbounded && !i.UpperUnbounded && i.Lower > i.Upper {
		return nil, fmt.Errorf("inverted interval lower=%d > upper=%d", i.Lower, i.Upper)
	}
	return &Multiplicity{
		lower:          i.Lower,
		upper:          i.Upper,
		lowerUnbounded: i.LowerUnbounded,
		upperUnbounded: i.UpperUnbounded,
	}, nil
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
