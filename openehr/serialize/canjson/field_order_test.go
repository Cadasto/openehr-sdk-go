package canjson_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// REQ-052 § Field order: the decoder MUST accept members in any order,
// `_type` included — JSON member order carries no meaning (RFC 8259 § 4)
// and servers differ in the order they emit. A permuted spelling, with
// `_type` last at every level, decodes to the same value as the canonical
// spelling and re-encodes to the same bytes.
func TestDecodeAcceptsAnyMemberOrder(t *testing.T) {
	const canonical = `{"_type":"DV_CODED_TEXT","value":"x","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"},"code_string":"532"}}`
	const permuted = `{"defining_code":{"code_string":"532","terminology_id":{"value":"openehr","_type":"TERMINOLOGY_ID"},"_type":"CODE_PHRASE"},"value":"x","_type":"DV_CODED_TEXT"}`

	var a, b rm.DVCodedText
	if err := canjson.Unmarshal([]byte(canonical), &a); err != nil {
		t.Fatalf("decode canonical: %v", err)
	}
	if err := canjson.Unmarshal([]byte(permuted), &b); err != nil {
		t.Fatalf("decode permuted: %v", err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("permuted member order decoded to a different value:\n canonical %+v\n permuted  %+v", a, b)
	}
	ea, err := canjson.Marshal(&a)
	if err != nil {
		t.Fatalf("encode canonical: %v", err)
	}
	eb, err := canjson.Marshal(&b)
	if err != nil {
		t.Fatalf("encode permuted: %v", err)
	}
	if !bytes.Equal(ea, eb) {
		t.Fatalf("re-encode differs by input order:\n %s\n %s", ea, eb)
	}
	if string(ea) != canonical {
		t.Fatalf("re-encode is not the canonical profile:\n got  %s\n want %s", ea, canonical)
	}
}

// The polymorphic half of the same rule: `_type` read from the last
// position of a slot value still drives dispatch, so a server that emits
// the discriminator last decodes exactly like one that emits it first.
func TestDecodePolymorphicSlotWithTypeLast(t *testing.T) {
	const head = `{"_type":"ELEMENT","archetype_node_id":"at0004","name":{"_type":"DV_TEXT","value":"n"},"value":`
	const typeFirst = head + `{"_type":"DV_QUANTITY","magnitude":120,"units":"mm[Hg]"}}`
	const typeLast = head + `{"units":"mm[Hg]","magnitude":120,"_type":"DV_QUANTITY"}}`

	var a, b rm.Element
	if err := canjson.Unmarshal([]byte(typeFirst), &a); err != nil {
		t.Fatalf("decode _type-first: %v", err)
	}
	if err := canjson.Unmarshal([]byte(typeLast), &b); err != nil {
		t.Fatalf("decode _type-last: %v", err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("slot dispatch depends on _type position:\n first %+v\n last  %+v", a, b)
	}
	ea, err := canjson.Marshal(&a)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	eb, err := canjson.Marshal(&b)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytes.Equal(ea, eb) || !strings.Contains(string(eb), `"_type":"DV_QUANTITY"`) {
		t.Fatalf("re-encode differs by _type position or lost the slot type:\n %s\n %s", ea, eb)
	}
}

// REQ-052 § Field order: the encoder MUST write the deterministic
// profile, so two encodes of one value are byte-identical and `_type` is
// the first key. PROBE-030 pins the fixed-point half over the cassette
// corpus; this pins the repeat-encode half on a small value.
func TestEncodeIsDeterministic(t *testing.T) {
	v := rm.DVCodedText{
		DVText: rm.DVText{Value: "x"},
		DefiningCode: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "openehr"},
			CodeString:    "532",
		},
	}
	first, err := canjson.Marshal(&v)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	second, err := canjson.Marshal(&v)
	if err != nil {
		t.Fatalf("encode again: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("two encodes of one value differ:\n %s\n %s", first, second)
	}
	if !bytes.HasPrefix(first, []byte(`{"_type":"DV_CODED_TEXT",`)) {
		t.Fatalf("_type is not the first key: %s", first)
	}
}
