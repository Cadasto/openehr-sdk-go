package rm

import (
	"bytes"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/refused"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// Integer is the BMM Integer primitive. Decode reads a whole number however
// the literal writes it, so 1, 1.0, 1e0 and "1.0" all decode as 1;
// UnmarshalJSON lists the exact literals it accepts. Encode always writes the
// integer form.
type Integer int32

// UnmarshalJSON accepts a literal in one of two forms, and its value must be
// a whole number in the int32 range:
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
// A literal with a non-zero fraction, or one outside the int32 range, is
// refused, and the receiver keeps its value: nothing is truncated or
// rounded. The refusal is the parse error beneath it, an encoding/json/v2
// *json.SemanticError for a bare number and a *strconv.NumError for a
// quoted one, each reachable with errors.AsType. A quoted literal is named
// once, by that cause, never again by the prefix; the refusal also carries
// the literal's JSON text, quotes included, which a canjson entry point
// holds in DecodeError.Value. The strconv sentinel the cause carries tells
// the two refusals apart however the literal writes the number: a non-zero fraction carries strconv.ErrSyntax,
// whatever the magnitude, and a whole number outside the range carries
// strconv.ErrRange, so errors.Is(err, strconv.ErrRange) holds for
// 2147483648, 2147483648.0 and "1e10" alike.
//
// A nil receiver is refused with an error rather than dereferenced. That
// refusal carries typereg.ErrNilReceiver, not typereg.ErrInvalidShape,
// because caller misuse is not a wire-shape problem.
func (i *Integer) UnmarshalJSON(b []byte) error {
	if i == nil {
		return fmt.Errorf("rm.Integer: %w", typereg.ErrNilReceiver)
	}
	b = bytes.Trim(b, jsonSpace)
	if len(b) == 0 {
		return errors.New("rm.Integer: empty input")
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("rm.Integer: %w", err)
		}
		n, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			w, werr := wholeInt(s, 32)
			if werr != nil {
				if errors.Is(werr, strconv.ErrRange) {
					// ParseInt reports "1e10" as a syntax error; the whole
					// number it writes is out of range, as for "10000000000".
					err = &strconv.NumError{Func: "ParseInt", Num: s, Err: strconv.ErrRange}
				}
				// The *strconv.NumError already quotes the literal once; a
				// second echo here would double it (wire.md § REQ-052). The
				// JSON text as written rides beside it for the entry point's
				// DecodeError.Value, and never reaches the message.
				return refused.Literal(b, fmt.Errorf("rm.Integer: parse quoted literal: %w", err))
			}
			n = w
		}
		*i = Integer(n)
		return nil
	}
	var n int32
	if err := json.Unmarshal(b, &n); err != nil {
		// encoding/json/v2 refuses any literal with a fraction or an
		// exponent for an int32, 1.0 included, so a refused literal is read
		// again as a whole number before the refusal stands.
		w, werr := wholeInt(string(b), 32)
		if werr != nil {
			if se, ok := errors.AsType[*json.SemanticError](err); ok && se != nil && errors.Is(werr, strconv.ErrRange) {
				// encoding/json/v2 reports 1e10 as a syntax error but
				// 10000000000 as a range error; the same whole number gets
				// the same cause however the literal writes it. The copy
				// keeps the codec's own error untouched.
				ranged := *se
				ranged.Err = strconv.ErrRange
				err = &ranged
			}
			return fmt.Errorf("rm.Integer: %w", err)
		}
		n = int32(w)
	}
	*i = Integer(n)
	return nil
}

// maxExponent caps the exponent wholeInt accumulates. Any exponent past it
// already puts a non-zero literal far outside the int64 range, and so the
// int32 range, or far below 1, so capping it keeps the arithmetic from
// overflowing without changing an answer.
const maxExponent = 1 << 40

// wholeInt reads lit as a signed integer of bitSize bits, 32 or 64, when lit
// is a JSON number literal (RFC 8259 § 6) whose value is a whole number in
// that range: 1.0, 1e0, 10e-1 and 0.1e1 all read as 1. Otherwise it returns
// strconv.ErrSyntax for a literal that is not a JSON number or has a non-zero
// digit after the decimal point once the exponent is applied, and
// strconv.ErrRange for a whole number outside the range. A non-zero fraction
// is reported first, so 2147483648.5 at 32 bits and 9223372036854775808.5 at
// 64 bits are strconv.ErrSyntax.
//
// It never goes through a float, which would round 1.0000000000000001 to 1.
// Instead it places the decimal point in the literal's digit run and checks
// that no non-zero digit falls after it. Integer and Integer64 both read
// through it, so the two widths cannot drift.
func wholeInt(lit string, bitSize int) (int64, error) {
	s, neg := strings.CutPrefix(lit, "-")
	mant, exp := s, ""
	hasExp := false
	if k := strings.IndexAny(s, "eE"); k >= 0 {
		mant, exp, hasExp = s[:k], s[k+1:], true
	}
	intPart, frac, hasPoint := strings.Cut(mant, ".")
	if !isDigits(intPart) || (len(intPart) > 1 && intPart[0] == '0') || (hasPoint && !isDigits(frac)) {
		return 0, strconv.ErrSyntax
	}
	var e int64
	if hasExp {
		digits, expNeg := strings.CutPrefix(exp, "-")
		if !expNeg {
			digits = strings.TrimPrefix(digits, "+")
		}
		if !isDigits(digits) {
			return 0, strconv.ErrSyntax
		}
		for _, c := range []byte(digits) {
			if e <= maxExponent {
				e = e*10 + int64(c-'0')
			}
		}
		if expNeg {
			e = -e
		}
	}

	// point is where the decimal point sits in digits: the value is
	// 0.<digits> × 10^point. Leading zeros move it left; trailing zeros do
	// not move it and are dropped, so the first and last digits left are
	// both non-zero.
	digits := intPart + frac
	point := int64(len(intPart)) + e
	trimmed := strings.TrimLeft(digits, "0")
	point -= int64(len(digits) - len(trimmed))
	trimmed = strings.TrimRight(trimmed, "0")
	if trimmed == "" {
		return 0, nil // every digit is zero: the value is 0
	}
	if point < int64(len(trimmed)) {
		return 0, strconv.ErrSyntax // a non-zero digit falls after the decimal point
	}

	// Build the magnitude one integer digit at a time, padding with zeros
	// past the digit run. It is held unsigned because the negative bound,
	// 2^(bitSize-1), is one past the largest positive value of the width.
	// The range check runs before each digit is added, so the magnitude
	// never passes the limit and cannot overflow; the leading digit is
	// non-zero, so the loop stops within 20 digits however large the
	// exponent.
	limit := uint64(1)<<(bitSize-1) - 1
	if neg {
		limit++
	}
	var v uint64
	for k := range point {
		var d uint64
		if k < int64(len(trimmed)) {
			d = uint64(trimmed[k] - '0')
		}
		if v > (limit-d)/10 {
			return 0, strconv.ErrRange // a whole number outside the range
		}
		v = v*10 + d
	}
	if neg {
		// The negated magnitude converts exactly in two's complement, so a
		// magnitude of 2^63 reads as math.MinInt64.
		return int64(-v), nil
	}
	return int64(v), nil
}

// isDigits reports whether s is one or more ASCII decimal digits.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range []byte(s) {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// MarshalJSON emits a JSON number.
func (i Integer) MarshalJSON() ([]byte, error) {
	return json.Marshal(int32(i))
}
