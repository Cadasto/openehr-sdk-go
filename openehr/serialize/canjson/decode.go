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
// instead (REQ-052).
//
// The sentinel is attached over the codec's own error, which stays
// reachable through unwrapping, in two situations (REQ-052):
//
//   - A shape failure raised inside a generated RM type's decode (the
//     `canjson: <RM_TYPE>:` family), where the bytes are valid JSON but
//     the wrong shape for the target. The cause is the codec's own error,
//     a *encoding/json/v2.SemanticError. When the failure happens inside
//     the concrete type selected at a polymorphic slot the error is ALSO
//     a [DecodeError] naming that slot, so both classifications hold: the
//     path from the [DecodeError], the kind from this sentinel.
//   - A JSON object carrying the same member name twice. jsontext refuses
//     it (RFC 8259 § 4: names SHOULD be unique, and duplicates leave no
//     single defined value) before any generated decode runs; the entry
//     point classifies that refusal with this sentinel, preserving the
//     message, so errors.Is finds both this sentinel and
//     jsontext.ErrDuplicateName.
//
// Two decode failures stay OUTSIDE the sentinel by design (REQ-052) and
// never acquire it:
//
//   - Malformed JSON, which the codec reports before any generated decode
//     runs, as the codec's own syntax or truncated-input error (an
//     encoding/json/jsontext.SyntacticError). No sentinel: [Unmarshal]
//     reports it directly, and [Decoder.Decode] reports an empty stream
//     as io.EOF. Invalid UTF-8 and a lone surrogate escape are refused on
//     this same path (jsontext rejects them before rm.Character sees the
//     bytes), so they too are malformed input carrying no sentinel.
//   - A polymorphic dispatch failure (a missing, unknown or mismatched
//     `_type`, at a slot or on `/_type` for the whole value), which
//     arrives as a [DecodeError] carrying the path. No sentinel either,
//     even when it travels out through an enclosing type's
//     `canjson: <RM_TYPE>:` prefix. Match it with errors.As for
//     [DecodeError], or errors.Is against [typereg.ErrMissingType] /
//     [typereg.ErrUnknownType] / [typereg.ErrTypeMismatch].
//
// The value lives in [typereg] so generated code under openehr/rm can
// attach it without forming an `openehr/rm → openehr/serialize` import
// cycle; this is the caller-facing name for it.
var ErrInvalidShape = typereg.ErrInvalidShape

// DecodeError is the unified error returned by the decoder at
// polymorphic dispatch sites. Re-exported from the internal poly
// helper so consumers can `errors.As` against a stable type without
// importing internal packages.
type DecodeError = poly.DecodeError

// DecoderOption configures a [Decoder]. Use [WithRelaxedTypeDispatch]
// to switch the polymorphic-dispatch policy from strict (default)
// to relaxed.
type DecoderOption func(*decoderConfig)

// decoderConfig holds the option-driven state of a Decoder. Kept
// unexported so the option list can grow without churning the
// [Decoder] type.
type decoderConfig struct {
	relaxedTypeDispatch bool
}

// WithRelaxedTypeDispatch toggles the polymorphic-dispatch policy
// from STRICT (default — missing `_type` at a polymorphic site is an
// error) to RELAXED (missing `_type` is allowed when the declared
// abstract field has exactly one concrete descendant in the merged
// BMM; the decoder then instantiates that descendant).
//
// v1 NOTE: the relaxed escape hatch is recognised by the option
// surface but enforced by future generator output — the current
// generated decode methods only implement strict dispatch. Setting
// this option today is a no-op for built-in RM types; the hook stays
// here so the API does not break when the relaxed path lands.
func WithRelaxedTypeDispatch(enabled bool) DecoderOption {
	return func(c *decoderConfig) { c.relaxedTypeDispatch = enabled }
}

// classifyDecode gives a duplicate-object-member-name refusal the
// decode-side shape classification REQ-052 mandates. jsontext raises
// [jsontext.ErrDuplicateName] before any generated decode method runs (a
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
func classifyDecode(err error) error {
	return typereg.ClassifyDuplicate(err)
}

// Unmarshal parses canonical-JSON-encoded data and stores the result
// in the value pointed to by v. v MUST be a non-nil pointer to a
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
func Unmarshal(data []byte, v any) error {
	return classifyDecode(json.Unmarshal(data, v, typereg.Unmarshalers()))
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
// from r. Apply options to configure dispatch policy.
func NewDecoder(r io.Reader, opts ...DecoderOption) *Decoder {
	d := &Decoder{dec: jsontext.NewDecoder(r)}
	for _, o := range opts {
		if o != nil {
			o(&d.cfg)
		}
	}
	return d
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
func (d *Decoder) Decode(v any) error {
	return classifyDecode(json.UnmarshalDecode(d.dec, v, typereg.Unmarshalers()))
}

// RelaxedTypeDispatch reports whether the decoder was configured with
// the relaxed dispatch policy. Used by generated decode methods once
// they support the relaxed path (currently informational only).
func (d *Decoder) RelaxedTypeDispatch() bool { return d.cfg.relaxedTypeDispatch }
