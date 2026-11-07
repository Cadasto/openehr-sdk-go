package canxml

import (
	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlunmatched"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/internal/poly"
)

// ErrInvalidShape is the canxml-local sentinel reserved for XML-level
// shape errors (malformed XML, element-name mismatch on a
// non-polymorphic field, numeric overflow, `xmi:type` rejection).
// Only part of that range is produced today: decode returns it for an
// `xmi:type` discriminator on the wire (via [XSITypeOf]); for the text of
// a primitive element that does not decode as its type, such as a boolean
// written `maybe`, a number out of its type's range, a `TERM_MAPPING.match`
// that is not one character or a DV_MULTIMEDIA `data` that is not base64,
// inside a [DecodeError] that names the element (see [Unmarshal]); through
// [Unmarshal], for content before the root element other than the XML
// declaration, comments, processing instructions, whitespace and one
// document type declaration, and after it other than comments, processing
// instructions and whitespace, and for a processing instruction whose
// target is xml in any letter case anywhere but the XML declaration at the
// start, those three together with [ErrNotWellFormed]; and for an interval side written bounded with no bound where the
// bound is held by value. Encode returns it for a nil value or a value that
// does not implement xml.Marshaler. Other XML
// syntax errors come back unchanged from encoding/xml, so an errors.Is
// against this sentinel does not match a truncated root or a syntax error
// inside it.
//
// Polymorphic-discrimination errors come from the typereg package
// instead: match those with errors.Is against [typereg.ErrMissingType]
// / [typereg.ErrUnknownType] / [typereg.ErrTypeMismatch], not against
// this sentinel.
var ErrInvalidShape = xmlunmatched.ErrInvalidShape

// ErrNotWellFormed classifies the decode's own refusal of input that is not
// well-formed XML, apart from the syntax errors encoding/xml reports:
//
//   - an element that carries one attribute twice, by its name as written or
//     as the namespaces in scope resolve it;
//   - a processing instruction whose target is xml in any letter case
//     anywhere but the XML declaration at the very start, after a UTF-8 byte
//     order mark at most, whether before, inside or after the root;
//   - content before or after the root element that XML does not allow
//     there, such as text, an end element, a second root or markup
//     encoding/xml cannot read after the root, apart from such markup before
//     the root with nothing refused before it, which may be the root's own
//     start tag.
//
// Each such refusal wraps [ErrInvalidShape] as well, so a check against that
// sentinel keeps matching it. The error names the rule the input breaks and
// carries none of the input. A syntax error encoding/xml reports itself, such
// as a truncated root, comes back unchanged as an *xml.SyntaxError, which this
// sentinel does not match; nor does a document of the wrong shape, such as a
// primitive element whose text does not decode as its type.
var ErrNotWellFormed = xmlunmatched.ErrNotWellFormed

// ErrUnknownElement is the sentinel for content the decode does not read:
// an element or attribute that names no attribute of the RM type its
// element decodes as, other than namespace declarations and attributes in
// the XML Schema instance namespace (`xsi:type`, `xsi:schemaLocation`,
// `xsi:nil`, …), an element in a namespace other than the openEHR one
// (see [ErrNamespace]), and text other than XML whitespace directly inside
// an element whose content is elements. An element whose content this codec
// does not model, a `Hash` whose values are `Hash`es,
// counts as such content too.
//
// Such content is ignored by default. Only a decode given [WithStrict]
// refuses it: the refusal wraps this sentinel and names the item's path
// from the root of the value (see [WithUnmatched] for the form of a
// path, `/text()` last for text). The path carries the item's name but none
// of its value or text.
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

// ErrLimitExceeded is the sentinel for input past a limit of the decode. The
// limits this package applies are the depth [WithMaxDepth] sets and the depth
// encoding/xml decodes, 10 000 nested elements (see "Nesting depth" in the
// package documentation): an element nested deeper is refused with a
// [DecodeError] that wraps this sentinel and names the element's path, its
// depth and the limit, never its content. A reader that applies a limit of
// its own to canonical XML, such as one on the size of its input, can wrap it
// too, so that one errors.Is test matches each such refusal.
var ErrLimitExceeded = xmlunmatched.ErrLimitExceeded

// DecodeError is the unified error returned by the decoder at
// polymorphic dispatch sites, for the text of a primitive element that
// does not decode as its type and for an element nested deeper than
// [WithMaxDepth] allows or encoding/xml decodes. Re-exported from the
// internal poly helper so consumers can `errors.As` against a stable type
// without importing internal packages. It has the same shape as canjson's.
//
// Each one names its own slot. A failure inside a slot that sits in
// another slot comes back as one DecodeError per slot, each the Inner of
// the one outside it: errors.As finds the outermost, and the message
// names every slot from the outside in, then the failure itself.
//
// A primitive failure names the element instead, with no Type, and its
// Inner wraps [ErrInvalidShape] and the conversion's own error, such as a
// *strconv.NumError. Where the decode knows the element's path from the
// root of the value, the DecodeError comes back on its own with that path,
// such as "/ehr_status[1]/is_queryable[1]", in the form [WithUnmatched]
// uses; a [Decoder] given no option names the element alone, such as
// "/is_queryable", inside the envelope of each slot it sits in.
type DecodeError = poly.DecodeError
