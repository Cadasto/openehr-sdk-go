package transport

// nilwiredereference_internal_test.go — REQ-025 § No panics on the *consumer*
// half of the nil-receiver axis: the unexported helpers in this package that
// extract a *WireError with errors.AsType and then read a field off it.
// errors.As / errors.AsType report ok=true for a boxed typed nil (the value a
// caller's own failed match leaves behind and passes onward), so `ok` alone is
// not proof there is a struct to read. These live in the internal test package
// because the helpers are unexported.

import (
	"errors"
	"net/http"
	"testing"
)

// boxedNilWireError is what a consumer hands back to the SDK after its own
// errors.As against *WireError failed: a non-nil error interface carrying a
// nil *WireError.
func boxedNilWireError() error {
	var e *WireError
	return error(e)
}

// TestIsWire401ToleratesABoxedNilWireError pins the guard at the isWire401
// extraction site. Deleting `we != nil` there makes this test panic.
func TestIsWire401ToleratesABoxedNilWireError(t *testing.T) {
	boxed := boxedNilWireError()

	we, ok := errors.AsType[*WireError](boxed)
	if !ok {
		t.Fatalf("premise gone: errors.AsType did not match a boxed typed-nil *WireError")
	}
	if we != nil {
		t.Fatalf("premise gone: errors.AsType returned a non-nil *WireError (%v) for a boxed typed nil", we)
	}

	if got := isWire401(boxed); got {
		t.Errorf("isWire401(boxed typed-nil WireError) = true, want false — there is no status to read")
	}
	// The real 401 still classifies, so the guard did not swallow the signal.
	real401 := &WireError{StatusCode: 401, Sentinel: ErrUnauthorized}
	if got := isWire401(real401); !got {
		t.Errorf("isWire401(%v) = false, want true", real401)
	}
}

// TestShouldRetryToleratesABoxedNilWireError pins the guard at the shouldRetry
// extraction site. Deleting `we != nil` there makes this test panic.
func TestShouldRetryToleratesABoxedNilWireError(t *testing.T) {
	c := &Client{cfg: config{retry: RetryPolicy{MaxAttempts: 3}}}
	req := &Request{Method: http.MethodGet, Path: "/"}

	// A boxed typed nil carries no status. It falls through to the generic
	// "network / transient" arm, which retries an idempotent method — the same
	// answer any other unclassifiable error gets.
	if got := c.shouldRetry(req, nil, boxedNilWireError(), 0); !got {
		t.Errorf("shouldRetry(boxed typed-nil WireError) = false, want true — an unclassifiable error retries like any other transient failure")
	}
	// A real *WireError still routes through the status gate.
	if got := c.shouldRetry(req, nil, &WireError{StatusCode: 503, Sentinel: ErrServerError}, 0); !got {
		t.Errorf("shouldRetry(503 WireError) = false, want true")
	}
	if got := c.shouldRetry(req, nil, &WireError{StatusCode: 404, Sentinel: ErrNotFound}, 0); got {
		t.Errorf("shouldRetry(404 WireError) = true, want false")
	}
}
