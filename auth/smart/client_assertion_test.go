package smart_test

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// assertionTestEndpoints returns endpoints on as.example that list signAlgs
// as the server's token_endpoint_auth_signing_alg_values_supported.
func assertionTestEndpoints(signAlgs []string) discovery.AuthEndpoints {
	return discovery.AuthEndpoints{
		AuthorizationEndpoint: discovery.MustParseURL("https://as.example/authorize"),
		TokenEndpoint:         discovery.MustParseURL("https://as.example/token"),
		TokenEndpointAuthSigningAlgValuesSupported: signAlgs,
	}
}

// TestClientAssertionKeyNeedsKeyID pins that WithClientAssertionKey with an
// empty kid fails construction with auth.ErrInvalidConfig, through New and
// through NewFromCatalog. The same key with a kid is accepted, so the kid is
// what is refused.
func TestClientAssertionKeyNeedsKeyID(t *testing.T) { // REQ-068
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		BaseURL: "https://platform.example/openehr",
		Issuer:  "https://as.example",
		Auth:    assertionTestEndpoints(nil),
	})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}
	builders := []struct {
		name  string
		build func(opts ...smart.Option) (*smart.Source, error)
	}{
		{name: "New", build: func(opts ...smart.Option) (*smart.Source, error) {
			return newSource("client-asym", assertionTestEndpoints(nil), opts...)
		}},
		{name: "NewFromCatalog", build: func(opts ...smart.Option) (*smart.Source, error) {
			return smart.NewFromCatalog(catalog, "client-asym", opts...)
		}},
	}
	for _, b := range builders {
		t.Run(b.name, func(t *testing.T) {
			src, err := b.build(smart.WithHTTPClient(&http.Client{}), smart.WithClientAssertionKey(key, "RS384", ""))
			if !errors.Is(err, auth.ErrInvalidConfig) {
				t.Errorf("%s with WithClientAssertionKey(key, RS384, \"\") error = %v, want auth.ErrInvalidConfig", b.name, err)
			}
			if src != nil {
				t.Errorf("%s with an empty kid returned a source, want nil", b.name)
			}
			if _, err := b.build(smart.WithHTTPClient(&http.Client{}), smart.WithClientAssertionKey(key, "RS384", "kid-1")); err != nil {
				t.Errorf("%s with WithClientAssertionKey(key, RS384, kid-1) error = %v, want nil", b.name, err)
			}
		})
	}
}

// TestClientAssertionKeyRefusesNilKey pins that WithClientAssertionKey
// with a nil key of a concrete type, a nil *rsa.PrivateKey or a nil
// *ecdsa.PrivateKey passed as a crypto.Signer, fails construction with
// auth.ErrInvalidConfig instead of panicking.
func TestClientAssertionKeyRefusesNilKey(t *testing.T) { // REQ-068
	tests := []struct {
		name string
		key  crypto.Signer
		alg  string
	}{
		{name: "nil RSA key", key: (*rsa.PrivateKey)(nil), alg: "RS384"},
		{name: "nil ECDSA key", key: (*ecdsa.PrivateKey)(nil), alg: "ES384"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("New with WithClientAssertionKey(%T(nil), %s, kid-1) panicked: %v, want auth.ErrInvalidConfig", tc.key, tc.alg, r)
				}
			}()
			src, err := newSource("client-asym", assertionTestEndpoints(nil),
				smart.WithHTTPClient(&http.Client{}),
				smart.WithClientAssertionKey(tc.key, tc.alg, "kid-1"),
			)
			if !errors.Is(err, auth.ErrInvalidConfig) {
				t.Errorf("New with WithClientAssertionKey(%T(nil), %s, kid-1) error = %v, want auth.ErrInvalidConfig", tc.key, tc.alg, err)
			}
			if src != nil {
				t.Errorf("New with WithClientAssertionKey(%T(nil), %s, kid-1) returned a source, want nil", tc.key, tc.alg)
			}
		})
	}
}

// TestClientAssertionAlgMustBeAdvertised pins the check of the assertion
// algorithm against the server's token_endpoint_auth_signing_alg_values_supported:
// a non-empty list that leaves the algorithm out fails construction with
// auth.ErrInvalidConfig, and an absent or empty list does not constrain it.
// An empty alg is checked as RS384, the algorithm it defaults to.
func TestClientAssertionAlgMustBeAdvertised(t *testing.T) { // REQ-068
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		advertised []string
		key        crypto.Signer
		alg        string
		wantErr    bool
	}{
		{name: "listed", advertised: []string{"RS384", "ES384"}, key: rsaKey, alg: "RS384"},
		{name: "listed ES384", advertised: []string{"ES384"}, key: ecKey, alg: "ES384"},
		{name: "not listed", advertised: []string{"ES384"}, key: rsaKey, alg: "RS384", wantErr: true},
		{name: "not listed ES384", advertised: []string{"RS384", "RS256"}, key: ecKey, alg: "ES384", wantErr: true},
		{name: "absent list", advertised: nil, key: rsaKey, alg: "RS384"},
		{name: "empty list", advertised: []string{}, key: rsaKey, alg: "RS384"},
		{name: "default RS384 listed", advertised: []string{"RS384"}, key: rsaKey, alg: ""},
		{name: "default RS384 not listed", advertised: []string{"ES384"}, key: rsaKey, alg: "", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src, err := newSource("client-asym", assertionTestEndpoints(tc.advertised),
				smart.WithHTTPClient(&http.Client{}),
				smart.WithClientAssertionKey(tc.key, tc.alg, "kid-1"),
			)
			if tc.wantErr {
				if !errors.Is(err, auth.ErrInvalidConfig) {
					t.Errorf("alg %q with advertised %q: error = %v, want auth.ErrInvalidConfig", tc.alg, tc.advertised, err)
				}
				if src != nil {
					t.Errorf("alg %q with advertised %q: returned a source, want nil", tc.alg, tc.advertised)
				}
				return
			}
			if err != nil {
				t.Errorf("alg %q with advertised %q: error = %v, want nil", tc.alg, tc.advertised, err)
			}
		})
	}
}

// TestClientAssertionSentLifetimeAndKeyID pins the client assertion
// auth/smart sends on the code exchange: its exp is after its iat and at
// most five minutes after it, and its header carries the configured kid and
// algorithm (RS384 when alg is empty).
func TestClientAssertionSentLifetimeAndKeyID(t *testing.T) { // REQ-068
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		key     crypto.Signer
		alg     string
		wantAlg string
	}{
		{name: "ES384", key: ecKey, alg: "ES384", wantAlg: "ES384"},
		{name: "default alg", key: rsaKey, alg: "", wantAlg: "RS384"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const kid = "kid-2026-10"
			var (
				mu        sync.Mutex
				assertion string
			)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				form, perr := url.ParseQuery(string(b))
				if perr != nil {
					t.Errorf("token request body %q: %v", b, perr)
				}
				mu.Lock()
				assertion = form.Get("client_assertion")
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"access_token":"at","token_type":"Bearer","expires_in":3600}`)
			}))
			defer srv.Close()

			src, err := newSource("client-asym",
				discovery.AuthEndpoints{
					AuthorizationEndpoint: discovery.MustParseURL(srv.URL + "/authorize"),
					TokenEndpoint:         discovery.MustParseURL(srv.URL + "/token"),
				},
				smart.WithHTTPClient(srv.Client()),
				smart.WithRedirectURI("https://app.example/callback"),
				smart.WithClientAssertionKey(tc.key, tc.alg, kid),
			)
			if err != nil {
				t.Fatalf("newSource: %v", err)
			}
			req, err := src.BeginAuthorization("state-1")
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", "state-1", req); err != nil {
				t.Fatalf("ExchangeAuthorizationCode: %v", err)
			}

			mu.Lock()
			jwt := assertion
			mu.Unlock()
			parts := strings.Split(jwt, ".")
			if len(parts) != 3 {
				t.Fatalf("client_assertion %q has %d segments, want 3", jwt, len(parts))
			}
			var header struct {
				Alg string `json:"alg"`
				Kid string `json:"kid"`
				Typ string `json:"typ"`
			}
			var claims struct {
				Iat int64 `json:"iat"`
				Exp int64 `json:"exp"`
			}
			decodeSegment(t, parts[0], &header)
			decodeSegment(t, parts[1], &claims)
			if header.Kid != kid {
				t.Errorf("client_assertion kid = %q, want %q", header.Kid, kid)
			}
			if header.Alg != tc.wantAlg {
				t.Errorf("client_assertion alg = %q, want %q", header.Alg, tc.wantAlg)
			}
			if header.Typ != "JWT" {
				t.Errorf("client_assertion typ = %q, want JWT", header.Typ)
			}
			if claims.Exp <= claims.Iat || claims.Exp > claims.Iat+300 {
				t.Errorf("client_assertion iat = %d, exp = %d, want exp after iat and at most 300 seconds after it", claims.Iat, claims.Exp)
			}
		})
	}
}

// decodeSegment decodes one base64url JWT segment as JSON into v.
func decodeSegment(t *testing.T, seg string, v any) {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		t.Fatalf("decode JWT segment %q: %v", seg, err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("unmarshal JWT segment %s: %v", b, err)
	}
}
