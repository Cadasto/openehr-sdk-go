package aom14_test

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// TestREQ056_ArchetypeTermCodeDecodesFromAttribute pins that canonical XML
// reads ARCHETYPE_TERM.code from the `code` attribute the AM 1.4 ITS-XML
// schema declares (`xs:attribute name="code" use="required"`), in every
// decode mode, and still reads a `code` child element an older SDK wrote.
func TestREQ056_ArchetypeTermCodeDecodesFromAttribute(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		opts []canxml.DecoderOption
	}{
		{
			name: "schema attribute",
			doc:  `<term xmlns="http://schemas.openehr.org/v1" code="at0000"/>`,
		},
		{
			name: "schema attribute under WithStrict",
			doc:  `<term xmlns="http://schemas.openehr.org/v1" code="at0000"/>`,
			opts: []canxml.DecoderOption{canxml.WithStrict()},
		},
		{
			name: "child element an older encoder wrote",
			doc:  `<term xmlns="http://schemas.openehr.org/v1"><code>at0000</code></term>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got aom14.ArchetypeTerm
			if err := canxml.Unmarshal([]byte(tc.doc), &got, tc.opts...); err != nil {
				t.Fatalf("canxml.Unmarshal(%s) = %v; want nil", tc.doc, err)
			}
			if got.Code != "at0000" {
				t.Errorf("canxml.Unmarshal(%s): Code = %q; want %q", tc.doc, got.Code, "at0000")
			}
		})
	}
}

// TestREQ056_ArchetypeTermCodeEncodesAsAttribute pins that canonical XML
// writes ARCHETYPE_TERM.code as the `code` attribute of the term's element,
// the shape the AM 1.4 ITS-XML schema declares, and writes no `code` child.
func TestREQ056_ArchetypeTermCodeEncodesAsAttribute(t *testing.T) {
	b, err := canxml.Marshal(&aom14.ArchetypeTerm{Code: "at0000"})
	if err != nil {
		t.Fatalf("canxml.Marshal: %v", err)
	}
	dec := xml.NewDecoder(bytes.NewReader(b))
	depth := 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("reading %s: %v", b, err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		depth++
		if depth > 1 {
			t.Errorf("canxml.Marshal wrote a child element <%s> in %s; want the code as an attribute only", start.Name.Local, b)
			continue
		}
		var code string
		var found bool
		for _, a := range start.Attr {
			if a.Name.Local == "code" && a.Name.Space == "" {
				code, found = a.Value, true
			}
		}
		if !found || code != "at0000" {
			t.Errorf("canxml.Marshal root <%s> carries code attribute %q (present %v) in %s; want %q", start.Name.Local, code, found, b, "at0000")
		}
	}
	if depth == 0 {
		t.Fatalf("canxml.Marshal wrote no element: %s", b)
	}
}
