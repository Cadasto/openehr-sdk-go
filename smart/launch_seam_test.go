package smart_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// seamServer is a stub authorization server: /jwks serves the testRSAKey
// set and counts its fetches, and /token answers with the body last set.
type seamServer struct {
	srv         *httptest.Server
	jwksFetches atomic.Int32

	mu   sync.Mutex
	body []byte
}

func newSeamServer(t *testing.T, jwksBody []byte) *seamServer {
	t.Helper()
	s := &seamServer{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/jwks":
			s.jwksFetches.Add(1)
			_, _ = w.Write(jwksBody)
		case "/token":
			s.mu.Lock()
			body := s.body
			s.mu.Unlock()
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *seamServer) setTokenBody(t *testing.T, body map[string]any) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.body = b
	s.mu.Unlock()
}

// TestLaunchContextFromCompletedAuthorization joins auth/smart and smart: a
// real Source completes an authorization whose token response carries a
// signed ID token, and LaunchContextFromTokenResponse, given no options at
// all, builds the context from the claims the Source verified. The key set
// is fetched once, by the Source, and never again. A body fhirUser never
// names the user.
func TestLaunchContextFromCompletedAuthorization(t *testing.T) { // REQ-064
	priv, jwksBody := testRSAKey(t)

	tests := []struct {
		name     string
		sub      string
		fhirUser string
		wantUser string
	}{
		{name: "verified fhirUser", sub: "user-seam", fhirUser: "Practitioner/verified", wantUser: "Practitioner/verified"},
		{name: "verified sub only", sub: "user-seam", wantUser: "user-seam"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stub := newSeamServer(t, jwksBody)
			src, err := authsmart.New(claimsClientID, discovery.AuthEndpoints{
				AuthorizationEndpoint: discovery.MustParseURL(claimsIssuer + "/authorize"),
				TokenEndpoint:         discovery.MustParseURL(stub.srv.URL + "/token"),
				JWKSURI:               discovery.MustParseURL(stub.srv.URL + "/jwks"),
			},
				authsmart.WithAudience("https://platform.example"),
				authsmart.WithHTTPClient(stub.srv.Client()),
				authsmart.WithRedirectURI("https://app.example/callback"),
				authsmart.WithIssuer(claimsIssuer),
				authsmart.WithScopes("openid", "launch/patient"),
			)
			if err != nil {
				t.Fatalf("authsmart.New: %v", err)
			}
			req, err := src.BeginAuthorization("")
			if err != nil {
				t.Fatalf("BeginAuthorization: %v", err)
			}

			now := time.Now()
			claims := map[string]any{
				"iss":   claimsIssuer,
				"aud":   claimsClientID,
				"exp":   now.Add(time.Hour).Unix(),
				"iat":   now.Unix(),
				"nonce": req.Nonce,
			}
			claims["sub"] = tc.sub
			if tc.fhirUser != "" {
				claims["fhirUser"] = tc.fhirUser
			}
			stub.setTokenBody(t, map[string]any{
				"access_token": "at",
				"token_type":   "Bearer",
				"expires_in":   3600,
				"id_token":     signJWT(t, priv, "test-kid", claims),
				"patient":      "patient-1",
				"fhirUser":     "Practitioner/body",
			})

			callback := url.Values{"code": {"code-1"}, "state": {req.State}}
			_, tr, err := src.CompleteAuthorization(t.Context(), callback, req)
			if err != nil {
				t.Fatalf("CompleteAuthorization: %v", err)
			}
			if tr.IDTokenClaims == nil {
				t.Fatal("CompleteAuthorization returned no verified ID-token claims")
			}
			fetched := stub.jwksFetches.Load()
			if fetched != 1 {
				t.Fatalf("CompleteAuthorization fetched the key set %d times, want 1 (the Source verifies the ID token)", fetched)
			}

			lc, err := smart.LaunchContextFromTokenResponse(t.Context(), tr)
			if err != nil {
				t.Fatalf("LaunchContextFromTokenResponse(completed authorization, no options) error = %v, want nil", err)
			}
			if got := stub.jwksFetches.Load(); got != fetched {
				t.Errorf("LaunchContextFromTokenResponse fetched the key set again: %d fetches after it, %d after the exchange", got, fetched)
			}
			if lc.IDToken != tr.IDTokenClaims {
				t.Errorf("LaunchContextFromTokenResponse IDToken = %#v, want the Source's verified claims %#v", lc.IDToken, tr.IDTokenClaims)
			}
			if lc.User != tc.wantUser {
				t.Errorf("LaunchContextFromTokenResponse(%s) User = %q, want %q (body fhirUser %q)", tc.name, lc.User, tc.wantUser, tr.FHIRUser)
			}
			if lc.Patient != "patient-1" || lc.Issuer != claimsIssuer {
				t.Errorf("LaunchContextFromTokenResponse Patient = %q, Issuer = %q, want patient-1, %s", lc.Patient, lc.Issuer, claimsIssuer)
			}
		})
	}
}

// TestLaunchContextVerifiedClaimsNamingNobody pins REQ-064: verified claims
// that name no user, with neither fhirUser nor sub, leave User empty, and a
// fhirUser member in the token-endpoint body does not fill it in. auth/smart
// refuses an ID token without sub, so the claims are supplied directly, as a
// Source would hand them over.
func TestLaunchContextVerifiedClaimsNamingNobody(t *testing.T) { // REQ-064
	claims := &smart.IDTokenClaims{Issuer: claimsIssuer, Audience: []string{claimsClientID}}
	tr := authsmart.TokenResponse{
		AccessToken:   "at",
		FHIRUser:      "Practitioner/body",
		Raw:           map[string]any{"fhirUser": "Practitioner/body"},
		IDTokenClaims: claims,
	}
	lc, err := smart.LaunchContextFromTokenResponse(t.Context(), tr)
	if err != nil {
		t.Fatalf("LaunchContextFromTokenResponse(claims naming nobody) error = %v, want nil", err)
	}
	if lc.IDToken != claims {
		t.Errorf("LaunchContextFromTokenResponse IDToken = %#v, want the supplied claims %#v", lc.IDToken, claims)
	}
	if lc.User != "" {
		t.Errorf("LaunchContextFromTokenResponse User = %q, want empty: the verified claims name nobody (body fhirUser %q)", lc.User, tr.FHIRUser)
	}
}
