// Package canxml implements the openEHR canonical XML codec for the
// SDK's generated Reference Model types. It is the XML counterpart of
// [github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson].
//
// The codec is a thin orchestration layer over stdlib encoding/xml:
// the per-RM-type [MarshalXML] and [UnmarshalXML] methods that the BMM
// code generator emits do the work, together with the [BMMName]
// discriminator method shared with the type registry. This package only
// exposes the public entry points, the [BMMNamer] interface used at
// polymorphic boundaries, and a shared error type ([DecodeError]).
//
// # Dependencies
//
// Consumers import this package directly, e.g.
//
//	import "github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
//
// The codec is usable without the HTTP client, auth, transport, or any
// other SDK subsystem. The only dependencies are the generated RM types
// and the type registry under
// [github.com/cadasto/openehr-sdk-go/openehr/rm/typereg].
//
// # Wire profile
//
// The codec produces deterministic canonical XML:
//
//   - One root element per document. [Unmarshal] refuses an element, or
//     text other than whitespace, after the root, and text, an end element
//     or markup other than one document type declaration before it, in
//     every mode, with [ErrInvalidShape]. Comments, processing
//     instructions and whitespace are accepted on either side of the root,
//     as XML allows them, and so are, before it, the XML declaration, one
//     document type declaration and a UTF-8 byte order mark at the very
//     start.
//   - Default namespace: `http://schemas.openehr.org/v1`.
//   - `xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"` is
//     declared on the document root whenever any descendant carries
//     `xsi:type`.
//   - Polymorphic discrimination: `xsi:type="<BMM_CLASS_NAME>"`
//     (unprefixed openEHR class name). Three AOM 1.4 classes are
//     written with their name in the AM 1.4 schema instead
//     (C_CODE_PHRASE for C_CODED_TEXT, C_DV_ORDINAL for C_ORDINAL,
//     C_DV_QUANTITY for C_QUANTITY), and either name decodes. The
//     encoder emits it as the first attribute on every concrete
//     element at a polymorphic site. The decoder requires it at polymorphic sites;
//     [WithRelaxedTypeDispatch], which would relax that, is accepted but
//     not yet enforced. Discriminator values are
//     matched unprefixed; namespace-prefixed values (the Better/Marand
//     `xsi:type="ns2:DV_QUANTITY"` dialect) are not supported and fail
//     closed with [typereg.ErrUnknownType].
//   - Element local names are snake_case BMM property/class names
//     (identical to canjson JSON keys; e.g. `dv_quantity`,
//     `magnitude_status`).
//   - Child elements follow the `xs:sequence` of the class's type in
//     the openEHR ITS-XML schemas (RM and BASE Release-1.1.0, AM
//     Release-1.4), or of its nearest ancestor's type when the schemas
//     have none, with the elements of the type it extends first. A
//     property the schemas do not declare, one a later release added
//     such as `EHR.tags`, comes after the declared ones. The decoder
//     accepts children in any order.
//   - Nil-pointer optional fields and empty containers with
//     `cardinality.lower == 0` are emitted as absent (no element).
//     Both absent and an empty self-closing element are accepted on
//     decode, for optionals and containers; an interface-typed member
//     (below) needs its `xsi:type` even when empty.
//   - ISO 8601 dates/times/durations are passed through as element
//     text content; the codec does not parse them.
//   - Numeric magnitudes use IEEE 754 double-precision (same as
//     canonical JSON). A Real element whose text is a lossy literal,
//     one the float64 it parses to does not give back at the literal's
//     own count of significant digits, fails decode as a primitive
//     failure (see Primitive failures below). Element text is not
//     quoted: a quoted number is text that does not spell its type.
//   - Compact XML (no insignificant inter-element whitespace) is the
//     byte-equality target for round-trip tests. The encoder always
//     emits compact form; [MarshalIndent] is for human inspection
//     only.
//   - `xmi:type` is rejected on decode with [ErrInvalidShape]; only
//     `xsi:type` is recognised (openEHR ITS-XML uses XMI in UML
//     diagrams, not on the wire).
//   - An Octet list, DV_MULTIMEDIA's `data` and `integrity_check`, is one
//     element of standard base64 text, the `xs:base64Binary` the ITS-XML
//     schema gives it and the string canonical JSON writes; XML
//     whitespace inside the text is ignored on decode.
//   - The AOM 1.4 `original_language`, a TERMINOLOGY_CODE, is written in
//     the CODE_PHRASE shape the AOM 1.4 ITS-XML schema gives it, with the
//     terminology identifier in a `value` child of `terminology_id`;
//     decode also reads a plain text `terminology_id`.
//   - The AOM 1.4 C_CODED_TEXT `terminology` is written as the
//     `terminology_id` element the AM 1.4 schema type C_CODE_PHRASE
//     declares in its place, the identifier in a `value` child; decode
//     also reads a `terminology` element.
//   - A member typed by a generic parameter that is instantiated with an
//     interface, such as the `data` of ORIGINAL_VERSION<any>, is a
//     polymorphic site and needs `xsi:type`, even when its element is
//     empty. With no element at all, the member stays nil.
//
// # Strict vs relaxed type dispatch
//
// The decoder defaults to strict polymorphism: at any element whose
// declared type is an abstract RM class or interface, the input
// element must carry `xsi:type` or the decode fails with
// [typereg.ErrMissingType] wrapped in [DecodeError].
//
// [Unmarshal] and [NewDecoder] accept [WithRelaxedTypeDispatch], which
// names relaxed dispatch (as in canjson): when the declared abstract
// field has exactly one concrete descendant in the merged BMM, the
// decoder would instantiate that descendant without `xsi:type`. The
// option is accepted but not yet enforced, so dispatch stays strict.
//
// # Unmatched content
//
// Content the decode does not read is unmatched: an element or attribute
// that names no attribute of the RM type its element decodes as; an
// element in a namespace other than the openEHR one ([ErrNamespace]); text
// other than XML whitespace directly inside an element whose content is
// elements, such as an RM object's, a CDATA section included; and the
// element of a `Hash` member or of a BMM `Any` member (see below). The text
// of a primitive element is its value, and whitespace between elements is
// never unmatched, so an indented document decodes as a compact one does.
// Two kinds of attribute are never unmatched: a namespace declaration,
// and an attribute in the XML Schema instance namespace, whether the
// `xsi:type` discriminator, `xsi:schemaLocation`, `xsi:nil` or another.
// A bare `type` attribute, in no namespace, is read as the discriminator
// only where dispatch reads it, at a polymorphic slot; anywhere else it is
// unmatched. An element in no namespace is read as an openEHR element,
// since many producers leave the declaration out. The root element is not
// matched by name, since the value decoded into says what it is, so its
// own namespace is not checked.
//
// The check covers every class the generated decoders read, the AOM 1.4
// classes included.
//
// By default the decoder ignores unmatched content: the value holds the
// rest of the input and none of it. [WithUnmatched] passes each item, in
// document order, to a caller function as its path from the root of the
// value, such as "/composition[1]/content[2]/x-note[1]",
// "/composition[1]/@x-flag" or "/ehr_status[1]/subject[1]/text()", the last
// naming the text of an element, one item however many runs of text it
// holds; the path names the item and carries none of its value. [WithStrict] refuses the decode at the first item with an
// error wrapping [ErrUnknownElement] (and [ErrNamespace] for a foreign
// element). Strict never refuses an absent element: a mandatory one is a
// missing member, which [WithMissing] reports (see below).
//
// An unknown `xsi:type` fails in every mode. The decoder reads the input
// as a stream and checks content as it goes, so strict returns the first
// problem in document order: an unmatched item that comes before an
// unknown `xsi:type` refuses the decode with [ErrUnknownElement], and an
// unknown `xsi:type` that comes first fails as itself. Without strict, the
// same input fails on the unknown `xsi:type`.
//
// The options reach the generated UnmarshalXML methods, which encoding/xml
// calls with nothing but the *xml.Decoder, through the decoder itself: a
// decode given [WithUnmatched], [WithStrict] or [WithMissing] reads its
// input through a token source that tracks the open elements, and registers
// its decoder with its own policy while it runs. Concurrent decodes each
// have their own decoder and so their own policy. The generated methods
// hand the content they do not read to helpers in an internal package of
// the SDK, which look the decoder up and, for an untracked decode, behave
// as encoding/xml does, except for the errors below.
//
// # Missing members
//
// A member is missing when the Reference Model makes the attribute mandatory
// on the class its element decodes as, the class its `xsi:type` names, or
// else the type the RM declares where it sits, as "Unmatched content" above
// reads it, and the element carries no value for it: it has no child element
// of that name, nor one the AM 1.4 schema declares in its place
// (`terminology_id` for a C_CODE_PHRASE's terminology) holding a value, and
// no XML attribute of it where the ITS-XML schema carries the attribute as
// one (`archetype_node_id`, an ARCHETYPE_TERM's `code`); the child it has carries
// `xsi:nil="true"` (or `1`), whatever it holds; or the child is a primitive
// element whose text is empty or XML whitespace only, which carries no
// value, as an empty interval flag element does. The decoder cannot always
// show that in the value it returns: a member the SDK holds by value, a
// number, a boolean, a string or a value struct such as PARTY_SELF or
// ARCHETYPE_ID, decodes to its zero value whether the input carried it or
// not, so a DV_QUANTITY without its `magnitude` reads as one of 0 and an
// EHR_STATUS without its `subject` reads as one with a bare PARTY_SELF.
//
// By default the decoder decodes an input with missing members as it decodes
// one without: no error, each such member at its zero value. A missing
// member is never a decode error in any mode: [WithStrict] covers unmatched
// content only.
//
// [WithMissing] passes each missing member to a function, once, as the
// decode streams: the members of one element as the element closes, in the
// order the RM declares the attributes (an ancestor's before the class's
// own), so the members of the elements inside it come first. The decode
// returns the same value as the default. The member is named by its JSON
// Pointer (RFC 6901) in the canonical-JSON form of the instance, from the
// root of the value, such as
// "/content/0/data/events/0/data/items/0/value/magnitude": the attribute's
// name under the pointer of the element that lacks it, and each child
// element of a multi-valued attribute by its position among its same-named
// siblings, counted from 0. That is not the element path [WithUnmatched]
// reports: a missing member is a member of the instance, not an item of the
// document, so both codecs name it one way and a consumer of either format
// hands the validators one list. The pointer carries no value from the
// input.
//
// A member that carries a value is never missing, whatever the value: an
// empty struct
// element such as `<subject/>` or a bare `<subject xsi:type="PARTY_SELF"/>`,
// `<magnitude>0</magnitude>`, an `archetype_node_id` written as an XML
// attribute, and a `Hash` or `Any` element the codec does not model are
// present, and whether such a value is valid is for the validators to say.
// Canonical XML has no element for an empty list, so a mandatory list the
// input leaves empty, a CLUSTER's `items`, is missing, where canonical
// JSON's `[]` is present. Nothing inside a nilled element is reported: the
// member is reported once, by its parent, as a `null` is in canonical JSON.
// Three kinds of attribute are never missing: one no serialization carries,
// which is every function the model computes, such as EVENT `offset`, and
// RESOURCE_DESCRIPTION `parent_resource`; the four interval boundary flags,
// which take their defaults when absent; and `xsi:type`, which names no
// attribute.
//
// The check runs in the same pass over the input that serves
// [WithUnmatched], through the description of its class each generated
// decoder hands the runtime on entry, so the two options may be combined,
// each with its own grammar, and every class the generated decoders read is
// checked, the AOM 1.4 classes included, where canonical JSON checks the RM
// classes only. A decode that fails makes no promise about which members it
// reported before the failure. One member escapes the check: a primitive
// already held in an interface-typed slot, which encoding/xml reads itself,
// counts as a value even when its element is empty.
//
// The option changes nothing about what the decoder reads, and it is not
// available on REST reads through the transport and client packages, which
// decode with the default.
//
// # Primitive failures
//
// The text of a primitive element that does not decode as its type, such
// as a boolean written `maybe`, a whole number written `2.5`, a number out
// of its type's range, a lossy Real, a `TERM_MAPPING.match` that is not one
// character or a DV_MULTIMEDIA `data` that is not base64, fails the decode
// in every mode with a [DecodeError] whose Inner wraps [ErrInvalidShape]
// and the conversion's own error. Its text names the element and carries
// none of its text. [Unmarshal], and a [Decoder] given [WithUnmatched],
// [WithStrict] or [WithMissing], return it on its own with the element's
// path from the root of the value, such as
// "/ehr_status[1]/is_queryable[1]". Without an
// option, [Unmarshal] decodes the input a second time to find that path,
// once the first decode has failed so; a decode that succeeds never pays
// for it. A [Decoder] given no option cannot read its stream again, so it
// names the element alone, "/is_queryable", inside the envelope of each
// polymorphic slot the element sits in.
//
// # Polymorphic dispatch
//
// [EncodePoly] uses a small reflect.New fallback only to bridge
// value-typed concrete fields at generic instantiation sites (e.g.
// DVInterval[DVQuantity].Lower where Lower is DVQuantity by value).
// This is not type dispatch, which stays registry-driven, and the copy
// is read-only for marshalling.
//
// The codec consults [typereg.Default] for every `xsi:type` lookup,
// reusing the type registry that powers canjson: `xsi:type`
// discriminator strings are the same identifiers as canjson `_type`.
// External consumers must not register types into the default
// registry.
//
// # ITS-XML / XSD pin
//
// Element names and the canonical RM XML shape are pinned to the
// openEHR ITS-XML release shipping the XSD set under the BMM bump
// procedure (see resources/bmm/README.md). The codec does not
// perform XSD validation at the wire layer; OPT-driven validation is
// available under openehr/template/.
//
// # Unsupported: Hash<K,V> elements and Any content
//
// BMM Hash<K,V> (map-typed) RM attributes are not supported by this
// codec, and concrete RM types hit this limit. Resource-metadata types
// carry such fields: ResourceDescription (original_author,
// other_details, details), ResourceDescriptionItem (other_details), and
// TranslationDetails (author, other_details), all map[string]…
// attributes.
//
// Decode does not read these elements: it treats each as one item of
// unmatched content, so it skips it by default, [WithUnmatched] reports it
// and [WithStrict] refuses it, and the map field stays nil. Encode cannot
// emit them at all: the generated codec hands the Go map to
// encoding/xml, which rejects map types, so [Marshal] of any
// AuthoredResource / ResourceDescription fails with
// "xml: unsupported type: map[string]…", even when the maps are nil,
// since original_author is always encoded. Canonical XML for these
// resource-metadata types is therefore unavailable until a Hash codec
// exists.
//
// Nor can the codec decode the content of a BMM `Any` attribute, held as
// a Go empty interface: the AOM 1.4 `assumed_value` and `EXPR_LEAF.item`.
// Decode treats such an element the same way, as one item of unmatched
// content, and leaves the field at its zero value. So it would for a
// member whose Go type has no canonical-XML decoder of its own; every
// member of the RM and AOM 1.4 types has one.
//
// The canjson codec is unaffected (encoding/json handles maps).
//
// # See also
//
//   - [github.com/cadasto/openehr-sdk-go/openehr/rm/typereg]: the
//     `xsi:type` registry shared with canjson.
//   - [github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson]:
//     the JSON codec.
//   - [github.com/cadasto/openehr-sdk-go/openehr/serialize]: codec
//     family overview.
package canxml
