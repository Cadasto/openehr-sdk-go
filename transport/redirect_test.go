package transport_test

// redirect_test.go — REQ-092 § No downgrade with a credential.
//
// External on purpose: the assertions are about what a consumer sees — the
// error Do returns, which server the request reached, and the *http.Client the
// consumer injected.

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

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

// newRedirector starts an https server on 127.0.0.1 that answers every request
// with a 307 to location, and counts the requests it received.
func newRedirector(t *testing.T, location string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, location, http.StatusTemporaryRedirect)
	}))
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

// TestDoRefusesHTTPSToHTTPRedirectWithToken reproduces the downgrade: an https
// origin answers 307 to a plain-http URL on the same host, and net/http copies
// the Authorization header to a same-host target whatever its scheme.
func TestDoRefusesHTTPSToHTTPRedirectWithToken(t *testing.T) { // REQ-092
	t.Parallel()
	plain := newLanding(t, false)
	origin, _ := newRedirector(t, plain.srv.URL+"/landing")
	c := newRedirectClient(t, origin, origin.Client(),
		transport.WithTokenSource(auth.StaticTokenSource(auth.Token{Value: "secret-tok"})))

	_, err := c.Do(t.Context(), &transport.Request{Path: "/ehr"})
	if got := plain.hits.Load(); got != 0 {
		t.Errorf("plain-http target received %d request(s) carrying Authorization %q, want none", got, plain.authHeaders())
	}
	if err == nil {
		t.Fatal("Do followed an https to http redirect with a token: err = nil, want a refusal")
	}
}
