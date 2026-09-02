package rm_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// wireEscapes maps a match character to the substring encoding/json's default
// HTML-escaping emits for it inside a string literal — `<`, `>` and `&` come
// out as the literal six-byte sequences `\u003c`, `\u003e`, `\u0026` (documented
// at wire.md § Unknown response keys); every other character round-trips
// through canjson.Marshal literally.
var wireEscapes = map[string]string{"<": "\\u003c", ">": "\\u003e", "&": "\\u0026"}

// REQ-046 / REQ-052: TERM_MAPPING.match is a single-character canonical JSON string.
func TestTermMappingMatchRoundTrip(t *testing.T) {
	for _, want := range []string{">", "=", "<", "?"} {
		in := []byte(`{"_type":"TERM_MAPPING","match":"` + want + `",` +
			`"target":{"_type":"CODE_PHRASE","terminology_id":` +
			`{"_type":"TERMINOLOGY_ID","value":"SNOMED-CT"},"code_string":"371532007"}}`)
		var tm rm.TermMapping
		if err := canjson.Unmarshal(in, &tm); err != nil {
			t.Fatalf("decode %q: %v", want, err)
		}
		if string(tm.Match) != want {
			t.Fatalf("match = %q, want %q", string(tm.Match), want)
		}
		out, err := canjson.Marshal(&tm)
		if err != nil {
			t.Fatalf("encode %q: %v", want, err)
		}
		wireForm := want
		if esc, escaped := wireEscapes[want]; escaped {
			wireForm = esc
		}
		if !bytes.Contains(out, []byte(`"match":"`+wireForm+`"`)) {
			t.Fatalf("encoded form = %s, want match as one-char string %q", out, want)
		}
	}
}

func TestTermMappingMatchRejectsBadLength(t *testing.T) {
	for _, bad := range []string{`""`, `"=="`} {
		var c rm.Character
		if err := c.UnmarshalJSON([]byte(bad)); err == nil {
			t.Fatalf("UnmarshalJSON(%s) = nil error, want a length error", bad)
		}
	}
}

// TestTermMappingMatchEncodeRefusal: canjson.Marshal refuses a TermMapping
// whose Match is empty or multi-character, wrapping the encoder's error in
// canjson.ErrInvalidValue (REQ-052).
func TestTermMappingMatchEncodeRefusal(t *testing.T) {
	for _, bad := range []rm.Character{"", "=="} {
		tm := rm.TermMapping{
			Match: bad,
			Target: rm.CodePhrase{
				TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"},
				CodeString:    "371532007",
			},
		}
		if _, err := canjson.Marshal(&tm); !errors.Is(err, canjson.ErrInvalidValue) {
			t.Fatalf("Marshal(match=%q) err = %v, want errors.Is(err, canjson.ErrInvalidValue)", bad, err)
		}
	}
}
