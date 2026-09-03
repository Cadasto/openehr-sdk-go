package rm

import (
	"bytes"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
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
//
// The BASE Character primitive excludes no code point, so U+FFFD (the Unicode
// replacement character) is itself a legal Character and survives every codec.
// What is refused is a *substituted* U+FFFD — one encoding/json manufactured
// from a lone UTF-16 surrogate escape — which only the JSON string arm can
// detect, because only it still holds the wire bytes; see
// jsonLiteralSpellsReplacement.
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
// with typereg.ErrInvalidShape (see classifyShape).
//
// Every code point passes, U+FFFD included: the BASE Character
// primitive excludes none, so refusing one here would make a legal
// character unrepresentable in JSON, text and XML alike. The narrower
// question — whether a decoded U+FFFD was on the wire or was
// substituted for corrupted input — needs the wire bytes, which this
// function does not have; it is answered by
// jsonLiteralSpellsReplacement, on the one arm that does.
func characterFault(s string) error {
	if !utf8.ValidString(s) {
		return errors.New("value is not valid UTF-8")
	}
	if count := utf8.RuneCountInString(s); count != 1 {
		return fmt.Errorf("must be exactly one character, got %d", count)
	}
	return nil
}

// jsonLiteralSpellsReplacement reports whether the raw JSON string
// literal b — quotes included — spells U+FFFD itself, rather than
// having had it substituted in. It is only meaningful for a literal that
// decoded to a single U+FFFD, and its precondition is the caller's: b
// begins with a quote and json.Unmarshal has already accepted it, so it
// is a complete string literal.
//
// encoding/json performs the substitution silently. A lone UTF-16
// surrogate escape ("\uD800", "\uDC00") and an invalid UTF-8 byte both
// decode to U+FFFD with no error reported, so the decoded rune alone
// cannot say whether a producer wrote a replacement character or whether
// one was manufactured out of corrupted input — and a rune count would
// launder the corrupted case into an apparently valid Character.
//
// The literal can say. U+FFFD is genuine exactly when the bytes between
// the quotes are its own UTF-8 encoding (0xEF 0xBF 0xBD) or its
// six-byte escape (`�`, hex digits in any case); any other literal
// that decodes to one U+FFFD had it substituted. A well-formed surrogate
// PAIR is not affected — it decodes to the astral character it names, not
// to U+FFFD, so it never reaches this question.
func jsonLiteralSpellsReplacement(b []byte) bool {
	// json.Unmarshal tolerates trailing whitespace after the closing quote,
	// and a direct caller may pass it; encoding/json itself never does.
	b = bytes.TrimRight(b, " \t\r\n")
	if len(b) < 2 {
		return false
	}
	raw := b[1 : len(b)-1] // between the quotes
	if len(raw) == 3 && raw[0] == 0xEF && raw[1] == 0xBF && raw[2] == 0xBD {
		return true // U+FFFD written as itself
	}
	if len(raw) != 6 || raw[0] != '\\' || raw[1] != 'u' {
		return false
	}
	// JSON hex digits are ASCII, so a case-insensitive compare is all the
	// "hex in any case" allowance needs.
	return strings.EqualFold(string(raw[2:]), "fffd")
}

// UnmarshalJSON accepts a canonical one-character JSON string. For backward
// compatibility with the pre-fix encoder — which wrote match as a number — a
// JSON number is also accepted on decode, but it is never the encoded form
// (docs/specifications/wire.md § REQ-052's `TERM_MAPPING.match` bullet). A
// JSON null is a no-op per the encoding/json convention for Unmarshaler
// ("approximate the behavior of Unmarshal itself"), leaving the receiver
// unchanged rather than writing the zero value.
//
// The string arm runs its checks in this order: the raw bytes must be
// valid UTF-8; the literal must decode; the decoded value must be one
// rune (characterFault); and a decoded U+FFFD must be spelled as itself
// on the wire (jsonLiteralSpellsReplacement) rather than substituted for
// a lone UTF-16 surrogate escape. A genuine U+FFFD therefore decodes,
// which is right — the BASE primitive admits it — while the corrupted
// spelling that decodes to the same rune does not.
//
// Every refusal on this decode path carries typereg.ErrInvalidShape, so a
// bare-Character decode classifies the way the generated TERM_MAPPING funnel
// would (REQ-052 § Decode-side shape sentinel). The classification rides on
// errors.Is and leaves the message and the cause untouched (see
// classifyShape), so encoding/json's own error stays both readable and
// reachable with errors.AsType. The encode direction deliberately stays
// outside that sentinel — canjson attaches its own encode-only
// ErrInvalidValue there.
//
// A nil receiver is refused rather than dereferenced (REQ-025, idiom.md
// § No panics): the method assigns through the pointer, and a nil
// pointer is caller-constructible input reachable through the documented
// API. That refusal is a plain error — caller misuse is not a wire-shape
// problem — so it stays outside typereg.ErrInvalidShape.
func (c *Character) UnmarshalJSON(b []byte) error {
	if c == nil {
		return errors.New("rm.Character: nil receiver")
	}
	if len(b) == 0 {
		return errors.New("rm.Character: empty input")
	}
	if string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		// The UTF-8 check goes first, on the raw bytes: json.Unmarshal
		// replaces an invalid byte with U+FFFD and reports nothing, so
		// after it there is no invalid input left to see.
		if !utf8.Valid(b) {
			return classifyShape(errors.New("rm.Character: input is not valid UTF-8"))
		}
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return classifyShape(fmt.Errorf("rm.Character: %w", err))
		}
		if err := characterFault(s); err != nil {
			return classifyShape(fmt.Errorf("rm.Character: %w", err))
		}
		if r, _ := utf8.DecodeRuneInString(s); r == utf8.RuneError && !jsonLiteralSpellsReplacement(b) {
			return classifyShape(errors.New("rm.Character: a lone UTF-16 surrogate escape was substituted"))
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
		return classifyShape(fmt.Errorf("rm.Character: %w", err))
	}
	if n == 0 || !utf8.ValidRune(n) {
		return classifyShape(errors.New("rm.Character: number is not a usable code point"))
	}
	// Past that gate string(n) is one valid UTF-8 rune by construction, so
	// characterFault has nothing left to add. 65533 is accepted here: a
	// number names a code point and carries no bytes that could have been
	// substituted, so U+FFFD spelled numerically is always genuine.
	*c = Character(string(n))
	return nil
}

// MarshalJSON emits a one-character JSON string. A value that is not a single
// valid UTF-8 character — empty, multi-rune, or malformed bytes — is an encode
// error (never silently coerced, and never emitted as "�"), which canjson
// wraps as ErrInvalidValue (§ REQ-052's `TERM_MAPPING.match` bullet). A U+FFFD
// value encodes normally: it is a legal Character, and on this side there are
// no wire bytes that could have been substituted. Diagnostics are value-free
// (REQ-093).
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
// through XML unvalidated (REQ-046 / REQ-052 / REQ-056).
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
// U+FFFD, and reports no error — so it is indistinguishable here from a
// genuine U+FFFD. Refusing U+FFFD to close it would make a legal
// Character unrepresentable in XML, which is the defect this rule
// exists to avoid; the JSON string arm, which does hold the literal,
// still refuses its own surrogate escapes.
// TestCharacterXMLElementContentValidated pins both halves.
//
// There is no numeric back-compat arm here: the pre-fix encoder wrote a
// number in JSON only, and XML element content carries no JSON number
// kind to be tolerant of.
//
// The error is a plain error, deliberately outside typereg.ErrInvalidShape:
// that sentinel's own text names canonical JSON, and canxml classifies
// nothing at element level — it returns encoding/xml's errors unchanged
// and reserves canxml.ErrInvalidShape for xmi:type rejection and for a
// nil / non-xml.Marshaler root. A nil receiver is refused rather than
// dereferenced, on the same terms as [Character.UnmarshalJSON] (REQ-025).
//
// encoding/json prefers [Character.UnmarshalJSON] over this method for
// every JSON value, so the JSON surface is unchanged by its presence.
func (c *Character) UnmarshalText(text []byte) error {
	if c == nil {
		return errors.New("rm.Character: nil receiver")
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
// empty or multi-character value is an encode error in XML exactly as it is
// in JSON rather than an empty or over-long element (REQ-052 / REQ-056). As
// on the JSON side, a U+FFFD value encodes normally.
//
// encoding/json prefers [Character.MarshalJSON] over this method, so the
// JSON surface is unchanged by its presence.
func (c Character) MarshalText() ([]byte, error) {
	if err := characterFault(string(c)); err != nil {
		return nil, fmt.Errorf("rm.Character: %w", err)
	}
	return []byte(c), nil
}
