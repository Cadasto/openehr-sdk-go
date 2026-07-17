package canxml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// NSXMI is the XMI namespace. ITS-XML rejects `xmi:type` on the
// wire — only `xsi:type` is recognised. The decoder maps any
// `xmi:type` it sees to [ErrInvalidShape].
const NSXMI = "http://www.omg.org/XMI"

// DecoderOption configures a [Decoder]. Use [WithRelaxedTypeDispatch]
// to switch the polymorphic-dispatch policy from strict (default)
// to relaxed.
type DecoderOption func(*decoderConfig)

// decoderConfig holds the option-driven state of a Decoder.
type decoderConfig struct {
	relaxedTypeDispatch bool
}

// WithRelaxedTypeDispatch toggles the polymorphic-dispatch policy
// for ABSTRACT slots from STRICT (default — missing `xsi:type` at a
// polymorphic site is an error) to RELAXED (missing `xsi:type` is
// allowed when the declared abstract field has exactly one concrete
// descendant in the merged BMM; the decoder then instantiates that
// descendant). Scope: abstract slots only — slot types like
// `DATA_VALUE`, `DV_ORDERED`, `ITEM_STRUCTURE`, `PARTY_PROXY`.
//
// REQ-052 narrow-interface slots (`<Parent>Like` — DVTextLike,
// PartyIdentifiedLike, …) have an independent, always-on fallback:
// a missing `xsi:type` defaults to the declared parent's concrete
// type, served by [DecodeAsOrDefault] from the generator emission.
// That fallback is deterministic (the parent type is fixed by the
// BMM) so it is not gated by this option.
//
// v1 NOTE: the relaxed escape hatch for ABSTRACT slots is recognised
// by the option surface but enforced by future generator output —
// the current generated [UnmarshalXML] methods at abstract slots
// still implement strict dispatch. Setting this option today is a
// no-op for those slots; the hook stays so the API does not break
// when the relaxed path lands.
func WithRelaxedTypeDispatch(enabled bool) DecoderOption {
	return func(c *decoderConfig) { c.relaxedTypeDispatch = enabled }
}

// Decoder reads and decodes canonical-XML values from a stream.
// Wrapping `encoding/xml.Decoder` keeps the swap path cheap.
type Decoder struct {
	dec *xml.Decoder
	cfg decoderConfig
}

// NewDecoder returns a [Decoder] that reads canonical-XML from r.
func NewDecoder(r io.Reader, opts ...DecoderOption) *Decoder {
	d := &Decoder{dec: xml.NewDecoder(r)}
	for _, o := range opts {
		o(&d.cfg)
	}
	return d
}

// Decode reads the next XML element from the stream and stores it
// in v. Errors follow the same classification as [Unmarshal].
func (d *Decoder) Decode(v any) error {
	return d.dec.Decode(v)
}

// RelaxedTypeDispatch reports whether the decoder was configured
// with the relaxed dispatch policy. Used by generated [UnmarshalXML]
// methods once they support the relaxed path (currently
// informational only).
func (d *Decoder) RelaxedTypeDispatch() bool { return d.cfg.relaxedTypeDispatch }

// Unmarshal parses canonical-XML-encoded data and stores the result
// in the value pointed to by v. v MUST be a non-nil pointer to a
// generated RM type whose UnmarshalXML method is wired through this
// package (every concrete RM class generated under openehr/rm/
// implements it).
//
// Polymorphic fields on v are populated via the per-type
// [UnmarshalXML] methods the BMM generator emits; each consults
// [typereg.Default] to resolve `xsi:type` discriminators.
//
// Returns [DecodeError] wrapping a typereg sentinel
// ([typereg.ErrMissingType] / ErrUnknownType / ErrTypeMismatch) at
// polymorphic failures, and [ErrInvalidShape] (wrapped) for XML
// shape errors such as `xmi:type` or malformed content.
func Unmarshal(data []byte, v any) error {
	return xml.NewDecoder(bytes.NewReader(data)).Decode(v)
}

// XSITypeOf scans a start element's attribute list and returns the
// value of the `xsi:type` discriminator. It accepts the
// namespace-resolved form (`Space == NSXSI, Local == "type"`) — what
// encoding/xml produces when `xmlns:xsi` is in scope — and the
// literal form (`Local == "xsi:type"`) produced by directly
// constructed tokens, e.g. the encoder's own [XSITypeAttrName].
// Note an `xsi:type` written with NO in-scope `xmlns:xsi` does not
// take the literal branch: encoding/xml yields `Space == "xsi",
// Local == "type"`, which matches neither branch and is reported as a
// missing discriminator (below).
//
// The returned discriminator is normalised: a leading `xsd:` (XML
// Schema datatype) prefix is stripped so foundation primitives like
// `xsd:string` decode against the BMM primitive name `String`. A
// namespace-prefixed RM value (e.g. Better's `ns2:DV_QUANTITY`) is
// NOT stripped and so fails registry lookup as an unknown type —
// namespace-prefixed discriminators are out of scope per
// docs/specifications/wire.md § REQ-056.
//
// Returns [ErrInvalidShape] when an `xmi:type` attribute is
// encountered — ITS-XML pins `xsi:type` and the SDK rejects XMI
// discriminators on the wire.
//
// Returns ("", nil) when the element carries no discriminator at
// all; the caller decides whether that is an error (strict) or
// acceptable (relaxed dispatch).
func XSITypeOf(start xml.StartElement) (string, error) {
	for _, attr := range start.Attr {
		// Reject xmi:type (both resolved and literal forms).
		if attr.Name.Local == "type" && attr.Name.Space == NSXMI {
			return "", fmt.Errorf("canxml: %w: xmi:type on the wire is rejected (use xsi:type)", ErrInvalidShape)
		}
		if attr.Name.Local == "xmi:type" {
			return "", fmt.Errorf("canxml: %w: xmi:type on the wire is rejected (use xsi:type)", ErrInvalidShape)
		}
		// Accept xsi:type (resolved and literal forms).
		if attr.Name.Local == "type" && (attr.Name.Space == NSXSI || attr.Name.Space == "") {
			return stripXSDPrefix(attr.Value), nil
		}
		if attr.Name.Local == "xsi:type" {
			return stripXSDPrefix(attr.Value), nil
		}
	}
	return "", nil
}

// stripXSDPrefix strips a leading `xsd:` namespace prefix from a
// discriminator. Some ITS-XML producers tag foundation primitives
// with their XML Schema datatype (`xsd:string`, `xsd:int`, …); the
// SDK's type registry indexes the BMM primitive name (`String`,
// `Integer`, …). The stripped name is fed to typereg.Lookup.
//
// Idempotent for inputs that already lack the prefix.
func stripXSDPrefix(s string) string {
	const prefix = "xsd:"
	if len(s) > len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

// DecodeAs reads one polymorphic child element from dec — the
// element whose start token is `start` — and returns the typereg-
// dispatched concrete value, asserted to T. Used by generated
// UnmarshalXML methods at every polymorphic-field decode site.
//
// Behaviour:
//
//   - If start carries `xsi:type`, the discriminator is looked up in
//     [typereg.Default]; the matching constructor produces a fresh
//     concrete instance which receives the element body via
//     `dec.DecodeElement`.
//   - If start carries no `xsi:type`, returns [typereg.ErrMissingType]
//     wrapped in [DecodeError]. Relaxed-dispatch fallback lives on
//     the generator and the [Decoder.RelaxedTypeDispatch] flag —
//     see canxml/doc.go.
//   - If the concrete value does not satisfy T, returns
//     [typereg.ErrTypeMismatch] wrapped in [DecodeError].
//   - The body bytes are consumed up to and including the matching
//     end element so the parent decoder is positioned correctly for
//     the next child.
func DecodeAs[T any](dec *xml.Decoder, start xml.StartElement) (T, error) {
	var zero T
	typeName, err := XSITypeOf(start)
	if err != nil {
		return zero, &DecodeError{Path: "/" + start.Name.Local, Inner: err}
	}
	if typeName == "" {
		return zero, &DecodeError{Path: "/" + start.Name.Local, Inner: fmt.Errorf("canxml: %w", typereg.ErrMissingType)}
	}
	ctor, ok := typereg.Default.Lookup(typeName)
	if !ok {
		return zero, &DecodeError{
			Path:  "/" + start.Name.Local,
			Type:  typeName,
			Inner: fmt.Errorf("canxml: %q: %w", typeName, typereg.ErrUnknownType),
		}
	}
	v := ctor()
	if err := dec.DecodeElement(v, &start); err != nil {
		return zero, &DecodeError{
			Path:  "/" + start.Name.Local,
			Type:  typeName,
			Inner: fmt.Errorf("canxml: %w", err),
		}
	}
	if t, ok := v.(T); ok {
		return t, nil
	}
	// Registry ctors return pointers; when T is a concrete value
	// shape (e.g. `DVInterval[DVQuantity].Lower` dispatches via
	// `DecodeAs[DVQuantity]`), assert to T first, then close the
	// pointer-to-value gap via *T without reflection.
	if pt, ok := v.(*T); ok && pt != nil {
		return *pt, nil
	}
	return zero, &DecodeError{
		Path:  "/" + start.Name.Local,
		Type:  typeName,
		Inner: fmt.Errorf("canxml: decoded %T: %w", v, typereg.ErrTypeMismatch),
	}
}

// DecodeAsOrDefault is the polySingleNarrow (REQ-052) XML
// counterpart of [DecodeAs]. When the element carries an `xsi:type`,
// dispatch goes through [typereg.Default] exactly like DecodeAs.
// When `xsi:type` is absent, the supplied defaultCtor instantiates
// the declared parent type and dec.DecodeElement populates it —
// preserving openEHR canonical XML where the static field type fixes
// the concrete subtype.
func DecodeAsOrDefault[T any](dec *xml.Decoder, start xml.StartElement, defaultCtor func() any) (T, error) {
	var zero T
	typeName, err := XSITypeOf(start)
	if err != nil {
		return zero, &DecodeError{Path: "/" + start.Name.Local, Inner: err}
	}
	var v any
	if typeName == "" {
		if defaultCtor == nil {
			return zero, &DecodeError{Path: "/" + start.Name.Local, Inner: fmt.Errorf("canxml: %w", typereg.ErrMissingType)}
		}
		v = defaultCtor()
		if err := dec.DecodeElement(v, &start); err != nil {
			return zero, &DecodeError{
				Path:  "/" + start.Name.Local,
				Inner: fmt.Errorf("canxml: %w", err),
			}
		}
	} else {
		ctor, ok := typereg.Default.Lookup(typeName)
		if !ok {
			return zero, &DecodeError{
				Path:  "/" + start.Name.Local,
				Type:  typeName,
				Inner: fmt.Errorf("canxml: %q: %w", typeName, typereg.ErrUnknownType),
			}
		}
		v = ctor()
		if err := dec.DecodeElement(v, &start); err != nil {
			return zero, &DecodeError{
				Path:  "/" + start.Name.Local,
				Type:  typeName,
				Inner: fmt.Errorf("canxml: %w", err),
			}
		}
	}
	if t, ok := v.(T); ok {
		return t, nil
	}
	if pt, ok := v.(*T); ok && pt != nil {
		return *pt, nil
	}
	return zero, &DecodeError{
		Path:  "/" + start.Name.Local,
		Type:  typeName,
		Inner: fmt.Errorf("canxml: decoded %T: %w", v, typereg.ErrTypeMismatch),
	}
}
