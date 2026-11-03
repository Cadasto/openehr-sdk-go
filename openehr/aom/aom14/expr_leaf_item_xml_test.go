package aom14_test

// REQ-176 § Shapes, REQ-056: EXPR_LEAF.item, a BMM `Any`, decodes as the type
// its `xsi:type` names, a class or an XML Schema built-in read as a String
// under a prefix bound to the XML Schema namespace where the attribute stands,
// or, with none, as its text; the value remembers the form it was read in,
// and the encoder writes that form back.

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// xmlSchemaNamespace is the XML Schema namespace.
const xmlSchemaNamespace = "http://www.w3.org/2001/XMLSchema"

// itemForm is one way to write the item of an EXPR_LEAF inside an
// ARCHETYPE_SLOT's second include: its attributes and content, and the
// namespace declarations of the slot (the root) and of the item's parent.
type itemForm struct {
	root, parent, item, content string
}

// slotWithItem returns an ARCHETYPE_SLOT with two includes. The first one's
// left operand is an `xs:string` item that declares its own prefix, which the
// default decode reads before it meets the second include's left operand,
// written as f says. A decode that read the slot twice into one value would
// hold three includes.
func slotWithItem(f itemForm) []byte {
	include := func(parent, item, content string) string {
		return `<includes><expression xsi:type="EXPR_BINARY_OPERATOR"><type>Boolean</type><operator>2007</operator>` +
			`<precedence_overridden>false</precedence_overridden>` +
			`<left_operand xsi:type="EXPR_LEAF"` + parent + `><type>String</type><item` + item + `>` + content + `</item>` +
			`<reference_type>attribute</reference_type></left_operand>` +
			`<right_operand xsi:type="EXPR_LEAF"><type>String</type><item xsi:type="C_STRING"><pattern>.*</pattern></item>` +
			`<reference_type>constraint</reference_type></right_operand></expression></includes>`
	}
	return []byte(`<archetype_slot xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"` + f.root + `>` +
		`<rm_type_name>CLUSTER</rm_type_name><occurrences><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>1</upper></occurrences>` +
		`<node_id>at0001</node_id>` +
		include("", ` xmlns:xs="`+xmlSchemaNamespace+`" xsi:type="xs:string"`, "first") +
		include(f.parent, f.item, f.content) +
		`</archetype_slot>`)
}

// leftItem returns the item of the left operand of the slot's include i.
func leftItem(t *testing.T, s *aom14.ArchetypeSlot, i int) any {
	t.Helper()
	if len(s.Includes) != 2 {
		t.Fatalf("the slot holds %d includes; want 2", len(s.Includes))
	}
	op, ok := s.Includes[i].Expression.(*aom14.ExprBinaryOperator)
	if !ok {
		t.Fatalf("include %d's expression = %T; want *aom14.ExprBinaryOperator", i, s.Includes[i].Expression)
	}
	leaf, ok := op.LeftOperand.(*aom14.ExprLeaf)
	if !ok {
		t.Fatalf("include %d's left operand = %T; want *aom14.ExprLeaf", i, op.LeftOperand)
	}
	return leaf.Item
}

// itemModes are the ways a caller decodes canonical XML: Unmarshal and a
// Decoder, each with no option and with each option that tracks the decode,
// none of which may report anything here.
func itemModes(t *testing.T) map[string]func([]byte, any) error {
	t.Helper()
	unmarshal := func(opts ...canxml.DecoderOption) func([]byte, any) error {
		return func(data []byte, v any) error { return canxml.Unmarshal(data, v, opts...) }
	}
	decoder := func(opts ...canxml.DecoderOption) func([]byte, any) error {
		return func(data []byte, v any) error { return canxml.NewDecoder(bytes.NewReader(data), opts...).Decode(v) }
	}
	report := canxml.WithUnmatched(func(p string) { t.Errorf("reported unmatched %s; want nothing", p) })
	missing := canxml.WithMissing(func(p string) { t.Errorf("reported missing %s; want nothing", p) })
	return map[string]func([]byte, any) error{
		"Unmarshal":               unmarshal(),
		"Unmarshal WithUnmatched": unmarshal(report),
		"Unmarshal WithStrict":    unmarshal(canxml.WithStrict()),
		"Unmarshal WithMissing":   unmarshal(missing),
		"Decoder":                 decoder(),
		"Decoder WithUnmatched":   decoder(report),
		"Decoder WithStrict":      decoder(canxml.WithStrict()),
		"Decoder WithMissing":     decoder(missing),
	}
}

// TestREQ176_REQ056_ExprLeafItemForms pins the four forms the corpus writes an
// EXPR_LEAF item in, in every mode with nothing reported: `C_STRING` decodes as
// a *aom14.CString; `xs:string` with its prefix declared on the item and
// `xsd:string` with its prefix declared on the root as a *aom14.TypedString
// that keeps the `xsi:type` as written; and an item with no `xsi:type` as its
// text, a Go string. Each encodes back in the form it was read in, the typed
// string declaring the prefix it uses on the item, and reads back as the same
// value.
func TestREQ176_REQ056_ExprLeafItemForms(t *testing.T) {
	pattern := ".*"
	forms := []struct {
		name    string
		form    itemForm
		want    any
		written string
	}{
		{
			name:    "C_STRING",
			form:    itemForm{item: ` xsi:type="C_STRING"`, content: "<pattern>.*</pattern>"},
			want:    &aom14.CString{Pattern: &pattern},
			written: `<item xsi:type="C_STRING"><pattern>.*</pattern></item>`,
		},
		{
			name:    "xs:string declared on the item",
			form:    itemForm{item: ` xmlns:xs="` + xmlSchemaNamespace + `" xsi:type="xs:string"`, content: "archetype_id/value"},
			want:    &aom14.TypedString{Value: "archetype_id/value", Type: "xs:string"},
			written: `<item xmlns:xs="` + xmlSchemaNamespace + `" xsi:type="xs:string">archetype_id/value</item>`,
		},
		{
			name:    "xsd:string declared on the root",
			form:    itemForm{root: ` xmlns:xsd="` + xmlSchemaNamespace + `"`, item: ` xsi:type="xsd:string"`, content: "archetype_id/value"},
			want:    &aom14.TypedString{Value: "archetype_id/value", Type: "xsd:string"},
			written: `<item xmlns:xsd="` + xmlSchemaNamespace + `" xsi:type="xsd:string">archetype_id/value</item>`,
		},
		{
			name:    "no xsi:type",
			form:    itemForm{content: "archetype_id/value"},
			want:    "archetype_id/value",
			written: `<item>archetype_id/value</item>`,
		},
	}
	for mode, decode := range itemModes(t) {
		for _, f := range forms {
			var slot aom14.ArchetypeSlot
			if err := decode(slotWithItem(f.form), &slot); err != nil {
				t.Errorf("%s, %s: decode = %v; want nil", mode, f.name, err)
				continue
			}
			if got := leftItem(t, &slot, 1); !reflect.DeepEqual(got, f.want) {
				t.Errorf("%s, %s: item = %#v; want %#v", mode, f.name, got, f.want)
			}
			out, err := canxml.Marshal(&slot)
			if err != nil {
				t.Errorf("%s, %s: Marshal = %v", mode, f.name, err)
				continue
			}
			if !bytes.Contains(out, []byte(f.written)) {
				t.Errorf("%s, %s: encoded\n%s\nwant it to hold %s", mode, f.name, out, f.written)
			}
			var again aom14.ArchetypeSlot
			if err := canxml.Unmarshal(out, &again, canxml.WithStrict()); err != nil {
				t.Errorf("%s, %s: decoding the encoded slot = %v", mode, f.name, err)
				continue
			}
			if !reflect.DeepEqual(again, slot) {
				t.Errorf("%s, %s: the encoded slot reads back as another value", mode, f.name)
			}
		}
	}
}

// TestREQ176_REQ056_ExprLeafItemPrefix pins how an item's prefixed `xsi:type`
// resolves where the attribute stands (wire.md § REQ-056), in every mode:
// bound to the XML Schema namespace on the item, on an ancestor or on the
// root, or the literal `xsd:` bound nowhere, it reads as a String that keeps
// the `xsi:type` as written; `xsd:` bound to another namespace and `ns2:` are
// refused with typereg.ErrUnknownType.
func TestREQ176_REQ056_ExprLeafItemPrefix(t *testing.T) {
	xsdOn := func(prefix string) string { return fmt.Sprintf(` xmlns:%s="%s"`, prefix, xmlSchemaNamespace) }
	cases := []struct {
		name string
		form itemForm
		want string // the TypedString's Type; empty for a refusal
	}{
		{"xs: bound on the item", itemForm{item: xsdOn("xs") + ` xsi:type="xs:string"`}, "xs:string"},
		{"xs: bound on an ancestor", itemForm{parent: xsdOn("xs"), item: ` xsi:type="xs:string"`}, "xs:string"},
		{"xsd: bound on the root", itemForm{root: xsdOn("xsd"), item: ` xsi:type="xsd:string"`}, "xsd:string"},
		{"an unbound literal xsd:", itemForm{item: ` xsi:type="xsd:string"`}, "xsd:string"},
		{"xsd: bound to another namespace", itemForm{root: ` xmlns:xsd="urn:example:other"`, item: ` xsi:type="xsd:string"`}, ""},
		{"ns2: bound to the openEHR namespace", itemForm{root: ` xmlns:ns2="http://schemas.openehr.org/v1"`, item: ` xsi:type="ns2:string"`}, ""},
		{"ns2: bound nowhere", itemForm{item: ` xsi:type="ns2:string"`}, ""},
	}
	for mode, decode := range itemModes(t) {
		for _, tc := range cases {
			tc.form.content = "archetype_id/value"
			var slot aom14.ArchetypeSlot
			err := decode(slotWithItem(tc.form), &slot)
			if tc.want == "" {
				if !errors.Is(err, typereg.ErrUnknownType) {
					t.Errorf("%s, %s: decode = %v; want typereg.ErrUnknownType", mode, tc.name, err)
				}
				continue
			}
			if err != nil {
				t.Errorf("%s, %s: decode = %v; want nil", mode, tc.name, err)
				continue
			}
			want := &aom14.TypedString{Value: "archetype_id/value", Type: tc.want}
			if got := leftItem(t, &slot, 1); !reflect.DeepEqual(got, want) {
				t.Errorf("%s, %s: item = %#v; want %#v", mode, tc.name, got, want)
			}
		}
	}
}

// TestREQ176_TypedStringRefusesAnUnreadableType pins the encoder's refusal of
// a TypedString whose Type is no prefixed XML Schema built-in datatype, which
// would not read back as the value written: among them a prefix that is no
// XML NCName, which writes malformed XML or a namespace declaration XML
// forbids, and the prefixes `xml`, `xmlns` and `xsi`, whose declaration would
// rebind a namespace the document needs; and a prefix outside the characters
// encoding/xml reads in a name, narrower than XML 1.0 fifth edition's beyond
// Latin-1. Any other NCName prefix reads back.
func TestREQ176_TypedStringRefusesAnUnreadableType(t *testing.T) {
	for _, typ := range []string{
		"string", ":string", "xs:no_such_datatype", "",
		"a b:string", "1a:string", "-a:string", ".a:string", "a<b:string", "a\"b:string",
		"\u2c00:string", "a\u203f:string", "\xff:string",
		"xml:string", "xmlns:string", "xsi:string",
	} {
		leaf := &aom14.ExprLeaf{Type: "String", ReferenceType: "attribute", Item: &aom14.TypedString{Value: "x", Type: typ}}
		_, err := canxml.Marshal(leaf)
		if !errors.Is(err, canxml.ErrInvalidShape) {
			t.Errorf("Marshal(a TypedString of Type %q) = %v; want canxml.ErrInvalidShape", typ, err)
		}
	}
	for _, typ := range []string{"xs:string", "xsd:string", "my-ns.v1_2:string", "\u00e9t\u00e9:string", "a\u00b7b:string", "xmlx:string"} {
		in := &aom14.ExprLeaf{Type: "String", ReferenceType: "attribute", Item: &aom14.TypedString{Value: "x", Type: typ}}
		out, err := canxml.Marshal(in)
		if err != nil {
			t.Errorf("Marshal(a TypedString of Type %q) = %v; want nil", typ, err)
			continue
		}
		var back aom14.ExprLeaf
		if err := canxml.Unmarshal(out, &back, canxml.WithStrict()); err != nil {
			t.Errorf("Unmarshal(%s) = %v; want nil", out, err)
			continue
		}
		if got, ok := back.Item.(*aom14.TypedString); !ok || got.Type != typ || got.Value != "x" {
			t.Errorf("the item of %s reads back as %#v; want the typed string %q of Type %q", out, back.Item, "x", typ)
		}
	}
}
