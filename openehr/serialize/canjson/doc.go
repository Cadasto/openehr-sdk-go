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
// Per REQ-013 the codec MUST be usable without the HTTP client, auth,
// transport, or any other SDK subsystem. The only dependencies are
// the generated RM types and the type registry under
// [github.com/cadasto/openehr-sdk-go/openehr/rm/typereg].
//
// # Wire profile
//
// The codec implements the deterministic profile pinned by REQ-052
// (see specs/wire.md):
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
//     (no silent float32 coercion). Overflow on decode is reported as
//     [ErrInvalidShape] rather than silently rounded.
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
