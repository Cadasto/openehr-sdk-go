package rm_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestQuotedLiteralParseErrorNamesTheLiteralOnce pins wire.md § REQ-052
// "Causes may name the literal": the *strconv.NumError beneath a quoted-arm
// parse failure already quotes the offending literal, so the codec's own
// prefix must not repeat it. Restoring the former `parse %q: %w` wrap fails
// this test with a count of two; dropping the %w wrap fails the errors.AsType
// arm.
func TestQuotedLiteralParseErrorNamesTheLiteralOnce(t *testing.T) { // REQ-052
	t.Parallel()
	cases := []struct {
		name    string
		decode  func([]byte) error
		literal string
	}{
		{"Real", func(b []byte) error { var r rm.Real; return r.UnmarshalJSON(b) }, "12x"},
		{"Integer", func(b []byte) error { var i rm.Integer; return i.UnmarshalJSON(b) }, "7y"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.decode([]byte(strconv.Quote(tc.literal)))
			if err == nil {
				t.Fatalf("%s.UnmarshalJSON(%q) = nil, want a parse error", tc.name, tc.literal)
			}
			if n := strings.Count(err.Error(), tc.literal); n != 1 {
				t.Errorf("%s.UnmarshalJSON(%q) err = %q names the literal %d times, want exactly once (the strconv cause quotes it)", tc.name, tc.literal, err, n)
			}
			if _, ok := errors.AsType[*strconv.NumError](err); !ok {
				t.Errorf("%s.UnmarshalJSON(%q) err = %v (%T); want the *strconv.NumError reachable through the wrap", tc.name, tc.literal, err, err)
			}
		})
	}
}
