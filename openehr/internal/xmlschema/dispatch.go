package xmlschema

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlunmatched"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// nsXMI is the XMI namespace, whose `xmi:type` the decoder refuses.
const nsXMI = "http://www.omg.org/XMI"

// Discriminated is what the discriminator of an element says about the type
// the element decodes as.
type Discriminated struct {
	// Attr names the attribute read as the discriminator, for
	// xmlunmatched.DiscriminatorRead; the zero name when there is none.
	Attr xml.Name
	// Name is the type the element decodes as: an openEHR class name, or the
	// BMM primitive an XML Schema built-in datatype reads as; empty when the
	// element carries no discriminator.
	Name string
	// Written is the discriminator as written, its prefix included.
	Written string
	// Builtin reports that Name is the BMM primitive of the XML Schema
	// built-in datatype Written names.
	Builtin bool
}

// Discriminator returns what the discriminator of start says: its `xsi:type`,
// in the XSI namespace or written literally, or else a bare `type` attribute,
// so that a bare `type` beside an `xsi:type` stays unmatched content. An
// `xmi:type` is refused with xmlunmatched.ErrInvalidShape.
//
// A value with no prefix names an openEHR class. A prefixed value names an XML
// Schema built-in datatype, and reads as its BMM primitive, when its prefix is
// bound to the XML Schema namespace ([NS]) where the attribute stands, or is
// the literal `xsd` bound by no declaration in scope ([LiteralPrefix]); any
// other prefixed value is kept as written, which names no type. A value whose
// local name is no built-in datatype is kept as written whatever its prefix,
// so only a built-in's prefix is resolved: against the declarations in scope
// that dec keeps ([xmlunmatched.LookupPrefix]), or, when dec is nil, against
// those start carries itself. When dec does not keep the declarations around
// start, as a plain encoding/xml decoder does not, and start does not declare
// the prefix itself, Discriminator refuses the value with
// [xmlunmatched.UnresolvedPrefix].
func Discriminator(dec *xml.Decoder, start xml.StartElement) (Discriminated, error) {
	bare := -1
	for i, attr := range start.Attr {
		// Reject xmi:type (both resolved and literal forms).
		if (attr.Name.Local == "type" && attr.Name.Space == nsXMI) || attr.Name.Local == "xmi:type" {
			return Discriminated{}, fmt.Errorf("canxml: %w: xmi:type on the wire is rejected (use xsi:type)", xmlunmatched.ErrInvalidShape)
		}
		if !xmlunmatched.IsDiscriminator(attr.Name) {
			continue
		}
		// Accept xsi:type (resolved and literal forms) at once; keep a bare
		// type for when there is none.
		if attr.Name.Space != "" || attr.Name.Local != "type" {
			return resolveType(dec, start, attr)
		}
		if bare < 0 {
			bare = i
		}
	}
	if bare >= 0 {
		return resolveType(dec, start, start.Attr[bare])
	}
	return Discriminated{}, nil
}

// resolveType returns what attr, the discriminator of start, says, as
// [Discriminator] reads it.
func resolveType(dec *xml.Decoder, start xml.StartElement, attr xml.Attr) (Discriminated, error) {
	d := Discriminated{Attr: attr.Name, Name: attr.Value, Written: attr.Value}
	prefix, local, prefixed := strings.Cut(attr.Value, ":")
	if !prefixed {
		return d, nil
	}
	primitive, ok := Builtin(local)
	if !ok {
		return d, nil
	}
	uri, bound, known := xmlunmatched.LookupPrefix(dec, start, prefix)
	if !known && dec != nil {
		return Discriminated{}, xmlunmatched.UnresolvedPrefix()
	}
	if (bound && uri == NS) || (!bound && prefix == LiteralPrefix) {
		d.Name, d.Builtin = primitive, true
	}
	return d, nil
}

// LookupType returns the constructor of the class an `xsi:type` of typeName
// reads as: the class registered in typereg.Default under typeName, or else
// the one registered under the name typeName is a type alias of
// ([RegisterTypeAlias]).
func LookupType(typeName string) (func() any, bool) {
	if ctor, ok := typereg.Default.Lookup(typeName); ok {
		return ctor, true
	}
	name, ok := AliasOf(typeName)
	if !ok {
		return nil, false
	}
	return typereg.Default.Lookup(name)
}

// Enclose returns the *typereg.DecodeError for err, a failure while the
// element at path decoded as typeName (empty when no discriminator chose the
// type).
//
// A failure that is itself a DecodeError comes from a polymorphic slot further
// in. It becomes the Inner of the new one as it is, so a failure deep inside
// nested slots is a chain of envelopes, one per slot, whose message is written
// once, when it is asked for. Wrapping it in a formatted error instead would
// write out the whole message below at every slot on the way out, at a cost
// that grows with the square of the depth. Any other failure keeps the canxml
// prefix.
func Enclose(path, typeName string, err error) *typereg.DecodeError {
	if de, ok := err.(*typereg.DecodeError); ok && de != nil { //nolint:errorlint // a direct envelope only: a wrapped one keeps its wrapper, prefixed below
		return &typereg.DecodeError{Path: path, Type: typeName, Inner: de}
	}
	return &typereg.DecodeError{Path: path, Type: typeName, Inner: fmt.Errorf("canxml: %w", err)}
}
