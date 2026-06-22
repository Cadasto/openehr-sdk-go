package smart_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart"
)

func TestLaunchContextFromTokenResponse(t *testing.T) {
	tr := authsmart.TokenResponse{
		Patient:   "patient-1",
		Encounter: "enc-1",
		Scope:     "openid patient/*.read",
		FHIRUser:  "Practitioner/abc",
		Raw:       map[string]any{"patient": "patient-1"},
	}
	lc, err := smart.LaunchContextFromTokenResponse(context.Background(), tr)
	if err != nil {
		t.Fatal(err)
	}
	if lc.Patient != "patient-1" || lc.User != "Practitioner/abc" || len(lc.Scopes) != 2 {
		t.Fatalf("lc = %#v", lc)
	}
	ctx := smart.WithLaunchContext(context.Background(), lc)
	got, ok := smart.LaunchContextFromContext(ctx)
	if !ok || got.Patient != "patient-1" {
		t.Fatalf("context = %#v ok=%v", got, ok)
	}
}

func TestLaunchContextValidatesIDToken(t *testing.T) {
	priv, jwksBody := testRSAKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwksBody)
	}))
	defer srv.Close()

	jwks, err := authsmart.NewJWKS(srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Unix(1_700_000_000, 0)
	claims := map[string]any{
		"iss":            "https://issuer.example",
		"sub":            "user-1",
		"aud":            "client-id",
		"exp":            now.Add(time.Hour).Unix(),
		"iat":            now.Unix(),
		"nonce":          "nonce-xyz",
		"fhirUser":       "Practitioner/99",
		"principal_uid":  "uid-42",
		"principal_type": "PERSON",
	}
	idTok := signJWT(t, priv, "test-kid", claims)

	tr := authsmart.TokenResponse{
		AccessToken: "at",
		Patient:     "p-1",
		IDToken:     idTok,
		Scope:       "openid",
	}
	lc, err := smart.LaunchContextFromTokenResponse(
		context.Background(), tr,
		smart.WithJWKS(jwks),
		smart.WithIssuer("https://issuer.example"),
		smart.WithClientID("client-id"),
		smart.WithExpectedNonce("nonce-xyz"),
		smart.WithValidationTime(now),
	)
	if err != nil {
		t.Fatal(err)
	}
	if lc.IDToken == nil || lc.IDToken.Subject != "user-1" {
		t.Fatalf("id claims = %#v", lc.IDToken)
	}
	if lc.Principal == nil || lc.Principal.UID != "uid-42" || lc.Principal.Type != smart.PrincipalTypePerson {
		t.Fatalf("principal = %#v", lc.Principal)
	}
}

func TestValidateIDTokenRejectsExpired(t *testing.T) {
	priv, jwksBody := testRSAKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(jwksBody)
	}))
	defer srv.Close()
	jwks, err := authsmart.NewJWKS(srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Unix(1_700_000_000, 0)
	claims := map[string]any{
		"iss": "https://issuer.example",
		"sub": "u",
		"aud": "c",
		"exp": now.Add(-time.Hour).Unix(),
		"iat": now.Add(-2 * time.Hour).Unix(),
	}
	idTok := signJWT(t, priv, "test-kid", claims)
	_, err = smart.ValidateIDToken(context.Background(), idTok, jwks, "https://issuer.example", "c", "", now, nil)
	if err == nil || !isJWKSFail(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestLaunchContextRequiresTrustAnchorsForIDToken(t *testing.T) {
	priv, jwksBody := testRSAKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(jwksBody)
	}))
	defer srv.Close()
	jwks, err := authsmart.NewJWKS(srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	idTok := signJWT(t, priv, "test-kid", map[string]any{
		"iss": "https://issuer.example",
		"sub": "u",
		"aud": "client-id",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tr := authsmart.TokenResponse{IDToken: idTok}

	_, err = smart.LaunchContextFromTokenResponse(context.Background(), tr, smart.WithJWKS(jwks))
	if err == nil || !errors.Is(err, auth.ErrInvalidConfig) {
		t.Fatalf("missing issuer/client_id: err = %v", err)
	}
}

func TestValidateIDTokenRejectsFutureNBF(t *testing.T) {
	priv, jwksBody := testRSAKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(jwksBody)
	}))
	defer srv.Close()
	jwks, err := authsmart.NewJWKS(srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	claims := map[string]any{
		"iss": "https://issuer.example",
		"sub": "u",
		"aud": "c",
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(),
		"nbf": now.Add(2 * time.Hour).Unix(),
	}
	idTok := signJWT(t, priv, "test-kid", claims)
	_, err = smart.ValidateIDToken(context.Background(), idTok, jwks, "https://issuer.example", "c", "", now, nil)
	if err == nil || !isJWKSFail(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestPrincipalFromTokenResponseBody(t *testing.T) {
	tr := authsmart.TokenResponse{
		Patient: "p1",
		Raw: map[string]any{
			"principal_uid":  "body-uid",
			"principal_type": "PERSON",
		},
	}
	lc, err := smart.LaunchContextFromTokenResponse(context.Background(), tr)
	if err != nil {
		t.Fatal(err)
	}
	if lc.Principal == nil || lc.Principal.UID != "body-uid" {
		t.Fatalf("principal = %#v", lc.Principal)
	}
}

func TestPrincipalFromCustomClaimNames(t *testing.T) {
	priv, jwksBody := testRSAKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(jwksBody)
	}))
	defer srv.Close()
	jwks, err := authsmart.NewJWKS(srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	idTok := signJWT(t, priv, "test-kid", map[string]any{
		"iss":         "https://issuer.example",
		"sub":         "u",
		"aud":         "client-id",
		"exp":         now.Add(time.Hour).Unix(),
		"iat":         now.Unix(),
		"custom_uid":  "uid-99",
		"custom_type": "AGENT",
	})
	tr := authsmart.TokenResponse{IDToken: idTok}
	lc, err := smart.LaunchContextFromTokenResponse(
		context.Background(), tr,
		smart.WithJWKS(jwks),
		smart.WithIssuer("https://issuer.example"),
		smart.WithClientID("client-id"),
		smart.WithValidationTime(now),
		smart.WithPrincipalClaimNames(smart.PrincipalClaimNames{
			UIDClaim:  "custom_uid",
			TypeClaim: "custom_type",
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if lc.Principal == nil || lc.Principal.UID != "uid-99" || lc.Principal.Type != smart.PrincipalTypeAgent {
		t.Fatalf("principal = %#v", lc.Principal)
	}
}

func TestPrincipalFromFHIRUserClaimName(t *testing.T) {
	priv, jwksBody := testRSAKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(jwksBody)
	}))
	defer srv.Close()
	jwks, err := authsmart.NewJWKS(srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	idTok := signJWT(t, priv, "test-kid", map[string]any{
		"iss":      "https://issuer.example",
		"sub":      "u",
		"aud":      "client-id",
		"exp":      now.Add(time.Hour).Unix(),
		"iat":      now.Unix(),
		"fhirUser": "Practitioner/custom",
	})
	tr := authsmart.TokenResponse{IDToken: idTok}
	lc, err := smart.LaunchContextFromTokenResponse(
		context.Background(), tr,
		smart.WithJWKS(jwks),
		smart.WithIssuer("https://issuer.example"),
		smart.WithClientID("client-id"),
		smart.WithValidationTime(now),
		smart.WithPrincipalClaimNames(smart.PrincipalClaimNames{UIDClaim: "fhirUser"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if lc.Principal == nil || lc.Principal.UID != "Practitioner/custom" {
		t.Fatalf("principal = %#v", lc.Principal)
	}
}

func TestPrincipalAbsentWhenNoClaims(t *testing.T) {
	tr := authsmart.TokenResponse{Patient: "p"}
	lc, err := smart.LaunchContextFromTokenResponse(context.Background(), tr)
	if err != nil {
		t.Fatal(err)
	}
	if lc.Principal != nil {
		t.Fatalf("principal = %#v", lc.Principal)
	}
}

// TestLaunchContextOpenEHRClaims asserts that the openEHR-native ehrId and
// episodeId claims are mapped onto LaunchContext typed fields. REQ-064
func TestLaunchContextOpenEHRClaims(t *testing.T) {
	tr := authsmart.TokenResponse{
		AccessToken: "tok",
		EHRID:       "ehr-1",
		EpisodeID:   "ep-9",
	}
	lc, err := smart.LaunchContextFromTokenResponse(context.Background(), tr)
	if err != nil {
		t.Fatal(err)
	}
	if lc.EHRID != "ehr-1" {
		t.Fatalf("EHRID = %q, want %q", lc.EHRID, "ehr-1")
	}
	if lc.EpisodeID != "ep-9" {
		t.Fatalf("EpisodeID = %q, want %q", lc.EpisodeID, "ep-9")
	}
}

// TestLaunchContextSMARTCompatExtras asserts that the SMART-compat extras
// (intent, smart_style_url, need_patient_banner, tenant) are mapped onto
// LaunchContext typed fields. REQ-064
func TestLaunchContextSMARTCompatExtras(t *testing.T) {
	wantBanner := true
	tr := authsmart.TokenResponse{
		AccessToken:       "tok",
		Intent:            "patient-search",
		SMARTStyleURL:     "https://example.com/style.json",
		NeedPatientBanner: &wantBanner,
		Tenant:            "tenant-42",
	}
	lc, err := smart.LaunchContextFromTokenResponse(context.Background(), tr)
	if err != nil {
		t.Fatal(err)
	}
	if lc.Intent != "patient-search" {
		t.Fatalf("Intent = %q, want %q", lc.Intent, "patient-search")
	}
	if lc.SMARTStyleURL != "https://example.com/style.json" {
		t.Fatalf("SMARTStyleURL = %q, want %q", lc.SMARTStyleURL, "https://example.com/style.json")
	}
	if lc.NeedPatientBanner == nil || !*lc.NeedPatientBanner {
		t.Fatalf("NeedPatientBanner = %v, want non-nil true", lc.NeedPatientBanner)
	}
	if lc.Tenant != "tenant-42" {
		t.Fatalf("Tenant = %q, want %q", lc.Tenant, "tenant-42")
	}
}

func testRSAKey(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.PublicKey
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
	body, err := json.Marshal(map[string]any{
		"keys": []map[string]string{{
			"kty": "RSA",
			"kid": "test-kid",
			"n":   n,
			"e":   e,
			"alg": "RS256",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return priv, body
}

func signJWT(t *testing.T, priv *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	hdr, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": kid})
	pl, _ := json.Marshal(claims)
	hb := base64.RawURLEncoding.EncodeToString(hdr)
	pb := base64.RawURLEncoding.EncodeToString(pl)
	input := hb + "." + pb
	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	sb := base64.RawURLEncoding.EncodeToString(sig)
	return input + "." + sb
}

func isJWKSFail(err error) bool {
	return errors.Is(err, auth.ErrJWKSValidationFailed)
}
