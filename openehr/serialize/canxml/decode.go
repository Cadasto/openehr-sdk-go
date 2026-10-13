package canxml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlunmatched"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// NSXMI is the XMI namespace. ITS-XML rejects `xmi:type` on the
// wire; only `xsi:type` is recognised. The decoder maps any
// `xmi:type` it sees to [ErrInvalidShape].
const NSXMI = "http://www.omg.org/XMI"

// DecoderOption configures a decode, through [Unmarshal] or a [Decoder].
// [WithUnmatched] and [WithStrict] say what happens to content the decode
// does not read. [WithRelaxedTypeDispatch] names relaxed polymorphic
// dispatch; it is accepted but not yet enforced, so dispatch stays strict.
type DecoderOption func(*decoderConfig)

// decoderConfig holds the option-driven state of a decode. Kept unexported so
// the option list can grow without churning the [Decoder] type.
type decoderConfig struct {
	relaxedTypeDispatch bool
	unmatched           func(path string)
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

// tracked reports whether the decode reports or refuses unmatched content.
func (c decoderConfig) tracked() bool {
	return c.unmatched != nil || c.strict
}

// WithUnmatched passes every unmatched item of the input to fn while the
// decode carries on. An item is unmatched when the decode does not read it:
// an element or attribute that names no attribute of the RM type its element
// decodes as, other than namespace declarations and attributes in the XML
// Schema instance namespace (`xsi:type`, `xsi:schemaLocation`, `xsi:nil`,
// …); an element in a namespace other than the openEHR one (see
// [ErrNamespace]), the root element aside; and an element whose content this codec does not model:
// a `Hash` member, a BMM `Any` member (the AOM 1.4 `assumed_value` and
// `EXPR_LEAF.item`), and a member whose Go type has no decoder of its own
// (see the package documentation). A bare `type` attribute is unmatched
// except where dispatch reads it as the discriminator. It applies at any
// depth: inside a polymorphic slot, an interval bound, a primitive element or
// an AOM 1.4 value too.
//
// By default the decoder ignores unmatched content: the value it returns
// holds the rest of the input and none of it. With this option fn receives
// each item, once, in the order it appears in the input, and the value
// returned is the same as without the option. An unmatched element is one
// item, whatever it holds.
//
// fn gets the item's path from the root of the value: one `/name[n]` segment
// per element, where name is the element's local name and n its position
// among the earlier siblings of the same name plus one, counting from 1; an
// attribute adds a last `/@name` segment. For example
// "/composition[1]/content[2]/x-note[1]" or "/composition[1]/@x-flag". The
// path carries names only, never a value from the input. fn runs during the
// decode, on the caller's goroutine.
//
// A nil fn is the same as leaving the option out. Combined with
// [WithStrict], fn sees the first unmatched item before the decode is
// refused.
func WithUnmatched(fn func(path string)) DecoderOption {
	return func(c *decoderConfig) { c.unmatched = fn }
}

// WithStrict refuses the decode at its first unmatched item (see
// [WithUnmatched] for what is unmatched and for the form of its path). The
// error wraps [ErrUnknownElement] and names the item's path from the root of
// the value; it carries none of the item's value. An element in a foreign
// namespace also wraps [ErrNamespace]. The value v points to is unspecified
// after the refusal.
//
// Strict covers unmatched content only. An absent element is never refused,
// mandatory or not, and neither is a boundary flag an interval leaves out.
// An unknown `xsi:type` fails in every mode (see [DecodeError]). The decode
// reads the input as a stream, so strict returns the first problem in
// document order: an unmatched item before an unknown `xsi:type` refuses the
// decode with [ErrUnknownElement], and an unknown `xsi:type` first fails as
// itself.
//
// By default the decoder ignores unmatched content.
func WithStrict() DecoderOption {
	return func(c *decoderConfig) { c.strict = true }
}

// WithRelaxedTypeDispatch toggles the polymorphic-dispatch policy
// for abstract slots from strict (the default: missing `xsi:type` at a
// polymorphic site is an error) to relaxed (missing `xsi:type` is
// allowed when the declared abstract field has exactly one concrete
// descendant in the merged BMM; the decoder then instantiates that
// descendant). It applies to abstract slots only, i.e. slot types like
// `DATA_VALUE`, `DV_ORDERED`, `ITEM_STRUCTURE`, `PARTY_PROXY`.
//
// Narrow-interface slots (`<Parent>Like`: DVTextLike,
// PartyIdentifiedLike, …) have an independent, always-on fallback:
// a missing `xsi:type` defaults to the declared parent's concrete
// type, served by [DecodeAsOrDefault] from the generated code.
// That fallback is deterministic (the parent type is fixed by the
// BMM), so this option does not control it.
//
// The option is accepted for abstract slots but not yet enforced: the
// generated [UnmarshalXML] methods at abstract slots implement strict
// dispatch only, so setting it is currently a no-op for those slots. It
// exists so the API does not change when relaxed dispatch is implemented.
func WithRelaxedTypeDispatch(enabled bool) DecoderOption {
	return func(c *decoderConfig) { c.relaxedTypeDispatch = enabled }
}

// Decoder reads and decodes canonical-XML values from a stream.
// Wrapping `encoding/xml.Decoder` keeps the swap path cheap.
type Decoder struct {
	dec *xml.Decoder
	cfg decoderConfig
	// trk is the decoder's tracker when an option reports or refuses
	// unmatched content, and nil otherwise.
	trk *xmlunmatched.Tracker
}

// NewDecoder returns a [Decoder] that reads canonical-XML from r. The
// options apply to every value it decodes; a path that [WithUnmatched] or
// [WithStrict] reports is relative to the value being decoded.
func NewDecoder(r io.Reader, opts ...DecoderOption) *Decoder {
	cfg := newDecoderConfig(opts)
	if !cfg.tracked() {
		return &Decoder{dec: xml.NewDecoder(r), cfg: cfg}
	}
	t := xmlunmatched.NewTracker(cfg.unmatched, cfg.strict)
	return &Decoder{dec: t.NewDecoder(r), cfg: cfg, trk: t}
}

// Decode reads the next XML element from the stream and stores it
// in v. Errors follow the same classification as [Unmarshal].
//
// Decode reads no further than the end of that element, so whatever
// follows it, another element included, is left for the next call. A
// stream may hold many values, one after another. [Unmarshal], which is
// given a whole document, is the call that holds it to one root element.
func (d *Decoder) Decode(v any) error {
	if d.trk == nil {
		return d.dec.Decode(v)
	}
	return d.trk.Decode(d.dec, v)
}

// RelaxedTypeDispatch reports whether the decoder was configured
// with the relaxed dispatch policy. Used by generated [UnmarshalXML]
// methods once they support the relaxed path (currently
// informational only).
func (d *Decoder) RelaxedTypeDispatch() bool { return d.cfg.relaxedTypeDispatch }

// Unmarshal parses canonical-XML-encoded data and stores the result
// in the value pointed to by v. v must be a non-nil pointer to a
// generated RM type whose UnmarshalXML method is wired through this
// package (every concrete RM class generated under openehr/rm/
// implements it).
//
// Polymorphic fields on v are populated via the per-type
// [UnmarshalXML] methods the BMM generator emits; each consults
// [typereg.Default] to resolve `xsi:type` discriminators.
//
// data is one document, so it holds one root element. Comments,
// processing instructions and whitespace may follow the root, as XML
// allows. Another element, or text other than whitespace, after the root
// is not well-formed XML, and the decode refuses it in every mode, with
// or without [WithUnmatched] or [WithStrict]: it is malformed input, not
// unmatched content. The error wraps [ErrInvalidShape] and carries none
// of the content after the root. A root that fails to decode fails as
// itself, and the decode does not read on.
//
// Returns [DecodeError] wrapping a typereg sentinel
// ([typereg.ErrMissingType] / ErrUnknownType / ErrTypeMismatch) at
// polymorphic failures, and [ErrInvalidShape] (wrapped) for an
// `xmi:type` discriminator failure and for content after the root.
// Other XML syntax errors are not classified: they come back unchanged
// from encoding/xml, so an errors.Is against [ErrInvalidShape] does not
// match a truncated root or a syntax error inside it.
//
// Content the decode does not read is ignored by default. [WithUnmatched]
// reports each such item and [WithStrict] refuses the first, wrapping
// [ErrUnknownElement].
func Unmarshal(data []byte, v any, opts ...DecoderOption) error {
	d := NewDecoder(bytes.NewReader(data), opts...)
	if err := d.Decode(v); err != nil {
		return err
	}
	return d.end()
}

// end reads the rest of the document after the root element. Comments,
// processing instructions and whitespace pass. Anything else, an element,
// other text, a document type declaration or a token encoding/xml cannot
// read, is refused: the error wraps [ErrInvalidShape] and names none of
// it.
func (d *Decoder) end() error {
	for {
		tok, err := d.dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return errAfterRoot()
		}
		switch t := tok.(type) {
		case xml.Comment, xml.ProcInst:
			continue
		case xml.CharData:
			if isXMLSpace(t) {
				continue
			}
		}
		return errAfterRoot()
	}
}

// errAfterRoot is the refusal of content after the root element.
func errAfterRoot() error {
	return fmt.Errorf("%w: content after the root element", ErrInvalidShape)
}

// isXMLSpace reports whether text holds XML whitespace only: spaces, tabs,
// carriage returns and line feeds.
func isXMLSpace(text []byte) bool {
	return len(bytes.Trim(text, " \t\r\n")) == 0
}

// XSITypeOf scans a start element's attribute list and returns the
// value of the `xsi:type` discriminator. It accepts the
// namespace-resolved form (`Space == NSXSI, Local == "type"`), which
// encoding/xml produces when `xmlns:xsi` is in scope, and the
// literal form (`Local == "xsi:type"`) produced by directly
// constructed tokens, e.g. the encoder's own [XSITypeAttrName].
// An `xsi:type` written with no in-scope `xmlns:xsi` does not
// take the literal branch: encoding/xml yields `Space == "xsi",
// Local == "type"`, which matches neither branch and is reported as a
// missing discriminator (below). A bare `type` attribute, in no
// namespace, is read as the discriminator too, but only when the element
// carries no `xsi:type` in either form above: an `xsi:type` wins
// wherever it is written.
//
// The returned discriminator is normalised: a leading `xsd:` (XML
// Schema datatype) prefix is stripped so foundation primitives like
// `xsd:string` decode against the BMM primitive name `String`. A
// namespace-prefixed RM value (e.g. Better's `ns2:DV_QUANTITY`) is
// not stripped and so fails registry lookup as an unknown type;
// namespace-prefixed discriminators are not supported.
//
// Returns [ErrInvalidShape] when an `xmi:type` attribute is
// encountered: ITS-XML specifies `xsi:type` and the SDK rejects XMI
// discriminators on the wire.
//
// Returns ("", nil) when the element carries no discriminator at
// all; the caller decides whether that is an error (strict) or
// acceptable (relaxed dispatch).
func XSITypeOf(start xml.StartElement) (string, error) {
	_, typeName, err := discriminator(start)
	return typeName, err
}

// discriminator returns the attribute of start that [XSITypeOf] reads as the
// `xsi:type` discriminator, by name, with its normalised value. An `xsi:type`
// in the XSI namespace or written literally wins; a bare `type` is read only
// when there is none, so a bare `type` beside an `xsi:type` stays unmatched
// content. The name lets dispatch record which attribute it read.
func discriminator(start xml.StartElement) (xml.Name, string, error) {
	bare := -1
	for i, attr := range start.Attr {
		// Reject xmi:type (both resolved and literal forms).
		if attr.Name.Local == "type" && attr.Name.Space == NSXMI {
			return xml.Name{}, "", fmt.Errorf("canxml: %w: xmi:type on the wire is rejected (use xsi:type)", ErrInvalidShape)
		}
		if attr.Name.Local == "xmi:type" {
			return xml.Name{}, "", fmt.Errorf("canxml: %w: xmi:type on the wire is rejected (use xsi:type)", ErrInvalidShape)
		}
		if !xmlunmatched.IsDiscriminator(attr.Name) {
			continue
		}
		// Accept xsi:type (resolved and literal forms) at once; keep a bare
		// type for when there is none.
		if attr.Name.Space != "" || attr.Name.Local != "type" {
			return attr.Name, stripXSDPrefix(attr.Value), nil
		}
		if bare < 0 {
			bare = i
		}
	}
	if bare >= 0 {
		return start.Attr[bare].Name, stripXSDPrefix(start.Attr[bare].Value), nil
	}
	return xml.Name{}, "", nil
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

// DecodeAs reads one polymorphic child element from dec (the
// element whose start token is `start`) and returns the typereg-
// dispatched concrete value, asserted to T. Generated
// UnmarshalXML methods call it at every polymorphic-field decode site.
//
// Behaviour:
//
//   - If start carries a discriminator, an `xsi:type` or failing that a
//     bare `type` (see [XSITypeOf]), it is looked up in [typereg.Default];
//     the matching constructor produces a fresh concrete instance, and the
//     element body decodes into it as the decode's options say (see
//     [WithUnmatched]).
//   - If start carries neither, returns [typereg.ErrMissingType] wrapped
//     in [DecodeError], always: relaxed dispatch
//     ([WithRelaxedTypeDispatch]) is not implemented yet.
//   - If the concrete value does not satisfy T, returns
//     [typereg.ErrTypeMismatch] wrapped in [DecodeError].
//   - The body bytes are consumed up to and including the matching
//     end element so the parent decoder is positioned correctly for
//     the next child.
func DecodeAs[T any](dec *xml.Decoder, start xml.StartElement) (T, error) {
	var zero T
	attrName, typeName, err := discriminator(start)
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
	xmlunmatched.DiscriminatorRead(dec, attrName)
	v := ctor()
	if err := xmlunmatched.DecodeElement(dec, &start, v); err != nil {
		return zero, enclose("/"+start.Name.Local, typeName, err)
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

// DecodeAsOrDefault is the XML counterpart of [DecodeAs] for
// narrow-interface (`<Parent>Like`) slots. When the element carries a
// discriminator, an `xsi:type` or failing that a bare `type` (see
// [XSITypeOf]), dispatch goes through [typereg.Default] exactly like
// DecodeAs. When it carries neither, the decode does not fail: the
// supplied defaultCtor instantiates the declared parent type, and the
// element body decodes into it as the decode's options say (see
// [WithUnmatched]). This preserves openEHR canonical XML where the static
// field type fixes the concrete subtype.
func DecodeAsOrDefault[T any](dec *xml.Decoder, start xml.StartElement, defaultCtor func() any) (T, error) {
	var zero T
	attrName, typeName, err := discriminator(start)
	if err != nil {
		return zero, &DecodeError{Path: "/" + start.Name.Local, Inner: err}
	}
	var v any
	if typeName == "" {
		if defaultCtor == nil {
			return zero, &DecodeError{Path: "/" + start.Name.Local, Inner: fmt.Errorf("canxml: %w", typereg.ErrMissingType)}
		}
		v = defaultCtor()
		if err := xmlunmatched.DecodeElement(dec, &start, v); err != nil {
			return zero, enclose("/"+start.Name.Local, "", err)
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
		xmlunmatched.DiscriminatorRead(dec, attrName)
		v = ctor()
		if err := xmlunmatched.DecodeElement(dec, &start, v); err != nil {
			return zero, enclose("/"+start.Name.Local, typeName, err)
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

// enclose returns the [DecodeError] for err, a failure while the element at
// path decoded as typeName (empty when no discriminator chose the type).
//
// A failure that is itself a DecodeError comes from a polymorphic slot
// further in. It becomes the Inner of the new one as it is, so a failure deep
// inside nested slots is a chain of envelopes, one per slot, whose message is
// written once, when it is asked for. Wrapping it in a formatted error instead
// would write out the whole message below at every slot on the way out, at a
// cost that grows with the square of the depth. Any other failure keeps the
// canxml prefix.
func enclose(path, typeName string, err error) *DecodeError {
	if de, ok := err.(*DecodeError); ok && de != nil { //nolint:errorlint // a direct envelope only: a wrapped one keeps its wrapper, prefixed below
		return &DecodeError{Path: path, Type: typeName, Inner: de}
	}
	return &DecodeError{Path: path, Type: typeName, Inner: fmt.Errorf("canxml: %w", err)}
}
