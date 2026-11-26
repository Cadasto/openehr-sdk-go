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
// a string type; consumers rarely need it.
func EncodeString(e *xml.Encoder, name, s string) error {
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

// EncodeText writes the text v marshals to as one element named name, the
// bytes e.EncodeElement(v, …) writes, without the copy of v that passing it
// as an any makes. Generator-emitted MarshalXML methods call it for each
// member of a primitive type that marshals itself to text, such as a Real;
// consumers rarely need it.
func EncodeText[T encoding.TextMarshaler](e *xml.Encoder, name string, v T) error {
	start := xml.StartElement{Name: xml.Name{Local: name}}
	text, err := v.MarshalText()
	// EncodeElement writes the start tag before it reports the failure, and
	// a newline as `&#xA;`, which character data written as a token keeps as
	// it is.
	if err != nil || bytes.IndexByte(text, '\n') >= 0 {
		return e.EncodeElement(v, start)
	}
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	if err := e.EncodeToken(xml.CharData(text)); err != nil {
		return err
	}
	return e.EncodeToken(start.End())
}
