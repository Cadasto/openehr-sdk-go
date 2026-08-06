package ehr_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
)

func TestItemTagHeaderRoundTrip(t *testing.T) {
	in := []ehr.ItemTag{
		{Key: "category", Value: "final"},
		{Key: `say "hi"`, Value: `with "quote"`, TargetPath: "/path"},
	}
	encoded, err := ehr.FormatItemTagHeader(in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ehr.ParseItemTagHeader(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Key != "category" || got[1].Key != `say "hi"` {
		t.Fatalf("got = %#v", got)
	}
}

// TestItemTagHeaderRoundTripBackslash verifies a literal backslash survives
// encode→parse (the escaper escapes \ as well as ").
func TestItemTagHeaderRoundTripBackslash(t *testing.T) {
	in := []ehr.ItemTag{{Key: "path", Value: `C:\temp\"x"`}}
	encoded, err := ehr.FormatItemTagHeader(in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ehr.ParseItemTagHeader(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Value != `C:\temp\"x"` {
		t.Fatalf("backslash did not round-trip: encoded=%q got=%#v", encoded, got)
	}
}

// TestFormatItemTagHeader_ControlCharsRejected verifies that CR/LF/NUL in any
// of Key, Value, or TargetPath cause FormatItemTagHeader to return a non-nil
// error mentioning "control characters", and that the returned string is empty.
func TestFormatItemTagHeader_ControlCharsRejected(t *testing.T) {
	cases := []struct {
		name string
		tag  ehr.ItemTag
	}{
		{
			name: "CR LF in Key",
			tag:  ehr.ItemTag{Key: "k\r\nX-Evil: 1", Value: "v"},
		},
		{
			name: "LF in Value",
			tag:  ehr.ItemTag{Key: "k", Value: "v\ninjected"},
		},
		{
			name: "CR in TargetPath",
			tag:  ehr.ItemTag{Key: "k", Value: "v", TargetPath: "/path\rbad"},
		},
		{
			name: "NUL in Key",
			tag:  ehr.ItemTag{Key: "k\x00z"},
		},
		{
			name: "DEL in Value",
			tag:  ehr.ItemTag{Key: "k", Value: "v\x7fz"},
		},
		// The guard is defined over bytes, so a control byte must still be
		// seen when it sits next to multi-byte UTF-8. (These pass under a
		// rune-ranging loop too — a continuation byte is always 0x80–0xBF
		// and so can never itself be C0 or DEL — but they pin the RFC 9110
		// contract the doc comment states.)
		{
			name: "LF after a multi-byte rune",
			tag:  ehr.ItemTag{Key: "k", Value: "café\ninjected"},
		},
		{
			name: "DEL between multi-byte runes",
			tag:  ehr.ItemTag{Key: "k", Value: "é\x7fé"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ehr.FormatItemTagHeader([]ehr.ItemTag{tc.tag})
			if err == nil {
				t.Fatalf("expected error, got nil (result=%q)", got)
			}
			if !strings.Contains(err.Error(), "control characters") {
				t.Errorf("error should mention 'control characters', got: %v", err)
			}
			if !strings.Contains(err.Error(), "item tag[0]") {
				t.Errorf("error should identify the offending tag index, got: %v", err)
			}
			if got != "" {
				t.Errorf("expected empty string on error, got %q", got)
			}
		})
	}
}

// TestFormatItemTagHeader_TabAllowed verifies that a horizontal tab (0x09)
// inside a value does not cause an error (RFC 9110 permits HT in field values).
func TestFormatItemTagHeader_TabAllowed(t *testing.T) {
	tags := []ehr.ItemTag{
		{Key: "k\tey", Value: "v\twith\ttabs", TargetPath: "/p\tath"},
	}
	got, err := ehr.FormatItemTagHeader(tags)
	if err != nil {
		t.Fatalf("tab should be allowed, got error: %v", err)
	}
	if got == "" {
		t.Fatal("expected non-empty result")
	}
}
