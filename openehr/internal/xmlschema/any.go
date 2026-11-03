package xmlschema

import (
	"encoding/xml"
	"fmt"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlunmatched"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// TypedString is a String a BMM `Any` member holds where its canonical XML
// gives it an XML Schema built-in datatype, such as the AOM 1.4
// `<item xsi:type="xs:string">archetype_id/value</item>` of an EXPR_LEAF:
// Value is the element's text, and Type the `xsi:type` as written, its prefix
// included, such as `xs:string` or `xsd:string`, which the encoder writes
// back, declaring the prefix it uses bound to the XML Schema namespace. An
// item with no `xsi:type` holds its text as a plain Go string instead.
type TypedString struct {
	// Value is the text of the element.
	Value string
	// Type is the element's `xsi:type`, an XML Schema built-in datatype under
	// a prefix, such as `xs:string`.
	Type string
}

// MarshalXML writes s as the element start names: its text, with Type as its
// `xsi:type` and a declaration that binds Type's prefix to the XML Schema
// namespace. A Type that is no prefixed XML Schema built-in datatype, which
// would not read back as one, is refused with xmlunmatched.ErrInvalidShape.
func (s TypedString) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if !isTypedStringType(s.Type) {
		return fmt.Errorf("%w: the xsi:type of a typed string is no prefixed XML Schema built-in datatype", xmlunmatched.ErrInvalidShape)
	}
	prefix, _, _ := strings.Cut(s.Type, ":")
	start.Attr = append(slices.Clip(start.Attr),
		xml.Attr{Name: xml.Name{Local: "xmlns:" + prefix}, Value: NS},
		xml.Attr{Name: xml.Name{Local: "xsi:type"}, Value: s.Type},
	)
	return e.EncodeElement(s.Value, start)
}

// DecodeAny decodes the element start opens, the value of a BMM `Any` member
// such as the AOM 1.4 `EXPR_LEAF.item`, as the type its discriminator names
// ([Discriminator]), and returns the value. start must be the token the
// decoder read last.
//
//   - A class the type registry holds, or a type alias names, such as
//     `C_STRING`: a pointer to a new value of the class, which reads the
//     element as at a polymorphic slot.
//   - An XML Schema built-in datatype, such as `xs:string`: a *TypedString
//     holding the element's text and the `xsi:type` as written.
//   - No discriminator: the element's text, as a Go string.
//
// Any other discriminator fails as at a polymorphic slot, with a
// *typereg.DecodeError wrapping typereg.ErrUnknownType. The text of an
// element read as a String is read as a primitive's is: a tracked decode
// reports an attribute or child element on it as unmatched content.
func DecodeAny(dec *xml.Decoder, start xml.StartElement) (any, error) {
	path := "/" + start.Name.Local
	d, err := Discriminator(dec, start)
	if err != nil {
		return nil, &typereg.DecodeError{Path: path, Inner: err}
	}
	if d.Name == "" || d.Builtin {
		xmlunmatched.DiscriminatorRead(dec, d.Attr)
		var text string
		if err := xmlunmatched.DecodeElement(dec, &start, &text); err != nil {
			return nil, err
		}
		if d.Name == "" {
			return text, nil
		}
		return &TypedString{Value: text, Type: d.Written}, nil
	}
	ctor, ok := LookupType(d.Name)
	if !ok {
		return nil, &typereg.DecodeError{Path: path, Type: d.Name, Inner: fmt.Errorf("canxml: %q: %w", d.Name, typereg.ErrUnknownType)}
	}
	xmlunmatched.DiscriminatorRead(dec, d.Attr)
	v := ctor()
	if err := xmlunmatched.DecodeElement(dec, &start, v); err != nil {
		return nil, Enclose(path, d.Name, err)
	}
	return v, nil
}
