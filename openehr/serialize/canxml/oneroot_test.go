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
