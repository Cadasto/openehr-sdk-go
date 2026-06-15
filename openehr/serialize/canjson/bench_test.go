package canjson_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// benchCompositionPayload synthesises a Composition with `width`
// repeated ADMIN_ENTRY content items (each with an ITEM_TREE data
// payload). Used to exercise the codec on payloads roughly
// comparable to real CDR traffic (~50 KiB at width≈400). The
// composer/content polymorphic sites are populated so the dispatch
// path is exercised — pure-leaf benchmarks would understate
// generated-UnmarshalJSON cost. For HISTORY/EVENT-bearing inputs see
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
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
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
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var c rm.Composition
		if err := canjson.Unmarshal(payload, &c); err != nil {
			b.Fatalf("Unmarshal: %v", err)
		}
	}
}

// BenchmarkEncodeDVQuantity isolates leaf-type encode cost so the
// generator-emitted MarshalJSON overhead per concrete type is
// visible in profiles.
func BenchmarkEncodeDVQuantity(b *testing.B) {
	q := &rm.DVQuantity{Magnitude: 80.5, Units: "kg"}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := canjson.Marshal(q); err != nil {
			b.Fatalf("Marshal: %v", err)
		}
	}
}

// BenchmarkDecodeDVQuantity is the symmetric leaf-type decode. No
// generated UnmarshalJSON exists for DV_QUANTITY (no polymorphic
// fields), so this benchmark measures the encoding/json default
// path through the generated `_type` tag handling.
func BenchmarkDecodeDVQuantity(b *testing.B) {
	body := []byte(`{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg"}`)
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var q rm.DVQuantity
		if err := canjson.Unmarshal(body, &q); err != nil {
			b.Fatalf("Unmarshal: %v", err)
		}
	}
}
