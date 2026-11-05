package canxml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"slices"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/decodeopts"
	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlschema"
	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlunmatched"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// NSXMI is the XMI namespace. ITS-XML rejects `xmi:type` on the
// wire; only `xsi:type` is recognised. The decoder maps any
// `xmi:type` it sees to [ErrInvalidShape].
const NSXMI = "http://www.omg.org/XMI"

// DecoderOption configures a decode, through [Unmarshal] or a [Decoder].
// [WithUnmatched] and [WithStrict] say what happens to content the decode
// does not read; [WithMissing] reports the members the input leaves out, and
// [WithPresence] those it sends without a value; [WithMaxDepth] refuses an
// element nested too deep.
// [WithRelaxedTypeDispatch] names relaxed polymorphic dispatch; it is
// accepted but not yet enforced, so dispatch stays strict.
type DecoderOption func(*decoderConfig)

// decoderConfig holds the option-driven state of a decode. Kept unexported so
// the option list can grow without churning the [Decoder] type.
type decoderConfig struct {
	unmatched           func(path string)
	missing             func(pointer string)
	presence            func(pointer string, p Presence)
	maxDepth            int // 0 for no limit
	relaxedTypeDispatch bool
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

// init registers with decodeopts the function that tells the operational
// template reader of openehr/aom/aom14 three things a []DecoderOption asks
// for: whether it is strict, which turns off the reader's weaker readings,
// whether it reports to a function, which the reader then calls from one
// decode only, and the depth limit it sets, which the reader applies to what
// its weaker readings leave out as well.
func init() {
	decodeopts.Register(func(opts any) decodeopts.Settings {
		list, _ := opts.([]DecoderOption)
		c := newDecoderConfig(list)
		return decodeopts.Settings{Strict: c.strict, Reports: c.unmatched != nil || c.missing != nil || c.presence != nil, MaxDepth: c.maxDepth}
	})
}

// tracked reports whether the decode reports or refuses unmatched content,
// or reports missing members or members sent without a value: each needs the
// decode to follow the elements it reads.
func (c decoderConfig) tracked() bool {
	return c.unmatched != nil || c.strict || c.missing != nil || c.presence != nil
}

// tracker returns the tracker of a decode with c's options, and nil for a
// decode that needs none, which reads as encoding/xml does.
func (c decoderConfig) tracker() *xmlunmatched.Tracker {
	if !c.tracked() {
		return nil
	}
	t := xmlunmatched.NewTracker(c.unmatched, c.strict)
	t.ReportMissing(c.missing)
	t.ReportPresence(reportPresence(c.presence))
	t.LimitDepth(c.maxDepth)
	return t
}

// documentTracker returns the tracker [Unmarshal] decodes a document through:
// c's tracker, or, under a depth limit alone, one that applies no policy and
// follows the elements for the limit. Such a tracker decodes a document as a
// decode with no tracker does, in one pass: that decode names a primitive
// failure by its path, and resolves a prefix declared around its attribute,
// by decoding the document again through one.
func (c decoderConfig) documentTracker() *xmlunmatched.Tracker {
	if t := c.tracker(); t != nil || c.maxDepth == 0 {
		return t
	}
	t := xmlunmatched.NewTracker(nil, false)
	t.LimitDepth(c.maxDepth)
	return t
}

// WithUnmatched passes every unmatched item of the input to fn while the
// decode carries on. An item is unmatched when the decode does not read it:
// an element or attribute that names no attribute of the RM type its element
// decodes as, other than namespace declarations and attributes in the XML
// Schema instance namespace (`xsi:type`, `xsi:schemaLocation`, `xsi:nil`,
// …); an element in a namespace other than the openEHR one (see
// [ErrNamespace]), the root element aside; text other than XML whitespace
// directly inside an element whose content is elements, such as an RM
// object's, a CDATA section included; a child element that repeats an
// earlier child of the same name under a single-valued attribute, such as a
// second `language` in a COMPOSITION, whose first child gives the member its
// value in every mode; and an element whose content this codec does not
// model: a `Hash` whose values are `Hash`es (RESOURCE_ANNOTATIONS
// `documentation`) and a member whose Go type has no decoder of its own (see
// the package documentation). A bare `type`
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
// mandatory or not ([WithMissing] reports a mandatory one), and neither is
// a boundary flag an interval leaves out.
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

// WithMaxDepth refuses an element nested deeper than n, counted from the root
// of the value, which is at depth 1: the document's root element for
// [Unmarshal], the root element of each value a [Decoder] decodes. The decode
// refuses the element when it reads its start tag, before anything decodes
// the element, in every mode, with [WithUnmatched], [WithStrict] or
// [WithMissing] or without. Use it on input from outside the caller's
// control, whose nesting would otherwise cost the decode memory and time in
// proportion to its depth.
//
// The refusal is a [DecodeError] on its own, outside any slot envelope, that
// wraps [ErrLimitExceeded]. Its Path is the element's path from the root of
// the value, in the form [WithUnmatched] uses, with no more than eight
// segments at each end: a longer path has a `/...` segment in place of those
// between. Its text names the element's depth and n, and carries none of the
// element's content. The value v points to is unspecified after the refusal.
// A [Decoder] that refuses an element returns the same refusal from every
// later Decode, since its stream stands inside that element.
//
// n <= 0 sets no limit, as leaving the option out does, and does not lift a
// limit another WithMaxDepth gave. Given more than one, the smallest positive
// n holds, in whatever order they come. Following the elements costs a decode
// some time, as [WithUnmatched] does; a decode without a limit pays nothing
// for it.
//
// No n lifts the depth encoding/xml decodes, 10 000 nested elements: with n
// above it, or with no limit, the decode refuses an element nested deeper
// than that all the same, with a refusal of the same form that names 10 000
// as the limit (see "Nesting depth" in the package documentation).
func WithMaxDepth(n int) DecoderOption {
	return func(c *decoderConfig) {
		if n > 0 && (c.maxDepth == 0 || n < c.maxDepth) {
			c.maxDepth = n
		}
	}
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
	// unmatched content or reports missing members, and nil otherwise.
	trk *xmlunmatched.Tracker
	// rec records what dec has read of the stream since the value being
	// decoded began, for a decoder given none of those options that reads
	// through a plain encoding/xml decoder from a source it cannot seek; nil
	// otherwise.
	rec *recorder
	// seeker is the source of a decoder given none of those options when it
	// is an io.ByteReader and an io.Seeker, which dec reads directly and the
	// decoder seeks back in to decode a value again; nil otherwise.
	seeker io.ReadSeeker
	// scope keeps the namespace declarations in scope, and follows the
	// elements, for a decoder given none of those options once it has decoded
	// a value a second time, to resolve a prefix or to name an element nested
	// past encoding/xml's depth limit: it reads the rest of the stream through
	// scope. nil until then. For a decoder given none of them but
	// [WithMaxDepth], scope is there from the start.
	scope *xmlunmatched.Scope
}

// NewDecoder returns a [Decoder] that reads canonical-XML from r. The
// options apply to every value it decodes; a path that [WithUnmatched] or
// [WithStrict] reports, and a pointer that [WithMissing] reports, is
// relative to the value being decoded.
//
// When r is an io.ByteReader, such as a *bufio.Reader, the Decoder reads it a
// byte at a time and takes from it no byte past the end of the value it
// decodes, whatever its options, so what follows the value stays in r. Any
// other r it reads ahead, as encoding/xml does. When r is also an io.Seeker,
// such as a *bytes.Reader, a Decoder given no option seeks r back to decode a
// value a second time (see [Decoder.Decode]).
func NewDecoder(r io.Reader, opts ...DecoderOption) *Decoder {
	cfg := newDecoderConfig(opts)
	if t := cfg.tracker(); t != nil {
		return &Decoder{dec: t.NewDecoder(r), cfg: cfg, trk: t}
	}
	if cfg.maxDepth > 0 {
		// The limit needs the decode to follow the elements it reads. A scope
		// does so and applies no policy, so a value decodes in one pass, and a
		// primitive failure is named as a decoder given no option names it.
		s := new(xmlunmatched.Scope)
		s.LimitDepth(cfg.maxDepth)
		return &Decoder{dec: s.NewDecoder(r), cfg: cfg, scope: s}
	}
	if s, ok := r.(byteSeeker); ok {
		return &Decoder{dec: xml.NewDecoder(s), cfg: cfg, seeker: s}
	}
	rec := &recorder{src: r}
	rec.bsrc, _ = r.(io.ByteReader)
	return &Decoder{dec: xml.NewDecoder(rec), cfg: cfg, rec: rec}
}

// byteSeeker is a source a [Decoder] given no option reads directly, as
// encoding/xml reads an io.ByteReader, and seeks back in to decode a value
// again, so that it keeps none of the bytes it reads.
type byteSeeker interface {
	io.ByteReader
	io.ReadSeeker
}

// recorder is the reader a [Decoder] given no option reads its stream
// through when the stream is no [byteSeeker]. It keeps the bytes read from
// src since the value being decoded began, so that the value can be read
// again. encoding/xml reads it a byte at a time, as an io.ByteReader, so it
// buffers nothing else and the bytes it keeps are the only copy of them. A
// src that is an io.ByteReader is read a byte at a time too, so the recorder
// takes from it no byte past the last one encoding/xml asks for; any other
// src is read ahead, in blocks.
type recorder struct {
	src  io.Reader
	bsrc io.ByteReader // src, when it is an io.ByteReader; nil otherwise
	buf  []byte        // the bytes read from src from offset base on
	pos  int           // the index in buf of the next byte to return
	base int64         // the offset in the stream of buf[0]
	err  error         // the error src returned, once it has returned one
}

// minRead is the least room a recorder makes for one read of its source.
const minRead = 4096

// ReadByte returns the next byte of the stream.
func (r *recorder) ReadByte() (byte, error) {
	if r.pos == len(r.buf) {
		if r.bsrc != nil {
			c, err := r.takeByte()
			if err != nil {
				return 0, err
			}
			r.pos++
			return c, nil
		}
		if err := r.fill(); err != nil {
			return 0, err
		}
	}
	c := r.buf[r.pos]
	r.pos++
	return c, nil
}

// Read reads the next bytes of the stream into p. It makes a recorder the
// io.Reader encoding/xml takes, which reads it through ReadByte.
func (r *recorder) Read(p []byte) (int, error) {
	if r.pos == len(r.buf) {
		if err := r.fill(); err != nil {
			return 0, err
		}
	}
	n := copy(p, r.buf[r.pos:])
	r.pos += n
	return n, nil
}

// fill reads src into the room after the bytes buf holds until it reads one
// or more, making room first when there is none, and returns the error src
// gave, which every later call returns too. A src that is an io.ByteReader
// gives one byte.
func (r *recorder) fill() error {
	if r.bsrc != nil {
		_, err := r.takeByte()
		return err
	}
	for range 100 {
		if r.err != nil {
			return r.err
		}
		if len(r.buf) == cap(r.buf) {
			r.buf = slices.Grow(r.buf, max(len(r.buf), minRead))
		}
		n, err := r.src.Read(r.buf[len(r.buf):cap(r.buf)])
		r.buf = r.buf[:len(r.buf)+n]
		r.err = err
		if n > 0 {
			return nil
		}
	}
	r.err = io.ErrNoProgress
	return r.err
}

// takeByte reads the next byte of bsrc, appends it to buf, making room first
// as fill does when there is none, and returns it; or it returns the error
// bsrc gave, which every later call returns too.
func (r *recorder) takeByte() (byte, error) {
	if r.err != nil {
		return 0, r.err
	}
	c, err := r.bsrc.ReadByte()
	if err != nil {
		r.err = err
		return 0, err
	}
	if len(r.buf) == cap(r.buf) {
		r.buf = slices.Grow(r.buf, max(len(r.buf), minRead))
	}
	r.buf = append(r.buf, c)
	return c, nil
}

// from drops the bytes before off, an offset in the stream no later than the
// next byte to return, which a value decoded from off on does not need.
func (r *recorder) from(off int64) {
	n := int(off - r.base)
	r.buf = r.buf[:copy(r.buf, r.buf[n:])]
	r.pos -= n
	r.base = off
}

// replay returns a reader of the stream from where the record starts: the
// bytes recorded, then the rest of src, or the error src gave. For a src that
// is an io.ByteReader it is an io.ByteReader too, so a decoder reading it
// puts no buffer of its own in front of it and takes from src no byte past
// the last one it asks for. Such a src has given no error when a value is
// replayed: the decode stops at the prefix it cannot resolve, in a start
// element it has read to its end and no further.
func (r *recorder) replay() io.Reader {
	if r.bsrc != nil {
		return &byteReplay{rec: r.buf, src: r.bsrc}
	}
	if r.err != nil {
		return io.MultiReader(bytes.NewReader(r.buf), failedReader{r.err})
	}
	return io.MultiReader(bytes.NewReader(r.buf), r.src)
}

// byteReplay is the replay of a recorder whose source is an io.ByteReader:
// the bytes recorded, then the rest of the source a byte at a time.
type byteReplay struct {
	rec []byte        // the bytes recorded that are still to be read
	src io.ByteReader // the recorder's source
}

// ReadByte returns the next byte of the stream.
func (r *byteReplay) ReadByte() (byte, error) {
	if len(r.rec) > 0 {
		c := r.rec[0]
		r.rec = r.rec[1:]
		return c, nil
	}
	return r.src.ReadByte()
}

// Read reads the next byte of the stream into p. It makes a byteReplay the
// io.Reader encoding/xml takes, which reads it through ReadByte.
func (r *byteReplay) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	c, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	p[0] = c
	return 1, nil
}

// failedReader is a reader that returns err.
type failedReader struct{ err error }

func (f failedReader) Read([]byte) (int, error) { return 0, f.err }

// Decode reads the next XML element from the stream and stores it
// in v. Errors follow the same classification as [Unmarshal], with one
// difference: a Decoder given none of [WithUnmatched], [WithStrict] and
// [WithMissing] does not name the path of the elements it reads, so the
// [DecodeError] for the text of a primitive element that does not decode as
// its type names the element alone, such as "/is_queryable", inside the
// envelope of each polymorphic slot it sits in. A Decoder given any of them
// returns it on its own, with the element's path from the root of the value,
// as [Unmarshal] does. [WithMaxDepth] does not change that.
//
// A prefixed `xsi:type` is resolved against the namespace declarations in
// scope where the attribute stands (see [XSITypeOf]). A Decoder given any of
// those options, or [WithMaxDepth], keeps every declaration in scope as it
// reads. One given none reads an element's own declarations as it goes; when
// a prefix is declared on an element around the attribute, Decode decodes the
// value a second time, from where the call began, through a reader that keeps
// every declaration in scope, into v cleared to its zero value. The Decoder
// then reads the rest of the stream through that reader. It does the same
// when the value nests deeper than encoding/xml decodes, 10 000 elements, to
// name the element refused by its path (see [WithMaxDepth]). To read the value
// again, a Decoder whose reader is an io.ByteReader and an io.Seeker, such as
// a *bytes.Reader, seeks the reader back to where the value began, and a seek
// that fails is the error Decode returns; a Decoder over any other reader
// keeps the bytes of the value it is decoding instead. So only a value that
// needs the second decode pays for it, and over a reader that cannot seek any
// other pays for keeping its bytes.
//
// Decode reads no further than the end of that element, so whatever
// follows it, another element included, is left for the next call. A
// stream may hold many values, one after another, and Decode skips what
// precedes each one, as encoding/xml does. [Unmarshal], which is given a
// whole document, is the call that holds it to one root element and checks
// what comes before and after it.
func (d *Decoder) Decode(v any) error {
	switch {
	case d.trk != nil:
		return d.trk.Decode(d.dec, v)
	case d.scope != nil:
		return d.scope.Decode(d.dec, v)
	}
	start := d.dec.InputOffset()
	if d.rec != nil {
		d.rec.from(start)
	}
	err := d.dec.Decode(v)
	if !xmlunmatched.IsUnresolvedPrefix(err) && !xmlunmatched.IsXMLDepthError(err) {
		return err
	}
	src, err := d.rewind(start)
	if err != nil {
		return err
	}
	d.scope = new(xmlunmatched.Scope)
	d.dec = d.scope.NewDecoder(src)
	d.rec, d.seeker = nil, nil
	return d.scope.DecodeAgain(d.dec, v)
}

// rewind returns a reader of the stream from start, the input offset of d.dec
// where the value being decoded began: the seeker sought back by the bytes
// d.dec has taken since start, or else the recorder's replay. A decode stops
// at a prefix it cannot resolve at the end of a start element, where
// encoding/xml holds back no byte it has read, so the seeker stands that many
// bytes past where the value began.
func (d *Decoder) rewind(start int64) (io.Reader, error) {
	if d.seeker == nil {
		return d.rec.replay(), nil
	}
	if _, err := d.seeker.Seek(start-d.dec.InputOffset(), io.SeekCurrent); err != nil {
		return nil, fmt.Errorf("canxml: seek back to the value to decode it again: %w", err)
	}
	return d.seeker, nil
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
// may follow the root, as XML allows. XML reserves the processing
// instruction target xml, in any letter case, for the XML declaration at the
// start, so one elsewhere before or after the root, such as a second
// declaration or one after the root's end, is refused; a target that only starts with those letters, such as
// xml-stylesheet, passes. Anything else before or after the root, such as
// an element or text other than whitespace, is not well-formed XML, and the
// decode refuses it in every mode, with or without [WithUnmatched],
// [WithStrict] or [WithMissing]: it is malformed input, not unmatched
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
// A prefixed `xsi:type` is resolved against the namespace declarations in
// scope where the attribute stands (see [XSITypeOf]). Given no option,
// Unmarshal reads an element's own declarations as it goes; when the prefix
// is declared on an element around it, Unmarshal decodes data a second time,
// through a decoder that keeps every declaration in scope, into v cleared to
// its zero value, so only a document with such a value pays for the second
// decode.
//
// Content the decode does not read is ignored by default. [WithUnmatched]
// reports each such item and [WithStrict] refuses the first, wrapping
// [ErrUnknownElement]. A mandatory member the input leaves out is decoded
// as its zero value by default, and [WithMissing] reports each one, by its
// JSON Pointer in the canonical-JSON form of the instance; no mode refuses
// one. [WithMaxDepth] refuses an element nested deeper than its limit, and
// every decode refuses one nested deeper than encoding/xml decodes, 10 000
// elements, the same way; given no option, Unmarshal finds the path of such
// an element by decoding data a second time.
func Unmarshal(data []byte, v any, opts ...DecoderOption) error {
	return xmlunmatched.Unmarshal(data, v, newDecoderConfig(opts).documentTracker())
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
// The returned discriminator is an openEHR class name, matched as written,
// with no prefix. A value whose prefix is bound to the XML Schema namespace
// `http://www.w3.org/2001/XMLSchema` names an XML Schema built-in datatype
// and is returned as the BMM primitive it reads as, whatever the prefix, so
// `xs:string` and `xsd:string` both return `String`; so is a value with the
// literal prefix `xsd:` that no declaration binds. Any other prefixed value,
// such as Better's `ns2:DV_QUANTITY`, is returned as written and so fails
// registry lookup as an unknown type: namespace-prefixed class names are not
// supported. XSITypeOf sees only the namespace declarations start carries
// itself; the decoder resolves a prefix against every declaration in scope
// where the attribute stands.
//
// Returns [ErrInvalidShape] when an `xmi:type` attribute is
// encountered: ITS-XML specifies `xsi:type` and the SDK rejects XMI
// discriminators on the wire.
//
// Returns ("", nil) when the element carries no discriminator at
// all; the caller decides whether that is an error (strict) or
// acceptable (relaxed dispatch).
func XSITypeOf(start xml.StartElement) (string, error) {
	d, err := xmlschema.Discriminator(nil, start)
	return d.Name, err
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
	d, err := xmlschema.Discriminator(dec, start)
	if err != nil {
		return zero, &DecodeError{Path: "/" + start.Name.Local, Inner: err}
	}
	attrName, typeName := d.Attr, d.Name
	if typeName == "" {
		return zero, &DecodeError{Path: "/" + start.Name.Local, Inner: fmt.Errorf("canxml: %w", typereg.ErrMissingType)}
	}
	ctor, ok := xmlschema.LookupType(typeName)
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
		return zero, xmlschema.Enclose("/"+start.Name.Local, typeName, err)
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
	d, err := xmlschema.Discriminator(dec, start)
	if err != nil {
		return zero, &DecodeError{Path: "/" + start.Name.Local, Inner: err}
	}
	attrName, typeName := d.Attr, d.Name
	var v any
	if typeName == "" {
		if defaultCtor == nil {
			return zero, &DecodeError{Path: "/" + start.Name.Local, Inner: fmt.Errorf("canxml: %w", typereg.ErrMissingType)}
		}
		v = defaultCtor()
		if err := xmlunmatched.DecodeElement(dec, &start, v); err != nil {
			return zero, xmlschema.Enclose("/"+start.Name.Local, "", err)
		}
	} else {
		ctor, ok := xmlschema.LookupType(typeName)
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
			return zero, xmlschema.Enclose("/"+start.Name.Local, typeName, err)
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
