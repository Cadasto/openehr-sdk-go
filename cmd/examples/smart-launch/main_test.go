package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// The verifier and challenge of RFC 7636 Appendix B, so the stub's S256 check
// is held to the RFC rather than to its own hashing.
const (
	rfcVerifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	rfcChallenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	// otherVerifier and otherChallenge are a second pair, for a second launch.
	otherVerifier  = "another-verifier-0123456789-0123456789-01234"
	otherChallenge = "FquQ9bii1jAW7uLcxzAdAgjkH6pFSqn4bBqPvQKcjAg"
	// emptyVerifierChallenge is the S256 challenge of the empty string.
	emptyVerifierChallenge = "47DEQpj8HBSa-_TImW-5JCeuQeRkm5NMpJWZG3hSuFU"
)

// TestRun exercises the full standalone PKCE launch end-to-end against the
// in-process stub server. Passing here proves the flow works
// offline without any real SMART authorization server, from the authorize
// URL to CompleteAuthorization on the redirect's query. The stub refuses an
// authorization request without aud or an S256 challenge, and a token
// request whose verifier does not match that challenge, so passing also
// proves the source built from the catalog sends all three.
func TestRun(t *testing.T) {
	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestStubAuthorizeRequiresS256 checks that the stub refuses an authorization
// request without a PKCE challenge, or with a challenge method other than
// S256. RFC 7636 §4.3 makes plain the method when none is named.
func TestStubAuthorizeRequiresS256(t *testing.T) {
	tests := []struct {
		name      string
		challenge string
		method    string
	}{
		{name: "plain method", challenge: rfcVerifier, method: "plain"},
		{name: "no method", challenge: rfcVerifier},
		{name: "no challenge", method: "S256"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(newStubServer().mux)
			defer server.Close()

			status, code := authorize(t, server, "state-0123456789", tc.challenge, tc.method)
			if status != http.StatusBadRequest {
				t.Errorf("GET /authorize with code_challenge=%q code_challenge_method=%q: status %d, code %q; want %d",
					tc.challenge, tc.method, status, code, http.StatusBadRequest)
			}
		})
	}
}

// TestStubAuthorizeAcceptsShortState checks that the stub issues a code for a
// state of three characters; the SDK's states are longer, but OAuth 2.0 sets
// no minimum.
func TestStubAuthorizeAcceptsShortState(t *testing.T) {
	server := httptest.NewServer(newStubServer().mux)
	defer server.Close()

	status, code := authorize(t, server, "abc", rfcChallenge, "S256")
	if status != http.StatusOK || code == "" {
		t.Errorf("GET /authorize with state %q: status %d, code %q; want %d and a code", "abc", status, code, http.StatusOK)
	}
}

// TestStubCodesAreDistinct checks that two launches whose states share their
// first characters get codes of their own, each exchanged with the verifier
// of its own challenge.
func TestStubCodesAreDistinct(t *testing.T) {
	server := httptest.NewServer(newStubServer().mux)
	defer server.Close()

	_, first := authorize(t, server, "shared-prefix-1", rfcChallenge, "S256")
	_, second := authorize(t, server, "shared-prefix-2", otherChallenge, "S256")
	if first == second {
		t.Fatalf("GET /authorize for two states: both got code %q, want two codes", first)
	}
	for code, verifier := range map[string]string{first: rfcVerifier, second: otherVerifier} {
		if status, body := postToken(t, server, tokenForm(code, verifier)); status != http.StatusOK {
			t.Errorf("POST /token for code %q with its own verifier: status %d, body %s; want %d",
				code, status, body, http.StatusOK)
		}
	}
}

// TestStubTokenChecksPKCE checks that the stub's token endpoint exchanges a
// code only for the verifier whose S256 hash is the challenge /authorize
// stored, and refuses any other with an OAuth2 invalid_grant error that does
// not repeat the verifier.
func TestStubTokenChecksPKCE(t *testing.T) {
	tests := []struct {
		name       string
		challenge  string // sent to /authorize with the S256 method
		verifier   string // sent to /token; empty leaves code_verifier out
		wantStatus int
		wantError  string
	}{
		{name: "matching verifier", challenge: rfcChallenge, verifier: rfcVerifier, wantStatus: http.StatusOK},
		{
			name: "wrong verifier", challenge: rfcChallenge, verifier: otherVerifier,
			wantStatus: http.StatusBadRequest, wantError: "invalid_grant",
		},
		// The challenge of the empty string, so only the check for a missing
		// verifier can refuse the exchange.
		{
			name: "missing verifier", challenge: emptyVerifierChallenge,
			wantStatus: http.StatusBadRequest, wantError: "invalid_grant",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(newStubServer().mux)
			defer server.Close()

			status, code := authorize(t, server, "state-0123456789", tc.challenge, "S256")
			if status != http.StatusOK {
				t.Fatalf("GET /authorize with an S256 challenge: status %d, want %d", status, http.StatusOK)
			}
			status, body := postToken(t, server, tokenForm(code, tc.verifier))

			var answer struct {
				Error       string `json:"error"`
				AccessToken string `json:"access_token"`
			}
			if err := json.Unmarshal([]byte(body), &answer); err != nil {
				t.Fatalf("POST /token with code_verifier=%q: status %d, body is not JSON: %v", tc.verifier, status, err)
			}
			if status != tc.wantStatus || answer.Error != tc.wantError {
				t.Errorf("POST /token with code_verifier=%q: status %d, error %q; want status %d, error %q",
					tc.verifier, status, answer.Error, tc.wantStatus, tc.wantError)
			}
			if tc.wantError == "" && answer.AccessToken == "" {
				t.Errorf("POST /token with code_verifier=%q: no access_token in the answer", tc.verifier)
			}
			if tc.wantError != "" && tc.verifier != "" && strings.Contains(body, tc.verifier) {
				t.Errorf("POST /token refused code_verifier=%q with a body that repeats it", tc.verifier)
			}
		})
	}
}

// TestStubCodeIsOneUse checks that the stub's token endpoint accepts a code
// once, whatever the outcome of that first exchange: a second exchange of the
// same code with the right verifier is refused with invalid_grant, after a
// refused first attempt as after a successful one.
func TestStubCodeIsOneUse(t *testing.T) {
	tests := []struct {
		name          string
		firstVerifier string // sent with the first exchange
		firstStatus   int
	}{
		{name: "after a successful exchange", firstVerifier: rfcVerifier, firstStatus: http.StatusOK},
		{name: "after a refused exchange", firstVerifier: otherVerifier, firstStatus: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(newStubServer().mux)
			defer server.Close()

			status, code := authorize(t, server, "state-0123456789", rfcChallenge, "S256")
			if status != http.StatusOK {
				t.Fatalf("GET /authorize with an S256 challenge: status %d, want %d", status, http.StatusOK)
			}
			if status, body := postToken(t, server, tokenForm(code, tc.firstVerifier)); status != tc.firstStatus {
				t.Fatalf("first POST /token for code %q with code_verifier=%q: status %d, body %s; want %d",
					code, tc.firstVerifier, status, body, tc.firstStatus)
			}

			status, body := postToken(t, server, tokenForm(code, rfcVerifier))
			var answer struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal([]byte(body), &answer); err != nil {
				t.Fatalf("second POST /token for code %q: status %d, body is not JSON: %v", code, status, err)
			}
			if status != http.StatusBadRequest || answer.Error != "invalid_grant" {
				t.Errorf("second POST /token for code %q with the right verifier: status %d, error %q; want status %d, error %q",
					code, status, answer.Error, http.StatusBadRequest, "invalid_grant")
			}
		})
	}
}

// TestCompleteAuthorizationRefusesWrongVerifier checks what an app sees when
// the stub refuses its verifier: CompleteAuthorization fails with an error
// that unwraps to an *auth.OAuth2Error whose Code is invalid_grant.
func TestCompleteAuthorizationRefusesWrongVerifier(t *testing.T) {
	server := httptest.NewServer(newStubServer().mux)
	defer server.Close()

	source, err := authsmart.New(clientID,
		discovery.AuthEndpoints{
			AuthorizationEndpoint: discovery.MustParseURL(server.URL + "/authorize"),
			TokenEndpoint:         discovery.MustParseURL(server.URL + "/token"),
		},
		authsmart.WithHTTPClient(server.Client()),
		authsmart.WithRedirectURI(redirectURI),
		authsmart.WithAudience(server.URL),
	)
	if err != nil {
		t.Fatalf("smart.New: %v", err)
	}
	req, err := source.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	authorizeURL, err := source.AuthorizeURL(req, "")
	if err != nil {
		t.Fatalf("AuthorizeURL: %v", err)
	}
	callback, err := openAuthorizeURL(t.Context(), server.Client(), authorizeURL)
	if err != nil {
		t.Fatalf("open the authorize URL: %v", err)
	}

	// The verifier of another launch, as an app that stored the wrong
	// request would send.
	req.PKCE.Verifier = rfcVerifier
	_, _, err = source.CompleteAuthorization(t.Context(), callback, req)
	if !errors.Is(err, auth.ErrTokenExchangeFailed) {
		t.Errorf("CompleteAuthorization with the wrong verifier: error %v, want one matching auth.ErrTokenExchangeFailed", err)
	}
	if oauthErr, ok := errors.AsType[*auth.OAuth2Error](err); !ok || oauthErr == nil || oauthErr.Code != "invalid_grant" {
		t.Errorf("CompleteAuthorization with the wrong verifier: error %v, want an *auth.OAuth2Error with Code invalid_grant", err)
	}
}

// authorize sends the stub an authorization request carrying aud, state and
// the given PKCE parameters, leaving out the empty ones, and returns the
// status and the code it issued, if any.
func authorize(t *testing.T, server *httptest.Server, state, challenge, method string) (status int, code string) {
	t.Helper()

	query := url.Values{"aud": {server.URL}, "state": {state}}
	if challenge != "" {
		query.Set("code_challenge", challenge)
	}
	if method != "" {
		query.Set("code_challenge_method", method)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/authorize?"+query.Encode(), nil)
	if err != nil {
		t.Fatalf("build the authorize request: %v", err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /authorize: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, ""
	}
	var grant struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&grant); err != nil {
		t.Fatalf("decode the authorize answer: %v", err)
	}
	return resp.StatusCode, grant.Code
}

// tokenForm is an authorization_code grant for code, carrying verifier as the
// code_verifier unless it is empty.
func tokenForm(code, verifier string) url.Values {
	form := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
		"client_id":    {clientID},
	}
	if verifier != "" {
		form.Set("code_verifier", verifier)
	}
	return form
}

// postToken posts form to the stub's token endpoint and returns the status
// and the body of the answer.
func postToken(t *testing.T, server *httptest.Server, form url.Values) (status int, body string) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("build the token request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("POST /token: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read the token answer: %v", err)
	}
	return resp.StatusCode, string(raw)
}
