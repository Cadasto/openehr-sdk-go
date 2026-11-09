package xmlunmatched

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Unmarshal decodes data, one canonical-XML document, into v: through t, so
// that the generated decoders apply t's policy and t's hooks see what the
// decode reports, or through a plain encoding/xml decoder when t is nil.
// canxml.Unmarshal calls it with the tracker its options build, or none, so
// that a caller of this package that needs no option decodes a document by
// the same rule.
//
// data holds one root element. Before the root it may hold the XML
// declaration, comments, processing instructions, whitespace and one
// well-formed document type declaration ([doctypeWellFormed]), after a UTF-8
// byte order mark at the very start; comments, processing instructions and
// whitespace may follow the root, as XML allows. Whitespace is the literal
// characters: a CDATA section or a character reference is element content, so
// outside the root it is refused whatever it stands for. XML reserves the
// processing instruction target xml, in any letter case, for the XML
// declaration, which only the byte order mark may precede: one elsewhere,
// before, inside or after the root, a second declaration or one after
// whitespace among them, is refused, and so is a markup declaration (`<!…>`)
// inside the root. Anything else before or after the root, such as an element
// or text other than whitespace, is not well-formed XML, and the decode
// refuses it whatever t's policy: it is malformed input, not unmatched
// content. The error wraps [ErrInvalidShape] and [ErrNotWellFormed] and
// carries none of that content. A root that fails to decode fails as itself,
// and the decode does not read on.
//
// A primitive failure comes back on its own, naming the element by its path
// from the root of the value (see "Primitive failures" in the package
// documentation), and so does the refusal of an element nested past
// encoding/xml's own depth limit (see "Depth limit"). A tracked decode knows
// the path; an untracked one finds it by decoding data a second time, once
// the first decode has failed so, which a decode that succeeds never pays
// for. Other XML syntax errors come back unchanged from encoding/xml.
//
// A tracked decode also knows the namespace prefixes declared around each
// element, which an `xsi:type` written with a prefix needs ([LookupPrefix]).
// An untracked one sees only those its element declares itself; when an
// `xsi:type` needs another ([UnresolvedPrefix]), it decodes data a second
// time, through a decoder that sees them all, so that only a document with
// such a value pays for the second decode. The second decode reads into v as
// the first left it: each member it decodes replaces what v held, a list from
// its first item, so v keeps what the document leaves out, as after one
// decode.
func Unmarshal(data []byte, v any, t *Tracker) error {
	rootAt, err := checkProlog(data)
	if err != nil {
		return err
	}
	if t == nil {
		dec := xml.NewDecoder(bytes.NewReader(data))
		if err := dec.Decode(v); err != nil {
			if IsPrimitiveFailure(err) || IsUnresolvedPrefix(err) || IsXMLDepthError(err) {
				return decodeAgain(data, v, err)
			}
			return err
		}
		if err := afterRoot(data[dec.InputOffset():]); err != nil {
			return err
		}
		if xmlTargetPastStart(data) || mayHoldDeclaration(data[rootAt:]) {
			// Only a tracked decode sees the processing instructions and
			// the declarations inside the root, so a document that may hold
			// one it refuses is decoded again through one.
			return decodeAgain(data, v, nil)
		}
		return nil
	}
	dec := t.NewDecoder(bytes.NewReader(data))
	if err := t.Decode(dec, v); err != nil {
		return err
	}
	return afterRoot(data[t.inputOffset():])
}

// decodeAgain decodes data into v once more after an untracked decode failed
// with err: a primitive failure that names the element alone, an `xsi:type`
// whose prefix the decode could not resolve, or encoding/xml's refusal of an
// element nested past its own depth limit, which names no element; or after
// one that succeeded on data that may hold a processing instruction whose
// target is xml or a markup declaration inside its root, when err is nil.
// The second decode reads through a tracker that follows the open elements
// and their namespace scope and applies no policy, into v as the first left
// it: each member it decodes replaces what v held, a list from its first
// item, so v keeps what the document leaves out, as after one decode. A
// primitive failure fails again at the same element, named by its path from
// the root of the value, and so does a depth refusal, since a tracked decode
// refuses every element an untracked one hands to encoding/xml past its
// limit, a primitive element too ([Tracker.decodeText]). A prefix resolves,
// and the decode reads on to the end, as it does past a processing
// instruction or a declaration it finds to be no such one, or refuses one
// that is. It returns what the second decode returns, or err should a
// primitive failure not fail again.
func decodeAgain(data []byte, v any, err error) error {
	t := NewTracker(nil, false)
	dec := t.NewDecoder(bytes.NewReader(data))
	if again := t.Decode(dec, v); again != nil {
		return again
	}
	if IsPrimitiveFailure(err) {
		return err
	}
	return afterRoot(data[t.inputOffset():])
}

// afterRoot holds rest, the bytes of a document after its root element, to
// the rule [epilogue] describes.
func afterRoot(rest []byte) error {
	return epilogue(xml.NewDecoder(bytes.NewReader(rest)), func(from, to int64) []byte { return rest[from:to] })
}

// ReadAfterRoot reads r, the rest of a document after its root element, to
// its end, and holds it to the rule [Unmarshal] holds that rest to: comments,
// processing instructions and whitespace as written pass, and anything else
// is refused with an error that wraps [ErrInvalidShape] and
// [ErrNotWellFormed] and names none of it. An error r returns, other than
// [io.EOF], comes back as r returned it.
func ReadAfterRoot(r io.ByteReader) error {
	rec := &recorder{r: r}
	// rec.read starts at offset base: what comes before the token epilogue
	// reads is not read again, so it is let go, and the bytes kept are those
	// of one token at a time.
	var base int64
	err := epilogue(xml.NewDecoder(rec), func(from, to int64) []byte {
		rec.read = rec.read[from-base:]
		base = from
		return rec.read[:to-base]
	})
	if rec.err != nil {
		return rec.err
	}
	return err
}

// epilogue reads d's tokens, the rest of a document after its root element,
// to the end. Comments, processing instructions and whitespace pass, the
// whitespace as written, raw being the bytes d read from offset from to
// offset to, which it asks for each token in turn, never for an earlier one:
// a CDATA section or a character reference is element content,
// whatever it stands for (https://www.w3.org/TR/xml/#NT-Misc). Anything else,
// an element, other text, a document type declaration, a processing
// instruction whose target is xml in any letter case, or a token encoding/xml
// cannot read, is refused: the error wraps [ErrInvalidShape] and names none
// of it.
func epilogue(d *xml.Decoder, raw func(from, to int64) []byte) error {
	for {
		from := d.InputOffset()
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return errAfterRoot()
		}
		written := raw(from, d.InputOffset())
		switch t := tok.(type) {
		case xml.Comment:
			continue
		case xml.ProcInst:
			if !isXMLTarget(t.Target) {
				continue
			}
		case xml.CharData:
			if isXMLSpace(written) {
				continue
			}
		}
		return errAfterRoot()
	}
}

// errAfterRoot is the refusal of content after the root element.
func errAfterRoot() error {
	return fmt.Errorf("%w: %w", ErrInvalidShape, notWellFormed("content after the root element"))
}

// ContentAfterRoot returns the refusal of content after the root element that
// [Unmarshal] gives: it wraps [ErrInvalidShape] and [ErrNotWellFormed] and
// names none of the content. A reader that holds a document to the same rule
// returns it.
func ContentAfterRoot() error { return errAfterRoot() }

// ErrNotWellFormed classifies the decode's own refusal of input that is not
// well-formed XML, apart from the syntax errors encoding/xml reports. It is
// re-exported as canxml.ErrNotWellFormed, where it is documented.
var ErrNotWellFormed = errors.New("canxml: XML not well-formed")

// notWellFormed is the cause of a refusal of input that is not well-formed
// XML. Its text names the rule the input breaks and none of the input, and it
// matches [ErrNotWellFormed] without writing that sentinel's text, so a
// refusal's message reads as the rule alone.
type notWellFormed string

// Error returns the rule the input breaks.
func (e notWellFormed) Error() string { return string(e) }

// Is reports whether target is [ErrNotWellFormed].
func (e notWellFormed) Is(target error) bool { return target == ErrNotWellFormed }

// utf8BOM is the UTF-8 byte order mark, which a document may start with as
// its encoding signature.
var utf8BOM = []byte("\ufeff")

// checkProlog reads data up to its first element, the root, apart from the
// decode, which cannot see what encoding/xml skips there, and returns the
// offset in data of the root's start tag. A UTF-8 byte order mark at the
// very start, the XML declaration and other processing instructions,
// comments, whitespace as written and one well-formed document type
// declaration ([doctypeWellFormed]) pass. Anything else, other text or
// markup, a CDATA section or a character reference whatever it stands for, a
// second document type declaration or one XML's grammar does not allow, an
// end element, or a processing instruction whose target is xml in any letter
// case other than the XML declaration, in the internal subset of the document
// type declaration too, is refused: the error wraps [ErrInvalidShape] and
// names none of it. The XML declaration is a processing instruction whose
// target is xml, in lower case, that nothing comes before but the byte order
// mark. A token encoding/xml cannot read, and data that holds no element, are
// left to the decode, which fails on them as it would without this check; the
// offset is then 0. Data that opens with its root after whitespace, as the
// encoder writes it, costs no tokenizing.
func checkProlog(data []byte) (int, error) {
	body := bytes.TrimPrefix(data, utf8BOM)
	bom := len(data) - len(body)
	if rootFirst(body) {
		return bom + len(body) - len(bytes.TrimLeft(body, " \t\r\n")), nil
	}
	at, err := prolog(xml.NewDecoder(bytes.NewReader(body)), false, func(from, to int64) []byte { return body[from:to] })
	if at == 0 {
		return 0, err
	}
	return bom + int(at), err
}

// ReadProlog reads r up to the end of the start tag of its first element,
// the root, holding what comes before the root to the rules of [Unmarshal]
// (see checkProlog), and returns every byte it read, so that a decode reads
// the document from them and then from the rest of r. It reads on past the
// first token it refuses, to the root or to the end of r: it refuses that
// token, with an error that wraps [ErrInvalidShape] and names none of the
// content, only when a root follows it, and returns [io.EOF] when r ends
// first, since input with no root element holds no document at all. A token
// encoding/xml cannot read, and input that holds no element and nothing
// refused, are left to the decode, which fails on them as it would without
// this check.
//
// It takes a [bufio.Reader] so that it reads no byte past the root's start
// tag and can look for a byte order mark at the very start.
func ReadProlog(r *bufio.Reader) ([]byte, error) {
	rec := &recorder{r: r}
	if head, _ := r.Peek(len(utf8BOM)); bytes.Equal(head, utf8BOM) {
		rec.read = append(rec.read, utf8BOM...)
		_, _ = r.Discard(len(utf8BOM)) // the bytes Peek returned: Discard cannot fail
	}
	base := int64(len(rec.read))
	_, err := prolog(xml.NewDecoder(rec), true, func(from, to int64) []byte { return rec.read[base+from : base+to] })
	return rec.read, err
}

// recorder reads one byte at a time from r and keeps each byte it reads, and
// the first error r returns other than [io.EOF]. encoding/xml reads an
// io.ByteReader one byte at a time, with no buffering of its own, so the
// bytes it has not used stay in r.
type recorder struct {
	r    io.ByteReader
	read []byte
	err  error
}

// ReadByte reads one byte from r and keeps it.
func (c *recorder) ReadByte() (byte, error) {
	b, err := c.r.ReadByte()
	switch {
	case err == nil:
		c.read = append(c.read, b)
	case !errors.Is(err, io.EOF) && c.err == nil:
		c.err = err
	}
	return b, err
}

// Read reads one byte into p; encoding/xml calls ReadByte instead.
func (c *recorder) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	b, err := c.ReadByte()
	if err != nil {
		return 0, err
	}
	p[0] = b
	return 1, nil
}

// prolog reads d's tokens up to its first element, the root, by the rules
// checkProlog describes, and returns the offset of the root's start tag in
// d's input, or 0 when it reads none. raw returns the bytes of d's input from
// offset from to offset to. Given untilRoot, it reads on past the first token
// it refuses, as ReadProlog describes; a token encoding/xml cannot read after
// one it refused is refused as well.
func prolog(d *xml.Decoder, untilRoot bool, raw func(from, to int64) []byte) (int64, error) {
	doctype, refused := false, false
	// start holds for the first token alone, where the XML declaration may
	// stand: XML allows nothing before it, whitespace included
	// (https://www.w3.org/TR/xml/#NT-prolog). The byte order mark is gone by
	// now.
	start := true
	for {
		from := d.InputOffset()
		tok, err := d.RawToken()
		if err != nil {
			if !refused {
				return 0, nil
			}
			if errors.Is(err, io.EOF) {
				return 0, io.EOF
			}
			if _, ok := errors.AsType[*xml.SyntaxError](err); ok {
				return 0, errBeforeRoot()
			}
			return 0, err
		}
		if _, ok := tok.(xml.StartElement); ok {
			if refused {
				return 0, errBeforeRoot()
			}
			return from, nil
		}
		ok := refused || beforeRoot(tok, raw(from, d.InputOffset()), &doctype, start)
		start = false
		if ok {
			continue
		}
		if !untilRoot {
			return 0, errBeforeRoot()
		}
		refused = true
	}
}

// beforeRoot reports whether tok, written as raw, may come before the root
// element: a comment, whitespace as written, which a CDATA section or a
// character reference is not whatever it stands for
// (https://www.w3.org/TR/xml/#NT-Misc), the first document type declaration
// if XML's grammar allows it ([doctypeWellFormed]), which it records in
// doctype, or a processing instruction other than one whose target is xml in
// any letter case, unless that is the XML declaration, at the start.
func beforeRoot(tok xml.Token, raw []byte, doctype *bool, start bool) bool {
	switch t := tok.(type) {
	case xml.Comment:
		return true
	case xml.ProcInst:
		return !isXMLTarget(t.Target) || (start && t.Target == "xml")
	case xml.CharData:
		return isXMLSpace(raw)
	case xml.Directive:
		if !*doctype && doctypeWellFormed(raw) {
			*doctype = true
			return true
		}
	}
	return false
}

// rootFirst reports whether data opens with a start element after XML
// whitespace, so that nothing else comes before its root: its first markup
// is neither a processing instruction, a comment, a declaration nor an end
// element.
func rootFirst(data []byte) bool {
	head := bytes.TrimLeft(data, " \t\r\n")
	return len(head) > 1 && head[0] == '<' && head[1] != '?' && head[1] != '!' && head[1] != '/'
}

// xmlTargetPastStart reports whether data, one document, may hold a
// processing instruction whose target is xml anywhere but at its start, after
// a byte order mark, where the XML declaration stands ([opensXMLTarget]). A
// comment or a CDATA section can hold those bytes too, which only a decode
// tells apart. It costs one search of data for `<?`.
func xmlTargetPastStart(data []byte) bool {
	data = bytes.TrimPrefix(data, utf8BOM)
	for at := 1; at < len(data); at += len(piOpen) {
		i := bytes.Index(data[at:], piOpen)
		if i < 0 {
			return false
		}
		at += i
		if opensXMLTarget(data[at:]) {
			return true
		}
	}
	return false
}

// opensXMLTarget reports whether b opens with a processing instruction whose
// target is xml in any letter case: `<?`, those letters, and XML whitespace
// or `?`.
func opensXMLTarget(b []byte) bool {
	const n = len("<?xml")
	return len(b) > n && bytes.HasPrefix(b, piOpen) && bytes.EqualFold(b[len(piOpen):n], xmlTarget) && (b[n] == '?' || isXMLSpace(b[n:n+1]))
}

// mayHoldDeclaration reports whether b, a document from its root's start tag
// on, may hold a markup declaration (`<!…>`): `<!` that opens neither a
// comment nor a CDATA section. It reads the markup that can hold those bytes
// in document order, skipping each comment, CDATA section and processing
// instruction whole, so that one holding `<!`, or the opener of another, is
// no declaration and hides none. An unclosed one ends the search, the decode
// having refused it already. Each byte of b is searched at most once for
// each opener.
func mayHoldDeclaration(b []byte) bool {
	// pi is the offset of the next `<?` at or after at, -1 when none is
	// left; it is searched for again only once at has passed it.
	at, pi := 0, -1
	for piKnown := false; ; {
		d := bytes.Index(b[at:], declOpen)
		if d < 0 {
			return false
		}
		d += at
		if !piKnown || (pi >= 0 && pi < at) {
			if pi = bytes.Index(b[at:], piOpen); pi >= 0 {
				pi += at
			}
			piKnown = true
		}
		open, end := d+len(declOpen), []byte(nil)
		switch {
		case pi >= 0 && pi < d:
			open, end = pi+len(piOpen), piClose
		case bytes.HasPrefix(b[open:], commentOpen):
			end = commentClose
		case bytes.HasPrefix(b[open:], cdataOpen):
			end = cdataClose
		default:
			return true
		}
		j := bytes.Index(b[open:], end)
		if j < 0 {
			return false
		}
		at = open + j + len(end)
	}
}

// declOpen opens a markup declaration, a comment or a CDATA section; what
// follows it in the last two opens them, and their close ends them, as
// piClose ends a processing instruction ([mayHoldDeclaration]).
var (
	declOpen                  = []byte("<!")
	commentOpen, commentClose = []byte("--"), []byte("-->")
	cdataOpen, cdataClose     = []byte("[CDATA["), []byte("]]>")
	piClose                   = []byte("?>")
)

// piOpen opens a processing instruction, and xmlTarget is the target XML
// reserves for its declaration ([opensXMLTarget]).
var piOpen, xmlTarget = []byte("<?"), []byte("xml")

// isXMLTarget reports whether target, a processing instruction's, is xml in
// any letter case, which XML reserves for the XML declaration
// (https://www.w3.org/TR/xml/#NT-PITarget). A target that only starts with
// those letters, such as xml-stylesheet, is not.
func isXMLTarget(target string) bool {
	return strings.EqualFold(target, "xml")
}

// errBeforeRoot is the refusal of content before the root element.
func errBeforeRoot() error {
	return fmt.Errorf("%w: %w", ErrInvalidShape, notWellFormed("content before the root element"))
}
