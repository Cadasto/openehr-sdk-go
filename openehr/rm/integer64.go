package rm

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strconv"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/refused"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// Integer64 is the BMM Integer64 primitive, the 64-bit counterpart of
// [Integer]. Decode reads a whole number however the literal writes it, so 1,
// 1.0, 1e0 and "1.0" all decode as 1; UnmarshalJSON lists the exact literals
// it accepts. Encode always writes the integer form.
type Integer64 int64

// UnmarshalJSON accepts a literal in one of two forms, and its value must be
// a whole number in the int64 range:
//
//   - A bare JSON number. It may write the whole number with a zero fraction
//     or an exponent (1.0, 1e0, 10e-1).
//   - A quoted string holding either a decimal integer as strconv.ParseInt
//     reads it in base 10, which allows a leading sign and leading zeros
//     ("+1", "01", "007"), or a JSON number whose value is whole ("1.0",
//     "1e0"). That second reading keeps the JSON number grammar, so "+1.0"
//     and "01.0" are refused.
//
// The whole-number check works on the literal's decimal digits, never
// through a float.
//
// A literal with a non-zero fraction, or one outside the int64 range, is
// refused, and the receiver keeps its value: nothing is truncated or
// rounded. The refusal is the parse error beneath it, an encoding/json/v2
// *json.SemanticError for a bare number and a *strconv.NumError for a
// quoted one, each reachable with errors.AsType. A quoted literal is named
// once, by that cause, never again by the prefix; the refusal also carries
// the literal's JSON text, quotes included, which a canjson entry point
// holds in DecodeError.Value. The strconv sentinel the cause carries tells
// the two refusals apart however the literal writes the number: a non-zero
// fraction carries strconv.ErrSyntax, whatever the magnitude, and a whole
// number outside the range carries strconv.ErrRange, so
// errors.Is(err, strconv.ErrRange) holds for 9223372036854775808,
// 9223372036854775808.0 and "1e19" alike.
//
// A nil receiver is refused with an error rather than dereferenced. That
// refusal carries typereg.ErrNilReceiver, not typereg.ErrInvalidShape,
// because caller misuse is not a wire-shape problem.
func (i *Integer64) UnmarshalJSON(b []byte) error {
	if i == nil {
		return fmt.Errorf("rm.Integer64: %w", typereg.ErrNilReceiver)
	}
	if len(b) == 0 {
		return errors.New("rm.Integer64: empty input")
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("rm.Integer64: %w", err)
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			w, werr := wholeInt(s, 64)
			if werr != nil {
				if errors.Is(werr, strconv.ErrRange) {
					// ParseInt reports "1e19" as a syntax error; the whole
					// number it writes is out of range, as for
					// "10000000000000000000".
					err = &strconv.NumError{Func: "ParseInt", Num: s, Err: strconv.ErrRange}
				}
				// The *strconv.NumError already quotes the literal once; a
				// second echo here would double it (wire.md § REQ-052). The
				// JSON text as written rides beside it for the entry point's
				// DecodeError.Value, and never reaches the message.
				return refused.Literal(b, fmt.Errorf("rm.Integer64: parse quoted literal: %w", err))
			}
			n = w
		}
		*i = Integer64(n)
		return nil
	}
	var n int64
	if err := json.Unmarshal(b, &n); err != nil {
		// encoding/json/v2 refuses any literal with a fraction or an
		// exponent for an int64, 1.0 included, so a refused literal is read
		// again as a whole number before the refusal stands.
		w, werr := wholeInt(string(b), 64)
		if werr != nil {
			if se, ok := errors.AsType[*json.SemanticError](err); ok && se != nil && errors.Is(werr, strconv.ErrRange) {
				// encoding/json/v2 reports 1e19 as a syntax error but
				// 10000000000000000000 as a range error; the same whole
				// number gets the same cause however the literal writes it.
				// The copy keeps the codec's own error untouched.
				ranged := *se
				ranged.Err = strconv.ErrRange
				err = &ranged
			}
			return fmt.Errorf("rm.Integer64: %w", err)
		}
		n = w
	}
	*i = Integer64(n)
	return nil
}

// MarshalJSON emits a JSON number.
func (i Integer64) MarshalJSON() ([]byte, error) {
	return json.Marshal(int64(i))
}
