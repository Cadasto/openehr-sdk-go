package canjson

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/internal/poly"
)

// ErrInvalidShape is the canjson-local sentinel reserved for JSON-level
// shape errors (malformed JSON, type mismatch on a non-polymorphic field,
// numeric overflow). It is decode-only: encode failures wrap
// [ErrInvalidValue] instead. No decode path returns it today, so an
// errors.Is against it never matches. Polymorphic-discrimination errors
// come from the typereg package instead: match those with errors.Is
// against [typereg.ErrMissingType] / [typereg.ErrUnknownType] /
// [typereg.ErrTypeMismatch], not against this sentinel.
//
// A decode failure surfaces in one of three other shapes: malformed JSON
// reaches the caller unchanged from encoding/json, a shape error inside
// a generated RM type carries a `canjson: <RM_TYPE>:` prefix, and a
// failure at a polymorphic slot arrives as [DecodeError]. Wrapping
// decode failures with this sentinel is a spec-first REQ-052 follow-up,
// tracked by docs/plans/archive/2026-08-30-read-path-decode-taxonomy.md.
var ErrInvalidShape = errors.New("canjson: invalid JSON shape")

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
// generated [UnmarshalJSON] methods only implement strict dispatch.
// Setting this option today is a no-op for built-in RM types; the
// hook stays here so the API does not break when the relaxed path
// lands.
func WithRelaxedTypeDispatch(enabled bool) DecoderOption {
	return func(c *decoderConfig) { c.relaxedTypeDispatch = enabled }
}

// Unmarshal parses canonical-JSON-encoded data and stores the result
// in the value pointed to by v. v MUST be a non-nil pointer to a
// generated RM type (or a slice/map containing such types).
//
// Polymorphic fields on v are populated via the per-type
// [UnmarshalJSON] methods the BMM generator emits; each consults
// [typereg.Default] to resolve `_type` discriminators.
//
// Returns [poly.DecodeError] wrapping a typereg sentinel
// ([typereg.ErrMissingType] / ErrUnknownType / ErrTypeMismatch) at
// polymorphic failures (via generated UnmarshalJSON). Malformed JSON
// comes back unchanged from encoding/json as *json.SyntaxError; a
// shape error inside a generated RM type keeps a
// `canjson: <RM_TYPE>:` prefix from that type's UnmarshalJSON. None
// of these wraps [ErrInvalidShape] (REQ-052 producer deferred — see
// the sentinel's own documentation).
func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// Decoder reads and decodes canonical-JSON values from a stream.
// Wrapping `encoding/json.Decoder` keeps the swap path (sonic /
// easyjson) cheap.
type Decoder struct {
	dec *json.Decoder
	cfg decoderConfig
}

// NewDecoder returns a [Decoder] that reads canonical-JSON values
// from r. Apply options to configure dispatch policy.
func NewDecoder(r io.Reader, opts ...DecoderOption) *Decoder {
	d := &Decoder{dec: json.NewDecoder(r)}
	for _, o := range opts {
		if o != nil {
			o(&d.cfg)
		}
	}
	return d
}

// Decode reads the next JSON value from the stream and stores it in
// v. Errors follow the same classification as [Unmarshal], except
// where reading a stream rather than a whole input changes the
// answer: a truncated value is reported as io.ErrUnexpectedEOF here,
// where [Unmarshal] reports a *json.SyntaxError ("unexpected end of
// JSON input"); an empty or whitespace-only stream is io.EOF; and
// content after the first value is simply the next value in the
// stream, not a syntax error — `{"a":1}x` fails in [Unmarshal] and
// succeeds here. Other syntax errors are the same *json.SyntaxError
// in both.
func (d *Decoder) Decode(v any) error {
	return d.dec.Decode(v)
}

// RelaxedTypeDispatch reports whether the decoder was configured with
// the relaxed dispatch policy. Used by generated [UnmarshalJSON]
// methods once they support the relaxed path (currently informational
// only).
func (d *Decoder) RelaxedTypeDispatch() bool { return d.cfg.relaxedTypeDispatch }
