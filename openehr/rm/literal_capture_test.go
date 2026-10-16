package rm_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

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
			de, ok := typereg.EntryError(err).(*typereg.DecodeError) //nolint:errorlint // the envelope is the error itself
			if !ok || de == nil {
				t.Fatalf("typereg.EntryError(%v) is not a *typereg.DecodeError", err)
			}
			if got := de.Value.Reveal(); got != tc.in {
				t.Errorf("UnmarshalJSON(%s): the entry point's Value.Reveal() = %#v; want the literal as written, %#v", tc.in, got, tc.in)
			}
		})
	}
}
