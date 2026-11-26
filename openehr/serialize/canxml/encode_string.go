package canxml

import (
	"bytes"
	"encoding"
	"encoding/xml"
	"strings"
	"sync"
)

// EncodeString writes s as the text of one element named name, the bytes
// e.EncodeElement(s, …) writes, without the copy of s that passing it as an
// any makes. Generator-emitted MarshalXML methods call it for each member of
// a string type; consumers rarely need it. Like e.EncodeToken, it may leave
// the bytes buffered in e, so a caller outside a MarshalXML method flushes e,
// as e.Encode and e.EncodeElement do once the enclosing value is written. A
// nil e is refused with an error wrapping [typereg.ErrNilArgument] that names
// the element.
func EncodeString(e *xml.Encoder, name, s string) error {
	if e == nil {
		return nilArgument("EncodeString", "e", name)
	}
	start := xml.StartElement{Name: xml.Name{Local: name}}
	// Character data written as a token keeps a newline as it is, where
	// EncodeElement writes `&#xA;`; a long text is not worth the copy.
	if len(s) > maxPooledText || strings.IndexByte(s, '\n') >= 0 {
		return e.EncodeElement(s, start)
	}
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	if s != "" {
		buf := textBufs.Get().(*[]byte)
		text := append((*buf)[:0], s...)
		err := e.EncodeToken(xml.CharData(text))
		textBufs.Put(buf)
		if err != nil {
			return err
		}
	}
	return e.EncodeToken(start.End())
}

// maxPooledText is the longest text [EncodeString] copies into a pooled
// buffer.
const maxPooledText = 1024

// textBufs holds the buffers [EncodeString] copies a text into, each of
// [maxPooledText] bytes: the encoder takes character data as bytes, escapes
// them as it writes them and keeps none, so a buffer is free again once the
// token is written.
var textBufs = sync.Pool{New: func() any {
	b := make([]byte, 0, maxPooledText)
	return &b
}}

// EncodeText writes the text v's MarshalText method returns as one element
// named name, the bytes e.EncodeElement(v, …) writes for a value that
// marshals itself to text, without the copy of v that passing it as an any
// makes. Generator-emitted MarshalXML methods call it for each member of a
// primitive type that marshals itself to text, a Real or a Character;
// consumers rarely need it. It leaves the bytes buffered in e as
// [EncodeString] does.
//
// T's underlying type is float64 or string, so v is never a nil pointer,
// which EncodeElement writes nothing for. A MarshalXML method of T's own is
// not called: for such a type EncodeElement writes what MarshalXML writes,
// where EncodeText still writes MarshalText's text. A nil e is refused with
// an error wrapping [typereg.ErrNilArgument] that names the element.
func EncodeText[T interface {
	~float64 | ~string
	encoding.TextMarshaler
}](e *xml.Encoder, name string, v T) error {
	if e == nil {
		return nilArgument("EncodeText", "e", name)
	}
	start := xml.StartElement{Name: xml.Name{Local: name}}
	text, err := v.MarshalText()
	if err != nil {
		// EncodeElement writes the start tag before it reports the failure.
		if startErr := e.EncodeToken(start); startErr != nil {
			return startErr
		}
		return err
	}
	// EncodeElement writes a newline as `&#xA;`, which character data written
	// as a token keeps as it is; given the text itself, it writes it as it
	// writes a value that marshals itself to text.
	if bytes.IndexByte(text, '\n') >= 0 {
		return e.EncodeElement(text, start)
	}
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	if err := e.EncodeToken(xml.CharData(text)); err != nil {
		return err
	}
	return e.EncodeToken(start.End())
}
