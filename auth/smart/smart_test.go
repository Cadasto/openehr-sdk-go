package smart_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

func testAuthEndpoints(srv *httptest.Server) discovery.AuthEndpoints {
	return discovery.AuthEndpoints{
		AuthorizationEndpoint: discovery.MustParseURL(srv.URL + "/authorize"),
		TokenEndpoint:         discovery.MustParseURL(srv.URL + "/token"),
		JWKSURI:               discovery.MustParseURL(srv.URL + "/jwks"),
	}
}

func TestPKCEAndAuthorizeURL(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	src, err := smart.New("client-id", testAuthEndpoints(srv),
		smart.WithHTTPClient(srv.Client()),
		smart.WithRedirectURI("https://app.example/callback"),
		smart.WithScopes("openid", "patient/*.read"),
	)
	if err != nil {
		t.Fatal(err)
	}
	req, err := src.BeginAuthorization("state-123")
	if err != nil {
		t.Fatal(err)
	}
	u, err := src.AuthorizeURL(req, "launch-token")
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(u)
	q := parsed.Query()
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("challenge method = %q", q.Get("code_challenge_method"))
	}
	if q.Get("launch") != "launch-token" {
		t.Errorf("launch = %q", q.Get("launch"))
	}
	if q.Get("state") != "state-123" {
		t.Errorf("state = %q", q.Get("state"))
	}
}

func TestExchangeAndRefresh(t *testing.T) {
	var tokenCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			tokenCalls++
			b, _ := io.ReadAll(r.Body)
			vals, _ := url.ParseQuery(string(b))
			w.Header().Set("Content-Type", "application/json")
			if vals.Get("grant_type") == "authorization_code" {
				_, _ = w.Write([]byte(`{"access_token":"at-1","token_type":"Bearer","expires_in":3600,"refresh_token":"rt-1"}`))
				return
			}
			if vals.Get("grant_type") == "refresh_token" {
				_, _ = w.Write([]byte(`{"access_token":"at-2","token_type":"Bearer","expires_in":3600,"refresh_token":"rt-2"}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		case "/jwks":
			_, _ = w.Write(readJWKS())
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	src, err := smart.New("client-id", testAuthEndpoints(srv),
		smart.WithHTTPClient(srv.Client()),
		smart.WithRedirectURI("https://app.example/callback"),
	)
	if err != nil {
		t.Fatal(err)
	}
	req, err := src.BeginAuthorization("state-abc")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := src.ExchangeAuthorizationCode(context.Background(), "code-xyz", req)
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "at-1" {
		t.Fatalf("access = %q", tok.Value)
	}

	// Force stale and refresh.
	src.SetTokens(auth.Token{Value: "at-1", Type: "Bearer", ExpiresAt: time.Now().Add(-time.Minute)}, "rt-1")
	tok2, err := src.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok2.Value != "at-2" {
		t.Fatalf("refreshed = %q", tok2.Value)
	}
	if tokenCalls < 2 {
		t.Fatalf("token calls = %d", tokenCalls)
	}
}

func TestJWKSRefreshOnMiss(t *testing.T) {
	fetches := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches++
		_, _ = w.Write(readJWKS())
	}))
	defer srv.Close()

	jwks, err := smart.NewJWKS(srv.Client(), srv.URL+"/jwks")
	if err != nil {
		t.Fatal(err)
	}
	jwks.TTL = time.Hour
	if _, err := jwks.Key(context.Background(), "key-2026-04"); err != nil {
		t.Fatal(err)
	}
	if _, err := jwks.Key(context.Background(), "missing-kid"); err == nil {
		t.Fatal("expected miss")
	}
	if fetches != 2 {
		t.Fatalf("fetches = %d", fetches)
	}
}

func readJWKS() []byte {
	return []byte(`{"keys":[{"kty":"RSA","kid":"key-2026-04","n":"abc","e":"AQAB"}]}`)
}

func TestExchangeRequiresAuthorizationRequest(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	src, _ := smart.New("c", testAuthEndpoints(srv), smart.WithHTTPClient(srv.Client()), smart.WithRedirectURI("https://cb"))
	_, err := src.ExchangeAuthorizationCode(context.Background(), "code", smart.AuthorizationRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestConcurrentLaunchesDoNotClobberPKCE(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		b, _ := io.ReadAll(r.Body)
		vals, _ := url.ParseQuery(string(b))
		seen = append(seen, vals.Get("code_verifier"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"ok","token_type":"Bearer","expires_in":60}`))
	}))
	defer srv.Close()

	src, err := smart.New("c", testAuthEndpoints(srv), smart.WithHTTPClient(srv.Client()), smart.WithRedirectURI("https://cb"))
	if err != nil {
		t.Fatal(err)
	}
	reqA, _ := src.BeginAuthorization("state-a")
	reqB, _ := src.BeginAuthorization("state-b")
	if reqA.PKCE.Verifier == reqB.PKCE.Verifier {
		t.Fatal("expected distinct verifiers")
	}
	if _, err := src.ExchangeAuthorizationCode(context.Background(), "code-a", reqA); err != nil {
		t.Fatal(err)
	}
	if _, err := src.ExchangeAuthorizationCode(context.Background(), "code-b", reqB); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[0] != reqA.PKCE.Verifier || seen[1] != reqB.PKCE.Verifier {
		t.Fatalf("verifiers = %v, want %q then %q", seen, reqA.PKCE.Verifier, reqB.PKCE.Verifier)
	}
}

func TestTokenStaleWithoutRefreshDoesNotDeadlock(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	src, _ := smart.New("c", testAuthEndpoints(srv), smart.WithHTTPClient(srv.Client()), smart.WithRedirectURI("https://cb"))
	src.SetTokens(auth.Token{Value: "cached", Type: "Bearer", ExpiresAt: time.Now().Add(-time.Minute)}, "")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		_, _ = src.Token(ctx)
		close(done)
	}()
	if _, err := src.Token(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("concurrent Token deadlocked")
	}
}
