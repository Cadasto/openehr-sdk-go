package system_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/system"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestCapabilitiesNullBodyIsInvalidShape pins § REQ-151's refusal arm on the
// hand-rolled System read: a 200 whose body is the JSON `null` literal carries
// no representation and MUST classify as transport.ErrInvalidShape — not as a
// *transport.DecodeError, and not as an all-zero ServiceCapabilities, which is
// what encoding/json would silently produce for `null`. Reverting the leaf to a
// len(body) == 0 check fails this test with a nil error.
func TestCapabilitiesNullBodyIsInvalidShape(t *testing.T) { // REQ-151
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "null")
	}))
	defer srv.Close()

	caps, _, err := system.Capabilities(t.Context(), newClient(t, srv))
	if caps != nil {
		t.Fatalf("Capabilities(null body) = %+v, want nil", caps)
	}
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Fatalf("Capabilities(null body) err = %v, want errors.Is(err, transport.ErrInvalidShape)", err)
	}
	if _, ok := errors.AsType[*transport.DecodeError](err); ok {
		t.Fatalf("Capabilities(null body) err = %v (%T); the no-representation arm is not a DecodeError", err, err)
	}
}
