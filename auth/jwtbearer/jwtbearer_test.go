package jwtbearer

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
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

func verifyRS256(t *testing.T, pub *rsa.PublicKey, jwt string) {
	t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt has %d parts", len(parts))
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		t.Errorf("signature verify: %v", err)
	}
}

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
	if header["alg"] != "RS256" {
		t.Errorf("alg = %v", header["alg"])
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
	verifyRS256(t, &key.PublicKey, jwt)
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

func TestClaimsSignerValidates(t *testing.T) {
	tests := []struct {
		name string
		tmpl ClaimsTemplate
		k    crypto.Signer
		alg  string
	}{
		{"no signer", ClaimsTemplate{Issuer: "i", Audience: "a"}, nil, "RS256"},
		{"no issuer", ClaimsTemplate{Audience: "a"}, newKey(t), "RS256"},
		{"no audience", ClaimsTemplate{Issuer: "i"}, newKey(t), "RS256"},
		{"unsupported alg", ClaimsTemplate{Issuer: "i", Audience: "a"}, newKey(t), "HS256"},
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
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
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

	src, err := New(srv.URL, StaticAssertion("the-jwt"),
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
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("expected 1 hit, got %d", hits)
	}
}

func TestSourceCachesUntilExpiry(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(`{"access_token":"at","token_type":"Bearer","expires_in":3600}`))
	}))
	defer srv.Close()
	src, _ := New(srv.URL, StaticAssertion("a"), WithHTTPClient(srv.Client()))
	for range 5 {
		if _, err := src.Token(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("expected 1 hit, got %d", got)
	}
}

func TestSourceCoalescesConcurrent(t *testing.T) {
	var hits int32
	gate := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
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
	if got := atomic.LoadInt32(&hits); got != 1 {
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
