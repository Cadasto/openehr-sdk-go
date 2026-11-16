package jwtbearer_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/jwtbearer"
)

// scriptedTransport answers the n-th token request, counted from 1, with
// answer. Nothing goes over a network, so a synctest bubble sees every wait
// inside it.
type scriptedTransport struct {
	answer func(r *http.Request, n int) (*http.Response, error)
	posts  atomic.Int32
}

func (s *scriptedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Body != nil {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
	}
	return s.answer(r, int(s.posts.Add(1)))
}

// tokenAnswer is a JSON response to r.
func tokenAnswer(r *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    r,
	}
}

// tokenBody is the token response that grants "tok-<n>" for an hour.
func tokenBody(n int) string {
	return fmt.Sprintf(`{"access_token":"tok-%d","token_type":"Bearer","expires_in":3600}`, n)
}

// errCallerBug is the value a caller-supplied component panics with.
var errCallerBug = errors.New("caller bug")

// TestREQ026_ExchangePanicReleasesWaiters pins REQ-026 (idiom.md §
// Concurrency) with REQ-025: a panic in a shared exchange, from the HTTP
// client's RoundTripper or from the assertion source, goes up through the
// call that led it, unchanged, and does not leave the exchange in flight. The
// call waiting on it runs a new exchange instead of waiting out its deadline,
// and a later call gets that exchange's token.
func TestREQ026_ExchangePanicReleasesWaiters(t *testing.T) {
	for _, site := range []string{"transport", "assertion source"} {
		t.Run(site, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				arrived := make(chan struct{}, 1)
				release := make(chan struct{})
				var calls atomic.Int32
				// panicFirst holds the first call at the site under test until
				// release closes, then panics; later calls go through.
				panicFirst := func(at string) {
					if at == site && calls.Add(1) == 1 {
						arrived <- struct{}{}
						<-release
						panic(errCallerBug)
					}
				}
				rt := &scriptedTransport{answer: func(r *http.Request, n int) (*http.Response, error) {
					panicFirst("transport")
					return tokenAnswer(r, http.StatusOK, tokenBody(n)), nil
				}}
				assertion := jwtbearer.AssertionFunc(func(context.Context) (string, error) {
					panicFirst("assertion source")
					return "a.b.c", nil
				})
				src, err := jwtbearer.New("https://idp.test/token", assertion,
					jwtbearer.WithHTTPClient(&http.Client{Transport: rt}))
				if err != nil {
					t.Fatal(err)
				}

				waitCtx, cancelWait := context.WithTimeout(t.Context(), time.Minute)
				defer cancelWait()
				var (
					wg        sync.WaitGroup
					leadPanic any
					waitErr   error
					waitTok   auth.Token
				)
				wg.Go(func() {
					defer func() { leadPanic = recover() }()
					_, _ = src.Token(t.Context())
				})
				<-arrived
				wg.Go(func() { waitTok, waitErr = src.Token(waitCtx) })
				synctest.Wait() // the second call waits on the leader's exchange
				close(release)
				wg.Wait()

				// The transport counts the request that panicked in it; the
				// assertion source panics before any request is sent.
				posts, want := int32(1), "tok-1"
				if site == "transport" {
					posts, want = 2, "tok-2"
				}
				if leadPanic != errCallerBug { //nolint:errorlint // the panic value itself, as it was raised
					t.Errorf("leading Token() panicked with %v, want %v as it was raised", leadPanic, errCallerBug)
				}
				if waitErr != nil || waitTok.Value != want {
					t.Errorf("waiting Token() = (%q, %v), want (%s, nil) from a new exchange", waitTok.Value, waitErr, want)
				}
				if tok, err := src.Token(waitCtx); err != nil || tok.Value != want {
					t.Errorf("Token() after the panic = (%q, %v), want (%s, nil)", tok.Value, err, want)
				}
				if got := rt.posts.Load(); got != posts {
					t.Errorf("token-endpoint requests = %d, want %d", got, posts)
				}
			})
		})
	}
}

// TestREQ026_ExchangePanicDoesNotWedgeLaterCalls pins REQ-026 with REQ-025:
// a call that comes after a panicked exchange, with no call waiting on it,
// runs a new exchange instead of waiting on the abandoned one.
func TestREQ026_ExchangePanicDoesNotWedgeLaterCalls(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rt := &scriptedTransport{answer: func(r *http.Request, n int) (*http.Response, error) {
			if n == 1 {
				panic(errCallerBug)
			}
			return tokenAnswer(r, http.StatusOK, tokenBody(n)), nil
		}}
		src := newSource(t, "https://idp.test/token", &http.Client{Transport: rt})
		func() {
			defer func() { _ = recover() }()
			_, _ = src.Token(t.Context())
		}()

		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		if tok, err := src.Token(ctx); err != nil || tok.Value != "tok-2" {
			t.Errorf("Token() after a panicked exchange = (%q, %v), want (tok-2, nil)", tok.Value, err)
		}
	})
}

// TestREQ026_ExchangeFailureReachesWaitersAfterLeaderCancel pins REQ-026
// with REQ-063: an exchange that fails for a reason of its own reaches the
// calls waiting on it, even when the leader's context ended meanwhile. Only
// a failure the leader's ending caused stays the leader's.
func TestREQ026_ExchangeFailureReachesWaitersAfterLeaderCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		arrived := make(chan struct{}, 1)
		release := make(chan struct{})
		rt := &scriptedTransport{answer: func(r *http.Request, n int) (*http.Response, error) {
			select {
			case arrived <- struct{}{}:
			default:
			}
			<-release // answers without watching the request's context
			return tokenAnswer(r, http.StatusServiceUnavailable, `{"error":"temporarily_unavailable"}`), nil
		}}
		src := newSource(t, "https://idp.test/token", &http.Client{Transport: rt})

		leadCtx, cancelLead := context.WithCancel(t.Context())
		var (
			wg               sync.WaitGroup
			leadErr, waitErr error
		)
		wg.Go(func() { _, leadErr = src.Token(leadCtx) })
		<-arrived
		wg.Go(func() { _, waitErr = src.Token(t.Context()) })
		synctest.Wait()
		cancelLead()
		synctest.Wait() // the leader's context has ended; its request is still out
		close(release)
		wg.Wait()

		for _, c := range []struct {
			call string
			err  error
		}{{"leading Token()", leadErr}, {"waiting Token()", waitErr}} {
			ex, ok := errors.AsType[*auth.ExchangeError](c.err)
			if !ok || ex.StatusCode != http.StatusServiceUnavailable {
				t.Errorf("%s error = %v, want the 503 exchange failure", c.call, c.err)
			}
		}
		if got := rt.posts.Load(); got != 1 {
			t.Errorf("token-endpoint requests = %d, want 1 (one shared exchange)", got)
		}
	})
}

// errCallerGaveUp is the cause a leading call's context ends with.
var errCallerGaveUp = errors.New("caller gave up")

// heldTokenEndpoint is a token endpoint on the in-memory network that never
// answers: it holds each request until the client gives up on it. It is for
// tests inside a synctest bubble.
func heldTokenEndpoint(t *testing.T) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	// The server's own cleanup waits for its handlers; release them first.
	t.Cleanup(func() { close(release) })
	return srv
}

// TestREQ026_LeaderContextEndedWithACauseGetsItsContextError pins REQ-026
// (idiom.md § Concurrency) with REQ-063 for the call leading an exchange
// when its context ends with a cause, as context.WithCancelCause,
// context.WithTimeoutCause and errgroup end one. The HTTP client then
// reports the cause instead of the context's error. That call still gets an
// error that reports its context's error, and the cause and the exchange
// failure stay inside it.
func TestREQ026_LeaderContextEndedWithACauseGetsItsContextError(t *testing.T) {
	endings := []struct {
		name string
		// start returns the leading call's context and the function that ends
		// it with errCallerGaveUp while its request is held at the endpoint.
		start func(t *testing.T) (context.Context, func())
		// wantCtxErr is the error that context reports once it ended.
		wantCtxErr error
	}{
		{
			name: "cancelled with a cause",
			start: func(t *testing.T) (context.Context, func()) {
				ctx, cancel := context.WithCancelCause(t.Context())
				t.Cleanup(func() { cancel(nil) })
				return ctx, func() { cancel(errCallerGaveUp) }
			},
			wantCtxErr: context.Canceled,
		},
		{
			name: "deadline with a cause",
			start: func(t *testing.T) (context.Context, func()) {
				ctx, cancel := context.WithTimeoutCause(t.Context(), time.Minute, errCallerGaveUp)
				t.Cleanup(cancel)
				return ctx, func() { time.Sleep(2 * time.Minute) } // past the deadline
			},
			wantCtxErr: context.DeadlineExceeded,
		},
	}
	for _, ending := range endings {
		t.Run(ending.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				srv := heldTokenEndpoint(t)
				src := newSource(t, srv.URL, srv.Client())

				ctx, end := ending.start(t)
				var (
					wg  sync.WaitGroup
					err error
				)
				wg.Go(func() { _, err = src.Token(ctx) })
				synctest.Wait() // the exchange is held at the endpoint
				end()
				wg.Wait()

				if !errors.Is(err, ending.wantCtxErr) || !errors.Is(err, errCallerGaveUp) {
					t.Errorf("Token() error = %v, want one that reports both its context's error %v and the cause %v", err, ending.wantCtxErr, errCallerGaveUp)
				}
				if _, ok := errors.AsType[*auth.ExchangeError](err); !ok || !errors.Is(err, auth.ErrTokenExchangeFailed) {
					t.Errorf("Token() error = %v, want one that still carries the *auth.ExchangeError matching auth.ErrTokenExchangeFailed", err)
				}
			})
		})
	}
}
