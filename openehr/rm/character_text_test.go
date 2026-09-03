package rm_test

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// xmlMatchHolder is the smallest struct that reaches rm.Character through
// encoding/xml element content the way the generated canonical-XML decoder
// does (xml.Decoder.DecodeElement on the plain string kind). It keeps the
// contract pins independent of TERM_MAPPING's other required properties;
// TestCharacterXMLViaTermMapping below covers the generated path itself.
type xmlMatchHolder struct {
	XMLName xml.Name     `xml:"term_mapping"`
	Match   rm.Character `xml:"match"`
}

// TestCharacterXMLElementContentValidated pins REQ-046 / REQ-052 /
// REQ-056 on the canonical-XML side: Character validity is a property of
// the value, not of the codec that carried it. Before rm.Character
// implemented encoding.TextUnmarshaler / encoding.TextMarshaler,
// encoding/xml treated `match` as a plain string kind, so an empty or
// multi-character value passed through XML unvalidated while canonical
// JSON refused it. REQ-056 (canonical XML) is cited because the XML
// behaviour these cases fix is normative there too, not only in the JSON
// clause that discovered it.
//
// U+FFFD is ACCEPTED here, in every spelling XML offers, because it is a
// legal Character (the BASE primitive excludes no code point) and the
// text codec has no wire bytes to judge a substitution by — encoding/xml
// hands over decoded text only.
//
// That leaves one residual laundering channel, measured here rather than
// assumed: encoding/xml refuses invalid UTF-8 (a raw bad byte, and the
// CESU-8 spelling of a surrogate) and a character reference beyond
// U+10FFFF, but a SURROGATE-HALF reference — &#xD800; — it converts with
// Go's rune-to-string rule, which yields U+FFFD, and reports no error.
// So a surrogate half arrives at UnmarshalText indistinguishable from a
// genuine U+FFFD, and is accepted. Closing that would mean refusing
// U+FFFD in XML altogether, i.e. making a legal Character
// unrepresentable there — the very defect this round fixes on the JSON
// side. The channel is recorded, not closed; the JSON string arm, which
// does hold the literal, still refuses its own surrogate escapes.
func TestCharacterXMLElementContentValidated(t *testing.T) {
	decode := []struct {
		name    string
		doc     string
		want    rm.Character
		wantErr bool
	}{
		{name: "one character", doc: `<term_mapping><match>=</match></term_mapping>`, want: "="},
		{name: "escaped one character", doc: `<term_mapping><match>&lt;</match></term_mapping>`, want: "<"},
		{name: "empty element", doc: `<term_mapping><match></match></term_mapping>`, wantErr: true},
		{name: "self-closing element", doc: `<term_mapping><match/></term_mapping>`, wantErr: true},
		{name: "two characters", doc: `<term_mapping><match>ab</match></term_mapping>`, wantErr: true},
		{name: "literal U+FFFD", doc: `<term_mapping><match>` + "\uFFFD" + `</match></term_mapping>`, want: "\uFFFD"},
		{name: "U+FFFD character reference", doc: `<term_mapping><match>&#xFFFD;</match></term_mapping>`, want: "\uFFFD"},
		// The residual channel, pinned as it actually behaves: a
		// surrogate-half reference is substituted by encoding/xml and
		// reaches UnmarshalText as a plain U+FFFD, so it decodes.
		{name: "surrogate half character reference", doc: `<term_mapping><match>&#xD800;</match></term_mapping>`, want: "\uFFFD"},
		// The corrupted spellings encoding/xml refuses on its own, before
		// UnmarshalText is consulted.
		{name: "raw invalid UTF-8 byte", doc: "<term_mapping><match>\xff</match></term_mapping>", wantErr: true},
		{name: "CESU-8 surrogate bytes", doc: "<term_mapping><match>\xed\xa0\x80</match></term_mapping>", wantErr: true},
		{name: "character reference beyond Unicode", doc: `<term_mapping><match>&#x110000;</match></term_mapping>`, wantErr: true},
	}
	for _, tc := range decode {
		t.Run("decode "+tc.name, func(t *testing.T) {
			var got xmlMatchHolder
			err := xml.Unmarshal([]byte(tc.doc), &got)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Unmarshal(%s) = nil error, want a refusal; match = %q", tc.doc, string(got.Match))
				}
				return
			}
			if err != nil {
				t.Fatalf("Unmarshal(%s): %v", tc.doc, err)
			}
			if got.Match != tc.want {
				t.Errorf("Unmarshal(%s): match = %q, want %q", tc.doc, string(got.Match), string(tc.want))
			}
		})
	}

	encode := []struct {
		name    string
		match   rm.Character
		want    string
		wantErr bool
	}{
		{name: "one character", match: "=", want: `<term_mapping><match>=</match></term_mapping>`},
		{name: "U+FFFD", match: "\uFFFD", want: `<term_mapping><match>` + "\uFFFD" + `</match></term_mapping>`},
		{name: "empty", match: "", wantErr: true},
		{name: "two characters", match: "ab", wantErr: true},
	}
	for _, tc := range encode {
		t.Run("encode "+tc.name, func(t *testing.T) {
			out, err := xml.Marshal(xmlMatchHolder{Match: tc.match})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Marshal(match=%q) = %s, nil error; want a refusal", string(tc.match), out)
				}
				return
			}
			if err != nil {
				t.Fatalf("Marshal(match=%q): %v", string(tc.match), err)
			}
			if string(out) != tc.want {
				t.Errorf("Marshal(match=%q) = %s, want %s", string(tc.match), out, tc.want)
			}
		})
	}
}

// TestCharacterXMLViaTermMapping runs the same boundary through the
// generated canonical-XML codec (openehr/rm/data_types_text_xmlunmar_gen.go
// / _xmlmar_gen.go), which is the code path a consumer actually reaches:
// both call DecodeElement / EncodeElement on TermMapping.Match, so the
// Character text codec is what holds the invariant there.
func TestCharacterXMLViaTermMapping(t *testing.T) {
	const target = `<target><terminology_id><value>SNOMED-CT</value></terminology_id>` +
		`<code_string>371532007</code_string></target>`

	t.Run("decode one character", func(t *testing.T) {
		var tm rm.TermMapping
		doc := `<term_mapping><match>=</match>` + target + `</term_mapping>`
		if err := canxml.Unmarshal([]byte(doc), &tm); err != nil {
			t.Fatalf("Unmarshal(%s): %v", doc, err)
		}
		if tm.Match != "=" {
			t.Errorf("match = %q, want \"=\"", string(tm.Match))
		}
	})

	for _, bad := range []string{"", "ab"} {
		t.Run("decode "+bad+" refused", func(t *testing.T) {
			var tm rm.TermMapping
			doc := `<term_mapping><match>` + bad + `</match>` + target + `</term_mapping>`
			err := canxml.Unmarshal([]byte(doc), &tm)
			if err == nil {
				t.Fatalf("Unmarshal(%s) = nil error, want a refusal; match = %q", doc, string(tm.Match))
			}
			// The XML path carries no typereg.ErrInvalidShape: that sentinel's
			// own text names canonical JSON, and canxml classifies nothing at
			// element level — it returns encoding/xml's errors unchanged and
			// reserves canxml.ErrInvalidShape for xmi:type rejection and for a
			// nil / non-xml.Marshaler root.
			if errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("err = %v; the canonical-JSON shape sentinel must not classify an XML element failure", err)
			}
			if !strings.Contains(err.Error(), "rm.Character") {
				t.Errorf("err = %v; want the rm.Character refusal to reach the caller", err)
			}
		})
	}

	t.Run("encode", func(t *testing.T) {
		tm := rm.TermMapping{
			Match: "=",
			Target: rm.CodePhrase{
				TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"},
				CodeString:    "371532007",
			},
		}
		out, err := canxml.Marshal(&tm)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if !strings.Contains(string(out), `<match>=</match>`) {
			t.Errorf("encoded form = %s, want <match>=</match>", out)
		}

		for _, bad := range []rm.Character{"", "ab"} {
			tm.Match = bad
			if _, err := canxml.Marshal(&tm); err == nil {
				t.Errorf("Marshal(match=%q) = nil error, want a refusal", string(bad))
			}
		}
	})
}

// TestCharacterJSONUnaffectedByTextCodec verifies — rather than assumes —
// the claim that adding encoding.TextUnmarshaler / TextMarshaler leaves the
// JSON surface alone. encoding/json prefers json.Unmarshaler over
// encoding.TextUnmarshaler, and the discriminating facet is the back-compat
// number arm (§ REQ-052's `TERM_MAPPING.match` bullet): a JSON number
// reaches UnmarshalJSON but
// would never reach UnmarshalText, and the digits "61" are not a Character,
// so if the text codec had taken over, 61 would fail instead of decoding
// to "=".
func TestCharacterJSONUnaffectedByTextCodec(t *testing.T) {
	var c rm.Character
	if err := json.Unmarshal([]byte("61"), &c); err != nil {
		t.Fatalf("Unmarshal(61): %v — the JSON number arm must still be reached", err)
	}
	if c != "=" {
		t.Errorf("Unmarshal(61): c = %q, want \"=\"", string(c))
	}

	out, err := json.Marshal(rm.Character("="))
	if err != nil {
		t.Fatalf("Marshal(\"=\"): %v", err)
	}
	if string(out) != `"="` {
		t.Errorf("Marshal(\"=\") = %s, want the quoted one-char string", out)
	}
}

// TestCharacterTextCodecDirect pins the text codec's own contract, which
// deliberately has no numeric back-compat arm — the pre-fix encoder wrote a
// number in JSON only, and XML element content carries no JSON number kind
// to be tolerant of. The rule here is the value rule alone: valid UTF-8,
// exactly one rune. U+FFFD satisfies it and round-trips; only the JSON
// string arm, which still holds the raw wire bytes, can tell a genuine
// U+FFFD from a substituted one, and it is the only entry point that
// tries. Bytes that are not valid UTF-8 are still refused here, for a
// direct caller that hands them over without going through encoding/xml.
func TestCharacterTextCodecDirect(t *testing.T) {
	for _, want := range []rm.Character{"≥", "\uFFFD"} {
		t.Run("round-trip "+string(want), func(t *testing.T) {
			var c rm.Character
			if err := c.UnmarshalText([]byte(want)); err != nil {
				t.Fatalf("UnmarshalText(%q): %v", string(want), err)
			}
			if c != want {
				t.Fatalf("c = %q, want %q", string(c), string(want))
			}
			out, err := c.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText(%q): %v", string(want), err)
			}
			if string(out) != string(want) {
				t.Errorf("MarshalText(%q) = %q, want %q", string(want), out, string(want))
			}
		})
	}

	refusals := map[string][]byte{
		"empty":          {},
		"two characters": []byte("ab"),
		"invalid UTF-8":  {0xff},
		"decimal digits": []byte("61"),
	}
	for name, text := range refusals {
		t.Run("decode "+name, func(t *testing.T) {
			var c rm.Character
			if err := c.UnmarshalText(text); err == nil {
				t.Errorf("UnmarshalText(%q) = nil error, want a refusal; c = %q", text, string(c))
			}
		})
		t.Run("encode "+name, func(t *testing.T) {
			if out, err := rm.Character(text).MarshalText(); err == nil {
				t.Errorf("MarshalText(%q) = %q, nil error; want a refusal", text, out)
			}
		})
	}
}
