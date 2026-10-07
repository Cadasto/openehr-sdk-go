package constraints

import (
	"fmt"
	"unique"
)

// redactedText is what a Redacted that holds a value prints as.
const redactedText = "[redacted]"

// Redacted carries one value taken from the input under validation, such as
// the argument a Validate method rejected, and keeps it out of logs, messages
// and encoded output. [Redacted.Reveal] returns the value itself, for the
// places where showing it is safe.
//
// Printed with the fmt package, a Redacted that holds a value gives the text
// "[redacted]" for every verb other than %T and %p, %v, %+v and %#v included,
// and one that holds no value gives nothing. %T gives the type name. Under %p,
// and through an unexported field of a struct, fmt prints a Redacted without
// calling its methods: it shows memory addresses and never the value, but
// equal held values show the same address, so two such lines reveal that
// their values are equal. Encoded with encoding/json, v1 or v2, it gives
// null; encoded with encoding/gob, it gives no bytes and decodes as the zero
// Redacted. The text and JSON handlers of log/slog follow these rules, so
// logging a Redacted never writes the value. These rules cover fmt, JSON, gob
// and log/slog. A tool that reads unexported fields by reflection, such as a
// diff reporter or a debugger, can see the value.
//
// The zero value holds no value; [Redact] builds one that does. Comparing two
// Redacted values with == never panics. Two Redacted values compare equal when
// neither holds a value, or when they hold values that are equal under ==,
// and every value this package stores can be compared that way. A value that
// == cannot compare, such as a slice or a map, is held by reference, so a
// Redacted holding one equals only its own copies. [Redacted.Equal] gives the
// same answer as ==, for comparison libraries that skip unexported fields but
// call an Equal method.
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
//
// The value comes back with its type. A value that == can compare comes back
// equal under == to the one given to [Redact], with two caveats. A
// floating-point zero may come back with either sign: equal values share one
// handle, so the sign depends on which zero was interned first, and == finds
// the two signs equal. A NaN, alone or inside an array or a struct, comes
// back as a NaN in the same place; == never finds a NaN equal, not even to
// itself, so a value that holds one never compares equal to the one given.
// A value that == cannot compare, such as a slice or a map, comes back as
// the same value, not a copy.
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

// Equal reports whether r and o hold equal values, exactly as r == o does.
// It exists for comparison libraries that refuse to read unexported fields
// but call a type's Equal method instead, so they can still compare a
// [Violation] or any other value that holds a Redacted, without seeing the
// held value.
func (r Redacted) Equal(o Redacted) bool {
	return r == o
}

// String returns "[redacted]" when r holds a value and "" when it holds none.
func (r Redacted) String() string {
	if r.empty() {
		return ""
	}
	return redactedText
}

// Format writes the text of [Redacted.String] for every verb fmt passes to it,
// and ignores flags, width and precision, so no fmt verb prints the held
// value. fmt answers %T and %p itself, without calling Format; [Redacted]
// says what those print.
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
