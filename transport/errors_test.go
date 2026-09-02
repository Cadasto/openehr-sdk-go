package transport_test

// errors_test.go — REQ-093 § value-free diagnostics, on the effectiveRoute
// axis: a *transport.Request with no Route template falls back to Path for
// telemetry (span naming, http.route attribute, Observation.Route — REQ-090 /
// REQ-098), but Path may carry a caller-supplied identifier (an EHR id, a
// composition uid, …), and that must never reach a human-readable error
// string (REQ-093's value-free discipline). This file pins the fix across
// every surface that used to share the leak: the *transport.WireError built
// by doOnce's non-2xx branch, the *transport.DecodeError built by Decode's
// 2xx-but-undecodable branch, the two remaining doOnce
// "transport: %s %s: …" diagnostics (read-body failure, over-limit) that do
// not carry a typed error at all, and the URL a wrapped net/http *url.Error
// prints from the network-failure arm.
//
// The telemetry half is pinned here too, as the carve-out it is rather than
// an untested assumption: TestUnroutedObservationKeepsTheResolvedPath holds
// Observation.Route to the request Path for an unrouted request, so the
// placeholder cannot silently spread from the diagnostic axis onto the
// telemetry one.
//
// Deliberately external (package transport_test): every assertion is about
// what a consumer's errors.AsType / Error() call sees, reusing the
// newDecodeCatalog / newDecodeClient / decodeFake fixtures from
// decode_error_test.go in this same test package.

import (
	"errors"
	"net/http"
	"net/url"
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
func newBrokenTransportClient(t *testing.T, rt roundTripperFunc, opts ...transport.Option) *transport.Client {
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
	c, err := transport.New(cat, append([]transport.Option{transport.WithHTTPClient(&http.Client{Transport: rt})}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// recordingObserver captures the single Observation the transport emits
// per logical call (REQ-098), so a test can assert on what an
// out-of-band telemetry sink sees rather than only on the returned
// error.
type recordingObserver struct{ last transport.Observation }

func (o *recordingObserver) OnRequest(obs transport.Observation) { o.last = obs }

// networkFailureClient returns a *transport.Client whose every attempt
// fails at the RoundTripper with cause, plus the hit counter and the
// observer. Retries are switched off explicitly (they are off by
// default, REQ-096) so the counter pins one attempt and the test cannot
// pass by accident on a second try.
func networkFailureClient(t *testing.T, cause error, obs transport.Observer) (*transport.Client, *int) {
	t.Helper()
	hits := 0
	c := newBrokenTransportClient(t, func(*http.Request) (*http.Response, error) {
		hits++
		return nil, cause
	}, transport.WithRetry(transport.NoRetry), transport.WithObserver(obs))
	return c, &hits
}

// TestNetworkFailureSanitisesWrappedURL pins REQ-093 on the last
// surface that leaked: a network failure comes back from net/http as a
// *url.Error whose own Error() prints the resolved URL, so the
// identifier a caller put in Request.Path reached the diagnostic even
// though doOnce's own prefix already rendered the placeholder. The
// transport now substitutes the same route slot into the *url.Error's
// URL field — the route template, or "(unrouted)" — while keeping the
// error's type, Op and cause, so nothing a caller inspects changes.
func TestNetworkFailureSanitisesWrappedURL(t *testing.T) { // REQ-093
	t.Run("unrouted request renders the placeholder", func(t *testing.T) {
		cause := errors.New("simulated dial failure")
		c, hits := networkFailureClient(t, cause, &recordingObserver{})

		_, err := c.Do(t.Context(), requestWithUnroutedID("GET"))
		if err == nil {
			t.Fatal("Do with a RoundTripper that always fails succeeded; the premise of this test is gone")
		}
		if *hits != 1 {
			t.Fatalf("RoundTripper hit %d times, want 1 — retries must be off so this test measures one attempt", *hits)
		}
		assertValueFree(t, err.Error())
		if !errors.Is(err, cause) {
			t.Errorf("err = %v; want errors.Is(_, cause) — sanitising the URL must not sever the chain", err)
		}
		ue, ok := errors.AsType[*url.Error](err)
		if !ok {
			t.Fatalf("errors.AsType[*url.Error] did not match %T (%v); the typed error a net/http caller relies on must survive sanitising", err, err)
		}
		if ue.URL != unroutedPlaceholder {
			t.Errorf("url.Error.URL = %q, want the route slot %q — the resolved URL may carry a caller identifier", ue.URL, unroutedPlaceholder)
		}
		if strings.Contains(ue.Error(), routeUnsetIDMarker) {
			t.Errorf("url.Error.Error() = %q leaks the identifier-bearing path %q", ue.Error(), routeUnsetIDPath)
		}
	})

	t.Run("routed request keeps the template", func(t *testing.T) {
		const route = "/ehr/{ehr_id}"
		cause := errors.New("simulated dial failure")
		c, _ := networkFailureClient(t, cause, &recordingObserver{})

		_, err := c.Do(t.Context(), &transport.Request{Method: "GET", Path: routeUnsetIDPath, Route: route})
		if err == nil {
			t.Fatal("Do with a RoundTripper that always fails succeeded; the premise of this test is gone")
		}
		ue, ok := errors.AsType[*url.Error](err)
		if !ok {
			t.Fatalf("errors.AsType[*url.Error] did not match %T (%v)", err, err)
		}
		if ue.URL != route {
			t.Errorf("url.Error.URL = %q, want the route template %q", ue.URL, route)
		}
		if strings.Contains(err.Error(), routeUnsetIDMarker) {
			t.Errorf("error string = %q leaks the identifier-bearing path %q", err.Error(), routeUnsetIDPath)
		}
		if strings.Contains(err.Error(), unroutedPlaceholder) {
			t.Errorf("error string = %q carries the unrouted placeholder despite a routed request", err.Error())
		}
	})

	// REQ-098: the observation carries the same error the caller got, so
	// a telemetry sink that logs Observation.Err must not re-introduce
	// the leak the returned string no longer has.
	t.Run("observer sees a value-free error", func(t *testing.T) {
		cause := errors.New("simulated dial failure")
		obs := &recordingObserver{}
		c, _ := networkFailureClient(t, cause, obs)

		if _, err := c.Do(t.Context(), requestWithUnroutedID("GET")); err == nil {
			t.Fatal("Do with a RoundTripper that always fails succeeded; the premise of this test is gone")
		}
		if obs.last.Err == nil {
			t.Fatal("Observation.Err is nil after a network failure; the premise of this test is gone")
		}
		assertValueFree(t, obs.last.Err.Error())
	})
}

// TestUnroutedObservationKeepsTheResolvedPath pins the carve-out REQ-093
// states explicitly: the placeholder binds the human-readable error
// strings only. The telemetry surfaces resolve the route their own way,
// so Observation.Route (REQ-098) still carries the request Path — an
// identifier and all — for an unrouted request, and must NOT be
// rewritten to the placeholder. The returned error string is asserted
// beside it as the contrast: same call, two deliberately different
// answers.
func TestUnroutedObservationKeepsTheResolvedPath(t *testing.T) { // REQ-093, REQ-098
	obs := &recordingObserver{}
	c := newDecodeClient(t, http.StatusNotFound, `{"code":"NOT_FOUND"}`, nil, transport.WithObserver(obs))

	_, err := c.Do(t.Context(), requestWithUnroutedID("GET"))
	if err == nil {
		t.Fatal("Do of a 404 succeeded; the premise of this test is gone")
	}
	if obs.last.Route != routeUnsetIDPath {
		t.Errorf("Observation.Route = %q, want the request Path %q — telemetry keeps its Path fallback (REQ-090 / REQ-098)", obs.last.Route, routeUnsetIDPath)
	}
	if obs.last.Route == unroutedPlaceholder {
		t.Errorf("Observation.Route = %q; the REQ-093 placeholder must not reach the telemetry axis", obs.last.Route)
	}
	assertValueFree(t, err.Error())
}
