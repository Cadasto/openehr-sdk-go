package transport

// redirect_internal_test.go — REQ-021 and REQ-092: the copy a request
// carrying an Authorization header is sent with changes only its redirect
// policy, and the injected client is left as it is. The fields are read off
// the copy itself, so this lives in the internal test package.

import (
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"
)

// TestHTTPClientForChangesOnlyTheRedirectPolicy pins that the copy keeps the
// injected client's Transport, Jar and Timeout, that a request without an
// Authorization header gets the injected client itself, and that neither
// call writes to the injected client.
func TestHTTPClientForChangesOnlyTheRedirectPolicy(t *testing.T) { // REQ-021, REQ-092
	t.Parallel()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	rt := &http.Transport{}
	const timeout = 7 * time.Second
	injected := &http.Client{Transport: rt, Jar: jar, Timeout: timeout}
	c := &Client{cfg: config{httpClient: injected}}

	withAuth, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.test/ehr", nil)
	if err != nil {
		t.Fatalf("http.NewRequestWithContext() error = %v", err)
	}
	withAuth.Header.Set("Authorization", "Bearer tok")
	cp := c.httpClientFor(withAuth)
	if cp == injected {
		t.Fatal("httpClientFor(request with Authorization) returned the injected client, want a copy")
	}
	if cp.Transport != http.RoundTripper(rt) {
		t.Errorf("copy.Transport = %v, want the injected client's %p", cp.Transport, rt)
	}
	if cp.Jar != http.CookieJar(jar) {
		t.Errorf("copy.Jar = %v, want the injected client's %p", cp.Jar, jar)
	}
	if cp.Timeout != timeout {
		t.Errorf("copy.Timeout = %v, want the injected client's %v", cp.Timeout, timeout)
	}
	if cp.CheckRedirect == nil {
		t.Error("copy.CheckRedirect = nil, want the downgrade refusal")
	}

	without, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.test/ehr", nil)
	if err != nil {
		t.Fatalf("http.NewRequestWithContext() error = %v", err)
	}
	if got := c.httpClientFor(without); got != injected {
		t.Errorf("httpClientFor(request without Authorization) = %p, want the injected client %p", got, injected)
	}

	// Neither call writes to the injected client.
	if injected.Transport != http.RoundTripper(rt) {
		t.Errorf("injected.Transport = %v after httpClientFor, want %p unchanged", injected.Transport, rt)
	}
	if injected.Jar != http.CookieJar(jar) {
		t.Errorf("injected.Jar = %v after httpClientFor, want %p unchanged", injected.Jar, jar)
	}
	if injected.Timeout != timeout {
		t.Errorf("injected.Timeout = %v after httpClientFor, want %v unchanged", injected.Timeout, timeout)
	}
	if injected.CheckRedirect != nil {
		t.Error("injected.CheckRedirect is set after httpClientFor, want nil unchanged")
	}
}
