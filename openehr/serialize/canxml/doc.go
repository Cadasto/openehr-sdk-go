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
//   - Default namespace: `http://schemas.openehr.org/v1`.
//   - `xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"` is
//     declared on the document root whenever any descendant carries
//     `xsi:type`.
//   - Polymorphic discrimination: `xsi:type="<BMM_CLASS_NAME>"`
//     (unprefixed openEHR class name). The encoder emits it as the
//     first attribute on every concrete element at a polymorphic
//     site. The decoder requires it at polymorphic sites unless
//     [WithRelaxedTypeDispatch] is set. Discriminator values are
//     matched unprefixed; namespace-prefixed values (the Better/Marand
//     `xsi:type="ns2:DV_QUANTITY"` dialect) are not supported and fail
//     closed with [typereg.ErrUnknownType].
//   - Element local names are snake_case BMM property/class names
//     (identical to canjson JSON keys; e.g. `dv_quantity`,
//     `magnitude_status`).
//   - Child elements follow BMM property declaration order, the
//     same order the generator emits struct fields.
//   - Nil-pointer optional fields and empty containers with
//     `cardinality.lower == 0` are emitted as absent (no element).
//     Both absent and an empty self-closing element are accepted on
//     decode.
//   - ISO 8601 dates/times/durations are passed through as element
//     text content; the codec does not parse them.
//   - Numeric magnitudes use IEEE 754 double-precision (same as
//     canonical JSON); decode also accepts quoted decimal strings.
//   - Compact XML (no insignificant inter-element whitespace) is the
//     byte-equality target for round-trip tests. The encoder always
//     emits compact form; [MarshalIndent] is for human inspection
//     only.
//   - `xmi:type` is rejected on decode with [ErrInvalidShape]; only
//     `xsi:type` is recognised (openEHR ITS-XML uses XMI in UML
//     diagrams, not on the wire).
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
// element in a namespace other than the openEHR one ([ErrNamespace]); and
// the element of a `Hash` member or of a BMM `Any` member (see below).
// Two kinds of attribute are never unmatched: a namespace declaration,
// and an attribute in the XML Schema instance namespace, whether the
// `xsi:type` discriminator, `xsi:schemaLocation`, `xsi:nil` or another.
// An element in no namespace is read as an openEHR element, since many
// producers leave the declaration out.
//
// The check covers every class the generated decoders read, the AOM 1.4
// classes included.
//
// By default the decoder ignores unmatched content: the value holds the
// rest of the input and none of it. [WithUnmatched] passes each item, in
// document order, to a caller function as its path from the root of the
// value, such as "/composition[1]/content[2]/x-note[1]" or
// "/composition[1]/@x-flag"; the path names the item and carries none of
// its value. [WithStrict] refuses the decode at the first item with an
// error wrapping [ErrUnknownElement] (and [ErrNamespace] for a foreign
// element). Strict never refuses an absent element.
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
// decode given either option reads its input through a token source that
// tracks the open elements, and registers its decoder with its own policy
// while it runs. Concurrent decodes each have their own decoder and so
// their own policy. The generated methods hand the content they do not
// read to [SkipUnmatched], [CheckUnmatchedAttrs] and [DecodeElement], which
// look the decoder up and, for an untracked decode, behave as encoding/xml
// does.
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
// content, and leaves the field at its zero value.
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
