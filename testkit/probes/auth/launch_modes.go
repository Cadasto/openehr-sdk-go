package authprobes

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/clientcreds"
	"github.com/cadasto/openehr-sdk-go/auth/jwtbearer"
	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// LaunchModeStandalone proves the SMART standalone launch mode (REQ-068):
// the SDK initiates the launch by building an authorization URL with NO
// EHR-side `launch` parameter, while still carrying response_type=code and
// the PKCE challenge.
func LaunchModeStandalone(_ context.Context) (Result, error) { // REQ-068
	r := Result{Probe: "LAUNCH-standalone"}
	authURL, err := authorizeURLForLaunch("")
	if err != nil {
		return r, fmt.Errorf("standalone: %w", err)
	}
	q := authURL.Query()
	if q.Has("launch") {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("standalone authorize URL carries a launch param (%q); standalone MUST omit it", q.Get("launch"))
		return r, nil
	}
	if q.Get("response_type") != "code" {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("response_type = %q; want code", q.Get("response_type"))
		return r, nil
	}
	if q.Get("code_challenge_method") != "S256" {
		r.Status = "fail"
		r.Detail = "standalone authorize URL is missing the S256 PKCE challenge"
		return r, nil
	}
	r.Status = "pass"
	r.Detail = "standalone launch: authorize URL carries code+PKCE, no launch param"
	return r, nil
}

// LaunchModeEmbedded proves the SMART embedded (EHR) launch mode
// (REQ-068): an EHR-supplied `launch` parameter is forwarded verbatim to
// the authorization endpoint.
func LaunchModeEmbedded(_ context.Context) (Result, error) { // REQ-068
	r := Result{Probe: "LAUNCH-embedded"}
	const launch = "ehr-launch-token-abc"
	authURL, err := authorizeURLForLaunch(launch)
	if err != nil {
		return r, fmt.Errorf("embedded: %w", err)
	}
	q := authURL.Query()
	if q.Get("launch") != launch {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("embedded authorize URL launch = %q; want %q forwarded verbatim", q.Get("launch"), launch)
		return r, nil
	}
	if q.Get("response_type") != "code" {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("response_type = %q; want code", q.Get("response_type"))
		return r, nil
	}
	r.Status = "pass"
	r.Detail = "embedded launch: EHR launch parameter forwarded to the authorization endpoint"
	return r, nil
}

// authorizeURLForLaunch builds a SMART authorization URL for the given
// launch parameter (empty == standalone). Shared by the launch-mode
// probes; uses a real auth/smart.Source.
func authorizeURLForLaunch(launch string) (*url.URL, error) {
	authEP := discovery.AuthEndpoints{
		AuthorizationEndpoint: discovery.MustParseURL("https://auth.launch.example/authorize"),
		TokenEndpoint:         discovery.MustParseURL("https://auth.launch.example/token"),
	}
	src, err := authsmart.New(
		"launch-client",
		authEP,
		authsmart.WithHTTPClient(http.DefaultClient),
		authsmart.WithRedirectURI("https://app.launch.example/callback"),
		authsmart.WithScopes("openid", "launch", "patient/COMPOSITION.read"),
	)
	if err != nil {
		return nil, fmt.Errorf("build Source: %w", err)
	}
	areq, err := src.BeginAuthorization("")
	if err != nil {
		return nil, fmt.Errorf("BeginAuthorization: %w", err)
	}
	raw, err := src.AuthorizeURL(areq, launch)
	if err != nil {
		return nil, fmt.Errorf("AuthorizeURL: %w", err)
	}
	return url.Parse(raw)
}

// LaunchModeBackend proves the SMART backend-service launch mode
// (REQ-068): no user interaction, no launch context. It exercises three
// confidential backend flows and asserts the token request each produces
// on the wire:
//
//   - client_credentials + client_secret (symmetric, HTTP Basic)
//   - client_credentials + private_key_jwt (asymmetric, SMART Backend
//     Services — signed client_assertion, no Basic, no client_secret)
//   - JWT Bearer grant (RFC 7523 — the JWT is the authorization grant)
func LaunchModeBackend(ctx context.Context) (Result, error) { // REQ-068
	r := Result{Probe: "LAUNCH-backend"}

	// --- 1. client_credentials with a symmetric client_secret. ---
	secretForm, secretAuthHdr, err := captureBackendToken(ctx, func(tokenURL string, hc *http.Client) (tokenFetcher, error) {
		return clientcreds.New(
			"backend-client", "s3cr3t", tokenURL,
			clientcreds.WithHTTPClient(hc),
			clientcreds.WithScope("system/COMPOSITION.read"),
		)
	})
	if err != nil {
		return r, fmt.Errorf("backend(client_secret): %w", err)
	}
	if secretForm.Get("grant_type") != "client_credentials" {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("client_secret backend grant_type = %q; want client_credentials", secretForm.Get("grant_type"))
		return r, nil
	}
	if secretAuthHdr == "" {
		r.Status = "fail"
		r.Detail = "client_secret backend did not send an HTTP Basic Authorization header"
		return r, nil
	}

	// --- 2. client_credentials with an asymmetric private_key_jwt. ---
	signer, err := backendSigner("backend-client", "https://auth.backend.example/token")
	if err != nil {
		return r, fmt.Errorf("backend(private_key_jwt): build signer: %w", err)
	}
	pkjForm, pkjAuthHdr, err := captureBackendToken(ctx, func(tokenURL string, hc *http.Client) (tokenFetcher, error) {
		// The signer's audience is fixed at build time to the canonical
		// token URL; the in-process server accepts any audience, so the
		// wire-shape assertions below are what matter.
		return clientcreds.New(
			"backend-client", "", tokenURL,
			clientcreds.WithHTTPClient(hc),
			clientcreds.WithClientAssertion(signer),
		)
	})
	if err != nil {
		return r, fmt.Errorf("backend(private_key_jwt): %w", err)
	}
	if pkjForm.Get("grant_type") != "client_credentials" {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("private_key_jwt backend grant_type = %q; want client_credentials", pkjForm.Get("grant_type"))
		return r, nil
	}
	if pkjForm.Get("client_assertion_type") != "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("private_key_jwt backend client_assertion_type = %q; want jwt-bearer", pkjForm.Get("client_assertion_type"))
		return r, nil
	}
	if pkjForm.Get("client_assertion") == "" {
		r.Status = "fail"
		r.Detail = "private_key_jwt backend sent no signed client_assertion"
		return r, nil
	}
	if pkjAuthHdr != "" {
		r.Status = "fail"
		r.Detail = "private_key_jwt backend sent an HTTP Basic header; SMART Backend Services MUST omit it"
		return r, nil
	}
	if pkjForm.Get("client_secret") != "" {
		r.Status = "fail"
		r.Detail = "private_key_jwt backend leaked a client_secret form field"
		return r, nil
	}

	// --- 3. JWT Bearer grant (RFC 7523). ---
	bearerSigner, err := backendSigner("bearer-client", "https://auth.backend.example/token")
	if err != nil {
		return r, fmt.Errorf("backend(jwt-bearer): build signer: %w", err)
	}
	jbForm, _, err := captureBackendToken(ctx, func(tokenURL string, hc *http.Client) (tokenFetcher, error) {
		return jwtbearer.New(
			tokenURL, bearerSigner,
			jwtbearer.WithHTTPClient(hc),
			jwtbearer.WithScope("system/COMPOSITION.read"),
		)
	})
	if err != nil {
		return r, fmt.Errorf("backend(jwt-bearer): %w", err)
	}
	if jbForm.Get("grant_type") != jwtbearer.GrantType {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("jwt-bearer grant_type = %q; want %q", jbForm.Get("grant_type"), jwtbearer.GrantType)
		return r, nil
	}
	if jbForm.Get("assertion") == "" {
		r.Status = "fail"
		r.Detail = "jwt-bearer grant sent no assertion"
		return r, nil
	}

	r.Status = "pass"
	r.Detail = "backend launch: client_secret (Basic), private_key_jwt (client_assertion), and jwt-bearer (assertion) all produced the expected token requests"
	return r, nil
}

// tokenFetcher is the minimal auth.TokenSource surface the backend probe
// needs (Token only).
type tokenFetcher interface {
	Token(ctx context.Context) (auth.Token, error)
}

// captureBackendToken builds a backend token source via newSource against
// an in-process token endpoint, drives one Token() call, and returns the
// captured request form and Authorization header.
func captureBackendToken(ctx context.Context, newSource func(tokenURL string, hc *http.Client) (tokenFetcher, error)) (url.Values, string, error) {
	var (
		mu       sync.Mutex
		gotForm  url.Values
		gotAuth  string
		captured bool
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if err := req.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		gotForm = req.Form
		gotAuth = req.Header.Get("Authorization")
		captured = true
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"backend-at","token_type":"Bearer","expires_in":3600}`))
	}))
	defer srv.Close()

	src, err := newSource(srv.URL+"/token", srv.Client())
	if err != nil {
		return nil, "", fmt.Errorf("build source: %w", err)
	}
	if _, err := src.Token(ctx); err != nil {
		return nil, "", fmt.Errorf("Token: %w", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !captured {
		return nil, "", errors.New("token endpoint never received a request")
	}
	return gotForm, gotAuth, nil
}

// backendSigner builds a jwtbearer.ClaimsSigner (RS384) for backend
// client-assertion / jwt-bearer flows.
func backendSigner(clientID, tokenURL string) (*jwtbearer.ClaimsSigner, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	return jwtbearer.NewClaimsSigner(
		jwtbearer.ClaimsTemplate{
			Issuer:   clientID,
			Subject:  clientID,
			Audience: tokenURL,
			Lifetime: 5 * time.Minute,
		},
		key,
		jwtbearer.WithKeyID("backend-kid"),
	)
}
