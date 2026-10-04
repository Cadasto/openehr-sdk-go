package smart_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// confidentialTokenRequest is what the stand-in token endpoint received.
type confidentialTokenRequest struct {
	form      url.Values
	basicUser string
	basicPass string
	basicAuth bool
}

// confidentialTokenEndpoint starts a token endpoint that records the last
// request it received and answers it with a bearer token. The returned
// function reads that record, and reports false when no request arrived.
func confidentialTokenEndpoint(t *testing.T) (*httptest.Server, func() (confidentialTokenRequest, bool)) {
	t.Helper()
	var (
		mu   sync.Mutex
		last confidentialTokenRequest
		seen bool
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "unreadable body", http.StatusBadRequest)
			return
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			http.Error(w, "malformed form", http.StatusBadRequest)
			return
		}
		user, pass, ok := r.BasicAuth()
		mu.Lock()
		last = confidentialTokenRequest{form: form, basicUser: user, basicPass: pass, basicAuth: ok}
		seen = true
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"at-confidential","token_type":"Bearer","expires_in":3600}`)
	}))
	t.Cleanup(srv.Close)
	return srv, func() (confidentialTokenRequest, bool) {
		mu.Lock()
		defer mu.Unlock()
		return last, seen
	}
}

// TestREQ068_ConfidentialClientUsesPKCE pins REQ-068: "A confidential client
// uses PKCE and authenticates: HL7 SMART App Launch requires PKCE from every
// app, and PKCE does not replace client authentication." For a client secret
// sent by HTTP Basic, a client secret sent in the form body, and a signed
// client assertion, the authorization URL must carry the S256 code_challenge
// and the token request must carry the matching code_verifier next to the
// client's credential.
func TestREQ068_ConfidentialClientUsesPKCE(t *testing.T) {
	t.Parallel()
	const (
		clientID  = "confidential-client"
		secret    = "c0nfidential-s3cret"
		audience  = "https://platform.example"
		redirect  = "https://app.example/callback"
		assertion = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
	)
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey(P-384) error = %v", err)
	}

	cases := []struct {
		name       string
		credential smart.Option
		advertised []string
		// authenticates reports whether the token request carries this
		// client's credential.
		authenticates func(confidentialTokenRequest) bool
	}{
		{
			name:       "client_secret_basic",
			credential: smart.WithClientSecret(secret),
			authenticates: func(r confidentialTokenRequest) bool {
				return r.basicAuth && r.basicUser == clientID && r.basicPass == secret
			},
		},
		{
			name:       "client_secret_post",
			credential: smart.WithClientSecret(secret),
			advertised: []string{"client_secret_post"},
			authenticates: func(r confidentialTokenRequest) bool {
				return !r.basicAuth && r.form.Get("client_id") == clientID && r.form.Get("client_secret") == secret
			},
		},
		{
			name:       "private_key_jwt ES384",
			credential: smart.WithClientAssertionKey(key, "ES384", "kid-es384"),
			authenticates: func(r confidentialTokenRequest) bool {
				return r.form.Get("client_assertion_type") == assertion && r.form.Get("client_assertion") != ""
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, lastRequest := confidentialTokenEndpoint(t)
			endpoints := discovery.AuthEndpoints{
				AuthorizationEndpoint:             discovery.MustParseURL(srv.URL + "/authorize"),
				TokenEndpoint:                     discovery.MustParseURL(srv.URL + "/token"),
				TokenEndpointAuthMethodsSupported: tc.advertised,
			}
			src, err := smart.New(clientID, endpoints,
				smart.WithHTTPClient(srv.Client()),
				smart.WithRedirectURI(redirect),
				smart.WithAudience(audience),
				tc.credential,
			)
			if err != nil {
				t.Fatalf("smart.New(%s) error = %v, want nil", tc.name, err)
			}

			req, err := src.BeginAuthorization("")
			if err != nil {
				t.Fatalf("BeginAuthorization(\"\") error = %v, want nil", err)
			}
			// The challenge must be the S256 hash of the verifier, so the two
			// values checked below belong to one PKCE pair.
			sum := sha256.Sum256([]byte(req.PKCE.Verifier))
			if want := base64.RawURLEncoding.EncodeToString(sum[:]); req.PKCE.Challenge != want {
				t.Fatalf("BeginAuthorization PKCE.Challenge = %q, want S256(PKCE.Verifier) = %q", req.PKCE.Challenge, want)
			}

			raw, err := src.AuthorizeURL(req, "")
			if err != nil {
				t.Fatalf("AuthorizeURL error = %v, want nil", err)
			}
			u, err := url.Parse(raw)
			if err != nil {
				t.Fatalf("url.Parse(AuthorizeURL) error = %v", err)
			}
			q := u.Query()
			if got := q.Get("code_challenge"); got != req.PKCE.Challenge {
				t.Errorf("AuthorizeURL code_challenge = %q, want PKCE.Challenge %q (URL %s)", got, req.PKCE.Challenge, raw)
			}
			if got := q.Get("code_challenge_method"); got != "S256" {
				t.Errorf("AuthorizeURL code_challenge_method = %q, want \"S256\" (URL %s)", got, raw)
			}

			if _, _, err := src.ExchangeAuthorizationCode(t.Context(), "code-confidential", req.State, req); err != nil {
				t.Fatalf("ExchangeAuthorizationCode error = %v, want nil", err)
			}
			got, ok := lastRequest()
			if !ok {
				t.Fatal("ExchangeAuthorizationCode sent no request to the token endpoint")
			}
			if v := got.form.Get("code_verifier"); v != req.PKCE.Verifier {
				t.Errorf("token request code_verifier = %q, want PKCE.Verifier %q (form %v)", v, req.PKCE.Verifier, got.form)
			}
			if !tc.authenticates(got) {
				t.Errorf("token request carries no %s client credential next to the code_verifier (form %v, Basic auth %v)", tc.name, got.form, got.basicAuth)
			}
		})
	}
}
