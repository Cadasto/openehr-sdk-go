package rm

import (
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

// Character is the BMM Character primitive. In canonical openEHR JSON it is
// written as a single-character string (e.g. TERM_MAPPING.match "="), never a
// number. Its zero value ("") is not a legal Character, so an omitted attribute
// is distinguishable from a supplied one. Mirrors the named-primitive shape of
// Integer/Real. REQ-046 / REQ-052.
type Character string

// UnmarshalJSON accepts a canonical one-character JSON string. For backward
// compatibility with the pre-fix encoder — which wrote match as a number — a
// JSON number is also accepted on decode, but it is never the encoded form
// (REQ-052 point 4). A JSON null is a no-op per the encoding/json convention
// for Unmarshaler ("approximate the behavior of Unmarshal itself"), leaving
// the receiver unchanged rather than writing the zero value.
func (c *Character) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return errors.New("rm.Character: empty input")
	}
	if string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("rm.Character: %w", err)
		}
		if count := utf8.RuneCountInString(s); count != 1 {
			return fmt.Errorf("rm.Character: must be exactly one character, got %d", count)
		}
		*c = Character(s)
		return nil
	}
	// Back-compat only (REQ-052 point 4) — never the encoded form. n is
	// declared rune, not int32, so the conversion below reads as intent: Go
	// treats an integer-to-string conversion as "interpret this as a Unicode
	// code point", not as formatting decimal digits, which is exactly what a
	// legacy numeric match code (e.g. 61 for '=') needs. A code point of 0 or
	// one utf8.ValidRune rejects (negative, a UTF-16 surrogate half, or beyond
	// the Unicode range) is refused rather than silently mapped to U+0000 or
	// U+FFFD.
	var n rune
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("rm.Character: %w", err)
	}
	if n == 0 || !utf8.ValidRune(n) {
		return errors.New("rm.Character: number is not a usable code point")
	}
	*c = Character(n)
	return nil
}

// MarshalJSON emits a one-character JSON string. An empty or multi-rune value is
// an encode error (never silently coerced), which canjson wraps as
// ErrInvalidValue (REQ-052 point 3). Diagnostics are value-free (REQ-093).
func (c Character) MarshalJSON() ([]byte, error) {
	if count := utf8.RuneCountInString(string(c)); count != 1 {
		return nil, fmt.Errorf("rm.Character: must be exactly one character, got %d", count)
	}
	return json.Marshal(string(c))
}
