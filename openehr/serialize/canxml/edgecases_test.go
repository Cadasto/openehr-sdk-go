package canxml_test

import (
	"encoding/xml"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// TestDecoderAcceptsSelfClosingEmptyElements asserts the decoder
// treats `<foo/>` (self-closing) and `<foo></foo>` (empty-pair)
// equivalently. The encoder emits the empty-pair form for the
// canonical wire; both forms decode to the same value.
func TestDecoderAcceptsSelfClosingEmptyElements(t *testing.T) {
	in := []byte(`<composition xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><name><value>x</value></name><archetype_node_id>x</archetype_node_id><language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language><territory><terminology_id><value>ISO_3166-1</value></terminology_id><code_string>GB</code_string></territory><category><value>event</value><defining_code><terminology_id><value>openehr</value></terminology_id><code_string>433</code_string></defining_code></category><composer xsi:type="PARTY_SELF"/></composition>`)
	var c rm.Composition
	if err := canxml.Unmarshal(in, &c); err != nil {
		t.Fatalf("Unmarshal self-closing composer: %v", err)
	}
	if _, ok := c.Composer.(*rm.PartySelf); !ok {
		t.Errorf("composer = %T; want *rm.PartySelf", c.Composer)
	}
}

// TestREQ056_XSITypeOfReadsXMLSchemaPrefix pins how XSITypeOf reads a
// prefixed discriminator against the namespace declarations the element
// carries (wire.md § REQ-056): a prefix bound to the XML Schema namespace
// names a built-in datatype, returned as the BMM primitive it reads as,
// whatever the prefix, and so does the literal `xsd:` that no declaration
// binds; any other prefixed value, and a prefixed name that is no built-in
// datatype, comes back as written, which names no type. An unprefixed value
// and an absent one come back unchanged.
func TestREQ056_XSITypeOfReadsXMLSchemaPrefix(t *testing.T) {
	const xsd = "http://www.w3.org/2001/XMLSchema"
	cases := []struct {
		name  string
		value string
		decls []xml.Attr
		want  string
	}{
		{name: "xsd:string with no declaration", value: "xsd:string", want: "String"},
		{name: "xsd:integer with no declaration", value: "xsd:integer", want: "Integer"},
		{name: "xs:string bound on the element", value: "xs:string", decls: []xml.Attr{{Name: xml.Name{Space: "xmlns", Local: "xs"}, Value: xsd}}, want: "String"},
		{name: "xsd:string bound on the element", value: "xsd:string", decls: []xml.Attr{{Name: xml.Name{Space: "xmlns", Local: "xsd"}, Value: xsd}}, want: "String"},
		{name: "xsd:string bound to another namespace", value: "xsd:string", decls: []xml.Attr{{Name: xml.Name{Space: "xmlns", Local: "xsd"}, Value: "urn:example:other"}}, want: "xsd:string"},
		{name: "xs:string with no declaration", value: "xs:string", want: "xs:string"},
		{name: "ns2:DV_QUANTITY", value: "ns2:DV_QUANTITY", want: "ns2:DV_QUANTITY"},
		{name: "xsd:DV_QUANTITY, no built-in datatype", value: "xsd:DV_QUANTITY", want: "xsd:DV_QUANTITY"},
		{name: "DV_QUANTITY", value: "DV_QUANTITY", want: "DV_QUANTITY"},
		{name: "absent", value: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start := xml.StartElement{
				Name: xml.Name{Local: "x"},
				Attr: append(slices.Clone(tc.decls), xml.Attr{Name: canxml.XSITypeAttrName(), Value: tc.value}),
			}
			got, err := canxml.XSITypeOf(start)
			if err != nil {
				t.Fatalf("XSITypeOf(%q) err = %v", tc.value, err)
			}
			if got != tc.want {
				t.Errorf("XSITypeOf(%q) = %q; want %q", tc.value, got, tc.want)
			}
		})
	}
}

// TestXSITypeOfRejectsXMI asserts xmi:type is hard-rejected.
func TestXSITypeOfRejectsXMI(t *testing.T) {
	start := xml.StartElement{
		Name: xml.Name{Local: "x"},
		Attr: []xml.Attr{{Name: xml.Name{Space: canxml.NSXMI, Local: "type"}, Value: "DV_QUANTITY"}},
	}
	_, err := canxml.XSITypeOf(start)
	if err == nil {
		t.Fatal("expected error for xmi:type")
	}
	if !errors.Is(err, canxml.ErrInvalidShape) {
		t.Errorf("err = %v; want errors.Is(_, ErrInvalidShape)", err)
	}
}

// TestEncoderEmptyElementShapeDocumented pins the encoder's
// empty-element shape so changes get flagged in review. The
// canonical wire uses the empty-pair form (`<foo></foo>`) — NOT
// self-closing — because some downstream consumers parse the two
// differently. The decoder accepts both (asserted above).
func TestEncoderEmptyElementShapeDocumented(t *testing.T) {
	c := &rm.Composition{
		ArchetypeNodeID: "x",
		Name:            rm.DVText{Value: "x"},
		Language:        rm.CodePhrase{CodeString: "en"},
		Territory:       rm.CodePhrase{CodeString: "GB"},
		Category:        rm.DVCodedText{DVText: rm.DVText{Value: "event"}},
		Composer:        &rm.PartySelf{},
	}
	got, err := canxml.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(got), `<composer xsi:type="PARTY_SELF"></composer>`) {
		t.Errorf("expected empty-pair shape for empty composer: %s", got)
	}
	if strings.Contains(string(got), `<composer xsi:type="PARTY_SELF"/>`) {
		t.Errorf("encoder must not emit self-closing form: %s", got)
	}
}

// TestDeepFolderTreeStackSafe exercises a deep FOLDER nesting to
// confirm the decoder does not blow the stack on naturally-recursive
// RM structures.
func TestDeepFolderTreeStackSafe(t *testing.T) {
	const depth = 128
	root := &rm.Folder{Name: rm.DVText{Value: "root"}, ArchetypeNodeID: "root"}
	cur := root
	for range depth {
		child := &rm.Folder{Name: rm.DVText{Value: "child"}, ArchetypeNodeID: "child"}
		cur.Folders = append(cur.Folders, *child)
		cur = &cur.Folders[len(cur.Folders)-1]
	}
	b, err := canxml.Marshal(root)
	if err != nil {
		t.Fatalf("Marshal deep tree: %v", err)
	}
	var decoded rm.Folder
	if err := canxml.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal deep tree: %v", err)
	}
	// Walk down to confirm depth survives the round trip.
	got := &decoded
	for i := range depth {
		if len(got.Folders) != 1 {
			t.Fatalf("depth %d: got %d children; want 1", i, len(got.Folders))
		}
		got = &got.Folders[0]
	}
}

// TestUnknownElementSkipped asserts the decoder tolerates unknown
// child elements (forward compatibility with backends that add
// extension fields).
func TestUnknownElementSkipped(t *testing.T) {
	in := []byte(`<dv_quantity xmlns="http://schemas.openehr.org/v1"><magnitude>80.5</magnitude><units>kg</units><__unknown__>ignore</__unknown__></dv_quantity>`)
	var q rm.DVQuantity
	if err := canxml.Unmarshal(in, &q); err != nil {
		t.Fatalf("Unmarshal with unknown element: %v", err)
	}
	if q.Units != "kg" {
		t.Errorf("Units = %q; want kg", q.Units)
	}
}

// Tombstone: keep typereg in scope for future error-class regression
// tests that may live alongside the edge cases.
var _ = typereg.ErrMissingType
