package smart

import (
	"crypto/rand"
	"encoding/base64"

	"golang.org/x/oauth2"
)

const (
	challengeMethod = "S256"
	stateLen        = 32 // 256-bit CSRF state entropy (RFC 6749 §10.10)
)

// PKCEPair holds the RFC 7636 verifier and S256 challenge.
type PKCEPair struct {
	Verifier  string
	Challenge string
}

// NewPKCEPair generates a cryptographically random code_verifier and its S256
// code_challenge (REQ-061), delegating to golang.org/x/oauth2's RFC 7636
// helpers (GenerateVerifier — 32 octets of entropy — and
// S256ChallengeFromVerifier). The plain challenge method is never used. The
// error return is retained for API stability; the current implementation does
// not fail.
func NewPKCEPair() (PKCEPair, error) {
	v := oauth2.GenerateVerifier()
	return PKCEPair{
		Verifier:  v,
		Challenge: oauth2.S256ChallengeFromVerifier(v),
	}, nil
}

func base64URLEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// randBase64URL returns n cryptographically random bytes encoded as an
// unpadded base64url string. Used for the OAuth `state` (CSRF) value.
func randBase64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64URLEncode(b), nil
}
