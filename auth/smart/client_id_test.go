package smart_test

import (
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// TestTokenRequestClientIDByClientType pins which token requests carry the
// client_id form field (REQ-068, and the token exchange of REQ-061). A public
// client sends it on the code exchange and on the refresh. A confidential
// client authenticates instead and leaves it out, except as one half of the
// client_secret_post credential, where client_id and client_secret both go
// in the form body. Each case also pins how the client authenticates, so a
// case cannot pass by sending no credential at all.
func TestTokenRequestClientIDByClientType(t *testing.T) { // REQ-068 REQ-061
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const (
		clientID = "app-client"
		secret   = "s3cret"
	)

	clients := []struct {
		name string
		opts []smart.Option
		// methods is the server's token_endpoint_auth_methods_supported.
		methods []string
		// wantClientID: the form carries client_id, exactly once.
		wantClientID bool
		// wantSecret: the form carries client_secret, exactly once.
		wantSecret bool
		// wantBasic: the request carries HTTP Basic credentials.
		wantBasic bool
		// wantAssertion: the form carries a client_assertion.
		wantAssertion bool
	}{
		{
			name:         "public",
			wantClientID: true,
		},
		{
			name:      "client_secret_basic",
			opts:      []smart.Option{smart.WithClientSecret(secret)},
			wantBasic: true,
		},
		{
			name:         "client_secret_post",
			opts:         []smart.Option{smart.WithClientSecret(secret)},
			methods:      []string{"client_secret_post"},
			wantClientID: true,
			wantSecret:   true,
		},
		{
			name:          "private_key_jwt",
			opts:          []smart.Option{smart.WithClientAssertionKey(key, "RS384", "kid-1")},
			wantAssertion: true,
		},
	}

	grants := []struct {
		name string
		// run makes the one token request the grant needs.
		run func(t *testing.T, src *smart.Source)
	}{
		{
			name: "authorization_code",
			run: func(t *testing.T, src *smart.Source) {
				req, err := src.BeginAuthorization("state-1")
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", "state-1", req); err != nil {
					t.Fatalf("ExchangeAuthorizationCode: %v", err)
				}
			},
		},
		{
			name: "refresh_token",
			run: func(t *testing.T, src *smart.Source) {
				stale := auth.Token{Value: "at-old", Type: auth.TokenTypeBearer, ExpiresAt: time.Now().Add(-time.Minute)}
				src.SetTokens(stale, "rt-old")
				if _, err := src.Token(t.Context()); err != nil {
					t.Fatalf("Token on a stale token (refresh): %v", err)
				}
			},
		},
	}

	for _, cc := range clients {
		t.Run(cc.name, func(t *testing.T) {
			for _, gc := range grants {
				t.Run(gc.name, func(t *testing.T) {
					t.Parallel()
					var (
						mu       sync.Mutex
						calls    int
						form     url.Values
						user     string
						pass     string
						hasBasic bool
					)
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path != "/token" {
							http.NotFound(w, r)
							return
						}
						b, _ := io.ReadAll(r.Body)
						f, perr := url.ParseQuery(string(b))
						if perr != nil {
							t.Errorf("token request body %q: %v", b, perr)
						}
						u, p, ok := r.BasicAuth()
						mu.Lock()
						calls++
						form, user, pass, hasBasic = f, u, p, ok
						mu.Unlock()
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"access_token":"at-new","token_type":"Bearer","expires_in":3600,"refresh_token":"rt-new"}`)
					}))
					defer srv.Close()

					ep := discovery.AuthEndpoints{
						AuthorizationEndpoint:             discovery.MustParseURL(srv.URL + "/authorize"),
						TokenEndpoint:                     discovery.MustParseURL(srv.URL + "/token"),
						TokenEndpointAuthMethodsSupported: cc.methods,
					}
					opts := append([]smart.Option{
						smart.WithHTTPClient(srv.Client()),
						smart.WithRedirectURI("https://app.example/callback"),
					}, cc.opts...)
					src, err := newSource(clientID, ep, opts...)
					if err != nil {
						t.Fatalf("newSource: %v", err)
					}

					gc.run(t, src)

					mu.Lock()
					defer mu.Unlock()
					if calls != 1 {
						t.Fatalf("token endpoint calls = %d, want 1", calls)
					}
					if got := form.Get("grant_type"); got != gc.name {
						t.Fatalf("grant_type = %q, want %q", got, gc.name)
					}
					checkFormField(t, form, "client_id", clientID, cc.wantClientID)
					checkFormField(t, form, "client_secret", secret, cc.wantSecret)
					if hasBasic != cc.wantBasic {
						t.Errorf("HTTP Basic credentials sent = %t, want %t", hasBasic, cc.wantBasic)
					}
					if cc.wantBasic && (user != clientID || pass != secret) {
						t.Errorf("HTTP Basic credentials = %q:%q, want %q:%q", user, pass, clientID, secret)
					}
					if got := form.Get("client_assertion") != ""; got != cc.wantAssertion {
						t.Errorf("client_assertion sent = %t, want %t", got, cc.wantAssertion)
					}
				})
			}
		})
	}
}

// checkFormField reports an error unless the form carries name exactly once
// with value want (when present is true), or does not carry name at all.
func checkFormField(t *testing.T, form url.Values, name, want string, present bool) {
	t.Helper()
	got, ok := form[name]
	switch {
	case present && !slices.Equal(got, []string{want}):
		t.Errorf("form field %s = %q, want exactly [%q]", name, got, want)
	case !present && ok:
		t.Errorf("form field %s = %q, want no such field", name, got)
	}
}
