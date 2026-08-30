package transport_test

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestNilWireErrorAnswersInsteadOfPanicking pins REQ-025 § No panics on
// *WireError: a typed nil (the value a failed errors.As / errors.AsType
// leaves behind) answers Error and Unwrap instead of dereferencing.
// Removing either nil-receiver guard MUST fail this test.
func TestNilWireErrorAnswersInsteadOfPanicking(t *testing.T) {
	var e *transport.WireError

	t.Run("Error", func(t *testing.T) {
		got := e.Error()
		if got == "" {
			t.Fatal(`nil.Error() = "", want a non-empty string`)
		}
		if want := (&transport.WireError{}).Error(); got != want {
			t.Errorf("nil.Error() = %q, want %q (what the zero WireError says)", got, want)
		}
	})

	t.Run("Unwrap", func(t *testing.T) {
		if got := e.Unwrap(); got != nil {
			t.Errorf("nil.Unwrap() = %v, want nil", got)
		}
	})
}

// TestFailedErrorsAsLeavesAWireErrorThatStillAnswers walks the
// errors.As out-parameter shape: a failed match leaves the variable
// nil, boxing it as error still answers rather than panicking.
func TestFailedErrorsAsLeavesAWireErrorThatStillAnswers(t *testing.T) {
	notAWireError := errors.New("transport: some other failure")

	var e *transport.WireError
	if errors.As(notAWireError, &e) {
		t.Fatalf("premise gone: errors.As matched %T as *transport.WireError", notAWireError)
	}
	if e != nil {
		t.Fatalf("premise gone: a failed errors.As left e = %v, want it still nil", e)
	}

	var boxed error = e
	if boxed == nil {
		t.Fatal("premise gone: boxing a typed-nil *WireError produced a nil error interface")
	}
	got := boxed.Error()
	if got == "" {
		t.Fatal(`boxed typed-nil Error() = "", want a non-empty string`)
	}
	if unwrapped := errors.Unwrap(boxed); unwrapped != nil {
		t.Errorf("boxed typed-nil Unwrap() = %v, want nil", unwrapped)
	}
}
