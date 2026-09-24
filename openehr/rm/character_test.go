package rm_test

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// wireEscapes maps a match character to its `\uXXXX` escape spelling, an
// alternative accepted decode input for `<`, `>` and `&`. Under
// encoding/json/v2 (ADR 0022) the encoder does NOT HTML-escape, so
// canjson.Marshal emits these characters literally (census row 21); the escape
// spelling is still accepted on decode. This is the carve-out
// docs/specifications/wire.md § REQ-052's TERM_MAPPING.match bullet records
// (mirroring § Unknown response keys): the escaped form decodes to the
// identical single character, so the normative obligation is on the decoded
// value, not the literal bytes.
var wireEscapes = map[string]string{"<": "\\u003c", ">": "\\u003e", "&": "\\u0026"}

// REQ-046 / REQ-052: TERM_MAPPING.match is a single-character canonical JSON string.
func TestTermMappingMatchRoundTrip(t *testing.T) {
	for _, want := range []string{">", "=", "<", "?"} {
		// Feed both spellings the carve-out admits: the literal character,
		// and, for `<`, `>` and `&`, the `\uXXXX` escape spelling a producer
		// may still send. Decoding the escaped spelling pins the other half of
		// the wire.md carve-out: the obligation is on the decoded character, so
		// the escape must decode to the identical single Character, not to a
		// six-rune literal.
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
				// v2 does not HTML-escape (ADR 0022), so the encoder emits
				// the literal character (census row 21); the `\uXXXX` escape
				// spelling is an accepted decode input only, not what Marshal
				// writes.
				if !bytes.Contains(out, []byte(`"match":"`+want+`"`)) {
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
// boundary (wire.md § REQ-052's `TERM_MAPPING.match` bullet): a legacy
// numeric code point still decodes, a JSON
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
		// that came in (§ REQ-052's `TERM_MAPPING.match` bullet, "never the
		// encoded form").
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

	// 65533 (U+FFFD) is deliberately absent from this list: it is a genuine
	// code point, and a number can only ever name one — a JSON number carries
	// no bytes that could have been substituted — so the number arm accepts
	// it. TestCharacterRefusesSubstitutedReplacementRune pins that acceptance.
	for _, bad := range []string{"0", "-5", "1114112"} {
		t.Run(bad, func(t *testing.T) {
			var c rm.Character
			if err := c.UnmarshalJSON([]byte(bad)); err == nil {
				t.Errorf("UnmarshalJSON(%s) = nil error, want a code-point error; c = %q", bad, string(c))
			}
		})
	}
}

// TestCharacterRefusesSubstitutedReplacementRune pins where the U+FFFD
// line is drawn. The openEHR BASE Character primitive excludes no code
// point, so U+FFFD is itself a legal Character and MUST survive every
// codec; refusing it outright would make a valid character
// unrepresentable in JSON, text and XML alike. What MUST be refused is a
// SUBSTITUTED U+FFFD: a lone UTF-16 surrogate escape or a raw invalid
// UTF-8 byte, which under v1 decoded to U+FFFD with no error and could
// launder corrupted input into an apparently valid Character.
//
// Under encoding/json/v2 the tokenizer refuses both forms while decoding
// the string (ruling R15), so they never reach the one-rune rule: the
// refusal is json.Unmarshal's, not a byte inspection rm.Character runs.
// A U+FFFD written as itself (three raw bytes or a \uFFFD escape) decodes
// cleanly and is accepted, and a well-formed surrogate PAIR decodes to
// the astral character it names (REQ-052; diagnostics value-free per
// REQ-093).
func TestCharacterRefusesSubstitutedReplacementRune(t *testing.T) {
	// Every refusal must leave the receiver untouched, so each case starts
	// from a legal Character rather than the zero value: a decoder that
	// wrote before validating would show up as "=" turning into U+FFFD.
	refusedDecodes := map[string][]byte{
		"lone high surrogate escape": []byte(`"\uD800"`),
		"lone low surrogate escape":  []byte(`"\uDC00"`),
		"raw invalid UTF-8 byte":     {'"', 0xff, '"'},
	}
	for name, in := range refusedDecodes {
		t.Run("decode "+name, func(t *testing.T) {
			c := rm.Character("=")
			err := c.UnmarshalJSON(in)
			if err == nil {
				t.Fatalf("UnmarshalJSON(%s) = nil error, want a refusal; c = %q", in, string(c))
			}
			if c != "=" {
				t.Errorf("UnmarshalJSON(%s) wrote %q to the receiver on failure, want it untouched", in, string(c))
			}
		})
	}

	// The accepted spellings all name U+FFFD (or, for the surrogate-pair
	// cases, prove a well-formed pair decodes to the astral character it
	// names) and all re-encode to the literal character: a genuine U+FFFD
	// rune is valid UTF-8, so the encoder writes it verbatim.
	acceptedDecodes := []struct {
		name string
		in   []byte
		want rm.Character
	}{
		{"escaped U+FFFD", []byte(`"\uFFFD"`), rm.Character(utf8.RuneError)},
		{"mixed-case U+FFFD escape", []byte(`"\uFffD"`), rm.Character(utf8.RuneError)},
		{"literal U+FFFD", []byte(`"` + string(utf8.RuneError) + `"`), rm.Character(utf8.RuneError)},
		{"number 65533", []byte("65533"), rm.Character(utf8.RuneError)},
		{"escaped surrogate pair", []byte(`"\uD83D\uDE00"`), "\U0001F600"},
		{"literal astral character", []byte(`"😀"`), "\U0001F600"},
	}
	for _, tc := range acceptedDecodes {
		t.Run("decode "+tc.name, func(t *testing.T) {
			var c rm.Character
			if err := c.UnmarshalJSON(tc.in); err != nil {
				t.Fatalf("UnmarshalJSON(%s): %v", tc.in, err)
			}
			if c != tc.want {
				t.Fatalf("UnmarshalJSON(%s): c = %q, want %q", tc.in, string(c), string(tc.want))
			}
			out, err := json.Marshal(c)
			if err != nil {
				t.Fatalf("Marshal(%q) after decoding %s: %v", string(c), tc.in, err)
			}
			if want := `"` + string(tc.want) + `"`; string(out) != want {
				t.Errorf("re-encoded %s as %s, want the literal one-character string %s", tc.in, out, want)
			}
		})
	}

	// Encode has no wire bytes to inspect, so the rule there is the value
	// rule alone: valid UTF-8, one rune. A U+FFFD value passes; bytes that
	// are not valid UTF-8 do not.
	t.Run("encode U+FFFD", func(t *testing.T) {
		c := rm.Character(utf8.RuneError)
		out, err := c.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON(U+FFFD): %v", err)
		}
		if want := `"` + string(utf8.RuneError) + `"`; string(out) != want {
			t.Errorf("MarshalJSON(U+FFFD) = %s, want %s", out, want)
		}
	})

	t.Run("encode invalid UTF-8", func(t *testing.T) {
		c := rm.Character(string([]byte{0xff}))
		if out, err := c.MarshalJSON(); err == nil {
			t.Fatalf("MarshalJSON(0xff) = %s, nil error; want a refusal", out)
		}
	})
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

// TestCharacterNilReceiverIsRefusedNotPanicked pins the nil-receiver axis
// (idiom.md § No panics, REQ-025). Both decode entry points assign
// through the pointer, so a direct call on a nil *rm.Character would
// dereference it — and a nil pointer is caller-constructible input
// reachable through the documented API, not a programmer error. The
// refusal is a PLAIN error: caller misuse is not a wire-shape problem, so
// it must stay outside typereg.ErrInvalidShape, which a consumer uses to
// classify bad bytes. A panic here fails the test run outright.
func TestCharacterNilReceiverIsRefusedNotPanicked(t *testing.T) {
	// `null` is the discriminating second input: it is the one value both
	// methods otherwise treat as a no-op, so a nil check placed after the
	// null arm would let it through and panic on nothing at all.
	for _, in := range []string{`"="`, "null"} {
		t.Run("UnmarshalJSON "+in, func(t *testing.T) {
			var c *rm.Character
			err := c.UnmarshalJSON([]byte(in))
			if err == nil {
				t.Fatalf("(*rm.Character)(nil).UnmarshalJSON(%s) = nil error, want a refusal", in)
			}
			if errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("err = %v; caller misuse must not classify as a wire-shape failure", err)
			}
		})
		t.Run("UnmarshalText "+in, func(t *testing.T) {
			var c *rm.Character
			err := c.UnmarshalText([]byte(in))
			if err == nil {
				t.Fatalf("(*rm.Character)(nil).UnmarshalText(%s) = nil error, want a refusal", in)
			}
			if errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("err = %v; caller misuse must not classify as a wire-shape failure", err)
			}
		})
	}
}

// TestCharacterDecodeRefusalMessageUnchangedByClassification pins the
// wire.md § REQ-052 Decode-side shape sentinel MUST that the failure's
// message is "unchanged by the classification" and the cause "stays
// reachable through unwrapping". The sentinel therefore cannot ride on a
// fmt.Errorf("%w: %w") wrap, which splices the sentinel's own text
// ("canjson: invalid JSON shape") into Error(); it rides on Is instead
// (see typereg.ClassifyShape).
//
// The discriminating facet is the exact text: a classified error reads
// exactly as its cause, so errors.Unwrap(err).Error() == err.Error().
func TestCharacterDecodeRefusalMessageUnchangedByClassification(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		// want is the exact message for a refusal rm.Character spells itself.
		// wantContains is used instead for a tokenizer refusal, whose text is
		// jsontext's own and carries an offset that would make an exact pin
		// brittle across inputs and Go versions; that row asserts a stable
		// fragment plus the "unchanged by the classification" property.
		want         string
		wantContains string
	}{
		{"empty string", []byte(`""`), "rm.Character: must be exactly one character, got 0", ""},
		{"two characters", []byte(`"=="`), "rm.Character: must be exactly one character, got 2", ""},
		// Under encoding/json/v2 the tokenizer refuses these two forms while
		// decoding the string, before characterFault runs (ruling R15), so the
		// message is jsontext's own, still behind rm.Character's prefix.
		// TestTermMappingMatchSubstitutedSurrogateRefusedThroughFunnel pins the
		// same bytes through the canjson funnel, where they carry NO sentinel:
		// there the tokenizer refuses the bytes during tokenisation,
		// while on this direct call the tokenizer runs inside rm.Character,
		// whose string arm classifies the refusal, so the two tests answer the
		// ErrInvalidShape question oppositely and both correctly.
		{"raw invalid UTF-8 byte", []byte{'"', 0xff, '"'}, "", "invalid UTF-8"},
		{"substituted surrogate escape", []byte(`"\uD800"`), "", "invalid surrogate pair"},
		{"unusable code point", []byte("0"), "rm.Character: number is not a usable code point", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var c rm.Character
			err := c.UnmarshalJSON(tc.in)
			if err == nil {
				t.Fatalf("UnmarshalJSON(%s) = nil error, want a refusal", tc.in)
			}
			got := err.Error()
			if tc.want != "" {
				if got != tc.want {
					t.Errorf("UnmarshalJSON(%s) err = %q, want %q", tc.in, got, tc.want)
				}
			} else {
				if !strings.HasPrefix(got, "rm.Character: ") || !strings.Contains(got, tc.wantContains) {
					t.Errorf("UnmarshalJSON(%s) err = %q, want it to start with %q and contain %q", tc.in, got, "rm.Character: ", tc.wantContains)
				}
				// The classification adds a sentinel, not a word of text, so the
				// cause reads exactly as the classified error (REQ-052).
				cause := errors.Unwrap(err)
				if cause == nil {
					t.Fatalf("UnmarshalJSON(%s): errors.Unwrap = nil; the cause must stay reachable", tc.in)
				}
				if cause.Error() != got {
					t.Errorf("UnmarshalJSON(%s): classified err = %q but its cause reads %q; the classification must not change the message", tc.in, got, cause.Error())
				}
			}
			if !errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("UnmarshalJSON(%s) err = %v; want errors.Is(err, typereg.ErrInvalidShape)", tc.in, err)
			}
		})
	}

	// A pass-through codec error is classified the same way, so the typed
	// cause stays reachable: the half of the MUST that a text-only wrap
	// would satisfy and an opaque replacement would not.
	t.Run("codec pass-through", func(t *testing.T) {
		var c rm.Character
		err := c.UnmarshalJSON([]byte("true"))
		if err == nil {
			t.Fatal("UnmarshalJSON(true) = nil error, want a refusal")
		}
		if !errors.Is(err, typereg.ErrInvalidShape) {
			t.Errorf("err = %v; want errors.Is(err, typereg.ErrInvalidShape)", err)
		}
		if _, ok := errors.AsType[*jsonv2.SemanticError](err); !ok {
			t.Errorf("err = %v (%T); want errors.AsType[*encoding/json/v2.SemanticError] to reach the cause", err, err)
		}
		cause := errors.Unwrap(err)
		if cause == nil {
			t.Fatalf("errors.Unwrap(%v) = nil; the cause must stay reachable through unwrapping", err)
		}
		if cause.Error() != err.Error() {
			t.Errorf("classified err = %q but its cause reads %q; the classification must not change the message", err.Error(), cause.Error())
		}
	})
}

// TestCharacterRefusalTextCarriesNoSentinelProse is the can-fail control
// for the test above, stated as the property rather than as a literal:
// no Character refusal on any codec may splice the sentinel's own text
// into its message. Restoring the `%w: %w` wrap fails this.
func TestCharacterRefusalTextCarriesNoSentinelProse(t *testing.T) {
	const prose = "invalid JSON shape"
	inputs := [][]byte{[]byte(`""`), []byte(`"=="`), []byte(`"\uD800"`), []byte("0"), []byte("true"), []byte(`1.5`)}
	for _, in := range inputs {
		var c rm.Character
		err := c.UnmarshalJSON(in)
		if err == nil {
			t.Fatalf("UnmarshalJSON(%s) = nil error, want a refusal", in)
		}
		if strings.Contains(err.Error(), prose) {
			t.Errorf("UnmarshalJSON(%s) err = %q; must not contain the sentinel's own prose %q", in, err.Error(), prose)
		}
	}
}

// TestTermMappingMatchSubstitutedSurrogateRefusedThroughFunnel runs a lone
// UTF-16 surrogate escape in TERM_MAPPING.match through the GENERATED
// TERM_MAPPING decoder, the path a consumer actually reaches, rather than
// through rm.Character alone. Under encoding/json/v2 (ADR 0022) jsontext
// refuses the lone surrogate during tokenisation, before rm.Character's
// substituted-U+FFFD detector can inspect the value (Task 2 report Q4, ruling
// R15). So canjson refuses the value as malformed input and it carries NO SDK
// sentinel, the same as any other syntactic refusal (REQ-052: invalid UTF-8
// and a lone surrogate are malformed input). The same migration reconciles
// rm.Character's own side of this.
func TestTermMappingMatchSubstitutedSurrogateRefusedThroughFunnel(t *testing.T) {
	in := []byte(`{"_type":"TERM_MAPPING","match":"\uD800","target":{"_type":"CODE_PHRASE",` +
		`"terminology_id":{"_type":"TERMINOLOGY_ID","value":"local"},"code_string":"x"}}`)
	var tm rm.TermMapping
	err := canjson.Unmarshal(in, &tm)
	if err == nil {
		t.Fatalf("Unmarshal(%s) = nil error, want a refusal; match = %q", in, string(tm.Match))
	}
	// A lone surrogate is malformed input jsontext refuses during tokenisation,
	// so it does NOT acquire the decode-side shape sentinel.
	if errors.Is(err, canjson.ErrInvalidShape) {
		t.Errorf("err = %v; a lone surrogate is malformed input and must not carry canjson.ErrInvalidShape", err)
	}
	// The jsontext refusal text still reaches the caller, so the diagnostic is
	// not lost; only the SDK classification is withheld.
	if !strings.Contains(err.Error(), "jsontext: invalid surrogate pair") {
		t.Errorf("err = %v; want the jsontext surrogate refusal to reach the caller", err)
	}
}

// TestCharacterGenuineReplacementSurvivesTrailingWhitespace guards the
// literal inspection against a direct caller's trailing whitespace, which
// json.Unmarshal accepts: a genuine U+FFFD must not be misread as
// substituted just because the literal did not end at the closing quote.
func TestCharacterGenuineReplacementSurvivesTrailingWhitespace(t *testing.T) {
	for _, in := range []string{"\"\\ufffd\" ", "\"\\uFFFD\"\n", "\"\xef\xbf\xbd\"\t"} {
		var c rm.Character
		if err := c.UnmarshalJSON([]byte(in)); err != nil {
			t.Errorf("UnmarshalJSON(%q): %v, want a genuine U+FFFD to decode", in, err)
			continue
		}
		if c != "\uFFFD" {
			t.Errorf("UnmarshalJSON(%q) = %q, want U+FFFD", in, string(c))
		}
	}
}
