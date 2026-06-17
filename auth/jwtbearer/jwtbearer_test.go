package jwtbearer

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
)

func newKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// decodeJWT splits a JWT into header, claims, signature for inspection.
func decodeJWT(t *testing.T, jwt string) (header, claims map[string]any, sig []byte) {
	t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt has %d parts, want 3", len(parts))
	}
	dec := func(s string) []byte {
		b, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			t.Fatalf("decode %q: %v", s, err)
		}
		return b
	}
	header = map[string]any{}
	claims = map[string]any{}
	if err := json.Unmarshal(dec(parts[0]), &header); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(dec(parts[1]), &claims); err != nil {
		t.Fatal(err)
	}
	sig = dec(parts[2])
	return header, claims, sig
}

// verifyRSA verifies PKCS1v15 signatures for RS256, RS384, or RS512. REQ-068
func verifyRSA(t *testing.T, alg string, pub *rsa.PublicKey, jwt string) {
	t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt has %d parts", len(parts))
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(parts[0] + "." + parts[1])
	var hashID crypto.Hash
	var digest []byte
	switch alg {
	case "RS256":
		hashID = crypto.SHA256
		h := sha256.Sum256(input)
		digest = h[:]
	case "RS384":
		hashID = crypto.SHA384
		h := sha512.Sum384(input)
		digest = h[:]
	default:
		t.Fatalf("verifyRSA: unsupported alg %q", alg)
	}
	if err := rsa.VerifyPKCS1v15(pub, hashID, digest, sig); err != nil {
		t.Errorf("RSA signature verify (%s): %v", alg, err)
	}
}

// verifyECDSA verifies an ECDSA signature encoded as JOSE r‖s. REQ-068
func verifyECDSA(t *testing.T, alg string, pub *ecdsa.PublicKey, jwt string) {
	t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt has %d parts", len(parts))
	}
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	// JOSE ECDSA signatures are r‖s, each padded to the curve's byte length.
	if len(sigBytes)%2 != 0 {
		t.Fatalf("ECDSA JOSE sig length %d is odd", len(sigBytes))
	}
	half := len(sigBytes) / 2
	r := new(big.Int).SetBytes(sigBytes[:half])
	s := new(big.Int).SetBytes(sigBytes[half:])
	input := []byte(parts[0] + "." + parts[1])
	var digest []byte
	switch alg {
	case "ES256":
		h := sha256.Sum256(input)
		digest = h[:]
	case "ES384":
		h := sha512.Sum384(input)
		digest = h[:]
	default:
		t.Fatalf("verifyECDSA: unsupported alg %q", alg)
	}
	if !ecdsa.VerifyASN1(pub, digest, mustEncodeASN1(t, r, s)) {
		t.Errorf("ECDSA signature verify (%s): invalid", alg)
	}
}

// mustEncodeASN1 encodes (r, s) as DER for ecdsa.VerifyASN1.
func mustEncodeASN1(t *testing.T, r, s *big.Int) []byte {
	t.Helper()
	// Minimal DER SEQUENCE { INTEGER r, INTEGER s }
	encInt := func(n *big.Int) []byte {
		b := n.Bytes()
		if len(b) == 0 {
			b = []byte{0}
		}
		if b[0]&0x80 != 0 {
			b = append([]byte{0}, b...)
		}
		return append([]byte{0x02, byte(len(b))}, b...)
	}
	rb, sb := encInt(r), encInt(s)
	body := append(rb, sb...)
	return append([]byte{0x30, byte(len(body))}, body...)
}

// TestClaimsSignerProducesValidJWT covers the default RS384 path: claims
// structure, header fields, and signature verification. REQ-068
func TestClaimsSignerProducesValidJWT(t *testing.T) {
	key := newKey(t)
	signer, err := NewClaimsSigner(ClaimsTemplate{
		Issuer:   "client-xyz",
		Audience: "https://auth.example.com/token",
		Extra:    map[string]any{"scope": "patient/*.read"},
	}, key, WithKeyID("k1"))
	if err != nil {
		t.Fatal(err)
	}
	jwt, err := signer.Assertion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	header, claims, _ := decodeJWT(t, jwt)
	// Default is RS384 (SMART client-confidential-asymmetric baseline). REQ-068
	if header["alg"] != "RS384" {
		t.Errorf("alg = %v, want RS384", header["alg"])
	}
	if header["typ"] != "JWT" {
		t.Errorf("typ = %v", header["typ"])
	}
	if header["kid"] != "k1" {
		t.Errorf("kid = %v", header["kid"])
	}
	if claims["iss"] != "client-xyz" {
		t.Errorf("iss = %v", claims["iss"])
	}
	if claims["sub"] != "client-xyz" {
		t.Errorf("sub defaulted from iss = %v", claims["sub"])
	}
	if claims["aud"] != "https://auth.example.com/token" {
		t.Errorf("aud = %v", claims["aud"])
	}
	if claims["jti"] == "" || claims["jti"] == nil {
		t.Errorf("jti missing")
	}
	if claims["scope"] != "patient/*.read" {
		t.Errorf("scope = %v", claims["scope"])
	}
	verifyRSA(t, "RS384", &key.PublicKey, jwt)
}

func TestClaimsSignerJTIUnique(t *testing.T) {
	key := newKey(t)
	signer, _ := NewClaimsSigner(ClaimsTemplate{Issuer: "i", Audience: "a"}, key)
	seen := map[string]bool{}
	for range 4 {
		jwt, err := signer.Assertion(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		_, claims, _ := decodeJWT(t, jwt)
		jti, _ := claims["jti"].(string)
		if seen[jti] {
			t.Errorf("jti %q reused", jti)
		}
		seen[jti] = true
	}
}

func TestJTIHasCryptoRandEntropy(t *testing.T) {
	// A 24-byte JTI (8 time + 8 counter + 8 rand) base64url-encodes to
	// exactly 32 characters (24 * 4/3 = 32, no padding with RawURLEncoding).
	// The old 16-byte JTI encoded to 22 characters. This length check is a
	// deterministic proxy for verifying that the crypto/rand bytes are
	// present.
	s := &ClaimsSigner{}
	jti, err := newJTI(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(jti) != 32 {
		t.Fatalf("JTI len = %d, want 32 (24 bytes base64url); crypto/rand bytes missing?", len(jti))
	}

	// Two fresh signers with counter=1 and potentially the same nanosecond
	// must produce distinct JTIs thanks to the rand component.
	s2 := &ClaimsSigner{}
	jti2, err := newJTI(s2)
	if err != nil {
		t.Fatal(err)
	}
	if jti == jti2 {
		t.Fatalf("two fresh signers (counter=1 each) produced identical JTI %q — crypto/rand entropy missing", jti)
	}

	// Verify 1000 calls from a single signer are all distinct (length + uniqueness
	// are a deterministic proxy for crypto/rand entropy presence).
	const N = 1000
	seen := make(map[string]struct{}, N+1)
	seen[jti] = struct{}{} // seed with the first signer's initial JTI
	for range N {
		id, err := newJTI(s)
		if err != nil {
			t.Fatal(err)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate JTI %q after %d calls", id, len(seen))
		}
		seen[id] = struct{}{}
	}
}

func newECKey(t *testing.T, curve elliptic.Curve) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestClaimsSignerValidates(t *testing.T) {
	rsaKey := newKey(t)
	ecKey := newECKey(t, elliptic.P384())
	tests := []struct {
		name string
		tmpl ClaimsTemplate
		k    crypto.Signer
		alg  string
	}{
		// basic field validations — REQ-068
		{"no signer", ClaimsTemplate{Issuer: "i", Audience: "a"}, nil, "RS384"},
		{"no issuer", ClaimsTemplate{Audience: "a"}, rsaKey, "RS384"},
		{"no audience", ClaimsTemplate{Issuer: "i"}, rsaKey, "RS384"},
		// unsupported algorithm families
		{"unsupported alg HS256", ClaimsTemplate{Issuer: "i", Audience: "a"}, rsaKey, "HS256"},
		// key-type mismatches — REQ-068
		{"ES384 with RSA key", ClaimsTemplate{Issuer: "i", Audience: "a"}, rsaKey, "ES384"},
		{"RS384 with EC key", ClaimsTemplate{Issuer: "i", Audience: "a"}, ecKey, "RS384"},
		{"ES256 with P-384 key", ClaimsTemplate{Issuer: "i", Audience: "a"}, ecKey, "ES256"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var opts []SignerOption
			if tc.alg != "" {
				opts = append(opts, WithAlgorithm(tc.alg))
			}
			_, err := NewClaimsSigner(tc.tmpl, tc.k, opts...)
			if !errors.Is(err, auth.ErrInvalidConfig) {
				t.Errorf("expected ErrInvalidConfig, got %v", err)
			}
		})
	}
}

func TestStaticAssertion(t *testing.T) {
	src := StaticAssertion("pre-minted-jwt")
	got, err := src.Assertion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "pre-minted-jwt" {
		t.Errorf("got %q", got)
	}
}

func TestSourceExchangesAssertion(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if g := r.PostForm.Get("grant_type"); g != GrantType {
			t.Errorf("grant_type = %q", g)
		}
		if a := r.PostForm.Get("assertion"); a != "the-jwt" {
			t.Errorf("assertion = %q", a)
		}
		if id := r.PostForm.Get("client_id"); id != "client-xyz" {
			t.Errorf("client_id = %q", id)
		}
		_, _ = w.Write([]byte(`{"access_token":"at-1","token_type":"Bearer","expires_in":600}`))
	}))
	defer srv.Close()

	src, err := New(
		srv.URL, StaticAssertion("the-jwt"),
		WithHTTPClient(srv.Client()),
		WithClientID("client-xyz"),
	)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "at-1" {
		t.Errorf("Value = %q", tok.Value)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("expected 1 hit, got %d", got)
	}
}

func TestSourceCachesUntilExpiry(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"access_token":"at","token_type":"Bearer","expires_in":3600}`))
	}))
	defer srv.Close()
	src, _ := New(srv.URL, StaticAssertion("a"), WithHTTPClient(srv.Client()))
	for range 5 {
		if _, err := src.Token(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("expected 1 hit, got %d", got)
	}
}

func TestSourceCoalescesConcurrent(t *testing.T) {
	var hits atomic.Int32
	gate := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-gate
		_, _ = w.Write([]byte(`{"access_token":"at","token_type":"Bearer","expires_in":3600}`))
	}))
	defer srv.Close()
	src, _ := New(srv.URL, StaticAssertion("a"), WithHTTPClient(srv.Client()))
	var wg sync.WaitGroup
	const N = 5
	wg.Add(N)
	for range N {
		go func() {
			defer wg.Done()
			_, _ = src.Token(context.Background())
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(gate)
	wg.Wait()
	if got := hits.Load(); got != 1 {
		t.Errorf("expected 1 hit, got %d", got)
	}
}

func TestSourceMapsOAuth2Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"expired"}`))
	}))
	defer srv.Close()
	src, _ := New(srv.URL, StaticAssertion("a"), WithHTTPClient(srv.Client()))
	_, err := src.Token(context.Background())
	if !errors.Is(err, auth.ErrTokenExchangeFailed) {
		t.Errorf("expected ErrTokenExchangeFailed, got %v", err)
	}
	var oa *auth.OAuth2Error
	if !errors.As(err, &oa) || oa.Code != "invalid_grant" {
		t.Errorf("expected invalid_grant, got %v", oa)
	}
}

func TestNewValidates(t *testing.T) {
	_, err := New("", StaticAssertion("a"), WithHTTPClient(http.DefaultClient))
	if !errors.Is(err, auth.ErrInvalidConfig) {
		t.Errorf("missing URL: %v", err)
	}
	_, err = New("https://x/t", nil, WithHTTPClient(http.DefaultClient))
	if !errors.Is(err, auth.ErrInvalidConfig) {
		t.Errorf("missing assertion: %v", err)
	}
	_, err = New("https://x/t", StaticAssertion("a"))
	if !errors.Is(err, auth.ErrInvalidConfig) {
		t.Errorf("missing http client: %v", err)
	}
}

// TestClaimsSignerDefaultRS384 verifies that a signer built without
// WithAlgorithm produces alg=="RS384" (SMART client-confidential-asymmetric
// baseline). REQ-068
func TestClaimsSignerDefaultRS384(t *testing.T) {
	key := newKey(t)
	s, err := NewClaimsSigner(ClaimsTemplate{Issuer: "i", Audience: "a"}, key)
	if err != nil {
		t.Fatal(err)
	}
	jwt, err := s.Assertion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	header, _, _ := decodeJWT(t, jwt)
	if header["alg"] != "RS384" {
		t.Errorf("default alg = %v, want RS384", header["alg"])
	}
	verifyRSA(t, "RS384", &key.PublicKey, jwt)
}

// TestClaimsSignerRS384 exercises RS384 explicitly and verifies the signature
// against the RSA public key. HL7 SMART client-confidential-asymmetric SHALL
// support RS384. REQ-068
func TestClaimsSignerRS384(t *testing.T) {
	key := newKey(t)
	s, err := NewClaimsSigner(
		ClaimsTemplate{Issuer: "client-a", Audience: "https://as.example/token"},
		key,
		WithAlgorithm("RS384"),
		WithKeyID("rsa-kid"),
	)
	if err != nil {
		t.Fatal(err)
	}
	jwt, err := s.Assertion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	header, claims, _ := decodeJWT(t, jwt)
	if header["alg"] != "RS384" {
		t.Errorf("alg = %v, want RS384", header["alg"])
	}
	if header["kid"] != "rsa-kid" {
		t.Errorf("kid = %v", header["kid"])
	}
	if claims["iss"] != "client-a" {
		t.Errorf("iss = %v", claims["iss"])
	}
	verifyRSA(t, "RS384", &key.PublicKey, jwt)
}

// TestClaimsSignerRS256 verifies that RS256 is still reachable via
// WithAlgorithm("RS256") for backwards compatibility. REQ-068
func TestClaimsSignerRS256(t *testing.T) {
	key := newKey(t)
	s, err := NewClaimsSigner(
		ClaimsTemplate{Issuer: "client-b", Audience: "https://as.example/token"},
		key,
		WithAlgorithm("RS256"),
	)
	if err != nil {
		t.Fatal(err)
	}
	jwt, err := s.Assertion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	header, _, _ := decodeJWT(t, jwt)
	if header["alg"] != "RS256" {
		t.Errorf("alg = %v, want RS256", header["alg"])
	}
	verifyRSA(t, "RS256", &key.PublicKey, jwt)
}

// TestClaimsSignerES384 exercises ES384 with a P-384 key. HL7 SMART
// client-confidential-asymmetric SHALL support ES384. REQ-068
func TestClaimsSignerES384(t *testing.T) {
	key := newECKey(t, elliptic.P384())
	s, err := NewClaimsSigner(
		ClaimsTemplate{Issuer: "client-ec", Audience: "https://as.example/token"},
		key,
		WithAlgorithm("ES384"),
		WithKeyID("ec-kid"),
	)
	if err != nil {
		t.Fatal(err)
	}
	jwt, err := s.Assertion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	header, claims, _ := decodeJWT(t, jwt)
	if header["alg"] != "ES384" {
		t.Errorf("alg = %v, want ES384", header["alg"])
	}
	if header["kid"] != "ec-kid" {
		t.Errorf("kid = %v", header["kid"])
	}
	if claims["iss"] != "client-ec" {
		t.Errorf("iss = %v", claims["iss"])
	}
	verifyECDSA(t, "ES384", &key.PublicKey, jwt)
}

// TestClaimsSignerES256 exercises ES256 with a P-256 key. REQ-068
func TestClaimsSignerES256(t *testing.T) {
	key := newECKey(t, elliptic.P256())
	s, err := NewClaimsSigner(
		ClaimsTemplate{Issuer: "client-ec256", Audience: "https://as.example/token"},
		key,
		WithAlgorithm("ES256"),
	)
	if err != nil {
		t.Fatal(err)
	}
	jwt, err := s.Assertion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	header, _, _ := decodeJWT(t, jwt)
	if header["alg"] != "ES256" {
		t.Errorf("alg = %v, want ES256", header["alg"])
	}
	verifyECDSA(t, "ES256", &key.PublicKey, jwt)
}
