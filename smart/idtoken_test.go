package smart_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gojose "github.com/go-jose/go-jose/v4"

	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart"
)

// defaultIDClaims returns a baseline, valid id_token claim set anchored at now.
func defaultIDClaims(now time.Time) map[string]any {
	return map[string]any{
		"iss":      "https://issuer.example",
		"sub":      "user-1",
		"aud":      "client-id",
		"exp":      now.Add(time.Hour).Unix(),
		"iat":      now.Unix(),
		"nonce":    "nonce-xyz",
		"fhirUser": "Practitioner/99",
	}
}

// joseSign signs claims with the given JOSE alg and private key, emitting kid.
func joseSign(t *testing.T, alg gojose.SignatureAlgorithm, key any, kid string, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	opts := (&gojose.SignerOptions{}).WithType("JWT")
	if kid != "" {
		opts = opts.WithHeader("kid", kid)
	}
	signer, err := gojose.NewSigner(gojose.SigningKey{Algorithm: alg, Key: key}, opts)
	if err != nil {
		t.Fatal(err)
	}
	jws, err := signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	compact, err := jws.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return compact
}

// jwksFromPublicKeys serves a JWKS document advertising pub under kid.
func jwksServer(t *testing.T, kid string, pub crypto.PublicKey, alg string) *authsmart.JWKS {
	t.Helper()
	jwk := gojose.JSONWebKey{Key: pub, KeyID: kid, Algorithm: alg, Use: "sig"}
	set := gojose.JSONWebKeySet{Keys: []gojose.JSONWebKey{jwk}}
	body, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
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

func newRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func newECKey(t *testing.T, curve elliptic.Curve) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// TestValidateIDTokenRS256 confirms the RS256 baseline still verifies. REQ-062 REQ-064
func TestValidateIDTokenRS256(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwks := jwksServer(t, "kid-rs256", &priv.PublicKey, "RS256")
	tok := joseSign(t, gojose.RS256, priv, "kid-rs256", defaultIDClaims(now))

	claims, err := smart.ValidateIDToken(context.Background(), tok, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, nil)
	if err != nil {
		t.Fatalf("RS256 should verify: %v", err)
	}
	if claims.Subject != "user-1" || claims.FHIRUser != "Practitioner/99" {
		t.Fatalf("claims = %#v", claims)
	}
}

// TestValidateIDTokenRS384 confirms RS384 verifies (alg agility). REQ-062 REQ-064
func TestValidateIDTokenRS384(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwks := jwksServer(t, "kid-rs384", &priv.PublicKey, "RS384")
	tok := joseSign(t, gojose.RS384, priv, "kid-rs384", defaultIDClaims(now))

	claims, err := smart.ValidateIDToken(context.Background(), tok, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, nil)
	if err != nil {
		t.Fatalf("RS384 should verify: %v", err)
	}
	if claims.Subject != "user-1" {
		t.Fatalf("claims = %#v", claims)
	}
}

// TestValidateIDTokenES384 confirms ES384 (ECDSA P-384) verifies. REQ-062 REQ-064
func TestValidateIDTokenES384(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newECKey(t, elliptic.P384())
	jwks := jwksServer(t, "kid-es384", &priv.PublicKey, "ES384")
	tok := joseSign(t, gojose.ES384, priv, "kid-es384", defaultIDClaims(now))

	claims, err := smart.ValidateIDToken(context.Background(), tok, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, nil)
	if err != nil {
		t.Fatalf("ES384 should verify: %v", err)
	}
	if claims.Subject != "user-1" {
		t.Fatalf("claims = %#v", claims)
	}
}

// TestValidateIDTokenES256 confirms ES256 (ECDSA P-256) verifies. REQ-062 REQ-064
func TestValidateIDTokenES256(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newECKey(t, elliptic.P256())
	jwks := jwksServer(t, "kid-es256", &priv.PublicKey, "ES256")
	tok := joseSign(t, gojose.ES256, priv, "kid-es256", defaultIDClaims(now))

	claims, err := smart.ValidateIDToken(context.Background(), tok, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, nil)
	if err != nil {
		t.Fatalf("ES256 should verify: %v", err)
	}
	if claims.Subject != "user-1" {
		t.Fatalf("claims = %#v", claims)
	}
}

// TestValidateIDTokenRespectsDiscoveryAllowlist confirms the discovery
// id_token_signing_alg_values_supported narrows the accepted set: a token
// signed with an alg outside the advertised list is rejected. REQ-062 REQ-064
func TestValidateIDTokenRejectsUnlistedAlg(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newECKey(t, elliptic.P384())
	jwks := jwksServer(t, "kid-es384", &priv.PublicKey, "ES384")
	tok := joseSign(t, gojose.ES384, priv, "kid-es384", defaultIDClaims(now))

	// Allowlist permits only RS256; ES384 must be rejected.
	_, err := smart.ValidateIDToken(context.Background(), tok, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, []string{"RS256"})
	if err == nil || !isJWKSFail(err) {
		t.Fatalf("unlisted alg should be rejected with JWKS failure, got %v", err)
	}
}

// TestValidateIDTokenRejectsAlgNone confirms an unsigned (alg:none) token is
// rejected. REQ-062 REQ-064
func TestValidateIDTokenRejectsAlgNone(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwks := jwksServer(t, "kid-rs256", &priv.PublicKey, "RS256")

	hdr, _ := json.Marshal(map[string]string{"alg": "none", "typ": "JWT", "kid": "kid-rs256"})
	pl, _ := json.Marshal(defaultIDClaims(now))
	tok := base64.RawURLEncoding.EncodeToString(hdr) + "." +
		base64.RawURLEncoding.EncodeToString(pl) + "."

	_, err := smart.ValidateIDToken(context.Background(), tok, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, nil)
	if err == nil || !isJWKSFail(err) {
		t.Fatalf("alg:none should be rejected with JWKS failure, got %v", err)
	}
}

// TestValidateIDTokenRejectsBadNonce confirms the SDK's nonce check still
// applies after go-oidc signature verification. REQ-064
func TestValidateIDTokenRejectsBadNonce(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwks := jwksServer(t, "kid-rs256", &priv.PublicKey, "RS256")
	tok := joseSign(t, gojose.RS256, priv, "kid-rs256", defaultIDClaims(now))

	_, err := smart.ValidateIDToken(context.Background(), tok, jwks,
		"https://issuer.example", "client-id", "wrong-nonce", now, nil)
	if err == nil || !isJWKSFail(err) {
		t.Fatalf("nonce mismatch should be rejected, got %v", err)
	}
}
