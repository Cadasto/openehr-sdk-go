// Package refused carries the JSON text of a value one of the SDK's
// hand-written primitive codecs refused, from that codec to the canonical-JSON
// decode entry point that reports the failure.
//
// A primitive such as rm.Real decodes through an UnmarshalJSON method, and
// encoding/json/v2 records no literal for an error that method returns. The
// codec holds the text, so it wraps its refusal with [Literal]; the entry
// point finds the wrapper by its type and puts the text in the value-bearing
// field of the error it returns. The wrapper adds nothing a caller reads: its
// message is the refusal's own, and one errors.Unwrap step reaches the
// refusal, so every sentinel and typed cause beneath it still matches.
//
// The package imports only the standard library. The type registry, whose
// error text skips the wrapper, and the RM primitives, which create it, both
// import it, and neither imports the other through it.
package refused

// Literal returns err, a codec's refusal of the JSON text literal, carrying
// that text as the document wrote it. A nil err returns nil, so a success
// cannot become an error.
//
// literal is copied, so a codec can pass the bytes encoding/json/v2 lends its
// UnmarshalJSON method.
func Literal(literal []byte, err error) error {
	if err == nil {
		return nil
	}
	return &Error{text: string(literal), err: err}
}

// Error is the error [Literal] returns: a codec's refusal and the JSON text it
// refused.
type Error struct {
	text string
	err  error
}

// Text returns the JSON text the codec refused, as the document wrote it. A
// nil receiver returns "".
func (e *Error) Text() string {
	if e == nil {
		return ""
	}
	return e.text
}

// Error returns the refusal's own message, unchanged. A nil receiver answers
// with a fixed text rather than dereferencing.
func (e *Error) Error() string {
	if e == nil || e.err == nil {
		return "unspecified error"
	}
	return e.err.Error()
}

// Unwrap returns the refusal. A nil receiver unwraps to nil.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}
