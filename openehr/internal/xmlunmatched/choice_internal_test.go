package xmlunmatched

// choice_internal_test.go: DecodeChoice, the helper a generated decoder calls
// for a member whose element takes one of several shapes, which a root schema
// names in its schema_forms (REQ-041), such as the symbol of a REQ-176 ORDINAL,
// written as a DV_CODED_TEXT or as a CODE_PHRASE. The first child element
// chooses the shape, and the chosen shape's decoder reads the element as if it
// had been called on it directly: in a tracked decode its unmatched content
// and its missing members are its own, at the element's path.

import (
	"encoding/xml"
	"errors"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/reportpath"
)

// codeShape reads as a generated decoder reads a class with two mandatory
// members, `terminology_id` and `code_string`.
type codeShape struct{ terminology, code string }

var codeShapeClass = &Class{Name: "CODE", Required: []string{"terminology_id", "code_string"}}

func (c *codeShape) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	if err := Enter(dec, start, codeShapeClass); err != nil {
		return err
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			switch LocalName(tk.Name) {
			case "terminology_id":
				err = DecodeElement(dec, &tk, &c.terminology)
			case "code_string":
				err = DecodeElement(dec, &tk, &c.code)
			default:
				err = SkipUnmatched(dec, tk)
			}
			if err != nil {
				return err
			}
		case xml.CharData:
			if err := CheckUnmatchedText(dec, tk); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		}
	}
}

// textShape reads as a generated decoder reads a class with an optional
// `value` and a mandatory `defining_code` of the code shape.
type textShape struct {
	value string
	code  codeShape
}

var textShapeClass = &Class{Name: "TEXT", Required: []string{"defining_code"}}

func (x *textShape) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	if err := Enter(dec, start, textShapeClass); err != nil {
		return err
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			switch LocalName(tk.Name) {
			case "value":
				err = DecodeElement(dec, &tk, &x.value)
			case "defining_code":
				err = dec.DecodeElement(&x.code, &tk)
			default:
				err = SkipUnmatched(dec, tk)
			}
			if err != nil {
				return err
			}
		case xml.CharData:
			if err := CheckUnmatchedText(dec, tk); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		}
	}
}

// symbolShapes are the shapes of the holder's symbol, text first.
var symbolShapes = []Shape{
	{Elements: []string{"value", "defining_code"}, New: func() any { return new(textShape) }},
	{Elements: []string{"terminology_id", "code_string"}, New: func() any { return new(codeShape) }},
}

// holder reads as a generated decoder reads a class with a mandatory `n` and
// a mandatory `symbol` whose element takes one of symbolShapes.
type holder struct {
	n      int
	symbol any
}

var holderClass = &Class{Name: "HOLDER", Required: []string{"n", "symbol"}}

func (h *holder) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	if err := Enter(dec, start, holderClass); err != nil {
		return err
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			switch LocalName(tk.Name) {
			case "n":
				err = DecodeElement(dec, &tk, &h.n)
			case "symbol":
				h.symbol, err = DecodeChoice(dec, tk, symbolShapes)
			default:
				err = SkipUnmatched(dec, tk)
			}
			if err != nil {
				return err
			}
		case xml.CharData:
			if err := CheckUnmatchedText(dec, tk); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		}
	}
}

// holderDoc returns a HOLDER document whose symbol element holds symbol, with
// an `n` after it.
func holderDoc(symbol string) string {
	return `<holder xmlns="http://schemas.openehr.org/v1"><symbol>` + symbol + `</symbol><n>1</n></holder>`
}

const (
	codeXML = `<terminology_id>local</terminology_id><code_string>at1</code_string>`
	textXML = `<value/><defining_code>` + codeXML + `</defining_code>`
)

// TestDecodeChoice_REQ041_REQ176_ChoosesByTheFirstChild pins that the first
// child element chooses the shape, in every mode, white space and a comment
// before it included, and that the chosen shape reads the whole element and
// the decode goes on after it: a text-shaped symbol, with or without its
// optional first element, and a code-shaped one.
func TestDecodeChoice_REQ041_REQ176_ChoosesByTheFirstChild(t *testing.T) {
	want := codeShape{terminology: "local", code: "at1"}
	for name, tc := range map[string]struct {
		symbol string
		text   bool
	}{
		"text":                 {textXML, true},
		"text without a value": {`<defining_code>` + codeXML + `</defining_code>`, true},
		"text after space":     {"\n  <!-- c -->\n  " + textXML + "\n", true},
		"code":                 {codeXML, false},
		"code after space":     {"\n\t" + codeXML, false},
	} {
		for mode, tracker := range documentModes() {
			t.Run(name+"/"+mode, func(t *testing.T) {
				in := holderDoc(tc.symbol)
				var h holder
				if err := Unmarshal([]byte(in), &h, tracker()); err != nil {
					t.Fatalf("Unmarshal(%q) = %v; want nil", in, err)
				}
				if h.n != 1 {
					t.Errorf("Unmarshal(%q): n = %d; want 1, the decode going on after the symbol", in, h.n)
				}
				switch s := h.symbol.(type) {
				case *textShape:
					if !tc.text || s.code != want {
						t.Errorf("Unmarshal(%q): symbol = %+v; want the %s shape holding %+v", in, s, shapeName(tc.text), want)
					}
				case *codeShape:
					if tc.text || *s != want {
						t.Errorf("Unmarshal(%q): symbol = %+v; want the %s shape holding %+v", in, s, shapeName(tc.text), want)
					}
				default:
					t.Errorf("Unmarshal(%q): symbol is %T; want the %s shape", in, h.symbol, shapeName(tc.text))
				}
			})
		}
	}
}

func shapeName(text bool) string {
	if text {
		return "text"
	}
	return "code"
}

// TestDecodeChoice_REQ041_REQ170_ShapeContentIsItsOwn pins that, in a tracked
// decode, the chosen shape's decoder reads the element as its own: an element
// no shape names, first or after the first child, is unmatched content at its
// path inside the symbol, reported or refused; an element with no child, or
// whose first child no shape names, decodes as the first shape.
func TestDecodeChoice_REQ041_REQ170_ShapeContentIsItsOwn(t *testing.T) {
	for name, tc := range map[string]struct {
		symbol string
		want   []string
	}{
		"an unknown element after the first child": {codeXML + `<bogus/>`, []string{"/holder[1]/symbol[1]/bogus[1]"}},
		"an unknown first child":                   {`<bogus/>` + textXML, []string{"/holder[1]/symbol[1]/bogus[1]"}},
		"text inside the symbol":                   {codeXML + `stray`, []string{"/holder[1]/symbol[1]/text()"}},
	} {
		t.Run(name, func(t *testing.T) {
			in := holderDoc(tc.symbol)
			var reported []string
			var h holder
			if err := Unmarshal([]byte(in), &h, NewTracker(func(p *reportpath.Node) { reported = append(reported, p.String()) }, false)); err != nil {
				t.Fatalf("Unmarshal(%q, report) = %v; want nil", in, err)
			}
			if !slices.Equal(reported, tc.want) {
				t.Errorf("Unmarshal(%q, report) reported %q; want %q", in, reported, tc.want)
			}
			if err := Unmarshal([]byte(in), new(holder), NewTracker(nil, true)); !errors.Is(err, ErrUnknownElement) {
				t.Errorf("Unmarshal(%q, strict) = %v; want ErrUnknownElement", in, err)
			}
			if err := Unmarshal([]byte(in), new(holder), nil); err != nil {
				t.Errorf("Unmarshal(%q, untracked) = %v; want nil", in, err)
			}
		})
	}
	for _, symbol := range []string{``, `<bogus/>`} {
		var h holder
		if err := Unmarshal([]byte(holderDoc(symbol)), &h, nil); err != nil {
			t.Fatalf("Unmarshal(%q) = %v; want nil", holderDoc(symbol), err)
		}
		if _, ok := h.symbol.(*textShape); !ok {
			t.Errorf("Unmarshal(%q): symbol is %T; want the first shape, *textShape", holderDoc(symbol), h.symbol)
		}
	}
}

// TestDecodeChoice_REQ041_ReportsTheShapesMissingMembers pins that the chosen
// shape describes the symbol element itself to the missing-member report: a
// mandatory member of the shape the symbol leaves out is reported at the
// symbol's pointer, not at its first child's.
func TestDecodeChoice_REQ041_ReportsTheShapesMissingMembers(t *testing.T) {
	for name, tc := range map[string]struct {
		symbol string
		want   []string
	}{
		"a code without its code string": {`<terminology_id>local</terminology_id>`, []string{"/symbol/code_string"}},
		"a text without its code":        {`<value>x</value>`, []string{"/symbol/defining_code"}},
		"an empty symbol":                {``, []string{"/symbol/defining_code"}},
		"a whole code":                   {codeXML, nil},
	} {
		t.Run(name, func(t *testing.T) {
			in := holderDoc(tc.symbol)
			var missing []string
			tr := NewTracker(nil, false)
			tr.ReportMissing(func(p *reportpath.Node) { missing = append(missing, p.String()) })
			if err := Unmarshal([]byte(in), new(holder), tr); err != nil {
				t.Fatalf("Unmarshal(%q) = %v; want nil", in, err)
			}
			if !slices.Equal(missing, tc.want) {
				t.Errorf("Unmarshal(%q) reported missing %q; want %q", in, missing, tc.want)
			}
		})
	}
}
