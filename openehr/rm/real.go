package rm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Real is the BMM Real primitive. Upstream CDR canonical JSON sometimes
// emits decimal magnitudes as quoted strings; decode accepts both forms.
type Real float64

// maxSignificantDigits is the digit-count budget this SDK uses to
// decide that a decimal literal carries more precision than it will
// accept (REQ-052's "rather than silently rounding" decode clause; see
// significantDigits). 17 is float64's shortest-round-trip maximum: every
// float64 can be printed with at most 17 significant decimal digits and
// read back identically.
//
// It is a digit-count budget, NOT an exactness test, and
// docs/specifications/wire.md § Floating-point precision records that it
// is wrong in both directions:
//
//   - False positive — a literal past 17 digits can still be exactly
//     representable and is refused anyway. 18446744073709551616 (2^64)
//     is exact in float64 yet carries 20 significant digits, so the
//     budget refuses it.
//   - False negative — a shorter literal can be binary-inexact and is
//     accepted unreported. 9007199254740993 (2^53+1, 16 digits) decodes
//     to 9007199254740992, and any decimal fraction that is not a power
//     of two (0.1 included) is binary-inexact regardless of digit count.
//
// Classifying both correctly would need an exactness check (big.Float),
// which the plan rejected because it would also reject 0.1 and every
// other ordinary binary-inexact clinical literal. >17 is chosen instead
// as a cheap, deterministic budget that never reports a short clinical
// literal; it is not a losslessness guarantee.
const maxSignificantDigits = 17

// significantDigits counts the significant decimal digits in a numeric
// literal s — either a bare JSON number or the content of a quoted
// decimal string. Only the mantissa counts: a leading sign, the decimal
// point, and any exponent part are excluded, as are leading zeros before
// the first nonzero digit and trailing zeros at the end of the digit
// run. Used to apply the maxSignificantDigits budget (REQ-052) — it
// counts digits and makes no claim about representability.
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

// errPrecisionLoss is returned, carrying typereg.ErrInvalidShape, when a
// decimal literal exceeds maxSignificantDigits. Kept in one place (both
// UnmarshalJSON branches share it) so the sentinel and the value-free
// message text (REQ-093) cannot drift between them.
//
// The text names the digit budget rather than claiming the value is
// unrepresentable, because for the budget's documented false positive
// (2^64) that claim is simply false — see maxSignificantDigits. The
// sentinel is attached with classifyShape, which leaves the message
// exactly as written: REQ-052 requires the failure's message to be
// unchanged by the classification, and a fmt.Errorf("%w: %w") wrap would
// splice the sentinel's own prose into it.
var errPrecisionLoss = classifyShape(fmt.Errorf("rm.Real: literal carries more than %d significant decimal digits", maxSignificantDigits))

// UnmarshalJSON accepts a JSON number or a decimal string. A literal
// carrying more than maxSignificantDigits significant digits fails
// rather than silently rounding (REQ-052); the error is value-free
// (REQ-093). A JSON null is a no-op per the encoding/json convention
// for Unmarshaler ("approximate the behavior of Unmarshal itself"),
// leaving the receiver unchanged rather than writing the zero value —
// mirroring rm.Character.
//
// The two checks run in this order: the literal is PARSED first, into a
// temporary, and a parse or range failure is returned as it comes —
// *strconv.NumError from the quoted arm, *json.UnmarshalTypeError from
// the bare arm, each reachable with errors.As and each staying outside
// typereg.ErrInvalidShape. Only a literal that parsed is then measured
// against maxSignificantDigits, and only a literal that passed both is
// assigned to the receiver. Reversing the order would report
// "1e400" or "123456789012345678x" as precision loss, which is a
// misdiagnosis: the value never parsed at all.
//
// A nil receiver is refused rather than dereferenced (REQ-025, idiom.md
// § No panics): the method assigns through the pointer, and a nil
// pointer is caller-constructible input reachable through the documented
// API. That refusal is a plain error, outside typereg.ErrInvalidShape —
// caller misuse is not a wire-shape problem.
func (r *Real) UnmarshalJSON(b []byte) error {
	if r == nil {
		return errors.New("rm.Real: nil receiver")
	}
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
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return fmt.Errorf("rm.Real: parse %q: %w", s, err)
		}
		if significantDigits(s) > maxSignificantDigits {
			return errPrecisionLoss
		}
		*r = Real(f)
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("rm.Real: %w", err)
	}
	if significantDigits(string(b)) > maxSignificantDigits {
		return errPrecisionLoss
	}
	*r = Real(f)
	return nil
}

// MarshalJSON emits a JSON number per REQ-052.
func (r Real) MarshalJSON() ([]byte, error) {
	return json.Marshal(float64(r))
}
