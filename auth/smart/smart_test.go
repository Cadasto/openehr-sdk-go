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
	"sync"
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

	var (
		capMu              sync.Mutex
		capturedForm       url.Values
		capturedAuthHeader string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			b, _ := io.ReadAll(r.Body)
			form, _ := url.ParseQuery(string(b))
			hdr := r.Header.Get("Authorization")
			capMu.Lock()
			capturedForm = form
			capturedAuthHeader = hdr
			capMu.Unlock()
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

	// Snapshot the captured values under the lock before asserting.
	capMu.Lock()
	gotAuthHeader := capturedAuthHeader
	gotForm := capturedForm
	capMu.Unlock()

	// No HTTP Basic header — confidential auth is by assertion, not secret.
	if gotAuthHeader != "" {
		t.Fatalf("Authorization header = %q, want empty (no client_secret_basic)", gotAuthHeader)
	}

	if got := gotForm.Get("client_assertion_type"); got != "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" {
		t.Fatalf("client_assertion_type = %q", got)
	}
	assertion := gotForm.Get("client_assertion")
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

// TestExchangeWithClientSecretBasic verifies that, when a Source is configured
// with WithClientSecret (confidential symmetric, client_secret_basic), the
// authorization-code token exchange sends an HTTP Basic auth header with the
// correct base64-encoded clientID:secret credential, includes grant_type and
// code fields on the form body, and does NOT send client_assertion /
// client_assertion_type form fields (REQ-068).
func TestExchangeWithClientSecretBasic(t *testing.T) { // REQ-068
	var (
		capMu        sync.Mutex
		capturedAuth string
		capturedForm url.Values
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			b, _ := io.ReadAll(r.Body)
			form, _ := url.ParseQuery(string(b))
			hdr := r.Header.Get("Authorization")
			capMu.Lock()
			capturedForm = form
			capturedAuth = hdr
			capMu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"at-sym","token_type":"Bearer","expires_in":3600}`))
		case "/jwks":
			_, _ = w.Write(readJWKS())
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	const (
		clientID = "sym-client"
		secret   = "s3cret"
	)
	src, err := smart.New(
		clientID, testAuthEndpoints(srv),
		smart.WithHTTPClient(srv.Client()),
		smart.WithRedirectURI("https://app.example/callback"),
		smart.WithClientSecret(secret),
	)
	if err != nil {
		t.Fatal(err)
	}

	req, err := src.BeginAuthorization("state-sym")
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := src.ExchangeAuthorizationCode(context.Background(), "code-sym", "state-sym", req)
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode error = %v", err)
	}
	if tok.Value != "at-sym" {
		t.Fatalf("access = %q, want at-sym", tok.Value)
	}

	// Snapshot the captured values under the lock before asserting.
	capMu.Lock()
	gotAuth := capturedAuth
	gotForm := capturedForm
	capMu.Unlock()

	// The Authorization header must be HTTP Basic with base64(clientID:secret).
	// http.Request.SetBasicAuth encodes clientID and secret directly (no
	// url.QueryEscape) and uses standard base64 encoding.
	wantBasic := "Basic " + base64.StdEncoding.EncodeToString([]byte(clientID+":"+secret))
	if gotAuth != wantBasic {
		t.Fatalf("Authorization header = %q, want %q", gotAuth, wantBasic)
	}

	// The form must include the expected grant_type.
	if got := gotForm.Get("grant_type"); got != "authorization_code" {
		t.Fatalf("grant_type = %q, want authorization_code", got)
	}

	// The form must include the authorization code (Fix 4: previously missing assertion).
	if got := gotForm.Get("code"); got != "code-sym" {
		t.Fatalf("code = %q, want code-sym", got)
	}

	// No client_assertion or client_assertion_type — symmetric auth uses Basic,
	// not a signed JWT (REQ-068).
	if got := gotForm.Get("client_assertion"); got != "" {
		t.Fatalf("client_assertion = %q, want empty (symmetric client must not send JWT assertion)", got)
	}
	if got := gotForm.Get("client_assertion_type"); got != "" {
		t.Fatalf("client_assertion_type = %q, want empty (symmetric client must not send assertion type)", got)
	}
}

// TestExchangeWithClientSecretPost verifies that, when the authorization
// server advertises only client_secret_post (and not client_secret_basic), a
// Source configured with WithClientSecret presents its credential in the form
// body (client_id / client_secret) and sends NO HTTP Basic Authorization
// header (REQ-068).
func TestExchangeWithClientSecretPost(t *testing.T) { // REQ-068
	var (
		capMu        sync.Mutex
		capturedAuth string
		capturedForm url.Values
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			b, _ := io.ReadAll(r.Body)
			form, _ := url.ParseQuery(string(b))
			hdr := r.Header.Get("Authorization")
			capMu.Lock()
			capturedForm = form
			capturedAuth = hdr
			capMu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"at-post","token_type":"Bearer","expires_in":3600}`))
		case "/jwks":
			_, _ = w.Write(readJWKS())
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	const (
		clientID = "post-client"
		secret   = "p0st-s3cret"
	)
	eps := discovery.AuthEndpoints{
		AuthorizationEndpoint:             discovery.MustParseURL(srv.URL + "/authorize"),
		TokenEndpoint:                     discovery.MustParseURL(srv.URL + "/token"),
		JWKSURI:                           discovery.MustParseURL(srv.URL + "/jwks"),
		TokenEndpointAuthMethodsSupported: []string{"client_secret_post"},
	}
	src, err := smart.New(
		clientID, eps,
		smart.WithHTTPClient(srv.Client()),
		smart.WithRedirectURI("https://app.example/callback"),
		smart.WithClientSecret(secret),
	)
	if err != nil {
		t.Fatal(err)
	}

	req, err := src.BeginAuthorization("state-post")
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := src.ExchangeAuthorizationCode(context.Background(), "code-post", "state-post", req)
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode error = %v", err)
	}
	if tok.Value != "at-post" {
		t.Fatalf("access = %q, want at-post", tok.Value)
	}

	capMu.Lock()
	gotAuth := capturedAuth
	gotForm := capturedForm
	capMu.Unlock()

	if gotAuth != "" {
		t.Fatalf("Authorization header = %q, want empty (client_secret_post must not use HTTP Basic)", gotAuth)
	}
	if got := gotForm.Get("client_id"); got != clientID {
		t.Fatalf("client_id = %q, want %q", got, clientID)
	}
	if got := gotForm.Get("client_secret"); got != secret {
		t.Fatalf("client_secret = %q, want %q", got, secret)
	}
}

// TestClientAssertionAndSecretBothRejected verifies that configuring both
// WithClientSecret and WithClientAssertionKey is rejected at construction
// with ErrInvalidConfig (REQ-068).
func TestClientAssertionAndSecretBothRejected(t *testing.T) { // REQ-068
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	_, err = smart.New(
		"client-id", testAuthEndpoints(srv),
		smart.WithHTTPClient(srv.Client()),
		smart.WithClientSecret("some-secret"),
		smart.WithClientAssertionKey(key, "RS384", "kid-1"),
	)
	if !errors.Is(err, auth.ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig when both secret and assertion key configured, got %v", err)
	}
}

// TestG3CrossCheckRejectsUnsupportedMethod verifies the G-3 discovery
// cross-check: when the server advertises token_endpoint_auth_methods_supported
// that does NOT include private_key_jwt, construction must fail with
// ErrInvalidConfig. Also verifies that when the list includes private_key_jwt
// (or is empty), construction succeeds (REQ-068).
func TestG3CrossCheckRejectsUnsupportedMethod(t *testing.T) { // REQ-068
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	// Negative case: server only advertises client_secret_basic — private_key_jwt absent.
	epReject := discovery.AuthEndpoints{
		AuthorizationEndpoint:             discovery.MustParseURL(srv.URL + "/authorize"),
		TokenEndpoint:                     discovery.MustParseURL(srv.URL + "/token"),
		TokenEndpointAuthMethodsSupported: []string{"client_secret_basic"},
	}
	_, err = smart.New(
		"client-id", epReject,
		smart.WithHTTPClient(srv.Client()),
		smart.WithClientAssertionKey(key, "RS384", "kid-1"),
	)
	if !errors.Is(err, auth.ErrInvalidConfig) {
		t.Fatalf("G-3 negative: expected ErrInvalidConfig when server does not advertise private_key_jwt, got %v", err)
	}

	// Positive case: server advertises private_key_jwt — construction must succeed.
	epAllow := discovery.AuthEndpoints{
		AuthorizationEndpoint:             discovery.MustParseURL(srv.URL + "/authorize"),
		TokenEndpoint:                     discovery.MustParseURL(srv.URL + "/token"),
		TokenEndpointAuthMethodsSupported: []string{"client_secret_basic", "private_key_jwt"},
	}
	_, err = smart.New(
		"client-id", epAllow,
		smart.WithHTTPClient(srv.Client()),
		smart.WithClientAssertionKey(key, "RS384", "kid-1"),
	)
	if err != nil {
		t.Fatalf("G-3 positive: expected no error when server advertises private_key_jwt, got %v", err)
	}

	// Positive case: empty advertised list is not constraining — must also succeed.
	epEmpty := discovery.AuthEndpoints{
		AuthorizationEndpoint: discovery.MustParseURL(srv.URL + "/authorize"),
		TokenEndpoint:         discovery.MustParseURL(srv.URL + "/token"),
	}
	_, err = smart.New(
		"client-id", epEmpty,
		smart.WithHTTPClient(srv.Client()),
		smart.WithClientAssertionKey(key, "RS384", "kid-1"),
	)
	if err != nil {
		t.Fatalf("G-3 positive (empty list): expected no error when methods list is empty, got %v", err)
	}
}

func TestTokenStaleWithoutRefreshDoesNotDeadlock(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	src, _ := smart.New("c", testAuthEndpoints(srv), smart.WithHTTPClient(srv.Client()), smart.WithRedirectURI("https://cb"))
	// Stale but NOT expired (within the 30s proactive-refresh threshold), no
	// refresh token: Token() returns the still-valid cached token without
	// deadlocking on concurrent calls.
	src.SetTokens(auth.Token{Value: "cached", Type: "Bearer", ExpiresAt: time.Now().Add(10 * time.Second)}, "")

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

// TestTokenExpiredWithoutRefreshReturnsReauthRequired asserts that a cached
// access token past its ExpiresAt with no refresh_token is NOT returned
// silently — Token() returns ErrReauthRequired (REQ-063).
func TestTokenExpiredWithoutRefreshReturnsReauthRequired(t *testing.T) { // REQ-063
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	src, _ := smart.New("c", testAuthEndpoints(srv), smart.WithHTTPClient(srv.Client()), smart.WithRedirectURI("https://cb"))
	src.SetTokens(auth.Token{Value: "stale", Type: "Bearer", ExpiresAt: time.Now().Add(-time.Minute)}, "")

	tok, err := src.Token(context.Background())
	if !errors.Is(err, auth.ErrReauthRequired) {
		t.Fatalf("Token() err = %v, want ErrReauthRequired", err)
	}
	if tok.Value != "" {
		t.Errorf("expected zero token, got %q", tok.Value)
	}
}

// ---------------------------------------------------------------------------
// REQ-063: F-L refresh clearing
// ---------------------------------------------------------------------------

// TestRefreshFailureClearsRefreshTokenOnlyWhenTerminal verifies that:
//   - a terminal refresh failure (400 invalid_grant) clears s.refresh so that
//     the next Token() call returns ErrReauthRequired without hitting the endpoint;
//   - a transient refresh failure (503) retains s.refresh and returns ErrRefreshFailed.
//
// (REQ-063)
func TestRefreshFailureClearsRefreshTokenOnlyWhenTerminal(t *testing.T) { // REQ-063
	t.Run("terminal_clears_refresh", func(t *testing.T) {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/token" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			calls++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"token revoked"}`))
		}))
		defer srv.Close()

		src, err := smart.New("c", testAuthEndpoints(srv), smart.WithHTTPClient(srv.Client()), smart.WithRedirectURI("https://cb"))
		if err != nil {
			t.Fatal(err)
		}
		// Seed a stale access token + a refresh token.
		src.SetTokens(auth.Token{Value: "at", Type: "Bearer", ExpiresAt: time.Now().Add(-time.Minute)}, "rt-1")

		// First call: should hit the endpoint, get 400 invalid_grant, classify as terminal.
		_, firstErr := src.Token(context.Background())
		if !errors.Is(firstErr, auth.ErrReauthRequired) {
			t.Fatalf("Token() after terminal refresh failure: got %v, want ErrReauthRequired", firstErr)
		}
		if calls != 1 {
			t.Fatalf("expected 1 endpoint call, got %d", calls)
		}

		// Second call: refresh token should be cleared — must return ErrReauthRequired
		// immediately without a second endpoint call.
		_, secondErr := src.Token(context.Background())
		if !errors.Is(secondErr, auth.ErrReauthRequired) {
			t.Fatalf("second Token() after terminal: got %v, want ErrReauthRequired", secondErr)
		}
		if calls != 1 {
			t.Fatalf("second Token() must not hit endpoint; calls = %d", calls)
		}
	})

	t.Run("transient_retains_refresh", func(t *testing.T) {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/token" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			calls++
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		src, err := smart.New("c", testAuthEndpoints(srv), smart.WithHTTPClient(srv.Client()), smart.WithRedirectURI("https://cb"))
		if err != nil {
			t.Fatal(err)
		}
		src.SetTokens(auth.Token{Value: "at", Type: "Bearer", ExpiresAt: time.Now().Add(-time.Minute)}, "rt-1")

		_, firstErr := src.Token(context.Background())
		if !errors.Is(firstErr, auth.ErrRefreshFailed) {
			t.Fatalf("Token() after 503: got %v, want ErrRefreshFailed", firstErr)
		}
		// s.cur and s.refresh are both retained after a transient failure — no
		// re-seed needed. A second Token() call must use the retained refresh
		// token and hit the endpoint again (proving s.refresh was not cleared).
		_, _ = src.Token(context.Background())
		if calls != 2 {
			t.Fatalf("expected 2 endpoint calls (refresh retained and reused), got %d", calls)
		}
	})
}

// ---------------------------------------------------------------------------
// REQ-063: RefreshIfNeeded
// ---------------------------------------------------------------------------

// TestRefreshIfNeeded verifies that:
//   - a fresh token produces no endpoint call (no-op);
//   - a stale token with a refresh token triggers a refresh POST.
//
// (REQ-063)
func TestRefreshIfNeeded(t *testing.T) { // REQ-063
	t.Run("fresh_no_call", func(t *testing.T) {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			t.Errorf("unexpected endpoint call to %s", r.URL.Path)
		}))
		defer srv.Close()

		src, err := smart.New("c", testAuthEndpoints(srv), smart.WithHTTPClient(srv.Client()), smart.WithRedirectURI("https://cb"))
		if err != nil {
			t.Fatal(err)
		}
		// Seed a non-stale token (expires well in the future).
		src.SetTokens(auth.Token{Value: "at", Type: "Bearer", ExpiresAt: time.Now().Add(time.Hour)}, "rt-1")

		if err := src.RefreshIfNeeded(context.Background()); err != nil {
			t.Fatalf("RefreshIfNeeded on fresh token: %v", err)
		}
		if calls != 0 {
			t.Fatalf("expected 0 endpoint calls, got %d", calls)
		}
	})

	t.Run("stale_triggers_refresh", func(t *testing.T) {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/token" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			calls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"at-new","token_type":"Bearer","expires_in":3600,"refresh_token":"rt-2"}`))
		}))
		defer srv.Close()

		src, err := smart.New("c", testAuthEndpoints(srv), smart.WithHTTPClient(srv.Client()), smart.WithRedirectURI("https://cb"))
		if err != nil {
			t.Fatal(err)
		}
		src.SetTokens(auth.Token{Value: "at", Type: "Bearer", ExpiresAt: time.Now().Add(-time.Minute)}, "rt-1")

		if err := src.RefreshIfNeeded(context.Background()); err != nil {
			t.Fatalf("RefreshIfNeeded on stale token: %v", err)
		}
		if calls != 1 {
			t.Fatalf("expected 1 endpoint call, got %d", calls)
		}
	})
}

// ---------------------------------------------------------------------------
// REQ-063: WithRefreshThreshold configurable early-expiry buffer
// ---------------------------------------------------------------------------

// TestRefreshThresholdConfigurable verifies that WithRefreshThreshold sets the
// proactive-refresh window: a token expiring within the configured threshold is
// considered stale and triggers a refresh POST (REQ-063 / G-2).
func TestRefreshThresholdConfigurable(t *testing.T) { // REQ-063
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at-new","token_type":"Bearer","expires_in":3600,"refresh_token":"rt-2"}`))
	}))
	defer srv.Close()

	src, err := smart.New(
		"c", testAuthEndpoints(srv),
		smart.WithHTTPClient(srv.Client()),
		smart.WithRedirectURI("https://cb"),
		smart.WithRefreshThreshold(2*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	// Token expires in 90s — fresh under the default 30s threshold, but stale
	// under the 2-minute threshold we configured.
	src.SetTokens(auth.Token{Value: "at", Type: "Bearer", ExpiresAt: time.Now().Add(90 * time.Second)}, "rt-1")

	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() = %v", err)
	}
	if tok.Value != "at-new" {
		t.Fatalf("expected refreshed token, got %q", tok.Value)
	}
	if calls != 1 {
		t.Fatalf("expected 1 endpoint call, got %d", calls)
	}
}

// ---------------------------------------------------------------------------
// REQ-063: Reauth
// ---------------------------------------------------------------------------

// TestSourceReauthForcesRefresh verifies that Reauth forces a refresh POST even
// when the current token is not yet stale, updates the cached token, and that
// *smart.Source satisfies the auth.Reauther interface (REQ-063).
func TestSourceReauthForcesRefresh(t *testing.T) { // REQ-063
	// Compile-time interface check.
	var _ auth.Reauther = (*smart.Source)(nil)

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at-reauthed","token_type":"Bearer","expires_in":3600,"refresh_token":"rt-2"}`))
	}))
	defer srv.Close()

	src, err := smart.New("c", testAuthEndpoints(srv), smart.WithHTTPClient(srv.Client()), smart.WithRedirectURI("https://cb"))
	if err != nil {
		t.Fatal(err)
	}
	// Seed a non-stale token — Token() would normally return it without a network call.
	src.SetTokens(auth.Token{Value: "at-old", Type: "Bearer", ExpiresAt: time.Now().Add(time.Hour)}, "rt-1")

	// Reauth must ignore freshness and force a refresh POST.
	if err := src.Reauth(context.Background()); err != nil {
		t.Fatalf("Reauth: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 endpoint call from Reauth, got %d", calls)
	}
	// After Reauth, Token() should return the new token without another call.
	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() after Reauth: %v", err)
	}
	if tok.Value != "at-reauthed" {
		t.Fatalf("expected at-reauthed, got %q", tok.Value)
	}
	if calls != 1 {
		t.Fatalf("Token() after Reauth must not make an extra call; calls = %d", calls)
	}
}

// TestSourceReauthNoRefreshTokenKeepsValidToken verifies that, when no
// refresh_token is available, Reauth signals re-authentication WITHOUT
// discarding a cached access token that is still within its ExpiresAt — a wire
// 401 may be scope-related, and a public client has no other credential to fall
// back on (REQ-063).
func TestSourceReauthNoRefreshTokenKeepsValidToken(t *testing.T) { // REQ-063
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("token endpoint must not be called without a refresh_token; got %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	src, err := smart.New("c", testAuthEndpoints(srv), smart.WithHTTPClient(srv.Client()), smart.WithRedirectURI("https://cb"))
	if err != nil {
		t.Fatal(err)
	}
	// Still-valid token, no refresh_token.
	src.SetTokens(auth.Token{Value: "at-valid", Type: "Bearer", ExpiresAt: time.Now().Add(time.Hour)}, "")

	if err := src.Reauth(context.Background()); !errors.Is(err, auth.ErrReauthRequired) {
		t.Fatalf("Reauth err = %v, want ErrReauthRequired", err)
	}
	// The still-valid cached token must survive — Token() returns it as-is.
	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() after Reauth: %v", err)
	}
	if tok.Value != "at-valid" {
		t.Fatalf("cached token was discarded: got %q, want at-valid", tok.Value)
	}
}
