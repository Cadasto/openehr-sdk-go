package authprobes

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"

	xoauth2 "golang.org/x/oauth2"

	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// pkceFlowCapture records the wire artefacts of a single SMART
// authorization-code + PKCE launch: the authorization-request query
// parameters (from AuthorizeURL) and the token-request form fields
// (captured at the httptest token endpoint).
type pkceFlowCapture struct {
	authQuery url.Values
	tokenForm url.Values
	verifier  string
	challenge string
	// tokenResp is the parsed token-endpoint response the SDK returned
	// from ExchangeAuthorizationCode.
	tokenResp authsmart.TokenResponse
}

// runPKCEFlow drives a full SMART authorization-code + PKCE launch against
// an in-process token endpoint and returns the captured wire artefacts.
// scopes is the configured scope request; launch, when non-empty, is the
// EHR-launch parameter forwarded to the authorization endpoint.
//
// The token endpoint records the inbound form and replies with a minimal
// success body that echoes the requested scope (mirroring an authorization
// server that grants exactly what was asked).
func runPKCEFlow(ctx context.Context, scopes []string, launch string) (*pkceFlowCapture, error) {
	cap := &pkceFlowCapture{}
	var mu sync.Mutex

	// The authorization server grants the scope bound to the code at
	// authorize time. SMART authorization-code exchange does not resend
	// `scope` on the token request, so the granted scope is carried back
	// on the token response — that is the round-trip PROBE-005 asserts.
	grantedScope := strings.Join(scopes, " ")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if err := req.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		cap.tokenForm = req.Form
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":"at-1","token_type":"Bearer","expires_in":3600,"scope":%q,"refresh_token":"rt-1"}`, grantedScope)
	}))

	authEP := discovery.AuthEndpoints{
		AuthorizationEndpoint: discovery.MustParseURL("https://auth.probe.example/authorize"),
		TokenEndpoint:         discovery.MustParseURL(srv.URL + "/token"),
	}
	src, err := authsmart.New(
		"probe-client",
		authEP,
		authsmart.WithHTTPClient(srv.Client()),
		authsmart.WithRedirectURI("https://app.probe.example/callback"),
		authsmart.WithScopes(scopes...),
		authsmart.WithAudience("https://api.probe.example/openehr/v1"),
	)
	if err != nil {
		srv.Close()
		return nil, fmt.Errorf("build Source: %w", err)
	}

	areq, err := src.BeginAuthorization("")
	if err != nil {
		srv.Close()
		return nil, fmt.Errorf("BeginAuthorization: %w", err)
	}
	cap.verifier = areq.PKCE.Verifier
	cap.challenge = areq.PKCE.Challenge

	authURL, err := src.AuthorizeURL(areq, launch)
	if err != nil {
		srv.Close()
		return nil, fmt.Errorf("AuthorizeURL: %w", err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		srv.Close()
		return nil, fmt.Errorf("parse authorize URL: %w", err)
	}
	cap.authQuery = parsed.Query()

	// The redirect callback returns the issued code and the same state.
	_, tr, err := src.ExchangeAuthorizationCode(ctx, "auth-code-xyz", areq.State, areq)
	if err != nil {
		srv.Close()
		return nil, fmt.Errorf("ExchangeAuthorizationCode: %w", err)
	}
	cap.tokenResp = tr
	srv.Close()
	return cap, nil
}

// Probe004PKCEVerifierRoundTrip implements PROBE-004: a SMART launch using
// S256 PKCE carries code_challenge + code_challenge_method=S256 on the
// authorization request and code_verifier on the token exchange, and the
// token response is a 200 carrying an access_token (REQ-061).
//
// G-7 PKCE parity: the probe additionally asserts the SDK's verifier
// matches RFC 7636 / golang.org/x/oauth2 properties:
//   - the decoded verifier has >= 32 bytes of entropy;
//   - the verifier is base64.RawURLEncoding (URL-safe alphabet, no padding);
//   - challenge == base64url(SHA256(verifier)) — cross-checked against
//     x/oauth2.S256ChallengeFromVerifier — with method S256.
func Probe004PKCEVerifierRoundTrip(ctx context.Context) (Result, error) { // PROBE-004 (REQ-061)
	r := Result{Probe: "PROBE-004"}
	cap, err := runPKCEFlow(ctx, []string{"patient/COMPOSITION.read"}, "")
	if err != nil {
		return r, fmt.Errorf("PROBE-004: %w", err)
	}

	// Authorization-request assertions.
	if got := cap.authQuery.Get("code_challenge"); got != cap.challenge {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("authorization code_challenge = %q; want %q", got, cap.challenge)
		return r, nil
	}
	if got := cap.authQuery.Get("code_challenge_method"); got != "S256" {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("code_challenge_method = %q; want S256", got)
		return r, nil
	}

	// Token-exchange assertion: the verifier travels on the token request.
	if got := cap.tokenForm.Get("code_verifier"); got != cap.verifier {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("token code_verifier = %q; want %q", got, cap.verifier)
		return r, nil
	}
	if got := cap.tokenForm.Get("grant_type"); got != "authorization_code" {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("token grant_type = %q; want authorization_code", got)
		return r, nil
	}

	// G-7 parity: verifier entropy + alphabet.
	raw, decErr := base64.RawURLEncoding.DecodeString(cap.verifier)
	if decErr != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("verifier is not base64.RawURLEncoding (no padding, URL-safe): %v", decErr)
		return r, nil
	}
	if len(raw) < 32 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("verifier carries %d bytes of entropy; RFC 7636 RECOMMENDS >= 32", len(raw))
		return r, nil
	}
	if strings.ContainsAny(cap.verifier, "+/=") {
		r.Status = "fail"
		r.Detail = "verifier contains standard-base64 / padding characters; want URL-safe unpadded"
		return r, nil
	}

	// G-7 parity: challenge == base64url(SHA256(verifier)), cross-checked
	// against x/oauth2 and recomputed directly.
	wantChallenge := xoauth2.S256ChallengeFromVerifier(cap.verifier)
	sum := sha256.Sum256([]byte(cap.verifier))
	direct := base64.RawURLEncoding.EncodeToString(sum[:])
	if cap.challenge != wantChallenge || cap.challenge != direct {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("challenge %q != base64url(SHA256(verifier)) (x/oauth2=%q, direct=%q)", cap.challenge, wantChallenge, direct)
		return r, nil
	}

	r.Status = "pass"
	r.Detail = fmt.Sprintf("S256 PKCE round-trip: challenge on authz, verifier on token; G-7 parity holds (%d-byte verifier, RFC 7636 / x/oauth2)", len(raw))
	return r, nil
}
