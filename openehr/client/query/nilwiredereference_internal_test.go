package query

// nilwiredereference_internal_test.go — REQ-025 § No panics on the consumer
// half of the axis: mapQueryError extracts a *transport.WireError with
// errors.AsType and then reads StatusCode off it. errors.As / errors.AsType
// report ok=true for a boxed typed nil — the value a caller's own failed match
// leaves behind — so `ok` alone is not proof there is a struct to read. This
// test is internal because mapQueryError is unexported.

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestMapQueryErrorToleratesABoxedNilWireError pins the guard. Deleting
// `we == nil` from the early return makes this test panic.
func TestMapQueryErrorToleratesABoxedNilWireError(t *testing.T) {
	var typedNil *transport.WireError
	boxed := error(typedNil)

	we, ok := errors.AsType[*transport.WireError](boxed)
	if !ok {
		t.Fatal("premise gone: errors.AsType did not match a boxed typed-nil *transport.WireError")
	}
	if we != nil {
		t.Fatalf("premise gone: errors.AsType returned a non-nil *WireError (%v) for a boxed typed nil", we)
	}

	// No status to map, so the error passes through untouched rather than
	// being dressed up as an AQLError.
	if got := mapQueryError(boxed); !errors.Is(got, boxed) {
		t.Errorf("mapQueryError(boxed typed-nil WireError) = %#v, want the input error back unchanged", got)
	}

	// A real wire error still maps, so the guard did not swallow the signal.
	real400 := &transport.WireError{StatusCode: 400, Sentinel: transport.ErrUnprocessable}
	mapped := mapQueryError(real400)
	if _, ok := errors.AsType[*AQLError](mapped); !ok {
		t.Errorf("mapQueryError(400 WireError) = %#v, want an *AQLError", mapped)
	}
}
