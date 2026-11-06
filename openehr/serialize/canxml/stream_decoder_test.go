package canxml_test

// REQ-056 (wire.md § REQ-056): a Decoder given no option resolves the prefix
// of an `xsi:type` against every namespace declaration in scope, as Unmarshal
// does. A stream value whose prefix is declared on an element around the
// attribute is decoded again from where Decode began, into the value as the
// first decode left it, and the values after it decode as they would have.

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
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
// a *bytes.Reader the Decoder seeks back in to decode a value again; buffered,
// an io.ByteReader that cannot seek, whose bytes the Decoder keeps one at a
// time; and a byte at a time and half of each request at a time, readers the
// Decoder reads ahead, so that the bytes it keeps of a value end wherever a
// read ends.
var streamReaders = map[string]func([]byte) io.Reader{
	"whole":       func(b []byte) io.Reader { return bytes.NewReader(b) },
	"buffered":    func(b []byte) io.Reader { return bufio.NewReader(bytes.NewReader(b)) },
	"one byte":    func(b []byte) io.Reader { return iotest.OneByteReader(bytes.NewReader(b)) },
	"half a read": func(b []byte) io.Reader { return iotest.HalfReader(bytes.NewReader(b)) },
}

// streamOnce is a value a Decoder given no option decodes once, its item
// declaring nothing, and streamAgain one it decodes a second time, its item's
// prefix being declared on the item's parent.
var (
	streamOnce  = streamValue{item: ` xsi:type="C_STRING"`, content: "<pattern>.*</pattern>"}
	streamAgain = streamValue{parent: ` xmlns:xs="` + xsdNamespace + `"`, item: ` xsi:type="xs:string"`, content: "archetype_id/value"}
)

// TestREQ056_UnmarshalDecodesAgainWithoutAddingToTheValue pins Unmarshal
// given no option on a slot whose second include's item declares its `xs`
// prefix on its parent: the first decode reads the first include, then stops
// at that item, and Unmarshal decodes the document a second time, tracked,
// into the value as the first decode left it, whose includes start over at
// their first item, so the slot holds the two includes it was written with,
// as a tracked decode reads it, not three.
//
// Can-fail control: let a list member's decoder append to the items the value
// holds instead of starting over at its first item, and the slot holds three
// includes.
func TestREQ056_UnmarshalDecodesAgainWithoutAddingToTheValue(t *testing.T) {
	in := []byte(streamAgain.slot())
	var plain, tracked aom14.ArchetypeSlot
	if err := canxml.Unmarshal(in, &plain); err != nil {
		t.Fatalf("Unmarshal: err = %v; want nil, the second decode resolving the prefix", err)
	}
	if err := canxml.Unmarshal(in, &tracked, canxml.WithMissing(func(string) {})); err != nil {
		t.Fatalf("Unmarshal WithMissing: err = %v; want nil", err)
	}
	want := &aom14.TypedString{Value: "archetype_id/value", Type: "xs:string"}
	if got := secondItem(t, &plain); !reflect.DeepEqual(got, want) {
		t.Errorf("Unmarshal: the second include's item = %#v; want %#v", got, want)
	}
	if !reflect.DeepEqual(plain, tracked) {
		t.Errorf("Unmarshal decodes a slot other than Unmarshal WithMissing: %d includes, want %d", len(plain.Includes), len(tracked.Includes))
	}
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

// errStream is the failure of a stream test's reader.
var errStream = errors.New("the stream failed")

// failingReader returns its data and err from its first Read, and io.EOF from
// every later one, as a reader whose failure arrives with its last bytes may.
type failingReader struct {
	data []byte
	err  error
	read bool
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.read {
		return 0, io.EOF
	}
	r.read = true
	return copy(p, r.data), r.err
}

// TestREQ056_DecoderKeepsTheReadersFailureForTheSecondDecode pins a stream
// whose reader fails with the last bytes of a value that needs the second
// decode: the value decodes from the bytes the Decoder kept, and the next
// Decode returns the reader's failure, which a second read of the reader
// would not give again.
func TestREQ056_DecoderKeepsTheReadersFailureForTheSecondDecode(t *testing.T) {
	v := streamValue{root: ` xmlns:xs="` + xsdNamespace + `"`, item: ` xsi:type="xs:string"`, content: "archetype_id/value"}
	dec := canxml.NewDecoder(&failingReader{data: []byte(v.slot()), err: errStream})
	var slot aom14.ArchetypeSlot
	if err := dec.Decode(&slot); err != nil {
		t.Fatalf("Decode of the value = %v; want nil", err)
	}
	if got, want := secondItem(t, &slot), (&aom14.TypedString{Value: "archetype_id/value", Type: "xs:string"}); !reflect.DeepEqual(got, want) {
		t.Errorf("the value's second item = %#v; want %#v", got, want)
	}
	if err := dec.Decode(new(aom14.ArchetypeSlot)); !errors.Is(err, errStream) {
		t.Errorf("Decode after the value = %v; want the reader's failure", err)
	}
}

// stalledReader returns nothing and no error from every Read.
type stalledReader struct{}

func (stalledReader) Read([]byte) (int, error) { return 0, nil }

// TestREQ056_DecoderRefusesAReaderThatMakesNoProgress pins a Decoder given no
// option over a reader that returns no bytes and no error, read after read:
// Decode fails with io.ErrNoProgress rather than read on for ever.
func TestREQ056_DecoderRefusesAReaderThatMakesNoProgress(t *testing.T) {
	if err := canxml.NewDecoder(stalledReader{}).Decode(new(aom14.ArchetypeSlot)); !errors.Is(err, io.ErrNoProgress) {
		t.Errorf("Decode = %v; want io.ErrNoProgress", err)
	}
}

// TestREQ056_DecoderReadsNoFurtherThanTheValue pins a Decoder over an
// io.ByteReader, in every mode: Decode takes from the reader no byte past the
// end of the value it decodes, so what follows the stream's values is left in
// the reader for the caller. It holds for a bufio.Reader, which cannot seek,
// and for a strings.Reader, which can, for a stream of values each decoded
// once, and for one whose first value is decoded a second time to resolve a
// prefix declared around its item, after which the Decoder reads the rest of
// the stream through the reader it decodes the value again from.
func TestREQ056_DecoderReadsNoFurtherThanTheValue(t *testing.T) {
	const trailer = "TRAILER-BYTES"
	streams := []struct {
		name   string
		values []streamValue
	}{
		{"values decoded once", []streamValue{streamOnce, streamOnce}},
		{"a value decoded again, then one more", []streamValue{streamAgain, streamOnce}},
	}
	readers := []struct {
		name string
		open func(string) io.Reader
	}{
		{"bufio.Reader", func(s string) io.Reader { return bufio.NewReader(strings.NewReader(s)) }},
		{"strings.Reader", func(s string) io.Reader { return strings.NewReader(s) }},
	}
	modes := []struct {
		name string
		opts []canxml.DecoderOption
	}{
		{"no option", nil},
		{"WithUnmatched", []canxml.DecoderOption{canxml.WithUnmatched(func(string) {})}},
		{"WithStrict", []canxml.DecoderOption{canxml.WithStrict()}},
		{"WithMissing", []canxml.DecoderOption{canxml.WithMissing(func(string) {})}},
	}
	for _, s := range streams {
		var stream strings.Builder
		for _, v := range s.values {
			stream.WriteString(v.slot())
		}
		stream.WriteString(trailer)
		for _, rd := range readers {
			for _, m := range modes {
				r := rd.open(stream.String())
				dec := canxml.NewDecoder(r, m.opts...)
				for i := range s.values {
					if err := dec.Decode(new(aom14.ArchetypeSlot)); err != nil {
						t.Fatalf("%s, %s, %s: Decode of value %d = %v; want nil", s.name, rd.name, m.name, i+1, err)
					}
				}
				if rest, err := io.ReadAll(r); err != nil || string(rest) != trailer {
					t.Errorf("%s, %s, %s: io.ReadAll after the last Decode = %q, %v; want %q, nil", s.name, rd.name, m.name, rest, err, trailer)
				}
			}
		}
	}
}

// TestREQ056_DecoderSeeksBackToDecodeAValueAgain pins a Decoder given no
// option over a *bytes.Reader, an io.ByteReader that is also an io.Seeker:
// a value decoded once, a value whose prefix is declared around its item, which
// the Decoder decodes again from where Decode began, and a value after it each
// decode, and each Decode leaves the reader at the end of its value.
func TestREQ056_DecoderSeeksBackToDecodeAValueAgain(t *testing.T) {
	pattern := ".*"
	once := &aom14.CString{Pattern: &pattern}
	values := []struct {
		value streamValue
		want  any
	}{
		{streamOnce, once},
		{streamAgain, &aom14.TypedString{Value: "archetype_id/value", Type: "xs:string"}},
		{streamOnce, once},
	}
	var stream []byte
	for _, v := range values {
		stream = append(stream, v.value.slot()...)
	}
	r := bytes.NewReader(append(stream, "TRAILER-BYTES"...))
	dec := canxml.NewDecoder(r)
	var end int64
	for i, v := range values {
		var slot aom14.ArchetypeSlot
		if err := dec.Decode(&slot); err != nil {
			t.Fatalf("Decode of value %d = %v; want nil", i+1, err)
		}
		if got := secondItem(t, &slot); !reflect.DeepEqual(got, v.want) {
			t.Errorf("value %d's second item = %#v; want %#v", i+1, got, v.want)
		}
		end += int64(len(v.value.slot()))
		if got := r.Size() - int64(r.Len()); got != end {
			t.Errorf("after Decode of value %d the reader stands at offset %d; want %d, the value's end", i+1, got, end)
		}
	}
}

// TestREQ056_DecoderKeepsNoBytesFromAReaderThatCanSeek pins a Decoder given no
// option over a *bytes.Reader, which it would seek back in to decode a value
// again: it keeps no copy of the bytes it reads, so a Decode allocates less
// than one over the same bytes through an io.ByteReader that cannot seek,
// whose bytes the Decoder keeps as it reads them.
func TestREQ056_DecoderKeepsNoBytesFromAReaderThatCanSeek(t *testing.T) {
	data := []byte(streamOnce.slot())
	r := bytes.NewReader(data)
	var unseekable io.Reader = struct {
		io.ByteReader
		io.Reader
	}{r, r}
	allocs := func(src io.Reader) float64 {
		return testing.AllocsPerRun(20, func() {
			r.Reset(data)
			if err := canxml.NewDecoder(src).Decode(new(aom14.ArchetypeSlot)); err != nil {
				t.Fatalf("Decode = %v; want nil", err)
			}
		})
	}
	if seeking, keeping := allocs(r), allocs(unseekable); seeking >= keeping {
		t.Errorf("a Decode over a *bytes.Reader allocates %v times, over an io.ByteReader that cannot seek %v; want fewer", seeking, keeping)
	}
}

// errSeek is the failure of a stream test's seeker.
var errSeek = errors.New("the reader cannot seek")

// failingSeeker is a strings.Reader whose Seek fails.
type failingSeeker struct{ *strings.Reader }

func (failingSeeker) Seek(int64, int) (int64, error) { return 0, errSeek }

// TestREQ056_DecoderReportsASeekThatFails pins a Decoder given no option over
// an io.ByteReader whose Seek fails: a value it decodes once needs no seek and
// decodes, and the decode of a value it decodes again, which seeks back to
// where Decode began, fails with the seek's error.
func TestREQ056_DecoderReportsASeekThatFails(t *testing.T) {
	dec := canxml.NewDecoder(failingSeeker{strings.NewReader(streamOnce.slot() + streamAgain.slot())})
	if err := dec.Decode(new(aom14.ArchetypeSlot)); err != nil {
		t.Fatalf("Decode of the value decoded once = %v; want nil", err)
	}
	if err := dec.Decode(new(aom14.ArchetypeSlot)); !errors.Is(err, errSeek) {
		t.Errorf("Decode of the value decoded again = %v; want the seek's error", err)
	}
}
