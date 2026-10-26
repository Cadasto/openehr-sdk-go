package smart

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"

	"golang.org/x/oauth2"
)

const (
	challengeMethod = "S256"
	stateLen        = 32 // 256-bit CSRF state entropy (RFC 6749 §10.10)
)

// PKCEPair holds the RFC 7636 verifier and S256 challenge.
//
// Printed with fmt or logged with log/slog, a PKCEPair shows its Verifier as
// "[redacted]" and its Challenge, which travels in the authorization URL, as
// it is: see [PKCEPair.String]. Its JSON encoding is unchanged.
type PKCEPair struct {
	Verifier  string
	Challenge string
}

// String describes p with its Verifier replaced by "[redacted]", or left
// empty when it is empty, and its Challenge as it is.
func (p PKCEPair) String() string {
	return fmt.Sprintf("smart.PKCEPair{Verifier: %q, Challenge: %q}", redact(p.Verifier), p.Challenge)
}

// GoString returns the text of [PKCEPair.String], so that printing p in Go
// syntax shows no verifier either.
func (p PKCEPair) GoString() string { return p.String() }

// Format writes the text of [PKCEPair.String] for every fmt verb it is called
// for, whatever the flags and width. fmt answers %T and %p without calling
// it, and %p of a PKCEPair value prints its fields, so never print a PKCEPair
// with %p.
func (p PKCEPair) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, p.String()) }

// LogValue lets log/slog log p as a group of its fields, keyed by field name,
// with the Verifier replaced as [PKCEPair.String] replaces it.
func (p PKCEPair) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("Verifier", redact(p.Verifier)),
		slog.String("Challenge", p.Challenge),
	)
}

// NewPKCEPair generates a cryptographically random code_verifier and its S256
// code_challenge, delegating to golang.org/x/oauth2's RFC 7636 helpers
// (GenerateVerifier, which uses 32 octets of entropy, and
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

// randBase64URL returns n cryptographically random bytes encoded as an
// unpadded base64url string. It generates the OAuth `state` (CSRF) value
// and the OpenID Connect nonce. crypto/rand.Read never fails and always
// fills b.
func randBase64URL(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
