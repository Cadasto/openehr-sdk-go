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
// which the options for unmatched and missing members read.
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
//     as `null`. Both absent and `null` are accepted on decode, for an
//     optional member and a mandatory one alike: a mandatory member the
//     input leaves out, or sends as `null`, decodes to its zero value with
//     no error, and [WithMissing] reports it. See "Missing members" below.
//
//   - Empty containers with BMM cardinality.lower == 0 are emitted as
//     absent (omitempty), not as `[]`.
//
//   - A mandatory member that holds no value, a nil list, map, pointer or
//     interface, or a polymorphic slot holding a typed-nil pointer, is
//     emitted as absent, not as `null`; a present empty mandatory list or
//     map is written as `[]` or `{}`. A mandatory `any` field holds a value,
//     not an RM object, so a nil list, a nil map or a typed-nil pointer it
//     holds is written as `null`.
//
//   - ISO 8601 dates/times/durations are passed through as JSON
//     strings; the codec does not parse them to time.Time.
//
//   - Numeric magnitudes use IEEE 754 double-precision JSON numbers
//     (no silent float32 coercion). A lossy literal fails decode with a
//     typed error instead of being silently rounded. A literal is lossy
//     when the float64 it parses to, written back correctly rounded to
//     as many significant decimal digits as the literal carries, denotes
//     another value. 9007199254740993 (2^53+1, which parses to
//     9007199254740992), 1.2345678901234567890, 1.23e-322 (which parses
//     to 1.24e-322) and 1e-400 (which underflows to zero) are lossy, and
//     each fails decode wrapping [ErrInvalidShape], bare or quoted. A
//     literal of at most 15 significant digits is never lossy while its
//     magnitude is at least 2^-1022, the smallest normal float64, so 0.1
//     decodes although no float64 holds it exactly. A longer literal the
//     float64 gives back at its own precision decodes too:
//     18446744073709551616 (2^64), and 0.10000000000000001, the 17-digit
//     spelling of 0.1. An out-of-range magnitude (e.g. 1e400) fails with
//     its own typed error, a *json.SemanticError, reachable with
//     errors.As through the generated decode-method wrapper. A malformed
//     or out-of-range literal reports its own parse/range error first, so
//     the lossy test applies only once a literal has parsed.
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
// Every failure the input causes comes back from [Unmarshal] and
// [Decoder.Decode] as a [DecodeError] at the top of the chain, so the type
// alone no longer tells one failure from another: classify with errors.Is
// against the sentinels named below, or errors.AsType for a typed cause. The
// DecodeError's Path is the failing value's JSON Pointer, where the decoder
// knows it. Its text names the failure's class and the pointer and carries
// no value from the document, so a log line or an HTTP error body that
// quotes it carries none either: no number, no string content, no escape
// sequence or character the tokenizer refused. Member names and `_type`
// values are structure and do appear, a duplicate member's name and an
// unknown `_type` included.
//
// Its Value holds the JSON text of the value the decoder refused, as the
// document wrote it, quotes and escapes included, in two cases:
//
//   - a number or string of the right JSON kind that its Go type cannot
//     hold, such as 3.5 for an rm.Integer64 or 2147483648 for an
//     rm.Integer;
//   - a value one of the SDK's primitive codecs refused: a quoted literal
//     rm.Real, rm.Integer or rm.Integer64 could not parse, a lossy
//     rm.Real number, and an rm.Character string that is
//     not exactly one character or number that names no usable code point.
//
// Value is empty for any other failure: a value of the wrong JSON kind for
// its target (a string where a boolean belongs), a string that is not valid
// base64 for a byte field such as DV_MULTIMEDIA data or integrity_check
// (encoding/json/v2 records the text of neither), malformed JSON, a dispatch
// failure, a nesting-depth refusal and an unknown member. Value prints as
// "[redacted]"; Value.Reveal returns the text. The codec's own error stays
// reachable beneath the DecodeError, and its own text may name the value.
// Two errors come back without a DecodeError, because the input does not
// cause them: io.EOF from a [Decoder] whose stream holds no further value,
// itself, so err == io.EOF holds; and a read failure of the reader under a
// [Decoder], as jsontext reports it, with the reader's own error reachable
// through errors.Is.
//
// What lies beneath that DecodeError depends on where the failure happens:
//
//   - Malformed JSON is refused by the tokenizer as the value it
//     malforms is decoded, except where a value is buffered first: a
//     polymorphic slot is read whole before it is decoded, and so is each
//     value a [Decoder] given [WithUnmatched], [WithStrict] or [WithMissing]
//     reads, so malformed bytes anywhere inside such a value are reported
//     ahead of a shape failure inside it.
//     No sentinel: the cause is the codec's own syntax or truncated-input
//     error (a *jsontext.SyntacticError), except that [Decoder.Decode]
//     reports an empty stream as io.EOF, unwrapped, and a truncated value
//     wraps io.ErrUnexpectedEOF. Invalid UTF-8 and a lone surrogate escape are
//     refused on this same path, before rm.Character sees the bytes, so
//     they too are malformed input carrying no sentinel. rm.Character's
//     own string arm relies on that same tokenizer refusal and does not
//     inspect the raw literal for a substituted U+FFFD.
//   - A duplicate member name is refused during tokenisation. The entry
//     point classifies that refusal with [ErrInvalidShape], keeping the
//     cause reachable, so errors.Is finds both the sentinel and
//     jsontext.ErrDuplicateName (RFC 8259 § 4).
//   - A polymorphic dispatch failure (a missing, unknown or
//     mismatched `_type`) is raised as a [DecodeError] carrying the path,
//     either at a slot or on `/_type` where the whole value's `_type`
//     names a different class than the target; it wraps
//     [typereg.ErrMissingType], [typereg.ErrUnknownType] or
//     [typereg.ErrTypeMismatch]. No sentinel: an
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
//     rm.Integer, rm.Integer64 or rm.Character handed to [Unmarshal]
//     directly rather than reached through a generated type) carries its own
//     `rm.<Type>:` prefix, not the `canjson: <RM_TYPE>:` funnel. Which
//     refusals carry [ErrInvalidShape] differs by primitive. On
//     rm.Character a refusal it raises itself, applying the
//     one-character rule to a value the tokenizer accepted (an empty or
//     multi-character string, and the encoding/json/v2 type mismatch of
//     its string arm), carries the
//     sentinel. A lone surrogate or invalid UTF-8 does not reach
//     rm.Character on this path: the tokenizer refuses it first, so it is
//     malformed input carrying no sentinel, the same treatment as
//     malformed JSON above. On rm.Real only the lossy-literal refusal carries
//     the sentinel; on rm.Integer and rm.Integer64 none does. An empty
//     input carries no sentinel on any of the four, and neither does a
//     strconv or encoding/json/v2 parse or range failure beneath rm.Real,
//     rm.Integer or rm.Integer64; those causes stay reachable with
//     errors.AsType. A nil receiver on any of them, as on every generated
//     type, is a typereg.ErrNilReceiver error.
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
//   - [WithStrict] refuses the decode at the first one, with a
//     [DecodeError] whose Path is its pointer and which wraps
//     [ErrUnknownMember]. An absent member is never refused.
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
// # Missing members
//
// A member is missing when the RM makes the attribute mandatory on the RM
// type its object decodes as, read as "Unmatched members" above reads it,
// and the input carries no value for it: the object has no member of that
// name, or holds `null` there. The decoder cannot always show that in the
// value it returns. A member the SDK holds by value, a number, a boolean, a
// string or a value struct such as PARTY_SELF or ARCHETYPE_ID, decodes to
// its zero value whether the input carried it or not, so a DV_QUANTITY
// without its `magnitude` reads as one of 0 and an EHR_STATUS without its
// `subject` reads as one with a bare PARTY_SELF.
//
// By default the decoder decodes an input with missing members as it decodes
// one without: no error, each such member at its zero value. A missing
// member is never a decode error in any mode: [WithStrict] covers unmatched
// members only.
//
// [WithMissing] passes each missing member to a function, once, as a JSON
// Pointer (RFC 6901) from the root of the value, such as
// "/content/0/data/events/0/data/items/0/value/magnitude": the attribute's
// name under the pointer of the object that lacks it, each item of a list
// by its position counted from 0, each entry of a `Hash` by its key. The
// pointer carries no value from the input. The members of one object are
// reported as the object is read to its end, in the order the RM declares
// the attributes, an ancestor's before the class's own, so the members of
// the objects inside it come first. The decode returns the same value as
// the default.
//
// A member that carries a value is never missing, whatever the value: an
// empty string, a
// zero, an empty list or object and a bare {"_type":"PARTY_SELF"} are
// present, and whether such a value is valid is for the validators to say.
// Three kinds of attribute are never missing: one no serialization carries,
// which is every function the model computes, such as EVENT `offset`, and
// RESOURCE_DESCRIPTION `parent_resource`; the four interval boundary flags,
// which take their defaults when absent; and `_type`, which names no
// attribute.
//
// The option checks the members once the decode has succeeded, in the same
// pass over the input that serves [WithUnmatched], so the two options may be
// combined and a decode that fails reports no member. What it checks is
// what that pass reaches, as "Unmatched members" sets out: the RM values
// reached from the decode target's own RM shape. A value of a class outside
// the RM, such as an AOM 1.4 archetype, a BASE interval, a generic payload
// the decoder holds as plain JSON and a `Hash` of plain values are not
// checked, and nothing inside them is reported.
//
// The option changes nothing about what the decoder reads, and it is not
// available on REST reads through the transport and client packages, which
// decode with the default. The validators in openehr/validation read the
// same report, so a caller that validates a decoded document need not call
// the option itself.
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
