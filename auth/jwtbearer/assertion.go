package jwtbearer

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"maps"
	"sync/atomic"
	"time"

	gojose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/cryptosigner"

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
//
// Supported algorithms (SMART client-confidential-asymmetric baseline):
//   - RS384 (default) — RSA PKCS1v15 with SHA-384; mandated by HL7 SMART
//   - ES384 — ECDSA P-384 with SHA-384; mandated by HL7 SMART
//   - RS256 — RSA PKCS1v15 with SHA-256; accepted for back-compat
//   - ES256 — ECDSA P-256 with SHA-256; common in practice
//
// Signing is delegated to go-jose/v4, which handles correct JOSE encoding
// (including ECDSA r‖s padding) internally. (REQ-068)
type ClaimsSigner struct {
	Template ClaimsTemplate
	// Signer is the private-key signer. Required.
	Signer crypto.Signer
	// Algorithm is the JOSE "alg" name. Default: RS384 (SMART baseline).
	// Supported: RS384, ES384, RS256, ES256.
	Algorithm string
	// KeyID is the "kid" JWS header.
	KeyID string

	jtiCounter atomic.Uint64
}

// NewClaimsSigner constructs a ClaimsSigner. Returns ErrInvalidConfig
// when required fields are missing, the algorithm is unsupported, or the
// signer's key type does not match the algorithm family.
//
// Key requirements per algorithm:
//   - RS256, RS384: *rsa.PrivateKey
//   - ES256: *ecdsa.PrivateKey on P-256
//   - ES384: *ecdsa.PrivateKey on P-384
func NewClaimsSigner(template ClaimsTemplate, signer crypto.Signer, opts ...SignerOption) (*ClaimsSigner, error) {
	s := &ClaimsSigner{Template: template, Signer: signer, Algorithm: "RS384"}
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
	if _, err := toJoseAlg(s.Algorithm); err != nil {
		return nil, err
	}
	if err := validateKeyAlg(s.Signer, s.Algorithm); err != nil {
		return nil, err
	}
	return s, nil
}

// SignerOption configures a ClaimsSigner.
type SignerOption func(*ClaimsSigner)

// WithKeyID sets the JOSE "kid" header.
func WithKeyID(kid string) SignerOption {
	return func(s *ClaimsSigner) { s.KeyID = kid }
}

// WithAlgorithm overrides the JOSE "alg" name. Supported values:
// RS384 (default, SMART baseline), ES384, RS256 (back-compat), ES256.
// The key type must match the algorithm family; mismatches are rejected
// at construction with ErrInvalidConfig.
func WithAlgorithm(alg string) SignerOption {
	return func(s *ClaimsSigner) { s.Algorithm = alg }
}

// Assertion produces a freshly signed JWT bearer assertion. Each call
// allocates a new "iat"/"exp"/"jti" so two assertions from the same
// signer are never identical (RFC 7523 § 3 requires jti to be unique
// within the assertion's lifetime).
//
// Signing is performed by go-jose/v4, which handles all JOSE encoding
// including ECDSA r‖s byte padding. (REQ-068)
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

	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal JWT claims: %w", err)
	}

	joseAlg, err := toJoseAlg(s.Algorithm)
	if err != nil {
		return "", err // ErrInvalidConfig-wrapped; consistent whether or not NewClaimsSigner was used
	}
	var key any
	switch s.Signer.(type) {
	case *rsa.PrivateKey, *ecdsa.PrivateKey:
		key = s.Signer // concrete keys: go-jose handles natively
	default:
		key = cryptosigner.Opaque(s.Signer) // opaque/KMS signers (RSA + ECDSA)
	}
	signingKey := gojose.SigningKey{Algorithm: joseAlg, Key: key}
	signerOpts := (&gojose.SignerOptions{}).WithType("JWT")
	if s.KeyID != "" {
		signerOpts = signerOpts.WithHeader("kid", s.KeyID)
	}
	joseSigner, err := gojose.NewSigner(signingKey, signerOpts)
	if err != nil {
		return "", fmt.Errorf("create JWS signer: %w", err)
	}
	jws, err := joseSigner.Sign(claimsBytes)
	if err != nil {
		return "", fmt.Errorf("JWS sign: %w", err)
	}
	compact, err := jws.CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("JWS compact serialize: %w", err)
	}
	return compact, nil
}

// toJoseAlg maps a JOSE algorithm string to the go-jose constant.
// Returns ErrInvalidConfig for unsupported algorithms.
func toJoseAlg(alg string) (gojose.SignatureAlgorithm, error) {
	switch alg {
	case "RS256":
		return gojose.RS256, nil
	case "RS384":
		return gojose.RS384, nil
	case "ES256":
		return gojose.ES256, nil
	case "ES384":
		return gojose.ES384, nil
	default:
		return "", fmt.Errorf("%w: algorithm %q is not supported (supported: RS384, ES384, RS256, ES256)", auth.ErrInvalidConfig, alg)
	}
}

// validateKeyAlg checks that the signer's public key type and curve match
// the requested algorithm family. For opaque crypto.Signer implementations
// whose Public() does not return a concrete *rsa.PublicKey or *ecdsa.PublicKey
// (e.g. KMS handles wrapped in an adapter), validation is skipped here.
//
// Opaque crypto.Signer implementations (e.g. KMS/HSM adapters) are supported:
// at signing time a non-concrete signer is wrapped with
// github.com/go-jose/go-jose/v4/cryptosigner, which handles both RSA and
// ECDSA (including ES256/ES384). validateKeyAlg only inspects Public(); when
// Public() returns a concrete *rsa.PublicKey / *ecdsa.PublicKey the key/alg
// pairing is checked here, otherwise the pairing is enforced by go-jose at
// sign time. (REQ-068)
func validateKeyAlg(signer crypto.Signer, alg string) error {
	pub := signer.Public()
	switch alg {
	case "RS256", "RS384":
		if _, ok := pub.(*rsa.PublicKey); !ok {
			return fmt.Errorf("%w: %s requires an RSA signer, got %T", auth.ErrInvalidConfig, alg, pub)
		}
	case "ES256":
		ecPub, ok := pub.(*ecdsa.PublicKey)
		if !ok {
			return fmt.Errorf("%w: ES256 requires an ECDSA signer, got %T", auth.ErrInvalidConfig, pub)
		}
		if ecPub.Curve != elliptic.P256() {
			return fmt.Errorf("%w: ES256 requires a P-256 key, got %s", auth.ErrInvalidConfig, ecPub.Curve.Params().Name)
		}
	case "ES384":
		ecPub, ok := pub.(*ecdsa.PublicKey)
		if !ok {
			return fmt.Errorf("%w: ES384 requires an ECDSA signer, got %T", auth.ErrInvalidConfig, pub)
		}
		if ecPub.Curve != elliptic.P384() {
			return fmt.Errorf("%w: ES384 requires a P-384 key, got %s", auth.ErrInvalidConfig, ecPub.Curve.Params().Name)
		}
	}
	return nil
}

// newJTI returns a per-assertion unique identifier composed of three
// 8-byte segments: time-now-nanoseconds (rough ordering), an atomic
// counter (guaranteed uniqueness within a process), and crypto/rand
// bytes (unpredictability and cross-restart uniqueness). The 24 bytes
// encode to exactly 32 base64url characters with no padding.
func newJTI(s *ClaimsSigner) (string, error) {
	counter := s.jtiCounter.Add(1)
	var b [24]byte
	binary.BigEndian.PutUint64(b[:8], uint64(time.Now().UnixNano()))
	binary.BigEndian.PutUint64(b[8:16], counter)
	if _, err := rand.Read(b[16:]); err != nil {
		return "", fmt.Errorf("jwtbearer: jti entropy: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
