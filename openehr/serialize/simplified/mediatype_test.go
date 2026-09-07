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
	// "well-formed parameters are ignored whether or not this package
	// recognises them".
	{"charset parameter ignored", "application/openehr.wt.flat+json; charset=utf-8", simplified.FormatFlat},
	{"schema variant with charset", "application/openehr.wt.structured.schema+json;charset=UTF-8", simplified.FormatStructured},
	// "`q` included, so a `q=0` range still classifies" — a `q` parameter on a
	// single range is well-formed, and this call does no Accept negotiation,
	// so dropping a range the caller disabled is the caller's job. A `q=0`
	// range inside a comma-separated list is a different value entirely and is
	// refused; see the q-weighted rows below.
	{"q zero still classifies", "application/openehr.wt.flat+json; q=0", simplified.FormatFlat},
	// mime.ParseMediaType tolerates surrounding whitespace; observed, so pinned.
	{"surrounding whitespace tolerated", " application/openehr.wt.flat+json ", simplified.FormatFlat},

	// "A comma-separated Accept list is not accepted: split it upstream."
	// A plain list trips "mime: unexpected content after media subtype" and no
	// type comes back, so refusing it needs no rule of its own.
	{"accept list refused", "application/openehr.wt.flat+json, application/json", simplified.FormatUnknown},
	// A q-weighted list is the case that makes the strict rule load-bearing:
	// mime.ParseMediaType reads the first entry's type, then reports the
	// comma-bearing parameter as mime.ErrInvalidMediaParameter. Classifying on
	// the returned type would silently answer with whichever range happens to
	// come first — including one the caller disabled with q=0 — so every parse
	// error refuses. Both orders are pinned so the answer cannot be right by
	// accident of ordering.
	{"q weighted accept list flat first", "application/openehr.wt.flat+json;q=0, application/openehr.wt.structured+json;q=1", simplified.FormatUnknown},
	{"q weighted accept list structured first", "application/openehr.wt.structured+json;q=1, application/openehr.wt.flat+json;q=0", simplified.FormatUnknown},
	{"q weighted accept list with space", "application/openehr.wt.flat+json; q=0.8, application/openehr.wt.structured+json", simplified.FormatUnknown},

	// "A value whose type part or parameters do not parse is refused." These
	// three are mime.ErrInvalidMediaParameter with the recognised type still
	// returned; the type is not classified on, because a value the stdlib
	// cannot finish parsing may hold more than the one range it appears to.
	{"parameter without value", "application/openehr.wt.flat+json; charset", simplified.FormatUnknown},
	{"parameter without name", "application/openehr.wt.flat+json; =utf-8", simplified.FormatUnknown},
	{"empty parameter", "application/openehr.wt.flat+json;;", simplified.FormatUnknown},
	// A duplicate parameter name is a different error class — "mime: duplicate
	// parameter name", not ErrInvalidMediaParameter — and mime.ParseMediaType
	// returns no type alongside it. Same refusal, one arm earlier.
	{"duplicate parameter name", "application/openehr.wt.flat+json; charset=a; charset=b", simplified.FormatUnknown},
	// A well-formed parameter does not rescue a broken type part, and neither
	// value yields a type to look up ("mime: expected token after slash",
	// "mime: no media type" respectively).
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
	{"not a media type", "not a media type", simplified.FormatUnknown}, // "mime: expected slash after first token"

	// REQ-025 no-panic, pinned the way this repo pins it: a named table of
	// hostile inputs beside FuzzParseMediaType, so the property holds as a
	// deterministic test and not only under a fuzz run. Each row records what
	// mime.ParseMediaType actually does with the value; all but one are
	// refused, and the refusal is what the row pins.
	{"semicolon only", ";", simplified.FormatUnknown},                     // "mime: no media type"
	{"slash only", "/", simplified.FormatUnknown},                         // "mime: no media type"
	{"type with empty subtype", "application/", simplified.FormatUnknown}, // "mime: expected token after slash"
	// An unterminated quoted parameter value is ErrInvalidMediaParameter with
	// the type intact — the same shape as the q-weighted list above, and the
	// same refusal.
	{"unterminated quoted parameter", `application/openehr.wt.flat+json; charset="utf`, simplified.FormatUnknown},
	// The one row here that is not refused: an RFC 2231 extended parameter
	// parses cleanly (charset=x), so it is an ignored well-formed parameter
	// and the type classifies with no error at all.
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
