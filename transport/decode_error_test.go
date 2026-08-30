package transport_test

// decode_error_test.go — REQ-151 § Typed 2xx decode failure.
//
// The package's other test files are internal (package transport). These are
// deliberately external: every assertion here is about the surface a consumer
// sees — the exported *transport.DecodeError, what errors.AsType recovers from
// the returned error, and what Error() is allowed to say — so the test compiles
// against the public API exactly as a consumer's own code would.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// decodeFake is a plain representation type. A top-level JSON array cannot
// decode into it, which is what makes the failure in these tests guaranteed —
// an object with unexpected keys would decode cleanly, because unknown fields
// are ignored and no required-field validation runs.
type decodeFake struct {
	Type  string `json:"_type"`
	Value string `json:"value"`
}

// phiMarker is a distinctive token planted in a response body so a leak of
// either the body or the decoder's text into Error() is unmistakable.
const phiMarker = "NOTANUMBER-8f2c1d"

// phiEchoingField reproduces the decoder class REQ-151 names by name: an
// UnmarshalJSON whose error embeds the offending value in a `parse %q` message,
// exactly as rm.Integer and rm.Real do (openehr/rm/integer.go, real.go). It is
// spelled out here rather than imported so the guard does not depend on any
// particular rm type keeping that message shape.
type phiEchoingField struct{}

func (*phiEchoingField) UnmarshalJSON(b []byte) error {
	return fmt.Errorf("rmfake.Integer: parse %q: invalid syntax", string(b))
}

// phiEchoingRM decodes through phiEchoingField, so a body carrying phiMarker
// produces a decoder error whose own text carries phiMarker too.
type phiEchoingRM struct {
	Magnitude phiEchoingField `json:"magnitude"`
}

// newDecodeCatalog points the openEHR REST service at srv.
func newDecodeCatalog(t *testing.T, srv *httptest.Server) *discovery.ServiceCatalog {
	t.Helper()
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://test.example.com",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

// newDecodeClient serves body with status and the given headers on every
// request, and returns a client aimed at it.
func newDecodeClient(t *testing.T, status int, body string, header http.Header) *transport.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for k, vs := range header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	c, err := transport.New(newDecodeCatalog(t, srv), transport.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestDecodeTypedErrorCarriesBody pins the whole positive contract of
// REQ-151 § The typed error: the failure is recoverable as a
// *transport.DecodeError, it carries the bytes the server delivered, it
// attributes the request, it still unwraps to the decoder's own typed
// diagnostics, and the metadata beside it survives (§ Metadata still arrives).
func TestDecodeTypedErrorCarriesBody(t *testing.T) { // REQ-151
	const body = `[1, 2, 3]`
	c := newDecodeClient(t, http.StatusOK, body, http.Header{"ETag": {`"v1"`}})

	out, meta, err := transport.Decode[decodeFake](t.Context(), c, &transport.Request{
		Method: "GET",
		Path:   "/ehr/abc/composition/def",
		Route:  "/ehr/{ehr_id}/composition/{uid}",
	})
	if err == nil {
		t.Fatal("Decode of a top-level JSON array into a struct succeeded; the premise of this test is gone")
	}
	if out != nil {
		t.Errorf("Decode returned a non-nil value %v beside the error; a failed decode has nothing to hand back", out)
	}

	de, ok := errors.AsType[*transport.DecodeError](err)
	if !ok {
		t.Fatalf("errors.AsType[*transport.DecodeError] did not match %T (%v); REQ-151 requires the 2xx decode failure be recoverable as that type", err, err)
	}

	if got := string(de.Body); got != body {
		t.Errorf("DecodeError.Body = %q, want the bytes the server delivered, %q", got, body)
	}
	if de.Method != "GET" {
		t.Errorf("DecodeError.Method = %q, want %q", de.Method, "GET")
	}
	if want := "/ehr/{ehr_id}/composition/{uid}"; de.Route != want {
		t.Errorf("DecodeError.Route = %q, want the route template %q, not the expanded URL", de.Route, want)
	}

	if de.Unwrap() == nil {
		t.Error("DecodeError.Unwrap() = nil, want the decoder's error — REQ-151 requires the codec diagnostics stay reachable")
	}
	// The codec's own typed diagnostic, reached through the DecodeError: proof
	// the chain is intact rather than merely non-nil.
	if _, ok := errors.AsType[*json.UnmarshalTypeError](err); !ok {
		t.Errorf("errors.AsType[*json.UnmarshalTypeError] did not reach through the DecodeError (inner = %v)", de.Unwrap())
	}

	if meta == nil {
		t.Fatal("Decode returned a nil *Metadata beside the decode error; REQ-151 § Metadata still arrives requires the headers survive")
	}
	if meta.ETag != "v1" {
		t.Errorf("Metadata.ETag = %q, want %q — the response headers must remain available beside the error", meta.ETag, "v1")
	}
}

// TestDecodeErrorStringIsValueFree is the guard behind REQ-151 § Error() stays
// value-free. It serves a body whose decoder error echoes the offending value
// in a `parse %q` message — the exact leak the rule exists to block — and pins
// that neither the body nor the decoder's text reaches the string surface.
// Reinstating the old `%w` wrap of the decoder error fails this test.
func TestDecodeErrorStringIsValueFree(t *testing.T) { // REQ-151
	const (
		body   = `{"magnitude":"` + phiMarker + `"}`
		method = "GET"
		route  = "/ehr/{ehr_id}/directory"
	)
	c := newDecodeClient(t, http.StatusOK, body, nil)

	_, _, err := transport.Decode[phiEchoingRM](t.Context(), c, &transport.Request{
		Method: method,
		Path:   "/ehr/abc/directory",
		Route:  route,
	})
	de, ok := errors.AsType[*transport.DecodeError](err)
	if !ok {
		t.Fatalf("errors.AsType[*transport.DecodeError] did not match %T (%v)", err, err)
	}

	inner := de.Unwrap()
	if inner == nil {
		t.Fatal("DecodeError.Unwrap() = nil; this test needs the decoder error it is guarding against echoing")
	}
	if !strings.Contains(inner.Error(), phiMarker) {
		t.Fatalf("the decoder error %q does not embed %q; the premise of this guard — a codec message that carries payload values — is gone", inner, phiMarker)
	}

	got := de.Error()

	// The attribution REQ-151 does require.
	if !strings.Contains(got, method) {
		t.Errorf("Error() = %q, want it to name the HTTP method %q", got, method)
	}
	if !strings.Contains(got, route) {
		t.Errorf("Error() = %q, want it to name the route template %q", got, route)
	}

	// The two whole strings, checked plainly.
	if strings.Contains(got, body) {
		t.Errorf("Error() = %q interpolates the response body; REQ-151 forbids it", got)
	}
	if strings.Contains(got, inner.Error()) {
		t.Errorf("Error() = %q interpolates the wrapped decoder's text; REQ-151 forbids it", got)
	}

	// And every distinctive word of either, so a partial echo is caught too.
	var e *transport.DecodeError // typed nil: the value-free template, with no method or route in it
	allowed := words(e.Error() + " " + method + " " + route)
	for _, src := range []string{body, inner.Error()} {
		for w := range words(src) {
			if _, ok := allowed[w]; ok {
				continue
			}
			if strings.Contains(strings.ToLower(got), w) {
				t.Errorf("Error() = %q leaks %q, a word of %q; REQ-151 § Error() stays value-free allows only the method, the route and the classification", got, w, src)
			}
		}
	}
}

// words splits s into its lowercased alphanumeric words of three characters or
// more — the granularity at which a leak is legible and a coincidence is not.
func words(s string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len(w) >= 3 {
			out[w] = struct{}{}
		}
	}
	return out
}

// TestDecodeNon2xxStaysWireError pins REQ-151 § A non-2xx is never this error:
// the two classifications are disjoint, and recovering one must not recover the
// other.
func TestDecodeNon2xxStaysWireError(t *testing.T) { // REQ-151
	c := newDecodeClient(t, http.StatusNotFound, `{"message":"no such ehr"}`, nil)

	_, _, err := transport.Decode[decodeFake](t.Context(), c, &transport.Request{
		Method: "GET",
		Path:   "/ehr/abc",
		Route:  "/ehr/{ehr_id}",
	})
	if err == nil {
		t.Fatal("Decode of a 404 succeeded; the premise of this test is gone")
	}
	if _, ok := errors.AsType[*transport.WireError](err); !ok {
		t.Errorf("errors.AsType[*transport.WireError] did not match %T (%v); a wire failure must stay a WireError", err, err)
	}
	if de, ok := errors.AsType[*transport.DecodeError](err); ok {
		t.Errorf("errors.AsType[*transport.DecodeError] matched a 404 (%v); REQ-151 forbids a non-2xx carrying that type", de)
	}
	if !errors.Is(err, transport.ErrNotFound) {
		t.Errorf("errors.Is(err, transport.ErrNotFound) = false for a 404 (%v)", err)
	}
}

// TestDecodeEmptyBodyStaysInvalidShape pins REQ-151 § An empty 2xx body keeps
// its existing contract: an absent body is not an unusable one, so callers
// keying on transport.ErrInvalidShape keep working unchanged.
func TestDecodeEmptyBodyStaysInvalidShape(t *testing.T) { // REQ-151
	c := newDecodeClient(t, http.StatusOK, "", nil)

	_, meta, err := transport.Decode[decodeFake](t.Context(), c, &transport.Request{
		Method: "GET",
		Path:   "/ehr/abc/ehr_status",
		Route:  "/ehr/{ehr_id}/ehr_status",
	})
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Errorf("errors.Is(err, transport.ErrInvalidShape) = false for an empty 200 body (%v); REQ-151 leaves this arm untouched", err)
	}
	if de, ok := errors.AsType[*transport.DecodeError](err); ok {
		t.Errorf("errors.AsType[*transport.DecodeError] matched an empty 200 body (%v); REQ-151 deliberately does not unify that arm", de)
	}
	if meta == nil {
		t.Error("Decode returned a nil *Metadata for an empty 200 body")
	}
}

// TestDecodeErrorNilReceiver pins the REQ-025 § No panics nil-receiver axis for
// the new exported type, in the shape NoRepresentationError and
// nilaqlerror_test.go already use. A failed errors.As leaves the caller holding
// a typed nil, which boxes into a non-nil error interface; every method must
// answer rather than dereference. Removing either nil guard fails this test.
func TestDecodeErrorNilReceiver(t *testing.T) { // REQ-151
	var e *transport.DecodeError

	t.Run("Error", func(t *testing.T) {
		got := e.Error()
		if got == "" {
			t.Fatal(`nil.Error() = "", want a non-empty string — fmt would print nothing for this error`)
		}
		if strings.Contains(got, "%!") {
			t.Errorf("nil.Error() = %q, which carries an fmt verb failure", got)
		}
	})

	t.Run("Unwrap", func(t *testing.T) {
		if got := e.Unwrap(); got != nil {
			t.Errorf("nil.Unwrap() = %v, want nil — a nil DecodeError wraps nothing", got)
		}
	})

	t.Run("boxed", func(t *testing.T) {
		// What a caller holds after a failed errors.As and passes onward: a
		// non-nil error interface carrying a nil *DecodeError. Each call below
		// dispatches into a method on that nil receiver.
		boxed := error(e)
		if boxed.Error() == "" {
			t.Error(`Error() on a boxed nil DecodeError = "", want a non-empty string`)
		}
		if got := errors.Unwrap(boxed); got != nil {
			t.Errorf("errors.Unwrap(boxed nil DecodeError) = %v, want nil", got)
		}
		if errors.Is(boxed, transport.ErrInvalidShape) {
			t.Error("errors.Is(boxed nil DecodeError, transport.ErrInvalidShape) = true, want false")
		}
	})
}
