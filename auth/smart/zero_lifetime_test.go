package smart_test

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
// failure. Each Token call that finds it sends one refresh request and
// returns that refresh's failure, and no request is sent between calls.
func TestREQ063_ZeroLifetimeTokenRenewsOncePerCall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := &scriptedServer{refresh: func(r *http.Request, n int) (*http.Response, error) {
			if n == 1 {
				return scriptedAnswer(r, http.StatusOK, expiresInBody("at-zero", "0")), nil
			}
			return scriptedAnswer(r, http.StatusServiceUnavailable, outageBody), nil
		}}
		src := scriptedSource(t, srv)
		src.SetTokens(auth.Token{Value: "at-old", Type: auth.TokenTypeBearer, ExpiresAt: time.Now().Add(-time.Minute)}, "rt-0")

		if tok, err := src.Token(t.Context()); err != nil || tok.Value != "at-zero" {
			t.Fatalf("Token() = (%q, %v), want (at-zero, nil): the call that obtained the token gets it", tok.Value, err)
		}
		for call := 2; call <= 4; call++ {
			if tok, err := src.Token(t.Context()); !errors.Is(err, auth.ErrRefreshFailed) || tok.Value != "" {
				t.Fatalf("Token() call %d = (%q, %v), want no token and an error matching auth.ErrRefreshFailed", call, tok.Value, err)
			}
			if n, _ := srv.counts(); n != call {
				t.Fatalf("after Token() call %d: refresh requests = %d, want %d: one per call", call, n, call)
			}
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if n, _ := srv.counts(); n != 4 {
			t.Errorf("refresh requests a minute after the last call = %d, want 4: none without a call", n)
		}
	})
}
