package rm

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// Character is the BMM Character primitive. In canonical openEHR JSON it is
// written as a single-character string (e.g. TERM_MAPPING.match "="), never a
// number. Its zero value ("") is not a legal Character, so an omitted attribute
// is distinguishable from a supplied one. Mirrors the named-primitive shape of
// Integer/Real. REQ-046 / REQ-052.
//
// Validity is a property of the value, not of the codec that carried it: the
// same one-rune rule (see characterFault) gates canonical JSON via
// [Character.UnmarshalJSON] / [Character.MarshalJSON] and canonical XML via the
// [encoding.TextUnmarshaler] / [encoding.TextMarshaler] implementations below,
// so an empty or multi-character `match` cannot slip in through XML element
// content the way a plain string field would.
type Character string

// The canonical-XML codec reaches `match` through these two interfaces, so
// a signature drift would silently fall back to encoding/xml's plain-string
// handling and lose the invariant. Pin them at compile time.
var (
	_ encoding.TextUnmarshaler = (*Character)(nil)
	_ encoding.TextMarshaler   = Character("")
)

// characterFault reports what disqualifies s from being a Character —
// bytes that are not valid UTF-8, a rune count other than one, or that
// single rune being U+FFFD — and nil when s is a legal Character.
//
// It is the one place the rule lives, shared by every entry point
// (UnmarshalJSON's string and number arms, MarshalJSON, UnmarshalText,
// MarshalText) so the four cannot drift apart. The returned error
// carries the value-free reason alone (REQ-093); each caller adds its
// own "rm.Character: " prefix, and only the decode callers add
// typereg.ErrInvalidShape on top.
//
// U+FFFD is refused because it is what malformed input decodes to:
// encoding/json substitutes the replacement character, reporting no
// error, for a lone UTF-16 surrogate escape ("\uD800") and for invalid
// raw UTF-8. A rune count alone sees one rune there and would launder
// corrupted input into an apparently valid Character. A genuine U+FFFD
// on the wire is refused with it — indistinguishable from the
// substituted form, and not a `match` character any producer needs.
func characterFault(s string) error {
	if !utf8.ValidString(s) {
		return errors.New("value is not valid UTF-8")
	}
	if count := utf8.RuneCountInString(s); count != 1 {
		return fmt.Errorf("must be exactly one character, got %d", count)
	}
	if r, _ := utf8.DecodeRuneInString(s); r == utf8.RuneError {
		return errors.New("value is the Unicode replacement character, which malformed input decodes to")
	}
	return nil
}

// UnmarshalJSON accepts a canonical one-character JSON string. For backward
// compatibility with the pre-fix encoder — which wrote match as a number — a
// JSON number is also accepted on decode, but it is never the encoded form
// (REQ-052 point 4). A JSON null is a no-op per the encoding/json convention
// for Unmarshaler ("approximate the behavior of Unmarshal itself"), leaving
// the receiver unchanged rather than writing the zero value.
//
// Every refusal on this decode path wraps typereg.ErrInvalidShape, so a
// bare-Character decode classifies the way the generated TERM_MAPPING funnel
// would (REQ-052 § decode-side shape sentinel); the underlying
// encoding/json error stays reachable through unwrapping, and the
// diagnostic text is unchanged by the classification. The encode
// direction deliberately stays outside that sentinel — canjson attaches
// its own encode-only ErrInvalidValue there.
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
			return fmt.Errorf("rm.Character: %w: %w", typereg.ErrInvalidShape, err)
		}
		if err := characterFault(s); err != nil {
			return fmt.Errorf("rm.Character: %w: %w", typereg.ErrInvalidShape, err)
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
		return fmt.Errorf("rm.Character: %w: %w", typereg.ErrInvalidShape, err)
	}
	if n == 0 || !utf8.ValidRune(n) {
		return fmt.Errorf("rm.Character: %w: number is not a usable code point", typereg.ErrInvalidShape)
	}
	// The rune is a code point; characterFault decides whether it is a
	// Character. It is the same gate the string arm and the encoders use,
	// so this arm cannot decode a value the encoders would then refuse —
	// which a number naming U+FFFD (65533) otherwise would.
	s := string(n)
	if err := characterFault(s); err != nil {
		return fmt.Errorf("rm.Character: %w: %w", typereg.ErrInvalidShape, err)
	}
	*c = Character(s)
	return nil
}

// MarshalJSON emits a one-character JSON string. A value that is not a single
// valid UTF-8 character — empty, multi-rune, malformed bytes, or U+FFFD — is an
// encode error (never silently coerced, and never emitted as "�"), which
// canjson wraps as ErrInvalidValue (REQ-052 point 3). Diagnostics are
// value-free (REQ-093).
func (c Character) MarshalJSON() ([]byte, error) {
	if err := characterFault(string(c)); err != nil {
		return nil, fmt.Errorf("rm.Character: %w", err)
	}
	return json.Marshal(string(c))
}

// UnmarshalText implements [encoding.TextUnmarshaler] so canonical XML
// element content is held to the same rule as canonical JSON. The
// generated canonical-XML decoder reaches `match` through
// xml.Decoder.DecodeElement on the plain string kind, which consults
// this method; without it an empty or multi-character `match` would pass
// through XML unvalidated (REQ-046 / REQ-052).
//
// There is no numeric back-compat arm here: the pre-fix encoder wrote a
// number in JSON only, and XML element content carries no JSON number
// kind to be tolerant of.
//
// The error is a plain error, deliberately outside typereg.ErrInvalidShape:
// that sentinel's own text names canonical JSON, and canxml classifies
// nothing at element level — it returns encoding/xml's errors unchanged
// and reserves canxml.ErrInvalidShape for xmi:type rejection and for a
// nil / non-xml.Marshaler root.
//
// encoding/json prefers [Character.UnmarshalJSON] over this method for
// every JSON value, so the JSON surface is unchanged by its presence.
func (c *Character) UnmarshalText(text []byte) error {
	s := string(text)
	if err := characterFault(s); err != nil {
		return fmt.Errorf("rm.Character: %w", err)
	}
	*c = Character(s)
	return nil
}

// MarshalText implements [encoding.TextMarshaler], the encode counterpart
// of [Character.UnmarshalText]: the generated canonical-XML encoder emits
// `match` via xml.Encoder.EncodeElement, which consults this method, so an
// empty or multi-character value is an encode error in XML exactly as it is
// in JSON rather than an empty or over-long element (REQ-052).
//
// encoding/json prefers [Character.MarshalJSON] over this method, so the
// JSON surface is unchanged by its presence.
func (c Character) MarshalText() ([]byte, error) {
	if err := characterFault(string(c)); err != nil {
		return nil, fmt.Errorf("rm.Character: %w", err)
	}
	return []byte(c), nil
}
