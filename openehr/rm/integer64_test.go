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

// TestInteger64_REQ052_NilReceiverIsRefusedNotPanicked pins the nil-receiver
// rule (idiom.md, REQ-025) on rm.Integer64: UnmarshalJSON assigns through the
// pointer, so a nil *rm.Integer64 is refused with an error carrying
// typereg.ErrNilReceiver, never dereferenced, and caller misuse stays outside
// typereg.ErrInvalidShape. `null` is the discriminating input: a nil check
// placed after the decode would let it through and panic.
func TestInteger64_REQ052_NilReceiverIsRefusedNotPanicked(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"42", `"42"`, "1.0", "null"} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			var i *rm.Integer64
			err := i.UnmarshalJSON([]byte(in))
			if !errors.Is(err, typereg.ErrNilReceiver) {
				t.Fatalf("(*rm.Integer64)(nil).UnmarshalJSON(%s) = %v, want an error carrying typereg.ErrNilReceiver", in, err)
			}
			if errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("(*rm.Integer64)(nil).UnmarshalJSON(%s) = %v; caller misuse must not classify as a wire-shape failure", in, err)
			}
		})
	}
}

// TestInteger64_REQ052_EmptyInputIsRefused pins that an empty literal is
// refused and leaves the receiver as it was.
func TestInteger64_REQ052_EmptyInputIsRefused(t *testing.T) {
	t.Parallel()
	const prior rm.Integer64 = 7
	got := prior
	if err := got.UnmarshalJSON(nil); err == nil {
		t.Fatalf("rm.Integer64.UnmarshalJSON(empty) = nil error, decoded %d; want a refusal", got)
	}
	if got != prior {
		t.Errorf("rm.Integer64.UnmarshalJSON(empty) refused but changed the receiver from %d to %d", prior, got)
	}
}

// TestInteger64_REQ052_WholeNumberLiteralDecodes pins wire.md § Floating-point
// precision on rm.Integer64: a number whose value is a whole number decodes as
// that integer however the literal writes it, with a zero fraction or an
// exponent, bare or quoted. The int64 bounds read in every form, and the
// values past the int32 range read too, so the reader runs at 64 bits.
func TestInteger64_REQ052_WholeNumberLiteralDecodes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want rm.Integer64
	}{
		{in: `1.0`, want: 1},
		{in: `1e0`, want: 1},
		{in: `1E1`, want: 10},
		{in: `1e+1`, want: 10},
		{in: `10.00`, want: 10},
		{in: `10e-1`, want: 1},
		{in: `0.1e1`, want: 1},
		{in: `20e-1`, want: 2},
		{in: `-3.000`, want: -3},
		{in: `-0.0`, want: 0},
		{in: `0e18446744073709551616`, want: 0},
		{in: `2147483648`, want: 2147483648},
		{in: `2147483648.0`, want: 2147483648},
		{in: `1e10`, want: 10000000000},
		{in: `1e18`, want: 1000000000000000000},
		{in: `9223372036854775807`, want: math.MaxInt64},
		{in: `9223372036854775807.0`, want: math.MaxInt64},
		{in: `922337203685477580.7e1`, want: math.MaxInt64},
		{in: `9.223372036854775807e18`, want: math.MaxInt64},
		{in: `-9223372036854775808`, want: math.MinInt64},
		{in: `-9223372036854775808.0`, want: math.MinInt64},
		{in: `-92233720368547758080e-1`, want: math.MinInt64},
		{in: `"1.0"`, want: 1},
		{in: `"1e0"`, want: 1},
		{in: `"2.0"`, want: 2},
		{in: `"1e10"`, want: 10000000000},
		{in: `"9223372036854775807"`, want: math.MaxInt64},
		{in: `"9223372036854775807.0"`, want: math.MaxInt64},
		{in: `"-9223372036854775808"`, want: math.MinInt64},
		{in: `"-9223372036854775808.0"`, want: math.MinInt64},
		{in: `42`, want: 42},
		{in: `"42"`, want: 42},
	} {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			var got rm.Integer64
			if err := got.UnmarshalJSON([]byte(tc.in)); err != nil {
				t.Fatalf("rm.Integer64.UnmarshalJSON(%s) = %v, want nil", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("rm.Integer64.UnmarshalJSON(%s) decoded %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// TestInteger64_REQ052_RefusesFractionOrOutOfRange pins the refusal half of
// wire.md § Floating-point precision on rm.Integer64: a number with a non-zero
// fraction, or one outside the int64 range, is refused with a typed error and
// never truncated or rounded, so the receiver keeps its value. The refusal is
// a *json.SemanticError for a bare number and a *strconv.NumError for a quoted
// one, carrying no typereg.ErrInvalidShape at this level.
//
// The strconv sentinel tells the two refusals apart however the literal
// writes the number: a non-zero fraction carries strconv.ErrSyntax whatever
// its magnitude, and a whole number outside the range carries
// strconv.ErrRange, so 9223372036854775808.0, 1e19 and "1e19" report the same
// cause as 9223372036854775808 does.
//
// Some rows catch an implementation that cuts corners.
// 1.0000000000000001 reads as exactly 1 through a float64. An exponent of
// 2^64 wraps to 0 in int64 arithmetic. 9223372036854775808 is one past
// math.MaxInt64 but within -math.MinInt64, so a reader that applies the
// negative bound to a positive number lets it through. 18446744073709551616
// is 2^64, which a uint64 accumulator that checks the range only after
// multiplying wraps to 0. 9223372036854775808.5 is both out of range
// and not whole, and reads as not whole.
func TestInteger64_REQ052_RefusesFractionOrOutOfRange(t *testing.T) {
	t.Parallel()
	for _, group := range []struct {
		cause, notCause error
		ins             []string
	}{
		{
			// A non-zero fraction.
			cause: strconv.ErrSyntax, notCause: strconv.ErrRange,
			ins: []string{
				`1.5`,
				`-1.5`,
				`0.5`,
				`2.5`,
				`25e-1`,
				`123e-2`,
				`1e-1`,
				`1.0000000000000001`,
				`10e-18446744073709551616`,
				`9223372036854775807.5`,
				`9223372036854775808.5`,
				`-9223372036854775809.5`,
				`"1.5"`,
				`"2.5"`,
				`"1.0000000000000001"`,
				`"9223372036854775808.5"`,
			},
		},
		{
			// Outside the int64 range.
			cause: strconv.ErrRange, notCause: strconv.ErrSyntax,
			ins: []string{
				`9223372036854775808`,
				`-9223372036854775809`,
				`9223372036854775808.0`,
				`-9223372036854775809.0`,
				`9.223372036854775808e18`,
				`92233720368547758080e-1`,
				`1e19`,
				`-1e19`,
				`18446744073709551615`,
				`18446744073709551616`,
				`18446744073709551616.0`,
				`-18446744073709551616`,
				`99999999999999999999`,
				`99999999999999999999.0`,
				`1e18446744073709551616`,
				`"9223372036854775808"`,
				`"-9223372036854775809"`,
				`"9223372036854775808.0"`,
				`"18446744073709551616"`,
				`"99999999999999999999.0"`,
				`"1e19"`,
				`"1e18446744073709551616"`,
			},
		},
	} {
		for _, in := range group.ins {
			t.Run(in, func(t *testing.T) {
				t.Parallel()
				const prior rm.Integer64 = 7
				got := prior
				err := got.UnmarshalJSON([]byte(in))
				if err == nil {
					t.Fatalf("rm.Integer64.UnmarshalJSON(%s) = nil error, decoded %d; want a refusal", in, got)
				}
				if got != prior {
					t.Errorf("rm.Integer64.UnmarshalJSON(%s) refused but changed the receiver from %d to %d", in, prior, got)
				}
				if errors.Is(err, typereg.ErrInvalidShape) {
					t.Errorf("rm.Integer64.UnmarshalJSON(%s) err = %v; a direct refusal carries no shape sentinel", in, err)
				}
				if !errors.Is(err, group.cause) || errors.Is(err, group.notCause) {
					t.Errorf("rm.Integer64.UnmarshalJSON(%s) err = %v; want it to carry %v and not %v", in, err, group.cause, group.notCause)
				}
				if strings.HasPrefix(in, `"`) {
					ne, ok := errors.AsType[*strconv.NumError](err)
					if !ok || ne == nil {
						t.Fatalf("rm.Integer64.UnmarshalJSON(%s) err = %v (%T); want a *strconv.NumError beneath it", in, err, err)
					}
					if lit := strings.Trim(in, `"`); ne.Func != "ParseInt" || ne.Num != lit {
						t.Errorf("rm.Integer64.UnmarshalJSON(%s) NumError{Func: %q, Num: %q}, want {Func: %q, Num: %q}", in, ne.Func, ne.Num, "ParseInt", lit)
					}
					return
				}
				if _, ok := errors.AsType[*json.SemanticError](err); !ok {
					t.Errorf("rm.Integer64.UnmarshalJSON(%s) err = %v (%T); want a *json.SemanticError beneath it", in, err, err)
				}
			})
		}
	}
}

// TestInteger64_REQ052_QuotedLiteralGrammar pins which quoted literals
// rm.Integer64 reads, so its doc comment cannot drift from the code. A quoted
// literal is read first as a decimal integer by strconv.ParseInt, which allows
// a leading sign and leading zeros, so "+1", "01" and "007" decode. Only when
// that fails is it read as a JSON number whose value is whole, and the JSON
// number grammar allows neither a plus sign nor a leading zero, so "+1.0" and
// "01.0" are refused with the syntax error of the integer read.
func TestInteger64_REQ052_QuotedLiteralGrammar(t *testing.T) {
	t.Parallel()
	const prior rm.Integer64 = 99
	for _, tc := range []struct {
		in      string
		want    rm.Integer64
		refused bool
	}{
		{in: `"+1"`, want: 1},
		{in: `"01"`, want: 1},
		{in: `"007"`, want: 7},
		{in: `"-007"`, want: -7},
		{in: `"+9223372036854775807"`, want: math.MaxInt64},
		{in: `"+1.0"`, refused: true},
		{in: `"01.0"`, refused: true},
		{in: `"+1e0"`, refused: true},
		{in: `"007.0"`, refused: true},
	} {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got := prior
			err := got.UnmarshalJSON([]byte(tc.in))
			if !tc.refused {
				if err != nil {
					t.Fatalf("rm.Integer64.UnmarshalJSON(%s) = %v, want nil", tc.in, err)
				}
				if got != tc.want {
					t.Errorf("rm.Integer64.UnmarshalJSON(%s) decoded %d, want %d", tc.in, got, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("rm.Integer64.UnmarshalJSON(%s) = nil error, decoded %d; want a refusal", tc.in, got)
			}
			if got != prior {
				t.Errorf("rm.Integer64.UnmarshalJSON(%s) refused but changed the receiver from %d to %d", tc.in, prior, got)
			}
			if ne, ok := errors.AsType[*strconv.NumError](err); !ok || ne == nil || !errors.Is(err, strconv.ErrSyntax) {
				t.Errorf("rm.Integer64.UnmarshalJSON(%s) err = %v (%T); want a *strconv.NumError carrying strconv.ErrSyntax", tc.in, err, err)
			}
		})
	}
}

// TestInteger64_REQ052_RefusesMalformedNumberLiteral pins that reading a
// whole number in a decimal form loosens no literal grammar: a bare literal
// that is not a JSON number, and a quoted one that is neither an integer nor
// a JSON number, are still refused and leave the receiver as it was. A
// quoted refusal keeps the *strconv.NumError beneath it.
func TestInteger64_REQ052_RefusesMalformedNumberLiteral(t *testing.T) {
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
		`true`,
		`"1."`,
		`".5"`,
		`"1e"`,
		`" 1.0"`,
		`"1.0x"`,
		`"0x10"`,
		`""`,
	} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			const prior rm.Integer64 = 7
			got := prior
			err := got.UnmarshalJSON([]byte(in))
			if err == nil {
				t.Fatalf("rm.Integer64.UnmarshalJSON(%s) = nil error, decoded %d; want a refusal", in, got)
			}
			if got != prior {
				t.Errorf("rm.Integer64.UnmarshalJSON(%s) refused but changed the receiver from %d to %d", in, prior, got)
			}
			if strings.HasPrefix(in, `"`) {
				if _, ok := errors.AsType[*strconv.NumError](err); !ok {
					t.Errorf("rm.Integer64.UnmarshalJSON(%s) err = %v (%T); want a *strconv.NumError beneath it", in, err, err)
				}
			}
		})
	}
}

// TestInteger64_REQ052_QuotedRefusalNamesTheLiteralOnce pins wire.md
// § REQ-052 *Value-free decode errors* on rm.Integer64: a quoted literal that
// fails to parse is reported as `parse quoted literal: <cause>`, so the
// literal appears exactly once, in the *strconv.NumError cause, and the
// refusal carries the literal's JSON text, quotes included, for a decode
// entry point's DecodeError.Value.
func TestInteger64_REQ052_QuotedRefusalNamesTheLiteralOnce(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in      string
		message string
		range_  bool
	}{
		{in: `"1.5"`, message: `rm.Integer64: parse quoted literal: strconv.ParseInt: parsing "1.5": invalid syntax`},
		{in: `"7y"`, message: `rm.Integer64: parse quoted literal: strconv.ParseInt: parsing "7y": invalid syntax`},
		{in: `"1e19"`, message: `rm.Integer64: parse quoted literal: strconv.ParseInt: parsing "1e19": value out of range`, range_: true},
		{in: `"9223372036854775808"`, message: `rm.Integer64: parse quoted literal: strconv.ParseInt: parsing "9223372036854775808": value out of range`, range_: true},
	} {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			var i rm.Integer64
			err := i.UnmarshalJSON([]byte(tc.in))
			if err == nil {
				t.Fatalf("rm.Integer64.UnmarshalJSON(%s) = nil; want a refusal", tc.in)
			}
			if err.Error() != tc.message {
				t.Errorf("rm.Integer64.UnmarshalJSON(%s) = %q; want %q", tc.in, err, tc.message)
			}
			if n := strings.Count(err.Error(), strings.Trim(tc.in, `"`)); n != 1 {
				t.Errorf("rm.Integer64.UnmarshalJSON(%s) = %q names the literal %d times; want once, in the cause", tc.in, err, n)
			}
			if got := errors.Is(err, strconv.ErrRange); got != tc.range_ {
				t.Errorf("rm.Integer64.UnmarshalJSON(%s): errors.Is(_, strconv.ErrRange) = %t; want %t", tc.in, got, tc.range_)
			}
			if got := carried(err); got != tc.in {
				t.Errorf("rm.Integer64.UnmarshalJSON(%s) carries %q for the entry point's Value; want the literal as written, %q", tc.in, got, tc.in)
			}
		})
	}
}

// TestInteger64_REQ052_EncodesTheBareInteger pins the encode half of wire.md
// § Floating-point precision: rm.Integer64 always writes the integer form, a
// bare JSON number, whatever literal it was decoded from.
func TestInteger64_REQ052_EncodesTheBareInteger(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want string
	}{
		{in: `2`, want: `2`},
		{in: `2.0`, want: `2`},
		{in: `"2.0"`, want: `2`},
		{in: `20e-1`, want: `2`},
		{in: `-9223372036854775808.0`, want: `-9223372036854775808`},
		{in: `"9223372036854775807"`, want: `9223372036854775807`},
	} {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			var i rm.Integer64
			if err := i.UnmarshalJSON([]byte(tc.in)); err != nil {
				t.Fatalf("rm.Integer64.UnmarshalJSON(%s) = %v, want nil", tc.in, err)
			}
			out, err := i.MarshalJSON()
			if err != nil {
				t.Fatalf("rm.Integer64(%d).MarshalJSON() = %v, want nil", i, err)
			}
			if string(out) != tc.want {
				t.Errorf("rm.Integer64 decoded from %s encodes as %s, want %s", tc.in, out, tc.want)
			}
		})
	}
}
