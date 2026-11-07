package canxml_test

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/decodeopts"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// observedDoc is a document with markup before its root and inside it: a
// declaration, comments, a processing instruction and a CDATA section.
const observedDoc = `<?xml version="1.0"?><!-- before --><items ` + ucNS + ` xsi:type="ELEMENT" archetype_node_id="at0004">` +
	`<name><value>weight</value></name><!-- inside --><?pi data?>` +
	`<value xsi:type="DV_QUANTITY"><magnitude>80.5</magnitude><units><![CDATA[kg]]></units></value></items>`

// rootTokens returns the tokens of toks from the start of the first element
// to its end, and how many elements toks opens outside any element.
func rootTokens(toks []xml.Token) (root []xml.Token, roots int) {
	depth := 0
	for _, tok := range toks {
		_, start := tok.(xml.StartElement)
		if start && depth == 0 {
			roots++
		}
		if roots == 1 && (depth > 0 || start) {
			root = append(root, tok)
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return root, roots
}

// plainTokens returns the tokens encoding/xml reads from doc.
func plainTokens(t *testing.T, doc string) []xml.Token {
	t.Helper()
	var toks []xml.Token
	dec := xml.NewDecoder(bytes.NewReader([]byte(doc)))
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return toks
		}
		if err != nil {
			t.Fatalf("encoding/xml Token: %v", err)
		}
		toks = append(toks, xml.CopyToken(tok))
	}
}

// TestDecodePassesEachTokenToTheObserverOnce pins that a decode given the
// token observer passes it every token of the root element, once and in
// order, as encoding/xml reads them, through Decoder.Decode and Unmarshal
// alike.
//
// Can-fail control: let Unmarshal decode without a tracker when only the
// observer is set, and its case observes no token.
func TestDecodePassesEachTokenToTheObserverOnce(t *testing.T) {
	t.Parallel()
	want, _ := rootTokens(plainTokens(t, observedDoc))
	if len(want) < 10 {
		t.Fatalf("the plain reading of the root holds %d tokens, want the whole element", len(want))
	}
	decodes := map[string]func(opt canxml.DecoderOption) error{
		"Decoder.Decode": func(opt canxml.DecoderOption) error {
			return canxml.NewDecoder(bytes.NewReader([]byte(observedDoc)), opt).Decode(new(rm.Element))
		},
		"Unmarshal": func(opt canxml.DecoderOption) error {
			return canxml.Unmarshal([]byte(observedDoc), new(rm.Element), opt)
		},
	}
	for name, decode := range decodes {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var observed []xml.Token
			opt, ok := decodeopts.Observer(func(tok xml.Token) {
				observed = append(observed, xml.CopyToken(tok))
			}).(canxml.DecoderOption)
			if !ok {
				t.Fatal("decodeopts.Observer gave no canxml.DecoderOption: canxml registered no observer")
			}
			if err := decode(opt); err != nil {
				t.Fatalf("decode: %v", err)
			}
			got, roots := rootTokens(observed)
			if roots != 1 {
				t.Errorf("the observer saw the root start %d times, want 1", roots)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("the observer saw the root as\n%#v\nwant, as encoding/xml reads it,\n%#v", got, want)
			}
		})
	}
}
