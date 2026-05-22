// Package canxml implements the openEHR canonical XML codec for the
// SDK's generated Reference Model types — the symmetric sibling of
// [github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson].
//
// The codec is a thin orchestration layer over stdlib encoding/xml:
// the heavy lifting lives in the per-RM-type [MarshalXML] and
// [UnmarshalXML] methods that the BMM code generator emits, plus the
// [BMMName] discriminator method shared with the type registry. This
// package only exposes the public entry points, the [BMMNamer]
// interface used at polymorphic boundaries, and a shared error type
// ([DecodeError]).
//
// # Building-block independence
//
// Consumers import this package directly, e.g.
//
//	import "github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
//
// Implements REQ-056 per docs/specifications/wire.md and REQ-040 per
// docs/specifications/rm-modeling.md. Per REQ-013 the codec MUST be usable without
// the HTTP client, auth, transport, or any other SDK subsystem. The
// only dependencies are the generated RM types and the type registry
// under [github.com/cadasto/openehr-sdk-go/openehr/rm/typereg].
//
// # Wire profile
//
// The codec implements the deterministic profile pinned by REQ-056
// (see docs/specifications/wire.md § Canonical XML):
//
//   - Default namespace: `http://schemas.openehr.org/v1`.
//   - `xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"` is
//     declared on the document root whenever any descendant carries
//     `xsi:type`.
//   - Polymorphic discrimination: `xsi:type="<BMM_CLASS_NAME>"`
//     (unprefixed openEHR class name). The encoder emits it as the
//     FIRST attribute on every concrete element at a polymorphic
//     site. The decoder requires it at polymorphic sites unless
//     [WithRelaxedTypeDispatch] is set.
//   - Element local names are **snake_case** BMM property/class names
//     (identical to canjson JSON keys; e.g. `dv_quantity`,
//     `magnitude_status`).
//   - Child elements follow **BMM property declaration order** — the
//     same order the generator emits struct fields.
//   - Nil-pointer optional fields and empty containers with
//     `cardinality.lower == 0` are emitted as ABSENT (no element).
//     Both ABSENT and an empty self-closing element are accepted on
//     decode.
//   - ISO 8601 dates/times/durations are passed through as element
//     text content; the codec does not parse them at codec layer
//     (REQ-046).
//   - Numeric magnitudes use IEEE 754 double-precision (same posture
//     as canonical JSON); decode also accepts quoted decimal strings
//     per docs/adr/0004-numeric-wire-tolerance.md.
//   - Compact XML (no insignificant inter-element whitespace) is the
//     byte-equality target for round-trip tests. The encoder always
//     emits compact form; [MarshalIndent] is for human inspection
//     only.
//   - `xmi:type` is REJECTED on decode with [ErrInvalidShape] — only
//     `xsi:type` is recognised (the openEHR ITS-XML pin uses XMI in
//     UML diagrams, not on the wire).
//
// # Strict vs relaxed decode
//
// The decoder defaults to STRICT polymorphism: at any element whose
// declared type is an abstract RM class or interface, the input
// element MUST carry `xsi:type` or the decode fails with
// [typereg.ErrMissingType] wrapped in [DecodeError].
//
// [NewDecoder] accepts [WithRelaxedTypeDispatch] to opt into relaxed
// dispatch (parallel to canjson): when the declared abstract field
// has exactly one concrete descendant in the merged BMM, the decoder
// instantiates that descendant without `xsi:type`. Default is OFF.
//
// # Polymorphic dispatch
//
// The codec consults [typereg.Default] for every `xsi:type` lookup,
// reusing the type registry that powers canjson — `xsi:type`
// discriminator strings are the same identifiers as canjson `_type`.
// External consumers MUST NOT register types into the default
// registry (REQ-040).
//
// # ITS-XML / XSD pin
//
// Element names and the canonical RM XML shape are pinned to the
// openEHR ITS-XML release shipping the XSD set under the BMM bump
// procedure (see resources/bmm/README.md). The codec does NOT
// perform XSD validation at the wire layer; OPT-driven validation is
// available under openehr/template/.
//
// # See also
//
//   - [github.com/cadasto/openehr-sdk-go/openehr/rm/typereg] — the
//     `xsi:type` registry shared with canjson.
//   - [github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson] —
//     symmetric JSON codec.
//   - [github.com/cadasto/openehr-sdk-go/openehr/serialize] — codec
//     family overview.
package canxml
