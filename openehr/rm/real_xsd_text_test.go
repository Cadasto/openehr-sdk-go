package rm_test

import (
	"encoding"
	"errors"
	"math"
	"strconv"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// TestREQ056_RealUnmarshalTextReadsXSDouble pins the forms of the xs:double
// lexical space Real element text reads: a decimal number with an optional
// sign, fraction and exponent, the special values INF, +INF, -INF and NaN,
// and XML whitespace around any of them (REQ-056).
func TestREQ056_RealUnmarshalTextReadsXSDouble(t *testing.T) {
	for _, tc := range []struct {
		text string
		want float64
	}{
		{"72.5", 72.5},
		{"+72.5", 72.5},
		{"-1.", -1},
		{".5", 0.5},
		{"1E-3", 0.001},
		{"1e+3", 1000},
		{"INF", math.Inf(1)},
		{"+INF", math.Inf(1)},
		{"-INF", math.Inf(-1)},
		{" \t\r\nINF\n", math.Inf(1)},
	} {
		t.Run(tc.text, func(t *testing.T) {
			var r rm.Real
			if err := r.UnmarshalText([]byte(tc.text)); err != nil {
				t.Fatalf("UnmarshalText(%q) = %v; want nil", tc.text, err)
			}
			if float64(r) != tc.want {
				t.Errorf("UnmarshalText(%q): r = %v; want %v", tc.text, float64(r), tc.want)
			}
		})
	}
	var r rm.Real
	if err := r.UnmarshalText([]byte("NaN")); err != nil || !math.IsNaN(float64(r)) {
		t.Errorf("UnmarshalText(%q) = %v, r = %v; want nil and NaN", "NaN", err, float64(r))
	}
}

// TestREQ056_RealUnmarshalTextRefusesBeyondXSDouble pins one refusal per form
// strconv.ParseFloat reads that the xs:double lexical space lacks: the
// hexadecimal form, a digit separator, Go's spellings of infinity and
// not-a-number, and whitespace XML does not collapse. Each is refused with a
// *strconv.NumError holding strconv.ErrSyntax, as text that spells no number
// is, leaves the receiver as it was, and carries no canonical-JSON shape
// sentinel (REQ-056).
func TestREQ056_RealUnmarshalTextRefusesBeyondXSDouble(t *testing.T) {
	for _, text := range []string{
		"0x1p-2",    // hexadecimal
		"0X1.8P1",   // hexadecimal, upper case
		"1_000",     // digit separator
		"1_000.5e1", // digit separator in a decimal with an exponent
		"Inf",       // Go's infinity, not INF
		"+Inf",      // the form encoding/xml wrote for an infinity
		"-inf",      // lower case
		"Infinity",  // spelled out
		"-Infinity", // spelled out, negative
		"nan",       // lower case
		"NAN",       // upper case
		"+NaN",      // NaN takes no sign
		" 72.5",     // a no-break space, which is no XML whitespace
	} {
		t.Run(text, func(t *testing.T) {
			r := rm.Real(80.5)
			err := r.UnmarshalText([]byte(text))
			if err == nil {
				t.Fatalf("UnmarshalText(%q) = nil, r = %v; want a refusal", text, float64(r))
			}
			if r != 80.5 {
				t.Errorf("UnmarshalText(%q) wrote %v to the receiver; want 80.5 unchanged", text, float64(r))
			}
			ne, ok := errors.AsType[*strconv.NumError](err)
			if !ok || ne == nil || !errors.Is(err, strconv.ErrSyntax) {
				t.Errorf("UnmarshalText(%q) err = %v (%T); want a *strconv.NumError with strconv.ErrSyntax", text, err, err)
			}
			if errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("UnmarshalText(%q) err = %v; want no canonical-JSON shape sentinel", text, err)
			}
		})
	}
}

// TestREQ056_RealMarshalTextWritesXSDouble pins Real element text on encode:
// an infinity as INF or -INF and a not-a-number as NaN, the xs:double
// spellings, and a finite value as the shortest decimal that reads back as
// the same float64, as encoding/xml writes a float64. Each reads back through
// UnmarshalText (REQ-056).
func TestREQ056_RealMarshalTextWritesXSDouble(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want string
	}{
		{math.Inf(1), "INF"},
		{math.Inf(-1), "-INF"},
		{math.NaN(), "NaN"},
		{72.5, "72.5"},
		{-0.001, "-0.001"},
		{1e21, "1e+21"},
		{18446744073709551616, "1.8446744073709552e+19"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			m, ok := any(rm.Real(tc.in)).(encoding.TextMarshaler)
			if !ok {
				t.Fatal("rm.Real implements no encoding.TextMarshaler, so encoding/xml writes it as a float64")
			}
			got, err := m.MarshalText()
			if err != nil || string(got) != tc.want {
				t.Fatalf("Real(%v).MarshalText() = %q, %v; want %q, nil", tc.in, got, err, tc.want)
			}
			var back rm.Real
			if err := back.UnmarshalText(got); err != nil {
				t.Fatalf("UnmarshalText(%q) = %v; want nil", got, err)
			}
			if float64(back) != tc.in && !(math.IsNaN(tc.in) && math.IsNaN(float64(back))) {
				t.Errorf("UnmarshalText(%q) = %v; want %v", got, float64(back), tc.in)
			}
		})
	}
}
