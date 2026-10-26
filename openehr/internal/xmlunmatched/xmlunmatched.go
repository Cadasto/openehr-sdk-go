// Package xmlunmatched tracks unmatched content on the canonical-XML decode
// path: the content a decode does not read. It is the runtime the generated
// UnmarshalXML methods of openehr/rm and openehr/aom/aom14 call, and that
// canxml drives from its decode entry points; canxml re-exports its
// sentinels.
//
// Content is unmatched when the decode does not read it: an element or
// attribute that names no attribute of the RM or AOM 1.4 type its element
// decodes as, other than namespace declarations and attributes in the XML
// Schema instance namespace (`xsi:type`, `xsi:schemaLocation`, `xsi:nil`, …);
// an element in a namespace other than the openEHR one; and an element whose
// content the codec does not model: a `Hash` member, a BMM `Any` member, and
// a member whose Go type has no decoder of its own. So is text other than
// XML whitespace directly inside an element whose content is elements, one
// item per element however many runs of text it holds. By default the decode
// skips it. A caller can instead track it: report each item, in document
// order, by its path, and optionally refuse the first one.
//
// # How it works
//
// encoding/xml hands a generated UnmarshalXML method only the *xml.Decoder
// and the start element, so the decode's policy cannot travel as an argument.
// It travels with the decoder instead:
//
//   - A tracked decode reads the input through the decoder [Tracker.NewDecoder]
//     returns, whose token source keeps the stack of open elements and, for
//     each, how many children of each name it has opened. Every token passes
//     through it, whichever code asks for it, so the path of the element just
//     read is always known.
//   - [Tracker.Decode] registers that decoder with the decode's [Tracker] for
//     exactly as long as one value decodes. Each tracked decode has its own
//     decoder and tracker, so concurrent decodes never see each other's
//     policy.
//   - The generated decoders call the helpers of this package where content
//     goes unread: [CheckUnmatchedAttrs] on entry, [LocalName] for the name
//     they switch on, [SkipUnmatched] in the arm for an element they do not
//     read, [CheckUnmatchedText] for text between the elements they read,
//     [DecodeElement] for an element decoded as text, [DecodeText] for one
//     whose text they convert themselves, such as an interval boundary flag,
//     [DecodeBase64] for an Octet list, and [DecodeParam] for a member typed
//     by an open generic parameter. The hand-written TERMINOLOGY_CODE
//     decoder calls the same helpers, and [CheckUnmatchedTextBefore] for
//     text it keeps until a child shows that the text is not its value.
//     canxml calls [DiscriminatorRead] where its dispatch reads a bare
//     `type`. Each helper looks the decoder up; an untracked decode finds
//     nothing and behaves as encoding/xml does, but for one thing.
//
// # Primitive failures
//
// The text of a primitive element that does not decode as its type is a
// primitive failure, in every decode: [DecodeElement], [DecodeParam],
// [DecodeText] and [DecodeBase64] return a *typereg.DecodeError whose cause
// wraps [ErrInvalidShape] and the conversion's own error, never the bare
// error encoding/xml or a TextUnmarshaler gives, whose text may quote the
// value. A tracked decode names the element by its path from the root of
// the value and [Tracker.Decode] returns the failure on its own; an
// untracked decode names the element alone, and the failure travels out
// inside the dispatch envelopes it passes through. [IsPrimitiveFailure] tells
// such a failure apart, so canxml can decode the input again, tracked, to
// find the path.
package xmlunmatched

import (
	"bytes"
	"encoding"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// Canonical XML namespaces: the values of canxml.NSDefault and canxml.NSXSI,
// which a canxml test holds equal to these.
const (
	// NSOpenEHR is the openEHR canonical-XML namespace.
	NSOpenEHR = "http://schemas.openehr.org/v1"
	// NSXSI is the XML Schema instance namespace.
	NSXSI = "http://www.w3.org/2001/XMLSchema-instance"
)

// ErrUnknownElement classifies content the decode does not read. canxml
// re-exports it as canxml.ErrUnknownElement, where it is documented.
var ErrUnknownElement = errors.New("canxml: unknown element or attribute")

// ErrNamespace classifies an element in a namespace other than the openEHR
// one. canxml re-exports it as canxml.ErrNamespace, where it is documented.
var ErrNamespace = errors.New("canxml: foreign XML namespace")

// ErrInvalidShape classifies input of the wrong shape, among it the text of a
// primitive element that does not decode as its type. canxml re-exports it as
// canxml.ErrInvalidShape, where it is documented.
var ErrInvalidShape = errors.New("canxml: invalid XML shape")

// Tracker is the policy and position of one tracked decode: the caller's
// report function, whether the first unmatched item refuses the decode, and
// the elements open at the decoder's current token. A Tracker serves one
// decoder, from one goroutine at a time.
type Tracker struct {
	report func(path string)
	strict bool
	// checked counts the elements whose content a decoder of the SDK has
	// checked: each generated UnmarshalXML, through [CheckUnmatchedAttrs],
	// and each primitive element [DecodeElement] reads as text. A rise
	// across a decode shows that something read the element.
	checked int
	// open holds one frame per element opened and not yet closed, after a
	// first frame for the document itself, which counts the top-level
	// elements of the value being decoded.
	open []frame
	// failure is the primitive failure the value being decoded raised, kept
	// so that [Tracker.Decode] returns it on its own; nil until one is raised.
	failure *typereg.DecodeError
}

// frame is one open element: its local name, its 1-based position among its
// earlier siblings of the same name plus one, the children it has opened so
// far, counted by local name, whether dispatch read its bare `type`
// attribute as the discriminator, and whether its text was found unmatched
// already.
//
// The child counts sit in a short slice while the element has opened children
// of few names, which is the canonical shape and costs no map. Once it opens
// children of more than [linearChildNames] names, the counts move to a map, so
// an element with many differently named children costs a lookup per child
// rather than a scan of every name seen before it.
type frame struct {
	name          string
	pos           int
	children      []nameCount
	byName        map[string]int // nil until the slice outgrows linearChildNames
	discriminator bool
	text          bool
}

// linearChildNames is the number of distinct child names a frame counts in its
// slice before it moves the counts to a map. An RM class has a few dozen
// attributes at most, and an element of canonical XML repeats few names.
const linearChildNames = 8

// open makes f the frame of a newly opened element named local at position
// pos, keeping the slice a frame left over from an earlier element had, so a
// long decode does not allocate one per element. A map from an earlier wide
// element is dropped rather than cleared, so a later narrow element never pays
// for it.
func (f *frame) open(local string, pos int) {
	f.name, f.pos, f.children, f.byName, f.discriminator, f.text = local, pos, f.children[:0], nil, false, false
}

// nameCount is how many children of one local name an element has opened.
type nameCount struct {
	name string
	n    int
}

// NewTracker returns the tracker of a decode that passes every unmatched item
// to report, when report is not nil, and refuses the first one when strict is
// set.
func NewTracker(report func(path string), strict bool) *Tracker {
	return &Tracker{report: report, strict: strict, open: make([]frame, 1, 16)}
}

// NewDecoder returns a decoder that reads canonical XML from r and records
// every element it opens and closes in t. Decode values from it with
// [Tracker.Decode].
func (t *Tracker) NewDecoder(r io.Reader) *xml.Decoder {
	return xml.NewTokenDecoder(&trackingReader{src: xml.NewDecoder(r), t: t})
}

// Decode decodes the next value of dec, a decoder from [Tracker.NewDecoder],
// into v, with dec registered for t meanwhile, so the generated decoders
// apply t's policy. Paths are relative to the value being decoded. A strict
// refusal and a primitive failure come back on their own, not inside the
// dispatch envelopes they passed through on their way out: each names its
// item from the root of the value, where each envelope names only its own
// slot.
func (t *Tracker) Decode(dec *xml.Decoder, v any) error {
	t.reset()
	tracking.Add(1)
	trackers.Store(dec, t)
	defer func() {
		trackers.Delete(dec)
		tracking.Add(-1)
	}()
	return t.alone(dec.Decode(v))
}

// reset starts a new value: the document frame forgets the top-level elements
// of the values decoded before, so every path is relative to the value being
// decoded, and no primitive failure is kept.
func (t *Tracker) reset() {
	t.open = t.open[:1]
	t.open[0].open("", 0)
	t.failure = nil
}

// alone returns the failure in err's chain that names its item from the root
// of the value, a strict refusal or the primitive failure t raised, on its
// own, and err unchanged when the chain holds neither (a nil err included).
func (t *Tracker) alone(err error) error {
	if err == nil {
		return nil
	}
	if ue, ok := errors.AsType[*unknownElementError](err); ok && ue != nil {
		return ue
	}
	if t.failure != nil && errors.Is(err, t.failure) {
		return t.failure
	}
	return err
}

// push records that the decoder opened an element named local inside the
// innermost open element. A frame left over from an earlier element is
// reused ([frame.open]).
func (t *Tracker) push(local string) {
	pos := t.open[len(t.open)-1].count(local)
	if len(t.open) < cap(t.open) {
		t.open = t.open[:len(t.open)+1]
		t.open[len(t.open)-1].open(local, pos)
		return
	}
	t.open = append(t.open, frame{name: local, pos: pos})
}

// pop records that the decoder closed the innermost open element.
func (t *Tracker) pop() {
	if len(t.open) > 1 {
		t.open = t.open[:len(t.open)-1]
	}
}

// count notes one more child named local and returns its position among the
// children of that name. It scans at most [linearChildNames] names, then
// looks the name up in the map.
func (f *frame) count(local string) int {
	if f.byName != nil {
		f.byName[local]++
		return f.byName[local]
	}
	for i := range f.children {
		if f.children[i].name == local {
			f.children[i].n++
			return f.children[i].n
		}
	}
	if len(f.children) < linearChildNames {
		f.children = append(f.children, nameCount{name: local, n: 1})
		return 1
	}
	f.byName = make(map[string]int, 4*linearChildNames)
	for _, c := range f.children {
		f.byName[c.name] = c.n
	}
	f.byName[local] = 1
	return 1
}

// path returns the path of the innermost open element: one `/name[n]`
// segment per open element, from the root of the value.
func (t *Tracker) path() string {
	var b strings.Builder
	t.writePath(&b, len(t.open))
	return b.String()
}

// writePath writes the path of the open element whose frame is t.open[n-1]:
// one `/name[n]` segment per open element from the root of the value down to
// it, and nothing for the document frame.
func (t *Tracker) writePath(b *strings.Builder, n int) {
	for _, f := range t.open[1:n] {
		writeSegment(b, f.name, f.pos)
	}
}

// writeSegment writes the path segment of the element named name at position
// pos among its siblings of that name.
func writeSegment(b *strings.Builder, name string, pos int) {
	b.WriteByte('/')
	b.WriteString(name)
	b.WriteByte('[')
	b.WriteString(strconv.Itoa(pos))
	b.WriteByte(']')
}

// place is where an element sits: its local name and its position among its
// siblings of that name. A decode keeps the place of an element it reads as
// text, so that it can name the element once the element has closed.
type place struct {
	name string
	pos  int
}

// here returns the place of the innermost open element.
func (t *Tracker) here() place {
	f := &t.open[len(t.open)-1]
	return place{name: f.name, pos: f.pos}
}

// closedPath returns the path of the element at p, a child of the innermost
// open element that has closed since its place was kept.
func (t *Tracker) closedPath(p place) string {
	var b strings.Builder
	t.writePath(&b, len(t.open))
	writeSegment(&b, p.name, p.pos)
	return b.String()
}

// unmatched applies the policy to the unmatched item at path: report it, then
// refuse it under strict. foreign marks an element in a foreign namespace.
func (t *Tracker) unmatched(path string, foreign bool) error {
	if t.report != nil {
		t.report(path)
	}
	if t.strict {
		return &unknownElementError{path: path, foreign: foreign}
	}
	return nil
}

// attrs applies the policy to every attribute of the innermost open element
// that is not read: neither an XML Schema instance attribute (the `xsi:type`
// discriminator among them), nor a namespace declaration, nor one of known in
// no namespace, nor a bare `type` that dispatch read as the discriminator.
func (t *Tracker) attrs(attrs []xml.Attr, known []string) error {
	t.checked++
	var base string
	read := t.open[len(t.open)-1].discriminator
	for _, a := range attrs {
		if isSchemaInstance(a.Name) || isNamespaceDecl(a.Name) ||
			(a.Name.Space == "" && slices.Contains(known, a.Name.Local)) ||
			(read && isBareType(a.Name)) {
			continue
		}
		if base == "" {
			base = t.path()
		}
		if err := t.unmatched(base+"/@"+a.Name.Local, false); err != nil {
			return err
		}
	}
	return nil
}

// text applies the policy to text the decoder read directly inside the open
// element whose frame is t.open[n-1], whose content is elements: unless it is
// XML whitespace, it is unmatched, at the element's path with a last
// `/text()` segment. An element is one item however many runs of text it
// holds, so only its first unmatched run counts. With n below 2 no element
// holds the text, and it is left alone.
func (t *Tracker) text(n int, text []byte) error {
	if n < 2 {
		return nil
	}
	f := &t.open[n-1]
	if f.text || isXMLSpace(text) {
		return nil
	}
	f.text = true
	var b strings.Builder
	t.writePath(&b, n)
	b.WriteString("/text()")
	return t.unmatched(b.String(), false)
}

// isXMLSpace reports whether text holds XML whitespace only: spaces, tabs,
// carriage returns and line feeds.
func isXMLSpace(text []byte) bool {
	return len(bytes.TrimLeft(text, " \t\r\n")) == 0
}

// decodeText decodes the element start opens, whose content is text, into v.
// An attribute and a child element are unmatched, and the text around a child
// element is kept, as encoding/xml keeps it. The text then decodes into v
// through encoding/xml itself, so the conversion is the one an untracked
// decode applies. Text that does not convert is a primitive failure, which
// names the element by its path from the root of the value.
func (t *Tracker) decodeText(dec *xml.Decoder, start *xml.StartElement, v any) error {
	at := t.here()
	if err := t.attrs(start.Attr, nil); err != nil {
		return err
	}
	var text []byte
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.CharData:
			text = append(text, tk...)
		case xml.StartElement:
			if err := t.unmatched(t.path(), isForeign(tk.Name)); err != nil {
				return err
			}
			if err := dec.Skip(); err != nil {
				return err
			}
		case xml.EndElement:
			name := start.Name
			// The tokens are whole, so the decode fails only where the text
			// does not convert to v's type.
			err := xml.NewTokenDecoder(&tokenList{toks: []xml.Token{
				xml.StartElement{Name: name}, xml.CharData(text), xml.EndElement{Name: name},
			}}).Decode(v)
			if err != nil {
				return t.fail(t.closedPath(at), err)
			}
			return nil
		}
	}
}

var (
	// trackers maps the decoder of each tracked decode in progress to its
	// policy, for exactly as long as that decode runs.
	trackers sync.Map // *xml.Decoder → *Tracker
	// tracking counts the tracked decodes in progress. While it is zero no
	// decoder is registered, and the helpers skip the lookup, so an untracked
	// decode pays one atomic load per helper call.
	tracking atomic.Int64
)

// trackerOf returns the policy registered for dec, or nil when dec belongs to
// no tracked decode.
func trackerOf(dec *xml.Decoder) *Tracker {
	if tracking.Load() == 0 {
		return nil
	}
	if t, ok := trackers.Load(dec); ok {
		return t.(*Tracker)
	}
	return nil
}

// trackingReader is the token source of a tracked decode. It reads src with
// Token, so the namespaces are resolved and the nesting checked there, and
// records every element opened and closed in t. It drops the namespace
// declarations from each start element: the decoder built on it then has no
// prefix to resolve and passes every name through as src resolved it.
type trackingReader struct {
	src *xml.Decoder
	t   *Tracker
}

// Token returns the next token of src, after recording it.
func (r *trackingReader) Token() (xml.Token, error) {
	tok, err := r.src.Token()
	switch tk := tok.(type) {
	case xml.StartElement:
		r.t.push(tk.Name.Local)
		tk.Attr = withoutNamespaceDecls(tk.Attr)
		return tk, err
	case xml.EndElement:
		r.t.pop()
	}
	return tok, err
}

// withoutNamespaceDecls returns attrs without its namespace declarations. It
// returns attrs itself when it holds none.
func withoutNamespaceDecls(attrs []xml.Attr) []xml.Attr {
	if !slices.ContainsFunc(attrs, func(a xml.Attr) bool { return isNamespaceDecl(a.Name) }) {
		return attrs
	}
	return slices.DeleteFunc(slices.Clone(attrs), func(a xml.Attr) bool { return isNamespaceDecl(a.Name) })
}

// tokenList is a token source over a fixed list of tokens.
type tokenList struct {
	toks []xml.Token
}

// Token returns the next token of the list, and io.EOF after the last.
func (l *tokenList) Token() (xml.Token, error) {
	if len(l.toks) == 0 {
		return nil, io.EOF
	}
	tok := l.toks[0]
	l.toks = l.toks[1:]
	return tok, nil
}

// IsDiscriminator reports whether an attribute named name is the `xsi:type`
// discriminator in a form canxml.XSITypeOf reads: in the XSI namespace, in
// none (a bare `type`), or written literally as `xsi:type`.
func IsDiscriminator(name xml.Name) bool {
	return (name.Local == "type" && (name.Space == NSXSI || name.Space == "")) || name.Local == "xsi:type"
}

// isSchemaInstance reports whether an attribute named name belongs to the XML
// Schema instance namespace, such as `xsi:schemaLocation`, `xsi:nil` or the
// `xsi:type` discriminator, in a form encoding/xml resolves or as written
// literally. A bare `type` does not: it is in no namespace. Nor does an
// `xsi:` prefix bound to no namespace: encoding/xml leaves it unresolved, so
// the attribute is not in the namespace.
func isSchemaInstance(name xml.Name) bool {
	return name.Space == NSXSI || strings.HasPrefix(name.Local, "xsi:")
}

// isBareType reports whether an attribute named name is a `type` in no
// namespace, which canxml.XSITypeOf also reads as the discriminator.
func isBareType(name xml.Name) bool {
	return name.Space == "" && name.Local == "type"
}

// DiscriminatorRead records that dispatch read the attribute name of the
// element the decoder read last as its `xsi:type` discriminator. canxml's
// polymorphic dispatch calls it. Only a bare `type` needs the record: an
// attribute in the XML Schema instance namespace is never unmatched, but a
// bare `type` is matched only on an element whose dispatch read it, and is
// unmatched content everywhere else.
func DiscriminatorRead(dec *xml.Decoder, name xml.Name) {
	if !isBareType(name) {
		return
	}
	if t := trackerOf(dec); t != nil {
		t.open[len(t.open)-1].discriminator = true
	}
}

// isNamespaceDecl reports whether name is a namespace declaration: `xmlns`,
// or `xmlns:p` as encoding/xml resolves it or as written literally.
func isNamespaceDecl(name xml.Name) bool {
	return name.Space == "xmlns" || (name.Space == "" && name.Local == "xmlns") || strings.HasPrefix(name.Local, "xmlns:")
}

// isForeign reports whether an element named name lies outside the openEHR
// namespace. An element in no namespace counts as an openEHR one.
func isForeign(name xml.Name) bool {
	return name.Space != "" && name.Space != NSOpenEHR
}

// unknownElementError is the refusal of an unmatched item under strict
// tracking. It names the item by its path from the root of the value and
// carries nothing else from the input, so no value reaches the message. It
// wraps [ErrUnknownElement], and [ErrNamespace] too for an element in a
// foreign namespace.
type unknownElementError struct {
	path    string
	foreign bool
}

// Error names the item's path, quoted so that it reads as one token.
func (e *unknownElementError) Error() string {
	msg := ErrUnknownElement.Error() + " " + strconv.Quote(e.path)
	if e.foreign {
		msg += ": " + ErrNamespace.Error()
	}
	return msg
}

// Unwrap returns the sentinels this refusal carries.
func (e *unknownElementError) Unwrap() []error {
	if e.foreign {
		return []error{ErrUnknownElement, ErrNamespace}
	}
	return []error{ErrUnknownElement}
}

// primitiveError is the cause of a primitive failure: the text of a primitive
// element did not decode as its type. It wraps [ErrInvalidShape] and the
// conversion's own error, whose text may name the value. The DecodeError that
// holds it writes no value: its text keeps the sentinel's and renders the
// conversion's error by its structure alone.
type primitiveError struct {
	cause error
}

// Error names the class, then the conversion's own error.
func (e *primitiveError) Error() string {
	return ErrInvalidShape.Error() + ": " + e.cause.Error()
}

// Unwrap returns the shape sentinel and the conversion's own error.
func (e *primitiveError) Unwrap() []error {
	return []error{ErrInvalidShape, e.cause}
}

// primitiveFailure returns the failure of the element at path, whose text did
// not decode as its type; cause is the conversion's own error.
func primitiveFailure(path string, cause error) *typereg.DecodeError {
	return &typereg.DecodeError{Path: path, Inner: &primitiveError{cause: cause}}
}

// fail returns the primitive failure of the element at path and keeps it, so
// that [Tracker.Decode] returns it on its own.
func (t *Tracker) fail(path string, cause error) error {
	t.failure = primitiveFailure(path, cause)
	return t.failure
}

// IsPrimitiveFailure reports whether err holds a primitive failure: the text
// of a primitive element that did not decode as its type.
func IsPrimitiveFailure(err error) bool {
	pe, ok := errors.AsType[*primitiveError](err)
	return ok && pe != nil
}

// conversionRefusal returns the conversion's own error when err, which
// encoding/xml returned decoding an element's text into a primitive, is the
// conversion's refusal: a *strconv.NumError, which encoding/xml returns for a
// number or a boolean, or the refusal of a TextUnmarshaler read through
// [textConversion]. It returns nil for any other error, such as a syntax
// error or one reading the input, and for nil.
func conversionRefusal(err error) error {
	switch e := err.(type) { //nolint:errorlint // encoding/xml returns the conversion's error as it is
	case *strconv.NumError:
		return e
	case *conversionError:
		return e.cause
	}
	return nil
}

// textConversion reads an element's text through u, as encoding/xml does for
// a TextUnmarshaler, and marks u's refusal as a [conversionError], so that it
// is told apart from an error reading the input.
type textConversion struct {
	u encoding.TextUnmarshaler
}

// UnmarshalText hands text to u.
func (c *textConversion) UnmarshalText(text []byte) error {
	if err := c.u.UnmarshalText(text); err != nil {
		return &conversionError{cause: err}
	}
	return nil
}

// conversionError is the refusal of a TextUnmarshaler read through
// [textConversion].
type conversionError struct {
	cause error
}

// Error is the refusal's own text.
func (e *conversionError) Error() string { return e.cause.Error() }

// Unwrap returns the refusal.
func (e *conversionError) Unwrap() error { return e.cause }

// asText returns, in a [textConversion], the TextUnmarshaler whose
// UnmarshalText method encoding/xml reads v through, and v itself when
// encoding/xml reads v otherwise. That TextUnmarshaler is v itself when v is
// one and no xml.Unmarshaler. When v is a pointer to a pointer, it is the
// pointer v points to, under the same condition, which asText allocates when
// it is nil, as encoding/xml would: a member typed as a pointer to a
// TextUnmarshaler, such as *time.Time, is decoded through the address of
// that pointer.
func asText(v any) any {
	if _, ok := v.(xml.Unmarshaler); ok {
		return v
	}
	if u, ok := v.(encoding.TextUnmarshaler); ok {
		return &textConversion{u: u}
	}
	if p, ok := pointee(v); ok {
		if _, ok := p.(xml.Unmarshaler); !ok {
			if u, ok := p.(encoding.TextUnmarshaler); ok {
				return &textConversion{u: u}
			}
		}
	}
	return v
}

// LocalName returns the local name of an element in the openEHR namespace or
// in none, and "" for an element in any other namespace. Generated
// UnmarshalXML methods switch on it, so an element in a foreign namespace
// matches no RM attribute and reaches their [SkipUnmatched] arm.
func LocalName(name xml.Name) string {
	if isForeign(name) {
		return ""
	}
	return name.Local
}

// SkipUnmatched skips the element start opens, which the generated decoder
// does not read: its name matches no attribute of the RM type being decoded,
// it lies in a foreign namespace, or it holds content the codec does not
// model, such as a `Hash` attribute, a BMM `Any` attribute or a member whose
// Go type has no decoder of its own. The element is one item, whatever it
// holds. start must be the token the decoder read last.
//
// By default it only skips. A tracked decode first reports the element's
// path, and a strict one refuses the decode with an error that wraps
// [ErrUnknownElement], and [ErrNamespace] too for an element in a foreign
// namespace.
func SkipUnmatched(dec *xml.Decoder, start xml.StartElement) error {
	if t := trackerOf(dec); t != nil {
		if err := t.unmatched(t.path(), isForeign(start.Name)); err != nil {
			return err
		}
	}
	return dec.Skip()
}

// CheckUnmatchedAttrs applies the decode's policy for unmatched content to
// the attributes of start that the generated decoder does not read: every
// attribute but those in the XML Schema instance namespace (the `xsi:type`
// discriminator, `xsi:schemaLocation`, `xsi:nil`, …), the namespace
// declarations, those named in known, which are read in no namespace, and a
// bare `type` that dispatch read as the discriminator ([DiscriminatorRead]).
// start must be the token the decoder read last.
//
// By default it does nothing. A tracked decode reports the path of each such
// attribute, in the order written, and a strict one refuses the first with an
// error that wraps [ErrUnknownElement].
func CheckUnmatchedAttrs(dec *xml.Decoder, start xml.StartElement, known ...string) error {
	t := trackerOf(dec)
	if t == nil {
		return nil
	}
	return t.attrs(start.Attr, known)
}

// CheckUnmatchedText applies the decode's policy for unmatched content to
// text, a run of character data the generated decoder read directly inside
// the element it decodes, whose content is elements. A CDATA section is such
// a run too: encoding/xml reports it as character data. dec must be the
// decoder the text was read from, with no child element of that element open.
//
// By default it does nothing, and costs no more than finding the decode
// untracked. Text that is XML whitespace only (spaces, tabs, carriage returns
// and line feeds) is never unmatched. A tracked decode reports any other text
// as one item per element, at the element's path with a last `/text()`
// segment, however many runs the element holds, and a strict one refuses it
// with an error that wraps [ErrUnknownElement]. The path never carries the
// text.
func CheckUnmatchedText(dec *xml.Decoder, text xml.CharData) error {
	t := trackerOf(dec)
	if t == nil {
		return nil
	}
	return t.text(len(t.open), text)
}

// CheckUnmatchedTextBefore applies the decode's policy for unmatched content
// to text read directly inside an element before the child element the
// decoder read last, as [CheckUnmatchedText] does for text read while no
// child is open. A decoder that keeps such text until a child tells it that
// the text is not the element's value calls it as that child opens, so the
// text is reported before anything inside the child. The path names the
// element that holds the text, with a last `/text()` segment, and that
// element remains one item however many runs of text it holds.
func CheckUnmatchedTextBefore(dec *xml.Decoder, text []byte) error {
	t := trackerOf(dec)
	if t == nil {
		return nil
	}
	return t.text(len(t.open)-1, text)
}

// DecodeElement decodes the element start opens into v, as
// dec.DecodeElement(v, start) does. v is a pointer to a primitive, such as a
// string, a number, a boolean or a TextUnmarshaler, a pointer to a pointer
// to a TextUnmarshaler, or a pointer to a type with an UnmarshalXML method.
// start must be the token the decoder read last.
//
// A primitive decodes from the element's text alone: by default, as with
// encoding/xml, an attribute or a child element on it is ignored. A tracked
// decode reports or refuses each as unmatched content, and the value decoded
// is the same as by default. A type with an UnmarshalXML method checks its
// own content. A member of interface type goes through [DecodeParam]
// instead, which sees its type.
//
// Text that does not decode as the primitive's type, such as a boolean
// written `maybe` or a string a TextUnmarshaler refuses, fails with a
// *typereg.DecodeError whose cause wraps [ErrInvalidShape] and the
// conversion's own error, and whose text carries none of the element's. A
// tracked decode names the element by its path from the root of the value;
// an untracked one, which does not know the path, by its name alone.
func DecodeElement(dec *xml.Decoder, start *xml.StartElement, v any) error {
	if t := trackerOf(dec); t != nil {
		if _, ok := v.(xml.Unmarshaler); ok {
			return dec.DecodeElement(v, start)
		}
		return t.decodeText(dec, start, v)
	}
	if err := dec.DecodeElement(asText(v), start); err != nil {
		return untrackedFailure(start, v, err)
	}
	return nil
}

// untrackedFailure returns the error of an untracked decode of the element
// start opens into v that failed with err: a primitive failure, which names
// the element by its name alone, when err is the conversion's refusal, and
// err itself otherwise. A type with an UnmarshalXML method reports its own
// failures.
func untrackedFailure(start *xml.StartElement, v any, err error) error {
	if _, ok := v.(xml.Unmarshaler); ok {
		return err
	}
	if cause := conversionRefusal(err); cause != nil {
		return primitiveFailure("/"+start.Name.Local, cause)
	}
	return err
}

// DecodeParam decodes the element start opens into v, a member typed by an
// open generic parameter T, choosing by T what [DecodeElement] chooses by v.
// start must be the token the decoder read last.
//
//   - T is an interface type, such as the `any` of ORIGINAL_VERSION<any> or
//     the DV_ORDERED of BASE Interval<DV_ORDERED>: the member is a
//     polymorphic slot. An empty one decodes through dispatch, the canxml
//     polymorphic dispatch on `xsi:type` the generated code passes in, so a
//     missing or unknown `xsi:type` fails as at any polymorphic slot. One
//     that already holds a value is decoded into ([decodeInterface]).
//   - T is a pointer to a type with an UnmarshalXML method: encoding/xml
//     allocates it and that method checks its own content.
//   - Any other T, a primitive, a pointer to a TextUnmarshaler or a type
//     with an UnmarshalXML method on its pointer, goes to [DecodeElement].
//
// Text that does not decode as a primitive's type fails as in
// [DecodeElement], whichever way the member decodes.
func DecodeParam[T any](dec *xml.Decoder, start *xml.StartElement, v *T, dispatch func(*xml.Decoder, xml.StartElement) (T, error)) error {
	var zero T
	switch any(zero).(type) {
	case nil:
		if held := any(*v); held != nil {
			return decodeInterface(dec, start, v, held)
		}
		got, err := dispatch(dec, *start)
		if err != nil {
			return err
		}
		*v = got
		return nil
	case xml.Unmarshaler:
		return dec.DecodeElement(v, start)
	}
	return DecodeElement(dec, start, v)
}

// decodeInterface decodes the element start opens into v, a pointer to a
// member of interface type that holds held.
//
// encoding/xml decodes into such a member only when it holds a non-nil
// pointer, and skips the element otherwise; the default decode does exactly
// that. A tracked decode leaves the choice to encoding/xml as well: a pointer
// to an RM type then checks its own content, and when nothing of the SDK
// checked the element's content, because encoding/xml skipped it or read it
// into a primitive, the element is reported or refused as one item once it is
// read. Text that does not decode as the held primitive's type fails as in
// [DecodeElement].
func decodeInterface(dec *xml.Decoder, start *xml.StartElement, v, held any) error {
	target := v
	if tc, ok := asText(held).(*textConversion); ok {
		// encoding/xml reads the element into a held TextUnmarshaler through
		// its UnmarshalText method; decoding into tc is the same decode with
		// the method's refusal marked.
		target = tc
	}
	t := trackerOf(dec)
	if t == nil {
		err := dec.DecodeElement(target, start)
		if cause := conversionRefusal(err); cause != nil {
			return primitiveFailure("/"+start.Name.Local, cause)
		}
		return err
	}
	path, checked := t.path(), t.checked
	if err := dec.DecodeElement(target, start); err != nil {
		if cause := conversionRefusal(err); cause != nil {
			return t.fail(path, cause)
		}
		return err
	}
	if t.checked == checked {
		return t.unmatched(path, isForeign(start.Name))
	}
	return nil
}

// DecodeText reads the text of the element start opens, as [DecodeElement]
// reads a string, and hands it to convert, which reads the text as the
// element's value. start must be the token the decoder read last.
//
// A tracked decode reports an attribute or child element on the element, as
// for any primitive. convert's refusal is a primitive failure, as in
// [DecodeElement]: a *typereg.DecodeError whose cause wraps [ErrInvalidShape]
// and convert's own error, and whose text carries none of the element's, so
// convert's error may quote the text. A tracked decode names the element by
// its path from the root of the value; an untracked one by its name alone.
func DecodeText(dec *xml.Decoder, start *xml.StartElement, convert func(text string) error) error {
	t := trackerOf(dec)
	var at place
	if t != nil {
		at = t.here()
	}
	var text string
	if err := DecodeElement(dec, start, &text); err != nil {
		return err
	}
	if err := convert(text); err != nil {
		if t != nil {
			return t.fail(t.closedPath(at), err)
		}
		return primitiveFailure("/"+start.Name.Local, err)
	}
	return nil
}

// DecodeBase64 decodes the element start opens, an `xs:base64Binary` value
// such as DV_MULTIMEDIA's `data` and `integrity_check`, into v. The element's
// text is read as [DecodeText] reads it, so a tracked decode reports an
// attribute or child element on it. XML whitespace inside the text is
// ignored, as the XML Schema type allows. An empty element leaves v nil, as
// an empty byte field reads in canonical JSON. A text that is no standard,
// padded base64 fails as a primitive's text fails in [DecodeElement]: the
// cause names the offset of the first bad byte, never the text.
func DecodeBase64(dec *xml.Decoder, start *xml.StartElement, v *[]byte) error {
	return DecodeText(dec, start, func(text string) error {
		b, err := base64.StdEncoding.DecodeString(strings.Map(dropXMLSpace, text))
		if err != nil {
			return err
		}
		if len(b) == 0 {
			b = nil
		}
		*v = b
		return nil
	})
}

// dropXMLSpace maps the four XML whitespace characters to -1, which drops
// them in strings.Map, and keeps every other rune.
func dropXMLSpace(r rune) rune {
	switch r {
	case ' ', '\t', '\n', '\r':
		return -1
	}
	return r
}
