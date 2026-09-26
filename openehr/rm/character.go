package rm

import (
	"encoding"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// Character is the BMM Character primitive. In canonical openEHR JSON it is
// written as a single-character string (e.g. TERM_MAPPING.match "="), never a
// number. Its zero value ("") is not a legal Character, so an omitted attribute
// is distinguishable from a supplied one. It has the same named-primitive shape
// as Integer and Real.
//
// The same one-rune rule applies whichever codec carried the value. It checks
// canonical JSON via [Character.UnmarshalJSON] / [Character.MarshalJSON] and
// canonical XML via the [encoding.TextUnmarshaler] / [encoding.TextMarshaler]
// implementations,
// so an empty or multi-character `match` cannot slip in through XML element
// content the way a plain string field would.
//
// The BASE Character primitive excludes no code point, so U+FFFD (the Unicode
// replacement character) is itself a legal Character and survives every codec.
// A U+FFFD that was manufactured out of corrupted input never reaches this type
// on the JSON path: encoding/json/v2's tokenizer refuses invalid UTF-8 and a
// lone UTF-16 surrogate escape while reading the value, so the string arm only
// ever sees bytes that already decoded cleanly.
type Character string

// The canonical-XML codec reaches `match` through these two interfaces, so
// a signature drift would silently fall back to encoding/xml's plain-string
// handling and lose the invariant. Pin them at compile time.
var (
	_ encoding.TextUnmarshaler = (*Character)(nil)
	_ encoding.TextMarshaler   = Character("")
)

// characterFault reports what disqualifies s from being a Character —
// bytes that are not valid UTF-8, or a rune count other than one — and
// nil when s is a legal Character.
//
// It is the one place the value rule lives, shared by every entry point
// (UnmarshalJSON's string and number arms, MarshalJSON, UnmarshalText,
// MarshalText) so the five cannot drift apart. The returned error
// carries the value-free reason alone (REQ-093); each caller adds its
// own "rm.Character: " prefix, and only the decode callers classify it
// with typereg.ErrInvalidShape (see typereg.ClassifyShape).
//
// Every code point passes, U+FFFD included: the BASE Character
// primitive excludes none, so refusing one here would make a legal
// character unrepresentable in JSON, text and XML alike. There is no
// narrower "genuine or substituted" question left to answer on the JSON
// path: the encoding/json/v2 tokenizer refuses invalid UTF-8 and a lone
// surrogate escape before the value is decoded, so a U+FFFD that arrives
// here was written as itself (ruling R15).
func characterFault(s string) error {
	if !utf8.ValidString(s) {
		return errors.New("value is not valid UTF-8")
	}
	if count := utf8.RuneCountInString(s); count != 1 {
		return fmt.Errorf("must be exactly one character, got %d", count)
	}
	return nil
}

// UnmarshalJSON accepts a canonical one-character JSON string. For backward
// compatibility with older SDK encoders, which wrote match as a number, a JSON
// number holding a Unicode code point is also accepted on decode; it is never
// the encoded form. A JSON null is a no-op per the encoding/json convention for Unmarshaler
// ("approximate the behavior of Unmarshal itself"), leaving the receiver
// unchanged rather than writing the zero value.
//
// The string arm runs two checks: the literal must decode, and the
// decoded value must be one rune. The encoding/json/v2 tokenizer refuses
// invalid UTF-8 and a lone UTF-16 surrogate escape while reading the
// value, so a corrupted spelling fails at json.Unmarshal and never
// decodes to a U+FFFD. A genuine U+FFFD, written as itself, decodes and
// passes the one-rune rule, because the BASE primitive admits it.
//
// Every refusal on this decode path carries typereg.ErrInvalidShape, so a
// bare-Character decode classifies the same way as a decode through the
// generated TERM_MAPPING code. The classification works through errors.Is
// and leaves the message and the cause untouched (see
// typereg.ClassifyShape), so the codec's own error stays both readable and
// reachable with errors.AsType. Encode errors are not classified with that
// sentinel; canjson wraps them in its own encode-only ErrInvalidValue.
//
// A nil receiver is refused with an error rather than dereferenced. That
// refusal carries typereg.ErrNilReceiver and not typereg.ErrInvalidShape,
// because caller misuse is not a wire-shape problem.
func (c *Character) UnmarshalJSON(b []byte) error {
	if c == nil {
		return fmt.Errorf("rm.Character: %w", typereg.ErrNilReceiver)
	}
	if len(b) == 0 {
		return errors.New("rm.Character: empty input")
	}
	if string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		// json.Unmarshal (encoding/json/v2) refuses invalid UTF-8 and a lone
		// UTF-16 surrogate escape here, so no manufactured U+FFFD survives to
		// reach characterFault (ruling R15).
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return typereg.ClassifyShape(fmt.Errorf("rm.Character: %w", err))
		}
		if err := characterFault(s); err != nil {
			return typereg.ClassifyShape(fmt.Errorf("rm.Character: %w", err))
		}
		*c = Character(s)
		return nil
	}
	// Back-compat only (§ REQ-052's `TERM_MAPPING.match` bullet) — never the
	// encoded form. n is declared rune, not int32, so the conversion below
	// reads as intent: Go treats an integer-to-string conversion as
	// "interpret this as a Unicode code point", not as formatting decimal
	// digits, which is exactly what a legacy numeric match code (e.g. 61 for
	// '=') needs. A code point that is 0, or that utf8.ValidRune rejects
	// (negative, a UTF-16 surrogate half, or beyond the Unicode range), is
	// refused rather than silently mapped to U+0000 or U+FFFD.
	var n rune
	if err := json.Unmarshal(b, &n); err != nil {
		return typereg.ClassifyShape(fmt.Errorf("rm.Character: %w", err))
	}
	if n == 0 || !utf8.ValidRune(n) {
		return typereg.ClassifyShape(errors.New("rm.Character: number is not a usable code point"))
	}
	// Past that gate string(n) is one valid UTF-8 rune by construction, so
	// characterFault has nothing left to add. 65533 is accepted here: a
	// number names a code point and carries no bytes that could have been
	// substituted, so U+FFFD spelled numerically is always genuine.
	*c = Character(string(n))
	return nil
}

// MarshalJSON emits a one-character JSON string. A value that is not a single
// valid UTF-8 character (empty, multi-rune, or malformed bytes) is an encode
// error, never silently coerced and never emitted as "�"; canjson wraps it as
// ErrInvalidValue. A U+FFFD value encodes normally: it is a legal Character,
// and on this side there are no wire bytes that could have been substituted.
// Error messages do not echo the value.
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
// this method, so an empty or multi-character `match` is refused in XML too.
//
// The rule here is the value rule alone: valid UTF-8, exactly one rune.
// A U+FFFD passes, because this method receives decoded text and has no
// literal to judge a substitution by. The utf8.ValidString check stays
// for a direct caller that hands over arbitrary bytes; encoding/xml
// itself never does, since it refuses invalid UTF-8 (a raw bad byte, and
// the CESU-8 spelling of a surrogate) and a character reference beyond
// U+10FFFF before consulting this method.
//
// One substitution channel does remain open through XML, and is accepted
// deliberately: encoding/xml converts a surrogate-half character
// reference (`&#xD800;`) with Go's rune-to-string rule, which yields
// U+FFFD, and reports no error, so it is indistinguishable here from a
// genuine U+FFFD. Refusing U+FFFD to close it would make a legal
// Character unrepresentable in XML. On the JSON side the encoding/json/v2
// tokenizer refuses a lone surrogate escape before this type sees the
// value, so that channel stays closed there.
//
// There is no numeric backward-compatibility arm here: older encoders
// wrote a number in JSON only, and XML element content has no number
// kind to accept.
//
// The error is deliberately outside typereg.ErrInvalidShape:
// that sentinel's own text names canonical JSON, and canxml classifies
// nothing at element level; it returns encoding/xml's errors unchanged
// and reserves canxml.ErrInvalidShape for xmi:type rejection and for a
// nil / non-xml.Marshaler root. A nil receiver is refused rather than
// dereferenced, on the same terms as [Character.UnmarshalJSON].
//
// encoding/json prefers [Character.UnmarshalJSON] over this method for
// every JSON value, so the JSON surface is unchanged by its presence.
func (c *Character) UnmarshalText(text []byte) error {
	if c == nil {
		return fmt.Errorf("rm.Character: %w", typereg.ErrNilReceiver)
	}
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
// empty or multi-character value is an encode error in XML, as it is in
// JSON, instead of producing an empty or over-long element. As on the JSON
// side, a U+FFFD value encodes normally.
//
// encoding/json prefers [Character.MarshalJSON] over this method, so the
// JSON surface is unchanged by its presence.
func (c Character) MarshalText() ([]byte, error) {
	if err := characterFault(string(c)); err != nil {
		return nil, fmt.Errorf("rm.Character: %w", err)
	}
	return []byte(c), nil
}
