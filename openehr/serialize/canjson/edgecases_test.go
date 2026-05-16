package canjson_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestDecodeNullEqualsAbsent — REQ-052 documents that the codec
// treats `null` and ABSENT as equivalent on decode and emits ABSENT
// on encode. Verified by feeding a payload with `"precision": null`
// and asserting (a) decode succeeds, (b) the resulting struct has
// nil there, (c) re-encoding emits no `precision` key.
func TestDecodeNullEqualsAbsent(t *testing.T) {
	in := []byte(`{"_type":"DV_QUANTITY","magnitude":1.0,"units":"kg","precision":null,"accuracy":null}`)
	var q rm.DVQuantity
	if err := canjson.Unmarshal(in, &q); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if q.Precision != nil {
		t.Errorf("Precision = %v; want nil for null input", *q.Precision)
	}
	if q.Accuracy != nil {
		t.Errorf("Accuracy = %v; want nil for null input", *q.Accuracy)
	}
	b, err := canjson.Marshal(&q)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(b), `"precision"`) || strings.Contains(string(b), `"accuracy"`) {
		t.Errorf("re-encode must omit nil-pointer optional fields: %s", b)
	}
}

// TestISO8601PassthroughOnString — the codec MUST NOT parse ISO
// 8601 strings into time.Time at the codec layer (REQ-046). String
// values reach the typed helpers (*_ext.go) untouched.
func TestISO8601PassthroughOnString(t *testing.T) {
	in := []byte(`{"_type":"DV_DATE_TIME","value":"2026-05-16T12:34:56.789+02:00"}`)
	var d rm.DVDateTime
	if err := canjson.Unmarshal(in, &d); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if d.Value != "2026-05-16T12:34:56.789+02:00" {
		t.Errorf("Value = %q; want passthrough of original string", d.Value)
	}
	b, err := canjson.Marshal(&d)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"2026-05-16T12:34:56.789+02:00"`) {
		t.Errorf("re-encode must preserve ISO 8601 string verbatim: %s", b)
	}
}

// TestEmptyContainerEncodesAbsent — REQ-052: BMM container
// properties with cardinality.lower == 0 emit ABSENT, not `[]`.
// The complement (decode of a fixture that omits the container leaves
// the slice nil) is also asserted to keep null-vs-absent symmetric.
func TestEmptyContainerEncodesAbsent(t *testing.T) {
	in := []byte(`{
		"_type": "COMPOSITION",
		"archetype_node_id": "x",
		"name": {"_type":"DV_TEXT","value":"x"},
		"language": {"_type":"CODE_PHRASE","code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}},
		"territory": {"_type":"CODE_PHRASE","code_string":"GB","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"}},
		"category": {"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}},
		"composer": {"_type":"PARTY_SELF"}
	}`)
	var c rm.Composition
	if err := canjson.Unmarshal(in, &c); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if c.Content != nil {
		t.Errorf("Content = %v; want nil when input omits the key", c.Content)
	}
	if c.Links != nil {
		t.Errorf("Links = %v; want nil when input omits the key", c.Links)
	}
	b, err := canjson.Marshal(&c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, banned := range []string{`"content"`, `"links"`} {
		if strings.Contains(string(b), banned) {
			t.Errorf("output must omit empty container %s: %s", banned, b)
		}
	}
}

// TestDecodeRecursiveFolder — deep FOLDER trees must decode without
// stack overflow at reasonable depths (>= 8). openEHR's directory
// model is unbounded in principle; this guards a representative
// nesting.
func TestDecodeRecursiveFolder(t *testing.T) {
	// Build a JSON tree 10 folders deep.
	tail := `{"_type":"FOLDER","name":{"_type":"DV_TEXT","value":"leaf"},"archetype_node_id":"at0001"}`
	for i := 0; i < 9; i++ {
		tail = `{"_type":"FOLDER","name":{"_type":"DV_TEXT","value":"node"},"archetype_node_id":"at0001","folders":[` + tail + `]}`
	}
	var f rm.Folder
	if err := canjson.Unmarshal([]byte(tail), &f); err != nil {
		t.Fatalf("Unmarshal deep folder: %v", err)
	}
	// Walk to the leaf, counting depth.
	depth := 0
	cur := &f
	for cur != nil && len(cur.Folders) > 0 {
		depth++
		cur = &cur.Folders[0]
	}
	if depth != 9 {
		t.Errorf("depth = %d; want 9", depth)
	}
}
