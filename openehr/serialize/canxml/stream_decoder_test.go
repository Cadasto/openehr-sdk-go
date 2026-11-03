package canxml_test

// REQ-056 (wire.md § REQ-056): a Decoder given no option resolves the prefix
// of an `xsi:type` against every namespace declaration in scope, as Unmarshal
// does. A stream value whose prefix is declared on an element around the
// attribute is decoded again from where Decode began, into the value cleared,
// and the values after it decode as they would have.

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"
	"testing/iotest"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// streamValue is one ARCHETYPE_SLOT of a stream: what precedes it, the
// namespace declarations of the slot and of the parent of its second
// include's item, and that item's attributes and content.
type streamValue struct {
	prelude, root, parent, item, content string
}

// slot returns v as an ARCHETYPE_SLOT with two includes, after its prelude.
// The first include's left operand is an `xs:string` item that declares its
// own prefix, which a decode through a plain encoding/xml decoder resolves
// before it reaches the second include's left operand. A decode that read the
// slot twice into one value would hold three includes.
func (v streamValue) slot() string {
	include := func(parent, item, content string) string {
		return `<includes><expression xsi:type="EXPR_BINARY_OPERATOR"><type>Boolean</type><operator>2007</operator>` +
			`<precedence_overridden>false</precedence_overridden>` +
			`<left_operand xsi:type="EXPR_LEAF"` + parent + `><type>String</type><item` + item + `>` + content + `</item>` +
			`<reference_type>attribute</reference_type></left_operand>` +
			`<right_operand xsi:type="EXPR_LEAF"><type>String</type><item xsi:type="C_STRING"><pattern>.*</pattern></item>` +
			`<reference_type>constraint</reference_type></right_operand></expression></includes>`
	}
	return v.prelude + `<archetype_slot xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"` + v.root + `>` +
		`<rm_type_name>CLUSTER</rm_type_name><occurrences><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>1</upper></occurrences>` +
		`<node_id>at0001</node_id>` +
		include("", ` xmlns:xs="`+xsdNamespace+`" xsi:type="xs:string"`, "first") +
		include(v.parent, v.item, v.content) +
		`</archetype_slot>`
}

// secondItem returns the item of the left operand of the slot's second
// include, after checking that the slot holds the two includes it was written
// with.
func secondItem(t *testing.T, s *aom14.ArchetypeSlot) any {
	t.Helper()
	if len(s.Includes) != 2 {
		t.Fatalf("the slot holds %d includes; want 2", len(s.Includes))
	}
	op, ok := s.Includes[1].Expression.(*aom14.ExprBinaryOperator)
	if !ok {
		t.Fatalf("the second include's expression = %T; want *aom14.ExprBinaryOperator", s.Includes[1].Expression)
	}
	leaf, ok := op.LeftOperand.(*aom14.ExprLeaf)
	if !ok {
		t.Fatalf("the second include's left operand = %T; want *aom14.ExprLeaf", op.LeftOperand)
	}
	return leaf.Item
}

// streamReaders are the readers a stream test reads its input through: whole,
// a byte at a time, and half of each request at a time, so that the bytes a
// Decoder keeps of a value end wherever a read ends.
var streamReaders = map[string]func([]byte) io.Reader{
	"whole":       func(b []byte) io.Reader { return bytes.NewReader(b) },
	"one byte":    func(b []byte) io.Reader { return iotest.OneByteReader(bytes.NewReader(b)) },
	"half a read": func(b []byte) io.Reader { return iotest.HalfReader(bytes.NewReader(b)) },
}

// TestREQ056_DecoderDecodesAValueAgainWhenItsPrefixIsOutOfSight pins a stream
// of four values, decoded one after another by a Decoder given no option. The
// first resolves every prefix on the element that uses it. The second, after
// a comment that holds markup, declares `xs` on the parent of an item whose
// `xsi:type` uses it, so its decode stops there and starts again from where
// Decode began; its value holds the two includes the slot was written with,
// not three. The third declares `xsd` on its root, and the fourth carries an
// item with no `xsi:type`; both decode after the second, and then the stream
// is at its end.
func TestREQ056_DecoderDecodesAValueAgainWhenItsPrefixIsOutOfSight(t *testing.T) {
	pattern := ".*"
	values := []struct {
		value streamValue
		want  any
	}{
		{
			value: streamValue{item: ` xsi:type="C_STRING"`, content: "<pattern>.*</pattern>"},
			want:  &aom14.CString{Pattern: &pattern},
		},
		{
			value: streamValue{
				prelude: "\n<!-- the next value declares xs around its item: <archetype_slot> -->\n",
				parent:  ` xmlns:xs="` + xsdNamespace + `"`,
				item:    ` xsi:type="xs:string"`,
				content: "archetype_id/value",
			},
			want: &aom14.TypedString{Value: "archetype_id/value", Type: "xs:string"},
		},
		{
			value: streamValue{prelude: "\n", root: ` xmlns:xsd="` + xsdNamespace + `"`, item: ` xsi:type="xsd:string"`, content: "openEHR-EHR-CLUSTER.x.v1"},
			want:  &aom14.TypedString{Value: "openEHR-EHR-CLUSTER.x.v1", Type: "xsd:string"},
		},
		{
			value: streamValue{prelude: "<!-- last -->", content: "archetype_id/value"},
			want:  "archetype_id/value",
		},
	}
	var stream []byte
	for _, v := range values {
		stream = append(stream, v.value.slot()...)
	}
	stream = append(stream, "\n"...)
	for name, reader := range streamReaders {
		dec := canxml.NewDecoder(reader(stream))
		for i, v := range values {
			var slot aom14.ArchetypeSlot
			if err := dec.Decode(&slot); err != nil {
				t.Fatalf("%s: Decode of value %d = %v; want nil", name, i+1, err)
			}
			if got := secondItem(t, &slot); !reflect.DeepEqual(got, v.want) {
				t.Errorf("%s: value %d's second item = %#v; want %#v", name, i+1, got, v.want)
			}
		}
		if err := dec.Decode(new(aom14.ArchetypeSlot)); !errors.Is(err, io.EOF) {
			t.Errorf("%s: Decode after the last value = %v; want io.EOF", name, err)
		}
	}
}

// TestREQ056_DecoderDecodesTheFirstValueAgainFromTheStreamStart pins a
// stream whose first value needs the second decode and opens with the XML
// declaration and a comment: the second decode reads them again, and the
// value after it decodes too.
func TestREQ056_DecoderDecodesTheFirstValueAgainFromTheStreamStart(t *testing.T) {
	first := streamValue{
		prelude: `<?xml version="1.0" encoding="UTF-8"?>` + "\n<!-- a template -->\n",
		root:    ` xmlns:xs="` + xsdNamespace + `"`,
		item:    ` xsi:type="xs:string"`,
		content: "archetype_id/value",
	}
	second := streamValue{prelude: "\n", item: ` xsi:type="C_STRING"`, content: "<pattern>.*</pattern>"}
	stream := []byte(first.slot() + second.slot())
	for name, reader := range streamReaders {
		dec := canxml.NewDecoder(reader(stream))
		var a, b aom14.ArchetypeSlot
		if err := dec.Decode(&a); err != nil {
			t.Fatalf("%s: Decode of the first value = %v; want nil", name, err)
		}
		if got, want := secondItem(t, &a), (&aom14.TypedString{Value: "archetype_id/value", Type: "xs:string"}); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: the first value's second item = %#v; want %#v", name, got, want)
		}
		if err := dec.Decode(&b); err != nil {
			t.Fatalf("%s: Decode of the second value = %v; want nil", name, err)
		}
		if _, ok := secondItem(t, &b).(*aom14.CString); !ok {
			t.Errorf("%s: the second value's second item = %#v; want a *aom14.CString", name, secondItem(t, &b))
		}
		if err := dec.Decode(new(aom14.ArchetypeSlot)); !errors.Is(err, io.EOF) {
			t.Errorf("%s: Decode after the last value = %v; want io.EOF", name, err)
		}
	}
}
