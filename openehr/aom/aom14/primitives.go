package aom14

import (
	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlschema"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// Wire-flexible BMM primitives re-exported from rm for generated AOM fields.
type (
	// Real is the wire-flexible BMM real primitive, aliased from rm.
	Real = rm.Real
	// Integer is the wire-flexible BMM integer primitive, aliased from rm.
	Integer = rm.Integer
)

// TypedString is the String an [ExprLeaf] Item holds where canonical XML gives
// the item an XML Schema built-in datatype, as in
// `<item xsi:type="xs:string">archetype_id/value</item>`: Value is the text,
// and Type the `xsi:type` as written, its prefix included, such as `xs:string`
// or `xsd:string`, which the encoder writes back, declaring the prefix bound
// to the XML Schema namespace. An Item read with no `xsi:type` holds its text
// as a plain Go string, and one whose `xsi:type` names a class, such as
// `C_STRING`, a pointer to a value of the class, such as a *[CString].
//
// In canonical JSON a TypedString is an object holding its Value and Type,
// `{"value":"archetype_id/value","xsi_type":"xs:string"}`, with no `_type`;
// a plain string is a JSON string, and a class value carries its `_type`.
type TypedString = xmlschema.TypedString
