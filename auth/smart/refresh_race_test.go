package smart_test

import (
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	gojose "github.com/go-jose/go-jose/v4"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// heldResponse is what the held refresh grant answers once released.
type heldResponse struct {
	status int
	body   string
}

// heldRefreshTransport is an in-memory authorization server. It holds the
// first refresh_token grant until the test releases it, answers later
// refresh grants and every authorization_code grant at once, and serves
// one signing key at /jwks. Every wait is on a channel, so a synctest
// bubble sees it as durably blocked.
type heldRefreshTransport struct {
	signer  *rsa.PrivateKey
	jwks    []byte
	arrived chan struct{}     // receives once, when the first refresh grant arrives
	release chan heldResponse // answers that first refresh grant

	mu           sync.Mutex
	codeBody     string
	laterRefresh string // body for every refresh grant after the first
	refreshForms []url.Values
}

func newHeldRefreshTransport(t *testing.T, key *rsa.PrivateKey) *heldRefreshTransport {
	t.Helper()
	set := gojose.JSONWebKeySet{Keys: []gojose.JSONWebKey{{Key: &key.PublicKey, KeyID: oidcKid, Algorithm: "RS256", Use: "sig"}}}
	body, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	return &heldRefreshTransport{signer: key, jwks: body, arrived: make(chan struct{}, 1), release: make(chan heldResponse)}
}

func (h *heldRefreshTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	respond := func(status int, body string) *http.Response {
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}
	}
	var form url.Values
	if r.Body != nil {
		raw, err := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if err != nil {
			return nil, err
		}
		if form, err = url.ParseQuery(string(raw)); err != nil {
			return nil, err
		}
	}
	switch {
	case r.URL.Path == "/jwks":
		return respond(http.StatusOK, string(h.jwks)), nil
	case form.Get("grant_type") == "authorization_code":
		h.mu.Lock()
		body := h.codeBody
		h.mu.Unlock()
		return respond(http.StatusOK, body), nil
	case form.Get("grant_type") == "refresh_token":
		h.mu.Lock()
		h.refreshForms = append(h.refreshForms, form)
		first := len(h.refreshForms) == 1
		later := h.laterRefresh
		h.mu.Unlock()
		if !first {
			return respond(http.StatusOK, later), nil
		}
		h.arrived <- struct{}{}
		select {
		case resp := <-h.release:
			return respond(resp.status, resp.body), nil
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
	}
	return respond(http.StatusNotFound, `{"error":"not_found"}`), nil
}

// refreshes returns the refresh grants the server has received.
func (h *heldRefreshTransport) refreshes() []url.Values {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]url.Values(nil), h.refreshForms...)
}

type tokenResult struct {
	tok auth.Token
	err error
}

// TestRefreshStartedBeforeANewSessionIsDiscarded pins REQ-064: a refresh
// that started before a code exchange or SetTokens replaced the session has
// its result discarded, success or failure. The caller that led the refresh
// and a caller waiting on it both get the new session's token, the source
// holds the new session's tokens and token response, and nothing from the
// old session's refresh (tokens, refresh token, launch context or a
// terminal clear) lands on the new session.
func TestRefreshStartedBeforeANewSessionIsDiscarded(t *testing.T) { // REQ-064
	key := newRSAKey(t)
	const oldSessionBody = `{"access_token":"A-2","token_type":"Bearer","expires_in":3600,"refresh_token":"rt-A2","patient":"P1"}`

	tests := []struct {
		name string
		held heldResponse
		// replace installs the new session while the old refresh is held.
		replace       func(t *testing.T, h *heldRefreshTransport, src *smart.Source)
		laterRefresh  string // what a refresh of the new session answers
		wantAccess    string // the token both callers get and the source holds
		wantRefresh   string
		wantRefreshes int    // refresh grants in all
		wantLastAcc   string // LastTokenResponse().AccessToken
		wantSubject   string // LastTokenResponse().IDTokenClaims.Subject; empty means no claims
	}{
		{
			name:          "code exchange, old refresh succeeds",
			held:          heldResponse{http.StatusOK, oldSessionBody},
			replace:       exchangeUserB,
			wantAccess:    "B-1",
			wantRefreshes: 1,
			wantLastAcc:   "B-1",
			wantSubject:   "user-B",
		},
		{
			name:          "code exchange, old refresh is refused as invalid_grant",
			held:          heldResponse{http.StatusBadRequest, `{"error":"invalid_grant"}`},
			replace:       exchangeUserB,
			wantAccess:    "B-1",
			wantRefreshes: 1,
			wantLastAcc:   "B-1",
			wantSubject:   "user-B",
		},
		{
			name: "SetTokens, old refresh succeeds",
			held: heldResponse{http.StatusOK, oldSessionBody},
			replace: func(_ *testing.T, _ *heldRefreshTransport, src *smart.Source) {
				src.SetTokens(freshAccess("C-1"), "rt-C")
			},
			wantAccess:    "C-1",
			wantRefresh:   "rt-C",
			wantRefreshes: 1,
		},
		{
			name: "SetTokens with a stale token, which the retry refreshes",
			held: heldResponse{http.StatusOK, oldSessionBody},
			replace: func(_ *testing.T, _ *heldRefreshTransport, src *smart.Source) {
				src.SetTokens(staleAccess("C-1"), "rt-C")
			},
			laterRefresh:  `{"access_token":"C-2","token_type":"Bearer","expires_in":3600,"refresh_token":"rt-C2"}`,
			wantAccess:    "C-2",
			wantRefresh:   "rt-C2",
			wantRefreshes: 2,
			wantLastAcc:   "C-2",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newHeldRefreshTransport(t, key)
				h.laterRefresh = tc.laterRefresh
				src, err := newSource("client-id", discovery.AuthEndpoints{
					AuthorizationEndpoint: discovery.MustParseURL("https://idp.test/authorize"),
					TokenEndpoint:         discovery.MustParseURL("https://idp.test/token"),
					JWKSURI:               discovery.MustParseURL("https://idp.test/jwks"),
				},
					smart.WithHTTPClient(&http.Client{Transport: h}),
					smart.WithRedirectURI("https://app.example/callback"),
					smart.WithIssuer(oidcIssuer),
					smart.WithScopes("openid"),
				)
				if err != nil {
					t.Fatalf("newSource: %v", err)
				}
				src.SetTokens(staleAccess("A-1"), "rt-A")

				leader := make(chan tokenResult, 1)
				go func() {
					tok, err := src.Token(t.Context())
					leader <- tokenResult{tok, err}
				}()
				<-h.arrived // the old session's refresh is now held at the server

				waiter := make(chan tokenResult, 1)
				go func() {
					tok, err := src.Token(t.Context())
					waiter <- tokenResult{tok, err}
				}()
				synctest.Wait() // the waiter is parked on the held refresh

				tc.replace(t, h, src)
				h.release <- tc.held

				for name, ch := range map[string]chan tokenResult{"leading caller": leader, "waiting caller": waiter} {
					r := <-ch
					if r.err != nil || r.tok.Value != tc.wantAccess {
						t.Errorf("%s: Token() = %q, %v; want the new session's %q", name, r.tok.Value, r.err, tc.wantAccess)
					}
				}
				if access, refresh := src.HeldTokens(); access.Value != tc.wantAccess || refresh != tc.wantRefresh {
					t.Errorf("held tokens = %q, %q; want the new session's %q, %q", access.Value, refresh, tc.wantAccess, tc.wantRefresh)
				}
				if tok, err := src.Token(t.Context()); err != nil || tok.Value != tc.wantAccess {
					t.Errorf("Token() afterwards = %q, %v; want %q", tok.Value, err, tc.wantAccess)
				}
				if n := len(h.refreshes()); n != tc.wantRefreshes {
					t.Errorf("refresh grants = %d, want %d", n, tc.wantRefreshes)
				}
				last := src.LastTokenResponse()
				if last.AccessToken != tc.wantLastAcc || last.Patient == "P1" {
					t.Errorf("LastTokenResponse() access_token = %q, patient = %q; want %q and nothing from the old session's refresh", last.AccessToken, last.Patient, tc.wantLastAcc)
				}
				switch c := last.IDTokenClaims; {
				case tc.wantSubject == "" && c != nil:
					t.Errorf("LastTokenResponse().IDTokenClaims = %+v, want none", c)
				case tc.wantSubject != "" && (c == nil || c.Subject != tc.wantSubject):
					t.Errorf("LastTokenResponse().IDTokenClaims = %+v, want subject %q", c, tc.wantSubject)
				}
			})
		})
	}
}

// exchangeUserB completes a code exchange for user B whose response has an
// ID token and no refresh token.
func exchangeUserB(t *testing.T, h *heldRefreshTransport, src *smart.Source) {
	t.Helper()
	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	c := idClaims(req.Nonce)
	c["sub"] = "user-B"
	h.mu.Lock()
	h.codeBody = tokenBody(t, "B-1", "", joseSign(t, gojose.RS256, h.signer, oidcKid, c))
	h.mu.Unlock()
	if _, _, err := src.ExchangeAuthorizationCode(t.Context(), "code-B", req.State, req); err != nil {
		t.Fatalf("ExchangeAuthorizationCode(session B) error = %v, want success", err)
	}
}
