package rm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// Real is the BMM Real primitive. Upstream CDR canonical JSON sometimes
// emits decimal magnitudes as quoted strings; decode accepts both forms.
type Real float64

// maxSignificantDigits is the trigger this SDK uses to detect a decimal
// literal that carries more precision than float64 can represent
// (REQ-052's "rather than silently rounding" decode clause; see
// significantDigits). It is a lower bound, not an equivalence: every
// float64 can be printed with at most 17 significant decimal digits and
// read back identically (float64's shortest-round-trip guarantee), so a
// literal past that point certainly cannot round-trip. The converse
// does not hold — a shorter literal can still lose precision, e.g.
// 2^53+1 = 9007199254740993 (16 digits) decodes to 9007199254740992 —
// and any decimal fraction that is not a power of two (0.1 included) is
// binary-inexact regardless of digit count. Catching every such case
// would need an exactness check (big.Float), which the plan rejected
// because it would also reject 0.1: the SDK deliberately accepts a
// shorter, binary-inexact literal unreported rather than over-report.
const maxSignificantDigits = 17

// significantDigits counts the significant decimal digits in a numeric
// literal s — either a bare JSON number or the content of a quoted
// decimal string. Only the mantissa counts: a leading sign, the decimal
// point, and any exponent part are excluded, as are leading zeros before
// the first nonzero digit and trailing zeros at the end of the digit
// run. Used to detect a literal that carries more precision than
// float64 can represent (REQ-052).
func significantDigits(s string) int {
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimPrefix(s, "-")
	s = strings.TrimPrefix(s, "+")

	var digits []byte
	for i := range len(s) {
		if c := s[i]; c >= '0' && c <= '9' {
			digits = append(digits, c)
		}
	}
	start := 0
	for start < len(digits) && digits[start] == '0' {
		start++
	}
	digits = digits[start:]
	end := len(digits)
	for end > 0 && digits[end-1] == '0' {
		end--
	}
	return end
}

// errPrecisionLoss is returned, wrapping typereg.ErrInvalidShape, when a
// decimal literal exceeds maxSignificantDigits. Kept in one place (both
// UnmarshalJSON branches share it) so the sentinel and the value-free
// message text (REQ-093) cannot drift between them.
var errPrecisionLoss = fmt.Errorf("rm.Real: %w: value carries more precision than float64 can represent", typereg.ErrInvalidShape)

// UnmarshalJSON accepts a JSON number or a decimal string. A literal
// carrying more than maxSignificantDigits significant digits fails
// rather than silently rounding (REQ-052); the error is value-free
// (REQ-093). A JSON null is a no-op per the encoding/json convention
// for Unmarshaler ("approximate the behavior of Unmarshal itself"),
// leaving the receiver unchanged rather than writing the zero value —
// mirroring rm.Character.
func (r *Real) UnmarshalJSON(b []byte) error {
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
		if significantDigits(s) > maxSignificantDigits {
			return errPrecisionLoss
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return fmt.Errorf("rm.Real: parse %q: %w", s, err)
		}
		*r = Real(f)
		return nil
	}
	if significantDigits(string(b)) > maxSignificantDigits {
		return errPrecisionLoss
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("rm.Real: %w", err)
	}
	*r = Real(f)
	return nil
}

// MarshalJSON emits a JSON number per REQ-052.
func (r Real) MarshalJSON() ([]byte, error) {
	return json.Marshal(float64(r))
}
