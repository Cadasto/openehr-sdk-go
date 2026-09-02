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
// out as the literal six-byte escape sequences `\u003c`, `\u003e`, `\u0026`.
// This is the same carve-out docs/specifications/wire.md § REQ-052's TERM_MAPPING.match
// bullet now records explicitly (mirroring § Unknown response keys): the
// escaped form decodes to the identical single character, so the normative
// obligation is on the decoded value, not the literal bytes. Every other
// character round-trips through canjson.Marshal literally.
var wireEscapes = map[string]string{"<": "\\u003c", ">": "\\u003e", "&": "\\u0026"}

// REQ-046 / REQ-052: TERM_MAPPING.match is a single-character canonical JSON string.
func TestTermMappingMatchRoundTrip(t *testing.T) {
	for _, want := range []string{">", "=", "<", "?"} {
		t.Run(want, func(t *testing.T) {
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
				t.Errorf("encoded form = %s, want match as one-char string %q", out, want)
			}
		})
	}
}

func TestTermMappingMatchRejectsBadLength(t *testing.T) {
	for _, bad := range []string{`""`, `"=="`} {
		t.Run(bad, func(t *testing.T) {
			var c rm.Character
			if err := c.UnmarshalJSON([]byte(bad)); err == nil {
				t.Errorf("UnmarshalJSON(%s) = nil error, want a length error", bad)
			}
		})
	}
}

// TestTermMappingMatchEncodeRefusal: canjson.Marshal refuses a TermMapping
// whose Match is empty or multi-character, wrapping the encoder's error in
// canjson.ErrInvalidValue (REQ-052).
func TestTermMappingMatchEncodeRefusal(t *testing.T) {
	cases := []struct {
		name  string
		match rm.Character
	}{
		{"empty", ""},
		{"two-char", "=="},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tm := rm.TermMapping{
				Match: tc.match,
				Target: rm.CodePhrase{
					TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"},
					CodeString:    "371532007",
				},
			}
			if _, err := canjson.Marshal(&tm); !errors.Is(err, canjson.ErrInvalidValue) {
				t.Errorf("Marshal(match=%q) err = %v, want errors.Is(err, canjson.ErrInvalidValue)", tc.match, err)
			}
		})
	}
}

// TestCharacterUnmarshalJSONNumber pins the back-compat number branch's
// REQ-052 point 4 boundary: a legacy numeric code point still decodes, a JSON
// null is a no-op (the encoding/json Unmarshaler convention), and anything
// that is not a usable Unicode code point — zero, negative, or beyond the
// Unicode range — is a decode error rather than a silently-manufactured
// character (U+0000 or U+FFFD).
func TestCharacterUnmarshalJSONNumber(t *testing.T) {
	t.Run("61 decodes to =", func(t *testing.T) {
		var c rm.Character
		if err := c.UnmarshalJSON([]byte("61")); err != nil {
			t.Fatalf("UnmarshalJSON(61): %v", err)
		}
		if c != "=" {
			t.Errorf("c = %q, want \"=\"", c)
		}
	})

	t.Run("null is a no-op", func(t *testing.T) {
		c := rm.Character("=")
		if err := c.UnmarshalJSON([]byte("null")); err != nil {
			t.Fatalf("UnmarshalJSON(null): %v", err)
		}
		if c != "=" {
			t.Errorf("c = %q after decoding null, want unchanged \"=\"", c)
		}
	})

	for _, bad := range []string{"0", "-5", "1114112"} {
		t.Run(bad, func(t *testing.T) {
			var c rm.Character
			if err := c.UnmarshalJSON([]byte(bad)); err == nil {
				t.Errorf("UnmarshalJSON(%s) = nil error, want a code-point error", bad)
			}
		})
	}
}
