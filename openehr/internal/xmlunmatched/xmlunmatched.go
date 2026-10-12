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
// a member whose Go type has no decoder of its own. By default the decode
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
//     read, [DecodeElement] for an element decoded as text, and
//     [DecodeParam] for a member typed by an open generic parameter. canxml
//     calls [DiscriminatorRead] where its dispatch reads a bare `type`. Each
//     helper looks the decoder up; an untracked decode finds nothing and
//     behaves as encoding/xml does.
package xmlunmatched

import (
	"encoding/xml"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
}

// frame is one open element: its local name, its 1-based position among its
// earlier siblings of the same name plus one, the children it has opened so
// far, counted by local name, and whether dispatch read its bare `type`
// attribute as the discriminator.
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
	f.name, f.pos, f.children, f.byName, f.discriminator = local, pos, f.children[:0], nil, false
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
// refusal comes back on its own, not inside the dispatch envelopes it passed
// through on its way out: it names the item from the root of the value, where
// each envelope names only its own slot.
func (t *Tracker) Decode(dec *xml.Decoder, v any) error {
	t.reset()
	tracking.Add(1)
	trackers.Store(dec, t)
	defer func() {
		trackers.Delete(dec)
		tracking.Add(-1)
	}()
	return unwrapUnknownElement(dec.Decode(v))
}

// reset starts a new value: the document frame forgets the top-level elements
// of the values decoded before, so every path is relative to the value being
// decoded.
func (t *Tracker) reset() {
	t.open = t.open[:1]
	t.open[0].open("", 0)
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
	for _, f := range t.open[1:] {
		b.WriteByte('/')
		b.WriteString(f.name)
		b.WriteByte('[')
		b.WriteString(strconv.Itoa(f.pos))
		b.WriteByte(']')
	}
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

// decodeText decodes the element start opens, whose content is text, into v.
// An attribute and a child element are unmatched, and the text around a child
// element is kept, as encoding/xml keeps it. The text then decodes into v
// through encoding/xml itself, so the conversion is the one an untracked
// decode applies.
func (t *Tracker) decodeText(dec *xml.Decoder, start *xml.StartElement, v any) error {
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
			return xml.NewTokenDecoder(&tokenList{toks: []xml.Token{
				xml.StartElement{Name: name}, xml.CharData(text), xml.EndElement{Name: name},
			}}).Decode(v)
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

// unwrapUnknownElement returns the strict refusal in err's chain on its own,
// or err unchanged when the chain holds none (a nil err included).
func unwrapUnknownElement(err error) error {
	if ue, ok := errors.AsType[*unknownElementError](err); ok && ue != nil {
		return ue
	}
	return err
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

// DecodeElement decodes the element start opens into v, as
// dec.DecodeElement(v, start) does. v is a pointer to a primitive, such as a
// string, a number or a boolean, or to a type with an UnmarshalXML method.
// start must be the token the decoder read last.
//
// A primitive decodes from the element's text alone: by default, as with
// encoding/xml, an attribute or a child element on it is ignored. A tracked
// decode reports or refuses each as unmatched content, and the value decoded
// is the same as by default. A type with an UnmarshalXML method checks its
// own content. A member of interface type goes through [DecodeParam]
// instead, which sees its type.
func DecodeElement(dec *xml.Decoder, start *xml.StartElement, v any) error {
	t := trackerOf(dec)
	if t == nil {
		return dec.DecodeElement(v, start)
	}
	if _, ok := v.(xml.Unmarshaler); ok {
		return dec.DecodeElement(v, start)
	}
	return t.decodeText(dec, start, v)
}

// DecodeParam decodes the element start opens into v, a member typed by an
// open generic parameter T, choosing by T what [DecodeElement] chooses by v.
// start must be the token the decoder read last.
//
//   - T is an interface type, such as the `any` of ORIGINAL_VERSION<any> or
//     the DV_ORDERED of BASE Interval<DV_ORDERED>: see [decodeInterface].
//   - T is a pointer to a type with an UnmarshalXML method: encoding/xml
//     allocates it and that method checks its own content.
//   - Any other T, a primitive or a type with an UnmarshalXML method on its
//     pointer, goes to [DecodeElement].
func DecodeParam[T any](dec *xml.Decoder, start *xml.StartElement, v *T) error {
	var zero T
	switch any(zero).(type) {
	case nil:
		return decodeInterface(dec, start, any(*v) == nil, v)
	case xml.Unmarshaler:
		return dec.DecodeElement(v, start)
	}
	return DecodeElement(dec, start, v)
}

// decodeInterface decodes the element start opens into v, a pointer to a
// member of interface type; empty reports whether the member holds nothing.
//
// encoding/xml decodes into a member of interface type only when it holds a
// non-nil pointer, and skips the element otherwise; the default decode does
// exactly that. An empty member gives it nothing to decode into, so a
// tracked decode reports or refuses the element as one item before it is
// skipped ([SkipUnmatched]). A member that holds a value is left to
// encoding/xml as well: a pointer to an RM type then checks its own content,
// and when nothing of the SDK checked the element's content, because
// encoding/xml skipped it or read it into a primitive, a tracked decode
// reports or refuses the element as one item once it is read.
func decodeInterface(dec *xml.Decoder, start *xml.StartElement, empty bool, v any) error {
	t := trackerOf(dec)
	if t == nil {
		return dec.DecodeElement(v, start)
	}
	if empty {
		return SkipUnmatched(dec, *start)
	}
	path, checked := t.path(), t.checked
	if err := dec.DecodeElement(v, start); err != nil {
		return err
	}
	if t.checked == checked {
		return t.unmatched(path, isForeign(start.Name))
	}
	return nil
}
