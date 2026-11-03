package xmlschema_test

// REQ-176 § Shapes, REQ-056: the encoder leaves out the empty children it is
// told to, writing the rest as the value's own encoder does.

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlschema"
)

// children is a value whose encoder writes the child elements it holds, each
// with its text, with one empty child of its own when nested names one, and
// with an `xsi:type` attribute when attr is set.
type children []struct {
	name, text, nested string
	attr               bool
}

// MarshalXML writes c's children inside start.
func (c children) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	for _, ch := range c {
		s := xml.StartElement{Name: xml.Name{Local: ch.name}}
		if ch.attr {
			s.Attr = []xml.Attr{{Name: xml.Name{Local: "xsi:type"}, Value: "DV_TEXT"}}
		}
		if err := e.EncodeToken(s); err != nil {
			return err
		}
		if ch.nested != "" {
			if err := e.EncodeElement("", xml.StartElement{Name: xml.Name{Local: ch.nested}}); err != nil {
				return err
			}
		}
		if ch.text != "" {
			if err := e.EncodeToken(xml.CharData(ch.text)); err != nil {
				return err
			}
		}
		if err := e.EncodeToken(s.End()); err != nil {
			return err
		}
	}
	return e.EncodeToken(start.End())
}

// TestEncodeElementOmittingEmpty pins xmlschema.EncodeElementOmittingEmpty: of the
// children of the element it writes, it leaves out each one it names that holds
// no attribute, no text and no child element, and writes everything else as
// the value's own encoder does, an `xsi:type` written literally and an
// escaped text included, compact or indented: a named child with text, with an
// attribute or with a child of its own, an empty child it does not name, and
// a grandchild of the name it names.
func TestEncodeElementOmittingEmpty(t *testing.T) {
	v := children{
		{name: "value"},
		{name: "value", text: "a < b"},
		{name: "value", attr: true},
		{name: "other"},
		{name: "value", nested: "inner"},
		{name: "wrap", nested: "value"},
	}
	const compact = `<symbol><value>a &lt; b</value><value xsi:type="DV_TEXT"></value><other></other>` +
		`<value><inner></inner></value><wrap><value></value></wrap></symbol>`
	const indented = "<symbol>\n  <value>a &lt; b</value>\n  <value xsi:type=\"DV_TEXT\"></value>\n  <other></other>\n" +
		"  <value>\n    <inner></inner>\n  </value>\n  <wrap>\n    <value></value>\n  </wrap>\n</symbol>"
	for indent, want := range map[string]string{"": compact, "  ": indented} {
		var b strings.Builder
		e := xml.NewEncoder(&b)
		e.Indent("", indent)
		if err := xmlschema.EncodeElementOmittingEmpty(e, v, xml.StartElement{Name: xml.Name{Local: "symbol"}}, "value"); err != nil {
			t.Fatalf("EncodeElementOmittingEmpty: %v", err)
		}
		if err := e.Flush(); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		if got := b.String(); got != want {
			t.Errorf("EncodeElementOmittingEmpty(indent %q) wrote\n%s\nwant\n%s", indent, got, want)
		}
	}
}
