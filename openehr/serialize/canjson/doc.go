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
//     docs/plans/archive/2026-08-30-read-path-decode-taxonomy.md.
//
// # Error classification
//
// The package keeps its two sentinels one-directional (REQ-052), so a
// caller can classify a failure with errors.Is alone:
//
//   - [ErrInvalidValue] — encode only. Every [Marshal] / [MarshalIndent]
//     failure wraps it, over the encoder's own error, which stays
//     reachable through unwrapping.
//   - [ErrInvalidShape] — decode only. Never appears on an encode
//     path. Every shape error raised inside a generated RM type's
//     [UnmarshalJSON] wraps it, over the encoding/json error, which
//     stays reachable through unwrapping. The other two decode
//     failures described below do not wrap it.
//
// Both are distinct from the transport-level transport.ErrInvalidShape,
// which classifies a response body rather than a codec operation.
// Do not confuse [ErrInvalidShape] with canxml.ErrInvalidShape either:
// same name, same subtree, a different value — and canxml's is raised
// in both directions, where this one is decode-only.
//
// What a decode failure looks like depends on where it happens:
//
//   - Malformed JSON reaches the caller unchanged from encoding/json,
//     because encoding/json validates the whole input before it
//     dispatches to any UnmarshalJSON method. [Unmarshal] returns
//     *json.SyntaxError for it; [Decoder.Decode] classifies a
//     truncated stream differently, as io.ErrUnexpectedEOF, and an
//     empty one as io.EOF.
//   - A shape error inside a generated RM type is wrapped by that
//     type's generated UnmarshalJSON with a `canjson: <RM_TYPE>:`
//     prefix, so the encoding/json error stays reachable with
//     errors.As but is not returned verbatim. This is the one that
//     wraps [ErrInvalidShape].
//   - A failure at a polymorphic slot arrives as [DecodeError]
//     carrying the path — whether its cause is a typereg sentinel
//     (missing, unknown or mismatched `_type`) or a plain
//     encoding/json error at that slot. It keeps that classification
//     even when it travels out through an enclosing type's
//     `canjson: <RM_TYPE>:` prefix, so a nested polymorphic failure
//     never turns into a shape error.
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
// External consumers do not register types into the default registry:
// it is populated once by the rm package's init() and stays
// append-only. The rule is normative in
// docs/specifications/rm-modeling.md § Type registry (REQ-040).
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
