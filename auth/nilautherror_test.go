package auth_test

// nilautherror_test.go — REQ-025 § No panics, on the nil-receiver axis of this
// package's exported pointer types. Two sit on it, and both because their own
// godoc names the extraction shape that produces the typed nil:
// *ExchangeError (errors.AsType[*auth.ExchangeError]) and *OAuth2Error
// (errors.AsType[*auth.OAuth2Error], plus ParseOAuth2Error, which returns nil
// for a body that is not an RFC 6749 §5.2 envelope). The rest of the exported
// surface is off-axis: ReautherFunc is a func type, TokenSource and Reauther
// are interfaces — neither kind has a nil receiver of its own — and Token is a
// plain struct with no methods.

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// TestNilExchangeErrorAnswersInsteadOfPanicking pins the three answers a nil
// *ExchangeError gives: a stable non-empty Error string (the zero value's own,
// so nil and zero cannot drift apart), nothing to unwrap, and a non-terminal
// verdict. Removing a nil-receiver guard MUST fail this test.
func TestNilExchangeErrorAnswersInsteadOfPanicking(t *testing.T) {
	var e *auth.ExchangeError

	t.Run("Error", func(t *testing.T) {
		got := e.Error()
		if got == "" {
			t.Fatal(`nil.Error() = "", want a non-empty string — fmt would print nothing for this error`)
		}
		if want := (&auth.ExchangeError{}).Error(); got != want {
			t.Errorf("nil.Error() = %q, want %q (what the zero ExchangeError says)", got, want)
		}
	})

	t.Run("Unwrap", func(t *testing.T) {
		if got := e.Unwrap(); got != nil {
			t.Errorf("nil.Unwrap() = %v, want nil — a nil ExchangeError wraps nothing", got)
		}
	})

	t.Run("Terminal", func(t *testing.T) {
		if e.Terminal() {
			t.Error("nil.Terminal() = true, want false — a nil ExchangeError is not a permanent rejection")
		}
	})
}

// TestZeroExchangeErrorAnswersInsteadOfPanicking is the sibling the nil guard
// depends on: Error reads Sentinel, which a caller-built zero value leaves
// nil, so delegating the nil receiver to the zero value only helps if the zero
// value itself answers.
func TestZeroExchangeErrorAnswersInsteadOfPanicking(t *testing.T) {
	zero := &auth.ExchangeError{}
	if got := zero.Error(); got == "" {
		t.Error(`(&auth.ExchangeError{}).Error() = "", want a non-empty string`)
	}
	if got := zero.Unwrap(); len(got) != 0 {
		t.Errorf("(&auth.ExchangeError{}).Unwrap() = %v, want nothing to unwrap", got)
	}
}

// TestNilOAuth2ErrorAnswersInsteadOfPanicking pins the one answer a nil
// *OAuth2Error gives. ParseOAuth2Error returns nil for an unparseable body, so
// this typed nil reaches consumers without any errors.As involved at all.
func TestNilOAuth2ErrorAnswersInsteadOfPanicking(t *testing.T) {
	var e *auth.OAuth2Error

	got := e.Error()
	if got == "" {
		t.Fatal(`nil.Error() = "", want a non-empty string — fmt would print nothing for this error`)
	}
	if want := (&auth.OAuth2Error{}).Error(); got != want {
		t.Errorf("nil.Error() = %q, want %q (what the zero OAuth2Error says)", got, want)
	}
}

// TestFailedErrorsAsTypeLeavesAnExchangeErrorThatStillAnswers walks the exact
// route the godoc documents: a failed errors.AsType binds a nil pointer, and a
// caller that passes it onward boxes a typed nil in a non-nil error interface,
// so every errors package walk dispatches into a method on the nil receiver.
func TestFailedErrorsAsTypeLeavesAnExchangeErrorThatStillAnswers(t *testing.T) {
	notAnExchangeError := errors.New("auth: some other failure")

	e, ok := errors.AsType[*auth.ExchangeError](notAnExchangeError)
	if ok {
		t.Fatalf("premise gone: errors.AsType matched %T as *auth.ExchangeError", notAnExchangeError)
	}
	if e != nil {
		t.Fatalf("premise gone: a failed errors.AsType left e = %v, want it still nil", e)
	}

	// Boxing that nil in an error interface is what a caller does the moment
	// it passes the variable onward: the interface value is non-nil (it
	// carries the *ExchangeError type with a nil value), so a %v or an
	// errors.Is/As walk dispatches into a method on the nil receiver. There
	// is no runtime assertion for that here because it holds by
	// construction: staticcheck proves `boxed == nil` is never true (SA4023)
	// and rejects the check as dead code.
	boxed := error(e)

	if got := boxed.Error(); got == "" {
		t.Error(`Error() on a boxed typed-nil ExchangeError = "", want a non-empty string`)
	}
	// errors.Is walks the multi-error Unwrap on the nil receiver.
	if errors.Is(boxed, auth.ErrTokenExchangeFailed) {
		t.Error("errors.Is(boxed typed-nil ExchangeError, auth.ErrTokenExchangeFailed) = true, want false")
	}
	if errors.Is(boxed, auth.ErrReauthRequired) {
		t.Error("errors.Is(boxed typed-nil ExchangeError, auth.ErrReauthRequired) = true, want false")
	}
	// And errors.AsType walks it looking for the nested envelope.
	if inner, ok := errors.AsType[*auth.OAuth2Error](boxed); ok {
		t.Errorf("errors.AsType[*auth.OAuth2Error](boxed typed-nil ExchangeError) matched %v, want no match", inner)
	}
}
