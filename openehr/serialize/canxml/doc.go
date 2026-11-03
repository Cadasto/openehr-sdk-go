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
//     (unprefixed openEHR class name). The encoder emits it as the
//     first attribute on every concrete element at a polymorphic
//     site. The decoder requires it at polymorphic sites;
//     [WithRelaxedTypeDispatch], which would relax that, is accepted but
//     not yet enforced. Discriminator values are
//     matched unprefixed, with one exception: a value whose prefix is
//     bound, where the attribute stands, to the XML Schema namespace
//     `http://www.w3.org/2001/XMLSchema` names an XML Schema built-in
//     datatype and reads as its BMM primitive, whatever the prefix, so
//     `xs:string` and `xsd:string` both read as `String`, and so does a
//     literal `xsd:` that no declaration in scope binds. Other
//     namespace-prefixed values (the Better/Marand
//     `xsi:type="ns2:DV_QUANTITY"` dialect) are not supported and fail
//     closed with [typereg.ErrUnknownType]. A type alias a generated
//     package registers, the AOM 1.4 `T_ARCHETYPE_ROOT` that some tools
//     write for `C_ARCHETYPE_ROOT`, reads as the class it names, which
//     the encoder writes by its own name.
//   - Element local names are snake_case BMM property/class names
//     (identical to canjson JSON keys; e.g. `dv_quantity`,
//     `magnitude_status`). The root element takes the class name, except
//     where the class's ITS-XML schema declares a global element of
//     another name for its type, which its generated package registers:
//     an AOM 1.4 OPERATIONAL_TEMPLATE is written as `template`. The decoder does not
//     match the root by name.
//   - Child elements follow the `xs:sequence` of the class's type in
//     the openEHR ITS-XML schemas (RM Release-1.1.0, BASE Release-1.2.0,
//     AM Release-1.4), or of its nearest ancestor's type when the schemas
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
//   - A TERMINOLOGY_CODE, such as the `concept` of a TERMINOLOGY_TERM, is
//     written in the CODE_PHRASE shape, with the terminology identifier in a
//     `value` child of `terminology_id`; decode also reads a plain text
//     `terminology_id`.
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
// element of a `Hash` of `Hash`es (see below). The
// key attribute of a `Hash` entry is read, not unmatched. The text of a
// primitive element is its value, and whitespace between elements is never
// unmatched, so an indented document decodes as a compact one does.
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
// of that name, and no XML attribute of it where the ITS-XML schema carries
// the attribute as one (`archetype_node_id`, an ARCHETYPE_TERM's `code`);
// the child it has carries `xsi:nil="true"` (or `1`), whatever it holds; or
// the child is a primitive element whose text is empty or XML whitespace
// only, which carries no value, as an empty interval flag element does. The
// decoder cannot always show that in the value it returns: a member the SDK
// holds by value, a number, a boolean, a string or a value struct such as
// PARTY_SELF or ARCHETYPE_ID, decodes to its zero value whether the input
// carried it or not, so a DV_QUANTITY without its `magnitude` reads as one
// of 0 and an EHR_STATUS without its `subject` reads as one with a bare
// PARTY_SELF.
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
// name under the pointer of the element that lacks it, each child element
// of a multi-valued attribute by its position among its same-named
// siblings, counted from 0, and each entry of a `Hash` by its key, such as
// "/description/details/en/purpose". That is not the element path
// [WithUnmatched] reports: a missing member is a member of the instance, not
// an item of the document, so both codecs name it one way and a consumer of
// either format hands the validators one list. The pointer carries no value
// from the input.
//
// A member that carries a value is never missing, whatever the value: an
// empty struct
// element such as `<subject/>` or a bare `<subject xsi:type="PARTY_SELF"/>`,
// `<magnitude>0</magnitude>`, an `archetype_node_id` written as an XML
// attribute, a `Hash` with an entry, even one whose value is empty, and an
// element the codec does not model, a `Hash` of `Hash`es, are present, and
// whether such a value is valid is for the validators to say.
// Canonical XML has no element for an empty list, so a mandatory list the
// input leaves empty, a CLUSTER's `items`, is missing, where canonical
// JSON's `[]` is present. Nothing inside a nilled element is reported: the
// member is reported once, by its parent, as a `null` is in canonical JSON.
// Three kinds of attribute are never missing: one no serialization carries,
// which is every function the model computes, such as EVENT `offset`, and
// RESOURCE_DESCRIPTION `parent_resource`; the four interval boundary flags,
// which take their defaults when absent; and `xsi:type`, which names no
// attribute. An element that takes one of several shapes, the AOM 1.4
// ORDINAL and SCALE `symbol`, is held to the members its shape's schema type
// requires, not to those of the class that reads it: the `value` of its
// DV_CODED_TEXT shape is optional there, so a symbol without one reports
// nothing.
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
// for it. A [Decoder] given no option does not decode a value again to find
// that path, so it names the element alone, "/is_queryable", inside the
// envelope of each polymorphic slot the element sits in.
//
// # Nesting depth
//
// A decode has no depth limit of its own. [WithMaxDepth] sets one, for
// input from outside the caller's control; given more than once, the
// smallest positive limit wins. The decode refuses an element nested deeper
// than the limit, the root of the value at depth 1, as it reads the
// element's start tag, before anything decodes it, in every mode. The
// refusal is a [DecodeError] on its own that wraps [ErrLimitExceeded]; it
// names the element's depth, the limit and the element's path, cut short in
// the middle when it is long, and none of the element's content. A [Decoder]
// that refuses an element returns the same refusal from every later Decode.
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
// # Hash members
//
// A BMM `Hash<K,V>` member, held as a Go map, is written one element per
// entry, named after the member, in ascending order of the keys, so a value
// encodes to one document whatever the order of its map, and read back one
// entry per element, in the shapes the AM Release-1.4 schemas give them:
//
//   - A `Hash<String,String>`, such as RESOURCE_DESCRIPTION's
//     `original_author` and `other_details` or ARCHETYPE_TERM's `items`, as
//     one StringDictionaryItem per entry: `<items id="text">Blood
//     pressure</items>`, the key in its `id` attribute and the value as its
//     text.
//   - A `Hash` of class values, RESOURCE_DESCRIPTION's `details` and
//     ARCHETYPE's `translations`, as one element per value, through the
//     value's own codec. RESOURCE_DESCRIPTION_ITEM and TRANSLATION_DETAILS
//     carry no `id` attribute, as AM Release-1.4 writes them, in an RM
//     document too: the entries are keyed by the value's `language` code, and
//     an `id` attribute on them is unmatched content.
//
// Decode takes an entry's key attribute off before it reads the value, so the
// key is never unmatched content; any other attribute of the entry, and a
// child element inside a String value, is. An empty or nil `Hash` writes no
// element, as an empty list does, and a mandatory one with no element is
// missing ([WithMissing]).
//
// A member's map starts empty at the element's first entry of it, in every
// mode, so a key repeats only within the element, and a value decoded into
// twice holds the second document's entries. Decode refuses an entry whose
// key an earlier entry of the member holds, and an entry with no key, no key
// attribute or no language code, with [ErrInvalidShape]: the error names the
// entry's path and not its key. The encoder refuses, with [ErrInvalidShape],
// an entry keyed by language held under a key other than its value's
// language code, the empty key included: decode would not read it back under
// that key.
//
// # Any members
//
// A BMM `Any` member, held as a Go empty interface, the AOM 1.4
// `EXPR_LEAF.item`, decodes as the type its `xsi:type` names, read as a
// discriminator is read (see "Wire profile" above): a class such as
// `C_STRING`, as a pointer to a value of the class; an XML Schema built-in
// datatype, such as `xs:string` under a prefix bound to the XML Schema
// namespace where the attribute stands, as an aom14.TypedString that holds
// the text and keeps the `xsi:type` as written; and, with no `xsi:type`, as
// its text, a Go string. The encoder writes each in the form it was read in: a
// class value with its `xsi:type`, a typed string with its own and a
// declaration of the prefix it uses, and a Go string with none. The item is
// read, not unmatched content; what its own type does not read is, as in any
// class or primitive element.
//
// # Unsupported: a Hash of Hashes
//
// The codec does not model a `Hash` whose values are `Hash`es,
// RESOURCE_ANNOTATIONS' `documentation`. Decode treats such an element as one
// item of unmatched content, so it skips it by default, [WithUnmatched]
// reports it and [WithStrict] refuses it, and the field stays at its zero
// value; [Marshal] of a RESOURCE_ANNOTATIONS fails, since encoding/xml rejects
// the map. So the decode would treat a member whose Go type has no
// canonical-XML decoder of its own; every member of the RM and AOM 1.4 types
// has one.
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
