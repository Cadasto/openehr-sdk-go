// Example: standalone SMART-on-openEHR launch with PKCE (public client).
//
// Demonstrates the #1 integration gotcha: the [auth/smart.AuthorizationRequest]
// (which carries both the CSRF state and the PKCE code_verifier) must be
// persisted across the HTTP redirect — between [Source.BeginAuthorization] /
// [Source.AuthorizeURL] and the redirect callback that handles [Source.ExchangeAuthorizationCode].
//
// The example is self-contained: it spins up an in-process SMART stub server
// (authorize + token endpoints) so `go run ./cmd/examples/smart-launch` and
// `go test ./cmd/examples/smart-launch` both work offline without secrets.
//
// Run: `go run ./cmd/examples/smart-launch` from any directory.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

func main() {
	if err := runFlow(); err != nil {
		log.Fatalf("smart-launch example failed: %v", err)
	}
}

// runFlow executes the full standalone PKCE launch against an in-process stub
// and returns an error on any failure.  It is also called from main_test.go.
func runFlow() error {
	// -------------------------------------------------------------------------
	// 1. Spin up an in-process SMART stub (authorize + token endpoints).
	// -------------------------------------------------------------------------
	stub := newStubServer()
	srv := httptest.NewServer(stub.mux)
	defer srv.Close()

	// Build AuthEndpoints pointing at the stub.
	authEP := discovery.AuthEndpoints{
		AuthorizationEndpoint: discovery.MustParseURL(srv.URL + "/authorize"),
		TokenEndpoint:         discovery.MustParseURL(srv.URL + "/token"),
	}

	const (
		clientID    = "my-public-app"
		redirectURI = "https://app.example/callback"
	)

	// -------------------------------------------------------------------------
	// 2. Build a Source for a PUBLIC client (PKCE, no client secret).
	//
	// Public clients omit WithClientSecret and WithClientAssertionKey — the
	// PKCE code_verifier alone authenticates the exchange (REQ-061).
	// Use the stub's *http.Client so requests reach the in-process server.
	// -------------------------------------------------------------------------
	src, err := authsmart.New(
		clientID,
		authEP,
		authsmart.WithHTTPClient(srv.Client()),
		authsmart.WithRedirectURI(redirectURI),
		authsmart.WithScopes(
			auth.ScopeOpenID,
			auth.ScopeLaunchPatient, // triggers ehrId claim in openEHR deployments
			auth.ScopeOfflineAccess, // requests a refresh_token
		),
	)
	if err != nil {
		return fmt.Errorf("smart.New: %w", err)
	}
	fmt.Println("step 1: Source built (public client, PKCE, standalone)")

	// -------------------------------------------------------------------------
	// 3. Begin the authorization — generates a random state and PKCE pair.
	//
	// Passing "" lets the SDK generate a cryptographically random state value
	// (32 bytes of entropy); for standalone launches there is no EHR-context
	// "launch" token to embed in the authorize URL.
	// -------------------------------------------------------------------------
	authReq, err := src.BeginAuthorization("")
	if err != nil {
		return fmt.Errorf("BeginAuthorization: %w", err)
	}
	fmt.Printf("step 2: BeginAuthorization → state=%q  verifier=%q…\n",
		authReq.State, authReq.PKCE.Verifier[:8]+"…")

	// -------------------------------------------------------------------------
	// 4. Build the redirect URL (standalone — no launch parameter).
	// -------------------------------------------------------------------------
	authorizeURL, err := src.AuthorizeURL(authReq, "" /* no launch param → standalone */)
	if err != nil {
		return fmt.Errorf("AuthorizeURL: %w", err)
	}
	fmt.Printf("step 3: authorize URL built (len=%d)\n", len(authorizeURL))

	// -------------------------------------------------------------------------
	// 5. PERSIST THE AuthorizationRequest ACROSS THE REDIRECT.
	//
	// KEY INTEGRATION POINT — this is the #1 gotcha for new integrators:
	//
	//   - authReq.State        is the CSRF guard returned to the callback.
	//   - authReq.PKCE.Verifier is the secret the token endpoint needs.
	//
	// In a real web app the user-agent (browser) navigates to authorizeURL,
	// authenticates at the authorization server, and is redirected back to the
	// app's callback URL with ?code=…&state=… query parameters.
	// The app's server MUST retrieve the stored AuthorizationRequest and verify
	// that the returned state matches before exchanging the code.
	//
	// A plain in-memory map works for single-server deployments; distributed
	// apps should use a shared session store (Redis, DB, encrypted cookie) keyed
	// by state.
	// -------------------------------------------------------------------------
	var (
		sessionMu sync.Mutex
		sessions  = make(map[string]authsmart.AuthorizationRequest) // state → AuthorizationRequest
	)

	// Store the AuthorizationRequest BEFORE redirecting the user.
	sessionMu.Lock()
	sessions[authReq.State] = authReq
	sessionMu.Unlock()
	fmt.Printf("step 4: AuthorizationRequest stored in session map (key=%q)\n", authReq.State)

	// -------------------------------------------------------------------------
	// 6. Simulate the user-agent navigating to the authorize URL and the
	//    authorization server redirecting back with a code and the original state.
	//
	// The stub parses state from the authorize URL, stores a code bound to it,
	// and would normally 302-redirect the browser.  Here we drive the exchange
	// inline to keep the example self-contained.
	// -------------------------------------------------------------------------
	callbackCode, callbackState, err := simulateUserAuth(srv.Client(), authorizeURL, stub)
	if err != nil {
		return fmt.Errorf("simulate user auth: %w", err)
	}
	fmt.Printf("step 5: redirect received  code=%q  state=%q\n", callbackCode, callbackState)

	// -------------------------------------------------------------------------
	// 7. Look up the stored AuthorizationRequest by the returned state.
	//
	// KEY INTEGRATION POINT — look up by state, NOT by some other session key:
	//   1. Retrieve the stored AuthorizationRequest using callbackState as the key.
	//   2. Pass it (unchanged) to ExchangeAuthorizationCode.
	//   3. ExchangeAuthorizationCode re-validates state internally and sends
	//      authReq.PKCE.Verifier to the token endpoint.
	//
	// If the state is unknown or has already been used (replay), reject the
	// callback immediately — do NOT exchange the code.
	// -------------------------------------------------------------------------
	sessionMu.Lock()
	storedReq, found := sessions[callbackState]
	if found {
		delete(sessions, callbackState) // consume once — prevents replay
	}
	sessionMu.Unlock()

	if !found {
		return fmt.Errorf("callback state %q not in session map (possible CSRF)", callbackState)
	}
	fmt.Printf("step 6: AuthorizationRequest retrieved from session map (state validated)\n")

	// -------------------------------------------------------------------------
	// 8. Exchange the authorization code for tokens.
	//
	// ExchangeAuthorizationCode re-validates callbackState == storedReq.State
	// internally (returns ErrLaunchInvalidState on mismatch) and then POSTs the
	// PKCE code_verifier to the token endpoint — completing the PKCE proof.
	// -------------------------------------------------------------------------
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tok, tr, err := src.ExchangeAuthorizationCode(ctx, callbackCode, callbackState, storedReq)
	if err != nil {
		return fmt.Errorf("ExchangeAuthorizationCode: %w", err)
	}

	fmt.Printf("step 7: token exchange complete\n")
	fmt.Printf("  access_token : %s\n", tok.Value)
	fmt.Printf("  token_type   : %s\n", tok.Type)
	fmt.Printf("  scope        : %s\n", tok.Scope)
	fmt.Printf("  expires_at   : %s\n", tok.ExpiresAt.UTC().Format(time.RFC3339))
	fmt.Printf("  refresh_token: %s\n", tr.RefreshToken)
	if tr.EHRID != "" {
		fmt.Printf("  ehrId        : %s\n", tr.EHRID)
	}
	fmt.Println("OK: standalone SMART PKCE launch flow completed (in-process stub)")
	return nil
}

// ---------------------------------------------------------------------------
// In-process SMART stub
// ---------------------------------------------------------------------------

// stubServer is a minimal in-process authorization server for the example.
// It implements:
//   - GET  /authorize — issues a one-use code bound to the state parameter.
//   - POST /token     — verifies grant_type + code, returns a canned token.
type stubServer struct {
	mux *http.ServeMux

	mu    sync.Mutex
	codes map[string]string // code → state
}

func newStubServer() *stubServer {
	s := &stubServer{
		mux:   http.NewServeMux(),
		codes: make(map[string]string),
	}
	s.mux.HandleFunc("/authorize", s.handleAuthorize)
	s.mux.HandleFunc("/token", s.handleToken)
	return s
}

// handleAuthorize issues a one-use authorization code bound to the state
// parameter from the redirect URL.  In a real server the user would
// authenticate here; we grant immediately.
func (s *stubServer) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		http.Error(w, "missing state", http.StatusBadRequest)
		return
	}
	// Issue a deterministic code so the caller can predict it in tests.
	code := "stub-code-" + state[:8]

	s.mu.Lock()
	s.codes[code] = state
	s.mu.Unlock()

	// In a real browser flow: w.Header().Set("Location", redirectURI+"?code="+code+"&state="+state)
	// Return the code+state in JSON so simulateUserAuth can parse them without
	// following an HTTP redirect.
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"code":  code,
		"state": state,
	})
}

// handleToken validates grant_type=authorization_code, verifies the code
// exists, and returns a canned token response (with refresh_token and ehrId).
func (s *stubServer) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		http.Error(w, "parse form", http.StatusBadRequest)
		return
	}
	if form.Get("grant_type") != "authorization_code" {
		http.Error(w, "unsupported grant_type", http.StatusBadRequest)
		return
	}
	code := form.Get("code")

	s.mu.Lock()
	_, ok := s.codes[code]
	if ok {
		delete(s.codes, code) // one-use
	}
	s.mu.Unlock()

	if !ok {
		http.Error(w, "unknown code", http.StatusBadRequest)
		return
	}
	// Return a canned token that mirrors a real SMART openEHR deployment:
	// access_token, refresh_token, scope, ehrId (openEHR launch context claim).
	scope := form.Get("scope")
	if scope == "" {
		scope = strings.Join([]string{auth.ScopeOpenID, auth.ScopeLaunchPatient, auth.ScopeOfflineAccess}, " ")
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token":  "stub-access-token-001",
		"token_type":    "Bearer",
		"expires_in":    3600,
		"refresh_token": "stub-refresh-token-001",
		"scope":         scope,
		"ehrId":         "00000000-0000-0000-0000-000000000001", // openEHR patient context
	})
}

// simulateUserAuth drives the authorize endpoint (as the user-agent would) and
// returns the code and state the authorization server would have redirected back.
func simulateUserAuth(client *http.Client, authorizeURL string, _ *stubServer) (code, state string, err error) {
	resp, err := client.Get(authorizeURL) //nolint:noctx // example only — no ctx required
	if err != nil {
		return "", "", fmt.Errorf("GET authorize: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("authorize returned %d", resp.StatusCode)
	}
	var payload map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", fmt.Errorf("decode authorize response: %w", err)
	}
	code = payload["code"]
	state = payload["state"]
	if code == "" || state == "" {
		return "", "", fmt.Errorf("authorize response missing code or state: %v", payload)
	}
	return code, state, nil
}
