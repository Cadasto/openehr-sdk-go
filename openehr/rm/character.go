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
// (REQ-052 point 4).
func (c *Character) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return errors.New("rm.Character: empty input")
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("rm.Character: %w", err)
		}
		if utf8.RuneCountInString(s) != 1 {
			return fmt.Errorf("rm.Character: must be exactly one character, got %d", utf8.RuneCountInString(s))
		}
		*c = Character(s)
		return nil
	}
	var n int32
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("rm.Character: %w", err)
	}
	*c = Character(n)
	return nil
}

// MarshalJSON emits a one-character JSON string. An empty or multi-rune value is
// an encode error (never silently coerced), which canjson wraps as
// ErrInvalidValue (REQ-052 point 3). Diagnostics are value-free (REQ-093).
func (c Character) MarshalJSON() ([]byte, error) {
	if utf8.RuneCountInString(string(c)) != 1 {
		return nil, fmt.Errorf("rm.Character: must be exactly one character, got %d", utf8.RuneCountInString(string(c)))
	}
	return json.Marshal(string(c))
}
