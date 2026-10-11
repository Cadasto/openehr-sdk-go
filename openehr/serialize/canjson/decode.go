package canjson

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"io"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/internal/poly"
)

// ErrInvalidShape is the canjson sentinel for JSON-level shape errors:
// valid JSON in the wrong shape for the target RM type, such as a type
// mismatch on a non-polymorphic field or a magnitude out of float64
// range. It is decode-only: encode failures wrap [ErrInvalidValue]
// instead.
//
// The sentinel is attached over the codec's own error, which stays
// reachable through unwrapping, in two situations:
//
//   - A shape failure raised inside a generated RM type's decode (the
//     `canjson: <RM_TYPE>:` family), where the bytes are valid JSON but
//     the wrong shape for the target. The cause is the codec's own error,
//     a *encoding/json/v2.SemanticError. When the failure happens inside
//     the concrete type selected at a polymorphic slot the error is also
//     a [DecodeError] naming that slot, so both classifications hold: the
//     path from the [DecodeError], the kind from this sentinel.
//   - A JSON object carrying the same member name twice. jsontext refuses
//     it during tokenisation (RFC 8259 § 4: names should be unique, and
//     duplicates leave no single defined value); the entry
//     point classifies that refusal with this sentinel, preserving the
//     message, so errors.Is finds both this sentinel and
//     jsontext.ErrDuplicateName.
//
// Four decode failures never carry the sentinel:
//
//   - Malformed JSON, which the codec reports as its own syntax or
//     truncated-input error (an encoding/json/jsontext.SyntacticError).
//     No sentinel: [Unmarshal]
//     reports it directly, and [Decoder.Decode] reports an empty stream
//     as io.EOF. Invalid UTF-8 and a lone surrogate escape are refused on
//     this same path (jsontext rejects them before rm.Character sees the
//     bytes), so they too are malformed input carrying no sentinel.
//   - A read failure from the underlying reader, on the streaming
//     [Decoder] entry point: jsontext reports it as its own IO error,
//     and it passes through unwrapped, carrying no sentinel. The
//     reader's own error stays reachable with errors.Is.
//   - A polymorphic dispatch failure (a missing, unknown or mismatched
//     `_type`, at a slot or on `/_type` for the whole value), which
//     arrives as a [DecodeError] carrying the path. No sentinel either,
//     even when it travels out through an enclosing type's
//     `canjson: <RM_TYPE>:` prefix. Match it with errors.As for
//     [DecodeError], or errors.Is against [typereg.ErrMissingType] /
//     [typereg.ErrUnknownType] / [typereg.ErrTypeMismatch].
//   - A nesting-depth refusal: [typereg.ErrMaxDepthExceeded] inside a
//     [DecodeError], raised when a value opens past the 512-level bound.
//     It passes through the enclosing values unchanged, so no
//     `canjson: <RM_TYPE>:` funnel adds the sentinel to it. Match it with
//     errors.Is against [typereg.ErrMaxDepthExceeded].
//
// The value lives in [typereg] so generated code under openehr/rm can
// attach it without forming an `openehr/rm → openehr/serialize` import
// cycle; this is the caller-facing name for it.
var ErrInvalidShape = typereg.ErrInvalidShape

// ErrUnknownMember is the sentinel for a member the decode does not read:
// its name is neither `_type` nor a member of the RM type its object decodes
// as. Such a member is ignored by default. Only a decode given [WithStrict]
// refuses one, and the refusal wraps this sentinel and names the member's
// JSON Pointer from the root of the input (RFC 6901). The pointer carries
// the member's name but none of its value.
//
// It is distinct from [ErrInvalidShape] and [ErrInvalidValue]: the member is
// not malformed, it is unknown, and errors.Is against one of the three
// sentinels never matches an error raised for another. Like
// [ErrInvalidShape], the value lives in [typereg] so generated code can
// reach it; this is the caller-facing name for it.
var ErrUnknownMember = typereg.ErrUnknownMember

// DecodeError is the unified error returned by the decoder at
// polymorphic dispatch sites. Re-exported from the internal poly
// helper so consumers can `errors.As` against a stable type without
// importing internal packages.
type DecodeError = poly.DecodeError

// DecoderOption configures a decode, through [Unmarshal] or a [Decoder].
// [WithUnmatched] and [WithStrict] say what happens to members the decode
// does not read; [WithRelaxedTypeDispatch] switches the polymorphic-dispatch
// policy from strict (default) to relaxed.
type DecoderOption func(*decoderConfig)

// decoderConfig holds the option-driven state of a decode. Kept
// unexported so the option list can grow without churning the
// [Decoder] type.
type decoderConfig struct {
	relaxedTypeDispatch bool
	unmatched           func(pointer string)
	strict              bool
}

// newDecoderConfig applies opts in order, skipping a nil option.
func newDecoderConfig(opts []DecoderOption) decoderConfig {
	var c decoderConfig
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	return c
}

// target returns what the decode writes into: v itself by default, or v
// wrapped so that unmatched members are reported or refused when an option
// asks for either.
func (c decoderConfig) target(v any) any {
	if c.unmatched == nil && !c.strict {
		return v
	}
	return typereg.TrackUnmatched(v, c.unmatched, c.strict)
}

// WithUnmatched passes every unmatched member of the input to fn while the
// decode carries on. A member is unmatched when its name is neither `_type`
// nor a member of the RM type its object decodes as, at any depth: inside a
// polymorphic slot, an interval bound or an AOM 1.4 value too.
//
// By default the decoder ignores unmatched members: the value it returns
// holds the rest of the input and none of them. With this option fn receives
// each one, once, in the order it appears in the input, and the value
// returned is the same as without the option. fn gets the member's JSON
// Pointer (RFC 6901) from the root of the input, such as
// "/content/0/data/x-note"; the pointer carries the member's name, never its
// value. fn runs during the decode, on the caller's goroutine.
//
// A nil fn is the same as leaving the option out. Combined with
// [WithStrict], fn sees the first unmatched member before the decode is
// refused.
//
// REST reads through the transport and client packages decode with the
// default and ignore unmatched members.
func WithUnmatched(fn func(pointer string)) DecoderOption {
	return func(c *decoderConfig) { c.unmatched = fn }
}

// WithStrict refuses the decode at its first unmatched member: a member
// whose name is neither `_type` nor a member of the RM type its object
// decodes as, at any depth. The error wraps [ErrUnknownMember] and names the
// member's JSON Pointer (RFC 6901) from the root of the input; it carries
// none of the member's value. The value v points to is unspecified after the
// refusal.
//
// Strict covers unmatched members only. An absent member is never refused,
// mandatory or not, and neither is a boundary flag an interval leaves out.
// The option is unrelated to `_type` dispatch: an unknown `_type` fails in
// every mode (see [DecodeError]).
//
// By default the decoder ignores unmatched members. REST reads through the
// transport and client packages decode with the default.
func WithStrict() DecoderOption {
	return func(c *decoderConfig) { c.strict = true }
}

// WithRelaxedTypeDispatch toggles the polymorphic-dispatch policy
// from strict (the default: missing `_type` at a polymorphic site is an
// error) to relaxed (missing `_type` is allowed when the declared
// abstract field has exactly one concrete descendant in the merged
// BMM; the decoder then instantiates that descendant).
//
// The option is accepted but not yet enforced: the generated decode
// methods implement strict dispatch only, so setting it is currently a
// no-op for built-in RM types. It exists so the API does not change
// when relaxed dispatch is implemented.
func WithRelaxedTypeDispatch(enabled bool) DecoderOption {
	return func(c *decoderConfig) { c.relaxedTypeDispatch = enabled }
}

// classifyDecode gives a duplicate-object-member-name refusal the
// decode-side shape classification REQ-052 mandates. jsontext raises
// [jsontext.ErrDuplicateName] during tokenisation (a
// well-formed value whose shape is nonetheless rejected), so it reaches
// the entry point as a bare *jsontext.SyntacticError with no sentinel.
// [typereg.ClassifyShape] attaches [ErrInvalidShape] the same way every
// in-funnel shape failure is wrapped: the message is preserved and a
// single errors.Unwrap step still lands on the cause, so errors.Is finds
// both this sentinel and jsontext.ErrDuplicateName. Every other error is
// returned untouched: a shape failure raised inside a generated type
// already carries the sentinel from its own funnel, and malformed JSON
// and dispatch failures MUST NOT acquire it.
//
// The gate itself lives in [typereg.ClassifyDuplicate], shared with the
// registry decode route ([typereg.Registry.Decode]) so both refuse a
// duplicate the same way. canjson keeps its own application here because
// [Unmarshal] and [Decoder.Decode] accept non-RM targets, which never
// travel through the registry, so the registry gate alone would not cover
// them.
//
// A [WithStrict] refusal of an unmatched member is returned on its own
// ([typereg.UnwrapUnknownMember]): it names the member from the root of the
// input, where the envelopes the decoder wrapped around it name positions
// relative to inner values.
func classifyDecode(err error) error {
	return typereg.ClassifyDuplicate(typereg.UnwrapUnknownMember(err))
}

// Unmarshal parses canonical-JSON-encoded data and stores the result
// in the value pointed to by v. v must be a non-nil pointer to a
// generated RM type (or a slice/map containing such types).
//
// Polymorphic fields on v are populated via the per-type decode methods
// the BMM generator emits (encoding/json/v2); each consults
// [typereg.Default] to resolve `_type` discriminators. The entry point
// threads [typereg.Unmarshalers], the aggregate of every generated
// interface hook, so a polymorphic slot at the top level resolves too.
//
// Returns a [DecodeError] wrapping a typereg sentinel
// ([typereg.ErrMissingType] / ErrUnknownType / ErrTypeMismatch) at
// polymorphic dispatch failures. Malformed JSON comes back as a
// *encoding/json/jsontext.SyntacticError carrying no sentinel; a
// duplicate member name and a shape error inside a generated RM type
// both wrap [ErrInvalidShape], the latter behind a `canjson: <RM_TYPE>:`
// prefix. A shape failure beneath a polymorphic slot carries both the
// sentinel and a [DecodeError]. See the sentinel's own documentation for
// where the lines fall.
//
// A member the target does not read is ignored by default. [WithUnmatched]
// reports each such member and [WithStrict] refuses the first, wrapping
// [ErrUnknownMember].
//
// The value v points to is unspecified after an error: a failed decode may
// leave it partly written.
func Unmarshal(data []byte, v any, opts ...DecoderOption) error {
	cfg := newDecoderConfig(opts)
	return classifyDecode(json.Unmarshal(data, cfg.target(v), typereg.Unmarshalers()))
}

// Decoder reads and decodes canonical-JSON values from a stream.
// It wraps an [encoding/json/jsontext.Decoder] and drives the
// generated encoding/json/v2 decode methods through
// [encoding/json/v2.UnmarshalDecode].
type Decoder struct {
	dec *jsontext.Decoder
	cfg decoderConfig
}

// NewDecoder returns a [Decoder] that reads canonical-JSON values
// from r. The options apply to every value it decodes; a JSON Pointer that
// [WithUnmatched] or [WithStrict] reports is relative to the value being
// decoded.
func NewDecoder(r io.Reader, opts ...DecoderOption) *Decoder {
	return &Decoder{dec: jsontext.NewDecoder(r), cfg: newDecoderConfig(opts)}
}

// Decode reads the next JSON value from the stream and stores it in
// v. Errors follow the same classification as [Unmarshal], except
// where reading a stream rather than a whole input changes the answer.
// An empty or whitespace-only stream is io.EOF, where [Unmarshal]
// reports the codec's own syntax error; and content after the first
// value is simply the next value in the stream, not an error, so
// `{"a":1}x` fails in [Unmarshal] and succeeds here. A truncated
// value is a *jsontext.SyntacticError wrapping io.ErrUnexpectedEOF in
// both.
//
// The value v points to is unspecified after an error: a failed decode may
// leave it partly written.
func (d *Decoder) Decode(v any) error {
	return classifyDecode(json.UnmarshalDecode(d.dec, d.cfg.target(v), typereg.Unmarshalers()))
}

// RelaxedTypeDispatch reports whether the decoder was configured with
// the relaxed dispatch policy. Used by generated decode methods once
// they support the relaxed path (currently informational only).
func (d *Decoder) RelaxedTypeDispatch() bool { return d.cfg.relaxedTypeDispatch }
