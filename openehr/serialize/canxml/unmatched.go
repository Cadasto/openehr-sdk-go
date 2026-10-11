package canxml

// Tracking of unmatched content on the canonical-XML decode path. Content is
// unmatched when the decode does not read it: an element or attribute that
// names no attribute of the RM or AOM 1.4 type its element decodes as, other
// than namespace declarations and attributes in the XML Schema instance
// namespace (`xsi:type`, `xsi:schemaLocation`, `xsi:nil`, …); an element in a
// namespace other than the openEHR one; and the element of a `Hash` member or
// of a BMM `Any` member, which the codec does not model. By default the decode
// skips it. A caller can instead track it: report each item, in document
// order, by its path, and optionally refuse the first one.
//
// How it works. encoding/xml hands a generated UnmarshalXML method only the
// *xml.Decoder and the start element, so the decode's policy cannot travel as
// an argument. It travels with the decoder instead:
//
//   - A tracked decode reads the input through a [trackingReader], which keeps
//     the stack of open elements and, for each, how many children of each
//     name it has opened. Every token passes through it, whichever code asks
//     for it, so the path of the element just read is always known.
//   - The decoder built on that reader is registered in [trackers] with the
//     decode's [tracker] for exactly as long as one value decodes. Each
//     tracked decode has its own decoder and tracker, so concurrent decodes
//     never see each other's policy.
//   - The generated decoders call the exported helpers below where content
//     goes unread: [SkipUnmatched] in the arm for an element they do not
//     read, [CheckUnmatchedAttrs] on entry, and [DecodeElement] for an
//     element decoded as text. Each helper looks the decoder up; an untracked
//     decode finds nothing and behaves as before.

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

// tracker is the policy and position of one tracked decode: the caller's
// report function, whether the first unmatched item refuses the decode, and
// the elements open at the decoder's current token.
type tracker struct {
	report func(path string)
	strict bool
	// open holds one frame per element opened and not yet closed, after a
	// first frame for the document itself, which counts the top-level
	// elements of the value being decoded.
	open []frame
}

// frame is one open element: its local name, its 1-based position among its
// earlier siblings of the same name plus one, and the children it has opened
// so far, counted by local name.
type frame struct {
	name     string
	pos      int
	children []nameCount
}

// nameCount is how many children of one local name an element has opened.
type nameCount struct {
	name string
	n    int
}

func newTracker(report func(path string), strict bool) *tracker {
	return &tracker{report: report, strict: strict, open: make([]frame, 1, 16)}
}

// reset starts a new value: the document frame forgets the top-level elements
// of the values decoded before, so every path is relative to the value being
// decoded.
func (t *tracker) reset() {
	t.open = t.open[:1]
	t.open[0].children = t.open[0].children[:0]
}

// push records that the decoder opened an element named local inside the
// innermost open element. A frame left over from an earlier element keeps its
// children slice, so a long decode does not allocate one per element.
func (t *tracker) push(local string) {
	pos := t.open[len(t.open)-1].count(local)
	if len(t.open) < cap(t.open) {
		t.open = t.open[:len(t.open)+1]
		f := &t.open[len(t.open)-1]
		f.name, f.pos, f.children = local, pos, f.children[:0]
		return
	}
	t.open = append(t.open, frame{name: local, pos: pos})
}

// pop records that the decoder closed the innermost open element.
func (t *tracker) pop() {
	if len(t.open) > 1 {
		t.open = t.open[:len(t.open)-1]
	}
}

// count notes one more child named local and returns its position among the
// children of that name.
func (f *frame) count(local string) int {
	for i := range f.children {
		if f.children[i].name == local {
			f.children[i].n++
			return f.children[i].n
		}
	}
	f.children = append(f.children, nameCount{name: local, n: 1})
	return 1
}

// path returns the path of the innermost open element: one `/name[n]`
// segment per open element, from the root of the value.
func (t *tracker) path() string {
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
func (t *tracker) unmatched(path string, foreign bool) error {
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
// no namespace.
func (t *tracker) attrs(attrs []xml.Attr, known []string) error {
	var base string
	for _, a := range attrs {
		if isSchemaInstance(a.Name) || isNamespaceDecl(a.Name) || (a.Name.Space == "" && slices.Contains(known, a.Name.Local)) {
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
func (t *tracker) decodeText(dec *xml.Decoder, start *xml.StartElement, v any) error {
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

// decode decodes the next value of dec, a decoder reading through t, into v,
// with dec registered for t meanwhile. A strict refusal comes back on its
// own (see [unwrapUnknownElement]).
func (t *tracker) decode(dec *xml.Decoder, v any) error {
	t.reset()
	tracking.Add(1)
	trackers.Store(dec, t)
	defer func() {
		trackers.Delete(dec)
		tracking.Add(-1)
	}()
	return unwrapUnknownElement(dec.Decode(v))
}

var (
	// trackers maps the decoder of each tracked decode in progress to its
	// policy, for exactly as long as that decode runs.
	trackers sync.Map // *xml.Decoder → *tracker
	// tracking counts the tracked decodes in progress. While it is zero no
	// decoder is registered, and the helpers skip the lookup, so an untracked
	// decode pays one atomic load per helper call.
	tracking atomic.Int64
)

// trackerOf returns the policy registered for dec, or nil when dec belongs to
// no tracked decode.
func trackerOf(dec *xml.Decoder) *tracker {
	if tracking.Load() == 0 {
		return nil
	}
	if t, ok := trackers.Load(dec); ok {
		return t.(*tracker)
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
	t   *tracker
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

// isDiscriminator reports whether name is the `xsi:type` discriminator in a
// form [XSITypeOf] reads: in the XSI namespace, in none, or written literally.
func isDiscriminator(name xml.Name) bool {
	return (name.Local == "type" && (name.Space == NSXSI || name.Space == "")) || name.Local == "xsi:type"
}

// isSchemaInstance reports whether an attribute named name belongs to the XML
// Schema instance namespace, such as `xsi:schemaLocation`, `xsi:nil` or the
// `xsi:type` discriminator, in a form encoding/xml resolves or as written
// literally. A bare `type`, which [XSITypeOf] reads as the discriminator,
// counts too. An `xsi:` prefix bound to no namespace does not: encoding/xml
// leaves it unresolved, so the attribute is not in the namespace.
func isSchemaInstance(name xml.Name) bool {
	return name.Space == NSXSI || strings.HasPrefix(name.Local, "xsi:") || isDiscriminator(name)
}

// isNamespaceDecl reports whether name is a namespace declaration: `xmlns`,
// or `xmlns:p` as encoding/xml resolves it or as written literally.
func isNamespaceDecl(name xml.Name) bool {
	return name.Space == "xmlns" || (name.Space == "" && name.Local == "xmlns") || strings.HasPrefix(name.Local, "xmlns:")
}

// isForeign reports whether an element named name lies outside the openEHR
// namespace. An element in no namespace counts as an openEHR one.
func isForeign(name xml.Name) bool {
	return name.Space != "" && name.Space != NSDefault
}

// unknownElementError is the refusal of an unmatched item under [WithStrict].
// It names the item by its path from the root of the value and carries
// nothing else from the input, so no value reaches the message. It wraps
// [ErrUnknownElement], and [ErrNamespace] too for an element in a foreign
// namespace.
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
// or err unchanged when the chain holds none (a nil err included). On its way
// out the refusal may pass through a [DecodeError] at each polymorphic slot,
// which names the slot's element alone; the refusal names the item from the
// root, so the entry point returns it alone.
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
// it lies in a foreign namespace, or it holds a `Hash` attribute or the
// content of a BMM `Any` attribute, which this codec does not model. The
// element is one item, whatever it holds. start must be the token the decoder
// read last.
//
// By default it only skips. Under [WithUnmatched] it first reports the
// element's path. Under [WithStrict] it refuses the decode with an error
// that wraps [ErrUnknownElement], and [ErrNamespace] too for an element in a
// foreign namespace. Generated UnmarshalXML methods call it; it is exported
// for them only.
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
// declarations, and those named in known, which are read in no namespace.
// start must be the token the decoder read last.
//
// By default it does nothing. Under [WithUnmatched] it reports the path of
// each such attribute, in the order written, and under [WithStrict] it
// refuses the first with an error that wraps [ErrUnknownElement]. Generated
// UnmarshalXML methods call it on entry; it is exported for them only.
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
// encoding/xml, an attribute or a child element on it is ignored. Under
// [WithUnmatched] and [WithStrict] each is unmatched content, reported or
// refused like any other, and the value decoded is the same as by default.
// A type with an UnmarshalXML method checks its own content. Generated
// UnmarshalXML methods call it; it is exported for them only.
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
