package smart_test

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
	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
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

// redirectBody is a well-formed token response that newRedirector sends
// with its 3xx, so the answer cannot fail on its body and only the status
// check can refuse it (REQ-060).
const redirectBody = `{"access_token":"from-a-3xx","token_type":"Bearer","expires_in":3600}`

// newRedirector returns a stub authorization server whose every endpoint
// answers with status, a Location header that names to and redirectBody, and
// counts the requests it receives.
func newRedirector(t *testing.T, status int, to string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Location", to)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(redirectBody))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// TestREQ060_TokenPostDoesNotFollowRedirect pins REQ-060 (§ Credential-bearing
// requests), REQ-063 and REQ-167: the code exchange, the refresh and the
// revocation of auth/smart refuse a 3xx answer, whatever redirect policy
// the injected client has, so a 307 or a 308 cannot carry the authorization
// code, the refresh token or the client secret to another server. The 3xx
// answer fails as an *auth.ExchangeError of the request's own class, and
// the injected client is left as it was.
func TestREQ060_TokenPostDoesNotFollowRedirect(t *testing.T) {
	kinds := []struct {
		name     string
		sentinel error
		run      func(t *testing.T, src *smart.Source) error
	}{
		{"code exchange", auth.ErrTokenExchangeFailed, func(t *testing.T, src *smart.Source) error {
			t.Helper()
			req, err := src.BeginAuthorization("")
			if err != nil {
				t.Fatalf("BeginAuthorization: %v", err)
			}
			_, _, err = src.ExchangeAuthorizationCode(t.Context(), "code-secret", req.State, req)
			return err
		}},
		{"refresh", auth.ErrRefreshFailed, func(t *testing.T, src *smart.Source) error {
			t.Helper()
			src.SetTokens(auth.Token{Value: "at-0"}, "rt-secret")
			return src.Reauth(t.Context())
		}},
		{"revocation", auth.ErrRevocationFailed, func(t *testing.T, src *smart.Source) error {
			t.Helper()
			src.SetTokens(auth.Token{Value: "at-0"}, "rt-secret")
			return src.Revoke(t.Context())
		}},
	}
	clients := []struct {
		name   string
		follow bool // the injected client has a CheckRedirect that follows
	}{
		{"default redirect policy", false},
		{"caller policy follows", true},
	}
	for _, kind := range kinds {
		for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
			for _, cl := range clients {
				t.Run(fmt.Sprintf("%s/%d/%s", kind.name, status, cl.name), func(t *testing.T) {
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
					src, err := newSource("client-id", discovery.AuthEndpoints{
						AuthorizationEndpoint: discovery.MustParseURL(as.URL + "/authorize"),
						TokenEndpoint:         discovery.MustParseURL(as.URL + "/token"),
						RevocationEndpoint:    discovery.MustParseURL(as.URL + "/revoke"),
					},
						smart.WithHTTPClient(hc),
						smart.WithClientSecret("client-secret"),
						smart.WithRedirectURI("https://app.example/callback"),
						smart.WithIssuer(completeIssuer),
					)
					if err != nil {
						t.Fatalf("newSource: %v", err)
					}

					err = kind.run(t, src)

					if got := target.received(); len(got) != 0 {
						t.Errorf("%s: the redirect target received %d request(s), want none: %q", kind.name, len(got), got)
					}
					if ee, ok := errors.AsType[*auth.ExchangeError](err); !ok || ee == nil {
						t.Errorf("%s error = %v, want an *auth.ExchangeError", kind.name, err)
					} else if ee.StatusCode != status {
						t.Errorf("%s ExchangeError.StatusCode = %d, want %d", kind.name, ee.StatusCode, status)
					}
					if !errors.Is(err, kind.sentinel) {
						t.Errorf("%s error = %v, want it to match %v", kind.name, err, kind.sentinel)
					}
					if got := asHits.Load(); got != 1 {
						t.Errorf("%s: the endpoint received %d request(s), want 1", kind.name, got)
					}

					// The injected client is as the caller left it.
					if got := callerPolicyCalls.Load(); got != 0 {
						t.Errorf("%s: the caller's CheckRedirect ran %d time(s) during the call, want 0", kind.name, got)
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
