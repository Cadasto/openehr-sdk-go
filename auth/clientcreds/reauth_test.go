package clientcreds_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/clientcreds"
)

// tokenEndpoint is a token endpoint that counts the POSTs it receives and
// answers the n-th one with the access token "tok-<n>", valid for an hour.
// While fail is set it answers 503 with an OAuth2 error body instead. When
// gate is non-nil, every POST after the first waits for gate to close.
type tokenEndpoint struct {
	posts atomic.Int32
	fail  atomic.Bool
	gate  chan struct{}
}

func (e *tokenEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := e.posts.Add(1)
	if e.gate != nil && n > 1 {
		<-e.gate
	}
	w.Header().Set("Content-Type", "application/json")
	if e.fail.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"temporarily_unavailable"}`))
		return
	}
	_, _ = fmt.Fprintf(w, `{"access_token":"tok-%d","token_type":"Bearer","expires_in":3600}`, n)
}

// TestReauthForcesFreshExchange — REQ-063: Reauth drops a cached token that
// is still fresh and obtains a new one with a second exchange; Token then
// returns the new token without a third exchange.
func TestReauthForcesFreshExchange(t *testing.T) { // REQ-063
	ep := &tokenEndpoint{}
	srv := httptest.NewServer(ep)
	defer srv.Close()

	src, err := clientcreds.New("c", "s", srv.URL, clientcreds.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	first, err := src.Token(t.Context())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if first.Value != "tok-1" {
		t.Fatalf("Token().Value = %q, want tok-1", first.Value)
	}

	if err := src.Reauth(t.Context()); err != nil {
		t.Fatalf("Reauth() error = %v, want nil", err)
	}
	if got := ep.posts.Load(); got != 2 {
		t.Fatalf("token-endpoint POSTs after Reauth = %d, want 2 (Reauth exchanges even though tok-1 is fresh)", got)
	}

	after, err := src.Token(t.Context())
	if err != nil {
		t.Fatalf("Token() after Reauth error = %v", err)
	}
	if after.Value != "tok-2" {
		t.Errorf("Token().Value after Reauth = %q, want tok-2", after.Value)
	}
	if got := ep.posts.Load(); got != 2 {
		t.Errorf("token-endpoint POSTs after Reauth then Token = %d, want 2 (Token reuses the new token)", got)
	}
}

// TestReauthCoalescesConcurrentCalls — REQ-063 with REQ-026: concurrent
// Reauth and Token calls share one exchange, so the token endpoint sees one
// POST beyond the one that primed the cache.
func TestReauthCoalescesConcurrentCalls(t *testing.T) { // REQ-063
	synctest.Test(t, func(t *testing.T) {
		ep := &tokenEndpoint{gate: make(chan struct{})}
		srv := httptest.NewTestServer(t, ep)
		// Client() starts the in-memory server, which is what populates
		// srv.URL, so it must be called before srv.URL is read.
		cli := srv.Client()
		src, err := clientcreds.New("c", "s", srv.URL, clientcreds.WithHTTPClient(cli))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := src.Token(t.Context()); err != nil {
			t.Fatalf("priming Token() error = %v", err)
		}

		const n = 8
		var wg sync.WaitGroup
		for range n {
			wg.Go(func() {
				if err := src.Reauth(t.Context()); err != nil {
					t.Errorf("Reauth() error = %v", err)
				}
			})
			wg.Go(func() {
				if _, err := src.Token(t.Context()); err != nil {
					t.Errorf("Token() error = %v", err)
				}
			})
		}
		synctest.Wait()
		close(ep.gate)
		wg.Wait()

		if got := ep.posts.Load(); got != 2 {
			t.Errorf("token-endpoint POSTs = %d, want 2 (one to prime, one shared by %d Reauth calls)", got, n)
		}
		tok, err := src.Token(t.Context())
		if err != nil {
			t.Fatalf("Token() after Reauth error = %v", err)
		}
		if tok.Value != "tok-2" {
			t.Errorf("Token().Value after Reauth = %q, want tok-2", tok.Value)
		}
	})
}

// TestReauthReturnsExchangeError — REQ-063: a failed Reauth exchange returns
// the *auth.ExchangeError wrapping auth.ErrTokenExchangeFailed, and the next
// Token call tries a new exchange rather than returning the dropped token.
func TestReauthReturnsExchangeError(t *testing.T) { // REQ-063
	ep := &tokenEndpoint{}
	srv := httptest.NewServer(ep)
	defer srv.Close()

	src, err := clientcreds.New("c", "s", srv.URL, clientcreds.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.Token(t.Context()); err != nil {
		t.Fatalf("priming Token() error = %v", err)
	}

	ep.fail.Store(true)
	err = src.Reauth(t.Context())
	if !errors.Is(err, auth.ErrTokenExchangeFailed) {
		t.Fatalf("Reauth() error = %v, want auth.ErrTokenExchangeFailed", err)
	}
	ee, ok := errors.AsType[*auth.ExchangeError](err)
	if !ok || ee == nil {
		t.Fatalf("Reauth() error = %T, want *auth.ExchangeError", err)
	}
	if ee.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("ExchangeError.StatusCode = %d, want %d", ee.StatusCode, http.StatusServiceUnavailable)
	}

	ep.fail.Store(false)
	tok, err := src.Token(t.Context())
	if err != nil {
		t.Fatalf("Token() after failed Reauth error = %v", err)
	}
	if tok.Value != "tok-3" {
		t.Errorf("Token().Value after failed Reauth = %q, want tok-3 (a new exchange, not the dropped tok-1)", tok.Value)
	}
	if got := ep.posts.Load(); got != 3 {
		t.Errorf("token-endpoint POSTs = %d, want 3 (prime, failed Reauth, retry)", got)
	}
}

func newSource(t *testing.T, tokenURL string, cli *http.Client) *clientcreds.Source {
	t.Helper()
	src, err := clientcreds.New("c", "s", tokenURL, clientcreds.WithHTTPClient(cli))
	if err != nil {
		t.Fatal(err)
	}
	return src
}

// TestReauthJoinersGetTheExchangeError — REQ-063 with REQ-026: a Reauth and
// a Token that join a Reauth exchange already in flight get that exchange's
// error, and the token endpoint sees one POST for all three calls.
func TestReauthJoinersGetTheExchangeError(t *testing.T) { // REQ-063
	synctest.Test(t, func(t *testing.T) {
		ep := &tokenEndpoint{gate: make(chan struct{})}
		srv := httptest.NewTestServer(t, ep)
		src := newSource(t, srv.URL, srv.Client())
		if _, err := src.Token(t.Context()); err != nil {
			t.Fatalf("priming Token() error = %v", err)
		}
		ep.fail.Store(true)

		var (
			wg                           sync.WaitGroup
			leadErr, reauthErr, tokenErr error
		)
		wg.Go(func() { leadErr = src.Reauth(t.Context()) })
		synctest.Wait() // the leading exchange is held at the endpoint
		wg.Go(func() { reauthErr = src.Reauth(t.Context()) })
		wg.Go(func() { _, tokenErr = src.Token(t.Context()) })
		synctest.Wait() // both joiners wait on the leading exchange
		close(ep.gate)
		wg.Wait()

		for _, c := range []struct {
			call string
			err  error
		}{
			{call: "leading Reauth", err: leadErr},
			{call: "joining Reauth", err: reauthErr},
			{call: "joining Token", err: tokenErr},
		} {
			ee, ok := errors.AsType[*auth.ExchangeError](c.err)
			if !ok || ee == nil || ee.StatusCode != http.StatusServiceUnavailable {
				t.Errorf("%s error = %v, want *auth.ExchangeError with status 503", c.call, c.err)
			}
		}
		if got := ep.posts.Load(); got != 2 {
			t.Errorf("token-endpoint POSTs = %d, want 2 (one to prime, one shared by all three calls)", got)
		}
	})
}

// joinerCall runs the call a test's joining goroutine makes, Token or Reauth.
func joinerCall(ctx context.Context, src *clientcreds.Source, call string) (auth.Token, error) {
	if call == "Reauth" {
		return auth.Token{}, src.Reauth(ctx)
	}
	return src.Token(ctx)
}

// TestJoinerOutlivesLeaderCancellation — REQ-063 with REQ-026: when the
// caller leading an exchange gives up, a caller waiting on that exchange
// with a live context does not get the leader's cancellation; it runs a new
// exchange and gets the new token.
func TestJoinerOutlivesLeaderCancellation(t *testing.T) { // REQ-063
	for _, call := range []string{"Token", "Reauth"} {
		t.Run(call, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ep := &tokenEndpoint{gate: make(chan struct{})}
				srv := httptest.NewTestServer(t, ep)
				src := newSource(t, srv.URL, srv.Client())
				if _, err := src.Token(t.Context()); err != nil {
					t.Fatalf("priming Token() error = %v", err)
				}

				leadCtx, cancelLead := context.WithCancel(t.Context())
				var (
					wg               sync.WaitGroup
					leadErr, joinErr error
				)
				wg.Go(func() { leadErr = src.Reauth(leadCtx) })
				synctest.Wait() // the leading exchange is held at the endpoint
				wg.Go(func() { _, joinErr = joinerCall(t.Context(), src, call) })
				synctest.Wait() // the joiner waits on the leading exchange
				cancelLead()
				synctest.Wait() // the leader has given up; the joiner's own exchange is held
				close(ep.gate)
				wg.Wait()

				if !errors.Is(leadErr, context.Canceled) {
					t.Errorf("leading Reauth error = %v, want context.Canceled", leadErr)
				}
				if joinErr != nil {
					t.Errorf("joining %s error = %v, want nil (the leader's cancellation is not the joiner's)", call, joinErr)
				}
				if got := ep.posts.Load(); got != 3 {
					t.Errorf("token-endpoint POSTs = %d, want 3 (prime, the cancelled exchange, the joiner's new one)", got)
				}
				tok, err := src.Token(t.Context())
				if err != nil || tok.Value != "tok-3" {
					t.Errorf("Token() after the joiner's exchange = (%q, %v), want tok-3", tok.Value, err)
				}
			})
		})
	}
}

// TestJoinerReturnsItsOwnContextError — REQ-063: a caller
// waiting on another caller's exchange returns its own context's error as
// soon as that context ends, without waiting for the exchange to finish.
func TestJoinerReturnsItsOwnContextError(t *testing.T) { // REQ-063
	for _, call := range []string{"Token", "Reauth"} {
		t.Run(call, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ep := &tokenEndpoint{gate: make(chan struct{})}
				srv := httptest.NewTestServer(t, ep)
				src := newSource(t, srv.URL, srv.Client())
				if _, err := src.Token(t.Context()); err != nil {
					t.Fatalf("priming Token() error = %v", err)
				}

				joinCtx, cancelJoin := context.WithCancel(t.Context())
				joined := make(chan error, 1)
				var (
					wg      sync.WaitGroup
					leadErr error
				)
				wg.Go(func() { leadErr = src.Reauth(t.Context()) })
				synctest.Wait() // the leading exchange is held at the endpoint
				wg.Go(func() {
					_, err := joinerCall(joinCtx, src, call)
					joined <- err
				})
				synctest.Wait() // the joiner waits on the leading exchange
				cancelJoin()
				synctest.Wait()
				select {
				case err := <-joined:
					if !errors.Is(err, context.Canceled) {
						t.Errorf("joining %s error = %v, want context.Canceled", call, err)
					}
				default:
					t.Errorf("joining %s still waits on the exchange after its own context ended", call)
				}
				close(ep.gate)
				wg.Wait()

				if leadErr != nil {
					t.Errorf("leading Reauth error = %v, want nil", leadErr)
				}
				if got := ep.posts.Load(); got != 2 {
					t.Errorf("token-endpoint POSTs = %d, want 2 (prime, the leading exchange)", got)
				}
			})
		})
	}
}

// TestJoinerSharesAClientTimeout — REQ-063 with REQ-026: when the shared
// exchange fails on the HTTP client's own timeout, the leader's context is
// still live. That failure reads as context.DeadlineExceeded, yet it is not a
// caller giving up, so a waiting caller gets it rather than starting another
// exchange.
func TestJoinerSharesAClientTimeout(t *testing.T) { // REQ-063
	synctest.Test(t, func(t *testing.T) {
		ep := &tokenEndpoint{gate: make(chan struct{})}
		srv := httptest.NewTestServer(t, ep)
		cli := &http.Client{Transport: srv.Client().Transport, Timeout: time.Second}
		src := newSource(t, srv.URL, cli)
		if _, err := src.Token(t.Context()); err != nil {
			t.Fatalf("priming Token() error = %v", err)
		}

		var (
			wg               sync.WaitGroup
			leadErr, joinErr error
		)
		wg.Go(func() { leadErr = src.Reauth(t.Context()) })
		synctest.Wait() // the leading exchange is held at the endpoint
		wg.Go(func() { _, joinErr = src.Token(t.Context()) })
		synctest.Wait() // the joiner waits on the leading exchange
		wg.Wait()       // the fake clock runs to the client timeout
		close(ep.gate)

		for _, c := range []struct {
			call string
			err  error
		}{
			{call: "leading Reauth", err: leadErr},
			{call: "joining Token", err: joinErr},
		} {
			if !errors.Is(c.err, auth.ErrTokenExchangeFailed) || !errors.Is(c.err, context.DeadlineExceeded) {
				t.Errorf("%s error = %v, want auth.ErrTokenExchangeFailed from the client timeout", c.call, c.err)
			}
		}
		if got := ep.posts.Load(); got != 2 {
			t.Errorf("token-endpoint POSTs = %d, want 2 (prime, one shared exchange)", got)
		}
	})
}

// TestReauthWithEndedContextKeepsToken — REQ-063: a Reauth
// whose context has already ended returns that context's error and leaves
// the cached token in place, so the next Token call needs no exchange.
func TestReauthWithEndedContextKeepsToken(t *testing.T) { // REQ-063
	ep := &tokenEndpoint{}
	srv := httptest.NewServer(ep)
	defer srv.Close()

	src := newSource(t, srv.URL, srv.Client())
	if _, err := src.Token(t.Context()); err != nil {
		t.Fatalf("priming Token() error = %v", err)
	}
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	if err := src.Reauth(ended); !errors.Is(err, context.Canceled) {
		t.Fatalf("Reauth(ended context) error = %v, want context.Canceled", err)
	}
	tok, err := src.Token(t.Context())
	if err != nil || tok.Value != "tok-1" {
		t.Errorf("Token() after Reauth(ended context) = (%q, %v), want tok-1", tok.Value, err)
	}
	if got := ep.posts.Load(); got != 1 {
		t.Errorf("token-endpoint POSTs = %d, want 1 (the cached token stays)", got)
	}
}
