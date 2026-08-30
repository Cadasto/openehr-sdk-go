package discovery_test

// nildiscoveryerror_test.go — REQ-025 § No panics, on the nil-receiver axis of
// this package's exported pointer types. *DiscoveryError is the one that
// matters: its godoc names errors.As as the way to reach it, and a failed
// match leaves the caller's variable nil. The other exported pointer types are
// constructor-supplied — *Resolver, *MemoryCache, *ServiceCatalog — so no
// documented extraction shape hands a consumer a typed nil of them;
// *ServiceCatalog is nil-safe anyway (Service and Stale both guard).

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// TestNilDiscoveryErrorAnswersInsteadOfPanicking pins the two answers a nil
// *DiscoveryError gives: a stable non-empty Error string (the zero value's
// own, so nil and zero cannot drift apart) and nothing to unwrap. Removing
// either nil-receiver guard MUST fail this test.
func TestNilDiscoveryErrorAnswersInsteadOfPanicking(t *testing.T) {
	var e *discovery.DiscoveryError

	t.Run("Error", func(t *testing.T) {
		got := e.Error()
		if got == "" {
			t.Fatal(`nil.Error() = "", want a non-empty string — fmt would print nothing for this error`)
		}
		if want := (&discovery.DiscoveryError{}).Error(); got != want {
			t.Errorf("nil.Error() = %q, want %q (what the zero DiscoveryError says)", got, want)
		}
	})

	t.Run("Unwrap", func(t *testing.T) {
		if got := e.Unwrap(); got != nil {
			t.Errorf("nil.Unwrap() = %v, want nil — a nil DiscoveryError wraps nothing", got)
		}
	})
}

// TestFailedErrorsAsLeavesADiscoveryErrorThatStillAnswers walks the route the
// defect is reachable by: the errors.As out-parameter shape leaves the
// variable nil on a failed match, and a caller that passes it onward boxes a
// typed nil in a non-nil error interface, so every errors package walk
// dispatches into a method on the nil receiver.
func TestFailedErrorsAsLeavesADiscoveryErrorThatStillAnswers(t *testing.T) {
	notADiscoveryError := errors.New("discovery: some other failure")

	var e *discovery.DiscoveryError
	if errors.As(notADiscoveryError, &e) {
		t.Fatalf("premise gone: errors.As matched %T as *discovery.DiscoveryError", notADiscoveryError)
	}
	if e != nil {
		t.Fatalf("premise gone: a failed errors.As left e = %v, want it still nil", e)
	}

	// Boxing that nil in an error interface is what a caller does the moment
	// it passes the variable onward: the interface value is non-nil (it
	// carries the *DiscoveryError type with a nil value), so a %v or an
	// errors.Is/As walk dispatches into a method on the nil receiver. There
	// is no runtime assertion for that here because it holds by
	// construction: staticcheck proves `boxed == nil` is never true (SA4023)
	// and rejects the check as dead code.
	boxed := error(e)

	if got := boxed.Error(); got == "" {
		t.Error(`Error() on a boxed typed-nil DiscoveryError = "", want a non-empty string`)
	}
	if got := errors.Unwrap(boxed); got != nil {
		t.Errorf("errors.Unwrap(boxed typed-nil DiscoveryError) = %v, want nil", got)
	}
	if errors.Is(boxed, errors.ErrUnsupported) {
		t.Error("errors.Is(boxed typed-nil DiscoveryError, errors.ErrUnsupported) = true, want false")
	}
}
