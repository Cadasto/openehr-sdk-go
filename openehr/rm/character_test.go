package rm_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"unicode/utf8"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
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
		// Feed both spellings the carve-out admits: the literal character,
		// and — for the three encoding/json HTML-escapes — the `\u003c` /
		// `\u003e` form the encoder itself emits. Decoding the escaped
		// spelling pins the other half of the wire.md carve-out: the
		// obligation is on the decoded character, so the escape must decode
		// to the identical single Character, not to a six-rune literal.
		spellings := map[string]string{"literal": want}
		if esc, escaped := wireEscapes[want]; escaped {
			spellings["escaped"] = esc
		}
		for form, onWire := range spellings {
			t.Run(want+" "+form, func(t *testing.T) {
				in := []byte(`{"_type":"TERM_MAPPING","match":"` + onWire + `",` +
					`"target":{"_type":"CODE_PHRASE","terminology_id":` +
					`{"_type":"TERMINOLOGY_ID","value":"SNOMED-CT"},"code_string":"371532007"}}`)
				var tm rm.TermMapping
				if err := canjson.Unmarshal(in, &tm); err != nil {
					t.Fatalf("decode %s: %v", in, err)
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
		// The back-compat arm is decode-only: re-encoding what it produced
		// MUST give the canonical one-character string, never the number
		// that came in (REQ-052 point 4, "never the encoded form").
		out, err := json.Marshal(c)
		if err != nil {
			t.Fatalf("Marshal after decoding 61: %v", err)
		}
		if string(out) != `"="` {
			t.Errorf("re-encoded 61 as %s, want the one-char string `\"=\"`", out)
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

	// 65533 is U+FFFD spelled as a number. It survives utf8.ValidRune, so
	// only the shared Character rule refuses it — and it must: MarshalJSON
	// refuses U+FFFD, so accepting it here would let the back-compat arm
	// manufacture a value the encoders cannot write back out.
	for _, bad := range []string{"0", "-5", "1114112", "65533"} {
		t.Run(bad, func(t *testing.T) {
			var c rm.Character
			if err := c.UnmarshalJSON([]byte(bad)); err == nil {
				t.Errorf("UnmarshalJSON(%s) = nil error, want a code-point error; c = %q", bad, string(c))
			}
		})
	}
}

// TestCharacterRejectsReplacementRune pins the U+FFFD boundary on both
// arms. encoding/json substitutes U+FFFD for a lone UTF-16 surrogate
// escape and for invalid raw UTF-8 WITHOUT reporting an error, so a
// rune-count check alone sees one rune and launders corrupted input into
// a "valid" Character. Both the substituted value and a literal U+FFFD
// on the wire are therefore refused on decode, and an invalid-UTF-8 or
// U+FFFD Go value is refused on encode (REQ-052; diagnostics value-free
// per REQ-093).
func TestCharacterRejectsReplacementRune(t *testing.T) {
	decodeRefusals := map[string][]byte{
		"lone surrogate escape": []byte(`"\uD800"`),
		"raw invalid UTF-8":     {'"', 0xff, '"'},
		"literal U+FFFD":        []byte(`"` + string(utf8.RuneError) + `"`),
	}
	for name, in := range decodeRefusals {
		t.Run("decode "+name, func(t *testing.T) {
			var c rm.Character
			err := c.UnmarshalJSON(in)
			if err == nil {
				t.Fatalf("UnmarshalJSON(%s) = nil error, want a refusal; c = %q", in, c)
			}
			if c != "" {
				t.Errorf("UnmarshalJSON(%s) wrote %q to the receiver on failure, want it untouched", in, c)
			}
		})
	}

	encodeRefusals := map[string]rm.Character{
		"invalid UTF-8":  rm.Character(string([]byte{0xff})),
		"literal U+FFFD": rm.Character(utf8.RuneError),
	}
	for name, c := range encodeRefusals {
		t.Run("encode "+name, func(t *testing.T) {
			out, err := c.MarshalJSON()
			if err == nil {
				t.Fatalf("MarshalJSON(%q) = %s, nil error; want a refusal", string(c), out)
			}
		})
	}
}

// TestCharacterAcceptsNonASCII guards against the U+FFFD and UTF-8
// checks over-reaching: a legal non-ASCII single character (U+2265
// GREATER-THAN OR EQUAL TO, a plausible TERM_MAPPING.match extension)
// must still decode, and must re-encode to the same one-character
// string.
func TestCharacterAcceptsNonASCII(t *testing.T) {
	const want = "\u2265" // ≥
	var c rm.Character
	if err := c.UnmarshalJSON([]byte(`"` + want + `"`)); err != nil {
		t.Fatalf("UnmarshalJSON(%q): %v", want, err)
	}
	if string(c) != want {
		t.Fatalf("c = %q, want %q", string(c), want)
	}
	out, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal(%q): %v", want, err)
	}
	var back rm.Character
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("Unmarshal(%s): %v", out, err)
	}
	if back != c {
		t.Errorf("round-trip gave %q, want %q", string(back), want)
	}
}

// TestCharacterDecodeRefusalsCarryShapeSentinel pins the classification
// split: every Character DECODE refusal wraps typereg.ErrInvalidShape,
// so a bare-Character decode classifies the way the generated
// TERM_MAPPING funnel would, while an ENCODE refusal stays outside it
// (canjson attaches its own encode-only ErrInvalidValue there). The two
// sentinels must never meet on one error (REQ-052 § three distinct
// sentinels).
func TestCharacterDecodeRefusalsCarryShapeSentinel(t *testing.T) {
	decodeRefusals := map[string][]byte{
		"empty string":       []byte(`""`),
		"two characters":     []byte(`"=="`),
		"replacement rune":   []byte(`"\uD800"`),
		"code point zero":    []byte(`0`),
		"code point too big": []byte(`1114112`),
		"fractional number":  []byte(`1.5`),
		"wrong JSON kind":    []byte(`true`),
	}
	for name, in := range decodeRefusals {
		t.Run("decode "+name, func(t *testing.T) {
			var c rm.Character
			err := c.UnmarshalJSON(in)
			if err == nil {
				t.Fatalf("UnmarshalJSON(%s) = nil error, want a refusal", in)
			}
			if !errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("UnmarshalJSON(%s) err = %v; want errors.Is(err, typereg.ErrInvalidShape)", in, err)
			}
			if errors.Is(err, canjson.ErrInvalidValue) {
				t.Errorf("UnmarshalJSON(%s) err = %v; the encode-only sentinel must not appear on a decode path", in, err)
			}
		})
	}

	for name, c := range map[string]rm.Character{"empty": "", "two characters": "=="} {
		t.Run("encode "+name, func(t *testing.T) {
			_, err := c.MarshalJSON()
			if err == nil {
				t.Fatalf("MarshalJSON(%q) = nil error, want a refusal", string(c))
			}
			if errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("MarshalJSON(%q) err = %v; the decode-side shape sentinel must not appear on an encode path", string(c), err)
			}
		})
	}
}
