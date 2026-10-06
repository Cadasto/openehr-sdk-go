package clientcreds_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/clientcreds"
	"github.com/cadasto/openehr-sdk-go/auth/jwtbearer"
)

// redirectTarget is the second server a token endpoint redirects to. It
// records what reaches it: the method, the Authorization header and the
// body of every request.
type redirectTarget struct {
	srv  *httptest.Server
	mu   sync.Mutex
	reqs []string
}

func newRedirectTarget(t *testing.T) *redirectTarget {
	t.Helper()
	rt := &redirectTarget{}
	rt.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rt.mu.Lock()
		rt.reqs = append(rt.reqs, fmt.Sprintf("%s authorization=%q body=%q", r.Method, r.Header.Get("Authorization"), body))
		rt.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"stolen","token_type":"Bearer","expires_in":3600}`))
	}))
	t.Cleanup(rt.srv.Close)
	return rt
}

// received returns what the target has received.
func (rt *redirectTarget) received() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]string(nil), rt.reqs...)
}

// newRedirector returns a stub token endpoint that answers every request
// with status and a Location header that names to, and counts the requests
// it receives.
func newRedirector(t *testing.T, status int, to string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Location", to)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// TestREQ060_TokenPostDoesNotFollowRedirect pins REQ-060 (§ Credential-bearing
// requests): the token request of auth/clientcreds refuses a 3xx answer, whatever redirect policy the injected client has
// and whichever way the client authenticates, so a 307 or a 308 cannot
// carry the client secret or the client assertion to another server. The
// 3xx answer fails as an *auth.ExchangeError, and the injected client is
// left as it was.
func TestREQ060_TokenPostDoesNotFollowRedirect(t *testing.T) {
	auths := []struct {
		name   string
		secret string
		opts   []clientcreds.Option
	}{
		{"client_secret_basic", "client-secret", []clientcreds.Option{clientcreds.WithAuthMethod(clientcreds.AuthBasic)}},
		{"client_secret_post", "client-secret", []clientcreds.Option{clientcreds.WithAuthMethod(clientcreds.AuthPost)}},
		{"private_key_jwt", "", []clientcreds.Option{clientcreds.WithClientAssertion(jwtbearer.StaticAssertion("signed.client.assertion"))}},
	}
	clients := []struct {
		name   string
		follow bool // the injected client has a CheckRedirect that follows
	}{
		{"default redirect policy", false},
		{"caller policy follows", true},
	}
	for _, au := range auths {
		for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
			for _, cl := range clients {
				t.Run(fmt.Sprintf("%s/%d/%s", au.name, status, cl.name), func(t *testing.T) {
					target := newRedirectTarget(t)
					as, asHits := newRedirector(t, status, target.srv.URL+"/leak")

					hc := &http.Client{Transport: as.Client().Transport}
					var callerPolicyCalls atomic.Int32
					if cl.follow {
						hc.CheckRedirect = func(*http.Request, []*http.Request) error {
							callerPolicyCalls.Add(1)
							return nil
						}
					}
					src, err := clientcreds.New("client-id", au.secret, as.URL+"/token",
						append([]clientcreds.Option{clientcreds.WithHTTPClient(hc)}, au.opts...)...)
					if err != nil {
						t.Fatalf("clientcreds.New: %v", err)
					}

					_, err = src.Token(t.Context())

					if got := target.received(); len(got) != 0 {
						t.Errorf("the redirect target received %d request(s), want none: %q", len(got), got)
					}
					if ee, ok := errors.AsType[*auth.ExchangeError](err); !ok || ee == nil {
						t.Errorf("Token() error = %v, want an *auth.ExchangeError", err)
					} else if ee.StatusCode != status {
						t.Errorf("ExchangeError.StatusCode = %d, want %d", ee.StatusCode, status)
					}
					if !errors.Is(err, auth.ErrTokenExchangeFailed) {
						t.Errorf("Token() error = %v, want it to match auth.ErrTokenExchangeFailed", err)
					}
					if got := asHits.Load(); got != 1 {
						t.Errorf("the endpoint received %d request(s), want 1", got)
					}

					// The injected client is as the caller left it.
					if got := callerPolicyCalls.Load(); got != 0 {
						t.Errorf("the caller's CheckRedirect ran %d time(s) during the call, want 0", got)
					}
					switch {
					case cl.follow:
						if hc.CheckRedirect == nil {
							t.Fatal("the injected client lost its CheckRedirect")
						}
						_ = hc.CheckRedirect(nil, nil)
						if got := callerPolicyCalls.Load(); got != 1 {
							t.Errorf("the injected client's CheckRedirect is no longer the caller's: it ran the caller's policy %d time(s), want 1", got)
						}
					case hc.CheckRedirect != nil:
						t.Error("the injected client gained a CheckRedirect")
					}
				})
			}
		}
	}
}
