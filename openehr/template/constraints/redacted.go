package constraints

import (
	"fmt"
	"unique"
)

// redactedText is what a Redacted that holds a value prints as.
const redactedText = "[redacted]"

// Redacted carries one value taken from the input under validation, such as
// the argument a Validate method rejected, and keeps it out of logs, messages
// and encoded output. A Redacted that holds a value always prints as
// "[redacted]"; [Redacted.Reveal] returns the value itself, for the places
// where showing it is safe.
//
// Printed with the fmt package, a Redacted that holds a value gives the text
// "[redacted]" for every verb, %v, %+v and %#v included, and one that holds
// no value gives nothing. Where fmt prints it without calling its methods,
// as under %p or through an unexported field of a struct, it shows only
// memory addresses. Encoded with encoding/json, v1 or v2, it gives null;
// encoded with encoding/gob, it gives no bytes and decodes as the zero
// Redacted. The text and JSON handlers of log/slog follow these rules, so
// logging a Redacted never writes the value.
//
// The zero value holds no value; [Redact] builds one that does. Comparing two
// Redacted values with == never panics. Two Redacted values that hold equal
// values compare equal when those values can be compared, and every value
// this package stores can be. A value that cannot be compared, such as a
// slice or a map, is held by reference, so a Redacted holding one equals only
// its own copies.
type Redacted struct {
	// Exactly one field is set when a value is held, and neither when none
	// is. Both are pointers inside, so fmt, reading them without calling a
	// method, prints addresses and never the value.
	h   unique.Handle[any] // a value == can compare, interned
	box *any               // a value == cannot compare, such as a slice
}

// Redact returns a [Redacted] holding v. Redact(nil) holds no value and
// equals the zero Redacted.
func Redact(v any) Redacted {
	if v == nil {
		return Redacted{}
	}
	if h, ok := handleOf(v); ok {
		return Redacted{h: h}
	}
	return Redacted{box: &v}
}

// handleOf interns v, so equal values share one handle. unique.Make panics
// when v holds a value that cannot be hashed, such as a slice; handleOf
// recovers and reports false.
func handleOf(v any) (h unique.Handle[any], ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return unique.Make(v), true
}

// empty reports whether r holds no value.
func (r Redacted) empty() bool {
	return r.box == nil && r.h == unique.Handle[any]{}
}

// Reveal returns the held value, or nil when none is held. It is the only way
// to read the value, so call it only where showing the value is safe, such as
// a form shown back to the person who filled it in.
func (r Redacted) Reveal() any {
	switch {
	case r.box != nil:
		return *r.box
	case r.empty():
		return nil
	default:
		return r.h.Value()
	}
}

// String returns "[redacted]" when r holds a value and "" when it holds none.
func (r Redacted) String() string {
	if r.empty() {
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

// MarshalJSON encodes the Redacted as JSON null, whether or not it holds a
// value.
func (Redacted) MarshalJSON() ([]byte, error) {
	return []byte("null"), nil
}

// GobEncode encodes the Redacted as no bytes, whether or not it holds a value,
// so encoding/gob drops the value as encoding/json does.
func (Redacted) GobEncode() ([]byte, error) {
	return []byte{}, nil
}

// GobDecode sets r to the zero Redacted, whatever the bytes hold: a Redacted
// read back from gob holds no value.
func (r *Redacted) GobDecode([]byte) error {
	*r = Redacted{}
	return nil
}
