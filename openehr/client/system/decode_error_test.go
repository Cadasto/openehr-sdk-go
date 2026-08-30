package system_test

// decode_error_test.go — REQ-151 § Typed 2xx decode failure, held against this
// package's single hand-rolled 2xx response decode: the ServiceCapabilities
// body in Capabilities.
//
// The neighbouring empty-body arm is a keyed exclusion and stays
// transport.ErrInvalidShape — there is no representation to decode and no
// bytes to hand back — so the guard below serves a non-empty body that cannot
// decode, which is what this requirement is about.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/system"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// capabilitiesArrayBody is a JSON array served where the single
// ServiceCapabilities object is expected. It is chosen because it is
// guaranteed to fail: an object with merely unexpected keys decodes cleanly,
// since unknown fields are ignored and nothing validates that a required
// field arrived.
const capabilitiesArrayBody = `[{"solution":"cadasto","rest_api_specs_version":"1.1.0"}]`

// TestCapabilitiesDecodeFailureIsTyped is REQ-151's positive contract at the
// System leaf: a 2xx body that cannot be decoded as ServiceCapabilities
// surfaces as a *transport.DecodeError, recoverable with errors.AsType through
// the leaf's own operation-name wrap, carrying the bytes the server delivered
// and attributing the request by method and route template.
func TestCapabilitiesDecodeFailureIsTyped(t *testing.T) { // REQ-151
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"caps-1"`)
		_, _ = w.Write([]byte(capabilitiesArrayBody))
	}))
	defer srv.Close()

	caps, meta, err := system.Capabilities(t.Context(), newClient(t, srv))
	if err == nil {
		t.Fatalf("Capabilities decoded %s into a ServiceCapabilities without error; the premise of this test is gone", capabilitiesArrayBody)
	}
	if caps != nil {
		t.Errorf("Capabilities returned %+v beside the decode error; a failed decode has nothing to hand back", caps)
	}
	// REQ-151 § Metadata still arrives: a decode failure does not cost the
	// caller the response headers.
	if meta == nil {
		t.Fatal("Capabilities returned nil *transport.Metadata beside the decode error; REQ-151 § Metadata still arrives requires the response headers survive")
	}
	if got := meta.ETag; got != "caps-1" {
		t.Errorf("Metadata.ETag = %q, want %q — the headers must survive the decode failure intact", got, "caps-1")
	}

	de, ok := errors.AsType[*transport.DecodeError](err)
	if !ok {
		t.Fatalf("errors.AsType[*transport.DecodeError] did not match %T (%v); REQ-151 requires a 2xx decode failure be recoverable as that type", err, err)
	}
	if got := string(de.Body); got != capabilitiesArrayBody {
		t.Errorf("DecodeError.Body = %q, want the bytes the server delivered, %q", got, capabilitiesArrayBody)
	}
	if de.Method != http.MethodOptions {
		t.Errorf("DecodeError.Method = %q, want %q — the System API's single operation is OPTIONS /", de.Method, http.MethodOptions)
	}
	if de.Route != "/" {
		t.Errorf("DecodeError.Route = %q, want %q — the route template the leaf put on its own request", de.Route, "/")
	}
	if de.Unwrap() == nil {
		t.Error("DecodeError.Unwrap() = nil, want the decoder's error — REQ-151 requires the codec diagnostics stay reachable")
	}
	if op := "system.Capabilities:"; !strings.HasPrefix(err.Error(), op) {
		t.Errorf("error message %q does not start with the operation name %q; the leaf's wrap must survive the conversion", err.Error(), op)
	}
}

// TestCapabilitiesEmptyBodyKeepsInvalidShape holds the other side of the
// requirement: REQ-151 § An empty 2xx body keeps its existing contract. This
// arm is deliberately not unified under *transport.DecodeError, so callers
// already keying on transport.ErrInvalidShape keep working unchanged.
func TestCapabilitiesEmptyBodyKeepsInvalidShape(t *testing.T) { // REQ-151
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, _, err := system.Capabilities(t.Context(), newClient(t, srv))
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Fatalf("err = %v, want transport.ErrInvalidShape on an empty 2xx body", err)
	}
	if _, ok := errors.AsType[*transport.DecodeError](err); ok {
		t.Error("an empty 2xx body produced a *transport.DecodeError; REQ-151's keyed exclusion keeps this arm on ErrInvalidShape")
	}
}

// TestCapabilitiesWireErrorIsNotDecodeError pins REQ-151 § A non-2xx is never
// this error: the two classifications are disjoint, and recovering one must
// not recover the other.
func TestCapabilitiesWireErrorIsNotDecodeError(t *testing.T) { // REQ-151
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`not json either`))
	}))
	defer srv.Close()

	_, _, err := system.Capabilities(t.Context(), newClient(t, srv))
	if _, ok := errors.AsType[*transport.WireError](err); !ok {
		t.Fatalf("err = %T (%v), want *transport.WireError on a 404", err, err)
	}
	if _, ok := errors.AsType[*transport.DecodeError](err); ok {
		t.Error("a non-2xx response produced a *transport.DecodeError; REQ-151 forbids it")
	}
}
