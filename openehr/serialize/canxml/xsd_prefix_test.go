package canxml_test

// REQ-056 (wire.md § REQ-056): a prefixed `xsi:type` names an XML Schema
// built-in datatype when its prefix is bound, where the attribute stands, to
// the XML Schema namespace, or is the literal `xsd:` no declaration binds;
// every other prefixed value is refused with typereg.ErrUnknownType. Each
// mode resolves the prefix against every declaration in scope.

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// xsdNamespace is the XML Schema namespace.
const xsdNamespace = "http://www.w3.org/2001/XMLSchema"

// prefixCase is one prefixed discriminator, written with the namespace
// declarations of the root, of the parent of the element that carries it, and
// of that element itself.
type prefixCase struct {
	name                    string
	value                   string
	root, parent, onElement string
}

// prefixCases are the prefix forms of wire.md § REQ-056, each with the type
// name it reads as: the BMM primitive of an XML Schema built-in, or the value
// as written, which names no type.
var prefixCases = []struct {
	prefixCase
	want string
}{
	{prefixCase{name: "xs: bound on the element", value: "xs:string", onElement: ` xmlns:xs="` + xsdNamespace + `"`}, "String"},
	{prefixCase{name: "xs: bound on an ancestor", value: "xs:string", parent: ` xmlns:xs="` + xsdNamespace + `"`}, "String"},
	{prefixCase{name: "xsd: bound on the root", value: "xsd:string", root: ` xmlns:xsd="` + xsdNamespace + `"`}, "String"},
	{prefixCase{name: "an unbound literal xsd:", value: "xsd:string"}, "String"},
	{prefixCase{name: "xsd: bound to another namespace", value: "xsd:string", root: ` xmlns:xsd="urn:example:other"`}, "xsd:string"},
	{prefixCase{name: "xs: bound on the root, rebound on an ancestor", value: "xs:string", root: ` xmlns:xs="` + xsdNamespace + `"`, parent: ` xmlns:xs="urn:example:other"`}, "xs:string"},
	{prefixCase{name: "ns2: bound to the openEHR namespace", value: "ns2:string", root: ` xmlns:ns2="http://schemas.openehr.org/v1"`}, "ns2:string"},
	{prefixCase{name: "ns2: an RM class", value: "ns2:DV_TEXT", root: ` xmlns:ns2="http://schemas.openehr.org/v1"`}, "ns2:DV_TEXT"},
}

// folderWithNameType returns a FOLDER whose sub-folder's name, a DV_TEXT slot,
// carries the discriminator c names, with c's declarations.
func folderWithNameType(c prefixCase) []byte {
	return fmt.Appendf(nil, `<folder xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"%s>`+
		`<name><value>f</value></name><archetype_node_id>a</archetype_node_id>`+
		`<folders%s><name%s xsi:type="%s"><value>x</value></name><archetype_node_id>b</archetype_node_id></folders>`+
		`</folder>`, c.root, c.parent, c.onElement, c.value)
}

// decodeModes are the ways a caller decodes canonical XML: Unmarshal and a
// Decoder, each with no option and with each option that tracks the decode.
func decodeModes(t *testing.T) map[string]func(data []byte, v any) error {
	t.Helper()
	unmarshal := func(opts ...canxml.DecoderOption) func([]byte, any) error {
		return func(data []byte, v any) error { return canxml.Unmarshal(data, v, opts...) }
	}
	decoder := func(opts ...canxml.DecoderOption) func([]byte, any) error {
		return func(data []byte, v any) error { return canxml.NewDecoder(bytes.NewReader(data), opts...).Decode(v) }
	}
	report := canxml.WithUnmatched(func(p string) { t.Errorf("reported %s; want nothing", p) })
	missing := canxml.WithMissing(func(string) {})
	return map[string]func([]byte, any) error{
		"Unmarshal":                 unmarshal(),
		"Unmarshal WithUnmatched":   unmarshal(report),
		"Unmarshal WithStrict":      unmarshal(canxml.WithStrict()),
		"Unmarshal WithMissing":     unmarshal(missing),
		"Decoder":                   decoder(),
		"Decoder WithUnmatched":     decoder(report),
		"Decoder WithStrict":        decoder(canxml.WithStrict()),
		"Decoder WithMissing":       decoder(missing),
		"Unmarshal WithStrict, all": unmarshal(canxml.WithStrict(), report, missing),
	}
}

// TestREQ056_PrefixedDiscriminatorResolvesInScope pins the prefix rule at a
// polymorphic slot, in every mode: the discriminator reads as the BMM
// primitive of an XML Schema built-in where its prefix is bound to the XML
// Schema namespace, on the element, an ancestor or the root, and where it is
// the literal `xsd:` no declaration binds, and as written otherwise; an RM
// slot holds no BMM primitive, so each is an unknown type, which the
// DecodeError names as it read it.
func TestREQ056_PrefixedDiscriminatorResolvesInScope(t *testing.T) {
	for mode, decode := range decodeModes(t) {
		for _, tc := range prefixCases {
			err := decode(folderWithNameType(tc.prefixCase), new(rm.Folder))
			if !errors.Is(err, typereg.ErrUnknownType) {
				t.Errorf("%s, %s: err = %v; want typereg.ErrUnknownType", mode, tc.name, err)
				continue
			}
			de, ok := errors.AsType[*canxml.DecodeError](err)
			if !ok || de == nil {
				t.Errorf("%s, %s: err = %v; want a *canxml.DecodeError", mode, tc.name, err)
				continue
			}
			if de.Type != tc.want {
				t.Errorf("%s, %s: DecodeError.Type = %q; want %q", mode, tc.name, de.Type, tc.want)
			}
		}
	}
}
