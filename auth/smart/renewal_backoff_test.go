package smart_test

import (
	"errors"
	"net/http"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
)

// outage answers every token request with a 503, a transient failure.
func outage(r *http.Request, _ int) (*http.Response, error) {
	return scriptedAnswer(r, http.StatusServiceUnavailable, outageBody), nil
}

// TestREQ063_TransientRenewalFailureServesTheHeldToken pins REQ-063 (auth.md
// § REQ-063): when a refresh fails transiently while the held access token
// is not yet past its ExpiresAt, Token returns that token without an error,
// and the calls that follow within the backoff send no request.
func TestREQ063_TransientRenewalFailureServesTheHeldToken(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := &scriptedServer{refresh: outage}
		var log changeLog
		src := scriptedSource(t, srv, smart.WithTokenChange(log.record))
		src.SetTokens(staleAccess("at-0"), "rt-0")

		for i := range 10 {
			tok, err := src.Token(t.Context())
			if err != nil || tok.Value != "at-0" {
				t.Errorf("Token() call %d = (%q, %v), want (at-0, nil): the held token is still valid", i+1, tok.Value, err)
			}
		}
		if n, _ := srv.counts(); n != 1 {
			t.Errorf("refresh requests after 10 Token() calls = %d, want 1", n)
		}
		if access, refresh := src.HeldTokens(); access.Value != "at-0" || refresh != "rt-0" {
			t.Errorf("held tokens = (%q, %q), want (at-0, rt-0)", access.Value, refresh)
		}
		if changes := log.all(); len(changes) != 0 {
			t.Errorf("token changes = %q, want none: serving the held token is not a successful refresh", accessValues(changes))
		}
	})
}

// TestREQ063_RenewalBackoffDoublesUntilExpiry pins REQ-063: the backoff after
// a transient failure is one second, twice as long after each further
// failure in a row, and ends no later than the held token's ExpiresAt. Past
// ExpiresAt the failure surfaces as auth.ErrRefreshFailed on every call.
func TestREQ063_RenewalBackoffDoublesUntilExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := &scriptedServer{refresh: outage}
		src := scriptedSource(t, srv)
		src.SetTokens(staleAccess("at-0"), "rt-0") // expires 10s from now

		// Each step sleeps, then calls Token once. Failures at 0s, 1s, 3s and
		// 7s; the fourth backoff of 8s is cut to the expiry at 10s.
		steps := []struct {
			sleep    time.Duration
			requests int
			expired  bool
		}{
			{sleep: 0, requests: 1},
			{sleep: 999 * time.Millisecond, requests: 1},
			{sleep: time.Millisecond, requests: 2},
			{sleep: 1999 * time.Millisecond, requests: 2},
			{sleep: time.Millisecond, requests: 3},
			{sleep: 3999 * time.Millisecond, requests: 3},
			{sleep: time.Millisecond, requests: 4},
			{sleep: 2999 * time.Millisecond, requests: 4},
			{sleep: time.Millisecond, requests: 5, expired: true},
			{sleep: 0, requests: 6, expired: true},
		}
		start := time.Now()
		for _, step := range steps {
			time.Sleep(step.sleep)
			at := time.Since(start)
			tok, err := src.Token(t.Context())
			if step.expired {
				if !errors.Is(err, auth.ErrRefreshFailed) || errors.Is(err, auth.ErrReauthRequired) {
					t.Errorf("Token() at %v error = %v, want auth.ErrRefreshFailed: the held token has expired", at, err)
				}
			} else if err != nil || tok.Value != "at-0" {
				t.Errorf("Token() at %v = (%q, %v), want (at-0, nil)", at, tok.Value, err)
			}
			if n, _ := srv.counts(); n != step.requests {
				t.Errorf("refresh requests at %v = %d, want %d", at, n, step.requests)
			}
		}
	})
}

// TestREQ063_NewTokensEndTheBackoff pins REQ-063: a successful refresh, or
// tokens SetTokens installs, ends the backoff, so the next transient failure
// waits one second again and new tokens are refreshed at once.
func TestREQ063_NewTokensEndTheBackoff(t *testing.T) {
	t.Run("successful refresh", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			srv := &scriptedServer{}
			srv.refresh = func(r *http.Request, n int) (*http.Response, error) {
				if n == 2 {
					// Stale again after 10 seconds under the 30-second threshold.
					return scriptedAnswer(r, http.StatusOK, `{"access_token":"at-new","token_type":"Bearer","expires_in":40}`), nil
				}
				return outage(r, n)
			}
			src := scriptedSource(t, srv)
			src.SetTokens(staleAccess("at-0"), "rt-0")

			_, _ = src.Token(t.Context()) // fails: one second of backoff, then two
			time.Sleep(time.Second)
			if tok, err := src.Token(t.Context()); err != nil || tok.Value != "at-new" {
				t.Fatalf("Token() after the backoff = (%q, %v), want (at-new, nil)", tok.Value, err)
			}
			time.Sleep(10500 * time.Millisecond)
			_, _ = src.Token(t.Context()) // fails again: one second of backoff
			time.Sleep(time.Second)
			_, _ = src.Token(t.Context())
			if n, _ := srv.counts(); n != 4 {
				t.Errorf("refresh requests = %d, want 4: the backoff after the success starts at one second again", n)
			}
		})
	})
	t.Run("SetTokens", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			srv := &scriptedServer{refresh: outage}
			src := scriptedSource(t, srv)
			src.SetTokens(staleAccess("at-0"), "rt-0")
			_, _ = src.Token(t.Context())

			src.SetTokens(staleAccess("at-1"), "rt-1")
			if tok, err := src.Token(t.Context()); err != nil || tok.Value != "at-1" {
				t.Errorf("Token() after SetTokens = (%q, %v), want (at-1, nil)", tok.Value, err)
			}
			if n, _ := srv.counts(); n != 2 {
				t.Errorf("refresh requests = %d, want 2: the tokens SetTokens installs are refreshed at once", n)
			}
		})
	})
}

// TestREQ063_ForcedRenewalIgnoresTheBackoff pins REQ-063: RefreshIfNeeded and
// Reauth send their request during a backoff and return its failure, and a
// token Reauth marked for renewal is not served in place of a failure.
func TestREQ063_ForcedRenewalIgnoresTheBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := &scriptedServer{refresh: outage}
		src := scriptedSource(t, srv)
		src.SetTokens(staleAccess("at-0"), "rt-0")
		wantRequests := func(step string, want int) {
			t.Helper()
			if n, _ := srv.counts(); n != want {
				t.Errorf("refresh requests after %s = %d, want %d", step, n, want)
			}
		}

		if tok, err := src.Token(t.Context()); err != nil || tok.Value != "at-0" {
			t.Fatalf("Token() = (%q, %v), want (at-0, nil)", tok.Value, err)
		}
		wantRequests("Token()", 1)

		if err := src.RefreshIfNeeded(t.Context()); !errors.Is(err, auth.ErrRefreshFailed) {
			t.Errorf("RefreshIfNeeded() during the backoff error = %v, want auth.ErrRefreshFailed", err)
		}
		wantRequests("RefreshIfNeeded()", 2)

		if tok, err := src.Token(t.Context()); err != nil || tok.Value != "at-0" {
			t.Errorf("Token() after RefreshIfNeeded() = (%q, %v), want (at-0, nil)", tok.Value, err)
		}
		wantRequests("a second Token()", 2)

		if err := src.Reauth(t.Context()); !errors.Is(err, auth.ErrRefreshFailed) {
			t.Errorf("Reauth() during the backoff error = %v, want auth.ErrRefreshFailed", err)
		}
		wantRequests("Reauth()", 3)

		if _, err := src.Token(t.Context()); !errors.Is(err, auth.ErrRefreshFailed) {
			t.Errorf("Token() after a failed Reauth() error = %v, want auth.ErrRefreshFailed: the token Reauth marked is not served", err)
		}
		wantRequests("Token() after Reauth()", 4)
	})
}

// TestREQ063_CoalescedRenewalSharesTheHeldToken pins REQ-063: the Token calls
// that share a refresh that fails transiently all get the held token, from
// one request, while RefreshIfNeeded waiting on the same refresh gets the
// failure.
func TestREQ063_CoalescedRenewalSharesTheHeldToken(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		srv := &scriptedServer{arrived: make(chan struct{}, 1)}
		srv.refresh = func(r *http.Request, n int) (*http.Response, error) {
			<-release
			return outage(r, n)
		}
		src := scriptedSource(t, srv)
		src.SetTokens(staleAccess("at-0"), "rt-0")

		var (
			wg               sync.WaitGroup
			leadTok, waitTok auth.Token
			leadErr, waitErr error
			forcedErr        error
		)
		wg.Go(func() { leadTok, leadErr = src.Token(t.Context()) })
		<-srv.arrived
		wg.Go(func() { waitTok, waitErr = src.Token(t.Context()) })
		wg.Go(func() { forcedErr = src.RefreshIfNeeded(t.Context()) })
		synctest.Wait() // both wait on the leader's refresh
		close(release)
		wg.Wait()

		if leadErr != nil || leadTok.Value != "at-0" {
			t.Errorf("leading Token() = (%q, %v), want (at-0, nil)", leadTok.Value, leadErr)
		}
		if waitErr != nil || waitTok.Value != "at-0" {
			t.Errorf("waiting Token() = (%q, %v), want (at-0, nil)", waitTok.Value, waitErr)
		}
		if !errors.Is(forcedErr, auth.ErrRefreshFailed) {
			t.Errorf("waiting RefreshIfNeeded() error = %v, want auth.ErrRefreshFailed", forcedErr)
		}
		if n, _ := srv.counts(); n != 1 {
			t.Errorf("refresh requests = %d, want 1 (one shared refresh)", n)
		}
	})
}

// TestREQ063_TerminalRefreshFailureIsNotServed pins REQ-063: a terminal
// failure surfaces as auth.ErrReauthRequired and drops both tokens even
// while the held access token is still valid.
func TestREQ063_TerminalRefreshFailureIsNotServed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := &scriptedServer{refresh: func(r *http.Request, _ int) (*http.Response, error) {
			return scriptedAnswer(r, http.StatusBadRequest, `{"error":"invalid_grant"}`), nil
		}}
		src := scriptedSource(t, srv)
		src.SetTokens(staleAccess("at-0"), "rt-0")

		if tok, err := src.Token(t.Context()); !errors.Is(err, auth.ErrReauthRequired) {
			t.Errorf("Token() = (%q, %v), want auth.ErrReauthRequired", tok.Value, err)
		}
		if access, refresh := src.HeldTokens(); !access.IsZero() || refresh != "" {
			t.Errorf("held tokens = (%q, %q), want none after a terminal failure", access.Value, refresh)
		}
	})
}
