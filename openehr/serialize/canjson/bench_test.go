package canjson_test

import (
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// benchCompositionPayload synthesises a Composition with `width`
// repeated ADMIN_ENTRY content items (each with an ITEM_TREE data
// payload). Used to exercise the codec on payloads roughly
// comparable to real CDR traffic (~50 KiB at width≈400). The
// composer/content polymorphic sites are populated so the dispatch
// path is exercised — pure-leaf benchmarks would understate
// generated-UnmarshalJSONFrom cost. For HISTORY/EVENT-bearing inputs see
// the cassette round-trip benchmarks (TestRoundTripCassettes
// fixtures decode through the same code path).
func benchCompositionPayload(b *testing.B, width int) []byte {
	b.Helper()
	var sb strings.Builder
	sb.WriteString(`{
		"_type": "COMPOSITION",
		"archetype_node_id": "openEHR-EHR-COMPOSITION.encounter.v1",
		"name": {"_type":"DV_TEXT","value":"bench"},
		"language": {"_type":"CODE_PHRASE","code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}},
		"territory": {"_type":"CODE_PHRASE","code_string":"GB","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"}},
		"category": {"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}},
		"composer": {"_type":"PARTY_SELF"},
		"content": [`)
	for i := range width {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{
			"_type": "ADMIN_ENTRY",
			"archetype_node_id": "openEHR-EHR-ADMIN_ENTRY.bench.v1",
			"name": {"_type":"DV_TEXT","value":"item"},
			"language": {"_type":"CODE_PHRASE","code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}},
			"encoding": {"_type":"CODE_PHRASE","code_string":"UTF-8","terminology_id":{"_type":"TERMINOLOGY_ID","value":"IANA_character-sets"}},
			"subject": {"_type":"PARTY_SELF"},
			"data": {"_type":"ITEM_TREE","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"tree"}}
		}`)
	}
	sb.WriteString(`]}`)
	return []byte(sb.String())
}

// BenchmarkEncodeComposition_400 measures full-tree encode of a
// width-400 composition (~ several tens of KiB).
func BenchmarkEncodeComposition_400(b *testing.B) {
	payload := benchCompositionPayload(b, 400)
	var c rm.Composition
	if err := canjson.Unmarshal(payload, &c); err != nil {
		b.Fatalf("setup decode: %v", err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := canjson.Marshal(&c); err != nil {
			b.Fatalf("Marshal: %v", err)
		}
	}
}

// BenchmarkDecodeComposition_400 measures full-tree decode of the
// same payload, including per-item typereg dispatch on `content`.
func BenchmarkDecodeComposition_400(b *testing.B) {
	payload := benchCompositionPayload(b, 400)
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	for b.Loop() {
		var c rm.Composition
		if err := canjson.Unmarshal(payload, &c); err != nil {
			b.Fatalf("Unmarshal: %v", err)
		}
	}
}

// BenchmarkEncodeDVQuantity isolates leaf-type encode cost so the
// generator-emitted MarshalJSONTo overhead per concrete type is
// visible in profiles.
func BenchmarkEncodeDVQuantity(b *testing.B) {
	q := &rm.DVQuantity{Magnitude: 80.5, Units: "kg"}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := canjson.Marshal(q); err != nil {
			b.Fatalf("Marshal: %v", err)
		}
	}
}

// BenchmarkDecodeDVQuantity is the symmetric leaf-type decode.
// DV_QUANTITY has a generated UnmarshalJSONFrom but no polymorphic field,
// so the method decodes into its companion struct and copies the
// fields across: this benchmark measures the nil-receiver guard, the
// `_type` check and the shape-error wrapper with no registry dispatch
// underneath them.
func BenchmarkDecodeDVQuantity(b *testing.B) {
	body := []byte(`{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg"}`)
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for b.Loop() {
		var q rm.DVQuantity
		if err := canjson.Unmarshal(body, &q); err != nil {
			b.Fatalf("Unmarshal: %v", err)
		}
	}
}

// benchCassette is the largest real cassette vendored under
// testkit/cassettes (97 725 bytes). The synthetic payloads above repeat one
// ADMIN_ENTRY shape; this one carries the depth and datatype spread of a real
// CDR document, DV_MULTIMEDIA included, so the two together bracket the codec
// between a wide tree and a deep one.
const benchCassette = "Demonstration.v1"

// BenchmarkDecodeCompositionCassette measures decode plus encode of the largest
// vendored cassette, the baseline the json/v2 migration is measured against
// (plan 2026-09-14-json-v2-migration.md phase 3.5). b.SetBytes reports against
// the input document, so the MB/s figure is throughput per input byte over a
// decode and an encode together. It is not comparable with the MB/s of
// BenchmarkDecodeComposition_400, which decodes only.
//
// The cassette now feeds PROBE-030's fidelity legs and is held out of the
// ValidateRM leg only (its vendored content has inverted DV_INTERVAL bounds);
// it decodes and encodes cleanly, which is all this benchmark asks of it. The
// setup decode and encode below are the control: a cassette that stopped
// decoding would fail here rather than quietly measuring an error path.
func BenchmarkDecodeCompositionCassette(b *testing.B) {
	payload, err := os.ReadFile(fixtures.CompositionJSON(benchCassette))
	if err != nil {
		b.Fatalf("read cassette %s: %v", benchCassette, err)
	}
	var setup rm.Composition
	if err := canjson.Unmarshal(payload, &setup); err != nil {
		b.Fatalf("setup decode of %s: %v", benchCassette, err)
	}
	if _, err := canjson.Marshal(&setup); err != nil {
		b.Fatalf("setup encode of %s: %v", benchCassette, err)
	}
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	for b.Loop() {
		var c rm.Composition
		if err := canjson.Unmarshal(payload, &c); err != nil {
			b.Fatalf("Unmarshal: %v", err)
		}
		if _, err := canjson.Marshal(&c); err != nil {
			b.Fatalf("Marshal: %v", err)
		}
	}
}
