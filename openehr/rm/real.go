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

// maxSignificantDigits is float64's shortest-round-trip guarantee: any
// decimal literal within this many significant digits round-trips
// through float64 without loss, so it is the trigger for REQ-052's
// "rather than silently rounding" decode clause (see significantDigits).
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

// UnmarshalJSON accepts a JSON number or a decimal string. A literal
// carrying more than maxSignificantDigits significant digits fails
// rather than silently rounding (REQ-052); the error is value-free
// (REQ-093).
func (r *Real) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return errors.New("rm.Real: empty input")
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("rm.Real: %w", err)
		}
		if significantDigits(s) > maxSignificantDigits {
			return fmt.Errorf("rm.Real: %w: value carries more precision than float64 can represent", typereg.ErrInvalidShape)
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return fmt.Errorf("rm.Real: parse %q: %w", s, err)
		}
		*r = Real(f)
		return nil
	}
	if significantDigits(string(b)) > maxSignificantDigits {
		return fmt.Errorf("rm.Real: %w: value carries more precision than float64 can represent", typereg.ErrInvalidShape)
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
