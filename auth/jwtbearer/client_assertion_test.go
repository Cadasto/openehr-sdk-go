package jwtbearer

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// TestNewClientAssertionClaims pins the claims and the JOSE header of the
// HL7 SMART asymmetric client assertion: iss and sub are the client ID, aud
// is the token URL, typ is JWT, kid is the configured key ID, every
// assertion has its own jti, and exp is five minutes after iat.
func TestNewClientAssertionClaims(t *testing.T) { // REQ-068
	const (
		clientID = "client-asym"
		tokenURL = "https://as.example/token"
		kid      = "key-2026"
	)
	rsaKey := newKey(t)
	ecKey := newECKey(t, elliptic.P384())
	tests := []struct {
		alg    string
		signer crypto.Signer
		verify func(t *testing.T, jwt string)
	}{
		{alg: "RS384", signer: rsaKey, verify: func(t *testing.T, jwt string) { verifyRSA(t, "RS384", &rsaKey.PublicKey, jwt) }},
		{alg: "ES384", signer: ecKey, verify: func(t *testing.T, jwt string) { verifyECDSA(t, "ES384", &ecKey.PublicKey, jwt) }},
	}
	for _, tc := range tests {
		t.Run(tc.alg, func(t *testing.T) {
			s, err := NewClientAssertion(clientID, tokenURL, tc.signer, tc.alg, kid)
			if err != nil {
				t.Fatalf("NewClientAssertion(%q, %q, key, %q, %q): %v", clientID, tokenURL, tc.alg, kid, err)
			}
			var jtis []any
			for range 2 {
				jwt, err := s.Assertion(t.Context())
				if err != nil {
					t.Fatalf("Assertion: %v", err)
				}
				tc.verify(t, jwt)
				header, claims, _ := decodeJWT(t, jwt)
				wantHeader := map[string]any{"alg": tc.alg, "typ": "JWT", "kid": kid}
				for name, want := range wantHeader {
					if got := header[name]; got != want {
						t.Errorf("header %s = %v, want %v", name, got, want)
					}
				}
				wantClaims := map[string]any{"iss": clientID, "sub": clientID, "aud": tokenURL}
				for name, want := range wantClaims {
					if got := claims[name]; got != want {
						t.Errorf("claim %s = %v, want %v", name, got, want)
					}
				}
				jti, ok := claims["jti"].(string)
				if !ok || jti == "" {
					t.Errorf("claim jti = %v, want a non-empty string", claims["jti"])
				}
				jtis = append(jtis, claims["jti"])
				iat, iatOK := claims["iat"].(float64)
				exp, expOK := claims["exp"].(float64)
				if !iatOK || !expOK {
					t.Fatalf("claims iat = %v, exp = %v, want two numbers", claims["iat"], claims["exp"])
				}
				if got := exp - iat; got != 300 {
					t.Errorf("exp - iat = %v seconds, want 300 (five minutes)", got)
				}
			}
			if jtis[0] == jtis[1] {
				t.Errorf("two assertions share jti %v, want a unique jti each", jtis[0])
			}
		})
	}
}

// unusableSigner is a non-nil crypto.Signer whose Public panics, as a
// broken key-store adapter might.
type unusableSigner struct{}

func (unusableSigner) Public() crypto.PublicKey { panic("key store unavailable") }

func (unusableSigner) Sign(io.Reader, []byte, crypto.SignerOpts) ([]byte, error) {
	return nil, errors.New("key store unavailable")
}

// TestNewClientAssertionRefusesBadArguments pins that NewClientAssertion
// fails with auth.ErrInvalidConfig, and returns no signer, when an argument
// is empty (a nil signer, or a nil key of a concrete type, counts as empty),
// the signer's Public method panics, or the key does not fit the algorithm.
// It never panics itself. An empty clientID, tokenURL or kid is refused with
// a message that names the argument.
func TestNewClientAssertionRefusesBadArguments(t *testing.T) { // REQ-068
	rsaKey := newKey(t)
	p256Key := newECKey(t, elliptic.P256())
	tests := []struct {
		name               string
		clientID, tokenURL string
		signer             crypto.Signer
		alg, kid           string
		// wantInMsg, when set, must appear in the error message.
		wantInMsg string
	}{
		{name: "empty clientID", tokenURL: "https://as.example/token", signer: rsaKey, alg: "RS384", kid: "k1", wantInMsg: "clientID"},
		{name: "empty tokenURL", clientID: "c1", signer: rsaKey, alg: "RS384", kid: "k1", wantInMsg: "tokenURL"},
		{name: "nil signer", clientID: "c1", tokenURL: "https://as.example/token", alg: "RS384", kid: "k1"},
		{name: "nil RSA key", clientID: "c1", tokenURL: "https://as.example/token", signer: (*rsa.PrivateKey)(nil), alg: "RS384", kid: "k1"},
		{name: "nil ECDSA key", clientID: "c1", tokenURL: "https://as.example/token", signer: (*ecdsa.PrivateKey)(nil), alg: "ES384", kid: "k1"},
		{name: "nil Ed25519 key", clientID: "c1", tokenURL: "https://as.example/token", signer: ed25519.PrivateKey(nil), alg: "RS384", kid: "k1"},
		{name: "nil custom signer", clientID: "c1", tokenURL: "https://as.example/token", signer: (*opaqueRSASigner)(nil), alg: "RS384", kid: "k1"},
		{
			name: "signer whose Public panics", clientID: "c1", tokenURL: "https://as.example/token", signer: unusableSigner{}, alg: "RS384", kid: "k1",
			wantInMsg: "Public method panicked",
		},
		{name: "empty alg", clientID: "c1", tokenURL: "https://as.example/token", signer: rsaKey, kid: "k1"},
		{name: "empty kid", clientID: "c1", tokenURL: "https://as.example/token", signer: rsaKey, alg: "RS384", wantInMsg: "kid"},
		{name: "RSA key for ES384", clientID: "c1", tokenURL: "https://as.example/token", signer: rsaKey, alg: "ES384", kid: "k1"},
		{name: "P-256 key for ES384", clientID: "c1", tokenURL: "https://as.example/token", signer: p256Key, alg: "ES384", kid: "k1"},
		{name: "ECDSA key for RS384", clientID: "c1", tokenURL: "https://as.example/token", signer: p256Key, alg: "RS384", kid: "k1"},
		{name: "unsupported alg", clientID: "c1", tokenURL: "https://as.example/token", signer: rsaKey, alg: "HS256", kid: "k1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("NewClientAssertion(%q, %q, %T, %q, %q) panicked: %v, want auth.ErrInvalidConfig",
						tc.clientID, tc.tokenURL, tc.signer, tc.alg, tc.kid, r)
				}
			}()
			s, err := NewClientAssertion(tc.clientID, tc.tokenURL, tc.signer, tc.alg, tc.kid)
			if !errors.Is(err, auth.ErrInvalidConfig) {
				t.Errorf("NewClientAssertion(%q, %q, %T, %q, %q) error = %v, want auth.ErrInvalidConfig",
					tc.clientID, tc.tokenURL, tc.signer, tc.alg, tc.kid, err)
			}
			if s != nil {
				t.Errorf("NewClientAssertion(%q, %q, %T, %q, %q) returned a signer with its error, want nil",
					tc.clientID, tc.tokenURL, tc.signer, tc.alg, tc.kid)
			}
			if tc.wantInMsg != "" && (err == nil || !strings.Contains(err.Error(), tc.wantInMsg)) {
				t.Errorf("NewClientAssertion(%q, %q, %T, %q, %q) error = %v, want a message naming %s",
					tc.clientID, tc.tokenURL, tc.signer, tc.alg, tc.kid, err, tc.wantInMsg)
			}
		})
	}
}
