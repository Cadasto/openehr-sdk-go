package jwtbearer_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/jwtbearer"
)

// grantFor is a token response to r that grants "tok-<n>" for expiresIn
// seconds.
func grantFor(r *http.Request, n, expiresIn int) *http.Response {
	return tokenAnswer(r, http.StatusOK, fmt.Sprintf(`{"access_token":"tok-%d","token_type":"Bearer","expires_in":%d}`, n, expiresIn))
}

// outage answers a token request with a 503, a transient failure.
func outage(r *http.Request, _ int) (*http.Response, error) {
	return tokenAnswer(r, http.StatusServiceUnavailable, `{"error":"temporarily_unavailable"}`), nil
}

// nearExpirySource returns a source holding tok-1, which expires 10 seconds
// from now and so lies inside the default 30-second refresh threshold, and
// whose later token requests answer serves.
func nearExpirySource(t *testing.T, answer func(r *http.Request, n int) (*http.Response, error)) (*jwtbearer.Source, *scriptedTransport) {
	t.Helper()
	rt := &scriptedTransport{answer: func(r *http.Request, n int) (*http.Response, error) {
		if n == 1 {
			return grantFor(r, 1, 10), nil
		}
		return answer(r, n)
	}}
	src := newSource(t, "https://idp.test/token", &http.Client{Transport: rt})
	if tok, err := src.Token(t.Context()); err != nil || tok.Value != "tok-1" {
		t.Fatalf("first Token() = (%q, %v), want (tok-1, nil)", tok.Value, err)
	}
	return src, rt
}

// TestREQ063_TransientExchangeFailureServesTheHeldToken pins REQ-063
// (auth.md § REQ-063): when a renewal fails transiently while the held
// access token is not yet past its ExpiresAt, Token returns that token
// without an error, and the calls that follow within the backoff send no
// request.
func TestREQ063_TransientExchangeFailureServesTheHeldToken(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		src, rt := nearExpirySource(t, outage)

		for i := range 10 {
			tok, err := src.Token(t.Context())
			if err != nil || tok.Value != "tok-1" {
				t.Errorf("Token() call %d = (%q, %v), want (tok-1, nil): the held token is still valid", i+1, tok.Value, err)
			}
		}
		if got := rt.posts.Load(); got != 2 {
			t.Errorf("token-endpoint requests = %d, want 2 (the grant, one failed renewal)", got)
		}
	})
}

// TestREQ063_ExchangeBackoffDoublesUntilExpiry pins REQ-063: the backoff
// after a transient failure is one second, twice as long after each further
// failure in a row, and ends no later than the held token's ExpiresAt. Past
// ExpiresAt the failure surfaces as auth.ErrTokenExchangeFailed on every
// call.
func TestREQ063_ExchangeBackoffDoublesUntilExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		src, rt := nearExpirySource(t, outage) // tok-1 expires 10s from now

		// Each step sleeps, then calls Token once. Failures at 0s, 1s, 3s and
		// 7s; the fourth backoff of 8s is cut to the expiry at 10s.
		steps := []struct {
			sleep    time.Duration
			requests int32
			expired  bool
		}{
			{sleep: 0, requests: 2},
			{sleep: 999 * time.Millisecond, requests: 2},
			{sleep: time.Millisecond, requests: 3},
			{sleep: 1999 * time.Millisecond, requests: 3},
			{sleep: time.Millisecond, requests: 4},
			{sleep: 3999 * time.Millisecond, requests: 4},
			{sleep: time.Millisecond, requests: 5},
			{sleep: 2999 * time.Millisecond, requests: 5},
			{sleep: time.Millisecond, requests: 6, expired: true},
			{sleep: 0, requests: 7, expired: true},
		}
		start := time.Now()
		for _, step := range steps {
			time.Sleep(step.sleep)
			at := time.Since(start)
			tok, err := src.Token(t.Context())
			if step.expired {
				if !errors.Is(err, auth.ErrTokenExchangeFailed) {
					t.Errorf("Token() at %v error = %v, want auth.ErrTokenExchangeFailed: the held token has expired", at, err)
				}
			} else if err != nil || tok.Value != "tok-1" {
				t.Errorf("Token() at %v = (%q, %v), want (tok-1, nil)", at, tok.Value, err)
			}
			if got := rt.posts.Load(); got != step.requests {
				t.Errorf("token-endpoint requests at %v = %d, want %d", at, got, step.requests)
			}
		}
	})
}

// TestREQ063_NewTokenEndsTheExchangeBackoff pins REQ-063: a successful
// exchange ends the backoff, so the next transient failure waits one second
// again.
func TestREQ063_NewTokenEndsTheExchangeBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		src, rt := nearExpirySource(t, func(r *http.Request, n int) (*http.Response, error) {
			if n == 3 {
				// Inside the 30-second threshold again after 10 seconds.
				return grantFor(r, n, 40), nil
			}
			return outage(r, n)
		})

		_, _ = src.Token(t.Context()) // fails: one second of backoff, then two
		time.Sleep(time.Second)
		if tok, err := src.Token(t.Context()); err != nil || tok.Value != "tok-3" {
			t.Fatalf("Token() after the backoff = (%q, %v), want (tok-3, nil)", tok.Value, err)
		}
		time.Sleep(10500 * time.Millisecond)
		_, _ = src.Token(t.Context()) // fails again: one second of backoff
		time.Sleep(time.Second)
		_, _ = src.Token(t.Context())
		if got := rt.posts.Load(); got != 5 {
			t.Errorf("token-endpoint requests = %d, want 5: the backoff after the success starts at one second again", got)
		}
	})
}

// TestREQ063_ReauthIgnoresTheExchangeBackoff pins REQ-063: Reauth sends its
// request during a backoff and returns its failure. It has dropped the held
// token, so the next Token call tries again and returns the failure too.
func TestREQ063_ReauthIgnoresTheExchangeBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		src, rt := nearExpirySource(t, outage)
		if tok, err := src.Token(t.Context()); err != nil || tok.Value != "tok-1" {
			t.Fatalf("Token() = (%q, %v), want (tok-1, nil)", tok.Value, err)
		}

		if err := src.Reauth(t.Context()); !errors.Is(err, auth.ErrTokenExchangeFailed) {
			t.Errorf("Reauth() during the backoff error = %v, want auth.ErrTokenExchangeFailed", err)
		}
		if got := rt.posts.Load(); got != 3 {
			t.Errorf("token-endpoint requests after Reauth() = %d, want 3", got)
		}
		if _, err := src.Token(t.Context()); !errors.Is(err, auth.ErrTokenExchangeFailed) {
			t.Errorf("Token() after a failed Reauth() error = %v, want auth.ErrTokenExchangeFailed", err)
		}
		if got := rt.posts.Load(); got != 4 {
			t.Errorf("token-endpoint requests after Token() = %d, want 4", got)
		}
	})
}

// TestREQ063_CoalescedExchangeSharesTheHeldToken pins REQ-063: the Token
// calls that share an exchange that fails transiently all get the held
// token, from one request.
func TestREQ063_CoalescedExchangeSharesTheHeldToken(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		arrived := make(chan struct{}, 1)
		release := make(chan struct{})
		src, rt := nearExpirySource(t, func(r *http.Request, n int) (*http.Response, error) {
			arrived <- struct{}{}
			<-release
			return outage(r, n)
		})

		var (
			wg               sync.WaitGroup
			leadTok, waitTok auth.Token
			leadErr, waitErr error
		)
		wg.Go(func() { leadTok, leadErr = src.Token(t.Context()) })
		<-arrived
		wg.Go(func() { waitTok, waitErr = src.Token(t.Context()) })
		synctest.Wait() // the second call waits on the leader's exchange
		close(release)
		wg.Wait()

		if leadErr != nil || leadTok.Value != "tok-1" {
			t.Errorf("leading Token() = (%q, %v), want (tok-1, nil)", leadTok.Value, leadErr)
		}
		if waitErr != nil || waitTok.Value != "tok-1" {
			t.Errorf("waiting Token() = (%q, %v), want (tok-1, nil)", waitTok.Value, waitErr)
		}
		if got := rt.posts.Load(); got != 2 {
			t.Errorf("token-endpoint requests = %d, want 2 (the grant, one shared renewal)", got)
		}
	})
}

// TestREQ063_TerminalExchangeFailureIsNotServed pins REQ-063: a terminal
// failure, here invalid_grant, is returned even while the cached token is
// still valid, and starts no backoff, so the next call tries again.
func TestREQ063_TerminalExchangeFailureIsNotServed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		src, rt := nearExpirySource(t, func(r *http.Request, _ int) (*http.Response, error) {
			return tokenAnswer(r, http.StatusBadRequest, `{"error":"invalid_grant"}`), nil
		})

		for i := range 2 {
			_, err := src.Token(t.Context())
			exErr, ok := errors.AsType[*auth.ExchangeError](err)
			if !ok || !exErr.Terminal() || !errors.Is(err, auth.ErrTokenExchangeFailed) {
				t.Errorf("Token() call %d error = %v, want the terminal invalid_grant failure", i+1, err)
			}
		}
		if got := rt.posts.Load(); got != 3 {
			t.Errorf("token-endpoint requests = %d, want 3 (the grant, one per call)", got)
		}
	})
}

// TestREQ063_ExchangeEndedByItsCallerStartsNoBackoff pins REQ-063 with
// REQ-026: an exchange that failed only because the context of the call
// running it ended is that call's own failure. The call gets its context's
// error, not the cached token, and the next call exchanges at once.
func TestREQ063_ExchangeEndedByItsCallerStartsNoBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		arrived := make(chan struct{}, 1)
		src, rt := nearExpirySource(t, func(r *http.Request, n int) (*http.Response, error) {
			if n == 2 {
				arrived <- struct{}{}
				<-r.Context().Done()
				return nil, r.Context().Err()
			}
			return grantFor(r, n, 3600), nil
		})

		ctx, cancel := context.WithCancel(t.Context())
		leadErr := make(chan error, 1)
		go func() {
			_, err := src.Token(ctx)
			leadErr <- err
		}()
		<-arrived
		cancel()
		if err := <-leadErr; !errors.Is(err, context.Canceled) {
			t.Errorf("Token() whose context was cancelled error = %v, want context.Canceled", err)
		}
		if tok, err := src.Token(t.Context()); err != nil || tok.Value != "tok-3" {
			t.Errorf("next Token() = (%q, %v), want (tok-3, nil) from a new exchange", tok.Value, err)
		}
		if got := rt.posts.Load(); got != 3 {
			t.Errorf("token-endpoint requests = %d, want 3", got)
		}
	})
}
