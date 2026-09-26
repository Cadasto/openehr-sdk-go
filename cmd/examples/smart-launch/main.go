// Walk through a standalone SMART-on-openEHR launch for a public client: the
// OAuth 2.0 authorization-code flow with PKCE, which is how a browser-based or
// native app that cannot keep a client secret obtains an access token. The
// program plays every party in turn: the app, the user's browser, and the
// authorization server.
//
// The one thing to take away is where the AuthorizationRequest lives. It
// carries the CSRF state and the PKCE verifier, both created before the
// redirect and both needed after it, so your app has to store it across the
// redirect and look it up again on the callback. Everything else is one SDK
// call per step.
//
// It runs offline: an in-process stub plays the authorization server, so no
// account, secret or network is needed. `go test ./cmd/examples/smart-launch`
// runs the same flow.
//
//	go run ./cmd/examples/smart-launch
package main

import (
	"context"
	"encoding/json"
	"errors"
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

const (
	// clientID is the identifier the authorization server knows the app by.
	clientID = "my-public-app"
	// redirectURI is the app's callback; the browser lands here with the code.
	redirectURI = "https://app.example/callback"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run drives the whole launch against the in-process stub. main_test.go calls
// it too, so the flow is checked by `go test` as well as by `go run`.
func run() error {
	// The stub authorization server. Its two endpoints are the ones a real
	// SMART server publishes in its discovery document: /authorize, where the
	// user logs in, and /token, where the app trades a code for tokens.
	stub := newStubServer()
	server := httptest.NewServer(stub.mux)
	defer server.Close()

	endpoints := discovery.AuthEndpoints{
		AuthorizationEndpoint: discovery.MustParseURL(server.URL + "/authorize"),
		TokenEndpoint:         discovery.MustParseURL(server.URL + "/token"),
	}

	// Step 1: build the Source for a public client. There is no
	// WithClientSecret: PKCE alone proves that the app exchanging the code is
	// the one that started the launch. The *http.Client is injected, as
	// everywhere in the SDK; here it is the one that reaches the stub.
	source, err := authsmart.New(
		clientID,
		endpoints,
		authsmart.WithHTTPClient(server.Client()),
		authsmart.WithRedirectURI(redirectURI),
		authsmart.WithScopes(
			auth.ScopeOpenID,
			auth.ScopeLaunchPatient, // asks for the ehrId claim on openEHR servers
			auth.ScopeOfflineAccess, // asks for a refresh token
		),
	)
	if err != nil {
		return fmt.Errorf("smart.New: %w", err)
	}
	fmt.Println("step 1: Source built (public client, PKCE, standalone)")

	// Step 2: begin the launch. The empty state asks the SDK to generate a
	// random one; the returned AuthorizationRequest also holds the fresh PKCE
	// verifier and challenge. A standalone launch has no "launch" token from
	// an EHR session, so nothing else goes in.
	authReq, err := source.BeginAuthorization("")
	if err != nil {
		return fmt.Errorf("BeginAuthorization: %w", err)
	}
	fmt.Printf("step 2: BeginAuthorization → state=%q  verifier=%q\n",
		authReq.State, preview(authReq.PKCE.Verifier))

	// Step 3: the URL the browser is sent to. It carries the client id, the
	// redirect URI, the scopes, the state and the PKCE challenge (never the
	// verifier). The empty launch argument again means standalone.
	authorizeURL, err := source.AuthorizeURL(authReq, "")
	if err != nil {
		return fmt.Errorf("AuthorizeURL: %w", err)
	}
	fmt.Printf("step 3: authorize URL built (len=%d)\n", len(authorizeURL))

	// Step 4: store the AuthorizationRequest before redirecting. This is the
	// step integrations get wrong. After the redirect the app receives only a
	// code and a state from the browser; the PKCE verifier that the token
	// endpoint will demand exists nowhere but in this value. The state lets
	// the callback find it again, but it only protects against a forged
	// callback (CSRF) if it is tied to the browser that started the launch:
	// a real app stores the request in that user's own session (for example
	// behind a session cookie set here) and checks it on the callback, and it
	// expires pending entries. A process-wide map keyed by state, as below,
	// keeps the demo short and is not enough on its own.
	sessions := newSessionStore()
	sessions.save(authReq)
	fmt.Printf("step 4: AuthorizationRequest stored in session map (key=%q)\n", authReq.State)

	// Step 5: the user's turn. In a real app the browser opens authorizeURL,
	// the user logs in, and the authorization server redirects the browser to
	// the app's callback with ?code=...&state=... in the query. The stub skips
	// the login screen and answers at once, so the program plays the browser.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	callbackCode, callbackState, err := openAuthorizeURL(ctx, server.Client(), authorizeURL)
	if err != nil {
		return fmt.Errorf("play the user's browser: %w", err)
	}
	fmt.Printf("step 5: redirect received  code=%q  state=%q\n", callbackCode, callbackState)

	// Step 6: the callback. Look the request up by the state the browser
	// brought back, and remove it in the same move so a replayed callback
	// finds nothing. An unknown state is rejected before any code is exchanged.
	storedReq, found := sessions.take(callbackState)
	if !found {
		return fmt.Errorf("callback state %q not in session map (possible CSRF)", callbackState)
	}
	fmt.Println("step 6: AuthorizationRequest retrieved from session map (state validated)")

	// Step 7: exchange the code for tokens. The SDK compares callbackState with
	// storedReq.State once more (ErrLaunchInvalidState on mismatch, before any
	// network call) and then posts the code together with the PKCE verifier to
	// the token endpoint. The server hashes the verifier and checks it against
	// the challenge it saw in step 5; that is the PKCE proof.
	token, tokenResp, err := source.ExchangeAuthorizationCode(ctx, callbackCode, callbackState, storedReq)
	if err != nil {
		return fmt.Errorf("ExchangeAuthorizationCode: %w", err)
	}
	// Tokens and the verifier are secrets: print only a short prefix, and
	// never write the full values to a log in a real app.
	fmt.Println("step 7: token exchange complete")
	fmt.Printf("  access_token : %s\n", preview(token.Value))
	fmt.Printf("  token_type   : %s\n", token.Type)
	fmt.Printf("  scope        : %s\n", token.Scope)
	fmt.Printf("  expires_at   : %s\n", token.ExpiresAt.UTC().Format(time.RFC3339))
	fmt.Printf("  refresh_token: %s\n", preview(tokenResp.RefreshToken))
	if tokenResp.EHRID != "" {
		// openEHR's launch context: the EHR the user selected, carried as the
		// ehrId claim because launch/patient was requested.
		fmt.Printf("  ehrId        : %s\n", tokenResp.EHRID)
	}
	fmt.Println("OK: standalone SMART PKCE launch flow completed (in-process stub)")
	return nil
}

// sessionStore is the app-side storage that bridges the redirect: it keeps
// each pending AuthorizationRequest under its state. The mutex matters
// because in a real app the redirect and the callback are different HTTP
// requests, usually served on different goroutines.
type sessionStore struct {
	mu      sync.Mutex
	pending map[string]authsmart.AuthorizationRequest // state → request
}

func newSessionStore() *sessionStore {
	return &sessionStore{pending: make(map[string]authsmart.AuthorizationRequest)}
}

// save stores the request under its own state, which is the key the callback
// will bring back.
func (s *sessionStore) save(req authsmart.AuthorizationRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[req.State] = req
}

// take returns the request stored under state and forgets it, so each state
// can complete a launch exactly once.
func (s *sessionStore) take(state string) (authsmart.AuthorizationRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	req, ok := s.pending[state]
	if ok {
		delete(s.pending, state)
	}
	return req, ok
}

// openAuthorizeURL plays the user's browser: it opens the authorize URL
// and returns the code and state the authorization server sends back to the
// app's callback.
func openAuthorizeURL(ctx context.Context, client *http.Client, authorizeURL string) (code, state string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authorizeURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("build authorize request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("GET authorize: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("authorize returned %d", resp.StatusCode)
	}
	var callback struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&callback); err != nil {
		return "", "", fmt.Errorf("decode authorize response: %w", err)
	}
	if callback.Code == "" || callback.State == "" {
		return "", "", errors.New("authorize response is missing code or state")
	}
	return callback.Code, callback.State, nil
}

// stubServer is a minimal authorization server, just enough to complete one
// launch:
//   - GET  /authorize issues a one-use code bound to the state it was given.
//   - POST /token     accepts that code once and returns a canned token.
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

// handleAuthorize stands in for the login screen. A real server authenticates
// the user and then redirects the browser to the app's redirect_uri with code
// and state in the query. The stub grants at once and returns the pair as
// JSON, so openAuthorizeURL can read it without following a redirect.
func (s *stubServer) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		http.Error(w, "missing state", http.StatusBadRequest)
		return
	}
	// A predictable code keeps the output readable; real ones are random.
	code := "stub-code-" + state[:8]

	s.mu.Lock()
	s.codes[code] = state
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	// An encode failure would surface as a decode error in the caller.
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "state": state})
}

// handleToken is the token endpoint. It checks the grant type and that the
// code was issued and not used before, then returns a canned token. A real
// server also hashes the code_verifier from the form and compares it with the
// code_challenge it saw on /authorize; the stub skips that check.
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
		delete(s.codes, code) // one use only
	}
	s.mu.Unlock()

	if !ok {
		http.Error(w, "unknown code", http.StatusBadRequest)
		return
	}
	// The canned response mirrors a real SMART-on-openEHR deployment:
	// access_token, refresh_token, scope, and the ehrId launch-context claim.
	scope := form.Get("scope")
	if scope == "" {
		scope = strings.Join([]string{auth.ScopeOpenID, auth.ScopeLaunchPatient, auth.ScopeOfflineAccess}, " ")
	}
	w.Header().Set("Content-Type", "application/json")
	// An encode failure would surface as a decode error in the SDK.
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token":  "stub-access-token-001",
		"token_type":    "Bearer",
		"expires_in":    3600,
		"refresh_token": "stub-refresh-token-001",
		"scope":         scope,
		"ehrId":         "00000000-0000-0000-0000-000000000001",
	})
}

// preview returns the first eight characters of a secret followed by an
// ellipsis, enough to tell two values apart without revealing either.
func preview(secret string) string {
	const shown = 8
	if len(secret) <= shown {
		return secret
	}
	return secret[:shown] + "…"
}
