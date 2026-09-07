package simplified_test

// REQ-053 — media-type negotiation. ParseMediaType accepts the two canonical
// Simplified Formats strings and the deprecated `.schema`-suffixed variants
// (retired from the specification, still served by EHRbase) on input;
// Format.MediaType emits only the canonical strings.

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
)

// parseMediaTypeCase is one media-type token and the Format ParseMediaType
// must return for it. A want of FormatUnknown means the token must be refused
// with ErrUnknownMediaType; any other want means it must classify with a nil
// error. Those two are the whole outcome set — FuzzParseMediaType asserts that
// on arbitrary input, and deriving wantErr from want here keeps both readings
// of the contract in one place.
type parseMediaTypeCase struct {
	name string
	in   string
	want simplified.Format
}

// parseMediaTypeCases is shared with FuzzParseMediaType, which seeds its
// corpus from these inputs. Every case pins the contract clause named in its
// trailing comment.
var parseMediaTypeCases = []parseMediaTypeCase{
	// Accepted vocabulary: the two canonical strings (REQ-053 MUST).
	{"flat canonical", "application/openehr.wt.flat+json", simplified.FormatFlat},
	{"structured canonical", "application/openehr.wt.structured+json", simplified.FormatStructured},
	// The deprecated `.schema`-suffixed variants — retired from the
	// specification, still served by EHRbase — accepted on input only
	// (REQ-053 SHOULD).
	{"flat schema variant", "application/openehr.wt.flat.schema+json", simplified.FormatFlat},
	{"structured schema variant", "application/openehr.wt.structured.schema+json", simplified.FormatStructured},
	// "matched case-insensitively (RFC 2045)" — mime.ParseMediaType lower-cases
	// the type it returns, so the package does no case folding of its own and
	// the lookup map is keyed lower-case. Stdlib behaviour the contract rests
	// on, pinned here so an upstream change surfaces as a failure.
	{"mixed case", "Application/OpenEHR.WT.Flat+JSON", simplified.FormatFlat},
	// "every parameter is ignored".
	{"charset parameter ignored", "application/openehr.wt.flat+json; charset=utf-8", simplified.FormatFlat},
	{"schema variant with charset", "application/openehr.wt.structured.schema+json;charset=UTF-8", simplified.FormatStructured},
	// "`q` included, so a `q=0` range still classifies" — this call does no
	// Accept negotiation, so a range the caller should have dropped still
	// classifies; dropping it is the caller's job.
	{"q zero still classifies", "application/openehr.wt.flat+json; q=0", simplified.FormatFlat},
	// mime.ParseMediaType tolerates surrounding whitespace; observed, so pinned.
	{"surrounding whitespace tolerated", " application/openehr.wt.flat+json ", simplified.FormatFlat},

	// "A comma-separated Accept list is not accepted: split it upstream."
	// mime.ParseMediaType rejects the second entry as content after the
	// subtype, so the whole list is refused rather than silently read as its
	// first range.
	{"accept list refused", "application/openehr.wt.flat+json, application/json", simplified.FormatUnknown},

	// "a malformed parameter included too: a broken parameter beside an
	// otherwise unambiguous type still classifies on that type".
	// mime.ParseMediaType reports mime.ErrInvalidMediaParameter for all three
	// of these while still returning the recognised type, so ParseMediaType
	// classifies on that type rather than refusing a body whose format is not
	// in doubt (REQ-053 is liberal on input, and the codec validates the bytes
	// anyway).
	{"parameter without value", "application/openehr.wt.flat+json; charset", simplified.FormatFlat},
	{"parameter without name", "application/openehr.wt.flat+json; =utf-8", simplified.FormatFlat},
	{"empty parameter", "application/openehr.wt.flat+json;;", simplified.FormatFlat},
	// A duplicate parameter name is a different error class — "mime: duplicate
	// parameter name", not ErrInvalidMediaParameter — and mime.ParseMediaType
	// returns no type alongside it, so there is nothing to classify on and the
	// value is refused.
	{"duplicate parameter name", "application/openehr.wt.flat+json; charset=a; charset=b", simplified.FormatUnknown},
	// "Only a value whose type part itself does not parse is refused" — a
	// well-formed parameter does not rescue a broken type part, and neither
	// value yields a type to look up ("mime: expected token after slash",
	// "mime: no media type").
	{"empty subtype with valid parameter", "application/; charset=utf-8", simplified.FormatUnknown},
	{"missing type before slash", "/flat+json", simplified.FormatUnknown},

	// "Anything naming neither format … fails with ErrUnknownMediaType": the
	// accepted vocabulary is four exact subtypes, so every near-miss spelling
	// is refused rather than guessed at.
	{"plural subtype", "application/openehr.wt.flats+json", simplified.FormatUnknown},
	{"suffix jsonx", "application/openehr.wt.flat+jsonx", simplified.FormatUnknown},
	{"versioned schema subtype", "application/openehr.wt.flat.schema.v2+json", simplified.FormatUnknown},
	{"no suffix", "application/openehr.wt.flat", simplified.FormatUnknown},
	{"schema variant as xml", "application/openehr.wt.flat.schema+xml", simplified.FormatUnknown},
	{"wrong top-level type", "text/openehr.wt.flat+json", simplified.FormatUnknown},
	{"x-prefixed subtype", "application/x-openehr.wt.flat+json", simplified.FormatUnknown},
	{"extra schema suffix", "application/openehr.wt.flat+json+schema", simplified.FormatUnknown},
	{"schema without json suffix", "application/openehr.wt.flat.schema", simplified.FormatUnknown},
	{"double suffix", "application/openehr.wt.flat+json+xml", simplified.FormatUnknown},
	// The WebTemplate resource type is a template projection, not a
	// composition format, so it is named in the doc comment as refused.
	{"webtemplate resource type", "application/openehr.wt+json", simplified.FormatUnknown},
	// Other media types and unparseable values take the same refusal.
	{"empty string", "", simplified.FormatUnknown},
	{"plain json", "application/json", simplified.FormatUnknown},
	{"text plain", "text/plain", simplified.FormatUnknown},
	{"not a media type", "not a media type", simplified.FormatUnknown},

	// REQ-025 no-panic, pinned the way this repo pins it: a named table of
	// hostile inputs beside FuzzParseMediaType, so the property holds as a
	// deterministic test and not only under a fuzz run. Each row records what
	// mime.ParseMediaType actually does with the value — several of these
	// classify under the liberal-parameter rule rather than being refused.
	{"semicolon only", ";", simplified.FormatUnknown},                     // "mime: no media type"
	{"slash only", "/", simplified.FormatUnknown},                         // "mime: no media type"
	{"type with empty subtype", "application/", simplified.FormatUnknown}, // "mime: expected token after slash"
	// An unterminated quoted parameter value is ErrInvalidMediaParameter with
	// the type intact, so it classifies under the liberal-parameter rule.
	{"unterminated quoted parameter", `application/openehr.wt.flat+json; charset="utf`, simplified.FormatFlat},
	// An RFC 2231 extended parameter parses cleanly (charset=x), so this one
	// classifies with no error at all — nothing hostile reaches the lookup.
	{"rfc 2231 parameter continuation", "application/openehr.wt.flat+json; charset*=utf-8''x", simplified.FormatFlat},
	// Control bytes after the subtype are content, not trailing whitespace:
	// "mime: unexpected content after media subtype", so no type comes back.
	{"nul byte after subtype", "application/openehr.wt.flat+json\x00", simplified.FormatUnknown},
	{"del byte after subtype", "application/openehr.wt.flat+json\x7f", simplified.FormatUnknown},
	// 10 KiB of one token character: mime.ParseMediaType accepts it as a type
	// with no slash and no error, so it reaches the lookup and misses there.
	{"ten kibibyte token", strings.Repeat("a", 10*1024), simplified.FormatUnknown},
}

func TestParseMediaType(t *testing.T) { // REQ-053
	t.Parallel()
	for _, tc := range parseMediaTypeCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := simplified.ParseMediaType(tc.in)
			if tc.want == simplified.FormatUnknown {
				if got != simplified.FormatUnknown || !errors.Is(err, simplified.ErrUnknownMediaType) {
					t.Errorf("ParseMediaType(%q) = %v, %v; want FormatUnknown and an error matching ErrUnknownMediaType", tc.in, got, err)
				}
				return
			}
			if got != tc.want || err != nil {
				t.Errorf("ParseMediaType(%q) = %v, %v; want %v, nil", tc.in, got, err, tc.want)
			}
		})
	}
}

func TestFormatMediaTypeEmitsCanonicalOnly(t *testing.T) { // REQ-053: the .schema variants MUST NOT be emitted
	t.Parallel()
	if got := simplified.FormatFlat.MediaType(); got != simplified.MediaTypeFlat {
		t.Errorf("FormatFlat.MediaType() = %q, want %q", got, simplified.MediaTypeFlat)
	}
	if got := simplified.FormatStructured.MediaType(); got != simplified.MediaTypeStructured {
		t.Errorf("FormatStructured.MediaType() = %q, want %q", got, simplified.MediaTypeStructured)
	}
	if got := simplified.FormatUnknown.MediaType(); got != "" {
		t.Errorf("FormatUnknown.MediaType() = %q, want empty", got)
	}
	if got := simplified.Format(99).MediaType(); got != "" {
		t.Errorf("Format(99).MediaType() = %q, want empty", got)
	}
	for _, f := range []simplified.Format{simplified.FormatFlat, simplified.FormatStructured} {
		if strings.Contains(f.MediaType(), ".schema") {
			t.Errorf("%v.MediaType() = %q emits a deprecated .schema variant; REQ-053 forbids emitting it", f, f.MediaType())
		}
		if back, err := simplified.ParseMediaType(f.MediaType()); err != nil || back != f {
			t.Errorf("ParseMediaType(%v.MediaType()) = %v, %v; want %v, nil", f, back, err, f)
		}
	}
}

func TestFormatString(t *testing.T) {
	t.Parallel()
	for f, want := range map[simplified.Format]string{
		simplified.FormatUnknown: "unknown", simplified.FormatFlat: "FLAT", simplified.FormatStructured: "STRUCTURED", simplified.Format(7): "Format(7)",
	} {
		if got := f.String(); got != want {
			t.Errorf("Format(%d).String() = %q, want %q", int(f), got, want)
		}
	}
}
