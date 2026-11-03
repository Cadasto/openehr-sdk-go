package canxml_test

import (
	"bufio"
	"bytes"
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
