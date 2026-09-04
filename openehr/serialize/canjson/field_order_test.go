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
	// The full slot spelling, `_type` leading, so an encoder that wrote the
	// discriminator anywhere but first inside the slot would fail here even
	// with both encodes equal to each other.
	const slot = `"value":{"_type":"DV_QUANTITY","magnitude":120,"units":"mm[Hg]"}`
	if !bytes.Equal(ea, eb) || !strings.Contains(string(eb), slot) {
		t.Fatalf("re-encode differs by _type position or the slot is not the canonical spelling:\n %s\n %s", ea, eb)
	}
}

// REQ-052 § Field order: `Hash` (map[K]V) keys are written in lexicographic
// key order, independent of struct field order and of the order the map was
// populated. The keys are chosen so that byte-wise order differs from a
// case-folded one ("B" sorts before "a"), pinning which "lexicographic" the
// profile means; the pointer-to-map spelling the generator uses for optional
// Hash fields is covered alongside the plain one.
func TestEncodeHashKeysLexicographic(t *testing.T) {
	other := map[string]string{"a": "x", "B": "y"}
	v := rm.TranslationDetails{
		Author:       map[string]string{"z": "3", "a": "1", "B": "2"},
		Language:     rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: "en"},
		OtherDetails: &other,
	}
	got, err := canjson.Marshal(&v)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	for _, want := range []string{
		`"author":{"B":"2","a":"1","z":"3"}`,
		`"other_details":{"B":"y","a":"x"}`,
	} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("Hash keys are not in lexicographic order: want %s in\n %s", want, got)
		}
	}
}

// REQ-052 § Field order: the encoder MUST write the deterministic
// profile, so two encodes of one value are byte-identical and `_type` is
// the first key. PROBE-030 pins encode-stability over the cassette corpus
// (decode → encode → decode → encode, the two encodes byte-identical and
// never compared against the input); this pins repeat-encode identity and
// `_type`-first on a small value.
func TestEncodeIsDeterministic(t *testing.T) {
	v := rm.DVCodedText{
		Value: "x",
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
