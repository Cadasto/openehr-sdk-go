package smart

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const (
	codeVerifierLen = 64
	challengeMethod = "S256"
	stateLen        = 32 // 256-bit CSRF state entropy (RFC 6749 §10.10)
)

// PKCEPair holds the RFC 7636 verifier and S256 challenge.
type PKCEPair struct {
	Verifier  string
	Challenge string
}

// NewPKCEPair generates a cryptographically random code_verifier and
// its S256 code_challenge (REQ-061). Plain challenge method is
// prohibited.
func NewPKCEPair() (PKCEPair, error) {
	verifier, err := randBase64URL(codeVerifierLen)
	if err != nil {
		return PKCEPair{}, fmt.Errorf("smart: generate code_verifier: %w", err)
	}
	sum := sha256.Sum256([]byte(verifier))
	return PKCEPair{
		Verifier:  verifier,
		Challenge: base64URLEncode(sum[:]),
	}, nil
}

func base64URLEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// randBase64URL returns n cryptographically random bytes encoded as an
// unpadded base64url string.
func randBase64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64URLEncode(b), nil
}
