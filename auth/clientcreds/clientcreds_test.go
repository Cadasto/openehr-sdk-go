package clientcreds

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/jwtbearer"
)

func TestNewValidatesConfig(t *testing.T) {
	tests := []struct {
		name       string
		id, sec, u string
		opts       []Option
		wantErr    bool
	}{
		{"missing httpclient", "id", "sec", "https://x/token", nil, true},
		{"missing id", "", "sec", "https://x/token", []Option{WithHTTPClient(http.DefaultClient)}, true},
		{"missing secret", "id", "", "https://x/token", []Option{WithHTTPClient(http.DefaultClient)}, true},
		{"missing url", "id", "sec", "", []Option{WithHTTPClient(http.DefaultClient)}, true},
		{"bad url", "id", "sec", "not a url", []Option{WithHTTPClient(http.DefaultClient)}, true},
		{"ok", "id", "sec", "https://auth.example.com/token", []Option{WithHTTPClient(http.DefaultClient)}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.id, tc.sec, tc.u, tc.opts...)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr && err != nil && !errors.Is(err, auth.ErrInvalidConfig) {
				t.Errorf("expected ErrInvalidConfig wrap, got %v", err)
			}
		})
	}
}

func TestTokenSuccess(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if user, _, ok := r.BasicAuth(); !ok || user == "" {
			t.Error("expected Basic auth header")
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if g := r.PostForm.Get("grant_type"); g != "client_credentials" {
			t.Errorf("grant_type = %q, want client_credentials", g)
		}
		if g := r.PostForm.Get("scope"); g != "patient/*.read" {
			t.Errorf("scope = %q, want patient/*.read", g)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-xyz",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"scope":        "patient/*.read",
		})
	}))
	defer srv.Close()

	src, err := New(
		"client", "secret", srv.URL,
		WithHTTPClient(srv.Client()),
		WithScope("patient/*.read"),
		WithIssuer("https://auth.example.com"),
	)
	if err != nil {
		t.Fatal(err)
	}

	tok, err := src.Token(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "tok-xyz" {
		t.Errorf("Value = %q, want tok-xyz", tok.Value)
	}
	if tok.Type != "Bearer" {
		t.Errorf("Type = %q, want Bearer", tok.Type)
	}
	if tok.Scope != "patient/*.read" {
		t.Errorf("Scope = %q", tok.Scope)
	}
	if tok.Issuer != "https://auth.example.com" {
		t.Errorf("Issuer = %q", tok.Issuer)
	}
	if time.Until(tok.ExpiresAt) < time.Hour-time.Minute {
		t.Errorf("ExpiresAt too close: %v", tok.ExpiresAt)
	}

	// Second call should hit cache.
	if _, err := src.Token(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("token endpoint hit %d times, want 1 (cache miss)", got)
	}
}

func TestTokenRefreshOnExpiry(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-" + string(byte('0'+n)),
			"token_type":   "Bearer",
			"expires_in":   1,
		})
	}))
	defer srv.Close()

	src, err := New(
		"c", "s", srv.URL,
		WithHTTPClient(srv.Client()),
		WithRefreshThreshold(5*time.Second), // 1s expiry < 5s threshold => always stale
	)
	if err != nil {
		t.Fatal(err)
	}
	t1, _ := src.Token(t.Context())
	t2, _ := src.Token(t.Context())
	if t1.Value == t2.Value {
		t.Errorf("expected refresh on stale; got same token %q", t1.Value)
	}
	if h := hits.Load(); h < 2 {
		t.Errorf("expected ≥2 hits, got %d", h)
	}
}

func TestTokenAuthBasicOAuth2FormEncoding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok {
			t.Fatal("expected Basic auth")
		}
		if user != url.QueryEscape("client:id") {
			t.Errorf("Basic user = %q, want %q", user, url.QueryEscape("client:id"))
		}
		if pass != url.QueryEscape("s@cret") {
			t.Errorf("Basic pass = %q, want %q", pass, url.QueryEscape("s@cret"))
		}
		_, _ = w.Write([]byte(`{"access_token":"x","token_type":"Bearer","expires_in":60}`))
	}))
	defer srv.Close()
	src, err := New("client:id", "s@cret", srv.URL, WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.Token(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestTokenAuthMethodPost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, _, ok := r.BasicAuth(); ok && user != "" {
			t.Errorf("did not expect Basic auth, got user=%q", user)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if id := r.PostForm.Get("client_id"); id != "c" {
			t.Errorf("client_id = %q, want c", id)
		}
		if sec := r.PostForm.Get("client_secret"); sec != "s" {
			t.Errorf("client_secret = %q, want s", sec)
		}
		_, _ = w.Write([]byte(`{"access_token":"x","token_type":"Bearer","expires_in":60}`))
	}))
	defer srv.Close()
	src, err := New(
		"c", "s", srv.URL,
		WithHTTPClient(srv.Client()),
		WithAuthMethod(AuthPost),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.Token(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestTokenOAuth2Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"bad creds"}`))
	}))
	defer srv.Close()
	src, _ := New("c", "s", srv.URL, WithHTTPClient(srv.Client()))
	_, err := src.Token(t.Context())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, auth.ErrTokenExchangeFailed) {
		t.Errorf("expected ErrTokenExchangeFailed wrap, got %v", err)
	}
	var oa *auth.OAuth2Error
	if !errors.As(err, &oa) {
		t.Fatal("expected OAuth2Error via errors.As")
	}
	if oa.Code != "invalid_client" {
		t.Errorf("OAuth2.Code = %q", oa.Code)
	}
}

func TestTokenConcurrentCoalesce(t *testing.T) {
	var hits atomic.Int32
	gate := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-gate // block until released
		_, _ = w.Write([]byte(`{"access_token":"x","token_type":"Bearer","expires_in":3600}`))
	}))
	defer srv.Close()

	src, _ := New("c", "s", srv.URL, WithHTTPClient(srv.Client()))
	var wg sync.WaitGroup
	const N = 8
	wg.Add(N)
	for range N {
		go func() {
			defer wg.Done()
			if _, err := src.Token(t.Context()); err != nil {
				t.Error(err)
			}
		}()
	}
	// Let goroutines pile up on the in-flight exchange.
	time.Sleep(20 * time.Millisecond)
	close(gate)
	wg.Wait()
	if got := hits.Load(); got != 1 {
		t.Errorf("expected coalesced into 1 exchange, got %d", got)
	}
}

func TestTokenCtxCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	src, _ := New("c", "s", srv.URL, WithHTTPClient(srv.Client()))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := src.Token(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestTokenMalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	src, _ := New("c", "s", srv.URL, WithHTTPClient(srv.Client()))
	_, err := src.Token(t.Context())
	if !errors.Is(err, auth.ErrTokenExchangeFailed) {
		t.Errorf("expected ErrTokenExchangeFailed, got %v", err)
	}
}

func TestTokenMissingAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"token_type":"Bearer","expires_in":60}`))
	}))
	defer srv.Close()
	src, _ := New("c", "s", srv.URL, WithHTTPClient(srv.Client()))
	_, err := src.Token(t.Context())
	if !errors.Is(err, auth.ErrTokenExchangeFailed) {
		t.Errorf("expected ErrTokenExchangeFailed, got %v", err)
	}
	if strings.Contains(err.Error(), "access_token") == false {
		t.Errorf("expected access_token in error, got %v", err)
	}
}

// TestClientCredentialsWithClientAssertion verifies SMART Backend Services wire shape:
// grant_type=client_credentials + client_assertion_type (jwt-bearer) + signed client_assertion,
// with no client secret and no HTTP Basic header. (REQ-068)
func TestClientCredentialsWithClientAssertion(t *testing.T) {
	// Capture the raw form body and headers from the token endpoint.
	var (
		capMu        sync.Mutex
		capturedForm url.Values
		capturedAuth string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr := r.Header.Get("Authorization")
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		form := r.PostForm
		capMu.Lock()
		capturedAuth = hdr
		capturedForm = form
		capMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "smart-backend-tok",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"scope":        "system/*.read",
		})
	}))
	defer srv.Close()

	// Build a ClaimsSigner with an RSA key. iss = sub = clientID, aud = token endpoint URL.
	const clientID = "my-backend-client"
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	signer, err := jwtbearer.NewClaimsSigner(
		jwtbearer.ClaimsTemplate{
			Issuer:   clientID,
			Subject:  clientID,
			Audience: srv.URL,
		},
		rsaKey,
		jwtbearer.WithAlgorithm("RS384"),
	)
	if err != nil {
		t.Fatalf("NewClaimsSigner: %v", err)
	}

	// Construct the Source using the new WithClientAssertion option; no client secret.
	src, err := New(
		clientID, "", srv.URL,
		WithHTTPClient(srv.Client()),
		WithScope("system/*.read"),
		WithClientAssertion(signer),
	)
	if err != nil {
		t.Fatalf("New with client assertion: %v", err)
	}

	tok, err := src.Token(t.Context())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.Value != "smart-backend-tok" {
		t.Errorf("Value = %q, want smart-backend-tok", tok.Value)
	}

	// Snapshot the captured values under the lock before asserting.
	capMu.Lock()
	gotForm := capturedForm
	gotAuth := capturedAuth
	capMu.Unlock()

	// Assert required form fields.
	if g := gotForm.Get("grant_type"); g != "client_credentials" {
		t.Errorf("grant_type = %q, want client_credentials", g)
	}
	if g := gotForm.Get("client_assertion_type"); g != "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" {
		t.Errorf("client_assertion_type = %q, want urn:ietf:params:oauth:client-assertion-type:jwt-bearer", g)
	}
	if g := gotForm.Get("client_assertion"); g == "" {
		t.Error("client_assertion must be non-empty")
	}
	if g := gotForm.Get("scope"); g != "system/*.read" {
		t.Errorf("scope = %q, want system/*.read", g)
	}

	// Assert no HTTP Basic Authorization header was sent.
	if strings.HasPrefix(gotAuth, "Basic ") {
		t.Errorf("expected no Basic Authorization header, got %q", gotAuth)
	}
}

// TestFromConfigRejectsSecretAndAssertion verifies that providing both a client
// secret and a client assertion source is rejected with ErrInvalidConfig. (REQ-068)
func TestFromConfigRejectsSecretAndAssertion(t *testing.T) {
	stub := jwtbearer.AssertionFunc(func(ctx context.Context) (string, error) { return "stub", nil })

	_, err := New(
		"c", "s", "https://auth.example.com/token",
		WithHTTPClient(http.DefaultClient),
		WithClientAssertion(stub),
	)
	if err == nil {
		t.Fatal("expected error when both secret and assertion are set, got nil")
	}
	if !errors.Is(err, auth.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

// TestFromConfigAssertionNoSecretRequired verifies that ClientSecret is not required
// when a client assertion source is provided. (REQ-068)
func TestFromConfigAssertionNoSecretRequired(t *testing.T) {
	stub := jwtbearer.AssertionFunc(func(ctx context.Context) (string, error) { return "stub", nil })

	_, err := New(
		"c", "", "https://auth.example.com/token",
		WithHTTPClient(http.DefaultClient),
		WithClientAssertion(stub),
	)
	if err != nil {
		t.Errorf("expected no error when assertion is set and secret is empty, got %v", err)
	}
}

// TestFromConfigRejectsPrivateKeyJWTWithoutAssertion verifies that setting
// AuthPrivateKeyJWT without also providing a ClientAssertion source is rejected
// with ErrInvalidConfig. (REQ-068)
func TestFromConfigRejectsPrivateKeyJWTWithoutAssertion(t *testing.T) {
	c := &http.Client{}
	_, err := New(
		"c", "", "https://auth.example.com/token",
		WithHTTPClient(c),
		WithAuthMethod(AuthPrivateKeyJWT),
	)
	if err == nil {
		t.Fatal("expected ErrInvalidConfig when AuthPrivateKeyJWT is set without ClientAssertion, got nil")
	}
	if !errors.Is(err, auth.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}
