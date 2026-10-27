package jwtbearer_test

import (
	"errors"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// TestREQ063_ZeroLifetimeTokenRenewsOncePerCall pins REQ-063 (auth.md §
// Token refresh): a token whose expires_in was 0 is past its ExpiresAt at
// once, so the transient-failure backoff never serves it in place of a
// failure. Each Token call that finds it sends one exchange and returns that
// exchange's failure, and no request is sent between calls.
func TestREQ063_ZeroLifetimeTokenRenewsOncePerCall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rt := &scriptedTransport{answer: func(r *http.Request, n int) (*http.Response, error) {
			if n == 1 {
				return tokenAnswer(r, http.StatusOK, expiresInBody("tok", "0")), nil
			}
			return tokenAnswer(r, http.StatusServiceUnavailable, `{"error":"temporarily_unavailable"}`), nil
		}}
		src := newSource(t, "https://idp.test/token", &http.Client{Transport: rt})

		if tok, err := src.Token(t.Context()); err != nil || tok.Value != "tok" {
			t.Fatalf("Token() = (%q, %v), want (tok, nil): the call that obtained the token gets it", tok.Value, err)
		}
		for call := int32(2); call <= 4; call++ {
			if tok, err := src.Token(t.Context()); !errors.Is(err, auth.ErrTokenExchangeFailed) || tok.Value != "" {
				t.Fatalf("Token() call %d = (%q, %v), want no token and an error matching auth.ErrTokenExchangeFailed", call, tok.Value, err)
			}
			if got := rt.posts.Load(); got != call {
				t.Fatalf("after Token() call %d: token-endpoint requests = %d, want %d: one per call", call, got, call)
			}
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if got := rt.posts.Load(); got != 4 {
			t.Errorf("token-endpoint requests a minute after the last call = %d, want 4: none without a call", got)
		}
	})
}
