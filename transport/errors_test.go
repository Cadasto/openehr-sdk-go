package transport_test

// errors_test.go — REQ-093 § value-free diagnostics, on the effectiveRoute
// axis: a *transport.Request with no Route template falls back to Path for
// telemetry (span naming, http.route attribute, Observation.Route — REQ-090 /
// REQ-098, deliberately untouched here), but Path may carry a caller-supplied
// identifier (an EHR id, a composition uid, …), and that must never reach a
// human-readable error string (REQ-093's value-free discipline). This file
// pins the fix across every surface that used to share the leak: the
// *transport.WireError built by doOnce's non-2xx branch, the
// *transport.DecodeError built by Decode's 2xx-but-undecodable branch, and
// the two remaining doOnce "transport: %s %s: …" diagnostics (read-body
// failure, over-limit) that do not carry a typed error at all.
//
// Deliberately external (package transport_test): every assertion is about
// what a consumer's errors.AsType / Error() call sees, reusing the
// newDecodeCatalog / newDecodeClient / decodeFake fixtures from
// decode_error_test.go in this same test package.

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// routeUnsetIDMarker is the distinctive fragment of routeUnsetIDPath that
// every assertion below checks is absent from an error string. Distinctive
// enough that an accidental substring match (e.g. against an unrelated word)
// would be a coincidence worth investigating on its own.
const routeUnsetIDMarker = "9c1a-live-request-id"

// routeUnsetIDPath is an identifier-bearing request path, standing in for a
// real EHR id or composition uid. Used with Route left empty — the "no route
// template" case this axis is about.
const routeUnsetIDPath = "/ehr/7f3e2c9a-91d4-4b8f-" + routeUnsetIDMarker

// unroutedPlaceholder is the stable, value-free text the fix substitutes for
// an unset Route template in diagnostic strings. Pinned here as a literal
// because, once shipped, its exact text is part of what a caller may already
// be matching against — REQ-093 requires it stay stable, not merely absent
// of the identifier.
const unroutedPlaceholder = "(unrouted)"

// requestWithUnroutedID builds a *transport.Request with Route unset and
// Path carrying routeUnsetIDMarker — the shape every subtest below drives
// through a different error path.
func requestWithUnroutedID(method string) *transport.Request {
	return &transport.Request{Method: method, Path: routeUnsetIDPath}
}

// assertValueFree fails t if got contains the identifier-bearing path, and
// separately fails it if got omits the stable placeholder — a silently empty
// or differently-shaped route slot would be as much a regression as the leak
// itself.
func assertValueFree(t *testing.T, got string) {
	t.Helper()
	if strings.Contains(got, routeUnsetIDMarker) {
		t.Errorf("error string = %q leaks the identifier-bearing path %q; REQ-093 requires diagnostics stay value-free when Route is unset", got, routeUnsetIDPath)
	}
	if !strings.Contains(got, unroutedPlaceholder) {
		t.Errorf("error string = %q, want it to carry the stable placeholder %q in place of the unset route template", got, unroutedPlaceholder)
	}
}

// TestErrorStringsValueFreeWhenRouteUnset pins REQ-093 across every surface
// that shared the effectiveRoute→Path leak: WireError (a non-2xx response),
// DecodeError (a 2xx body that fails to decode), and the two doOnce
// diagnostics that return a plain fmt.Errorf rather than a typed error
// (read-body failure, over-limit). A routed request is checked last as the
// regression guard: the fix must not touch the template a caller did set.
func TestErrorStringsValueFreeWhenRouteUnset(t *testing.T) { // REQ-093
	t.Run("WireError (non-2xx)", func(t *testing.T) {
		c := newDecodeClient(t, http.StatusNotFound, `{"code":"NOT_FOUND"}`, nil)

		_, err := c.Do(t.Context(), requestWithUnroutedID("GET"))
		if err == nil {
			t.Fatal("Do of a 404 succeeded; the premise of this test is gone")
		}
		we, ok := errors.AsType[*transport.WireError](err)
		if !ok {
			t.Fatalf("errors.AsType[*transport.WireError] did not match %T (%v)", err, err)
		}
		assertValueFree(t, we.Error())
		assertValueFree(t, we.Route)
	})

	t.Run("DecodeError (2xx, undecodable body)", func(t *testing.T) {
		c := newDecodeClient(t, http.StatusOK, `[1, 2, 3]`, nil)

		_, _, err := transport.Decode[decodeFake](t.Context(), c, requestWithUnroutedID("GET"))
		if err == nil {
			t.Fatal("Decode of a top-level JSON array into a struct succeeded; the premise of this test is gone")
		}
		de, ok := errors.AsType[*transport.DecodeError](err)
		if !ok {
			t.Fatalf("errors.AsType[*transport.DecodeError] did not match %T (%v)", err, err)
		}
		assertValueFree(t, de.Error())
		assertValueFree(t, de.Route)
	})

	t.Run("read-body failure", func(t *testing.T) {
		readErr := errors.New("simulated read failure")
		c := newBrokenTransportClient(t, func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       errReadCloser{err: readErr},
			}, nil
		})

		_, err := c.Do(t.Context(), requestWithUnroutedID("GET"))
		if err == nil {
			t.Fatal("Do with a body that always fails to read succeeded; the premise of this test is gone")
		}
		assertValueFree(t, err.Error())
	})

	t.Run("over-limit", func(t *testing.T) {
		oversized := strings.Repeat("x", 4<<10)
		c := newDecodeClient(t, http.StatusOK, oversized, nil, transport.WithMaxResponseBody(1<<10))

		_, err := c.Do(t.Context(), requestWithUnroutedID("GET"))
		if err == nil {
			t.Fatal("Do of an oversized response succeeded; the premise of this test is gone")
		}
		if !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("error %q should mention \"exceeds\"; the premise of this test — the over-limit branch — is gone", err.Error())
		}
		assertValueFree(t, err.Error())
	})

	// A DecodeError a caller builds directly — no Client involved — must
	// render the same placeholder rather than an empty slot mid-sentence.
	// WireError.Error() already guards this; the two are the same axis.
	t.Run("caller-constructed errors with an empty Route", func(t *testing.T) {
		de := &transport.DecodeError{Method: "GET"}
		if !strings.Contains(de.Error(), unroutedPlaceholder) {
			t.Errorf("DecodeError{Method: \"GET\"}.Error() = %q, want the %q placeholder in the empty route slot", de.Error(), unroutedPlaceholder)
		}
		we := &transport.WireError{Method: "GET", StatusCode: http.StatusNotFound}
		if !strings.Contains(we.Error(), unroutedPlaceholder) {
			t.Errorf("WireError{Method: \"GET\"}.Error() = %q, want the %q placeholder in the empty route slot", we.Error(), unroutedPlaceholder)
		}
	})

	// Regression guard: a routed request must keep printing the template
	// exactly as before — the fix targets the unset-Route case only.
	t.Run("routed request keeps the template", func(t *testing.T) {
		const route = "/ehr/{ehr_id}"
		c := newDecodeClient(t, http.StatusNotFound, `{"code":"NOT_FOUND"}`, nil)

		_, err := c.Do(t.Context(), &transport.Request{Method: "GET", Path: routeUnsetIDPath, Route: route})
		we, ok := errors.AsType[*transport.WireError](err)
		if !ok {
			t.Fatalf("errors.AsType[*transport.WireError] did not match %T (%v)", err, err)
		}
		if we.Route != route {
			t.Errorf("WireError.Route = %q, want the route template %q unchanged", we.Route, route)
		}
		if !strings.Contains(we.Error(), route) {
			t.Errorf("WireError.Error() = %q, want it to still carry the route template %q", we.Error(), route)
		}
		if strings.Contains(we.Error(), unroutedPlaceholder) {
			t.Errorf("WireError.Error() = %q carries the unrouted placeholder despite a routed request", we.Error())
		}
	})
}

// errReadCloser is an io.ReadCloser whose Read always fails with err —
// simulating a connection that drops mid-body, the doOnce read-body-failure
// arm (client.go's second "transport: %s %s: …" diagnostic).
type errReadCloser struct{ err error }

func (e errReadCloser) Read([]byte) (int, error) { return 0, e.err }
func (e errReadCloser) Close() error             { return nil }

// roundTripperFunc adapts a function to http.RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// newBrokenTransportClient returns a *transport.Client whose injected
// *http.Client never touches the network — every request is answered by rt
// directly. Used where the fixture needs to control the *http.Response (and
// in particular its Body) rather than what an httptest.Server can serve, so
// the target URL only needs to parse, not resolve.
func newBrokenTransportClient(t *testing.T, rt roundTripperFunc) *transport.Client {
	t.Helper()
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://test.example.com",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL("http://127.0.0.1/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := transport.New(cat, transport.WithHTTPClient(&http.Client{Transport: rt}))
	if err != nil {
		t.Fatal(err)
	}
	return c
}
