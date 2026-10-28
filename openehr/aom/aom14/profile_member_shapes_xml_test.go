package aom14_test

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// profileChild returns a C_SINGLE_ATTRIBUTE document whose one child is
// written `xsi:type` schemaType and holds body after the members every
// C_OBJECT requires, so that a decode with canxml.WithMissing reports
// nothing outside body.
func profileChild(schemaType, rmType, body string) string {
	return `<c_single_attribute xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<rm_attribute_name>value</rm_attribute_name><existence><lower>1</lower><upper>1</upper></existence>` +
		`<children xsi:type="` + schemaType + `"><rm_type_name>` + rmType + `</rm_type_name>` +
		`<occurrences><lower>1</lower><upper>1</upper></occurrences><node_id>at0001</node_id>` + body + `</children>` +
		`</c_single_attribute>`
}

// decodeMode is one way of decoding canonical XML, with what it collects.
type decodeMode struct {
	name string
	opts func(unmatched, missing *[]string) []canxml.DecoderOption
}

// decodeModes returns the four decode modes: the default, WithUnmatched,
// WithStrict and WithMissing.
func decodeModes() []decodeMode {
	return []decodeMode{
		{"default", func(_, _ *[]string) []canxml.DecoderOption { return nil }},
		{"WithUnmatched", func(u, _ *[]string) []canxml.DecoderOption {
			return []canxml.DecoderOption{canxml.WithUnmatched(func(p string) { *u = append(*u, p) })}
		}},
		{"WithStrict", func(_, _ *[]string) []canxml.DecoderOption {
			return []canxml.DecoderOption{canxml.WithStrict()}
		}},
		{"WithMissing", func(_, m *[]string) []canxml.DecoderOption {
			return []canxml.DecoderOption{canxml.WithMissing(func(p string) { *m = append(*m, p) })}
		}},
	}
}

// decodeOnlyChild decodes doc in mode and returns its one child, failing the
// test on an error, an unmatched item or a missing member.
func decodeOnlyChild(t *testing.T, doc string, mode decodeMode) aom14.CObject {
	t.Helper()
	var unmatched, missing []string
	var got aom14.CSingleAttribute
	if err := canxml.Unmarshal([]byte(doc), &got, mode.opts(&unmatched, &missing)...); err != nil {
		t.Fatalf("canxml.Unmarshal(%s) (%s) = %v; want nil", doc, mode.name, err)
	}
	if len(unmatched) != 0 || len(missing) != 0 {
		t.Errorf("canxml.Unmarshal(%s) (%s) reported unmatched %q and missing %q; want nothing", doc, mode.name, unmatched, missing)
	}
	if len(got.Children) != 1 {
		t.Fatalf("canxml.Unmarshal(%s) (%s): Children = %#v; want one", doc, mode.name, got.Children)
	}
	return got.Children[0]
}

// TestREQ056_QuantityPropertyDecodesBothShapes pins that canonical XML reads
// C_QUANTITY.property from the CODE_PHRASE the AM 1.4 schema type
// C_DV_QUANTITY declares, joining its terminology_id value and its
// code_string with `::`, and from the text an older SDK wrote, in every
// decode mode, with nothing unmatched or missing (REQ-056, REQ-170, REQ-175).
func TestREQ056_QuantityPropertyDecodesBothShapes(t *testing.T) {
	for _, tc := range []struct {
		name, property, want string
	}{
		{"CODE_PHRASE", `<property><terminology_id><value>openehr</value></terminology_id><code_string>125</code_string></property>`, "openehr::125"},
		{"indented CODE_PHRASE", "<property>\n  <terminology_id>\n    <value>openehr</value>\n  </terminology_id>\n  <code_string>125</code_string>\n</property>", "openehr::125"},
		{"older text", `<property>openehr::125</property>`, "openehr::125"},
		{"text with no ::", `<property>mass</property>`, "mass"},
	} {
		for _, mode := range decodeModes() {
			t.Run(tc.name+"/"+mode.name, func(t *testing.T) {
				doc := profileChild("C_DV_QUANTITY", "DV_QUANTITY", tc.property)
				c, ok := decodeOnlyChild(t, doc, mode).(*aom14.CQuantity)
				if !ok {
					t.Fatalf("canxml.Unmarshal(%s): child is not a *aom14.CQuantity", doc)
				}
				if c.Property != tc.want {
					t.Errorf("canxml.Unmarshal(%s) (%s): Property = %q; want %q", doc, mode.name, c.Property, tc.want)
				}
			})
		}
	}
}

// TestREQ056_QuantityPropertyEncodes pins that canonical XML writes a
// C_QUANTITY.property holding `::` as the CODE_PHRASE of the AM 1.4 schema
// type C_DV_QUANTITY, the part before the first `::` as its terminology_id
// value and the rest as its code_string, and one with no `::` as the
// element's text, and that each decodes back to the same string (REQ-056).
func TestREQ056_QuantityPropertyEncodes(t *testing.T) {
	type phrase struct {
		TerminologyID *struct {
			Value string `xml:"value"`
		} `xml:"terminology_id"`
		CodeString *string `xml:"code_string"`
		Text       string  `xml:",chardata"`
	}
	for _, tc := range []struct {
		property, terminology, code string
		phrase                      bool
	}{
		{property: "openehr::125", terminology: "openehr", code: "125", phrase: true},
		{property: "openehr::125::2", terminology: "openehr", code: "125::2", phrase: true},
		{property: "openehr::", terminology: "openehr", code: "", phrase: true},
		{property: "::125", terminology: "", code: "125", phrase: true},
		{property: "mass"},
		{property: ""},
	} {
		t.Run(fmt.Sprintf("%q", tc.property), func(t *testing.T) {
			in := &aom14.CSingleAttribute{
				RMAttributeName: "value",
				Children:        []aom14.CObject{&aom14.CQuantity{RMTypeName: "DV_QUANTITY", Property: tc.property}},
			}
			b, err := canxml.Marshal(in)
			if err != nil {
				t.Fatalf("canxml.Marshal: %v", err)
			}
			var doc struct {
				Children struct {
					Property []phrase `xml:"property"`
				} `xml:"children"`
			}
			if err := xml.Unmarshal(b, &doc); err != nil {
				t.Fatalf("reading %s: %v", b, err)
			}
			if len(doc.Children.Property) != 1 {
				t.Fatalf("canxml.Marshal wrote %d property elements in %s; want one", len(doc.Children.Property), b)
			}
			got := doc.Children.Property[0]
			if tc.phrase {
				if got.TerminologyID == nil || got.TerminologyID.Value != tc.terminology || got.CodeString == nil || *got.CodeString != tc.code || got.Text != "" {
					t.Errorf("canxml.Marshal of Property %q wrote %s; want a CODE_PHRASE with terminology_id value %q and code_string %q, and no text", tc.property, b, tc.terminology, tc.code)
				}
			} else if got.TerminologyID != nil || got.CodeString != nil || got.Text != tc.property {
				t.Errorf("canxml.Marshal of Property %q wrote %s; want the element's text alone", tc.property, b)
			}
			var back aom14.CSingleAttribute
			if err := canxml.Unmarshal(b, &back, canxml.WithStrict()); err != nil {
				t.Fatalf("canxml.Unmarshal(%s) = %v; want nil", b, err)
			}
			if c, ok := back.Children[0].(*aom14.CQuantity); !ok || c.Property != tc.property {
				t.Errorf("canxml.Unmarshal(%s): child %#v; want a *aom14.CQuantity whose Property is %q", b, back.Children[0], tc.property)
			}
		})
	}
}

// TestREQ175_QuantityPropertyMissing pins what canxml.WithMissing reports for
// C_QUANTITY.property (REQ-175, REQ-056): a CODE_PHRASE whose terminology_id
// value or code_string carries a value, and text, are present; one where
// neither does, an empty element and no element report the member once, by
// its canonical-JSON pointer, and nothing inside the CODE_PHRASE. The member
// decodes to the empty string when neither part carries a value.
func TestREQ175_QuantityPropertyMissing(t *testing.T) {
	const missing = "/children/0/property"
	for _, tc := range []struct {
		name, property, value string
		want                  []string
	}{
		{"CODE_PHRASE", `<property><terminology_id><value>openehr</value></terminology_id><code_string>125</code_string></property>`, "openehr::125", nil},
		{"code_string alone", `<property><terminology_id><value/></terminology_id><code_string>125</code_string></property>`, "::125", nil},
		{"terminology_id value alone", `<property><terminology_id><value>openehr</value></terminology_id><code_string/></property>`, "openehr::", nil},
		{"text", `<property>openehr::125</property>`, "openehr::125", nil},
		{"empty parts", `<property><terminology_id><value></value></terminology_id><code_string></code_string></property>`, "", []string{missing}},
		{"whitespace parts", `<property><terminology_id><value> </value></terminology_id><code_string> </code_string></property>`, "", []string{missing}},
		{"empty terminology_id and code_string", `<property><terminology_id/><code_string/></property>`, "", []string{missing}},
		{"empty terminology_id alone", `<property><terminology_id/></property>`, "", []string{missing}},
		{"empty element", `<property/>`, "", []string{missing}},
		{"no element", ``, "", []string{missing}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := profileChild("C_DV_QUANTITY", "DV_QUANTITY", tc.property)
			var got []string
			var v aom14.CSingleAttribute
			if err := canxml.Unmarshal([]byte(doc), &v, canxml.WithMissing(func(p string) { got = append(got, p) })); err != nil {
				t.Fatalf("canxml.Unmarshal(%s) = %v; want nil", doc, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("canxml.Unmarshal(%s) with WithMissing reported %q; want %q", doc, got, tc.want)
			}
			if c, ok := v.Children[0].(*aom14.CQuantity); !ok || c.Property != tc.value {
				t.Errorf("canxml.Unmarshal(%s): child %#v; want a *aom14.CQuantity whose Property is %q", doc, v.Children[0], tc.value)
			}
		})
	}
}

// ordinalSymbol is the symbol every ordinal case below decodes to.
var ordinalSymbol = rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at0001"}

// ordinalSymbolShapes are the two shapes the AM 1.4 schema type
// ORDINAL_SYMBOL gives an ORDINAL's symbol, each holding ordinalSymbol.
var ordinalSymbolShapes = []struct {
	name, symbol string
}{
	{"CODE_PHRASE", `<symbol><terminology_id><value>local</value></terminology_id><code_string>at0001</code_string></symbol>`},
	{"DV_CODED_TEXT with an empty value", `<symbol><value/><defining_code><terminology_id><value>local</value></terminology_id><code_string>at0001</code_string></defining_code></symbol>`},
	{"DV_CODED_TEXT with no value", `<symbol><defining_code><terminology_id><value>local</value></terminology_id><code_string>at0001</code_string></defining_code></symbol>`},
	{"DV_CODED_TEXT with every element", `<symbol><value>Mild</value><hyperlink><value>https://example.org/mild</value></hyperlink><formatting>plain</formatting>` +
		`<mappings><match>=</match><target><terminology_id><value>SNOMED-CT</value></terminology_id><code_string>255604002</code_string></target></mappings>` +
		`<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>` +
		`<encoding><terminology_id><value>IANA_character-sets</value></terminology_id><code_string>UTF-8</code_string></encoding>` +
		`<defining_code><terminology_id><value>local</value></terminology_id><code_string>at0001</code_string></defining_code></symbol>`},
}

// TestREQ056_OrdinalSymbolDecodesBothShapes pins that canonical XML reads an
// ORDINAL's symbol in both shapes of the AM 1.4 schema type ORDINAL_SYMBOL,
// the CODE_PHRASE and the DV_CODED_TEXT whose defining_code is the symbol, in
// every decode mode, and that the DV_CODED_TEXT's elements, an empty value
// among them, are neither unmatched content nor a missing member (REQ-056,
// REQ-170, REQ-175).
func TestREQ056_OrdinalSymbolDecodesBothShapes(t *testing.T) {
	for _, shape := range ordinalSymbolShapes {
		for _, mode := range decodeModes() {
			t.Run(shape.name+"/"+mode.name, func(t *testing.T) {
				doc := profileChild("C_DV_ORDINAL", "DV_ORDINAL", `<list><value>1</value>`+shape.symbol+`</list>`)
				c, ok := decodeOnlyChild(t, doc, mode).(*aom14.COrdinal)
				if !ok {
					t.Fatalf("canxml.Unmarshal(%s): child is not a *aom14.COrdinal", doc)
				}
				if len(c.List) != 1 || c.List[0].Value != 1 || !codePhraseEqual(c.List[0].Symbol, ordinalSymbol) {
					t.Errorf("canxml.Unmarshal(%s) (%s): List = %#v; want one ordinal 1 whose symbol is %#v", doc, mode.name, c.List, ordinalSymbol)
				}
			})
		}
	}
}

// codePhraseEqual reports whether a and b hold the same code phrase.
func codePhraseEqual(a, b rm.CodePhrase) bool {
	return a.TerminologyID == b.TerminologyID && a.CodeString == b.CodeString &&
		(a.PreferredTerm == nil) == (b.PreferredTerm == nil) &&
		(a.PreferredTerm == nil || *a.PreferredTerm == *b.PreferredTerm)
}

// TestREQ175_OrdinalSymbolMissing pins what canxml.WithMissing reports for an
// ORDINAL's symbol written in the DV_CODED_TEXT shape (REQ-175, REQ-056): its
// value is never missing, and a member of the CODE_PHRASE its defining_code
// holds is named by its pointer in the canonical-JSON form of the instance,
// where the symbol is that CODE_PHRASE and has no defining_code.
func TestREQ175_OrdinalSymbolMissing(t *testing.T) {
	const symbol = "/children/0/list/0/symbol"
	for _, tc := range []struct {
		name, symbol string
		want         []string
	}{
		{"empty value", `<symbol><value/><defining_code><terminology_id><value>local</value></terminology_id><code_string>at0001</code_string></defining_code></symbol>`, nil},
		{"defining_code without code_string", `<symbol><value/><defining_code><terminology_id><value>local</value></terminology_id></defining_code></symbol>`, []string{symbol + "/code_string"}},
		{"defining_code with an empty terminology_id", `<symbol><value/><defining_code><terminology_id/><code_string>at0001</code_string></defining_code></symbol>`, []string{symbol + "/terminology_id/value"}},
		{"no defining_code", `<symbol><value/></symbol>`, []string{symbol + "/terminology_id", symbol + "/code_string"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := profileChild("C_DV_ORDINAL", "DV_ORDINAL", `<list><value>1</value>`+tc.symbol+`</list>`)
			var got []string
			if err := canxml.Unmarshal([]byte(doc), new(aom14.CSingleAttribute), canxml.WithMissing(func(p string) { got = append(got, p) })); err != nil {
				t.Fatalf("canxml.Unmarshal(%s) = %v; want nil", doc, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("canxml.Unmarshal(%s) with WithMissing reported %q; want %q", doc, got, tc.want)
			}
		})
	}
}

// TestREQ056_OrdinalSymbolEncodesCodePhrase pins that canonical XML writes an
// ORDINAL's symbol in the CODE_PHRASE shape of the AM 1.4 schema type
// ORDINAL_SYMBOL, never the DV_CODED_TEXT one (REQ-056).
func TestREQ056_OrdinalSymbolEncodesCodePhrase(t *testing.T) {
	in := &aom14.CSingleAttribute{
		RMAttributeName: "value",
		Children: []aom14.CObject{&aom14.COrdinal{
			RMTypeName: "DV_ORDINAL",
			List:       []aom14.Ordinal{{Value: 1, Symbol: ordinalSymbol}},
		}},
	}
	b, err := canxml.Marshal(in)
	if err != nil {
		t.Fatalf("canxml.Marshal: %v", err)
	}
	want := `<symbol><terminology_id><value>local</value></terminology_id><code_string>at0001</code_string></symbol>`
	if !bytes.Contains(b, []byte(want)) || bytes.Contains(b, []byte("defining_code")) {
		t.Errorf("canxml.Marshal wrote %s; want the symbol as %s", b, want)
	}
}

// TestREQ056_CorpusProfileMembers pins that the C_DV_ORDINAL and C_DV_QUANTITY
// constraints of the vendored OPT corpus decode through canonical XML with
// their symbols and properties (REQ-056): every ordinal's symbol has a
// terminology and a code, and every property the OPT writes reads as
// `terminology::code`. The corpus writes symbols in the DV_CODED_TEXT shape
// and properties as CODE_PHRASEs; nothing under either is unmatched.
func TestREQ056_CorpusProfileMembers(t *testing.T) {
	files, err := filepath.Glob("../../../testkit/corpus/templates/*.opt")
	if err != nil || len(files) == 0 {
		t.Fatalf("no OPT corpus found (%d files, error %v)", len(files), err)
	}
	symbols, properties := 0, 0
	for _, file := range files {
		for i, child := range corpusProfileChildren(t, file) {
			doc := `<c_single_attribute xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
				`<rm_attribute_name>value</rm_attribute_name>` + child.raw + `</c_single_attribute>`
			var unmatched []string
			var got aom14.CSingleAttribute
			if err := canxml.Unmarshal([]byte(doc), &got, canxml.WithUnmatched(func(p string) { unmatched = append(unmatched, p) })); err != nil {
				t.Fatalf("%s #%d: canxml.Unmarshal = %v; want nil", filepath.Base(file), i, err)
			}
			for _, p := range unmatched {
				if strings.Contains(p, "/symbol[") || strings.Contains(p, "/property[") {
					t.Errorf("%s #%d: canxml.Unmarshal reported %q unmatched; want nothing under a symbol or a property", filepath.Base(file), i, p)
				}
			}
			switch c := got.Children[0].(type) {
			case *aom14.COrdinal:
				for _, o := range c.List {
					symbols++
					if o.Symbol.TerminologyID.Value == "" || o.Symbol.CodeString == "" {
						t.Errorf("%s #%d: ordinal %d decoded with symbol %#v; want a terminology and a code", filepath.Base(file), i, o.Value, o.Symbol)
					}
				}
			case *aom14.CQuantity:
				if !child.property {
					continue
				}
				properties++
				if before, after, ok := strings.Cut(c.Property, "::"); !ok || before == "" || after == "" {
					t.Errorf("%s #%d: property decoded as %q; want terminology::code", filepath.Base(file), i, c.Property)
				}
			default:
				t.Fatalf("%s #%d: child decoded as %T; want a C_ORDINAL or a C_QUANTITY", filepath.Base(file), i, c)
			}
		}
	}
	if symbols == 0 || properties == 0 {
		t.Fatalf("the corpus walk saw %d ordinal symbols and %d quantity properties: the test would pass without exercising either", symbols, properties)
	}
}

// corpusProfileChild is one `children` element of an OPT written
// `xsi:type="C_DV_ORDINAL"` or `"C_DV_QUANTITY"`, as the OPT has it, and
// whether it holds a `property` element.
type corpusProfileChild struct {
	raw      string
	property bool
}

// corpusProfileChildren returns the C_DV_ORDINAL and C_DV_QUANTITY children
// of the OPT in file, each cut from the file as written.
func corpusProfileChildren(t *testing.T, file string) []corpusProfileChild {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	const nsXSI = "http://www.w3.org/2001/XMLSchema-instance"
	dec := xml.NewDecoder(bytes.NewReader(data))
	var out []corpusProfileChild
	for {
		from := dec.InputOffset()
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "children" {
			continue
		}
		if !slices.ContainsFunc(start.Attr, func(a xml.Attr) bool {
			return a.Name.Space == nsXSI && a.Name.Local == "type" && (a.Value == "C_DV_ORDINAL" || a.Value == "C_DV_QUANTITY")
		}) {
			continue
		}
		if err := dec.Skip(); err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		raw := string(data[from:dec.InputOffset()])
		out = append(out, corpusProfileChild{raw: raw, property: strings.Contains(raw, "<property>")})
	}
}
