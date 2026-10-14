// Package canjson implements the openEHR canonical JSON codec for
// the SDK's generated Reference Model types.
//
// The codec is a thin orchestration layer over encoding/json/v2 and
// encoding/json/jsontext: the per-RM-type MarshalJSONTo /
// UnmarshalJSONFrom streaming methods that the BMM code generator emits
// do the work. This package only exposes the public entry points and a
// shared error type ([DecodeError]).
//
// # Dependencies
//
// Consumers import this package directly, e.g.
//
//	import "github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
//
// The codec is usable without the HTTP client, auth, transport, or any
// other SDK subsystem. The only dependencies are the generated RM types,
// the type registry under
// [github.com/cadasto/openehr-sdk-go/openehr/rm/typereg], and the RM's
// attribute lists in [github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo],
// which the options for unmatched members read.
//
// # Wire profile
//
// Member order carries no meaning (RFC 8259 § 4): the decoder accepts
// members in any order, `_type` included, and the SDK asserts no order
// for another implementation's output. Round-trip fidelity is checked
// semantically (typed deep equality plus the reference-model validation
// floor), never by comparing encoded bytes.
//
//   - The encoder emits `_type` first on every concrete RM value, so a
//     consumer decoding a stream can select the concrete type before
//     reading the rest. Member order is otherwise unspecified and a
//     consumer must not rely on it.
//
//   - Member names are matched case-sensitively on this codec path
//     (encoding/json/v2 default; canjson sets no case-insensitive
//     option). A wrongly-cased member, `Magnitude` for `magnitude`, does
//     not populate the field. The client surfaces that still decode with
//     encoding/json v1 (Definition, System and AQL) behave differently and
//     none of it applies here: they match a documented field name
//     case-insensitively, while their Extras check compares names exactly,
//     so a differently cased key can fill the field and still appear in
//     Extras.
//
//   - A caller driving the generated methods through bare v1
//     encoding/json inherits v1's decode options, and they govern the
//     whole decode, polymorphic slots included (the slot's `_type` is
//     peeked under the caller's options too). The caller therefore
//     inherits v1's leniencies: a duplicate member name is accepted (the
//     last one wins), member names match case-insensitively (`_TYPE`
//     dispatches like `_type`), invalid UTF-8 is accepted (replaced with
//     U+FFFD), and malformed bytes anywhere in the input are reported
//     ahead of a shape failure. These leniencies are v1's, not SDK
//     promises. The canonical-JSON guarantees bind the canjson entry
//     points and bare encoding/json/v2 with its default options; a v2
//     caller's own options (json.MatchCaseInsensitiveNames,
//     jsontext.AllowDuplicateNames, …) govern the whole decode the same
//     way, slots included.
//
//   - `<`, `>` and `&` are emitted literally: encoding/json/v2 does not
//     HTML-escape.
//
//   - `Hash` (map[K]V) keys are emitted in lexicographic key order, an
//     encoder-determinism property obtained with json.Deterministic(true).
//     Two encodes of one value therefore agree on member order; this is
//     a determinism property, not a byte-level promise to a consumer.
//
//   - A member whose name is neither `_type` nor an attribute of the RM
//     type its object decodes as is unmatched, and the decoder ignores it
//     by default: the decoded value holds the rest of the input and none
//     of the unmatched members, so a read, change and write drops a
//     producer's extension members. See "Unmatched members" below.
//
//   - Nil-pointer optional fields are emitted as absent (no key), not
//     as `null`. Both absent and `null` are accepted on decode.
//
//   - Empty containers with BMM cardinality.lower == 0 are emitted as
//     absent (omitempty), not as `[]`.
//
//   - A mandatory member that holds no value, a nil list, map, pointer or
//     interface, or a polymorphic slot holding a typed-nil pointer, is
//     emitted as absent, not as `null`; a present empty mandatory list or
//     map is written as `[]` or `{}`.
//
//   - ISO 8601 dates/times/durations are passed through as JSON
//     strings; the codec does not parse them to time.Time.
//
//   - Numeric magnitudes use IEEE 754 double-precision JSON numbers
//     (no silent float32 coercion). A wire value that exceeds JSON's
//     number precision fails decode with a typed error instead of being
//     silently rounded. The SDK defines that condition as a literal
//     carrying more than 17 significant decimal digits, float64's
//     shortest-round-trip maximum. This is a digit-count budget, not an
//     exactness test, and it is wrong in both directions: a literal past
//     17 digits can still be exactly representable in float64 and is
//     refused anyway (18446744073709551616, 2^64, is exact yet carries
//     20 significant digits), while a shorter literal can be
//     binary-inexact and decode unreported: an integer past 2^53 (e.g.
//     9007199254740993, 16 digits, decodes to 9007199254740992) or any
//     decimal fraction that is not a power of two (0.1 included). A
//     general exactness check would classify both cases correctly but
//     would also reject 0.1, which would over-report; the 17-digit
//     budget is cheap, deterministic, and never reports a short clinical
//     literal. In practice an out-of-range magnitude (e.g. 1e400) fails
//     with a typed error, a *json.SemanticError, reachable with
//     errors.As through the generated decode-method wrapper, and a
//     magnitude past 17 significant decimal digits, such as
//     0.1234567890123456789, fails decode wrapping [ErrInvalidShape]
//     instead of rounding to the nearest float64. A malformed or
//     out-of-range literal reports its own parse/range error first, so
//     the budget applies only once a literal has parsed.
//
// # Error classification
//
// The package keeps its three sentinels apart, so a caller can classify a
// failure with errors.Is alone, and errors.Is against one never matches an
// error raised for another:
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
//   - [ErrUnknownMember] is decode only, and only a decode given
//     [WithStrict] raises it, for the first unmatched member. The error
//     names the member's JSON Pointer and nothing else from the input.
//
// They are distinct from the transport-level transport.ErrInvalidShape,
// which classifies a response body rather than a codec operation.
// Do not confuse [ErrInvalidShape] with canxml.ErrInvalidShape either:
// they share a name and a subtree but are different values, and canxml's
// is raised in both directions, where this one is decode-only.
//
// What a decode failure looks like depends on where it happens:
//
//   - Malformed JSON is refused by the tokenizer as the value it
//     malforms is decoded, except where a value is buffered first: a
//     polymorphic slot is read whole before it is decoded, and so is each
//     value a [Decoder] given [WithUnmatched] or [WithStrict] reads, so
//     malformed bytes anywhere inside such a value are reported ahead of a
//     shape failure inside it.
//     No sentinel: the codec reports its own syntax or truncated-input
//     error (a *jsontext.SyntacticError), except that [Decoder.Decode]
//     reports an empty stream as io.EOF and a truncated value wraps
//     io.ErrUnexpectedEOF. Invalid UTF-8 and a lone surrogate escape are
//     refused on this same path, before rm.Character sees the bytes, so
//     they too are malformed input carrying no sentinel. rm.Character's
//     own string arm relies on that same tokenizer refusal and does not
//     inspect the raw literal for a substituted U+FFFD.
//   - A duplicate member name is refused during tokenisation. The entry
//     point classifies that refusal with [ErrInvalidShape], keeping the
//     cause reachable, so errors.Is finds both the sentinel and
//     jsontext.ErrDuplicateName (RFC 8259 § 4).
//   - A polymorphic dispatch failure (a missing, unknown or
//     mismatched `_type`) arrives as [DecodeError] carrying the path,
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
//     polymorphic slot, the error is also a [DecodeError] naming that
//     slot. A [DecodeError] does not strip a shape classification raised
//     beneath it, so a consumer reads the path off one and the kind off
//     the other.
//   - A hand-written primitive decoded at the top level (rm.Real,
//     rm.Integer or rm.Character handed to [Unmarshal] directly rather
//     than reached through a generated type) carries its own
//     `rm.<Type>:` prefix, not the `canjson: <RM_TYPE>:` funnel. Which
//     refusals carry [ErrInvalidShape] differs by primitive. On
//     rm.Character a refusal it raises itself, applying the
//     one-character rule to a value the tokenizer accepted (an empty or
//     multi-character string, or a control character, and the
//     encoding/json/v2 type mismatch of its string arm), carries the
//     sentinel. A lone surrogate or invalid UTF-8 does not reach
//     rm.Character on this path: the tokenizer refuses it first, so it is
//     malformed input carrying no sentinel, the same treatment as
//     malformed JSON above. On rm.Real only the precision refusal carries
//     the sentinel; on rm.Integer none does. An empty input carries no
//     sentinel on any of the three, and neither does a strconv or
//     encoding/json/v2 parse or range failure beneath rm.Real or
//     rm.Integer; those causes stay reachable with errors.AsType. A nil
//     receiver on any of them, as on every generated type, is a
//     typereg.ErrNilReceiver error.
//
// # Unmatched members
//
// A member is unmatched when its name is neither `_type` nor an attribute
// of the RM type its object decodes as. An object decodes as the class its
// `_type` names, or else as the type the RM declares where it sits: the
// declared type of the attribute that holds it, the class an interval slot
// binds its bounds to, the class of a `Hash` attribute's values, and at the
// root the RM type of the decode target, or of the elements of a slice,
// array or map target, with the class a generic target binds its type
// parameter to. That holds at any depth: inside a polymorphic slot, an
// interval and its bounds, and a `Hash` of RM values too. A member the
// decoder reads but refuses is not unmatched: an unknown `_type` is a
// dispatch failure in every mode, and a member of the wrong shape is a
// shape failure.
//
// [Unmarshal] and [NewDecoder] take two options for unmatched members:
//
//   - [WithUnmatched] passes each one to a function, in the order it
//     appears in the input, as a JSON Pointer (RFC 6901) from the root of
//     the value, such as "/content/0/x-note". The decode returns the same
//     value as the default.
//   - [WithStrict] refuses the decode at the first one, with an error that
//     wraps [ErrUnknownMember] and names its pointer. An absent member is
//     never refused.
//
// The options check the members once the decode has succeeded, in a second
// pass over the same input that reads the RM's own attribute lists. So a
// decode error, such as an unknown `_type`, is returned as itself in every
// mode, wherever it sits in the input, and a decode that fails reports no
// member.
//
// The walk starts from the decode target's own RM shape: an RM class or a
// polymorphic RM interface, or a slice, array or map of one, a generic class
// taking its type parameter from the target's type argument. A target of any
// other shape, such as a pointer to any, to a map[string]any or to a type of
// the caller's own, is not walked, so RM values held inside a type of the
// caller's own are not checked. The pass does not look inside a value it
// cannot read as an RM class either: a value of a class outside the RM, such
// as an AOM 1.4 archetype, decodes as by default and none of its members is
// checked, the RM values it holds included; nor does the pass check a generic
// payload the decoder holds as plain JSON (the data of an
// rm.OriginalVersion[any]), a `Hash` of strings, or an object whose type
// neither its `_type` nor its position names.
//
// Neither option changes what the decoder reads, and neither is available
// on REST reads through the transport and client packages, which decode
// with the default.
//
// # Strict vs relaxed type dispatch
//
// The decoder defaults to strict polymorphism: at any field whose
// declared type is an abstract RM class or interface, the input
// object must carry `_type` or the decode fails with
// [typereg.ErrMissingType] wrapped in [DecodeError].
//
// [NewDecoder] accepts [WithRelaxedTypeDispatch]. Relaxed dispatch would,
// when the declared abstract field has exactly one concrete descendant in
// the merged BMM, instantiate that descendant without `_type`. The option
// is reserved: the current generated decoders implement strict dispatch
// only, so setting it is a no-op for built-in RM types today. It stays on
// the API surface so a caller does not break when relaxed dispatch is
// implemented; the default is off.
//
// # Polymorphic dispatch
//
// The codec consults [typereg.Default] for every `_type` lookup.
// External consumers do not register types into the default registry:
// it is populated once by the rm package's init() and stays
// append-only.
//
// Abstract generic RM classes whose concrete descendants must
// dispatch on the wire (e.g. EVENT → POINT_EVENT / INTERVAL_EVENT)
// are generated as Go interfaces.
//
// # See also
//
//   - [github.com/cadasto/openehr-sdk-go/openehr/rm/typereg]: the
//     `_type` registry primitive shared with canxml.
//   - [github.com/cadasto/openehr-sdk-go/openehr/serialize]: codec
//     family overview.
package canjson
