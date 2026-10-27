package rm

import (
	"bytes"
	"encoding"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/refused"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// jsonSpace is the whitespace RFC 8259 allows around a JSON value. A caller
// of an UnmarshalJSON method in this package may pass a value with it, and
// each method trims it before it reads the value's first byte or digits.
const jsonSpace = " \t\r\n"

// Real is the BMM Real primitive. Upstream CDR canonical JSON sometimes
// emits decimal magnitudes as quoted strings; decode accepts both forms.
// Canonical XML reads it through [Real.UnmarshalText]. Either decode refuses
// a lossy literal, one a float64 cannot hold at the precision it is written
// to, rather than round it.
type Real float64

// The canonical-XML codec reaches a Real element through these interfaces,
// so a signature drift would silently fall back to encoding/xml's plain float
// handling and lose the lossy-literal refusal and the xs:double spellings.
// Pin them at compile time.
var (
	_ encoding.TextUnmarshaler = (*Real)(nil)
	_ encoding.TextMarshaler   = Real(0)
)

// xmlSpace is the whitespace XML collapses around the text of an xs:double
// element: space, tab, carriage return and line feed.
const xmlSpace = " \t\r\n"

const (
	// float64Digits is the number of significant decimal digits a normal
	// float64 gives back: a decimal literal of at most this many digits that
	// parses to a float64 of magnitude at least minNormal, written back
	// correctly rounded to the literal's own digit count, is the literal
	// again, so the lossy test skips the write-back for it. Below minNormal a
	// subnormal float64 keeps fewer digits, and such a literal is written back.
	float64Digits = 15
	// minNormal is 2^-1022, the smallest normal float64.
	minNormal = 0x1p-1022
	// maxExactDigits is the largest number of significant decimal digits in
	// the exact value of any float64. A literal carrying more cannot denote a
	// float64, so it is lossy without a write-back, which would otherwise
	// grow with the literal.
	maxExactDigits = 767
)

// significantDigits counts the significant decimal digits in a numeric
// literal s — either a bare JSON number or the content of a quoted
// decimal string. Only the mantissa counts: a leading sign, the decimal
// point, and any exponent part are excluded, as are leading zeros before
// the first nonzero digit and trailing zeros at the end of the digit
// run. It is the digit count the lossy test writes a literal back to.
func significantDigits(s string) int {
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimPrefix(s, "-")
	s = strings.TrimPrefix(s, "+")

	// Counted in one byte pass, with no intermediate digit buffer. digits
	// holds the run from the first nonzero digit onwards; a zero is parked
	// in pending until a later nonzero digit proves it was interior rather
	// than trailing. A zero before the first nonzero digit is dropped
	// outright, which is how "0.1" counts 1 and "1230000" counts 3.
	digits, pending := 0, 0
	for i := range len(s) {
		switch c := s[i]; {
		case c < '0' || c > '9':
			// The decimal point, and anything else that is not a digit,
			// contributes nothing.
		case c == '0':
			if digits > 0 {
				pending++ // interior or trailing — a later digit decides
			}
		default:
			digits += pending + 1
			pending = 0
		}
	}
	return digits
}

// lossy reports whether the literal s, which strconv.ParseFloat or
// encoding/json/v2 parsed to f, is lossy: whether f, written back
// correctly rounded to as many significant decimal digits as s carries,
// denotes a value other than s's own.
//
// A literal with no significant digit, a zero, is never lossy. Nor is one of
// at most float64Digits digits that parsed to a float64 of magnitude at least
// minNormal. That is the decode hot path, and it costs one digit count and no
// allocation. A short literal below minNormal goes to the write-back: a
// subnormal keeps fewer digits, so 1.23e-322, which parses to 1.24e-322, is
// lossy, and a literal that parsed to zero underflowed, which is lossy too.
// A literal that is not decimal, such as the hexadecimal form or Inf, which
// strconv.ParseFloat also reads, is never lossy: the rule governs decimal
// literals.
func lossy(s string, f float64) bool {
	n := significantDigits(s)
	if n == 0 || (n <= float64Digits && math.Abs(f) >= minNormal) {
		return false
	}
	digits, point, ok := decimalForm(s)
	if !ok {
		return false
	}
	if n > maxExactDigits {
		return true
	}
	var buf [64]byte
	back, backPoint, _ := decimalForm(string(strconv.AppendFloat(buf[:0], f, 'e', n-1, 64)))
	return point != backPoint || !sameDigits(digits, back)
}

// decimalForm reads s as a decimal literal: an optional sign, digits with at
// most one decimal point, and an optional exponent, with an underscore
// allowed between digits as strconv.ParseFloat allows it. It returns the run
// of s from its first non-zero digit to its last, and point, where the
// value's decimal point sits: the value is 0.<the run's digits> × 10^point.
// A zero returns an empty run. ok is false when s is not a decimal literal,
// such as the hexadecimal form, Inf or NaN.
func decimalForm(s string) (run string, point int64, ok bool) {
	if s != "" && (s[0] == '+' || s[0] == '-') {
		s = s[1:]
	}
	mant, exp, hasExp := s, "", false
	if k := strings.IndexAny(s, "eE"); k >= 0 {
		mant, exp, hasExp = s[:k], s[k+1:], true
	}
	// n counts the digits read, lead those before the first non-zero digit,
	// and before those before the decimal point.
	var n, lead, before int64
	first, last, hasPoint := -1, -1, false
	for i := range len(mant) {
		switch c := mant[i]; {
		case c >= '1' && c <= '9':
			if first < 0 {
				first, lead = i, n
			}
			last = i
			n++
		case c == '0':
			n++
		case c == '.' && !hasPoint:
			hasPoint, before = true, n
		case c == '_':
		default:
			return "", 0, false
		}
	}
	if n == 0 {
		return "", 0, false
	}
	if !hasPoint {
		before = n
	}
	var e int64
	if hasExp {
		if e, ok = exponent(exp); !ok {
			return "", 0, false
		}
	}
	if first < 0 {
		return "", 0, true
	}
	return mant[first : last+1], before - lead + e, true
}

// exponent reads the exponent part of a decimal literal, after its `e`: an
// optional sign and at least one digit, with an underscore allowed between
// digits. It stops growing at maxExponent, which already puts any non-zero
// literal far outside the float64 range, so the arithmetic cannot overflow.
func exponent(s string) (int64, bool) {
	digits, neg := strings.CutPrefix(s, "-")
	if !neg {
		digits = strings.TrimPrefix(digits, "+")
	}
	var e int64
	seen := false
	for _, c := range []byte(digits) {
		switch {
		case c >= '0' && c <= '9':
			seen = true
			if e <= maxExponent {
				e = e*10 + int64(c-'0')
			}
		case c == '_':
		default:
			return 0, false
		}
	}
	if neg {
		e = -e
	}
	return e, seen
}

// sameDigits reports whether the runs a and b hold the same decimal digits
// in the same order, skipping any other byte, a decimal point or an
// underscore, between them.
func sameDigits(a, b string) bool {
	i, j := 0, 0
	for {
		for i < len(a) && (a[i] < '0' || a[i] > '9') {
			i++
		}
		for j < len(b) && (b[j] < '0' || b[j] > '9') {
			j++
		}
		if i == len(a) || j == len(b) {
			return i == len(a) && j == len(b)
		}
		if a[i] != b[j] {
			return false
		}
		i++
		j++
	}
}

// errLossyLiteral is the refusal of a lossy literal, kept in one place so
// that both decodes and the value-free message text cannot drift apart. It
// carries no shape sentinel, so [Real.UnmarshalText] returns it as it is:
// canonical XML classifies the refusal itself, with canxml.ErrInvalidShape.
var errLossyLiteral = errors.New("rm.Real: literal cannot be held in a float64 at its own precision")

// errLossyShape is errLossyLiteral carrying typereg.ErrInvalidShape, the
// refusal both [Real.UnmarshalJSON] branches return. Each refusal returns it
// inside a per-call refused.Literal carrier holding the literal for the entry
// point's DecodeError.Value; the carrier's message is this one and one
// errors.Unwrap step reaches it, so errors.Is(err, errLossyLiteral) holds.
//
// The sentinel is attached with typereg.ClassifyShape, which leaves the
// message exactly as written: REQ-052 requires the failure's message to be
// unchanged by the classification, and a fmt.Errorf("%w: %w") wrap would
// splice the sentinel's own prose into it.
//
// This runs at package init across the import edge into typereg. It is
// safe: typereg's own package-level vars (ErrInvalidShape included)
// initialise before this rm var, and ClassifyShape reads the sentinel
// only inside its Is method, never while it builds the wrapper here.
var errLossyShape = typereg.ClassifyShape(errLossyLiteral)

// parseDecimal parses a decimal Real literal: the content of a quoted JSON
// literal, which ADR 0004 admits as a decimal string only, and a number in
// canonical-XML element text, where the xs:double lexical space is decimal
// apart from its special values. It refuses the other spellings
// strconv.ParseFloat reads (hexadecimal, Inf, Infinity, NaN, and an
// underscore between digits) with the *strconv.NumError ParseFloat gives a
// malformed literal; a NaN or an infinity would also fail canjson encode.
// A decimal literal is an optional sign, digits with at most one decimal
// point, and an optional exponent of an optional sign and digits.
func parseDecimal(s string) (float64, error) {
	mant, exp, hasExp := s, "", false
	if k := strings.IndexAny(s, "eE"); k >= 0 {
		mant, exp, hasExp = s[:k], s[k+1:], true
	}
	intPart, frac, _ := strings.Cut(trimSign(mant), ".")
	if (intPart == "" && frac == "") ||
		(intPart != "" && !isDigits(intPart)) ||
		(frac != "" && !isDigits(frac)) ||
		(hasExp && !isDigits(trimSign(exp))) {
		return 0, &strconv.NumError{Func: "ParseFloat", Num: s, Err: strconv.ErrSyntax}
	}
	return strconv.ParseFloat(s, 64)
}

// trimSign drops one leading sign from s.
func trimSign(s string) string {
	if s != "" && (s[0] == '+' || s[0] == '-') {
		return s[1:]
	}
	return s
}

// UnmarshalJSON accepts a JSON number or a decimal string. A lossy literal,
// one the float64 it parses to does not give back at the literal's own count
// of significant digits, fails rather than silently rounding; the error
// message does not echo the value. A JSON null is a no-op per the
// encoding/json convention for Unmarshaler ("approximate the behavior of
// Unmarshal itself"), leaving the receiver unchanged rather than writing the
// zero value, as rm.Character does.
//
// The two checks run in this order. The literal is parsed first, into a
// temporary, and a parse or range failure is returned as it comes:
// *strconv.NumError from the quoted arm, an encoding/json/v2
// *json.SemanticError (or a *jsontext.SyntacticError for a malformed
// literal) from the bare arm, each reachable with errors.As and each
// staying outside typereg.ErrInvalidShape. Only a literal that parsed is
// then tested for loss, and only a literal that passed both is assigned to
// the receiver. Reversing the order would report "1e400" or
// "123456789012345678x" as a lossy literal, which is a misdiagnosis: the
// value never parsed at all.
//
// Two refusals also carry the literal's JSON text as the document wrote it,
// quotes included for a quoted literal, which a canjson entry point holds in
// DecodeError.Value: a quoted literal that does not parse, and a lossy
// literal. The text adds nothing to the message, which names a quoted
// literal once, in the strconv cause.
//
// A nil receiver is refused with an error rather than dereferenced. That
// refusal carries typereg.ErrNilReceiver, not typereg.ErrInvalidShape,
// because caller misuse is not a wire-shape problem.
func (r *Real) UnmarshalJSON(b []byte) error {
	if r == nil {
		return fmt.Errorf("rm.Real: %w", typereg.ErrNilReceiver)
	}
	b = bytes.Trim(b, jsonSpace)
	if len(b) == 0 {
		return errors.New("rm.Real: empty input")
	}
	if string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("rm.Real: %w", err)
		}
		f, err := parseDecimal(s)
		if err != nil {
			// wire.md § REQ-052: the literal appears once, in the
			// *strconv.NumError cause, and in the entry point's
			// DecodeError.Value, which the carrier fills from the JSON text as
			// written. The prefix does not repeat it.
			return refused.Literal(b, fmt.Errorf("rm.Real: parse quoted literal: %w", err))
		}
		if lossy(s, f) {
			return refused.Literal(b, errLossyShape)
		}
		*r = Real(f)
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("rm.Real: %w", err)
	}
	if lossy(string(b), f) {
		return refused.Literal(b, errLossyShape)
	}
	*r = Real(f)
	return nil
}

// UnmarshalText implements [encoding.TextUnmarshaler] so that canonical XML
// element content is read in the xs:double lexical space and held to the
// lossy-literal rule canonical JSON applies. The generated canonical-XML
// decoder reaches a Real element through xmlunmatched, which hands its text
// to this method.
//
// Empty text is 0, and XML whitespace (space, tab, carriage return and line
// feed) around the text is trimmed. The rest is a decimal literal, an
// optional sign, digits with at most one decimal point and an optional
// exponent, or one of INF, +INF, -INF and NaN. Any other text, such as the
// hexadecimal form, a digit separator, Inf, Infinity or nan, whitespace alone
// and a quoted number among them, is refused with a *strconv.NumError as
// strconv.ParseFloat gives it for text that does not spell a number; so is a
// literal out of the float64 range. A number that parsed and is lossy is
// refused with a value-free error.
//
// Neither refusal carries typereg.ErrInvalidShape: that sentinel's own text
// names canonical JSON. canxml classifies the refusal itself: it comes back
// in a canxml.DecodeError that names the element's path and wraps
// canxml.ErrInvalidShape. A nil receiver is refused rather than
// dereferenced, on the same terms as [Real.UnmarshalJSON].
//
// encoding/json prefers [Real.UnmarshalJSON] over this method for every JSON
// value, so the JSON surface is unchanged by its presence.
func (r *Real) UnmarshalText(text []byte) error {
	if r == nil {
		return fmt.Errorf("rm.Real: %w", typereg.ErrNilReceiver)
	}
	if len(text) == 0 {
		*r = 0
		return nil
	}
	var f float64
	switch s := strings.Trim(string(text), xmlSpace); s {
	case "INF", "+INF":
		f = math.Inf(1)
	case "-INF":
		f = math.Inf(-1)
	case "NaN":
		f = math.NaN()
	default:
		var err error
		if f, err = parseDecimal(s); err != nil {
			// Unwrapped, so the refusal reads exactly as encoding/xml's own did.
			return err
		}
		if lossy(s, f) {
			return errLossyLiteral
		}
	}
	*r = Real(f)
	return nil
}

// MarshalText implements [encoding.TextMarshaler], the encode counterpart of
// [Real.UnmarshalText] that the generated canonical-XML encoder writes a Real
// element through. It writes an infinity as INF or -INF and a not-a-number as
// NaN, the xs:double spellings, and a finite value as the shortest decimal
// that reads back as the same float64, as encoding/xml writes a float64.
//
// encoding/json prefers [Real.MarshalJSON] over this method, so canonical
// JSON is unchanged by its presence.
func (r Real) MarshalText() ([]byte, error) {
	f := float64(r)
	switch {
	case math.IsInf(f, 1):
		return []byte("INF"), nil
	case math.IsInf(f, -1):
		return []byte("-INF"), nil
	case math.IsNaN(f):
		return []byte("NaN"), nil
	}
	return strconv.AppendFloat(nil, f, 'g', -1, 64), nil
}

// MarshalJSON emits a JSON number.
func (r Real) MarshalJSON() ([]byte, error) {
	return json.Marshal(float64(r))
}
