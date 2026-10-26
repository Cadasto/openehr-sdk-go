package canxml

import (
	"encoding/xml"
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

// tracker returns the tracker of a decode with c's options, and nil for a
// decode that needs none, which reads as encoding/xml does.
func (c decoderConfig) tracker() *xmlunmatched.Tracker {
	if !c.tracked() {
		return nil
	}
	return xmlunmatched.NewTracker(c.unmatched, c.strict)
}

// WithUnmatched passes every unmatched item of the input to fn while the
// decode carries on. An item is unmatched when the decode does not read it:
// an element or attribute that names no attribute of the RM type its element
// decodes as, other than namespace declarations and attributes in the XML
// Schema instance namespace (`xsi:type`, `xsi:schemaLocation`, `xsi:nil`,
// …); an element in a namespace other than the openEHR one (see
// [ErrNamespace]), the root element aside; text other than XML whitespace
// directly inside an element whose content is elements, such as an RM
// object's, a CDATA section included; and an element whose content this
// codec does not model: a `Hash` member, a BMM `Any` member (the AOM 1.4
// `assumed_value` and `EXPR_LEAF.item`), and a member whose Go type has no
// decoder of its own (see the package documentation). A bare `type`
// attribute is unmatched except where dispatch reads it as the
// discriminator. It applies at any depth: inside a polymorphic slot, an
// interval bound, a primitive element or an AOM 1.4 value too. The text of
// a primitive element, such as `magnitude`, is its value and is read, and
// XML whitespace (spaces, tabs, carriage returns and line feeds) between
// elements is never unmatched, so an indented document decodes as a compact
// one does.
//
// By default the decoder ignores unmatched content: the value it returns
// holds the rest of the input and none of it. With this option fn receives
// each item, once, in the order it appears in the input, and the value
// returned is the same as without the option. An unmatched element is one
// item, whatever it holds, and the text of an element is one item, however
// many runs of text it holds.
//
// fn gets the item's path from the root of the value: one `/name[n]` segment
// per element, where name is the element's local name and n its position
// among the earlier siblings of the same name plus one, counting from 1; an
// attribute adds a last `/@name` segment, and text a last `/text()` segment.
// For example "/composition[1]/content[2]/x-note[1]",
// "/composition[1]/@x-flag" or "/ehr_status[1]/subject[1]/text()". The path
// carries names only, never a value from the input. fn runs during the
// decode, on the caller's goroutine.
//
// A nil fn is the same as leaving the option out. Combined with
// [WithStrict], fn sees the first unmatched item before the decode is
// refused.
func WithUnmatched(fn func(path string)) DecoderOption {
	return func(c *decoderConfig) { c.unmatched = fn }
}

// WithStrict refuses the decode at its first unmatched item (see
// [WithUnmatched] for what is unmatched, text in an element whose content is
// elements among it, and for the form of its path). The error wraps
// [ErrUnknownElement] and names the item's path from the root of the value;
// it carries none of the item's value or text. An element in a foreign
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
	t := cfg.tracker()
	if t == nil {
		return &Decoder{dec: xml.NewDecoder(r), cfg: cfg}
	}
	return &Decoder{dec: t.NewDecoder(r), cfg: cfg, trk: t}
}

// Decode reads the next XML element from the stream and stores it
// in v. Errors follow the same classification as [Unmarshal], with one
// difference: a Decoder given neither [WithUnmatched] nor [WithStrict] reads
// the stream once and does not follow the path of the elements it reads, so
// the [DecodeError] for the text of a primitive element that does not decode
// as its type names the element alone, such as "/is_queryable", inside the
// envelope of each polymorphic slot it sits in. A Decoder given either
// option returns it on its own, with the element's path from the root of the
// value, as [Unmarshal] does.
//
// Decode reads no further than the end of that element, so whatever
// follows it, another element included, is left for the next call. A
// stream may hold many values, one after another, and Decode skips what
// precedes each one, as encoding/xml does. [Unmarshal], which is given a
// whole document, is the call that holds it to one root element and checks
// what comes before and after it.
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
// data is one document, so it holds one root element. Before the root it
// may hold the XML declaration, comments, processing instructions,
// whitespace and one document type declaration, after a UTF-8 byte order
// mark at the very start; comments, processing instructions and whitespace
// may follow the root, as XML allows. Anything else before or after the
// root, such as an element or text other than whitespace, is not
// well-formed XML, and the decode refuses it in every mode, with or without
// [WithUnmatched] or [WithStrict]: it is malformed input, not unmatched
// content. The error wraps [ErrInvalidShape] and carries none of that
// content. A root that fails to decode fails as itself, and the decode does
// not read on.
//
// Returns [DecodeError] wrapping a typereg sentinel
// ([typereg.ErrMissingType] / ErrUnknownType / ErrTypeMismatch) at
// polymorphic failures, and [ErrInvalidShape] (wrapped) for an
// `xmi:type` discriminator failure and for content before or after the
// root.
// Other XML syntax errors are not classified: they come back unchanged
// from encoding/xml, so an errors.Is against [ErrInvalidShape] does not
// match a truncated root or a syntax error inside it.
//
// The text of a primitive element that does not decode as its type, such
// as a boolean written `maybe` or a whole number written `2.5`, fails in
// every mode with a [DecodeError] on its own, outside any slot envelope,
// whose Path is the element's path from the root of the value, such as
// "/ehr_status[1]/is_queryable[1]", in the form [WithUnmatched] uses. It
// wraps [ErrInvalidShape] and keeps the conversion's own error, such as a
// *strconv.NumError, reachable through unwrapping; its text carries none of
// the element's. Given no option, Unmarshal finds that path by decoding
// data a second time once the first decode has failed so, which a decode
// that succeeds never does.
//
// Content the decode does not read is ignored by default. [WithUnmatched]
// reports each such item and [WithStrict] refuses the first, wrapping
// [ErrUnknownElement].
func Unmarshal(data []byte, v any, opts ...DecoderOption) error {
	return xmlunmatched.Unmarshal(data, v, newDecoderConfig(opts).tracker())
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
