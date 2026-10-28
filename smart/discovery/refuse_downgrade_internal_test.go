package discovery

import (
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

// errCallerPolicy is what the injected client's own redirect policy returns
// below, so a test can tell its answer from the resolver's.
var errCallerPolicy = errors.New("the caller's redirect policy")

// redirectTo returns a redirect request to a URL with the given scheme.
func redirectTo(scheme string) *http.Request {
	return &http.Request{URL: &url.URL{Scheme: scheme, Host: "platform.test", Path: WellKnownPath}}
}

// injectedClient returns a client with its own Transport, Jar and Timeout, and
// checkRedirect as its redirect policy, which may be nil.
func injectedClient(t *testing.T, checkRedirect func(*http.Request, []*http.Request) error) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	return &http.Client{Transport: &http.Transport{}, Jar: jar, Timeout: 7 * time.Second, CheckRedirect: checkRedirect}
}

// sameButRedirectPolicy reports where cp differs from want in the fields
// other than CheckRedirect.
func sameButRedirectPolicy(t *testing.T, name string, cp *http.Client, want *http.Client) {
	t.Helper()
	if cp.Transport != want.Transport {
		t.Errorf("%s.Transport = %p, want the injected client's %p", name, cp.Transport, want.Transport)
	}
	if cp.Jar != want.Jar {
		t.Errorf("%s.Jar = %p, want the injected client's %p", name, cp.Jar, want.Jar)
	}
	if cp.Timeout != want.Timeout {
		t.Errorf("%s.Timeout = %v, want the injected client's %v", name, cp.Timeout, want.Timeout)
	}
}

// TestRefuseDowngradeChangesOnlyTheRedirectPolicy pins REQ-021 and REQ-073 for
// the copy of the injected client that the resolver fetches with. The copy
// keeps the injected client's Transport, Jar and Timeout. Its redirect policy
// refuses a redirect to a URL that is not https, and otherwise applies the
// injected client's own policy, or net/http's limit of 10 redirects when the
// injected client has none. The injected client stays as it was. With
// allowInsecure the resolver still fetches with a copy of its own, which
// keeps the injected client's redirect policy, so a later change to the
// injected client reaches the resolver in neither mode.
func TestRefuseDowngradeChangesOnlyTheRedirectPolicy(t *testing.T) { // REQ-021, REQ-073
	t.Parallel()

	t.Run("caller's policy", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		policy := func(*http.Request, []*http.Request) error {
			calls.Add(1)
			return errCallerPolicy
		}
		injected := injectedClient(t, policy)
		before := *injected

		cp := refuseDowngrade(injected, false)
		if cp == injected {
			t.Fatal("refuseDowngrade(injected, false) returned the injected client, want a copy")
		}
		sameButRedirectPolicy(t, "copy", cp, &before)
		if cp.CheckRedirect == nil {
			t.Fatal("copy.CheckRedirect = nil, want the downgrade refusal")
		}
		if err := cp.CheckRedirect(redirectTo("http"), nil); !errors.Is(err, errInsecureRedirect) {
			t.Errorf("copy.CheckRedirect(redirect to http) = %v, want %v", err, errInsecureRedirect)
		}
		if got := calls.Load(); got != 0 {
			t.Errorf("caller's policy ran %d time(s) for a redirect to http, want 0: the copy refuses it first", got)
		}
		if err := cp.CheckRedirect(redirectTo("https"), nil); !errors.Is(err, errCallerPolicy) {
			t.Errorf("copy.CheckRedirect(redirect to https) = %v, want the caller's policy's %v", err, errCallerPolicy)
		}
		if got := calls.Load(); got != 1 {
			t.Errorf("caller's policy ran %d time(s) after a redirect to https, want 1", got)
		}

		sameButRedirectPolicy(t, "injected", injected, &before)
		if injected.CheckRedirect == nil {
			t.Fatal("injected.CheckRedirect = nil after refuseDowngrade, want the caller's policy")
		}
		if err := injected.CheckRedirect(redirectTo("http"), nil); !errors.Is(err, errCallerPolicy) {
			t.Errorf("injected.CheckRedirect(redirect to http) = %v, want the caller's policy's %v: the injected client keeps its own policy", err, errCallerPolicy)
		}
	})

	t.Run("no caller policy", func(t *testing.T) {
		t.Parallel()
		injected := injectedClient(t, nil)
		before := *injected

		cp := refuseDowngrade(injected, false)
		if cp == injected {
			t.Fatal("refuseDowngrade(injected, false) returned the injected client, want a copy")
		}
		sameButRedirectPolicy(t, "copy", cp, &before)
		if cp.CheckRedirect == nil {
			t.Fatal("copy.CheckRedirect = nil, want the downgrade refusal")
		}
		// net/http's own limit: the 10th redirect is followed, the 11th is not.
		tests := []struct {
			via     int // redirects followed so far
			wantErr bool
		}{
			{via: 9, wantErr: false},
			{via: 10, wantErr: true},
		}
		for _, tc := range tests {
			err := cp.CheckRedirect(redirectTo("https"), make([]*http.Request, tc.via))
			if (err != nil) != tc.wantErr || errors.Is(err, errInsecureRedirect) {
				t.Errorf("copy.CheckRedirect(redirect to https after %d redirects) = %v, want an error: %t, and never the downgrade refusal", tc.via, err, tc.wantErr)
			}
		}
		if err := cp.CheckRedirect(redirectTo("http"), nil); !errors.Is(err, errInsecureRedirect) {
			t.Errorf("copy.CheckRedirect(redirect to http) = %v, want %v", err, errInsecureRedirect)
		}

		sameButRedirectPolicy(t, "injected", injected, &before)
		if injected.CheckRedirect != nil {
			t.Error("injected.CheckRedirect is set after refuseDowngrade, want nil unchanged")
		}
	})

	t.Run("allowInsecure", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		policy := func(*http.Request, []*http.Request) error {
			calls.Add(1)
			return errCallerPolicy
		}
		injected := injectedClient(t, policy)
		before := *injected

		cp := refuseDowngrade(injected, true)
		if cp == injected {
			t.Fatal("refuseDowngrade(injected, true) returned the injected client, want a copy")
		}
		sameButRedirectPolicy(t, "copy", cp, &before)
		if cp.CheckRedirect == nil {
			t.Fatal("copy.CheckRedirect = nil, want the caller's policy")
		}
		if err := cp.CheckRedirect(redirectTo("http"), nil); !errors.Is(err, errCallerPolicy) || calls.Load() != 1 {
			t.Errorf("copy.CheckRedirect(redirect to http) = %v after %d call(s) of the caller's policy, want the caller's policy's %v after 1", err, calls.Load(), errCallerPolicy)
		}
		sameButRedirectPolicy(t, "injected", injected, &before)
	})
}
