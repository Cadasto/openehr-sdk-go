package canxml_test

import (
	"errors"
	"strings"
	"testing"

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
// whitespace after the root are accepted, as XML allows them.
//
// Can-fail control: stop reading after the root, or accept any text after it,
// and the refused cases decode without an error; refuse a comment or a
// processing instruction there and the accepted cases fail.
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
		"document type after the root":   `<!DOCTYPE extra>`,
		"unclosed second root":           `<extra>`,
		"second root after a comment":    `<!-- note --><extra/>`,
		"second copy of the root":        root,
		"element in a foreign namespace": `<x:extra xmlns:x="urn:example"/>`,
	}
	accepted := map[string]string{
		"nothing":                         "",
		"trailing whitespace":             " \n\t\r\n",
		"trailing comment":                `<!-- note -->`,
		"trailing processing instruction": `<?pi data?>`,
		"comments, PIs and whitespace":    "\n<!-- a -->\n<?pi x?>\n<!-- b -->\n",
	}

	for _, mode := range []string{"default", "report", "strict"} {
		decode := func(in string) (*rm.DVCount, []string, error) {
			var reported []string
			var opts []canxml.DecoderOption
			switch mode {
			case "report":
				opts = append(opts, canxml.WithUnmatched(func(p string) { reported = append(reported, p) }))
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
				if !errors.Is(err, canxml.ErrInvalidShape) {
					t.Fatalf("canxml.Unmarshal(root + %q) err = %v; want errors.Is(_, canxml.ErrInvalidShape)", tail, err)
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
// own, whatever follows it.
func TestREQ056_XMLOneRootAfterFailedRoot(t *testing.T) {
	in := []byte(`<dv_count xmlns="http://schemas.openehr.org/v1"><magnitude>not-a-number</magnitude></dv_count><extra/>`)
	err := canxml.Unmarshal(in, new(rm.DVCount))
	if err == nil {
		t.Fatal("canxml.Unmarshal = nil; want the root's own decode error")
	}
	if errors.Is(err, canxml.ErrInvalidShape) {
		t.Errorf("canxml.Unmarshal err = %v; want the root's own error, not the one-root refusal", err)
	}
}

// TestREQ056_XMLBeforeRoot pins the other half of the one-root rule: in every
// mode, canxml.Unmarshal refuses anything before the root element other than
// the XML declaration, whitespace, comments, processing instructions and one
// document type declaration, as malformed input. The refusal wraps
// canxml.ErrInvalidShape, carries none of the content, and is not unmatched
// content. A UTF-8 byte order mark at the very start is the document's
// encoding signature, not content, and is accepted. canxml.Decoder, which
// reads a stream of values, is unchanged.
//
// Can-fail control: drop the scan of what precedes the root and every
// refused case decodes without an error, or fails with an unclassified
// syntax error, since encoding/xml skips every token before the first start
// element; let a second document type declaration or an end element through
// and its case decodes; drop the byte order mark rule and the accepted
// cases that start with one fail.
func TestREQ056_XMLBeforeRoot(t *testing.T) {
	const root = `<dv_count xmlns="http://schemas.openehr.org/v1"><magnitude>1</magnitude></dv_count>`
	const bom = "\ufeff"

	refused := map[string]string{
		"text":                             xmlPlanted,
		"text after the XML declaration":   `<?xml version="1.0"?>` + xmlPlanted,
		"text after a comment":             `<!-- note -->` + xmlPlanted,
		"text after a PI":                  `<?pi data?>` + xmlPlanted,
		"text after the document type":     `<!DOCTYPE dv_count>` + xmlPlanted,
		"CDATA section holding text":       `<![CDATA[` + xmlPlanted + `]]>`,
		"second document type declaration": `<!DOCTYPE dv_count><!DOCTYPE extra>`,
		"other markup declaration":         `<!ELEMENT extra ANY>`,
		"stray end element":                `</extra>`,
		"byte order mark after whitespace": " " + bom,
		"text after a byte order mark":     bom + xmlPlanted,
		"no-break space":                   "\u00a0",
	}
	accepted := map[string]string{
		"nothing":                   "",
		"XML declaration":           `<?xml version="1.0" encoding="UTF-8"?>`,
		"whitespace":                " \n\t\r\n",
		"comment":                   `<!-- note -->`,
		"processing instruction":    `<?pi data?>`,
		"document type":             `<!DOCTYPE dv_count>`,
		"document type with subset": `<!DOCTYPE dv_count [<!ENTITY e "v">]>`,
		"byte order mark":           bom,
		"the whole prolog": bom + `<?xml version="1.0"?>` + "\n<!-- a -->\n" + `<!DOCTYPE dv_count SYSTEM "x.dtd">` +
			"\n<?pi x?>\n<!-- b -->\n",
	}

	for _, mode := range []string{"default", "report", "strict"} {
		decode := func(in string) (*rm.DVCount, []string, error) {
			var reported []string
			var opts []canxml.DecoderOption
			switch mode {
			case "report":
				opts = append(opts, canxml.WithUnmatched(func(p string) { reported = append(reported, p) }))
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
				if !errors.Is(err, canxml.ErrInvalidShape) {
					t.Fatalf("canxml.Unmarshal(%q + root) err = %v; want errors.Is(_, canxml.ErrInvalidShape)", head, err)
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
