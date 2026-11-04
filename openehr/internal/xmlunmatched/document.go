package xmlunmatched

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
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
// document type declaration, after a UTF-8 byte order mark at the very
// start; comments, processing instructions and whitespace may follow the
// root, as XML allows. Anything else before or after the root, such as an
// element or text other than whitespace, is not well-formed XML, and the
// decode refuses it whatever t's policy: it is malformed input, not
// unmatched content. The error wraps [ErrInvalidShape] and carries none of
// that content. A root that fails to decode fails as itself, and the decode
// does not read on.
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
// time, through a decoder that sees them all, into v cleared to its zero
// value, so that only a document with such a value pays for the second
// decode.
func Unmarshal(data []byte, v any, t *Tracker) error {
	if err := checkProlog(data); err != nil {
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
		return end(dec)
	}
	dec := t.NewDecoder(bytes.NewReader(data))
	if err := t.Decode(dec, v); err != nil {
		return err
	}
	return end(dec)
}

// decodeAgain decodes data into v once more after an untracked decode failed
// with err: a primitive failure that names the element alone, an `xsi:type`
// whose prefix the decode could not resolve, or encoding/xml's refusal of an
// element nested past its own depth limit, which names no element. The second
// decode reads through a tracker that follows the open elements and their
// namespace scope and applies no policy. A primitive failure fails again at
// the same element, named by its path from the root of the value, and so does
// a depth refusal. A prefix resolves, and the decode reads on to the end: so
// that it does not add to what the first decode left in v, such as the items
// of a list, the root's decoder clears v before it reads ([Enter]), as it does
// after a depth refusal, should the tracked decode read on where the first
// did not. It returns what the second decode returns, or err should a
// primitive failure not fail again.
func decodeAgain(data []byte, v any, err error) error {
	t := NewTracker(nil, false)
	t.again = IsUnresolvedPrefix(err) || IsXMLDepthError(err)
	dec := t.NewDecoder(bytes.NewReader(data))
	if again := t.Decode(dec, v); again != nil {
		return again
	}
	if IsPrimitiveFailure(err) {
		return err
	}
	return end(dec)
}

// end reads the rest of the document after the root element. Comments,
// processing instructions and whitespace pass. Anything else, an element,
// other text, a document type declaration or a token encoding/xml cannot
// read, is refused: the error wraps [ErrInvalidShape] and names none of
// it.
func end(dec *xml.Decoder) error {
	for {
		tok, err := dec.Token()
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

// utf8BOM is the UTF-8 byte order mark, which a document may start with as
// its encoding signature.
var utf8BOM = []byte("\ufeff")

// checkProlog reads data up to its first element, the root, apart from the
// decode, which cannot see what encoding/xml skips there. A UTF-8 byte order
// mark at the very start, the XML declaration and other processing
// instructions, comments, whitespace and one document type declaration
// pass. Anything else, other text or markup, a second document type
// declaration or an end element, is refused: the error wraps
// [ErrInvalidShape] and names none of it. A token encoding/xml cannot read,
// and data that holds no element, are left to the decode, which fails on
// them as it would without this check. Data that opens with its root after
// whitespace, as the encoder writes it, costs no tokenizing.
func checkProlog(data []byte) error {
	data = bytes.TrimPrefix(data, utf8BOM)
	if rootFirst(data) {
		return nil
	}
	return prolog(xml.NewDecoder(bytes.NewReader(data)), false)
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
	err := prolog(xml.NewDecoder(rec), true)
	return rec.read, err
}

// recorder reads one byte at a time from r and keeps each byte it reads.
// encoding/xml reads an io.ByteReader one byte at a time, with no buffering
// of its own, so the bytes it has not used stay in r.
type recorder struct {
	r    io.ByteReader
	read []byte
}

// ReadByte reads one byte from r and keeps it.
func (c *recorder) ReadByte() (byte, error) {
	b, err := c.r.ReadByte()
	if err == nil {
		c.read = append(c.read, b)
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
// checkProlog describes. Given untilRoot, it reads on past the first token
// it refuses, as ReadProlog describes; a token encoding/xml cannot read after
// one it refused is refused as well.
func prolog(d *xml.Decoder, untilRoot bool) error {
	doctype, refused := false, false
	for {
		tok, err := d.RawToken()
		if err != nil {
			if !refused {
				return nil
			}
			if errors.Is(err, io.EOF) {
				return io.EOF
			}
			if _, ok := errors.AsType[*xml.SyntaxError](err); ok {
				return errBeforeRoot()
			}
			return err
		}
		if _, ok := tok.(xml.StartElement); ok {
			if refused {
				return errBeforeRoot()
			}
			return nil
		}
		if refused || beforeRoot(tok, &doctype) {
			continue
		}
		if !untilRoot {
			return errBeforeRoot()
		}
		refused = true
	}
}

// beforeRoot reports whether tok may come before the root element: a
// processing instruction, a comment, whitespace, or the first document type
// declaration, which it records in doctype.
func beforeRoot(tok xml.Token, doctype *bool) bool {
	switch t := tok.(type) {
	case xml.ProcInst, xml.Comment:
		return true
	case xml.CharData:
		return isXMLSpace(t)
	case xml.Directive:
		if !*doctype && isDoctype(t) {
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

// errBeforeRoot is the refusal of content before the root element.
func errBeforeRoot() error {
	return fmt.Errorf("%w: content before the root element", ErrInvalidShape)
}

// isDoctype reports whether d, the text of a `<!…>` directive, is a document
// type declaration: `DOCTYPE` and XML whitespace before the name.
func isDoctype(d xml.Directive) bool {
	rest, ok := bytes.CutPrefix(d, []byte("DOCTYPE"))
	return ok && len(rest) > 0 && isXMLSpace(rest[:1])
}
