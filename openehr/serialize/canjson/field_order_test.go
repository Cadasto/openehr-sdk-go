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

// hashOrderEncodes is how many times [TestEncodeHashKeysLexicographic] encodes
// the same value. An encoder that does not sort map keys draws a fresh Go map
// iteration order on every encode, so detection rests on both the width of the
// map and on repetition. With eight keys in `author`, one encode of an unsorted
// encoder lands sorted by luck about one time in 8! (40320); requiring all
// twenty encodes to be sorted, and both maps on each of them, leaves no
// realistic way for an unsorted encoder to pass.
const hashOrderEncodes = 20

// REQ-052 § Field order: `Hash` (map[K]V) keys are written in lexicographic
// key order, independent of struct field order and of the order the map was
// populated. The keys are chosen so that byte-wise order differs from a
// case-folded one ("Zeta" sorts before "alpha", "_internal" between them),
// pinning which "lexicographic" the profile means; the pointer-to-map spelling
// the generator uses for optional Hash fields is covered alongside the plain
// one.
//
// Can-fail control. The mutation that turns this red is dropping
// `json.Deterministic(true)` from the generated TRANSLATION_DETAILS marshaler
// once the canonical-JSON path has moved to encoding/json/v2 (ADR 0022): v2
// writes map members in Go map iteration order unless that option is joined in,
// and that order is randomised per encode. Eight keys in `author` and seven in
// `other_details`, encoded [hashOrderEncodes] times with every encode required
// to be sorted, is what makes the miss reliable rather than a coin flip: the
// three and two keys this test carried before would have landed sorted by luck
// about one encode in six and one in two. Under encoding/json, which sorts map
// keys unconditionally, the guard holds trivially, which is the point of
// putting it in place before the codec moves.
func TestEncodeHashKeysLexicographic(t *testing.T) {
	other := map[string]string{
		"accuracy": "high", "Review": "2026-09-01", "scope": "site",
		"_draft": "no", "note": "n", "Purpose": "p", "version": "2",
	}
	v := rm.TranslationDetails{
		Author: map[string]string{
			"organisation": "Cadasto", "alpha": "1", "ORCID": "0000-0002",
			"zulu": "z", "_internal": "i", "beta": "2", "Zeta": "Z", "name": "T",
		},
		Language:     rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: "en"},
		OtherDetails: &other,
	}
	// Byte-wise key order: capitals (0x41 and up) before "_" (0x5F) before
	// lower case (0x61 and up). A case-folded or populate-order encoder
	// produces neither spelling.
	wants := []string{
		`"author":{"ORCID":"0000-0002","Zeta":"Z","_internal":"i","alpha":"1","beta":"2","name":"T","organisation":"Cadasto","zulu":"z"}`,
		`"other_details":{"Purpose":"p","Review":"2026-09-01","_draft":"no","accuracy":"high","note":"n","scope":"site","version":"2"}`,
	}
	for encode := range hashOrderEncodes {
		got, err := canjson.Marshal(&v)
		if err != nil {
			t.Fatalf("encode %d of %d: %v", encode+1, hashOrderEncodes, err)
		}
		for _, want := range wants {
			if !strings.Contains(string(got), want) {
				t.Fatalf("encode %d of %d: Hash keys are not in lexicographic order:\n want %s\n in   %s",
					encode+1, hashOrderEncodes, want, got)
			}
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
