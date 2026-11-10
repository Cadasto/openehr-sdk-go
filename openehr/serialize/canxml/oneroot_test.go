package canxml_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// TestREQ056_XMLOneRoot pins the one-root rule of canonical XML (REQ-056): a
// document holds one root element. In every mode (the default, WithUnmatched
// and WithStrict) the decode refuses an element, or text other than
// whitespace, after the root's end, as malformed input: the error wraps
// canxml.ErrInvalidShape, carries none of the trailing content, and is not
// unmatched content, so WithUnmatched reports nothing and the refusal does not
// wrap canxml.ErrUnknownElement. Comments, processing instructions and
// whitespace after the root are accepted, as XML allows them, except a
// processing instruction whose target is xml in any letter case, which XML
// reserves for the XML declaration at the start.
//
// Can-fail control: stop reading after the root, or accept any text after it,
// and the refused cases decode without an error; accept every processing
// instruction there and the XML declaration cases decode; refuse a comment or
// a processing instruction there and the accepted cases fail.
func TestREQ056_XMLOneRoot(t *testing.T) {
	const root = `<dv_count xmlns="http://schemas.openehr.org/v1"><magnitude>1</magnitude></dv_count>`

	refused := map[string]string{
		"second root":                    `<extra/>`,
		"second root holding text":       `<extra>` + xmlPlanted + `</extra>`,
		"second root after whitespace":   "\n<extra/>",
		"trailing text":                  xmlPlanted,
		"text after a comment":           `<!-- note -->` + xmlPlanted,
		"text after a PI":                `<?pi data?>` + xmlPlanted,
		"stray end element":              `</extra>`,
		"markup it cannot read":          `<1>`,
		"document type after the root":   `<!DOCTYPE extra>`,
		"unclosed second root":           `<extra>`,
		"second root after a comment":    `<!-- note --><extra/>`,
		"second copy of the root":        root,
		"element in a foreign namespace": `<x:extra xmlns:x="urn:example"/>`,
		"XML declaration":                `<?xml version="1.0"?>`,
		"xml target in capitals":         `<?XML pi data?>`,
		"CDATA section of white space":   `<![CDATA[ ]]>`,
		"empty CDATA section":            `<![CDATA[]]>`,
		"character reference to a space": `&#32;`,
		"white space, then a reference":  "\n&#xA;",
	}
	accepted := map[string]string{
		"nothing":                         "",
		"trailing whitespace":             " \n\t\r\n",
		"trailing comment":                `<!-- note -->`,
		"trailing processing instruction": `<?pi data?>`,
		"comments, PIs and whitespace":    "\n<!-- a -->\n<?pi x?>\n<!-- b -->\n",
		"target that starts with xml":     `<?xml-stylesheet href="a.xsl"?>`,
	}

	for _, mode := range []string{"default", "report", "strict"} {
		decode := func(in string) (*rm.DVCount, []string, error) {
			var reported []string
			var opts []canxml.DecoderOption
			switch mode {
			case "report":
				opts = append(opts, canxml.WithUnmatched(func(p canxml.Path) { reported = append(reported, p.String()) }))
			case "strict":
				opts = append(opts, canxml.WithStrict())
			}
			v := new(rm.DVCount)
			err := canxml.Unmarshal([]byte(in), v, opts...)
			return v, reported, err
		}

		for name, tail := range refused {
			t.Run(mode+"/refused/"+name, func(t *testing.T) {
				_, reported, err := decode(root + tail)
				if !errors.Is(err, canxml.ErrInvalidShape) || !errors.Is(err, canxml.ErrNotWellFormed) {
					t.Fatalf("canxml.Unmarshal(root + %q) err = %v; want errors.Is(_, canxml.ErrInvalidShape) and errors.Is(_, canxml.ErrNotWellFormed)", tail, err)
				}
				if errors.Is(err, canxml.ErrUnknownElement) {
					t.Errorf("canxml.Unmarshal(root + %q) err = %v; a second root is malformed input, not unmatched content", tail, err)
				}
				for _, leak := range []string{xmlPlanted, "extra", "note", "pi data"} {
					if strings.Contains(err.Error(), leak) {
						t.Errorf("canxml.Unmarshal(root + %q) err = %q; it echoes the trailing content %q", tail, err, leak)
					}
				}
				if len(reported) != 0 {
					t.Errorf("canxml.Unmarshal(root + %q, WithUnmatched) reported %q; want nothing: the refusal is not unmatched content", tail, reported)
				}
			})
		}

		for name, tail := range accepted {
			t.Run(mode+"/accepted/"+name, func(t *testing.T) {
				v, reported, err := decode(root + tail)
				if err != nil {
					t.Fatalf("canxml.Unmarshal(root + %q) = %v; want nil", tail, err)
				}
				if v.Magnitude != 1 {
					t.Errorf("canxml.Unmarshal(root + %q): Magnitude = %d; want 1", tail, v.Magnitude)
				}
				if len(reported) != 0 {
					t.Errorf("canxml.Unmarshal(root + %q, WithUnmatched) reported %q; want nothing", tail, reported)
				}
			})
		}
	}
}

// TestREQ056_XMLOneRootAfterFailedRoot pins that a root which fails to decode
// fails as itself: the decode does not read on, so the error is the root's
// own, a DecodeError naming its magnitude, whatever follows it, and not the
// refusal of content after the root, which is no DecodeError.
func TestREQ056_XMLOneRootAfterFailedRoot(t *testing.T) {
	in := []byte(`<dv_count xmlns="http://schemas.openehr.org/v1"><magnitude>not-a-number</magnitude></dv_count><extra/>`)
	err := canxml.Unmarshal(in, new(rm.DVCount))
	if err == nil {
		t.Fatal("canxml.Unmarshal = nil; want the root's own decode error")
	}
	if de, ok := errors.AsType[*canxml.DecodeError](err); !ok || de == nil || de.Path != "/dv_count[1]/magnitude[1]" {
		t.Errorf("canxml.Unmarshal err = %v; want the root's own error, a DecodeError on /dv_count[1]/magnitude[1], not the one-root refusal", err)
	}
}

// TestREQ056_XMLOneRootAfterTheSecondDecode pins the one-root rule on a
// document that canxml.Unmarshal, given no option, decodes a second time,
// because an `xsi:type` inside it names a prefix declared on an element
// around the attribute (prefixSlot): what follows the root is judged after
// that second decode as after the first.
//
// Can-fail control: skip the check after the root once the second decode
// succeeds, and the refused cases decode without an error.
func TestREQ056_XMLOneRootAfterTheSecondDecode(t *testing.T) {
	refused := map[string]string{
		"trailing text":   xmlPlanted,
		"second root":     `<extra/>`,
		"XML declaration": `<?xml version="1.0"?>`,
		"CDATA section":   `<![CDATA[ ]]>`,
	}
	for name, tail := range refused {
		t.Run("refused/"+name, func(t *testing.T) {
			err := canxml.Unmarshal(append(prefixSlot(), tail...), new(aom14.ArchetypeSlot))
			if !errors.Is(err, canxml.ErrInvalidShape) || !errors.Is(err, canxml.ErrNotWellFormed) {
				t.Fatalf("canxml.Unmarshal(slot + %q) err = %v; want errors.Is(_, canxml.ErrInvalidShape) and errors.Is(_, canxml.ErrNotWellFormed)", tail, err)
			}
			if strings.Contains(err.Error(), xmlPlanted) {
				t.Errorf("canxml.Unmarshal(slot + %q) err = %q; it echoes the trailing content", tail, err)
			}
		})
	}
	t.Run("accepted/comment", func(t *testing.T) {
		var slot aom14.ArchetypeSlot
		if err := canxml.Unmarshal(append(prefixSlot(), "<!-- note -->\n"...), &slot); err != nil {
			t.Fatalf("canxml.Unmarshal(slot + a comment) = %v; want nil", err)
		}
		if len(slot.Includes) != 1 {
			t.Errorf("canxml.Unmarshal(slot + a comment): the slot holds %d includes; want 1", len(slot.Includes))
		}
	})
}

// TestREQ056_XMLBeforeRoot pins the other half of the one-root rule: in every
// mode, canxml.Unmarshal refuses anything before the root element other than
// the XML declaration, whitespace, comments, processing instructions and one
// document type declaration, as malformed input. The refusal wraps
// canxml.ErrInvalidShape, carries none of the content, and is not unmatched
// content. A processing instruction whose target is xml in any letter case is
// the XML declaration only at the very start, after a byte order mark at
// most: one after whitespace, a second one, one after other markup, one in
// the document type's internal subset, or one in capitals is refused, and the same bytes in a literal or a comment of that subset pass.
// A UTF-8 byte order mark at the very
// start is the document's encoding signature, not content, and is accepted.
// canxml.Decoder, which reads a stream of values, is unchanged.
//
// Can-fail control: drop the scan of what precedes the root and every
// refused case decodes without an error, or fails with an unclassified
// syntax error, since encoding/xml skips every token before the first start
// element; let a second document type declaration, an end element or a
// misplaced XML declaration through and its case decodes; let a document type
// whose subset holds one through and its case decodes; drop the byte order
// mark rule and the accepted cases that start with one fail.
func TestREQ056_XMLBeforeRoot(t *testing.T) {
	const root = `<dv_count xmlns="http://schemas.openehr.org/v1"><magnitude>1</magnitude></dv_count>`
	const bom = "\ufeff"

	refused := map[string]string{
		"text":                                                   xmlPlanted,
		"text after the XML declaration":                         `<?xml version="1.0"?>` + xmlPlanted,
		"text after a comment":                                   `<!-- note -->` + xmlPlanted,
		"text after a PI":                                        `<?pi data?>` + xmlPlanted,
		"text after the document type":                           `<!DOCTYPE dv_count>` + xmlPlanted,
		"CDATA section holding text":                             `<![CDATA[` + xmlPlanted + `]]>`,
		"second document type declaration":                       `<!DOCTYPE dv_count><!DOCTYPE extra>`,
		"other markup declaration":                               `<!ELEMENT extra ANY>`,
		"stray end element":                                      `</extra>`,
		"byte order mark after whitespace":                       " " + bom,
		"text after a byte order mark":                           bom + xmlPlanted,
		"no-break space":                                         "\u00a0",
		"second XML declaration":                                 `<?xml version="1.0"?><?xml version="1.0"?>`,
		"XML declaration after a comment":                        `<!-- note --><?xml version="1.0"?>`,
		"XML declaration after a space":                          " " + `<?xml version="1.0"?>`,
		"XML declaration after a line feed":                      "\n" + `<?xml version="1.0"?>`,
		"XML declaration after a byte order mark and whitespace": bom + "\n " + `<?xml version="1.0"?>`,
		"XML declaration after a PI":                             `<?pi data?><?xml version="1.0"?>`,
		"xml target in capitals":                                 `<?XML pi data?>`,
		"xml target in the document type":                        `<!DOCTYPE dv_count [<?xml pi data?>]>`,
		"CDATA section of white space":                           `<![CDATA[ ]]>`,
		"character reference to a space":                         `&#32;`,
		"character reference after the XML declaration":          `<?xml version="1.0"?>&#x9;`,
		"document type with no name":                             `<!DOCTYPE >`,
		"document type whose name starts with a digit":           `<!DOCTYPE 1x>`,
		"document type with no system literal":                   `<!DOCTYPE dv_count SYSTEM>`,
		"document type with a public id alone":                   `<!DOCTYPE dv_count PUBLIC "a">`,
		"document type with an unknown declaration":              `<!DOCTYPE dv_count [<!FOO>]>`,
		"document type with text in its subset":                  `<!DOCTYPE dv_count [ ` + xmlPlanted + ` ]>`,
	}
	accepted := map[string]string{
		"nothing":                   "",
		"XML declaration":           `<?xml version="1.0" encoding="UTF-8"?>`,
		"whitespace":                " \n\t\r\n",
		"comment":                   `<!-- note -->`,
		"processing instruction":    `<?pi data?>`,
		"document type":             `<!DOCTYPE dv_count>`,
		"document type with subset": `<!DOCTYPE dv_count [<!ENTITY e "v">]>`,
		"document type of every item": `<!DOCTYPE dv_count PUBLIC "-//A//B" 'x.dtd' [ <!ELEMENT dv_count ANY>` +
			` <!ATTLIST dv_count a CDATA "v>]"> <!ENTITY % p "x"> %p; <!NOTATION n SYSTEM "u"> <?pi d?> <!-- c --> ]>`,
		"xml target in a subset literal": `<!DOCTYPE dv_count [<!ENTITY e "<?xml v?>">]>`,
		"xml target in a subset comment": `<!DOCTYPE dv_count [<!-- <?xml v?> -->]>`,
		"byte order mark":                bom,
		"target that starts with xml":    `<?xml version="1.0"?><?xml-stylesheet href="a.xsl"?>`,
		"the whole prolog": bom + `<?xml version="1.0"?>` + "\n<!-- a -->\n" + `<!DOCTYPE dv_count SYSTEM "x.dtd">` +
			"\n<?pi x?>\n<!-- b -->\n",
	}

	for _, mode := range []string{"default", "report", "strict"} {
		decode := func(in string) (*rm.DVCount, []string, error) {
			var reported []string
			var opts []canxml.DecoderOption
			switch mode {
			case "report":
				opts = append(opts, canxml.WithUnmatched(func(p canxml.Path) { reported = append(reported, p.String()) }))
			case "strict":
				opts = append(opts, canxml.WithStrict())
			}
			v := new(rm.DVCount)
			err := canxml.Unmarshal([]byte(in), v, opts...)
			return v, reported, err
		}

		for name, head := range refused {
			t.Run(mode+"/refused/"+name, func(t *testing.T) {
				_, reported, err := decode(head + root)
				if !errors.Is(err, canxml.ErrInvalidShape) || !errors.Is(err, canxml.ErrNotWellFormed) {
					t.Fatalf("canxml.Unmarshal(%q + root) err = %v; want errors.Is(_, canxml.ErrInvalidShape) and errors.Is(_, canxml.ErrNotWellFormed)", head, err)
				}
				if errors.Is(err, canxml.ErrUnknownElement) {
					t.Errorf("canxml.Unmarshal(%q + root) err = %v; content before the root is malformed input, not unmatched content", head, err)
				}
				for _, leak := range []string{xmlPlanted, "extra", "note", "pi data"} {
					if strings.Contains(err.Error(), leak) {
						t.Errorf("canxml.Unmarshal(%q + root) err = %q; it echoes the content %q", head, err, leak)
					}
				}
				if len(reported) != 0 {
					t.Errorf("canxml.Unmarshal(%q + root, WithUnmatched) reported %q; want nothing: the refusal is not unmatched content", head, reported)
				}
			})
		}

		for name, head := range accepted {
			t.Run(mode+"/accepted/"+name, func(t *testing.T) {
				v, reported, err := decode(head + root)
				if err != nil {
					t.Fatalf("canxml.Unmarshal(%q + root) = %v; want nil", head, err)
				}
				if v.Magnitude != 1 {
					t.Errorf("canxml.Unmarshal(%q + root): Magnitude = %d; want 1", head, v.Magnitude)
				}
				if len(reported) != 0 {
					t.Errorf("canxml.Unmarshal(%q + root, WithUnmatched) reported %q; want nothing", head, reported)
				}
			})
		}
	}

	t.Run("the stream decoder is unchanged", func(t *testing.T) {
		v := new(rm.DVCount)
		if err := canxml.NewDecoder(strings.NewReader(xmlPlanted + root)).Decode(v); err != nil {
			t.Fatalf("canxml.NewDecoder(text + root).Decode = %v; want nil: Decoder reads a stream of values, not a document", err)
		}
		if v.Magnitude != 1 {
			t.Errorf("canxml.NewDecoder(text + root).Decode: Magnitude = %d; want 1", v.Magnitude)
		}
	})
}

// TestREQ056_XMLTargetInsideTheRoot pins that XML reserves the processing
// instruction target xml, in any letter case, for the declaration at the start
// of a document, inside the root as before and after it (REQ-056):
// canxml.Unmarshal refuses one inside an element, or inside a document type
// declaration there, in every mode, and so does a canxml.Decoder given
// any option, canxml.WithMaxDepth included, with an error
// that wraps canxml.ErrInvalidShape, is no unmatched content and carries none
// of the instruction. A target that only starts with xml passes, and so do the
// same bytes inside a comment or a CDATA section, which are no instruction.
//
// Can-fail control: let the tracked token source pass such an instruction and
// every refused row decodes; let Unmarshal given no option skip its check of
// the bytes and that mode decodes them.
func TestREQ056_XMLTargetInsideTheRoot(t *testing.T) {
	const ns = `xmlns="http://schemas.openehr.org/v1"`
	refused := map[string]string{
		"declaration first in the root": `<dv_text ` + ns + `><?xml version="1.0"?><value>a</value></dv_text>`,
		"inside a primitive's text":     `<dv_text ` + ns + `><value>a<?xml version="1.0"?>b</value></dv_text>`,
		"after a member":                `<dv_text ` + ns + `><value>a</value><?xml version="1.0"?></dv_text>`,
		"xml target in capitals":        `<dv_text ` + ns + `><value>a</value><?XML pi data?></dv_text>`,
		"bare xml target":               `<dv_text ` + ns + `><value>a</value><?xml?></dv_text>`,
		"in a document type":            `<dv_text ` + ns + `><!DOCTYPE x [<?xml pi data?>]><value>a</value></dv_text>`,
	}
	accepted := map[string]struct{ in, value string }{
		"target that starts with xml": {`<dv_text ` + ns + `><?xml-stylesheet href="a.xsl"?><value>a</value></dv_text>`, "a"},
		"xml target in a comment":     {`<dv_text ` + ns + `><!-- <?xml version="1.0"?> --><value>a</value></dv_text>`, "a"},
		"xml target in CDATA":         {`<?xml version="1.0"?><dv_text ` + ns + `><value><![CDATA[<?xml version="1.0"?>]]></value></dv_text>`, `<?xml version="1.0"?>`},
	}
	modes := map[string]func(in string, v any) error{
		"Unmarshal": func(in string, v any) error { return canxml.Unmarshal([]byte(in), v) },
		"Unmarshal WithUnmatched": func(in string, v any) error {
			return canxml.Unmarshal([]byte(in), v, canxml.WithUnmatched(func(canxml.Path) {}))
		},
		"Unmarshal WithStrict": func(in string, v any) error { return canxml.Unmarshal([]byte(in), v, canxml.WithStrict()) },
		"Unmarshal WithMissing": func(in string, v any) error {
			return canxml.Unmarshal([]byte(in), v, canxml.WithMissing(func(canxml.Pointer) {}))
		},
		"Unmarshal WithMaxDepth": func(in string, v any) error { return canxml.Unmarshal([]byte(in), v, canxml.WithMaxDepth(100)) },
		"Decoder WithMaxDepth": func(in string, v any) error {
			return canxml.NewDecoder(strings.NewReader(in), canxml.WithMaxDepth(100)).Decode(v)
		},
		"Decoder WithStrict": func(in string, v any) error {
			return canxml.NewDecoder(strings.NewReader(in), canxml.WithStrict()).Decode(v)
		},
		"Decoder WithUnmatched": func(in string, v any) error {
			return canxml.NewDecoder(strings.NewReader(in), canxml.WithUnmatched(func(canxml.Path) {})).Decode(v)
		},
	}
	for mode, decode := range modes {
		for name, in := range refused {
			t.Run(mode+"/refused/"+name, func(t *testing.T) {
				err := decode(in, new(rm.DVText))
				if !errors.Is(err, canxml.ErrInvalidShape) || !errors.Is(err, canxml.ErrNotWellFormed) || errors.Is(err, canxml.ErrUnknownElement) {
					t.Fatalf("%s(%s) = %v; want an error that wraps canxml.ErrInvalidShape and canxml.ErrNotWellFormed, and not canxml.ErrUnknownElement", mode, in, err)
				}
				for _, leak := range []string{"version", "pi data"} {
					if strings.Contains(err.Error(), leak) {
						t.Errorf("%s(%s) err = %q; it echoes the instruction's %q", mode, in, err, leak)
					}
				}
			})
		}
		for name, c := range accepted {
			t.Run(mode+"/accepted/"+name, func(t *testing.T) {
				var v rm.DVText
				if err := decode(c.in, &v); err != nil {
					t.Fatalf("%s(%s) = %v; want nil", mode, c.in, err)
				}
				if v.Value != c.value {
					t.Errorf("%s(%s) decoded value %q; want %q", mode, c.in, v.Value, c.value)
				}
			})
		}
	}
}

// TestREQ056_XMLDeclarationInsideTheRoot pins that XML allows a markup
// declaration (`<!…>` other than a comment or a CDATA section), such as a
// document type declaration, only before the root element (REQ-056):
// canxml.Unmarshal refuses one inside an element, in a primitive's text or in
// content it skips as unmatched, in every mode, and so does a
// canxml.Decoder given any option, with an error that wraps
// canxml.ErrInvalidShape and canxml.ErrNotWellFormed, is no unmatched content
// and carries none of the declaration, whatever comes before the root. The
// same bytes in a comment or a CDATA section pass.
//
// Can-fail control: let the tracked token source pass a declaration and every
// refused row decodes; let Unmarshal given no option skip its scan of the
// bytes and that mode decodes them; start that scan past the root's start
// tag and the rows with a prolog decode in that mode.
func TestREQ056_XMLDeclarationInsideTheRoot(t *testing.T) {
	const ns = `xmlns="http://schemas.openehr.org/v1"`
	refused := map[string]string{
		"document type first in the root":   `<dv_text ` + ns + `><!DOCTYPE ` + xmlPlanted + `><value>a</value></dv_text>`,
		"inside a primitive's text":         `<dv_text ` + ns + `><value>a<!DOCTYPE ` + xmlPlanted + `>b</value></dv_text>`,
		"after a member":                    `<dv_text ` + ns + `><value>a</value><!ELEMENT ` + xmlPlanted + ` ANY></dv_text>`,
		"inside unmatched content":          `<dv_text ` + ns + `><value>a</value><extra><!ENTITY ` + xmlPlanted + ` "v"></extra></dv_text>`,
		"after a comment that holds <!":     `<dv_text ` + ns + `><!-- <! --><value>a</value><!DOCTYPE ` + xmlPlanted + `></dv_text>`,
		"a well-formed one with a subset":   `<dv_text ` + ns + `><!DOCTYPE dv_text [<!ENTITY ` + xmlPlanted + ` "v">]><value>a</value></dv_text>`,
		"after an instruction holding <!--": `<dv_text ` + ns + `><?pi <!-- ?><!DOCTYPE ` + xmlPlanted + `><value>a</value></dv_text>`,
		"after an instruction holding a CDATA opener": `<dv_text ` + ns + `><value>a<?pi <![CDATA[ ?><!ENTITY ` + xmlPlanted +
			` "v"></value></dv_text>`,
		"in a root after the XML declaration": `<?xml version="1.0"?><dv_text ` + ns + `><!DOCTYPE ` + xmlPlanted +
			`><value>a</value></dv_text>`,
		"in a root after a byte order mark and the XML declaration": "\ufeff" + `<?xml version="1.0"?><dv_text ` + ns +
			`><value>a</value><!DOCTYPE ` + xmlPlanted + `></dv_text>`,
		"in a root after a comment":       `<!-- c --><dv_text ` + ns + `><!DOCTYPE ` + xmlPlanted + `><value>a</value></dv_text>`,
		"in a root after a document type": `<!DOCTYPE dv_text><dv_text ` + ns + `><value>a<!DOCTYPE ` + xmlPlanted + `></value></dv_text>`,
		"in a root after whitespace":      "\n " + `<dv_text ` + ns + `><!DOCTYPE ` + xmlPlanted + `><value>a</value></dv_text>`,
	}
	accepted := map[string]struct{ in, value string }{
		"declaration in a comment":      {`<dv_text ` + ns + `><!-- <!DOCTYPE x> --><value>a</value></dv_text>`, "a"},
		"declaration in CDATA":          {`<dv_text ` + ns + `><value><![CDATA[<!DOCTYPE x>]]></value></dv_text>`, "<!DOCTYPE x>"},
		"declaration in an instruction": {`<dv_text ` + ns + `><?pi <!DOCTYPE x> ?><value>a</value></dv_text>`, "a"},
		"document type before":          {`<!DOCTYPE dv_text><dv_text ` + ns + `><value>a</value></dv_text>`, "a"},
	}
	modes := map[string]func(in string, v any) error{
		"Unmarshal": func(in string, v any) error { return canxml.Unmarshal([]byte(in), v) },
		"Unmarshal WithUnmatched": func(in string, v any) error {
			return canxml.Unmarshal([]byte(in), v, canxml.WithUnmatched(func(canxml.Path) {}))
		},
		"Unmarshal WithStrict": func(in string, v any) error { return canxml.Unmarshal([]byte(in), v, canxml.WithStrict()) },
		"Decoder WithMaxDepth": func(in string, v any) error {
			return canxml.NewDecoder(strings.NewReader(in), canxml.WithMaxDepth(100)).Decode(v)
		},
		"Decoder WithUnmatched": func(in string, v any) error {
			return canxml.NewDecoder(strings.NewReader(in), canxml.WithUnmatched(func(canxml.Path) {})).Decode(v)
		},
	}
	for mode, decode := range modes {
		for name, in := range refused {
			t.Run(mode+"/refused/"+name, func(t *testing.T) {
				if mode == "Unmarshal WithStrict" && name == "inside unmatched content" {
					t.Skip("strict refuses the unmatched element at its start tag, before what it holds")
				}
				err := decode(in, new(rm.DVText))
				if !errors.Is(err, canxml.ErrInvalidShape) || !errors.Is(err, canxml.ErrNotWellFormed) || errors.Is(err, canxml.ErrUnknownElement) {
					t.Fatalf("%s(%s) = %v; want an error that wraps canxml.ErrInvalidShape and canxml.ErrNotWellFormed, and not canxml.ErrUnknownElement", mode, in, err)
				}
				if strings.Contains(err.Error(), xmlPlanted) {
					t.Errorf("%s(%s) err = %q; it echoes the declaration", mode, in, err)
				}
			})
		}
		for name, c := range accepted {
			t.Run(mode+"/accepted/"+name, func(t *testing.T) {
				var v rm.DVText
				if err := decode(c.in, &v); err != nil {
					t.Fatalf("%s(%s) = %v; want nil", mode, c.in, err)
				}
				if v.Value != c.value {
					t.Errorf("%s(%s) decoded value %q; want %q", mode, c.in, v.Value, c.value)
				}
			})
		}
	}
}
