package noredirect_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/internal/noredirect"
)

// TestREQ060_ClientReturnsRedirectAnswer pins REQ-060 (§ Credential-bearing
// requests): a POST through the guarded client comes back as the 3xx
// answer, for every redirect status and whatever CheckRedirect the injected
// client has, and the second server never receives a request.
func TestREQ060_ClientReturnsRedirectAnswer(t *testing.T) {
	statuses := []int{
		http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect,
	}
	policies := []struct {
		name   string
		policy func(*http.Request, []*http.Request) error
	}{
		{"default policy", nil},
		{"policy follows", func(*http.Request, []*http.Request) error { return nil }},
		{"policy refuses", func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
	for _, status := range statuses {
		for _, p := range policies {
			t.Run(fmt.Sprintf("%d/%s", status, p.name), func(t *testing.T) {
				var targetHits atomic.Int32
				target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					targetHits.Add(1)
				}))
				t.Cleanup(target.Close)
				redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Location", target.URL)
					w.WriteHeader(status)
				}))
				t.Cleanup(redirector.Close)

				injected := &http.Client{Transport: redirector.Client().Transport, CheckRedirect: p.policy}
				req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, redirector.URL, strings.NewReader("refresh_token=rt-secret"))
				if err != nil {
					t.Fatalf("NewRequestWithContext() error = %v", err)
				}
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				resp, err := noredirect.Client(injected).Do(req)
				if err != nil {
					t.Fatalf("Do() error = %v, want the %d answer", err, status)
				}
				defer func() { _ = resp.Body.Close() }()
				_, _ = io.Copy(io.Discard, resp.Body)

				if resp.StatusCode != status {
					t.Errorf("Do() status = %d, want %d", resp.StatusCode, status)
				}
				if got := targetHits.Load(); got != 0 {
					t.Errorf("the redirect target received %d request(s), want none", got)
				}
			})
		}
	}
}

// TestREQ060_ClientLeavesInjectedClientAlone pins that the guarded client is
// a copy: the injected client keeps its CheckRedirect, and the copy keeps
// its Transport, Jar and Timeout.
func TestREQ060_ClientLeavesInjectedClientAlone(t *testing.T) {
	var calls atomic.Int32
	policy := func(*http.Request, []*http.Request) error {
		calls.Add(1)
		return nil
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	transport := &http.Transport{}
	injected := &http.Client{Transport: transport, Jar: jar, Timeout: 7 * time.Second, CheckRedirect: policy}

	guarded := noredirect.Client(injected)

	if guarded == injected {
		t.Fatal("Client() returned the injected client itself, want a copy")
	}
	if guarded.Transport != http.RoundTripper(transport) || guarded.Jar != http.CookieJar(jar) || guarded.Timeout != 7*time.Second {
		t.Errorf("copy = {Transport: %v, Jar: %v, Timeout: %v}, want the injected client's", guarded.Transport, guarded.Jar, guarded.Timeout)
	}
	if injected.CheckRedirect == nil {
		t.Fatal("the injected client lost its CheckRedirect")
	}
	_ = injected.CheckRedirect(&http.Request{URL: &url.URL{}}, nil)
	if got := calls.Load(); got != 1 {
		t.Errorf("the injected client's CheckRedirect ran the caller's policy %d time(s), want 1", got)
	}
	if err := guarded.CheckRedirect(&http.Request{URL: &url.URL{}}, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Errorf("copy.CheckRedirect() = %v, want http.ErrUseLastResponse", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("the copy ran the caller's policy: its count is %d, want 1", got)
	}
}
