package rm_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/refused"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// TestQuotedLiteralRefusal_REQ052_CarriesTheLiteral pins what rm.Real and
// rm.Integer hand a decode entry point when they refuse a quoted literal they
// cannot parse (REQ-052): the JSON text of the literal, quotes and escapes as
// the document wrote them, which the entry point's envelope holds in Value.
// The refusal's own message and causes are unchanged: the literal appears in
// it once, in the strconv cause.
//
// Can-fail control: return the refusal without the literal and the Value
// assertions fail.
func TestQuotedLiteralRefusal_REQ052_CarriesTheLiteral(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		decode  func(b []byte) error
		message string
		range_  bool
	}{
		{
			name:    "Real, not a number",
			in:      `"1.2.3"`,
			decode:  func(b []byte) error { var r rm.Real; return r.UnmarshalJSON(b) },
			message: `rm.Real: parse quoted literal: strconv.ParseFloat: parsing "1.2.3": invalid syntax`,
		},
		{
			name:    "Real, out of range",
			in:      `"1e400"`,
			decode:  func(b []byte) error { var r rm.Real; return r.UnmarshalJSON(b) },
			message: `rm.Real: parse quoted literal: strconv.ParseFloat: parsing "1e400": value out of range`,
			range_:  true,
		},
		{
			name:    "Integer, a fraction",
			in:      `"1.5"`,
			decode:  func(b []byte) error { var i rm.Integer; return i.UnmarshalJSON(b) },
			message: `rm.Integer: parse quoted literal: strconv.ParseInt: parsing "1.5": invalid syntax`,
		},
		{
			name:    "Integer, out of range",
			in:      `"1e10"`,
			decode:  func(b []byte) error { var i rm.Integer; return i.UnmarshalJSON(b) },
			message: `rm.Integer: parse quoted literal: strconv.ParseInt: parsing "1e10": value out of range`,
			range_:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.decode([]byte(tc.in))
			if err == nil {
				t.Fatalf("UnmarshalJSON(%s) = nil; want a refusal", tc.in)
			}
			if err.Error() != tc.message {
				t.Errorf("UnmarshalJSON(%s) = %q; want %q", tc.in, err, tc.message)
			}
			if got := strings.Count(err.Error(), strings.Trim(strings.ReplaceAll(tc.in, `.`, "."), `"`)); got != 1 {
				t.Errorf("UnmarshalJSON(%s) = %q names the literal %d times; want once, in the cause", tc.in, err, got)
			}
			if _, ok := errors.AsType[*strconv.NumError](err); !ok {
				t.Errorf("UnmarshalJSON(%s) = %v; want the *strconv.NumError reachable", tc.in, err)
			}
			if got := errors.Is(err, strconv.ErrRange); got != tc.range_ {
				t.Errorf("UnmarshalJSON(%s): errors.Is(_, strconv.ErrRange) = %t; want %t", tc.in, got, tc.range_)
			}
			if errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("UnmarshalJSON(%s) = %v; a parse failure must not carry typereg.ErrInvalidShape", tc.in, err)
			}
			if got := carried(err); got != tc.in {
				t.Errorf("UnmarshalJSON(%s) carries %q for the entry point's Value; want the literal as written, %q", tc.in, got, tc.in)
			}
		})
	}
}

// TestPrimitiveRefusal_REQ052_CarriesTheLiteral pins the other refusals of
// the SDK's primitive codecs that hand a decode entry point the literal
// (REQ-052): rm.Real past its 17-digit budget, bare or quoted, and
// rm.Character for a string that is not exactly one character. Each carries
// the JSON text as written while its message stays as it was and it still
// carries typereg.ErrInvalidShape. A value of the wrong JSON kind, and a
// number rm.Character takes as no usable code point, carry nothing.
//
// Can-fail control: return either refusal without the literal and the
// matching rows' carried-text assertion fails; carry a literal on the
// last two rows and theirs fails.
func TestPrimitiveRefusal_REQ052_CarriesTheLiteral(t *testing.T) {
	decodeReal := func(b []byte) error { var r rm.Real; return r.UnmarshalJSON(b) }
	decodeChar := func(b []byte) error { var c rm.Character; return c.UnmarshalJSON(b) }
	cases := []struct {
		name    string
		in      string
		decode  func(b []byte) error
		message string // the whole message, when the SDK writes it
		carried string // "" when the refusal carries no literal
	}{
		{
			name:    "Real past its digit budget, bare",
			in:      `0.1234567890123456789`,
			decode:  decodeReal,
			message: "rm.Real: literal carries more than 17 significant decimal digits",
			carried: `0.1234567890123456789`,
		},
		{
			name:    "Real past its digit budget, quoted",
			in:      `"18446744073709551616"`,
			decode:  decodeReal,
			message: "rm.Real: literal carries more than 17 significant decimal digits",
			carried: `"18446744073709551616"`,
		},
		{
			name:    "Character of two characters, escaped",
			in:      `"\u03a9="`,
			decode:  decodeChar,
			message: "rm.Character: must be exactly one character, got 2",
			carried: `"\u03a9="`,
		},
		{
			name:    "Character of no character",
			in:      `""`,
			decode:  decodeChar,
			message: "rm.Character: must be exactly one character, got 0",
			carried: `""`,
		},
		{
			name:   "Character of the wrong JSON kind",
			in:     `true`,
			decode: decodeChar,
			// No message pin: encoding/json/v2 varies its own wording.
		},
		{
			name:    "Character as a number naming no usable code point",
			in:      `0`,
			decode:  decodeChar,
			message: "rm.Character: number is not a usable code point",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.decode([]byte(tc.in))
			if err == nil {
				t.Fatalf("UnmarshalJSON(%s) = nil; want a refusal", tc.in)
			}
			if tc.message != "" && err.Error() != tc.message {
				t.Errorf("UnmarshalJSON(%s) = %q; want %q, the message unchanged", tc.in, err, tc.message)
			}
			if !errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("UnmarshalJSON(%s) = %v; want errors.Is(_, typereg.ErrInvalidShape)", tc.in, err)
			}
			if got := carried(err); got != tc.carried {
				t.Errorf("UnmarshalJSON(%s) carries %q for the entry point's Value; want %q", tc.in, got, tc.carried)
			}
		})
	}
}

// carried returns the literal err carries for a decode entry point's
// DecodeError.Value, or "" when it carries none.
func carried(err error) string {
	e, ok := errors.AsType[*refused.Error](err)
	if !ok {
		return ""
	}
	return e.Text()
}
