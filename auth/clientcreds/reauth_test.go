package clientcreds_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

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
