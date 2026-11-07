package xmlunmatched

// document_internal_test.go: the whole-document decode, Unmarshal, which
// canxml.Unmarshal drives with the tracker its options build, or none. It
// holds the input to one root element (REQ-056's one-root rule, carried by
// canxml.Unmarshal's godoc), names a primitive failure by its path from the
// root in both modes (REQ-052, REQ-056) and applies the tracker's policy
// (REQ-170, REQ-175). canxml's own tests drive every prolog and tail form
// through canxml.Unmarshal; this file pins the entry point itself.

import (
	"bufio"
	"encoding/xml"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/reportpath"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// counted reads as a generated decoder reads a class with one mandatory
// member, `n`, a whole number: text that is no number is a primitive failure.
type counted struct{ n int }

var countedClass = &Class{Name: "COUNTED", Required: []string{"n"}}

func (c *counted) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	if err := Enter(dec, start, countedClass); err != nil {
		return err
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			if LocalName(tk.Name) == "n" {
				if err := DecodeElement(dec, &tk, &c.n); err != nil {
					return err
				}
				continue
			}
			if err := SkipUnmatched(dec, tk); err != nil {
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

// isNumError reports whether err's chain holds a *strconv.NumError, the
// conversion's own refusal of text that is no number.
func isNumError(err error) bool {
	ne, ok := errors.AsType[*strconv.NumError](err)
	return ok && ne != nil
}

// documentModes are the ways canxml.Unmarshal calls Unmarshal: untracked,
// tracked with a reporting policy, and tracked with a strict one.
func documentModes() map[string]func() *Tracker {
	return map[string]func() *Tracker{
		"untracked": func() *Tracker { return nil },
		"report":    func() *Tracker { return NewTracker(func(*reportpath.Node) {}, false) },
		"strict":    func() *Tracker { return NewTracker(nil, true) },
	}
}

// TestREQ056_UnmarshalOneRoot pins that Unmarshal, tracked or not, decodes
// one document: the XML declaration, comments, processing instructions,
// whitespace, one document type declaration and a leading byte order mark
// pass before the root, and comments, processing instructions and
// whitespace after it; anything else on either side is refused with an error
// that wraps ErrInvalidShape, names which side, and carries none of the
// content. A processing instruction whose target is xml, in any letter case,
// is refused on either side unless it is the XML declaration at the start;
// a target that only starts with those letters passes. The root's value is
// decoded either way.
func TestREQ056_UnmarshalOneRoot(t *testing.T) {
	const root = `<counted xmlns="http://schemas.openehr.org/v1"><n>1</n></counted>`
	const planted = "planted-secret-value"
	accepted := map[string]string{
		"root alone":  root,
		"full prolog": "\ufeff" + `<?xml version="1.0"?>` + "\n<!-- a -->\n<!DOCTYPE counted>\n<?pi x?>\n" + root,
		"tail":        root + "\n<!-- b -->\n<?pi y?>\n ",
		"targets that only start with xml": `<?xml version="1.0"?><?xml-stylesheet href="a.xsl"?><?xmlfoo x?>` + root +
			`<?xml-stylesheet href="b.xsl"?><?XMLbar y?>`,
	}
	refused := map[string]struct{ in, side string }{
		"text before the root":             {planted + root, "content before the root element"},
		"second document type declaration": {`<!DOCTYPE counted><!DOCTYPE extra>` + root, "content before the root element"},
		"stray end element before":         {`</extra>` + root, "content before the root element"},
		"second root":                      {root + `<extra>` + planted + `</extra>`, "content after the root element"},
		"text after the root":              {root + planted, "content after the root element"},
		"document type after the root":     {root + `<!DOCTYPE extra>`, "content after the root element"},
		"second XML declaration":           {`<?xml version="1.0"?><?xml version="1.0" extra="` + planted + `"?>` + root, "content before the root element"},
		"XML declaration after a comment":  {`<!-- a --><?xml version="1.0"?>` + root, "content before the root element"},
		"xml target in capitals before":    {`<?XML ` + planted + `?>` + root, "content before the root element"},
		"XML declaration after the root":   {root + `<?xml version="1.0"?>`, "content after the root element"},
		"xml target in mixed case after":   {root + `<?XmL ` + planted + `?>`, "content after the root element"},
	}
	for mode, tracker := range documentModes() {
		for name, in := range accepted {
			t.Run(mode+"/accepted/"+name, func(t *testing.T) {
				var c counted
				if err := Unmarshal([]byte(in), &c, tracker()); err != nil {
					t.Fatalf("Unmarshal(%q) = %v; want nil", in, err)
				}
				if c.n != 1 {
					t.Errorf("Unmarshal(%q): n = %d; want 1, the root's value", in, c.n)
				}
			})
		}
		for name, tc := range refused {
			t.Run(mode+"/refused/"+name, func(t *testing.T) {
				err := Unmarshal([]byte(tc.in), new(counted), tracker())
				if !errors.Is(err, ErrInvalidShape) {
					t.Fatalf("Unmarshal(%q) err = %v; want errors.Is(_, ErrInvalidShape)", tc.in, err)
				}
				if !strings.Contains(err.Error(), tc.side) {
					t.Errorf("Unmarshal(%q) err = %q; want it to say %q", tc.in, err, tc.side)
				}
				for _, leak := range []string{planted, "extra"} {
					if strings.Contains(err.Error(), leak) {
						t.Errorf("Unmarshal(%q) err = %q; it echoes the content %q", tc.in, err, leak)
					}
				}
			})
		}
	}
}

// TestREQ056_ReadPrologXMLTarget pins that ReadProlog holds a processing
// instruction whose target is xml to the rule Unmarshal holds it to: the XML
// declaration passes as the first token, after a byte order mark too, and
// one anywhere else before the root, the document type's internal subset
// included, or one whose target is xml in another letter case, is refused with
// an error that wraps ErrInvalidShape and carries none of it; the same bytes in
// a literal of that subset pass.
func TestREQ056_ReadPrologXMLTarget(t *testing.T) {
	const root = `<counted><n>1</n></counted>`
	const planted = "planted-secret-value"
	accepted := map[string]string{
		"XML declaration":             `<?xml version="1.0"?>` + root,
		"after a byte order mark":     "\ufeff" + `<?xml version="1.0"?>` + root,
		"target that starts with xml": `<?xml version="1.0"?><?xml-stylesheet href="a.xsl"?>` + root,
		"xml target in a literal":     `<!DOCTYPE counted [<!ENTITY e "<?xml v?>">]>` + root,
	}
	refused := map[string]string{
		"second XML declaration":          `<?xml version="1.0"?><?xml version="1.0" extra="` + planted + `"?>` + root,
		"XML declaration after a comment": `<!-- a --><?xml version="1.0"?>` + root,
		"xml target in capitals":          `<?XML ` + planted + `?>` + root,
		"xml target in the document type": `<!DOCTYPE counted [<?xml ` + planted + `?>]>` + root,
	}
	for name, in := range accepted {
		t.Run("accepted/"+name, func(t *testing.T) {
			if _, err := ReadProlog(bufio.NewReader(strings.NewReader(in))); err != nil {
				t.Errorf("ReadProlog(%q) = %v; want nil", in, err)
			}
		})
	}
	for name, in := range refused {
		t.Run("refused/"+name, func(t *testing.T) {
			_, err := ReadProlog(bufio.NewReader(strings.NewReader(in)))
			if !errors.Is(err, ErrInvalidShape) {
				t.Fatalf("ReadProlog(%q) err = %v; want errors.Is(_, ErrInvalidShape)", in, err)
			}
			if strings.Contains(err.Error(), planted) {
				t.Errorf("ReadProlog(%q) err = %q; it echoes the content", in, err)
			}
		})
	}
}

// TestREQ056_XMLTargetPastStart pins what makes Unmarshal given no option
// decode a document a second time, through a decoder that refuses a
// processing instruction whose target is xml inside the root: such bytes past
// the start, in a comment too, but not the XML declaration, after a byte order
// mark as well, and not a target that only starts with xml. A declaration
// after whitespace is past the start; Unmarshal's prolog check refuses such a
// document before it gets here.
//
// Can-fail control: skip the whitespace before the declaration, as the check
// once did, and the rows with the declaration after whitespace report false.
func TestREQ056_XMLTargetPastStart(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		want       bool
	}{
		{"empty", "", false},
		{"whitespace only", " \n", false},
		{"no declaration", `<a/>`, false},
		{"the declaration", `<?xml version="1.0"?><a/>`, false},
		{"the declaration after a byte order mark", "\ufeff" + `<?xml version="1.0"?><a/>`, false},
		{"a declaration after whitespace", "\n " + `<?xml version="1.0"?><a/>`, true},
		{"a declaration after a byte order mark and whitespace", "\ufeff\n" + `<?xml version="1.0"?><a/>`, true},
		{"a target that starts with xml", `<?xml version="1.0"?><a><?xml-stylesheet href="a.xsl"?></a>`, false},
		{"one inside the root", `<a><?xml version="1.0"?></a>`, true},
		{"one after the declaration and whitespace", "\n" + `<?xml version="1.0"?><a><?XML?></a>`, true},
		{"one in a comment", `<a><!-- <?xml v?> --></a>`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := xmlTargetPastStart([]byte(tc.data)); got != tc.want {
				t.Errorf("xmlTargetPastStart(%q) = %t; want %t", tc.data, got, tc.want)
			}
		})
	}
}

// TestREQ052_UnmarshalLocatesAPrimitiveFailure pins that a primitive failure
// comes back on its own, naming the element by its path from the root of the
// value, in both modes: a tracked decode knows the path, and an untracked one
// decodes the input a second time to find it. The failure wraps
// ErrInvalidShape, keeps the conversion's own error reachable and carries
// none of the text; and a root that fails to decode fails as itself, whatever
// follows it, since the decode does not read on.
func TestREQ052_UnmarshalLocatesAPrimitiveFailure(t *testing.T) {
	const planted = "planted-secret-value"
	for _, in := range []string{
		`<counted xmlns="http://schemas.openehr.org/v1"><n>` + planted + `</n></counted>`,
		`<counted xmlns="http://schemas.openehr.org/v1"><n>` + planted + `</n></counted><extra/>`,
	} {
		for mode, tracker := range documentModes() {
			t.Run(mode, func(t *testing.T) {
				err := Unmarshal([]byte(in), new(counted), tracker())
				de, ok := errors.AsType[*typereg.DecodeError](err)
				if !ok || de == nil {
					t.Fatalf("Unmarshal(%q) err = %v; want a *typereg.DecodeError on its own", in, err)
				}
				if de.Path != "/counted[1]/n[1]" {
					t.Errorf("Unmarshal(%q) err.Path = %q; want %q, the element's path from the root", in, de.Path, "/counted[1]/n[1]")
				}
				if !IsPrimitiveFailure(err) || !errors.Is(err, ErrInvalidShape) || !isNumError(err) {
					t.Errorf("Unmarshal(%q) err = %v; want a primitive failure wrapping ErrInvalidShape and the *strconv.NumError", in, err)
				}
				if strings.Contains(err.Error(), planted) {
					t.Errorf("Unmarshal(%q) err = %q; it carries the element's text", in, err)
				}
			})
		}
	}
}

// TestREQ175_UnmarshalAppliesTheTracker pins that the tracker reaches the
// decode: its unmatched-content policy reports and refuses, and its
// missing-member hook reports, through Unmarshal as through Tracker.Decode.
func TestREQ175_UnmarshalAppliesTheTracker(t *testing.T) {
	const in = `<counted xmlns="http://schemas.openehr.org/v1" x="1"><extra/></counted>`
	var unmatched, missing []string
	tr := NewTracker(func(p *reportpath.Node) { unmatched = append(unmatched, p.String()) }, false)
	tr.ReportMissing(func(p *reportpath.Node) { missing = append(missing, p.String()) })
	if err := Unmarshal([]byte(in), new(counted), tr); err != nil {
		t.Fatalf("Unmarshal(report) = %v; want nil", err)
	}
	if want := []string{"/counted[1]/@x", "/counted[1]/extra[1]"}; !slices.Equal(unmatched, want) {
		t.Errorf("unmatched = %q; want %q", unmatched, want)
	}
	if want := []string{"/n"}; !slices.Equal(missing, want) {
		t.Errorf("missing = %q; want %q", missing, want)
	}
	if err := Unmarshal([]byte(in), new(counted), NewTracker(nil, true)); !errors.Is(err, ErrUnknownElement) {
		t.Errorf("Unmarshal(strict) = %v; want errors.Is(_, ErrUnknownElement)", err)
	}
}
