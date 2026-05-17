package canxml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"reflect"
	"strings"
)

// Canonical XML namespaces — pinned per specs/wire.md § Canonical XML.
const (
	// NSDefault is the openEHR canonical-XML default namespace.
	NSDefault = "http://schemas.openehr.org/v1"
	// NSXSI is the XML Schema Instance namespace used to carry the
	// `xsi:type` polymorphic discriminator.
	NSXSI = "http://www.w3.org/2001/XMLSchema-instance"
)

// XSITypeAttrName returns the canonical attribute name for the
// `xsi:type` polymorphic discriminator. The Local is the literal
// "xsi:type" (with prefix-colon-suffix as a single Local token) so
// the stdlib encoding/xml encoder reproduces the canonical openEHR
// prefix instead of synthesising an auto-prefix from the namespace
// URI. The matching `xmlns:xsi="…"` declaration is emitted on the
// root by [Marshal] via [XSINamespaceDecl].
func XSITypeAttrName() xml.Name { return xml.Name{Local: "xsi:type"} }

// XSINamespaceDecl returns the canonical root-level xmlns:xsi
// declaration. Emitted by [Marshal] / [MarshalIndent] on the root
// element so descendants can carry `xsi:type="…"` with the
// canonical openEHR prefix.
func XSINamespaceDecl() xml.Attr {
	return xml.Attr{Name: xml.Name{Local: "xmlns:xsi"}, Value: NSXSI}
}

// BMMNamer is implemented by every concrete generated RM type. It
// returns the BMM class name used as the `xsi:type` value at
// polymorphic boundaries — the same identifier the type registry
// uses (REQ-040).
//
// Generated code asserts this interface inline at polymorphic
// emission sites; consumers rarely call it directly.
type BMMNamer interface {
	BMMName() string
}

// BMMNameOf extracts the BMM class discriminator from a value.
// Returns ("", false) when the value does not implement [BMMNamer]
// — i.e. it is not a generated RM type. Used by the encoder at
// polymorphic boundaries to construct the `xsi:type` attribute.
func BMMNameOf(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	n, ok := v.(BMMNamer)
	if !ok {
		return "", false
	}
	return n.BMMName(), true
}

// Marshal returns the canonical XML encoding of v. v MUST be a
// pointer to a generated RM type that implements [xml.Marshaler]
// (every concrete RM class generated under openehr/rm/ does so).
//
// Output is compact (no insignificant whitespace) so byte-equality
// tests are stable. Use [MarshalIndent] for human inspection only;
// indented output is NOT a round-trip-stable form.
//
// At the root, the encoder does not emit `xsi:type` — the caller
// already knows the concrete type. Polymorphic descendants carry
// `xsi:type` at every concrete value boundary inside the document.
func Marshal(v any) ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("canxml: %w: nil value", ErrInvalidShape)
	}
	m, ok := v.(xml.Marshaler)
	if !ok {
		return nil, fmt.Errorf("canxml: %w: %T does not implement xml.Marshaler", ErrInvalidShape, v)
	}
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	start := rootStartElement(v)
	if err := m.MarshalXML(enc, start); err != nil {
		return nil, err
	}
	if err := enc.Flush(); err != nil {
		return nil, fmt.Errorf("canxml: flush: %w", err)
	}
	return buf.Bytes(), nil
}

// MarshalIndent is like [Marshal] but applies prefix and indent to
// each element. Use for human inspection only — byte-stability tests
// compare against compact [Marshal] output.
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("canxml: %w: nil value", ErrInvalidShape)
	}
	m, ok := v.(xml.Marshaler)
	if !ok {
		return nil, fmt.Errorf("canxml: %w: %T does not implement xml.Marshaler", ErrInvalidShape, v)
	}
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	enc.Indent(prefix, indent)
	start := rootStartElement(v)
	if err := m.MarshalXML(enc, start); err != nil {
		return nil, err
	}
	if err := enc.Flush(); err != nil {
		return nil, fmt.Errorf("canxml: flush: %w", err)
	}
	return buf.Bytes(), nil
}

// rootStartElement builds the canonical root xml.StartElement for v.
// The element local name is the snake-cased BMM class name (e.g.
// `dv_quantity`) so the root element matches the canjson key
// convention. The default namespace `http://schemas.openehr.org/v1`
// is declared on the root via an explicit `xmlns` attribute (rather
// than the Name.Space slot, which auto-decorates child elements with
// the openEHR namespace prefix), plus `xmlns:xsi` so descendants can
// carry `xsi:type` with the canonical prefix.
//
// No `xsi:type` attribute is set at the root: the caller already
// knows the concrete type they handed in. Polymorphic descendants
// receive `xsi:type` from their parents.
func rootStartElement(v any) xml.StartElement {
	local := ""
	if n, ok := v.(BMMNamer); ok {
		local = ElementName(n.BMMName())
	}
	return xml.StartElement{
		Name: xml.Name{Local: local},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "xmlns"}, Value: NSDefault},
			XSINamespaceDecl(),
		},
	}
}

// ElementName converts a BMM class or property name to its canonical
// XML element local name. BMM class names are upper-snake_case
// (DV_QUANTITY); property names are already lower-snake_case
// (magnitude_status). Both lower-case verbatim — the snake_case
// shape is identical to canjson JSON keys per specs/wire.md.
func ElementName(bmmName string) string {
	return strings.ToLower(bmmName)
}

// EncodePoly writes one polymorphic child element to e. The element
// is wrapped under local name `name`, with `xsi:type="<BMMName>"`
// as its FIRST attribute, then v's [xml.Marshaler] body. v MUST
// implement both [BMMNamer] (every concrete generated RM type does)
// and [xml.Marshaler].
//
// Generator-emitted MarshalXML methods call this at every
// polymorphic field boundary; consumers rarely need it. Nil values
// are emitted as ABSENT (no element written). A nil-interface that
// boxes a typed-nil pointer (`var p *T = nil; var i I = p`) is also
// treated as ABSENT.
func EncodePoly(e *xml.Encoder, name string, v any) error {
	if isNilValue(v) {
		return nil
	}
	bn, ok := v.(BMMNamer)
	if !ok {
		return fmt.Errorf("canxml: EncodePoly %s: %T does not implement BMMNamer", name, v)
	}
	m, ok := v.(xml.Marshaler)
	if !ok {
		return fmt.Errorf("canxml: EncodePoly %s: %T does not implement xml.Marshaler", name, v)
	}
	start := xml.StartElement{
		Name: xml.Name{Local: name},
		Attr: []xml.Attr{{Name: XSITypeAttrName(), Value: bn.BMMName()}},
	}
	return m.MarshalXML(e, start)
}

// isNilValue reports whether v is a nil interface or a non-nil
// interface boxing a typed nil pointer / slice / map / channel /
// func. Used by [EncodePoly] to omit absent polymorphic fields.
func isNilValue(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		return rv.IsNil()
	}
	return false
}
