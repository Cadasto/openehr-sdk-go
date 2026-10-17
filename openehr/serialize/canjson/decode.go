package canjson

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
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
//     a *encoding/json/v2.SemanticError, except that an interval side
//     written bounded with no bound, refused where the bound is held by
//     value, has a plain error naming the flag as its cause when the
//     interval is the decode root. When the failure happens inside
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
//   - Malformed JSON, whose cause is the codec's own syntax or
//     truncated-input error (an encoding/json/jsontext.SyntacticError)
//     inside the [DecodeError] every failure comes back in. No sentinel;
//     [Decoder.Decode] reports an empty stream as io.EOF. Invalid UTF-8
//     and a lone surrogate escape are refused on this same path (jsontext
//     rejects them before rm.Character sees the bytes), so they too are
//     malformed input carrying no sentinel.
//   - A read failure from the underlying reader, on the streaming
//     [Decoder] entry point: jsontext reports it as its own IO error,
//     and it passes through unwrapped, carrying no sentinel. The
//     reader's own error stays reachable with errors.Is.
//   - A polymorphic dispatch failure (a missing, unknown or mismatched
//     `_type`, at a slot or on `/_type` for the whole value), raised as a
//     [DecodeError] carrying the path. No sentinel either, even when it
//     travels out through an enclosing type's `canjson: <RM_TYPE>:`
//     prefix. Every failure the input causes is a [DecodeError] at the
//     top, so match a dispatch failure with errors.Is against
//     [typereg.ErrMissingType] / [typereg.ErrUnknownType] /
//     [typereg.ErrTypeMismatch], not by the type.
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

// DecodeError is the error [Unmarshal] and [Decoder.Decode] return for
// every failure the input causes, at the top of the chain, so
// errors.AsType[*DecodeError] finds it without unwrapping. Its Path is the
// JSON Pointer of the failing value from the root of the document, where the
// decoder knows it, and its text names the failure's class and the pointer
// but no value from the document.
//
// Its Value holds the JSON text of the value the decoder refused, as the
// document wrote it, in two cases: a number or string of the right JSON kind
// that its Go type cannot hold, and a value one of the SDK's primitive codecs
// refused (a quoted literal rm.Real or rm.Integer could not parse, an rm.Real
// number past its 17-digit budget, an rm.Character string that is not exactly
// one character). Value is empty for any other failure, a value of the wrong
// JSON kind for its target among them.
//
// Path, Type and the text are value-free; Value is value-bearing and prints
// as "[redacted]". The codec's own error is its Inner, so every sentinel and
// typed cause stays reachable with errors.Is and errors.As, and that cause's
// own text may name the value. A polymorphic slot that fails adds a
// DecodeError of its own, naming the slot, beneath the entry point's.
//
// Two errors come back as they are, because the input does not cause them:
// a read failure of the reader under a [Decoder], and io.EOF from a
// [Decoder] whose stream holds no further value.
//
// Since every failure is a DecodeError, classify by sentinel: see
// [ErrInvalidShape] for where the lines fall. The type is re-exported from
// the internal poly helper so consumers can match it without importing
// internal packages; [typereg.DecodeError] documents its fields.
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
// them. [WithUnmatched] and [WithStrict] check nothing in a target whose
// own shape is no RM class; a duplicate name in it is refused all the same.
func classifyDecode(err error) error {
	return typereg.ClassifyDuplicate(err)
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
// Every failure comes back as a [DecodeError] at the top of the chain,
// whose text carries no value from the document. Classify it by
// what it wraps: a polymorphic dispatch failure wraps a typereg sentinel
// ([typereg.ErrMissingType] / ErrUnknownType / ErrTypeMismatch); malformed
// JSON wraps a *encoding/json/jsontext.SyntacticError and carries no
// sentinel; a duplicate member name and a shape error inside a generated
// RM type both wrap [ErrInvalidShape], the latter behind a
// `canjson: <RM_TYPE>:` prefix. A shape failure beneath a polymorphic slot
// carries both the sentinel and the slot's own [DecodeError]. See the
// sentinel's own documentation for where the lines fall.
//
// A member the target does not read is ignored by default. [WithUnmatched]
// reports each such member and [WithStrict] refuses the first, wrapping
// [ErrUnknownMember]. Either option checks the members once the decode has
// succeeded and changes nothing about the decode itself; with neither, no
// member is checked.
//
// The value v points to is unspecified after an error: a failed decode may
// leave it partly written.
func Unmarshal(data []byte, v any, opts ...DecoderOption) error {
	err := classifyDecode(json.Unmarshal(data, v, typereg.Unmarshalers()))
	if err == nil && len(opts) > 0 {
		err = checkUnmatched(data, v, opts)
	}
	return entryError(err)
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
// v. Errors take the form and classification [Unmarshal]'s do, a
// [DecodeError] at the top of the chain, except where reading a stream
// rather than a whole input changes the answer. Two errors the input does
// not cause come back as they are: io.EOF once the stream holds no further
// value, an empty or whitespace-only stream included, where [Unmarshal]
// reports malformed JSON; and a read failure of the underlying reader.
// Content after the first value is simply the next value in the stream,
// not an error, so `{"a":1}x` fails in [Unmarshal] and succeeds here. A
// truncated value wraps a *jsontext.SyntacticError wrapping
// io.ErrUnexpectedEOF in both.
//
// With [WithUnmatched] or [WithStrict], Decode first reads the whole next
// value into the decoder's read buffer, then decodes those bytes as
// [Unmarshal] does and checks their members, with no copy of its own.
// Reading the value whole first changes which failure a bad value reports:
// malformed bytes anywhere in the value come back ahead of a shape failure
// earlier in it, and a shape failure's byte offset counts from the start of
// the value, not of the stream.
//
// The value v points to is unspecified after an error: a failed decode may
// leave it partly written.
func (d *Decoder) Decode(v any) error {
	if !d.cfg.checksMembers() {
		return streamError(classifyDecode(json.UnmarshalDecode(d.dec, v, typereg.Unmarshalers())))
	}
	raw, err := d.dec.ReadValue()
	if err != nil {
		return streamError(classifyDecode(err))
	}
	if err := classifyDecode(json.Unmarshal(raw, v, typereg.Unmarshalers())); err != nil {
		return entryError(err)
	}
	return entryError(d.cfg.walkUnmatched(raw, v))
}

// streamError returns err, an error from reading the stream, as
// [Decoder.Decode] reports it. jsontext and encoding/json/v2 report a failure
// the input causes as a *jsontext.SyntacticError or a *json.SemanticError,
// the SDK's own [DecodeError] travelling inside the latter, and such a
// failure comes back in the envelope [Unmarshal] returns (see
// entryError). The two errors the input does not cause carry none
// of these and come back as they are: io.EOF once the stream holds no
// further value, so a caller's err == io.EOF holds, and the reader's own
// failure, which jsontext returns as an I/O error wrapping it.
func streamError(err error) error {
	if err == nil || !fromInput(err) {
		return err
	}
	return entryError(err)
}

// fromInput reports whether err's chain holds an error the decoder raises for
// its input: a *jsontext.SyntacticError, a *json.SemanticError or a
// [DecodeError].
func fromInput(err error) bool {
	if _, ok := errors.AsType[*jsontext.SyntacticError](err); ok {
		return true
	}
	if _, ok := errors.AsType[*json.SemanticError](err); ok {
		return true
	}
	_, ok := errors.AsType[*DecodeError](err)
	return ok
}

// RelaxedTypeDispatch reports whether the decoder was configured with
// the relaxed dispatch policy. Used by generated decode methods once
// they support the relaxed path (currently informational only).
func (d *Decoder) RelaxedTypeDispatch() bool { return d.cfg.relaxedTypeDispatch }
