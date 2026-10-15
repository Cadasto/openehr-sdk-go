package rm_test

import (
	json "encoding/json/v2"
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// TestIntegerNilReceiverIsRefusedNotPanicked pins the nil-receiver axis
// (idiom.md § No panics, REQ-025): UnmarshalJSON assigns through the
// pointer, so a nil *rm.Integer would be dereferenced. A nil pointer is
// caller-constructible input reachable through the documented API, not a
// programmer error, so the method must fail closed with a plain error —
// caller misuse is not a wire-shape problem and must stay outside
// typereg.ErrInvalidShape. A panic fails the test run outright.
//
// `null` is the discriminating second input: encoding/json treats a null
// literal as a no-op for a non-pointer target, so it reaches the
// assignment with nothing to report, and a nil check placed after the
// decode would let it through and panic on a value that is not even
// there.
func TestIntegerNilReceiverIsRefusedNotPanicked(t *testing.T) {
	for _, in := range []string{"42", `"42"`, "null"} {
		t.Run(in, func(t *testing.T) {
			var i *rm.Integer
			err := i.UnmarshalJSON([]byte(in))
			if err == nil {
				t.Fatalf("(*rm.Integer)(nil).UnmarshalJSON(%s) = nil error, want a refusal", in)
			}
			if errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("err = %v; caller misuse must not classify as a wire-shape failure", err)
			}
		})
	}
}

// TestIntegerWholeNumberLiteralDecodes pins wire.md § Floating-point
// precision (REQ-052, ADR 0004 as amended) on rm.Integer itself: a number
// whose value is a whole number decodes as that integer however the literal
// writes it, with a zero fraction or an exponent, bare or quoted. The int32
// bounds are accepted in every form, and the integer forms decode as before.
func TestIntegerWholeNumberLiteralDecodes(t *testing.T) { // REQ-052
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want rm.Integer
	}{
		{in: `1.0`, want: 1},
		{in: `1e0`, want: 1},
		{in: `1E1`, want: 10},
		{in: `1e+1`, want: 10},
		{in: `10.00`, want: 10},
		{in: `10e-1`, want: 1},
		{in: `0.1e1`, want: 1},
		{in: `-3.000`, want: -3},
		{in: `-0.0`, want: 0},
		{in: `0e18446744073709551616`, want: 0},
		{in: `2147483647.0`, want: math.MaxInt32},
		{in: `2.147483647e9`, want: math.MaxInt32},
		{in: `-2147483648.0`, want: math.MinInt32},
		{in: `-21474836480e-1`, want: math.MinInt32},
		{in: `"1.0"`, want: 1},
		{in: `"1e0"`, want: 1},
		{in: `"-2147483648.0"`, want: math.MinInt32},
		{in: `42`, want: 42},
		{in: `"42"`, want: 42},
		{in: `-2147483648`, want: math.MinInt32},
	} {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			var got rm.Integer
			if err := got.UnmarshalJSON([]byte(tc.in)); err != nil {
				t.Fatalf("rm.Integer.UnmarshalJSON(%s) = %v, want nil", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("rm.Integer.UnmarshalJSON(%s) decoded %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// TestIntegerRefusesFractionOrOutOfRange pins the refusal half of wire.md
// § Floating-point precision (REQ-052): a number with a non-zero fraction,
// or one outside the int32 range, is refused with a typed error and never
// truncated or rounded, so the receiver keeps its value. The refusal is the
// codec cause it was before whole numbers were accepted: a
// *json.SemanticError for a bare number and a *strconv.NumError for a
// quoted one, carrying no typereg.ErrInvalidShape at this level.
//
// Some rows catch an implementation that cuts corners.
// 1.0000000000000001, bare or quoted, reads as exactly 1 through a float64.
// An exponent of 2^64 wraps to 0 in int64 arithmetic, which would read
// 1e18446744073709551616 as 1 and 10e-18446744073709551616 as 10.
func TestIntegerRefusesFractionOrOutOfRange(t *testing.T) { // REQ-052
	t.Parallel()
	for _, in := range []string{
		// A non-zero fraction.
		`1.5`,
		`-1.5`,
		`0.5`,
		`123e-2`,
		`1e-1`,
		`1.0000000000000001`,
		`10e-18446744073709551616`,
		`"1.5"`,
		`"1.0000000000000001"`,
		// Outside the int32 range.
		`2147483648`,
		`2147483648.0`,
		`-2147483649.0`,
		`2.147483648e9`,
		`1e10`,
		`1e18446744073709551616`,
		`"2147483648.0"`,
		`"1e10"`,
	} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			const prior rm.Integer = 7
			got := prior
			err := got.UnmarshalJSON([]byte(in))
			if err == nil {
				t.Fatalf("rm.Integer.UnmarshalJSON(%s) = nil error, decoded %d; want a refusal", in, got)
			}
			if got != prior {
				t.Errorf("rm.Integer.UnmarshalJSON(%s) refused but changed the receiver from %d to %d", in, prior, got)
			}
			if errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("rm.Integer.UnmarshalJSON(%s) err = %v; a direct refusal carries no shape sentinel", in, err)
			}
			if strings.HasPrefix(in, `"`) {
				if _, ok := errors.AsType[*strconv.NumError](err); !ok {
					t.Errorf("rm.Integer.UnmarshalJSON(%s) err = %v (%T); want a *strconv.NumError beneath it", in, err, err)
				}
				return
			}
			if _, ok := errors.AsType[*json.SemanticError](err); !ok {
				t.Errorf("rm.Integer.UnmarshalJSON(%s) err = %v (%T); want a *json.SemanticError beneath it", in, err, err)
			}
		})
	}
}

// TestIntegerRefusesMalformedNumberLiteral pins that reading a whole number
// in a decimal form loosens no literal grammar (REQ-052): a bare literal
// that is not a JSON number, and a quoted one that is neither an integer nor
// a JSON number, are still refused and leave the receiver as it was. A
// quoted refusal keeps the *strconv.NumError beneath it.
func TestIntegerRefusesMalformedNumberLiteral(t *testing.T) { // REQ-052
	t.Parallel()
	for _, in := range []string{
		`01.0`,
		`1.`,
		`.5`,
		`+1.0`,
		`1e`,
		`1e+`,
		`1.0x`,
		`-`,
		`"1."`,
		`".5"`,
		`"1e"`,
		`" 1.0"`,
		`"1.0x"`,
		`"0x10"`,
	} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			const prior rm.Integer = 7
			got := prior
			err := got.UnmarshalJSON([]byte(in))
			if err == nil {
				t.Fatalf("rm.Integer.UnmarshalJSON(%s) = nil error, decoded %d; want a refusal", in, got)
			}
			if got != prior {
				t.Errorf("rm.Integer.UnmarshalJSON(%s) refused but changed the receiver from %d to %d", in, prior, got)
			}
			if strings.HasPrefix(in, `"`) {
				if _, ok := errors.AsType[*strconv.NumError](err); !ok {
					t.Errorf("rm.Integer.UnmarshalJSON(%s) err = %v (%T); want a *strconv.NumError beneath it", in, err, err)
				}
			}
		})
	}
}
