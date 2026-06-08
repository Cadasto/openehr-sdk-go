package clientcreds

import (
	"context"
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
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
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

	src, err := New("client", "secret", srv.URL,
		WithHTTPClient(srv.Client()),
		WithScope("patient/*.read"),
		WithIssuer("https://auth.example.com"),
	)
	if err != nil {
		t.Fatal(err)
	}

	tok, err := src.Token(context.Background())
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
	if _, err := src.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("token endpoint hit %d times, want 1 (cache miss)", got)
	}
}

func TestTokenRefreshOnExpiry(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-" + string(byte('0'+n)),
			"token_type":   "Bearer",
			"expires_in":   1,
		})
	}))
	defer srv.Close()

	src, err := New("c", "s", srv.URL,
		WithHTTPClient(srv.Client()),
		WithRefreshThreshold(5*time.Second), // 1s expiry < 5s threshold => always stale
	)
	if err != nil {
		t.Fatal(err)
	}
	t1, _ := src.Token(context.Background())
	t2, _ := src.Token(context.Background())
	if t1.Value == t2.Value {
		t.Errorf("expected refresh on stale; got same token %q", t1.Value)
	}
	if h := atomic.LoadInt32(&hits); h < 2 {
		t.Errorf("expected ≥2 hits, got %d", h)
	}
}

// PROBE-074 proves REQ-063 — a token minted WITHOUT expires_in has a zero
// ExpiresAt and is therefore never treated as stale, so it is reused across
// Token calls; Invalidate drops it so the next call performs a fresh exchange.
// This is the recovery path transport/ drives after a wire 401 when the
// authorization server omits expires_in (the observed Cadasto acc behaviour).
func TestInvalidateForcesRefetchWhenNoExpiry(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-" + string(byte('0'+n)),
			"token_type":   "Bearer",
			// deliberately NO expires_in → zero ExpiresAt
		})
	}))
	defer srv.Close()

	src, err := New("c", "s", srv.URL, WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	t1, _ := src.Token(context.Background())
	t2, _ := src.Token(context.Background())
	if t1.Value != t2.Value {
		t.Errorf("no-expiry token should be reused; got %q then %q", t1.Value, t2.Value)
	}
	src.Invalidate()
	t3, _ := src.Token(context.Background())
	if t3.Value == t1.Value {
		t.Errorf("expected fresh token after Invalidate; still %q", t3.Value)
	}
	if h := atomic.LoadInt32(&hits); h != 2 {
		t.Errorf("expected exactly 2 token fetches (initial + post-invalidate), got %d", h)
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
	if _, err := src.Token(context.Background()); err != nil {
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
	src, err := New("c", "s", srv.URL,
		WithHTTPClient(srv.Client()),
		WithAuthMethod(AuthPost),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.Token(context.Background()); err != nil {
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
	_, err := src.Token(context.Background())
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
	var hits int32
	gate := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
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
			if _, err := src.Token(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	// Let goroutines pile up on the in-flight exchange.
	time.Sleep(20 * time.Millisecond)
	close(gate)
	wg.Wait()
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("expected coalesced into 1 exchange, got %d", got)
	}
}

func TestTokenCtxCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	src, _ := New("c", "s", srv.URL, WithHTTPClient(srv.Client()))
	ctx, cancel := context.WithCancel(context.Background())
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
	_, err := src.Token(context.Background())
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
	_, err := src.Token(context.Background())
	if !errors.Is(err, auth.ErrTokenExchangeFailed) {
		t.Errorf("expected ErrTokenExchangeFailed, got %v", err)
	}
	if strings.Contains(err.Error(), "access_token") == false {
		t.Errorf("expected access_token in error, got %v", err)
	}
}
