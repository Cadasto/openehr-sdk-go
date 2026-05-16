package jwtbearer

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// AssertionSource produces a signed JWT bearer assertion for a single
// token-exchange request. Implementations may:
//
//   - Sign on demand from a held private key (use ClaimsSigner).
//   - Return a pre-signed assertion supplied by an upstream identity
//     service.
//   - Fetch a fresh assertion from a trusted broker for each call.
//
// Each call MUST return an unexpired JWT (the SDK does not validate
// timing). Returning a stale assertion will be rejected by the
// authorization server and surface as auth.ErrTokenExchangeFailed.
type AssertionSource interface {
	Assertion(ctx context.Context) (string, error)
}

// AssertionFunc adapts a function into an AssertionSource.
type AssertionFunc func(ctx context.Context) (string, error)

// Assertion implements AssertionSource.
func (f AssertionFunc) Assertion(ctx context.Context) (string, error) { return f(ctx) }

// StaticAssertion returns an AssertionSource that yields jwt verbatim
// on every call. Useful for short-lived integration tests where the
// caller has pre-minted a long-lived assertion.
func StaticAssertion(jwt string) AssertionSource {
	return AssertionFunc(func(ctx context.Context) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return jwt, nil
	})
}

// ClaimsTemplate captures the RFC 7523 claim set the SDK signs on the
// consumer's behalf. Iat/Exp/Jti are populated automatically by
// ClaimsSigner unless overridden.
type ClaimsTemplate struct {
	// Issuer is the "iss" claim — typically the client_id of the
	// confidential client.
	Issuer string
	// Subject is the "sub" claim. For client-authentication assertions,
	// sub == iss. For on-behalf-of assertions, sub identifies the
	// principal.
	Subject string
	// Audience is the "aud" claim — typically the token endpoint URL.
	Audience string
	// Lifetime is how long the assertion is valid. The signer sets
	// "iat" to now and "exp" to now+Lifetime. Defaults to 5 minutes.
	Lifetime time.Duration
	// Extra carries any additional claims the deployment requires
	// (e.g. "scope", "act"). Keys collide with iss/sub/aud/iat/exp/jti
	// are silently overwritten by the signed values.
	Extra map[string]any
}

// ClaimsSigner is an AssertionSource that signs claims with a held
// crypto.Signer for each call. KeyID, when set, is emitted as the "kid"
// header so the authorization server can identify the signing key
// across the deployment's JWKS.
type ClaimsSigner struct {
	Template ClaimsTemplate
	// Signer is the private-key signer. Required.
	Signer crypto.Signer
	// Algorithm is the JOSE "alg" name (RS256 is the only built-in
	// option in v1). To use a different algorithm, sign externally
	// and pass via StaticAssertion / AssertionFunc.
	Algorithm string
	// KeyID is the "kid" JWS header.
	KeyID string

	jtiCounter uint64
}

// NewClaimsSigner constructs a ClaimsSigner. Returns ErrInvalidConfig
// when required fields are missing or the algorithm is unsupported.
func NewClaimsSigner(template ClaimsTemplate, signer crypto.Signer, opts ...SignerOption) (*ClaimsSigner, error) {
	s := &ClaimsSigner{Template: template, Signer: signer, Algorithm: "RS256"}
	for _, o := range opts {
		o(s)
	}
	if s.Signer == nil {
		return nil, fmt.Errorf("%w: signer is required", auth.ErrInvalidConfig)
	}
	if template.Issuer == "" {
		return nil, fmt.Errorf("%w: ClaimsTemplate.Issuer is required", auth.ErrInvalidConfig)
	}
	if template.Audience == "" {
		return nil, fmt.Errorf("%w: ClaimsTemplate.Audience is required", auth.ErrInvalidConfig)
	}
	if s.Template.Lifetime == 0 {
		s.Template.Lifetime = 5 * time.Minute
	}
	if s.Algorithm != "RS256" {
		return nil, fmt.Errorf("%w: algorithm %q is not built in (use AssertionFunc for external signers)", auth.ErrInvalidConfig, s.Algorithm)
	}
	if _, ok := s.Signer.Public().(*rsa.PublicKey); !ok {
		return nil, fmt.Errorf("%w: RS256 requires an RSA signer", auth.ErrInvalidConfig)
	}
	return s, nil
}

// SignerOption configures a ClaimsSigner.
type SignerOption func(*ClaimsSigner)

// WithKeyID sets the JOSE "kid" header.
func WithKeyID(kid string) SignerOption {
	return func(s *ClaimsSigner) { s.KeyID = kid }
}

// WithAlgorithm overrides the JOSE "alg" name. The built-in implementation
// only supports RS256; other values are rejected at construction.
func WithAlgorithm(alg string) SignerOption {
	return func(s *ClaimsSigner) { s.Algorithm = alg }
}

// Assertion produces a freshly signed JWT bearer assertion. Each call
// allocates a new "iat"/"exp"/"jti" so two assertions from the same
// signer are never identical (RFC 7523 § 3 requires jti to be unique
// within the assertion's lifetime).
func (s *ClaimsSigner) Assertion(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	now := time.Now()
	jti, err := newJTI(s)
	if err != nil {
		return "", err
	}
	claims := maps.Clone(s.Template.Extra)
	if claims == nil {
		claims = map[string]any{}
	}
	claims["iss"] = s.Template.Issuer
	sub := s.Template.Subject
	if sub == "" {
		sub = s.Template.Issuer
	}
	claims["sub"] = sub
	claims["aud"] = s.Template.Audience
	claims["iat"] = now.Unix()
	claims["exp"] = now.Add(s.Template.Lifetime).Unix()
	claims["jti"] = jti

	header := map[string]any{"alg": s.Algorithm, "typ": "JWT"}
	if s.KeyID != "" {
		header["kid"] = s.KeyID
	}

	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("marshal JWS header: %w", err)
	}
	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal JWT claims: %w", err)
	}
	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString(headerBytes) + "." + enc.EncodeToString(claimsBytes)

	sig, err := signRS256(s.Signer, signingInput)
	if err != nil {
		return "", err
	}
	return signingInput + "." + enc.EncodeToString(sig), nil
}

func signRS256(signer crypto.Signer, input string) ([]byte, error) {
	if signer == nil {
		return nil, errors.New("nil signer")
	}
	sum := sha256.Sum256([]byte(input))
	sig, err := signer.Sign(rand.Reader, sum[:], crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("RS256 sign: %w", err)
	}
	return sig, nil
}

// newJTI returns a per-assertion unique identifier. Built from
// time-now-nanoseconds plus an atomic counter so two calls within the
// same nanosecond still produce distinct ids.
func newJTI(s *ClaimsSigner) (string, error) {
	counter := atomic.AddUint64(&s.jtiCounter, 1)
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], uint64(time.Now().UnixNano()))
	binary.BigEndian.PutUint64(b[8:], counter)
	return strings.TrimRight(base64.RawURLEncoding.EncodeToString(b[:]), "="), nil
}
