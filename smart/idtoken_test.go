package smart_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart"
)

// The algorithm and claim rules of ID-token verification are tested in
// auth/smart, where the verification lives. These tests pin only that the
// smart names stay the same function and type. REQ-062 REQ-064

// smart.ValidateIDToken keeps the function type it had before verification
// moved to auth/smart, so a variable of that type still accepts it. REQ-062
var _ func(context.Context, string, *authsmart.JWKS, string, string, string, time.Time, []string) (*smart.IDTokenClaims, error) = smart.ValidateIDToken

// idTokenJWKS serves the testRSAKey set (kid "test-kid") and returns its JWKS.
func idTokenJWKS(t *testing.T, body []byte) *authsmart.JWKS {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	jwks, err := authsmart.NewJWKS(srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return jwks
}

// TestIDTokenClaimsIsTheAuthSmartType checks that smart.IDTokenClaims is the
// auth/smart type itself and not a copy of it, so claims flow between the two
// packages without a conversion. REQ-062 REQ-064
func TestIDTokenClaimsIsTheAuthSmartType(t *testing.T) {
	got, want := reflect.TypeFor[smart.IDTokenClaims](), reflect.TypeFor[authsmart.IDTokenClaims]()
	if got != want {
		t.Fatalf("smart.IDTokenClaims is %v, want the type %v itself", got, want)
	}
}

// TestValidateIDTokenWrapperMatchesAuthSmart checks that smart.ValidateIDToken
// returns what auth/smart.ValidateIDToken returns for the same arguments: the
// same claims for a valid token, and the same error class for a rejected token
// and for a missing trust anchor. REQ-062 REQ-064
func TestValidateIDTokenWrapperMatchesAuthSmart(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv, body := testRSAKey(t)
	jwks := idTokenJWKS(t, body)
	tok := signJWT(t, priv, "test-kid", map[string]any{
		"iss":      "https://issuer.example",
		"sub":      "user-1",
		"aud":      "client-id",
		"exp":      now.Add(time.Hour).Unix(),
		"iat":      now.Unix(),
		"nonce":    "nonce-xyz",
		"fhirUser": "Practitioner/99",
		"custom":   "value",
	})

	cases := []struct {
		name    string
		issuer  string
		nonce   string
		wantErr error // nil means the token verifies
	}{
		{name: "valid token", issuer: "https://issuer.example", nonce: "nonce-xyz"},
		{name: "nonce mismatch", issuer: "https://issuer.example", nonce: "other-nonce", wantErr: auth.ErrJWKSValidationFailed},
		{name: "no issuer configured", nonce: "nonce-xyz", wantErr: auth.ErrInvalidConfig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			viaSmart, errSmart := smart.ValidateIDToken(t.Context(), tok, jwks, tc.issuer, "client-id", tc.nonce, now, []string{"RS256"})
			viaAuth, errAuth := authsmart.ValidateIDToken(t.Context(), tok, jwks, tc.issuer, "client-id", tc.nonce, now, []string{"RS256"})
			if tc.wantErr == nil {
				if errSmart != nil || errAuth != nil {
					t.Fatalf("ValidateIDToken(%s): smart error = %v, auth/smart error = %v, want nil", tc.name, errSmart, errAuth)
				}
				if !reflect.DeepEqual(viaSmart, viaAuth) {
					t.Fatalf("ValidateIDToken(%s): smart claims = %#v, auth/smart claims = %#v, want the same", tc.name, viaSmart, viaAuth)
				}
				if viaSmart.Subject != "user-1" || viaSmart.FHIRUser != "Practitioner/99" || viaSmart.Extra["custom"] != "value" {
					t.Fatalf("ValidateIDToken(%s) claims = %#v, want sub user-1, fhirUser Practitioner/99, custom value", tc.name, viaSmart)
				}
				return
			}
			if !errors.Is(errSmart, tc.wantErr) || !errors.Is(errAuth, tc.wantErr) {
				t.Fatalf("ValidateIDToken(%s): smart error = %v, auth/smart error = %v, want both to match %v", tc.name, errSmart, errAuth, tc.wantErr)
			}
		})
	}
}

// TestTrustedAudiencesReachVerification checks that smart.ValidateIDToken,
// which takes no trusted audiences, refuses a token with an extra audience, and
// that LaunchContextFromTokenResponse accepts it only with
// WithTrustedAudiences naming that audience. REQ-062 REQ-064
func TestTrustedAudiencesReachVerification(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv, body := testRSAKey(t)
	jwks := idTokenJWKS(t, body)
	tok := signJWT(t, priv, "test-kid", map[string]any{
		"iss": "https://issuer.example",
		"sub": "user-1",
		"aud": []string{"client-id", "api"},
		"exp": now.Add(time.Hour).Unix(),
	})

	t.Run("ValidateIDToken", func(t *testing.T) {
		if _, err := smart.ValidateIDToken(t.Context(), tok, jwks, "https://issuer.example", "client-id", "", now, nil); !errors.Is(err, auth.ErrJWKSValidationFailed) {
			t.Fatalf("ValidateIDToken(aud [client-id api], no trusted audience) error = %v, want ErrJWKSValidationFailed", err)
		}
	})
	t.Run("LaunchContextFromTokenResponse", func(t *testing.T) {
		tr := authsmart.TokenResponse{AccessToken: "at", IDToken: tok}
		base := []smart.ValidateOption{
			smart.WithJWKS(jwks),
			smart.WithIssuer("https://issuer.example"),
			smart.WithClientID("client-id"),
			smart.WithValidationTime(now),
		}
		if _, err := smart.LaunchContextFromTokenResponse(t.Context(), tr, base...); !errors.Is(err, auth.ErrJWKSValidationFailed) {
			t.Fatalf("LaunchContextFromTokenResponse(aud [client-id api], no trusted audience) error = %v, want ErrJWKSValidationFailed", err)
		}
		lc, err := smart.LaunchContextFromTokenResponse(t.Context(), tr, append(base, smart.WithTrustedAudiences("api"))...)
		if err != nil {
			t.Fatalf("LaunchContextFromTokenResponse(aud [client-id api], trusted [api]) error = %v, want nil", err)
		}
		if lc.IDToken == nil || lc.IDToken.Subject != "user-1" {
			t.Fatalf("LaunchContextFromTokenResponse(trusted [api]) IDToken = %#v, want sub user-1", lc.IDToken)
		}
	})
}

// TestWithTrustedAudiencesKeepsItsOwnCopy checks that changing the caller's
// slice after building smart.WithTrustedAudiences does not change the trusted
// set LaunchContextFromTokenResponse passes to the verification. REQ-062
func TestWithTrustedAudiencesKeepsItsOwnCopy(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv, body := testRSAKey(t)
	jwks := idTokenJWKS(t, body)
	tok := signJWT(t, priv, "test-kid", map[string]any{
		"iss": "https://issuer.example",
		"sub": "user-1",
		"aud": []string{"client-id", "api"},
		"exp": now.Add(time.Hour).Unix(),
	})

	trusted := []string{"other"}
	opt := smart.WithTrustedAudiences(trusted...)
	trusted[0] = "api"
	_, err := smart.LaunchContextFromTokenResponse(t.Context(), authsmart.TokenResponse{AccessToken: "at", IDToken: tok},
		smart.WithJWKS(jwks),
		smart.WithIssuer("https://issuer.example"),
		smart.WithClientID("client-id"),
		smart.WithValidationTime(now),
		opt,
	)
	// REQ-062: the trusted set is the one given when the option was built.
	if !errors.Is(err, auth.ErrJWKSValidationFailed) {
		t.Fatalf("LaunchContextFromTokenResponse(aud [client-id api], trusted [other] then edited to [api]) error = %v, want ErrJWKSValidationFailed", err)
	}
}
