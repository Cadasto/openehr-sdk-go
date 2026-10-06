package constraints

import "fmt"

// redactedText is what a Redacted that holds a value prints as.
const redactedText = "[redacted]"

// Redacted carries one value taken from the input under validation, such as
// the argument a Validate method rejected, and keeps it out of logs, messages
// and JSON until a caller asks for it with [Redacted.Reveal].
//
// Printed with the fmt package, a Redacted that holds a value gives the text
// "[redacted]" for every verb, %v, %+v and %#v included, and one that holds
// no value gives nothing. Encoded with encoding/json, v1 or v2, it gives
// null. The text and JSON handlers of log/slog follow these two rules, so
// logging a Redacted never writes the value.
//
// The zero value holds no value; [Redact] builds one that does. Two Redacted
// values can be compared with == when the values they hold can be, and every
// value this package stores in one can be.
type Redacted struct {
	v any // the held value; nil when none is held
}

// Redact returns a [Redacted] holding v. Redact(nil) holds no value and
// equals the zero Redacted.
func Redact(v any) Redacted {
	return Redacted{v: v}
}

// Reveal returns the held value unchanged, or nil when none is held. It is
// the only way to read the value, so call it only where showing the value is
// safe, such as a form shown back to the person who filled it in.
func (r Redacted) Reveal() any {
	return r.v
}

// String returns "[redacted]" when r holds a value and "" when it holds none.
func (r Redacted) String() string {
	if r.v == nil {
		return ""
	}
	return redactedText
}

// Format writes the text of [Redacted.String] for every verb and ignores
// flags, width and precision, so no fmt verb prints the held value.
func (r Redacted) Format(f fmt.State, verb rune) {
	// A fmt.State writes into the caller's buffer and does not fail.
	_, _ = f.Write([]byte(r.String()))
}

// MarshalJSON encodes r as JSON null, whether or not it holds a value.
func (Redacted) MarshalJSON() ([]byte, error) {
	return []byte("null"), nil
}
