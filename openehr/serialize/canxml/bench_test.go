package canxml_test

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// loadCompositionFromJSON resolves a JSON fixture and decodes it
// into an *rm.Composition. The vendored body_weight.json fixture is
// ~7 KiB of canonical openEHR composition — close enough to the
// 50 KiB benchmark target the plan calls for while keeping CI cheap.
// Larger fixtures (BMI.json, vital_signs.json) are exercised in the
// extended bench runs (`-bench=BenchmarkAll`).
func loadCompositionFromJSON(b *testing.B, name string) *rm.Composition {
	b.Helper()
	path := fixtures.CompositionJSON(fixtures.TemplateSlug(strings.TrimSuffix(name, ".json")))
	raw, err := os.ReadFile(path)
	if err != nil {
		b.Fatalf("read fixture %q: %v", path, err)
	}
	var c rm.Composition
	if err := canjson.Unmarshal(raw, &c); err != nil {
		b.Fatalf("decode fixture %q: %v", path, err)
	}
	return &c
}

// BenchmarkMarshalComposition pins encode throughput against a
// known Composition graph. Comparison with the canjson Marshal
// benchmark in canjson/bench_test.go is the headline metric for the
// "encoding/xml vs encoding/json" trade-off documented in STRAND-04.
func BenchmarkMarshalComposition(b *testing.B) {
	c := loadCompositionFromJSON(b, "body_weight.json")
	for b.Loop() {
		if _, err := canxml.Marshal(c); err != nil {
			b.Fatal(err)
		}
	}
}

// TestEncodePolyCopiesNoAttributes pins EncodePoly, which the generated
// encoders call at each polymorphic member, to allocating nothing for the
// attributes it hands a MarshalXML, once the encoder has grown its stacks: a
// polymorphic ELEMENT, with its archetype_node_id, and its DV_TEXT name, each
// with its `xsi:type`.
func TestEncodePolyCopiesNoAttributes(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector drops pooled values at random")
	}
	e := xml.NewEncoder(io.Discard)
	element := &rm.Element{ArchetypeNodeID: "at0004", Name: &rm.DVText{}}
	n := testing.AllocsPerRun(100, func() {
		if err := canxml.EncodePoly(e, "items", element); err != nil {
			t.Fatalf("EncodePoly: %v", err)
		}
	})
	if n != 0 {
		t.Errorf("EncodePoly of an ELEMENT allocates %v times; want 0", n)
	}
}

// BenchmarkUnmarshalComposition pins decode throughput. The input
// is pre-encoded once via canxml.Marshal so the loop measures only
// the decode side.
func BenchmarkUnmarshalComposition(b *testing.B) {
	c := loadCompositionFromJSON(b, "body_weight.json")
	body, err := canxml.Marshal(c)
	if err != nil {
		b.Fatalf("seed encode: %v", err)
	}
	for b.Loop() {
		var into rm.Composition
		if err := canxml.Unmarshal(body, &into); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDecoderComposition pins the decode throughput of a stream Decoder
// given no option, on the input BenchmarkUnmarshalComposition decodes. The
// composition holds no prefixed `xsi:type`, so the Decoder decodes it once,
// as encoding/xml does. Its reader, a *bytes.Reader, is an io.ByteReader and an
// io.Seeker, which the Decoder reads directly, a byte at a time, so that it
// reads no byte past the value, and would seek back in to decode a value
// again, so that it keeps none of the value's bytes.
func BenchmarkDecoderComposition(b *testing.B) {
	body := encodedComposition(b)
	for b.Loop() {
		var into rm.Composition
		if err := canxml.NewDecoder(bytes.NewReader(body)).Decode(&into); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDecoderCompositionReadAhead is BenchmarkDecoderComposition over a
// reader that is no io.ByteReader, which the Decoder reads ahead, in blocks,
// keeping the bytes of the value.
func BenchmarkDecoderCompositionReadAhead(b *testing.B) {
	body := encodedComposition(b)
	for b.Loop() {
		var into rm.Composition
		if err := canxml.NewDecoder(struct{ io.Reader }{bytes.NewReader(body)}).Decode(&into); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDecoderCompositionBuffered is BenchmarkDecoderComposition over a
// *bufio.Reader, an io.ByteReader that cannot seek, whose bytes the Decoder
// keeps as it reads them, one at a time. The bufio.Reader is reset for each
// decode rather than made anew, so its buffer is not counted.
func BenchmarkDecoderCompositionBuffered(b *testing.B) {
	body := encodedComposition(b)
	src := bytes.NewReader(body)
	buf := bufio.NewReader(src)
	for b.Loop() {
		src.Reset(body)
		buf.Reset(src)
		var into rm.Composition
		if err := canxml.NewDecoder(buf).Decode(&into); err != nil {
			b.Fatal(err)
		}
	}
}

// encodedComposition returns the canonical XML of the body_weight.json
// composition, which the Decoder benchmarks decode.
func encodedComposition(b *testing.B) []byte {
	b.Helper()
	body, err := canxml.Marshal(loadCompositionFromJSON(b, "body_weight.json"))
	if err != nil {
		b.Fatalf("seed encode: %v", err)
	}
	return body
}

// BenchmarkUnmarshalCorpusTracked pins the cost of a tracked decode against a
// plain one on two corpus compositions, BMI.xml and Demonstration.v1.xml:
// Unmarshal given no option, and given WithMissing or WithPresence, each of
// which reads every element through the tracker and converts the text of each
// primitive element there.
func BenchmarkUnmarshalCorpusTracked(b *testing.B) {
	for _, id := range []string{"BMI", "Demonstration.v1"} {
		path := fixtures.CompositionXML(id)
		body, err := os.ReadFile(path)
		if err != nil {
			b.Fatalf("read fixture %q: %v", path, err)
		}
		for _, mode := range []struct {
			name string
			opts []canxml.DecoderOption
		}{
			{name: "plain"},
			{name: "WithMissing", opts: []canxml.DecoderOption{canxml.WithMissing(func(canxml.Pointer) {})}},
			{name: "WithPresence", opts: []canxml.DecoderOption{canxml.WithPresence(func(canxml.Pointer, canxml.Presence) {})}},
		} {
			b.Run(id+"/"+mode.name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var into rm.Composition
					if err := canxml.Unmarshal(body, &into, mode.opts...); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
