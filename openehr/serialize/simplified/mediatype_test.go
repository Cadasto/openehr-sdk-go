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

func TestParseMediaType(t *testing.T) { // REQ-053
	t.Parallel()
	cases := []struct {
		in   string
		want simplified.Format
	}{
		{"application/openehr.wt.flat+json", simplified.FormatFlat},
		{"application/openehr.wt.structured+json", simplified.FormatStructured},
		{"application/openehr.wt.flat.schema+json", simplified.FormatFlat},             // EHRbase variant
		{"application/openehr.wt.structured.schema+json", simplified.FormatStructured}, // EHRbase variant
		{"Application/OpenEHR.WT.Flat+JSON", simplified.FormatFlat},                    // media types are case-insensitive
		{"application/openehr.wt.flat+json; charset=utf-8", simplified.FormatFlat},     // parameters ignored
		{"application/openehr.wt.structured.schema+json;charset=UTF-8", simplified.FormatStructured},
	}
	for _, tc := range cases {
		got, err := simplified.ParseMediaType(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("ParseMediaType(%q) = %v, %v; want %v, nil", tc.in, got, err, tc.want)
		}
	}
	for _, in := range []string{
		"",
		"application/json",
		"text/plain",
		"application/openehr.wt+json",          // the WebTemplate resource type is not a composition format
		"application/openehr.wt.flat.schema",   // suffix missing
		"application/openehr.wt.flat+json+xml", // not a registered spelling
		"not a media type",
	} {
		got, err := simplified.ParseMediaType(in)
		if !errors.Is(err, simplified.ErrUnknownMediaType) || got != simplified.FormatUnknown {
			t.Errorf("ParseMediaType(%q) = %v, %v; want FormatUnknown, errors.Is ErrUnknownMediaType", in, got, err)
		}
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
