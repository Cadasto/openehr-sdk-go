package rm

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// TestSignificantDigits pins significantDigits' counting rule: sign,
// decimal point, and exponent are excluded, leading zeros before the
// first nonzero digit are stripped, and trailing zeros of the digit run
// are ignored (REQ-052).
func TestSignificantDigits(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"zero", "0", 0},
		{"zero with fraction", "0.0", 0},
		{"tenths", "0.1", 1},
		{"half", "0.5", 1},
		{"eighty point five", "80.5", 3},
		{"seventeen digits exactly", "12345678901234567", 17},
		{"trailing zeros only", "1.00000000000000000000", 1},
		{"leading zeros", "000123", 3},
		{"trailing zeros on integer", "1230000", 3},
		{"negative sign excluded", "-0.5", 1},
		{"twenty fractional digits", "0.12345678901234567890", 19},
		{"exponent form excludes exponent digits", "1.2345678901234567890e5", 19},
		{"exponent with explicit sign", "1e+5", 1},
		{"uppercase exponent", "1E10", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := significantDigits(tc.in); got != tc.want {
				t.Errorf("significantDigits(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// TestRealUnmarshalJSONCleanCases asserts that literals within float64's
// 17-significant-digit shortest-round-trip guarantee decode cleanly, in
// both bare-number and quoted-string form (REQ-052 — the SDK does not
// over-report on merely-inexact-but-round-tripping values like 0.1).
func TestRealUnmarshalJSONCleanCases(t *testing.T) {
	clean := []string{
		"0.1",
		"0.5",
		"80.5",
		"12345678901234567", // exactly 17 significant digits
		"1.00000000000000000000",
	}
	for _, lit := range clean {
		t.Run(lit, func(t *testing.T) {
			var r Real
			if err := r.UnmarshalJSON([]byte(lit)); err != nil {
				t.Errorf("UnmarshalJSON(%s) = %v, want nil", lit, err)
			}
		})
		t.Run(lit+" quoted", func(t *testing.T) {
			var r Real
			quoted := strconv.Quote(lit)
			if err := r.UnmarshalJSON([]byte(quoted)); err != nil {
				t.Errorf("UnmarshalJSON(%s) = %v, want nil", quoted, err)
			}
		})
	}
}

// TestRealUnmarshalJSONPrecisionLoss asserts that a literal carrying
// more than 17 significant decimal digits is a decode error wrapping
// typereg.ErrInvalidShape (REQ-052), in bare-number, quoted-string and
// exponent forms, and that the error names no magnitude (REQ-093).
func TestRealUnmarshalJSONPrecisionLoss(t *testing.T) {
	lossy := []string{
		"0.12345678901234567890",  // 20 fractional digits
		"1.2345678901234567890e5", // exponent form
	}
	for _, lit := range lossy {
		t.Run(lit, func(t *testing.T) {
			var r Real
			err := r.UnmarshalJSON([]byte(lit))
			if err == nil {
				t.Fatalf("UnmarshalJSON(%s) = nil error, want a precision error", lit)
			}
			if !errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("err = %v; want errors.Is(err, typereg.ErrInvalidShape)", err)
			}
			if strings.Contains(err.Error(), lit) {
				t.Errorf("err = %q; must not echo the literal (REQ-093)", err.Error())
			}
		})
		t.Run(lit+" quoted", func(t *testing.T) {
			var r Real
			quoted := strconv.Quote(lit)
			err := r.UnmarshalJSON([]byte(quoted))
			if err == nil {
				t.Fatalf("UnmarshalJSON(%s) = nil error, want a precision error", quoted)
			}
			if !errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("err = %v; want errors.Is(err, typereg.ErrInvalidShape)", err)
			}
		})
	}
}
