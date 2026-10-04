package smart_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart"
)

const (
	claimsIssuer   = "https://issuer.example"
	claimsClientID = "client-id"
)

// countingJWKS serves body as the key set and counts how often it is fetched,
// so a test can tell whether an ID token was verified.
func countingJWKS(t *testing.T, body []byte) (*authsmart.JWKS, *atomic.Int32) {
	t.Helper()
	var fetches atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetches.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	jwks, err := authsmart.NewJWKS(srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return jwks, &fetches
}

// claimsTrustAnchors are the options that let LaunchContextFromTokenResponse
// verify an ID token signed by the testRSAKey key.
func claimsTrustAnchors(jwks *authsmart.JWKS, now time.Time) []smart.ValidateOption {
	return []smart.ValidateOption{
		smart.WithJWKS(jwks),
		smart.WithIssuer(claimsIssuer),
		smart.WithClientID(claimsClientID),
		smart.WithValidationTime(now),
	}
}

// TestLaunchContextUsesVerifiedClaims pins that claims auth/smart already
// verified are used as they are: the key set is not fetched again, so the
// token is not verified a second time. A response with an ID token but no
// verified claims is still verified, and fetches the key set once.
func TestLaunchContextUsesVerifiedClaims(t *testing.T) { // REQ-064
	priv, jwksBody := testRSAKey(t)
	now := time.Unix(1_700_000_000, 0)
	idTok := signJWT(t, priv, "test-kid", map[string]any{
		"iss":      claimsIssuer,
		"sub":      "user-1",
		"aud":      claimsClientID,
		"exp":      now.Add(time.Hour).Unix(),
		"iat":      now.Unix(),
		"fhirUser": "Practitioner/token",
	})

	t.Run("verified claims are not verified again", func(t *testing.T) {
		jwks, fetches := countingJWKS(t, jwksBody)
		verified := &smart.IDTokenClaims{
			Subject:   "user-1",
			Audience:  []string{claimsClientID},
			Issuer:    claimsIssuer,
			ExpiresAt: now.Add(time.Hour),
			FHIRUser:  "Practitioner/verified",
		}
		tr := authsmart.TokenResponse{AccessToken: "at", IDToken: idTok, IDTokenClaims: verified}
		lc, err := smart.LaunchContextFromTokenResponse(t.Context(), tr, claimsTrustAnchors(jwks, now)...)
		if err != nil {
			t.Fatalf("LaunchContextFromTokenResponse(verified claims) error = %v, want nil", err)
		}
		if got := fetches.Load(); got != 0 {
			t.Errorf("LaunchContextFromTokenResponse(verified claims) fetched the key set %d times, want 0", got)
		}
		if lc.IDToken != verified {
			t.Errorf("LaunchContextFromTokenResponse(verified claims) IDToken = %#v, want the verified claims %#v", lc.IDToken, verified)
		}
		if lc.User != "Practitioner/verified" {
			t.Errorf("LaunchContextFromTokenResponse(verified claims) User = %q, want %q", lc.User, "Practitioner/verified")
		}
	})

	t.Run("an id_token without verified claims is verified", func(t *testing.T) {
		jwks, fetches := countingJWKS(t, jwksBody)
		tr := authsmart.TokenResponse{AccessToken: "at", IDToken: idTok}
		lc, err := smart.LaunchContextFromTokenResponse(t.Context(), tr, claimsTrustAnchors(jwks, now)...)
		if err != nil {
			t.Fatalf("LaunchContextFromTokenResponse(id_token only) error = %v, want nil", err)
		}
		if got := fetches.Load(); got != 1 {
			t.Errorf("LaunchContextFromTokenResponse(id_token only) fetched the key set %d times, want 1", got)
		}
		if lc.IDToken == nil || lc.IDToken.Subject != "user-1" || lc.User != "Practitioner/token" {
			t.Errorf("LaunchContextFromTokenResponse(id_token only) IDToken = %#v, User = %q, want sub user-1, User Practitioner/token", lc.IDToken, lc.User)
		}
	})

	t.Run("an id_token without verified claims that fails is refused", func(t *testing.T) {
		jwks, _ := countingJWKS(t, jwksBody)
		tr := authsmart.TokenResponse{AccessToken: "at", IDToken: idTok}
		opts := append(claimsTrustAnchors(jwks, now), smart.WithIssuer("https://other.example"))
		_, err := smart.LaunchContextFromTokenResponse(t.Context(), tr, opts...)
		if !errors.Is(err, auth.ErrJWKSValidationFailed) {
			t.Errorf("LaunchContextFromTokenResponse(id_token from another issuer) error = %v, want ErrJWKSValidationFailed", err)
		}
	})
}

// TestLaunchContextTrustsSuppliedClaims pins the documented trust: claims on
// TokenResponse.IDTokenClaims are taken as verified, so claims that no
// verification produced are used without a check and without trust anchors.
// That is why they must come from auth/smart.
func TestLaunchContextTrustsSuppliedClaims(t *testing.T) { // REQ-064
	forged := &smart.IDTokenClaims{Subject: "forged-user", Issuer: "https://forged.example"}
	tr := authsmart.TokenResponse{AccessToken: "at", IDToken: "not.a.jwt", IDTokenClaims: forged}
	lc, err := smart.LaunchContextFromTokenResponse(t.Context(), tr)
	if err != nil {
		t.Fatalf("LaunchContextFromTokenResponse(supplied claims, no trust anchors) error = %v, want nil", err)
	}
	if lc.IDToken != forged || lc.User != "forged-user" || lc.Issuer != "https://forged.example" {
		t.Errorf("LaunchContextFromTokenResponse(supplied claims) IDToken = %#v, User = %q, Issuer = %q, want the supplied claims, forged-user, https://forged.example",
			lc.IDToken, lc.User, lc.Issuer)
	}
}

// TestLaunchContextUserFromVerifiedIDTokenOnly pins that User comes from a
// verified ID token only, its fhirUser claim else its sub, whether
// auth/smart or LaunchContextFromTokenResponse verified it. A fhirUser
// member in the token-endpoint body never sets it, and stays readable on the
// response and on Raw.
func TestLaunchContextUserFromVerifiedIDTokenOnly(t *testing.T) { // REQ-064
	priv, jwksBody := testRSAKey(t)
	now := time.Unix(1_700_000_000, 0)
	tokenClaims := func(fhirUser string) map[string]any {
		c := map[string]any{
			"iss": claimsIssuer,
			"sub": "user-sub",
			"aud": claimsClientID,
			"exp": now.Add(time.Hour).Unix(),
			"iat": now.Unix(),
		}
		if fhirUser != "" {
			c["fhirUser"] = fhirUser
		}
		return c
	}
	verified := func(fhirUser string) *smart.IDTokenClaims {
		return &smart.IDTokenClaims{
			Subject:   "user-sub",
			Audience:  []string{claimsClientID},
			Issuer:    claimsIssuer,
			ExpiresAt: now.Add(time.Hour),
			FHIRUser:  fhirUser,
		}
	}

	tests := []struct {
		name         string
		bodyFHIRUser string
		idToken      string               // verified by LaunchContextFromTokenResponse
		claims       *smart.IDTokenClaims // verified by auth/smart
		wantUser     string
	}{
		{name: "body fhirUser, no id_token", bodyFHIRUser: "Practitioner/A", wantUser: ""},
		{name: "body fhirUser, verified claims with fhirUser", bodyFHIRUser: "Practitioner/A", claims: verified("Practitioner/B"), wantUser: "Practitioner/B"},
		{name: "body fhirUser, verified claims without fhirUser", bodyFHIRUser: "Practitioner/A", claims: verified(""), wantUser: "user-sub"},
		{name: "no body fhirUser, verified claims without fhirUser", claims: verified(""), wantUser: "user-sub"},
		{name: "body fhirUser, id_token with fhirUser", bodyFHIRUser: "Practitioner/A", idToken: signJWT(t, priv, "test-kid", tokenClaims("Practitioner/B")), wantUser: "Practitioner/B"},
		{name: "body fhirUser, id_token without fhirUser", bodyFHIRUser: "Practitioner/A", idToken: signJWT(t, priv, "test-kid", tokenClaims("")), wantUser: "user-sub"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			jwks, _ := countingJWKS(t, jwksBody)
			tr := authsmart.TokenResponse{
				AccessToken:   "at",
				IDToken:       tc.idToken,
				IDTokenClaims: tc.claims,
				FHIRUser:      tc.bodyFHIRUser,
			}
			if tc.bodyFHIRUser != "" {
				tr.Raw = map[string]any{"fhirUser": tc.bodyFHIRUser}
			}
			lc, err := smart.LaunchContextFromTokenResponse(t.Context(), tr, claimsTrustAnchors(jwks, now)...)
			if err != nil {
				t.Fatalf("LaunchContextFromTokenResponse(%s) error = %v, want nil", tc.name, err)
			}
			if lc.User != tc.wantUser {
				t.Errorf("LaunchContextFromTokenResponse(%s) User = %q, want %q", tc.name, lc.User, tc.wantUser)
			}
			if tc.bodyFHIRUser != "" && lc.Raw["fhirUser"] != tc.bodyFHIRUser {
				t.Errorf("LaunchContextFromTokenResponse(%s) Raw[fhirUser] = %v, want %q", tc.name, lc.Raw["fhirUser"], tc.bodyFHIRUser)
			}
		})
	}
}

// TestLaunchContextVerifiesNonceAndAlgorithms pins REQ-062 and REQ-064: an
// ID token without verified claims is verified with the options the caller
// passed, so a nonce other than the expected one, or an algorithm the
// allowlist leaves out, refuses it with auth.ErrJWKSValidationFailed, while
// the same token passes with the matching nonce and algorithm.
func TestLaunchContextVerifiesNonceAndAlgorithms(t *testing.T) { // REQ-062 REQ-064
	priv, jwksBody := testRSAKey(t)
	now := time.Unix(1_700_000_000, 0)
	idTok := signJWT(t, priv, "test-kid", map[string]any{
		"iss":   claimsIssuer,
		"sub":   "user-1",
		"aud":   claimsClientID,
		"exp":   now.Add(time.Hour).Unix(),
		"iat":   now.Unix(),
		"nonce": "nonce-launch",
	})
	tests := []struct {
		name    string
		opts    []smart.ValidateOption
		wantErr bool
	}{
		{name: "expected nonce, allowed algorithm", opts: []smart.ValidateOption{smart.WithExpectedNonce("nonce-launch"), smart.WithIDTokenSigningAlgs([]string{"RS256"})}},
		{name: "another nonce expected", opts: []smart.ValidateOption{smart.WithExpectedNonce("nonce-other")}, wantErr: true},
		{name: "allowlist without the token's algorithm", opts: []smart.ValidateOption{smart.WithIDTokenSigningAlgs([]string{"ES256"})}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			jwks, _ := countingJWKS(t, jwksBody)
			tr := authsmart.TokenResponse{AccessToken: "at", IDToken: idTok}
			lc, err := smart.LaunchContextFromTokenResponse(t.Context(), tr, append(claimsTrustAnchors(jwks, now), tc.opts...)...)
			if !tc.wantErr {
				if err != nil || lc.IDToken == nil || lc.IDToken.Subject != "user-1" {
					t.Fatalf("LaunchContextFromTokenResponse(%s) = %+v, %v; want the verified token for user-1", tc.name, lc, err)
				}
				return
			}
			if !errors.Is(err, auth.ErrJWKSValidationFailed) {
				t.Errorf("LaunchContextFromTokenResponse(%s) error = %v, want auth.ErrJWKSValidationFailed", tc.name, err)
			}
			if lc != nil {
				t.Errorf("LaunchContextFromTokenResponse(%s) = %+v on failure, want nil", tc.name, lc)
			}
		})
	}
}
