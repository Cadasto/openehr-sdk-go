package xmlschema

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
)

// EncodeElementOmittingEmpty writes v to e as e.EncodeElement(v, start) does,
// but leaves out each child element of start named in names that is empty:
// one that holds no attribute, no text and no child element. The other
// content is written as v's own encoder writes it.
//
// The generated encoders call it where the schema of the element v is written
// as makes a child optional that v's own encoder writes always, such as the
// `value` of the DV_CODED_TEXT shape of an AOM 1.4 ORDINAL symbol, which the
// AM Release-1.4 schemas make optional and an RM DV_CODED_TEXT always holds.
func EncodeElementOmittingEmpty(e *xml.Encoder, v any, start xml.StartElement, names ...string) error {
	var buf bytes.Buffer
	scratch := xml.NewEncoder(&buf)
	if err := scratch.EncodeElement(v, start); err != nil {
		return err
	}
	if err := scratch.Flush(); err != nil {
		return fmt.Errorf("canxml: flush: %w", err)
	}
	// The scratch encoding is read back token by token and written to e,
	// holding back a named child of start until it shows content.
	d := xml.NewDecoder(&buf)
	var held []xml.Token
	depth := 0 // the elements open after the token read
	for {
		tok, err := d.RawToken()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("canxml: re-reading %s: %w", start.Name.Local, err)
		}
		tok = withLiteralNames(xml.CopyToken(tok))
		switch tk := tok.(type) {
		case xml.StartElement:
			depth++
			if held == nil && depth == 2 && len(tk.Attr) == 0 && slices.Contains(names, tk.Name.Local) {
				held = []xml.Token{tk}
				continue
			}
		case xml.EndElement:
			depth--
			if held != nil && depth == 1 {
				held = nil // the named child closed with nothing in it
				continue
			}
		}
		for _, h := range held {
			if err := e.EncodeToken(h); err != nil {
				return err
			}
		}
		held = nil
		if err := e.EncodeToken(tok); err != nil {
			return err
		}
	}
}

// withLiteralNames returns tok with each prefixed name, as RawToken reads it,
// written back as the literal `prefix:local` the generated encoders write,
// such as `xsi:type`, so writing tok again declares no namespace of its own.
func withLiteralNames(tok xml.Token) xml.Token {
	literal := func(n xml.Name) xml.Name {
		if n.Space == "" {
			return n
		}
		return xml.Name{Local: n.Space + ":" + n.Local}
	}
	switch tk := tok.(type) {
	case xml.StartElement:
		tk.Name = literal(tk.Name)
		for i := range tk.Attr {
			tk.Attr[i].Name = literal(tk.Attr[i].Name)
		}
		return tk
	case xml.EndElement:
		tk.Name = literal(tk.Name)
		return tk
	}
	return tok
}
