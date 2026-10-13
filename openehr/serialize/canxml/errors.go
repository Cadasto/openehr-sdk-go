package canxml

import (
	"errors"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlunmatched"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/internal/poly"
)

// ErrInvalidShape is the canxml-local sentinel reserved for XML-level
// shape errors (malformed XML, element-name mismatch on a
// non-polymorphic field, numeric overflow, `xmi:type` rejection).
// Only part of that range is produced today: decode returns it for an
// `xmi:type` discriminator on the wire (via [XSITypeOf]) and, through
// [Unmarshal], for content after the root element other than comments,
// processing instructions and whitespace, and for an interval side written
// bounded with no bound where the bound is held by value; encode returns it for a nil
// value or a value that does not implement xml.Marshaler. Other XML syntax errors come back
// unchanged from encoding/xml, so an errors.Is against this sentinel
// does not match a truncated root or a syntax error inside it.
//
// Polymorphic-discrimination errors come from the typereg package
// instead: match those with errors.Is against [typereg.ErrMissingType]
// / [typereg.ErrUnknownType] / [typereg.ErrTypeMismatch], not against
// this sentinel.
var ErrInvalidShape = errors.New("canxml: invalid XML shape")

// ErrUnknownElement is the sentinel for content the decode does not read:
// an element or attribute that names no attribute of the RM type its
// element decodes as, other than namespace declarations and attributes in
// the XML Schema instance namespace (`xsi:type`, `xsi:schemaLocation`,
// `xsi:nil`, …), and an element in a namespace other than the openEHR one
// (see [ErrNamespace]). An element whose content this codec does not
// model, such as a `Hash` member or a BMM `Any` member, counts as such
// content too.
//
// Such content is ignored by default. Only a decode given [WithStrict]
// refuses it: the refusal wraps this sentinel and names the item's path
// from the root of the value (see [WithUnmatched] for the form of a
// path). The path carries the item's name but none of its value.
var ErrUnknownElement = xmlunmatched.ErrUnknownElement

// ErrNamespace classifies an element in a namespace other than the openEHR
// one, `http://schemas.openehr.org/v1`. An element in no namespace counts
// as an openEHR element, since many producers declare no default namespace.
//
// A foreign element is unmatched content, even when its local name is that
// of an RM attribute: the decode ignores it by default, [WithUnmatched]
// reports it, and [WithStrict] refuses it with an error that wraps both
// [ErrUnknownElement] and this sentinel. An attribute in a foreign
// namespace is unmatched content too, but its refusal wraps
// [ErrUnknownElement] only.
//
// The root element is exempt: the decode does not match it by name, since
// the value the caller decodes into says what it decodes as, so its own
// namespace is not checked. Its children are: an element that inherits a
// foreign default namespace from the root is foreign.
var ErrNamespace = xmlunmatched.ErrNamespace

// DecodeError is the unified error returned by the decoder at
// polymorphic dispatch sites. Re-exported from the internal poly
// helper so consumers can `errors.As` against a stable type without
// importing internal packages. It has the same shape as canjson's.
//
// Each one names its own slot. A failure inside a slot that sits in
// another slot comes back as one DecodeError per slot, each the Inner of
// the one outside it: errors.As finds the outermost, and the message
// names every slot from the outside in, then the failure itself.
type DecodeError = poly.DecodeError
