package xmlunmatched

import (
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
// documentation). A tracked decode knows the path; an untracked one finds it
// by decoding data a second time, once the first decode has failed so, which
// a decode that succeeds never pays for. Other XML syntax errors come back
// unchanged from encoding/xml.
func Unmarshal(data []byte, v any, t *Tracker) error {
	if err := checkProlog(data); err != nil {
		return err
	}
	if t == nil {
		dec := xml.NewDecoder(bytes.NewReader(data))
		if err := dec.Decode(v); err != nil {
			if IsPrimitiveFailure(err) {
				return locate(data, v, err)
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

// locate decodes data into v once more after an untracked decode failed
// with err, a primitive failure that names the element alone. The second
// decode reads through a tracker that follows the open elements and applies
// no policy, so it fails at the same element and names it by its path from
// the root of the value. locate returns that error, or err should the second
// decode not fail.
func locate(data []byte, v any, err error) error {
	t := NewTracker(nil, false)
	if located := t.Decode(t.NewDecoder(bytes.NewReader(data)), v); located != nil {
		return located
	}
	return err
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
	d := xml.NewDecoder(bytes.NewReader(data))
	doctype := false
	for {
		tok, err := d.RawToken()
		if err != nil {
			return nil
		}
		switch t := tok.(type) {
		case xml.StartElement:
			return nil
		case xml.ProcInst, xml.Comment:
			continue
		case xml.CharData:
			if isXMLSpace(t) {
				continue
			}
		case xml.Directive:
			if !doctype && isDoctype(t) {
				doctype = true
				continue
			}
		}
		return errBeforeRoot()
	}
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
