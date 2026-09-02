package parse_test

// nilsyntaxerror_test.go — REQ-025 § No panics, on the nil-receiver axis of
// this package's exported pointer types. *SyntaxError is the one on it: Parse
// returns it as its typed failure, so errors.As / errors.AsType against it is
// the documented way to read a position out of a parse failure, and a failed
// match leaves the caller's variable nil. *Document is constructor-supplied
// (only Parse hands one out, and never nil alongside a nil error), and
// Position / Span are plain value types.

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// TestNilSyntaxErrorAnswersInsteadOfPanicking pins the two answers a nil
// *SyntaxError gives: a stable non-empty Error string (the zero value's own,
// so nil and zero cannot drift apart) and the same aql.ErrSyntax
// classification a real one carries. Removing the nil-receiver guard in Error
// MUST fail this test.
func TestNilSyntaxErrorAnswersInsteadOfPanicking(t *testing.T) {
	var e *parse.SyntaxError

	t.Run("Error", func(t *testing.T) {
		got := e.Error()
		if got == "" {
			t.Fatal(`nil.Error() = "", want a non-empty string — fmt would print nothing for this error`)
		}
		if want := (&parse.SyntaxError{}).Error(); got != want {
			t.Errorf("nil.Error() = %q, want %q (what the zero SyntaxError says)", got, want)
		}
	})

	t.Run("Unwrap", func(t *testing.T) {
		// Unwrap reads no field, so a nil receiver classifies exactly as a
		// real one does — there is nothing here that could start dereferencing
		// without this assertion noticing.
		if got := e.Unwrap(); !errors.Is(got, aql.ErrSyntax) {
			t.Errorf("nil.Unwrap() = %v, want aql.ErrSyntax", got)
		}
	})
}

// TestFailedErrorsAsTypeLeavesASyntaxErrorThatStillAnswers walks the exact
// route the defect is reachable by: a failed errors.AsType binds a nil
// pointer, and a caller that passes it onward boxes a typed nil in a non-nil
// error interface, so every errors package walk dispatches into a method on
// the nil receiver.
func TestFailedErrorsAsTypeLeavesASyntaxErrorThatStillAnswers(t *testing.T) {
	notASyntaxError := errors.New("aql: some other failure")

	e, ok := errors.AsType[*parse.SyntaxError](notASyntaxError)
	if ok {
		t.Fatalf("premise gone: errors.AsType matched %T as *parse.SyntaxError", notASyntaxError)
	}
	if e != nil {
		t.Fatalf("premise gone: a failed errors.AsType left e = %v, want it still nil", e)
	}

	// Boxing that nil in an error interface is what a caller does the moment
	// it passes the variable onward: the interface value is non-nil (it
	// carries the *SyntaxError type with a nil value), so a %v or an
	// errors.Is/As walk dispatches into a method on the nil receiver. There
	// is no runtime assertion for that here because it holds by
	// construction: staticcheck proves `boxed == nil` is never true (SA4023)
	// and rejects the check as dead code.
	boxed := error(e)

	if got := boxed.Error(); got == "" {
		t.Error(`Error() on a boxed typed-nil SyntaxError = "", want a non-empty string`)
	}
	if !errors.Is(boxed, aql.ErrSyntax) {
		t.Error("errors.Is(boxed typed-nil SyntaxError, aql.ErrSyntax) = false, want true — Unwrap is receiver-independent")
	}
}

// TestRealSyntaxErrorStillCarriesItsPosition guards the other direction: the
// nil guard must not have flattened the text a real parse failure reports.
func TestRealSyntaxErrorStillCarriesItsPosition(t *testing.T) {
	positioned := &parse.SyntaxError{Pos: parse.Position{Line: 3, Col: 14}, Msg: "unexpected token"}
	want := "aql: syntax error at 3:14: unexpected token"
	if got := positioned.Error(); got != want {
		t.Errorf("SyntaxError.Error() = %q, want %q", got, want)
	}
}

// TestSyntaxErrorOmitsPositionWhenZero pins REQ-025 § No panics' companion
// honesty rule: a SyntaxError whose Pos was never set (the zero value, as a
// nil receiver's Error delegates to) MUST NOT claim a fabricated "at 0:0:" —
// that reads as a real position no construction site ever reports. Zero Pos
// is the zero *SyntaxError's own shape, so this also pins what
// TestNilSyntaxErrorAnswersInsteadOfPanicking's nil.Error() actually says.
func TestSyntaxErrorOmitsPositionWhenZero(t *testing.T) {
	zeroPos := &parse.SyntaxError{Msg: "unexpected token"}
	want := "aql: syntax error: unexpected token"
	if got := zeroPos.Error(); got != want {
		t.Errorf("SyntaxError{Pos: zero}.Error() = %q, want %q (no dangling \"at 0:0:\")", got, want)
	}
}
