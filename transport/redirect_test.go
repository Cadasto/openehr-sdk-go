package transport_test

// redirect_test.go — REQ-092 § No downgrade with a credential.
//
// External on purpose: the assertions are about what a consumer sees — the
// error Do returns, which server the request reached, and the *http.Client the
// consumer injected.

import (
	"cmp"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// landing is a redirect target. It answers 200 with an empty JSON object and
// records how many requests reached it and the Authorization header each one
// carried.
type landing struct {
	srv  *httptest.Server
	hits atomic.Int32
	mu   sync.Mutex
	auth []string
}

// newLanding starts a landing server on 127.0.0.1, over TLS when useTLS is
// true. Every httptest TLS server uses the same built-in certificate, so the
// client of one TLS server trusts another.
func newLanding(t *testing.T, useTLS bool) *landing {
	t.Helper()
	l := &landing{}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.hits.Add(1)
		l.mu.Lock()
		l.auth = append(l.auth, r.Header.Get("Authorization"))
		l.mu.Unlock()
		_, _ = w.Write([]byte(`{}`))
	})
	if useTLS {
		l.srv = httptest.NewTLSServer(h)
	} else {
		l.srv = httptest.NewServer(h)
	}
	t.Cleanup(l.srv.Close)
	return l
}

// authHeaders returns the Authorization headers the landing server saw.
func (l *landing) authHeaders() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.auth...)
}

// newRedirector starts a server on 127.0.0.1, over TLS when useTLS is true,
// that answers every request with a 307 to location, and counts the requests
// it received.
func newRedirector(t *testing.T, useTLS bool, location string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, location, http.StatusTemporaryRedirect)
	})
	var srv *httptest.Server
	if useTLS {
		srv = httptest.NewTLSServer(h)
	} else {
		srv = httptest.NewServer(h)
	}
	t.Cleanup(srv.Close)
	return srv, &hits
}

// newRedirectClient returns a transport.Client whose openEHR REST service is
// served by origin, sending with hc.
func newRedirectClient(t *testing.T, origin *httptest.Server, hc *http.Client, opts ...transport.Option) *transport.Client {
	t.Helper()
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://test.example.com",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(origin.URL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := transport.New(cat, append([]transport.Option{transport.WithHTTPClient(hc)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// copyClient returns a shallow copy of base, so a test can change the injected
// client without touching the httptest server's own client.
func copyClient(base *http.Client) *http.Client {
	hc := new(http.Client)
	*hc = *base
	return hc
}

// withToken is the client-default static bearer token the tests send.
func withToken(v string) transport.Option {
	return transport.WithTokenSource(auth.StaticTokenSource(auth.Token{Value: v}))
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestDoRefusesHTTPSToHTTPRedirectWithToken pins the refusal: an https hop
// answers 307 to a plain-http URL on the same host, where net/http would copy
// the Authorization header whatever the scheme. The second case starts on
// plain http and is upgraded to https first, so the hop is judged from the
// request just before the refused one, not from the first. The third case
// redirects to a scheme that is neither http nor https, which is refused too:
// the rule names any URL whose scheme is not https.
func TestDoRefusesHTTPSToHTTPRedirectWithToken(t *testing.T) { // REQ-092
	t.Parallel()
	for _, tc := range []struct {
		name     string
		upgraded bool
		location string // empty: the plain-http landing server
	}{
		{name: "https origin redirects to http"},
		{name: "http origin redirects to https, which redirects to http", upgraded: true},
		{name: "https origin redirects to a ws URL", location: "ws://127.0.0.1:1/landing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			plain := newLanding(t, false)
			location := cmp.Or(tc.location, plain.srv.URL+"/landing")
			origin, _ := newRedirector(t, true, location)
			hc := origin.Client()
			if tc.upgraded {
				origin, _ = newRedirector(t, false, origin.URL+"/openehr/v1/ehr")
			}
			c := newRedirectClient(t, origin, hc, withToken("secret-tok"))

			_, err := c.Do(t.Context(), &transport.Request{Path: "/ehr"})
			if got := plain.hits.Load(); got != 0 {
				t.Errorf("plain-http target received %d request(s) carrying Authorization %q, want none", got, plain.authHeaders())
			}
			if !errors.Is(err, transport.ErrInsecureRedirect) {
				t.Fatalf("Do() error = %v, want one matching transport.ErrInsecureRedirect", err)
			}
		})
	}
}

// TestDoRefusesHTTPSToHTTPRedirectWithCallerAuthorization pins that the
// refusal follows the header, not the token source: a caller who sets
// Authorization through Request.Headers, with no token source, is protected
// the same way.
func TestDoRefusesHTTPSToHTTPRedirectWithCallerAuthorization(t *testing.T) { // REQ-092
	t.Parallel()
	plain := newLanding(t, false)
	origin, _ := newRedirector(t, true, plain.srv.URL+"/landing")
	c := newRedirectClient(t, origin, origin.Client(), transport.WithTokenSource(auth.AnonymousTokenSource()))

	_, err := c.Do(t.Context(), &transport.Request{
		Path:    "/ehr",
		Headers: http.Header{"authorization": {"Basic dXNlcjpwYXNz"}},
	})
	if got := plain.hits.Load(); got != 0 {
		t.Errorf("plain-http target received %d request(s) carrying Authorization %q, want none", got, plain.authHeaders())
	}
	if !errors.Is(err, transport.ErrInsecureRedirect) {
		t.Fatalf("Do() error = %v, want one matching transport.ErrInsecureRedirect", err)
	}
}

// TestDoFollowsHTTPSToHTTPRedirectWithoutAuthorization pins that a request
// carrying no Authorization header follows the injected client's policy
// unchanged, even across an https to http hop.
func TestDoFollowsHTTPSToHTTPRedirectWithoutAuthorization(t *testing.T) { // REQ-092
	t.Parallel()
	for _, tc := range []struct {
		name string
		opts []transport.Option
		req  transport.Request
	}{
		{name: "anonymous token source", req: transport.Request{Path: "/ehr"}},
		{name: "NoAuth with a token source", opts: []transport.Option{withToken("tok")}, req: transport.Request{Path: "/ehr", NoAuth: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			plain := newLanding(t, false)
			origin, _ := newRedirector(t, true, plain.srv.URL+"/landing")
			c := newRedirectClient(t, origin, origin.Client(), tc.opts...)

			if _, err := c.Do(t.Context(), &tc.req); err != nil {
				t.Fatalf("Do() error = %v, want the redirect followed", err)
			}
			if got := plain.hits.Load(); got != 1 {
				t.Errorf("plain-http target received %d request(s), want 1", got)
			}
			if got := plain.authHeaders(); len(got) != 1 || got[0] != "" {
				t.Errorf("plain-http target saw Authorization %q, want one request without it", got)
			}
		})
	}
}

// TestDoFollowsHTTPToHTTPRedirectWithToken pins the scope of the refusal: it
// is about leaving https. A request that started on plain http and stays
// there is not a downgrade, so it follows the injected client's policy.
func TestDoFollowsHTTPToHTTPRedirectWithToken(t *testing.T) { // REQ-092
	t.Parallel()
	plain := newLanding(t, false)
	origin, _ := newRedirector(t, false, plain.srv.URL+"/landing")
	c := newRedirectClient(t, origin, origin.Client(), withToken("tok"))

	if _, err := c.Do(t.Context(), &transport.Request{Path: "/ehr"}); err != nil {
		t.Fatalf("Do() error = %v, want the http to http redirect followed", err)
	}
	if got := plain.hits.Load(); got != 1 {
		t.Errorf("plain-http target received %d request(s), want 1", got)
	}
}

// TestDoFollowsHTTPSToHTTPSRedirectWithToken pins that a redirect staying on
// https is followed with the token, as the injected client would.
func TestDoFollowsHTTPSToHTTPSRedirectWithToken(t *testing.T) { // REQ-092
	t.Parallel()
	secure := newLanding(t, true)
	origin, _ := newRedirector(t, true, secure.srv.URL+"/landing")
	c := newRedirectClient(t, origin, origin.Client(), withToken("tok"))

	if _, err := c.Do(t.Context(), &transport.Request{Path: "/ehr"}); err != nil {
		t.Fatalf("Do() error = %v, want the https to https redirect followed", err)
	}
	if got := secure.authHeaders(); len(got) != 1 || got[0] != "Bearer tok" {
		t.Errorf("https target saw Authorization %q, want one request with %q", got, "Bearer tok")
	}
}

// TestDoAppliesCallerRedirectPolicyWithToken pins that the copy defers to the
// injected client's own CheckRedirect for an https to https hop. The policy is
// set after transport.New, so a copy taken in New would not see it.
func TestDoAppliesCallerRedirectPolicyWithToken(t *testing.T) { // REQ-092, REQ-021
	t.Parallel()
	secure := newLanding(t, true)
	origin, _ := newRedirector(t, true, secure.srv.URL+"/landing")
	hc := copyClient(origin.Client())
	c := newRedirectClient(t, origin, hc, withToken("tok"))

	var calls atomic.Int32
	hc.CheckRedirect = func(*http.Request, []*http.Request) error {
		calls.Add(1)
		return http.ErrUseLastResponse
	}

	_, err := c.Do(t.Context(), &transport.Request{Path: "/ehr"})
	we, ok := errors.AsType[*transport.WireError](err)
	if !ok || we == nil || we.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("Do() error = %v, want a *transport.WireError with status 307 from the caller's policy", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("caller's CheckRedirect called %d time(s), want 1", got)
	}
	if got := secure.hits.Load(); got != 0 {
		t.Errorf("https target received %d request(s), want none: the caller's policy returns the 307", got)
	}
}

// TestDoKeepsDefaultRedirectLimitWithToken pins that the copy applies
// net/http's limit of 10 redirects when the injected client has no
// CheckRedirect. The server stops redirecting after 15 requests, so a copy
// without the limit would succeed instead of failing after 10.
func TestDoKeepsDefaultRedirectLimitWithToken(t *testing.T) { // REQ-092
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) > 15 {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		http.Redirect(w, r, "https://"+r.Host+"/loop", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	c := newRedirectClient(t, srv, srv.Client(), withToken("tok"))

	_, err := c.Do(t.Context(), &transport.Request{Path: "/ehr"})
	if err == nil {
		t.Fatalf("Do() error = nil after %d requests, want the 10-redirect limit to stop it", hits.Load())
	}
	if errors.Is(err, transport.ErrInsecureRedirect) {
		t.Errorf("Do() error = %v, want the redirect limit, not an insecure-redirect refusal", err)
	}
	if got := hits.Load(); got != 10 {
		t.Errorf("server received %d request(s), want 10 (the first plus 9 followed redirects)", got)
	}
}

// TestDoLeavesInjectedClientUnmodified pins that the refusal lives on a copy:
// after a refused redirect, the injected client's CheckRedirect is still the
// caller's value, and nil stays nil.
func TestDoLeavesInjectedClientUnmodified(t *testing.T) { // REQ-092, REQ-021
	t.Parallel()

	t.Run("nil CheckRedirect stays nil", func(t *testing.T) {
		t.Parallel()
		plain := newLanding(t, false)
		origin, _ := newRedirector(t, true, plain.srv.URL+"/landing")
		hc := copyClient(origin.Client())
		hc.CheckRedirect = nil
		c := newRedirectClient(t, origin, hc, withToken("tok"))

		if _, err := c.Do(t.Context(), &transport.Request{Path: "/ehr"}); !errors.Is(err, transport.ErrInsecureRedirect) {
			t.Fatalf("Do() error = %v, want one matching transport.ErrInsecureRedirect", err)
		}
		if hc.CheckRedirect != nil {
			t.Error("injected client's CheckRedirect is set after Do, want nil as the caller left it")
		}
	})

	t.Run("caller CheckRedirect stays the caller's", func(t *testing.T) {
		t.Parallel()
		plain := newLanding(t, false)
		origin, _ := newRedirector(t, true, plain.srv.URL+"/landing")
		hc := copyClient(origin.Client())
		errCaller := errors.New("caller policy")
		hc.CheckRedirect = func(*http.Request, []*http.Request) error { return errCaller }
		c := newRedirectClient(t, origin, hc, withToken("tok"))

		if _, err := c.Do(t.Context(), &transport.Request{Path: "/ehr"}); !errors.Is(err, transport.ErrInsecureRedirect) {
			t.Fatalf("Do() error = %v, want one matching transport.ErrInsecureRedirect", err)
		}
		// Ask the injected client's policy about the very hop the transport
		// refused: the caller's own function answers, not the refusal.
		from := httptest.NewRequest(http.MethodGet, origin.URL+"/openehr/v1/ehr", nil)
		to := httptest.NewRequest(http.MethodGet, plain.srv.URL+"/landing", nil)
		if got := hc.CheckRedirect(to, []*http.Request{from}); !errors.Is(got, errCaller) {
			t.Errorf("injected client's CheckRedirect(https to http) = %v, want the caller's own answer %q", got, errCaller)
		}
	})
}

// TestDoUsesTransportSetAfterNew pins that the copy is not taken in New: a
// Transport the caller sets on the injected client after transport.New
// carries the first request that has an Authorization header. A change made
// between two requests is TestDoAppliesInjectedClientChangesBetweenRequests.
func TestDoUsesTransportSetAfterNew(t *testing.T) { // REQ-092, REQ-021
	t.Parallel()
	secure := newLanding(t, true)
	base := secure.srv.Client()
	hc := copyClient(base)
	c := newRedirectClient(t, secure.srv, hc, withToken("tok"))

	var trips atomic.Int32
	hc.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		trips.Add(1)
		return base.Transport.RoundTrip(r)
	})

	if _, err := c.Do(t.Context(), &transport.Request{Path: "/ehr"}); err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if got := trips.Load(); got != 1 {
		t.Errorf("Transport set after transport.New carried %d request(s), want 1", got)
	}
}

// TestDoAppliesInjectedClientChangesBetweenRequests pins that the copy is
// taken for each request, not cached after the first: a change the caller
// makes to the injected client between two requests on one Client reaches
// the second request.
func TestDoAppliesInjectedClientChangesBetweenRequests(t *testing.T) { // REQ-092, REQ-021
	t.Parallel()

	t.Run("CheckRedirect set between requests", func(t *testing.T) {
		t.Parallel()
		secure := newLanding(t, true)
		origin, _ := newRedirector(t, true, secure.srv.URL+"/landing")
		hc := copyClient(origin.Client())
		c := newRedirectClient(t, origin, hc, withToken("tok"))

		if _, err := c.Do(t.Context(), &transport.Request{Path: "/ehr"}); err != nil {
			t.Fatalf("first Do() error = %v, want the https to https redirect followed", err)
		}
		var calls atomic.Int32
		hc.CheckRedirect = func(*http.Request, []*http.Request) error {
			calls.Add(1)
			return http.ErrUseLastResponse
		}
		_, err := c.Do(t.Context(), &transport.Request{Path: "/ehr"})
		we, ok := errors.AsType[*transport.WireError](err)
		if !ok || we == nil || we.StatusCode != http.StatusTemporaryRedirect {
			t.Fatalf("second Do() error = %v, want a *transport.WireError with status 307 from the policy set between requests", err)
		}
		if got := calls.Load(); got != 1 {
			t.Errorf("CheckRedirect set between requests called %d time(s), want 1", got)
		}
		if got := secure.hits.Load(); got != 1 {
			t.Errorf("https target received %d request(s), want 1: only the first request follows the redirect", got)
		}
	})

	t.Run("Transport set between requests", func(t *testing.T) {
		t.Parallel()
		secure := newLanding(t, true)
		base := secure.srv.Client()
		hc := copyClient(base)
		c := newRedirectClient(t, secure.srv, hc, withToken("tok"))

		if _, err := c.Do(t.Context(), &transport.Request{Path: "/ehr"}); err != nil {
			t.Fatalf("first Do() error = %v", err)
		}
		var trips atomic.Int32
		hc.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			trips.Add(1)
			return base.Transport.RoundTrip(r)
		})
		if _, err := c.Do(t.Context(), &transport.Request{Path: "/ehr"}); err != nil {
			t.Fatalf("second Do() error = %v", err)
		}
		if got := trips.Load(); got != 1 {
			t.Errorf("Transport set between requests carried %d request(s), want 1", got)
		}
	})
}

// TestDoDoesNotRetryRefusedRedirect pins that the refusal is final: with a
// retry policy that retries a GET's network errors, the origin is still asked
// exactly once, because every attempt would be refused the same way.
func TestDoDoesNotRetryRefusedRedirect(t *testing.T) { // REQ-092, REQ-091
	t.Parallel()
	plain := newLanding(t, false)
	origin, originHits := newRedirector(t, true, plain.srv.URL+"/landing")
	c := newRedirectClient(t, origin, origin.Client(), withToken("tok"),
		transport.WithRetry(transport.RetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     5 * time.Millisecond,
		}))

	_, err := c.Do(t.Context(), &transport.Request{Method: http.MethodGet, Path: "/ehr"})
	if !errors.Is(err, transport.ErrInsecureRedirect) {
		t.Fatalf("Do() error = %v, want one matching transport.ErrInsecureRedirect", err)
	}
	if got := originHits.Load(); got != 1 {
		t.Errorf("https origin received %d request(s), want 1: a refused redirect is not retried", got)
	}
	if got := plain.hits.Load(); got != 0 {
		t.Errorf("plain-http target received %d request(s), want none", got)
	}
}

// TestDoRefusedRedirectErrorIsValueFree pins that the refusal's text names the
// route template only: net/http reports the redirect target in its *url.Error,
// and that URL, the expanded path and the token must not reach Error().
func TestDoRefusedRedirectErrorIsValueFree(t *testing.T) { // REQ-092, REQ-093
	t.Parallel()
	const (
		token    = "tok-7f3e9a1c"
		targetID = "target-5c1b2d4e"
		ehrID    = "ehr-0a9b8c7d"
		route    = "/ehr/{ehr_id}"
	)
	plain := newLanding(t, false)
	origin, _ := newRedirector(t, true, plain.srv.URL+"/landing/"+targetID)
	c := newRedirectClient(t, origin, origin.Client(), withToken(token))

	_, err := c.Do(t.Context(), &transport.Request{Path: "/ehr/" + ehrID, Route: route})
	if !errors.Is(err, transport.ErrInsecureRedirect) {
		t.Fatalf("Do() error = %v, want one matching transport.ErrInsecureRedirect", err)
	}
	msg := err.Error()
	for _, leak := range []string{plain.srv.URL, targetID, origin.URL, ehrID, token} {
		if strings.Contains(msg, leak) {
			t.Errorf("Do() error %q contains %q, want it value-free", msg, leak)
		}
	}
	if !strings.Contains(msg, route) {
		t.Errorf("Do() error %q does not name the route template %q", msg, route)
	}
}
