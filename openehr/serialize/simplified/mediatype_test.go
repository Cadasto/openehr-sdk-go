package simplified_test

// REQ-053 — media-type negotiation. ParseMediaType accepts the two canonical
// Simplified Formats strings and EHRbase's `.schema`-suffixed variants on
// input; Format.MediaType emits only the canonical strings.

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
	// EHRbase's `.schema`-suffixed variants, accepted on input only (REQ-053 SHOULD).
	{"flat schema variant", "application/openehr.wt.flat.schema+json", simplified.FormatFlat},
	{"structured schema variant", "application/openehr.wt.structured.schema+json", simplified.FormatStructured},
	// "matched case-insensitively (RFC 2045)".
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

	// "a malformed parameter still fails the whole value" — mime.ParseMediaType
	// reports "invalid media parameter" for all three of these while still
	// returning the recognised type, and ParseMediaType checks the error first.
	{"parameter without value", "application/openehr.wt.flat+json; charset", simplified.FormatUnknown},
	{"parameter without name", "application/openehr.wt.flat+json; =utf-8", simplified.FormatUnknown},
	{"empty parameter", "application/openehr.wt.flat+json;;", simplified.FormatUnknown},

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
			t.Errorf("%v.MediaType() = %q emits the EHRbase .schema variant; REQ-053 forbids emitting it", f, f.MediaType())
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
