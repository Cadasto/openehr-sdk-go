package smart_test

import (
	"context"
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

func TestBeginAuthorizationEmptyStateGeneratesRandom(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	src, err := smart.New(
		"client-id", testAuthEndpoints(srv),
		smart.WithHTTPClient(srv.Client()),
		smart.WithRedirectURI("https://app.example/callback"),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Empty state must not error — it must generate a random state instead.
	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization(\"\") error = %v, want nil", err)
	}
	if req.State == "" {
		t.Fatal("BeginAuthorization(\"\") returned empty State, want non-empty")
	}
	// stateLen=32 random bytes base64url-encode to exactly 43 chars; an
	// exact check makes any entropy/encoding regression visible.
	if len(req.State) != 43 {
		t.Fatalf("BeginAuthorization(\"\") State len = %d, want 43", len(req.State))
	}

	// Two successive calls must produce distinct states (unpredictability).
	req2, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatal(err)
	}
	if req.State == req2.State {
		t.Fatalf("two BeginAuthorization(\"\") calls returned the same State %q", req.State)
	}
}

func TestBeginAuthorizationNonEmptyStatePreserved(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	src, err := smart.New(
		"client-id", testAuthEndpoints(srv),
		smart.WithHTTPClient(srv.Client()),
		smart.WithRedirectURI("https://app.example/callback"),
	)
	if err != nil {
		t.Fatal(err)
	}

	const supplied = "my-explicit-state"
	req, err := src.BeginAuthorization(supplied)
	if err != nil {
		t.Fatalf("BeginAuthorization(%q) error = %v, want nil", supplied, err)
	}
	if req.State != supplied {
		t.Fatalf("req.State = %q, want %q", req.State, supplied)
	}
}

func TestPKCEAndAuthorizeURL(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	src, err := smart.New(
		"client-id", testAuthEndpoints(srv),
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
				_, _ = w.Write([]byte(`{"access_token":"at-2","token_type":"Bearer","expires_in":3600,"refresh_token":"rt-2","patient":"p-refreshed"}`))
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

	src, err := smart.New(
		"client-id", testAuthEndpoints(srv),
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
	tok, tr, err := src.ExchangeAuthorizationCode(context.Background(), "code-xyz", "state-abc", req)
	if err != nil {
		t.Fatal(err)
	}
	if tr.AccessToken != "at-1" {
		t.Fatalf("token response access = %q", tr.AccessToken)
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
	if tr := src.LastTokenResponse(); tr.Patient != "p-refreshed" {
		t.Fatalf("LastTokenResponse after refresh = %#v", tr)
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
	_, _, err := src.ExchangeAuthorizationCode(context.Background(), "code", "", smart.AuthorizationRequest{})
	if !errors.Is(err, auth.ErrInvalidConfig) {
		t.Fatalf("err = %v, want ErrInvalidConfig (empty-request guard, not state mismatch)", err)
	}
}

// TestExchangeAuthorizationCodeStateMismatch verifies that a wrong callback
// state returns ErrLaunchInvalidState and does NOT hit the token endpoint
// (REQ-061: validate state before exchanging the code).
func TestExchangeAuthorizationCodeStateMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("token endpoint must not be called on state mismatch; got request to %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	src, err := smart.New(
		"client-id", testAuthEndpoints(srv),
		smart.WithHTTPClient(srv.Client()),
		smart.WithRedirectURI("https://app.example/callback"),
	)
	if err != nil {
		t.Fatal(err)
	}
	req, err := src.BeginAuthorization("known-state")
	if err != nil {
		t.Fatal(err)
	}
	_, _, exchangeErr := src.ExchangeAuthorizationCode(context.Background(), "code", "WRONG-state", req)
	if !errors.Is(exchangeErr, smart.ErrLaunchInvalidState) {
		t.Fatalf("expected ErrLaunchInvalidState, got %v", exchangeErr)
	}
}

// TestExchangeAuthorizationCodeStateMatch verifies that a matching callback
// state proceeds to the token exchange (REQ-061).
func TestExchangeAuthorizationCodeStateMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"at-ok","token_type":"Bearer","expires_in":3600}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	src, err := smart.New(
		"client-id", testAuthEndpoints(srv),
		smart.WithHTTPClient(srv.Client()),
		smart.WithRedirectURI("https://app.example/callback"),
	)
	if err != nil {
		t.Fatal(err)
	}
	req, err := src.BeginAuthorization("correct-state")
	if err != nil {
		t.Fatal(err)
	}
	tok, _, exchangeErr := src.ExchangeAuthorizationCode(context.Background(), "code", "correct-state", req)
	if exchangeErr != nil {
		t.Fatalf("expected no error on state match, got %v", exchangeErr)
	}
	if tok.Value != "at-ok" {
		t.Fatalf("access token = %q, want %q", tok.Value, "at-ok")
	}
}

// TestAuthorizeURLStateReachesURL verifies the state generated by BeginAuthorization
// is included verbatim in the authorize URL query parameter (2a).
func TestAuthorizeURLStateReachesURL(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	src, err := smart.New(
		"client-id", testAuthEndpoints(srv),
		smart.WithHTTPClient(srv.Client()),
		smart.WithRedirectURI("https://app.example/callback"),
	)
	if err != nil {
		t.Fatal(err)
	}
	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatal(err)
	}
	u, err := src.AuthorizeURL(req, "")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.Query().Get("state"); got != req.State {
		t.Fatalf("authorize URL state = %q, want %q", got, req.State)
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
	if _, _, err := src.ExchangeAuthorizationCode(context.Background(), "code-a", reqA.State, reqA); err != nil {
		t.Fatal(err)
	}
	if _, _, err := src.ExchangeAuthorizationCode(context.Background(), "code-b", reqB.State, reqB); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[0] != reqA.PKCE.Verifier || seen[1] != reqB.PKCE.Verifier {
		t.Fatalf("verifiers = %v, want %q then %q", seen, reqA.PKCE.Verifier, reqB.PKCE.Verifier)
	}
}

// REQ-068 — asymmetric confidential authorization-code flow
// (SMART client-confidential-asymmetric profile). The code exchange MUST
// authenticate with private_key_jwt: a signed client_assertion plus the
// jwt-bearer client_assertion_type, and NO HTTP Basic header.
func TestExchangeWithPrivateKeyJWT(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	var capturedForm url.Values
	var capturedAuthHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			b, _ := io.ReadAll(r.Body)
			capturedForm, _ = url.ParseQuery(string(b))
			capturedAuthHeader = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"at-pkjwt","token_type":"Bearer","expires_in":3600}`))
		case "/jwks":
			_, _ = w.Write(readJWKS())
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	const clientID = "confidential-asym-client"
	src, err := smart.New(
		clientID, testAuthEndpoints(srv),
		smart.WithHTTPClient(srv.Client()),
		smart.WithRedirectURI("https://app.example/callback"),
		smart.WithClientAssertionKey(key, "RS384", "kid-1"),
	)
	if err != nil {
		t.Fatal(err)
	}

	req, err := src.BeginAuthorization("state-pkjwt")
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := src.ExchangeAuthorizationCode(context.Background(), "code-1", "state-pkjwt", req)
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode error = %v", err)
	}
	if tok.Value != "at-pkjwt" {
		t.Fatalf("access = %q, want at-pkjwt", tok.Value)
	}

	// No HTTP Basic header — confidential auth is by assertion, not secret.
	if capturedAuthHeader != "" {
		t.Fatalf("Authorization header = %q, want empty (no client_secret_basic)", capturedAuthHeader)
	}

	if got := capturedForm.Get("client_assertion_type"); got != "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" {
		t.Fatalf("client_assertion_type = %q", got)
	}
	assertion := capturedForm.Get("client_assertion")
	if assertion == "" {
		t.Fatal("client_assertion is empty, want a signed JWT")
	}

	// Decode the JWT payload (header.payload.signature) and verify claims.
	parts := strings.Split(assertion, ".")
	if len(parts) != 3 {
		t.Fatalf("client_assertion has %d segments, want 3", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode assertion payload: %v", err)
	}
	var claims struct {
		Iss string `json:"iss"`
		Sub string `json:"sub"`
		Aud string `json:"aud"`
		Jti string `json:"jti"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("unmarshal assertion claims: %v", err)
	}
	if claims.Iss != clientID || claims.Sub != clientID {
		t.Errorf("iss/sub = %q/%q, want %q for both", claims.Iss, claims.Sub, clientID)
	}
	wantAud := testAuthEndpoints(srv).TokenEndpoint.String()
	if claims.Aud != wantAud {
		t.Errorf("aud = %q, want token endpoint %q", claims.Aud, wantAud)
	}
	if claims.Jti == "" {
		t.Error("jti is empty, want a unique identifier")
	}
	if maxExp := time.Now().Add(5 * time.Minute).Unix(); claims.Exp > maxExp {
		t.Errorf("exp = %d, want <= now+300s (%d)", claims.Exp, maxExp)
	}
	if claims.Exp <= time.Now().Unix() {
		t.Errorf("exp = %d, want in the future", claims.Exp)
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
