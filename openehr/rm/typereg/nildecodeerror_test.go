package typereg_test

// nildecodeerror_test.go — REQ-025 § No panics, on the nil-receiver axis of
// this package's exported pointer types. *DecodeError is the high-traffic one:
// the canjson and canxml codecs re-export it as canjson.DecodeError and
// canxml.DecodeError, and their godoc names errors.As / errors.AsType as the
// way to reach it, so a failed match on a decode error leaves a typed nil in
// consumer hands on the SDK's most-travelled read path. Registry is the other
// exported pointer type and is off this axis: a consumer only ever receives
// one from NewRegistry or the package-level Default, and no documented
// extraction shape can leave a nil one in a caller's variable.

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// TestNilDecodeErrorAnswersInsteadOfPanicking pins the two answers a nil
// *DecodeError gives: a stable non-empty Error string (the zero value's own,
// so nil and zero cannot drift apart) and nothing to unwrap. Removing either
// nil-receiver guard MUST fail this test.
func TestNilDecodeErrorAnswersInsteadOfPanicking(t *testing.T) {
	var e *typereg.DecodeError

	t.Run("Error", func(t *testing.T) {
		got := e.Error()
		if got == "" {
			t.Fatal(`nil.Error() = "", want a non-empty string — fmt would print nothing for this error`)
		}
		if want := (&typereg.DecodeError{}).Error(); got != want {
			t.Errorf("nil.Error() = %q, want %q (what the zero DecodeError says)", got, want)
		}
	})

	t.Run("Unwrap", func(t *testing.T) {
		if got := e.Unwrap(); got != nil {
			t.Errorf("nil.Unwrap() = %v, want nil — a nil DecodeError wraps nothing", got)
		}
	})
}

// TestFailedErrorsAsTypeLeavesADecodeErrorThatStillAnswers walks the exact
// route the codecs document: errors.AsType against *DecodeError, whose failed
// match binds a nil pointer just as the errors.As out-parameter form does. A
// caller that passes that value onward boxes a typed nil in a non-nil error
// interface, so every errors package walk dispatches into a method on the nil
// receiver.
func TestFailedErrorsAsTypeLeavesADecodeErrorThatStillAnswers(t *testing.T) {
	notADecodeError := errors.New("typereg: some other failure")

	e, ok := errors.AsType[*typereg.DecodeError](notADecodeError)
	if ok {
		t.Fatalf("premise gone: errors.AsType matched %T as *typereg.DecodeError", notADecodeError)
	}
	if e != nil {
		t.Fatalf("premise gone: a failed errors.AsType left e = %v, want it still nil", e)
	}

	// Boxing that nil in an error interface is what a caller does the moment
	// it passes the variable onward: the interface value is non-nil (it
	// carries the *DecodeError type with a nil value), so a %v or an
	// errors.Is/As walk dispatches into a method on the nil receiver. There
	// is no runtime assertion for that here because it holds by
	// construction: staticcheck proves `boxed == nil` is never true (SA4023)
	// and rejects the check as dead code.
	boxed := error(e)

	if got := boxed.Error(); got == "" {
		t.Error(`Error() on a boxed typed-nil DecodeError = "", want a non-empty string`)
	}
	if got := errors.Unwrap(boxed); got != nil {
		t.Errorf("errors.Unwrap(boxed typed-nil DecodeError) = %v, want nil", got)
	}
	// The classification entry the sentinels document walks Unwrap on the nil
	// receiver.
	if errors.Is(boxed, typereg.ErrMissingType) {
		t.Error("errors.Is(boxed typed-nil DecodeError, typereg.ErrMissingType) = true, want false")
	}
	if errors.Is(boxed, typereg.ErrUnknownType) {
		t.Error("errors.Is(boxed typed-nil DecodeError, typereg.ErrUnknownType) = true, want false")
	}
}

// unguardedNilError has no nil-receiver guard of its own — deliberately, to
// prove the DecodeError cause-rendering fix (REQ-025 nil-receiver axis)
// defends the GENERAL case (any Inner error, not merely this SDK's own
// already-guarded types such as *parse.SyntaxError).
type unguardedNilError struct{ msg string }

func (e *unguardedNilError) Error() string { return e.msg }

// TestDecodeErrorToleratesAnUnguardedNilInner pins the cause-rendering guard:
// a non-nil *DecodeError whose Inner is a boxed typed-nil error (a bare
// e.Inner.Error() call dereferences it and panics) MUST still answer Error()
// rather than take the caller down with it.
func TestDecodeErrorToleratesAnUnguardedNilInner(t *testing.T) {
	var inner *unguardedNilError
	e := &typereg.DecodeError{Path: "/composition/content", Type: "OBSERVATION", Inner: inner}

	got := e.Error() // panics here today = the finding
	if got == "" {
		t.Fatal(`Error() with a boxed typed-nil Inner = "", want a non-empty string`)
	}
	if !strings.Contains(got, "/composition/content") || !strings.Contains(got, "OBSERVATION") {
		t.Errorf("Error() = %q, want it to still name Path and Type despite the unreadable Inner", got)
	}

	// A withCause Inner still renders its own message, so the guard did not
	// swallow the signal.
	withCause := &typereg.DecodeError{Path: "/composition/content", Type: "OBSERVATION", Inner: errors.New("boom")}
	if got, want := withCause.Error(), "decode /composition/content (_type=\"OBSERVATION\"): boom"; got != want {
		t.Errorf("Error() with a withCause Inner = %q, want %q", got, want)
	}
}
