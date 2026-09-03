package rm

import (
	"encoding/json"
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

// TestRealUnmarshalJSONCleanCases asserts that a literal at or under the
// 17-significant-digit trigger decodes cleanly to the expected value, in
// both bare-number and quoted-string form (REQ-052 — the SDK does not
// over-report on a shorter literal that is merely binary-inexact, such
// as 0.1, or on a literal at the trigger's own boundary that does not in
// fact round-trip — see maxSignificantDigits).
func TestRealUnmarshalJSONCleanCases(t *testing.T) {
	tests := []struct {
		lit  string
		want float64
	}{
		{"0.1", 0.1},
		{"0.5", 0.5},
		{"80.5", 80.5},
		// The boundary case the >17 rule admits: exactly 17 significant
		// digits, so it decodes without error, but it does NOT round-trip
		// (float64 stores it as ...67568, one past the last digit) — the
		// rule's documented lower-bound trade-off, not a round-trip claim.
		{"12345678901234567", 1.2345678901234568e16},
		{"1.00000000000000000000", 1},
		// The documented FALSE NEGATIVE of the digit-count budget
		// (wire.md § Floating-point precision): 2^53+1 carries only 16
		// significant digits, so the budget admits it, and float64 rounds
		// it down to 2^53. Accepted unreported, by design.
		{"9007199254740993", 9007199254740992},
	}
	for _, tc := range tests {
		t.Run(tc.lit, func(t *testing.T) {
			var r Real
			if err := r.UnmarshalJSON([]byte(tc.lit)); err != nil {
				t.Errorf("UnmarshalJSON(%s) = %v, want nil", tc.lit, err)
			}
			if float64(r) != tc.want {
				t.Errorf("UnmarshalJSON(%s): r = %v, want %v", tc.lit, float64(r), tc.want)
			}
		})
		t.Run(tc.lit+" quoted", func(t *testing.T) {
			var r Real
			quoted := strconv.Quote(tc.lit)
			if err := r.UnmarshalJSON([]byte(quoted)); err != nil {
				t.Errorf("UnmarshalJSON(%s) = %v, want nil", quoted, err)
			}
			if float64(r) != tc.want {
				t.Errorf("UnmarshalJSON(%s): r = %v, want %v", quoted, float64(r), tc.want)
			}
		})
	}
}

// TestRealUnmarshalJSONNullIsNoOp asserts that a JSON null leaves the
// receiver unchanged, matching rm.Character's convention and the
// encoding/json Unmarshaler idiom for "not present".
func TestRealUnmarshalJSONNullIsNoOp(t *testing.T) {
	r := Real(80.5)
	if err := r.UnmarshalJSON([]byte("null")); err != nil {
		t.Fatalf("UnmarshalJSON(null) = %v, want nil", err)
	}
	if r != 80.5 {
		t.Errorf("UnmarshalJSON(null) overwrote the receiver: r = %v, want 80.5 unchanged", r)
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
		// The documented FALSE POSITIVE of the digit-count budget
		// (wire.md § Floating-point precision): 2^64 is exactly
		// representable in float64, yet its 20-digit spelling fails the
		// budget and is refused. Refused by design, not by accident — the
		// budget counts digits, it does not test exactness.
		"18446744073709551616",
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

// TestRealUnmarshalJSONParseBeforeDigitPolicy pins the ORDER of the two
// checks inside UnmarshalJSON: the literal is parsed first, so a
// malformed or out-of-range value surfaces the parser's own typed error,
// and the >17-significant-digit policy (REQ-052) only ever speaks about
// a literal that actually parsed. A long-but-broken literal must not be
// reported as precision loss, and must not acquire
// typereg.ErrInvalidShape — that sentinel classifies the SDK's own
// refusal of a literal past the digit budget, not an encoding/json or
// strconv failure. Removing the reorder fails this test.
func TestRealUnmarshalJSONParseBeforeDigitPolicy(t *testing.T) {
	t.Run("quoted non-numeric", func(t *testing.T) {
		const lit = `"123456789012345678x"`
		var r Real
		err := r.UnmarshalJSON([]byte(lit))
		if err == nil {
			t.Fatalf("UnmarshalJSON(%s) = nil error, want a parse error", lit)
		}
		if _, ok := errors.AsType[*strconv.NumError](err); !ok {
			t.Errorf("UnmarshalJSON(%s) err = %v (%T); want errors.AsType[*strconv.NumError] to reach it", lit, err, err)
		}
		if errors.Is(err, errPrecisionLoss) {
			t.Errorf("UnmarshalJSON(%s) err = %v; want the parse failure, not the precision refusal", lit, err)
		}
		if errors.Is(err, typereg.ErrInvalidShape) {
			t.Errorf("UnmarshalJSON(%s) err = %v; a strconv parse failure must not carry typereg.ErrInvalidShape", lit, err)
		}
	})

	t.Run("bare out of range", func(t *testing.T) {
		const lit = "1.234567890123456789e400"
		var r Real
		err := r.UnmarshalJSON([]byte(lit))
		if err == nil {
			t.Fatalf("UnmarshalJSON(%s) = nil error, want a range error", lit)
		}
		if _, ok := errors.AsType[*json.UnmarshalTypeError](err); !ok {
			t.Errorf("UnmarshalJSON(%s) err = %v (%T); want errors.AsType[*json.UnmarshalTypeError] to reach it", lit, err, err)
		}
		if errors.Is(err, errPrecisionLoss) {
			t.Errorf("UnmarshalJSON(%s) err = %v; want the range failure, not the precision refusal", lit, err)
		}
		if errors.Is(err, typereg.ErrInvalidShape) {
			t.Errorf("UnmarshalJSON(%s) err = %v; an encoding/json range failure must not carry typereg.ErrInvalidShape", lit, err)
		}
	})

	t.Run("quoted out of range", func(t *testing.T) {
		const lit = `"1.234567890123456789e400"`
		var r Real
		err := r.UnmarshalJSON([]byte(lit))
		if err == nil {
			t.Fatalf("UnmarshalJSON(%s) = nil error, want a range error", lit)
		}
		if _, ok := errors.AsType[*strconv.NumError](err); !ok {
			t.Errorf("UnmarshalJSON(%s) err = %v (%T); want errors.AsType[*strconv.NumError] to reach it", lit, err, err)
		}
		if errors.Is(err, errPrecisionLoss) {
			t.Errorf("UnmarshalJSON(%s) err = %v; want the range failure, not the precision refusal", lit, err)
		}
		if errors.Is(err, typereg.ErrInvalidShape) {
			t.Errorf("UnmarshalJSON(%s) err = %v; a strconv range failure must not carry typereg.ErrInvalidShape", lit, err)
		}
	})
}

// TestRealPrecisionRefusalMessageUnchangedByClassification pins the
// wire.md § REQ-052 Decode-side shape sentinel MUST on rm.Real: the
// classification adds a sentinel, not a word of text. The old
// fmt.Errorf("%w: %w") form spliced the sentinel's own prose
// ("canjson: invalid JSON shape") into Error(); the sentinel now rides
// on Is (see shapeClassified).
//
// The message text is also pinned honest (REQ-052 / REQ-093): the budget
// counts significant digits, so it may not claim the value is
// unrepresentable — 2^64 is exactly representable and still refused.
func TestRealPrecisionRefusalMessageUnchangedByClassification(t *testing.T) {
	const want = "rm.Real: literal carries more than 17 significant decimal digits"
	var r Real
	err := r.UnmarshalJSON([]byte("18446744073709551616"))
	if err == nil {
		t.Fatal("UnmarshalJSON(2^64) = nil error, want the precision refusal")
	}
	if got := err.Error(); got != want {
		t.Errorf("err = %q, want %q", got, want)
	}
	if !errors.Is(err, typereg.ErrInvalidShape) {
		t.Errorf("err = %v; want errors.Is(err, typereg.ErrInvalidShape)", err)
	}
	if strings.Contains(err.Error(), "invalid JSON shape") {
		t.Errorf("err = %q; must not splice the sentinel's own prose into the message", err.Error())
	}
	// "more precision than float64 can represent" was the old text and is
	// false for this very literal — 2^64 is exact in float64.
	if strings.Contains(err.Error(), "can represent") {
		t.Errorf("err = %q; the budget counts digits and must not claim unrepresentability", err.Error())
	}
	if cause := errors.Unwrap(err); cause == nil {
		t.Errorf("errors.Unwrap(%v) = nil; the cause must stay reachable through unwrapping", err)
	} else if cause.Error() != err.Error() {
		t.Errorf("classified err = %q but its cause reads %q; the classification must not change the message", err.Error(), cause.Error())
	}
}

// TestRealNilReceiverIsRefusedNotPanicked pins the nil-receiver axis
// (idiom.md § No panics, REQ-025): UnmarshalJSON assigns through the
// pointer, so a nil *Real would be dereferenced. `null` is the
// discriminating second input — it is the value the method otherwise
// treats as a no-op, so a nil check placed after the null arm would let
// it through. Caller misuse is not a wire-shape problem, so the refusal
// stays outside typereg.ErrInvalidShape. A panic fails the test run.
func TestRealNilReceiverIsRefusedNotPanicked(t *testing.T) {
	for _, in := range []string{"80.5", "null"} {
		t.Run(in, func(t *testing.T) {
			var r *Real
			err := r.UnmarshalJSON([]byte(in))
			if err == nil {
				t.Fatalf("(*Real)(nil).UnmarshalJSON(%s) = nil error, want a refusal", in)
			}
			if errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("err = %v; caller misuse must not classify as a wire-shape failure", err)
			}
		})
	}
}
