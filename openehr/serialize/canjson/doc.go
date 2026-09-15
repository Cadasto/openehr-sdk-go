// Package canjson implements the openEHR canonical JSON codec for
// the SDK's generated Reference Model types.
//
// The codec is a thin orchestration layer over encoding/json/v2 and
// encoding/json/jsontext: the heavy lifting lives in the per-RM-type
// MarshalJSONTo / UnmarshalJSONFrom streaming pair that the BMM code
// generator emits (ADR 0022). This package only exposes the public
// entry points and a shared error type ([DecodeError]).
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
// The codec implements the profile pinned by REQ-052 (see
// docs/specifications/wire.md). Member order carries no meaning
// (RFC 8259 § 4): the decoder accepts members in any order, `_type`
// included, and the SDK asserts no order for another implementation's
// output. Round-trip fidelity is asserted semantically (typed deep
// equality plus the reference-model validation floor), never by
// comparing encoded bytes (PROBE-030).
//
//   - The encoder emits `_type` first on every concrete RM value. This
//     is a SHOULD, so a consumer decoding a stream can select the
//     concrete type before reading the rest; member order is otherwise
//     unspecified and a consumer MUST NOT rely on it.
//
//   - Member names are matched case-sensitively on this codec path
//     (encoding/json/v2 default; canjson sets no case-insensitive
//     option). A wrongly-cased member, `Magnitude` for `magnitude`, does
//     not populate the field. This is a change from the v1 codec, which
//     matched a struct field case-insensitively. The case-insensitive
//     Extras rule (wire.md § Unknown response keys) binds only the
//     Definition, System and AQL surfaces, which stay on v1 (REQ-052).
//
//   - `<`, `>` and `&` are emitted literally: encoding/json/v2 does not
//     HTML-escape (REQ-052).
//
//   - `Hash` (map[K]V) keys are emitted in lexicographic key order, an
//     encoder-determinism property obtained with json.Deterministic(true)
//     (REQ-052). Two encodes of one value therefore agree on member
//     order; this is a determinism property, not a byte-level promise
//     REQ-052 makes to a consumer.
//
//   - Nil-pointer optional fields are emitted as ABSENT (no key), not
//     as `null`. Both ABSENT and `null` are accepted on decode.
//
//   - Empty containers with BMM cardinality.lower == 0 are emitted as
//     ABSENT (omitempty), not as `[]`.
//
//   - ISO 8601 dates/times/durations are passed through as JSON
//     strings; the codec does not parse them to time.Time (REQ-046).
//
//   - Numeric magnitudes use IEEE 754 double-precision JSON numbers
//     (no silent float32 coercion). REQ-052 requires a typed error on
//     decode "rather than silently rounding" when a wire value exceeds
//     JSON's number precision; the SDK defines that condition as a
//     literal carrying more than 17 significant decimal digits —
//     float64's shortest-round-trip maximum. This is a digit-count
//     budget, not an exactness test, and by design it is wrong in both
//     directions: a literal past 17 digits can still be exactly
//     representable in float64 and is refused anyway (18446744073709551616,
//     2^64, is exact yet carries 20 significant digits), while a shorter
//     literal can be binary-inexact and decode unreported — an integer
//     past 2^53 (e.g. 9007199254740993, 16 digits, decodes to
//     9007199254740992) or any decimal fraction that is not a power of
//     two (0.1 included). A general exactness check would classify both
//     cases correctly but would also reject 0.1, which the plan rejected
//     as over-reporting; >17 is chosen instead as a cheap, deterministic
//     budget that never reports a short clinical literal. Within that
//     definition both arms are met: an out-of-range magnitude (e.g.
//     1e400) fails with a typed error, a *json.SemanticError,
//     reachable with errors.As through the generated decode-method
//     wrapper — and a magnitude past 17 significant decimal digits, such
//     as 0.1234567890123456789, fails decode wrapping [ErrInvalidShape]
//     instead of rounding to the nearest float64; a malformed or
//     out-of-range literal reports its own parse/range error first, so
//     the budget applies only once a literal has parsed. Closed by
//     docs/plans/archive/2026-09-01-rm-canonical-json-fidelity.md.
//
// # Error classification
//
// The package keeps its two sentinels one-directional (REQ-052), so a
// caller can classify a failure with errors.Is alone:
//
//   - [ErrInvalidValue] is encode only. Every [Marshal] / [MarshalIndent]
//     failure wraps it, over the encoder's own error (a
//     *encoding/json/v2.SemanticError, or a *jsontext.SyntacticError for
//     invalid UTF-8 reached on output), which stays reachable through
//     unwrapping.
//   - [ErrInvalidShape] is decode only and never appears on an encode
//     path. A shape error raised inside a generated RM type's decode
//     method wraps it, over the encoding/json/v2 error, which stays
//     reachable through unwrapping; a duplicate member name wraps it too
//     (see below). The other decode outcomes described below do not
//     carry it.
//
// Both are distinct from the transport-level transport.ErrInvalidShape,
// which classifies a response body rather than a codec operation.
// Do not confuse [ErrInvalidShape] with canxml.ErrInvalidShape either:
// same name, same subtree, a different value — and canxml's is raised
// in both directions, where this one is decode-only.
//
// What a decode failure looks like depends on where it happens:
//
//   - Malformed JSON is refused by the tokenizer as the value it
//     malforms is decoded, not through a separate whole-input pass.
//     No sentinel: the codec reports its own syntax or truncated-input
//     error (a *jsontext.SyntacticError), except that [Decoder.Decode]
//     reports an empty stream as io.EOF and a truncated value wraps
//     io.ErrUnexpectedEOF. Invalid UTF-8 and a lone surrogate escape are
//     refused on this same path, before rm.Character sees the bytes, so
//     they too are malformed input carrying no sentinel. rm.Character's
//     own string arm relies on that same tokenizer refusal and no longer
//     inspects the raw literal for a substituted U+FFFD (ruling R15).
//   - A duplicate member name is refused during tokenisation. The entry
//     point classifies that refusal with [ErrInvalidShape], keeping the
//     cause reachable, so errors.Is finds both the sentinel and
//     jsontext.ErrDuplicateName (RFC 8259 § 4; REQ-052).
//   - A polymorphic dispatch failure — a missing, unknown or
//     mismatched `_type` — arrives as [DecodeError] carrying the path,
//     either at a slot or on `/_type` where the whole value's `_type`
//     names a different class than the target. No sentinel: an
//     enclosing type's `canjson: <RM_TYPE>:` funnel does not add one
//     to a [DecodeError] travelling out through it, so a nested
//     dispatch failure never turns into a shape error.
//   - A shape error inside a generated RM type is wrapped by that
//     type's generated decode method with a `canjson: <RM_TYPE>:`
//     prefix, so the encoding/json/v2 error stays reachable with
//     errors.As but is not returned verbatim. This wraps
//     [ErrInvalidShape]; when the failing type is the one selected at a
//     polymorphic slot, the error is ALSO a [DecodeError] naming that
//     slot. A [DecodeError] does not strip a shape classification raised
//     beneath it, so a consumer reads the path off one and the kind off
//     the other.
//   - A hand-written primitive decoded at the top level (rm.Real,
//     rm.Integer or rm.Character handed to [Unmarshal] directly rather
//     than reached through a generated type) carries its own
//     `rm.<Type>:` prefix, not the `canjson: <RM_TYPE>:` funnel. Which
//     refusals carry [ErrInvalidShape] differs by primitive, so it is
//     worth stating per arm. On rm.Character a refusal it raises itself,
//     applying the one-character rule to a value the tokenizer accepted
//     (an empty or multi-character string, or a control character, and the
//     encoding/json/v2 type mismatch of its string arm), carries the
//     sentinel. A lone surrogate or invalid UTF-8 does not reach
//     rm.Character on this path: the tokenizer refuses it first, so it is
//     malformed input carrying no sentinel, the same treatment as
//     malformed JSON above. On rm.Real only the precision refusal carries
//     the sentinel; on rm.Integer none does. An empty input carries no
//     sentinel on any of the three, and neither does a strconv or
//     encoding/json/v2 parse or range failure beneath rm.Real or rm.Integer
//     (the precedence rule wire.md § REQ-052 states), and those causes
//     stay reachable with errors.AsType. A nil receiver on any of them, as
//     on every generated type, is a typereg.ErrNilReceiver error (REQ-025).
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
