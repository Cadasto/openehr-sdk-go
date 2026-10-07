package transport

// redirect_internal_test.go — REQ-021 and REQ-092: the copy a request
// carrying an Authorization header is sent with changes only its redirect
// policy. The fields are read off the copy itself, so this lives in the
// internal test package.

import (
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"
)

// TestHTTPClientForChangesOnlyTheRedirectPolicy pins that the copy keeps the
// injected client's Transport, Jar and Timeout, and that a request without an
// Authorization header gets the injected client itself.
func TestHTTPClientForChangesOnlyTheRedirectPolicy(t *testing.T) { // REQ-021, REQ-092
	t.Parallel()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	rt := &http.Transport{}
	injected := &http.Client{Transport: rt, Jar: jar, Timeout: 7 * time.Second}
	c := &Client{cfg: config{httpClient: injected}}

	withAuth, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.test/ehr", nil)
	if err != nil {
		t.Fatal(err)
	}
	withAuth.Header.Set("Authorization", "Bearer tok")
	cp := c.httpClientFor(withAuth)
	if cp == injected {
		t.Fatal("httpClientFor(request with Authorization) returned the injected client, want a copy")
	}
	if cp.Transport != http.RoundTripper(rt) || cp.Jar != http.CookieJar(jar) || cp.Timeout != 7*time.Second {
		t.Errorf("copy = {Transport: %p, Jar: %p, Timeout: %v}, want the injected client's {%p, %p, %v}", cp.Transport, cp.Jar, cp.Timeout, rt, jar, injected.Timeout)
	}
	if cp.CheckRedirect == nil {
		t.Error("copy has no CheckRedirect, want the downgrade refusal")
	}

	without, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.test/ehr", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.httpClientFor(without); got != injected {
		t.Errorf("httpClientFor(request without Authorization) = %p, want the injected client %p", got, injected)
	}
}
