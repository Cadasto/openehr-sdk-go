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
	lc, err := smart.LaunchContextFromTokenResponse(context.Background(), tr,
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
	_, err = smart.ValidateIDToken(context.Background(), idTok, jwks, "https://issuer.example", "c", "", now)
	if err == nil || !isJWKSFail(err) {
		t.Fatalf("err = %v", err)
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
