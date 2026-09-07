package simplified_test

// REQ-025 / REQ-053 — ParseMediaType is a parser fed header values from the
// network, so it gets the no-panic property checked on arbitrary input, along
// with the closed outcome set its doc comment promises.

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
)

// FuzzParseMediaType asserts three things on any input at all: the call never
// panics (REQ-025), it returns exactly one of the two shapes its contract
// allows — a known Format with a nil error, or FormatUnknown with an error
// matching ErrUnknownMediaType — and a classified Format's canonical media
// type classifies back to that same Format.
func FuzzParseMediaType(f *testing.F) {
	for _, tc := range parseMediaTypeCases {
		f.Add(tc.in)
	}
	f.Fuzz(func(t *testing.T, in string) {
		got, err := simplified.ParseMediaType(in) // a panic here fails the target (REQ-025)
		switch got {
		case simplified.FormatFlat, simplified.FormatStructured:
			if err != nil {
				t.Fatalf("ParseMediaType(%q) = %v, %v; want a nil error alongside a classified format", in, got, err)
			}
			canonical := got.MediaType()
			back, backErr := simplified.ParseMediaType(canonical)
			if back != got || backErr != nil {
				t.Fatalf("ParseMediaType(%q) = %v, then ParseMediaType(%v.MediaType() = %q) = %v, %v; want %v, nil", in, got, got, canonical, back, backErr, got)
			}
		case simplified.FormatUnknown:
			if !errors.Is(err, simplified.ErrUnknownMediaType) {
				t.Fatalf("ParseMediaType(%q) = FormatUnknown, %v; want an error matching ErrUnknownMediaType", in, err)
			}
		default:
			t.Fatalf("ParseMediaType(%q) = %v (%d), %v; want FormatFlat, FormatStructured or FormatUnknown", in, got, int(got), err)
		}
	})
}
