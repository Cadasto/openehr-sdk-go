package rm

import (
	json "encoding/json/v2"
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
		{"trailing zero after a fraction digit", "1.10", 2},
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

// TestDecimalForm pins how the lossy test reads a decimal literal (REQ-052):
// the run from its first non-zero digit to its last, and where the decimal
// point sits, the value being 0.<run's digits> × 10^point. A literal that is
// not decimal is not read.
func TestDecimalForm(t *testing.T) {
	tests := []struct {
		in    string
		run   string
		point int64
		ok    bool
	}{
		{in: "12.5", run: "12.5", point: 2, ok: true},
		{in: "0.05", run: "5", point: -1, ok: true},
		{in: "500", run: "5", point: 3, ok: true},
		{in: "-1.10", run: "1.1", point: 1, ok: true},
		{in: ".5", run: "5", point: 0, ok: true},
		{in: "5.", run: "5", point: 1, ok: true},
		{in: "1.5e-3", run: "1.5", point: -2, ok: true},
		{in: "+1E+3", run: "1", point: 4, ok: true},
		{in: "1_000", run: "1", point: 4, ok: true},
		{in: "1e1_0", run: "1", point: 11, ok: true},
		{in: "9.007199254740992e+15", run: "9.007199254740992", point: 16, ok: true},
		{in: "-0.0e7", run: "", point: 0, ok: true},
		{in: "0x1p-2", ok: false},
		{in: "0x1e5", ok: false},
		{in: "Inf", ok: false},
		{in: "NaN", ok: false},
		{in: "", ok: false},
		{in: ".", ok: false},
		{in: "1e", ok: false},
		{in: "1e+", ok: false},
		{in: "1.2.3", ok: false},
	}
	for _, tc := range tests {
		run, point, ok := decimalForm(tc.in)
		if run != tc.run || point != tc.point || ok != tc.ok {
			t.Errorf("decimalForm(%q) = (%q, %d, %t), want (%q, %d, %t)", tc.in, run, point, ok, tc.run, tc.point, tc.ok)
		}
	}
}

// longestExact is the exact decimal value of the largest subnormal float64,
// whose 767 significant digits are the most any float64 has.
func longestExact(t *testing.T) string {
	t.Helper()
	s := strconv.FormatFloat(0x0.fffffffffffffp-1022, 'e', 800, 64)
	mant, exp, _ := strings.Cut(s, "e")
	mant = strings.TrimRight(mant, "0")
	if n := significantDigits(mant); n != maxExactDigits {
		t.Fatalf("the largest subnormal float64 has %d significant digits; want maxExactDigits, %d", n, maxExactDigits)
	}
	return mant + "e" + exp
}

// TestLossy pins the lossy test of wire.md § Floating-point precision
// (REQ-052) on the literals it names and on the edges of its shortcuts: a
// literal is lossy when the float64 it parses to, written back correctly
// rounded to as many significant digits as the literal carries, denotes
// another value. A literal of at most 15 digits whose float64 is at least
// 2^-1022, the smallest normal float64, is never lossy; a shorter literal
// below it is written back like any other, so a subnormal that keeps fewer
// digits, or an underflow to zero, is lossy. A literal past the 767 digits
// of the longest float64 is lossy without a write-back; a literal that is
// not decimal is never lossy.
//
// Can-fail control: skip the write-back and the 16-digit and longer rows
// that are lossy pass as clean; skip it for every short literal that parsed
// to a non-zero float64 and the short subnormal rows that are lossy pass;
// skip it for every short literal, zero included, and 1e-400 passes too;
// lower maxExactDigits and the 767-digit row is refused.
func TestLossy(t *testing.T) {
	exact := longestExact(t)
	mant, exp, _ := strings.Cut(exact, "e")
	tests := []struct {
		lit  string
		want bool
	}{
		// The literals wire.md names.
		{"9007199254740993", true},
		{"1.2345678901234567890", true},
		{"1e-400", true},
		{"1.23e-322", true},
		{"18446744073709551616", false},
		{"0.10000000000000001", false},
		{"0.1000000000000000055511151231257827", false},
		{"1.10", false},
		{"0.1", false},
		// The 17-digit literal the former digit budget admitted, which
		// parses to 12345678901234568.
		{"12345678901234567", true},
		{"-9007199254740993", true},
		{"9007199254740992", false},
		{"123456789012345", false},
		{"-1e-400", true},
		{"0e-400", false},
		{"-0", false},
		{"5e-324", false},
		{"1e308", false},
		// Short literals around the smallest normal float64, 2^-1022. Above
		// it, 15 digits always come back. Below it, a subnormal keeps fewer
		// digits, and the write-back decides: 1.23e-322 parses to 1.24e-322,
		// and the 15-digit 1.23456789012345e-310 to 1.23456789012346e-310.
		{"2.2250738585072014e-308", false},
		{"2.3e-308", false},
		{"-1.23e-322", true},
		{"1.23456789012345e-310", true},
		{"1.23456789012345e-309", false},
		{"4.9e-324", false},
		{"1e-310", false},
		{"9_007_199_254_740_993", true},
		{"1_000", false},
		// The longest float64 and one digit more.
		{exact, false},
		{mant + "1e" + exp, true},
		// Not decimal: never lossy, though the digit count reads digits in
		// them.
		{"0x0p5", false},
		{"0x1.fffffffffffffffffffp0", false},
		{"Inf", false},
		{"NaN", false},
	}
	for _, tc := range tests {
		f, err := strconv.ParseFloat(tc.lit, 64)
		if err != nil {
			t.Fatalf("strconv.ParseFloat(%.40s) = %v", tc.lit, err)
		}
		if got := lossy(tc.lit, f); got != tc.want {
			t.Errorf("lossy(%.40s, %v) = %t, want %t", tc.lit, f, got, tc.want)
		}
	}
}

// TestLossyShortLiteralDoesNotAllocate pins the cost of the decode hot path
// (REQ-052): a literal of at most 15 significant digits whose float64 is at
// least 2^-1022 is never lossy, so the test skips the write-back and
// allocates nothing.
func TestLossyShortLiteralDoesNotAllocate(t *testing.T) {
	if got := testing.AllocsPerRun(100, func() { _ = lossy("123456.789012345", 123456.789012345) }); got != 0 {
		t.Errorf("lossy on a 15-digit literal allocates %v times per call; want 0", got)
	}
}

// TestRealUnmarshalJSONCleanCases asserts that a literal that is not lossy
// decodes cleanly to the float64 nearest it, in both bare-number and
// quoted-string form (REQ-052): a short literal that is merely
// binary-inexact, such as 0.1, and a long literal that the float64 gives
// back at its own precision, such as 2^64 or the 17-digit spelling of 0.1.
func TestRealUnmarshalJSONCleanCases(t *testing.T) {
	tests := []struct {
		lit  string
		want float64
	}{
		{"0.1", 0.1},
		{"0.5", 0.5},
		{"80.5", 80.5},
		{"72.5", 72.5},
		{"1.10", 1.1},
		{"1.00000000000000000000", 1},
		{"9007199254740992", 9007199254740992},
		{"18446744073709551616", 18446744073709551616},
		{"0.10000000000000001", 0.1},
		{"0.1000000000000000055511151231257827", 0.1},
		{"5e-324", 5e-324},
		{"2.2250738585072014e-308", 0x1p-1022},
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

// TestRealUnmarshalJSONLossyLiteral asserts that a lossy literal is a decode
// error wrapping typereg.ErrInvalidShape (REQ-052), in bare-number,
// quoted-string and exponent forms, that the error names no magnitude
// (REQ-093), and that the receiver keeps its value.
func TestRealUnmarshalJSONLossyLiteral(t *testing.T) {
	lossyLits := []string{
		"9007199254740993",        // 2^53+1, parses to 2^53
		"1.2345678901234567890",   // 19 significant digits
		"1e-400",                  // underflows to zero
		"1.23e-322",               // a subnormal, parses to 1.24e-322
		"0.12345678901234567890",  // 20 fractional digits
		"1.2345678901234567890e5", // exponent form
	}
	for _, lit := range lossyLits {
		t.Run(lit, func(t *testing.T) {
			r := Real(80.5)
			err := r.UnmarshalJSON([]byte(lit))
			if err == nil {
				t.Fatalf("UnmarshalJSON(%s) = nil error, r = %v; want a lossy-literal refusal", lit, float64(r))
			}
			if !errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("err = %v; want errors.Is(err, typereg.ErrInvalidShape)", err)
			}
			if !errors.Is(err, errLossyLiteral) {
				t.Errorf("err = %v; want errors.Is(err, errLossyLiteral)", err)
			}
			if strings.Contains(err.Error(), lit) {
				t.Errorf("err = %q; must not echo the literal (REQ-093)", err.Error())
			}
			if r != 80.5 {
				t.Errorf("UnmarshalJSON(%s) wrote %v to the receiver; want 80.5 unchanged", lit, float64(r))
			}
		})
		t.Run(lit+" quoted", func(t *testing.T) {
			var r Real
			quoted := strconv.Quote(lit)
			err := r.UnmarshalJSON([]byte(quoted))
			if err == nil {
				t.Fatalf("UnmarshalJSON(%s) = nil error, want a lossy-literal refusal", quoted)
			}
			if !errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("err = %v; want errors.Is(err, typereg.ErrInvalidShape)", err)
			}
		})
	}
}

// TestRealUnmarshalJSONParseBeforeLossyTest pins the ORDER of the two
// checks inside UnmarshalJSON: the literal is parsed first, so a
// malformed or out-of-range value surfaces the parser's own typed error,
// and the lossy test (REQ-052) only ever speaks about a literal that
// actually parsed. A long-but-broken literal must not be reported as a
// lossy literal, and must not acquire typereg.ErrInvalidShape — that
// sentinel classifies the SDK's own refusal of a lossy literal, not an
// encoding/json or strconv failure. Removing the order fails this test.
func TestRealUnmarshalJSONParseBeforeLossyTest(t *testing.T) {
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
		if errors.Is(err, errLossyLiteral) {
			t.Errorf("UnmarshalJSON(%s) err = %v; want the parse failure, not the lossy-literal refusal", lit, err)
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
		if _, ok := errors.AsType[*json.SemanticError](err); !ok {
			t.Errorf("UnmarshalJSON(%s) err = %v (%T); want errors.AsType[*encoding/json/v2.SemanticError] to reach it", lit, err, err)
		}
		if errors.Is(err, errLossyLiteral) {
			t.Errorf("UnmarshalJSON(%s) err = %v; want the range failure, not the lossy-literal refusal", lit, err)
		}
		if errors.Is(err, typereg.ErrInvalidShape) {
			t.Errorf("UnmarshalJSON(%s) err = %v; an encoding/json/v2 range failure must not carry typereg.ErrInvalidShape", lit, err)
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
		if errors.Is(err, errLossyLiteral) {
			t.Errorf("UnmarshalJSON(%s) err = %v; want the range failure, not the lossy-literal refusal", lit, err)
		}
		if errors.Is(err, typereg.ErrInvalidShape) {
			t.Errorf("UnmarshalJSON(%s) err = %v; a strconv range failure must not carry typereg.ErrInvalidShape", lit, err)
		}
	})
}

// TestRealLossyRefusalMessageUnchangedByClassification pins the
// wire.md § REQ-052 Decode-side shape sentinel MUST on rm.Real: the
// classification adds a sentinel, not a word of text. The old
// fmt.Errorf("%w: %w") form spliced the sentinel's own prose
// ("canjson: invalid JSON shape") into Error(); the sentinel now rides
// on Is (see typereg.ClassifyShape). The message names no value
// (REQ-093).
func TestRealLossyRefusalMessageUnchangedByClassification(t *testing.T) {
	const (
		lit  = "9007199254740993"
		want = "rm.Real: literal cannot be held in a float64 at its own precision"
	)
	var r Real
	err := r.UnmarshalJSON([]byte(lit))
	if err == nil {
		t.Fatalf("UnmarshalJSON(%s) = nil error, want the lossy-literal refusal", lit)
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
	if cause := errors.Unwrap(err); cause == nil {
		t.Errorf("errors.Unwrap(%v) = nil; the cause must stay reachable through unwrapping", err)
	} else if cause.Error() != err.Error() {
		t.Errorf("classified err = %q but its cause reads %q; the classification must not change the message", err.Error(), cause.Error())
	}
}

// TestRealUnmarshalText pins the canonical-XML decode of a Real (REQ-052,
// REQ-056). Text reads as encoding/xml read a float64 before the method
// existed: empty text is 0, surrounding whitespace is trimmed, and text that
// does not spell a number, whitespace alone and a quoted number among them,
// fails with a *strconv.NumError. A lossy literal is refused with the
// value-free errLossyLiteral, and no refusal carries canonical JSON's
// typereg.ErrInvalidShape.
//
// Can-fail control: drop the lossy test from UnmarshalText and the lossy
// rows decode; return errLossyShape and the sentinel assertion fails.
func TestRealUnmarshalText(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		want       float64
	}{
		{name: "empty", text: "", want: 0},
		{name: "padded", text: " \n\t72.5\r\n ", want: 72.5},
		{name: "trailing zero", text: "1.10", want: 1.1},
		{name: "2^64", text: "18446744073709551616", want: 18446744073709551616},
		{name: "0.1 at 35 digits", text: "0.1000000000000000055511151231257827", want: 0.1},
		{name: "smallest subnormal", text: "5e-324", want: 5e-324},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Real(80.5)
			if err := r.UnmarshalText([]byte(tc.text)); err != nil {
				t.Fatalf("UnmarshalText(%q) = %v, want nil", tc.text, err)
			}
			if float64(r) != tc.want {
				t.Errorf("UnmarshalText(%q): r = %v, want %v", tc.text, float64(r), tc.want)
			}
		})
	}
	for _, tc := range []struct {
		name, text string
		lossy      bool
	}{
		{name: "2^53+1", text: "9007199254740993", lossy: true},
		{name: "19 digits, padded", text: " 1.2345678901234567890 ", lossy: true},
		{name: "underflow", text: "1e-400", lossy: true},
		{name: "subnormal", text: "1.23e-322", lossy: true},
		{name: "whitespace only", text: " \n\t "},
		{name: "quoted", text: `"72.5"`},
		{name: "not a number", text: "7777kg"},
		{name: "out of range", text: "1e400"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Real(80.5)
			err := r.UnmarshalText([]byte(tc.text))
			if err == nil {
				t.Fatalf("UnmarshalText(%q) = nil, r = %v; want a refusal", tc.text, float64(r))
			}
			if r != 80.5 {
				t.Errorf("UnmarshalText(%q) wrote %v to the receiver; want 80.5 unchanged", tc.text, float64(r))
			}
			if errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("UnmarshalText(%q) err = %v; want no canonical-JSON shape sentinel", tc.text, err)
			}
			if got := errors.Is(err, errLossyLiteral); got != tc.lossy {
				t.Errorf("UnmarshalText(%q) err = %v; errors.Is(_, errLossyLiteral) = %t, want %t", tc.text, err, got, tc.lossy)
			}
			if tc.lossy && strings.Contains(err.Error(), strings.TrimSpace(tc.text)) {
				t.Errorf("UnmarshalText(%q) err = %q; must not echo the literal (REQ-093)", tc.text, err)
			}
			if ne, ok := errors.AsType[*strconv.NumError](err); !tc.lossy && (!ok || ne == nil) {
				t.Errorf("UnmarshalText(%q) err = %v (%T); want a *strconv.NumError", tc.text, err, err)
			}
		})
	}
}

// TestRealUnmarshalJSONPrefersItsJSONMethod pins that encoding/json/v2 reads
// a Real through UnmarshalJSON, not the UnmarshalText canonical XML uses
// (REQ-052): a quoted lossy literal keeps its JSON refusal, classified with
// typereg.ErrInvalidShape, and a JSON null leaves the receiver as it was.
func TestRealUnmarshalJSONPrefersItsJSONMethod(t *testing.T) {
	var r Real
	err := json.Unmarshal([]byte(`"9007199254740993"`), &r)
	if !errors.Is(err, typereg.ErrInvalidShape) {
		t.Errorf(`json.Unmarshal("9007199254740993") err = %v; want errors.Is(_, typereg.ErrInvalidShape) from UnmarshalJSON`, err)
	}
	r = 80.5
	if err := json.Unmarshal([]byte(`null`), &r); err != nil || r != 80.5 {
		t.Errorf("json.Unmarshal(null) = %v, r = %v; want nil and 80.5 unchanged", err, float64(r))
	}
}

// TestRealNilReceiverIsRefusedNotPanicked pins the nil-receiver axis
// (idiom.md § No panics, REQ-025): UnmarshalJSON and UnmarshalText assign
// through the pointer, so a nil *Real would be dereferenced. `null` and
// empty text are the discriminating second inputs — each is the value its
// method otherwise reads without a parse, so a nil check placed after that
// arm would let it through. Caller misuse is not a wire-shape problem, so
// the refusal stays outside typereg.ErrInvalidShape. A panic fails the test
// run.
func TestRealNilReceiverIsRefusedNotPanicked(t *testing.T) {
	for _, in := range []string{"80.5", "null"} {
		t.Run("UnmarshalJSON "+in, func(t *testing.T) {
			var r *Real
			err := r.UnmarshalJSON([]byte(in))
			if err == nil {
				t.Fatalf("(*Real)(nil).UnmarshalJSON(%s) = nil error, want a refusal", in)
			}
			if !errors.Is(err, typereg.ErrNilReceiver) {
				t.Errorf("err = %v; want errors.Is(err, typereg.ErrNilReceiver)", err)
			}
			if errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("err = %v; caller misuse must not classify as a wire-shape failure", err)
			}
		})
	}
	for _, in := range []string{"80.5", ""} {
		t.Run("UnmarshalText "+strconv.Quote(in), func(t *testing.T) {
			var r *Real
			err := r.UnmarshalText([]byte(in))
			if err == nil {
				t.Fatalf("(*Real)(nil).UnmarshalText(%q) = nil error, want a refusal", in)
			}
			if !errors.Is(err, typereg.ErrNilReceiver) {
				t.Errorf("err = %v; want errors.Is(err, typereg.ErrNilReceiver)", err)
			}
		})
	}
}
