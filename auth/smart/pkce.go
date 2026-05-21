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
	b := make([]byte, codeVerifierLen)
	if _, err := rand.Read(b); err != nil {
		return PKCEPair{}, fmt.Errorf("smart: generate code_verifier: %w", err)
	}
	verifier := base64URLEncode(b)
	sum := sha256.Sum256([]byte(verifier))
	return PKCEPair{
		Verifier:  verifier,
		Challenge: base64URLEncode(sum[:]),
	}, nil
}

func base64URLEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}
