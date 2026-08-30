// Package canjson implements the openEHR canonical JSON codec for
// the SDK's generated Reference Model types.
//
// The codec is a thin orchestration layer over stdlib encoding/json:
// the heavy lifting lives in the per-RM-type [MarshalJSON] and
// [UnmarshalJSON] methods that the BMM code generator emits. This
// package only exposes the public entry points and a shared error
// type ([DecodeError]).
//
// # Building-block independence
//
// Consumers import this package directly, e.g.
//
//	import "github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
//
// Implements REQ-052 and REQ-040 per docs/specifications/wire.md and docs/specifications/rm-modeling.md.
// Per REQ-013 the codec MUST be usable without the HTTP client, auth,
// transport, or any other SDK subsystem. The only dependencies are
// the generated RM types and the type registry under
// [github.com/cadasto/openehr-sdk-go/openehr/rm/typereg].
//
// # Wire profile
//
// The codec implements the deterministic profile pinned by REQ-052
// (see docs/specifications/wire.md):
//
//   - `_type` is the first JSON object key on every encoded concrete
//     RM value.
//   - Remaining keys follow BMM property declaration order (= the
//     order the generator emits struct fields).
//   - `Hash` (map[K]V) keys are emitted in lexicographic key order
//     (stdlib behaviour), independent of struct field order.
//   - Nil-pointer optional fields are emitted as ABSENT (no key), not
//     as `null`. Both ABSENT and `null` are accepted on decode.
//   - Empty containers with BMM cardinality.lower == 0 are emitted as
//     ABSENT (omitempty), not as `[]`.
//   - ISO 8601 dates/times/durations are passed through as JSON
//     strings; the codec does not parse them to time.Time (REQ-046).
//   - Numeric magnitudes use IEEE 754 double-precision JSON numbers
//     (no silent float32 coercion). REQ-052 requires a typed error on
//     decode "rather than silently rounding" when a wire value exceeds
//     JSON's number precision. Half of that is met: an out-of-range
//     magnitude (e.g. 1e400) already fails with a typed error — a
//     *json.UnmarshalTypeError, reachable with errors.As through the
//     generated UnmarshalJSON wrapper. The open half is mantissa
//     precision loss, which is still silent: a magnitude of
//     0.1234567890123456789 decodes to 0.12345678901234568 with a nil
//     error. Closing it is tracked by
//     docs/plans/2026-08-30-read-path-decode-taxonomy.md.
//
// # Error classification
//
// [ErrInvalidShape] is decode-only and is reserved for JSON-level
// shape errors (malformed JSON, type mismatch on a non-polymorphic
// field, numeric overflow). No decode path returns it today, so an
// errors.Is against it never matches. Wrapping decode failures with
// the sentinel is a spec-first REQ-052 follow-up, tracked by
// docs/plans/2026-08-30-read-path-decode-taxonomy.md.
//
// What a decode failure does look like depends on where it happens:
//
//   - Malformed JSON reaches the caller unchanged from encoding/json,
//     because encoding/json validates the whole input before it
//     dispatches to any UnmarshalJSON method. [Unmarshal] returns
//     *json.SyntaxError for it; [Decoder.Decode] classifies a
//     truncated stream differently, as io.ErrUnexpectedEOF.
//   - A shape error inside a generated RM type is wrapped by that
//     type's generated UnmarshalJSON with a `canjson: <RM_TYPE>:`
//     prefix, so the encoding/json error stays reachable with
//     errors.As but is not returned verbatim.
//   - A failure at a polymorphic slot arrives as [DecodeError]
//     carrying the path — whether its cause is a typereg sentinel
//     (missing, unknown or mismatched `_type`) or a plain
//     encoding/json error at that slot.
//
// None of the three wraps [ErrInvalidShape].
//
// Do not confuse this sentinel with canxml.ErrInvalidShape: same
// name, same subtree, a different value — and unlike this one it does
// have producers, for `xmi:type` discriminator failures. The
// transport-level transport.ErrInvalidShape is a third distinct
// value; it is named here in plain text rather than as a doc link
// because canjson must not depend on transport (REQ-013).
//
// # Strict vs relaxed decode
//
// The decoder defaults to STRICT polymorphism: at any field whose
// declared type is an abstract RM class or interface, the input
// object MUST carry `_type` or the decode fails with
// [typereg.ErrMissingType] wrapped in [DecodeError].
//
// [NewDecoder] accepts [WithRelaxedTypeDispatch] to opt into relaxed
// dispatch: when the declared abstract field has exactly one concrete
// descendant in the merged BMM, the decoder instantiates that
// descendant without `_type`. This is a documented escape hatch for
// legacy producers; default is OFF.
//
// # Polymorphic dispatch
//
// The codec consults [typereg.Default] for every `_type` lookup.
// External consumers MUST NOT register types into the default
// registry — it is populated once by the rm package's init() and
// expected to stay append-only (REQ-040).
//
// Abstract generic RM classes whose concrete descendants must
// dispatch on the wire (e.g. EVENT → POINT_EVENT / INTERVAL_EVENT)
// are promoted to Go interfaces via the generator's
// `codecPolymorphicAbstractGenericNames` whitelist — see
// docs/adr/0003-rm-event-polymorphism.md.
//
// # See also
//
//   - [github.com/cadasto/openehr-sdk-go/openehr/rm/typereg] —
//     `_type` registry primitive shared with canxml.
//   - [github.com/cadasto/openehr-sdk-go/openehr/serialize] — codec
//     family overview.
package canjson
