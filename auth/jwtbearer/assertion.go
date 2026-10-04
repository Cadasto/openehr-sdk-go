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
// Each call must return an unexpired JWT (the SDK does not validate
// timing). The authorization server rejects a stale assertion, which
// surfaces as auth.ErrTokenExchangeFailed.
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
	// Issuer is the "iss" claim, typically the client_id of the
	// confidential client.
	Issuer string
	// Subject is the "sub" claim. For client-authentication assertions,
	// sub == iss. For on-behalf-of assertions, sub identifies the
	// principal.
	Subject string
	// Audience is the "aud" claim, typically the token endpoint URL.
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
//   - RS384 (default): RSA PKCS1v15 with SHA-384; mandated by HL7 FHIR SMART App Launch
//   - ES384: ECDSA P-384 with SHA-384; mandated by HL7 FHIR SMART App Launch
//   - RS256: RSA PKCS1v15 with SHA-256; accepted for back-compat
//   - ES256: ECDSA P-256 with SHA-256; common in practice
//
// Signing is delegated to go-jose/v4, which handles JOSE encoding
// (including ECDSA r‖s padding) internally.
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

// NewClaimsSigner constructs a ClaimsSigner. Returns ErrInvalidConfig, and
// never panics, when required fields are missing, the signer is nil or a
// nil *rsa.PrivateKey or *ecdsa.PrivateKey, the algorithm is unsupported,
// the signer's Public method panics or reports no usable public key (nil,
// or an RSA or ECDSA public key without its modulus or curve), or the
// signer's key type does not match the algorithm family. A Public method
// panics for a nil key of most other types held in a non-nil crypto.Signer,
// such as a nil ed25519.PrivateKey or a nil pointer to a signer type of the
// caller's own whose Public reads its key, so such a signer is refused
// instead of crashing the caller.
//
// A signer of the caller's own type whose Public reports a usable public key
// of the right type is accepted as it is: the SDK cannot see inside it, so
// whether it can sign is known only when it signs.
//
// Key requirements per algorithm:
//   - RS256, RS384: *rsa.PrivateKey
//   - ES256: *ecdsa.PrivateKey on P-256
//   - ES384: *ecdsa.PrivateKey on P-384
func NewClaimsSigner(template ClaimsTemplate, signer crypto.Signer, opts ...SignerOption) (*ClaimsSigner, error) {
	s := &ClaimsSigner{Template: template, Signer: signer, Algorithm: "RS384"}
	for _, o := range opts {
		if o != nil {
			o(s)
		}
	}
	if s.Signer == nil {
		return nil, fmt.Errorf("%w: signer is required", auth.ErrInvalidConfig)
	}
	// A nil key of a concrete type passes the check above, and its Public
	// method would panic, so refuse it before calling any method on it.
	if isNilKey(s.Signer) {
		return nil, fmt.Errorf("%w: signer is a nil %T", auth.ErrInvalidConfig, s.Signer)
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

// clientAssertionLifetime is how long a client assertion stays valid. The HL7
// SMART asymmetric client profile allows at most five minutes after issue.
const clientAssertionLifetime = 5 * time.Minute

// NewClientAssertion builds the client assertion of the HL7 SMART asymmetric
// client profile: the signed JWT that a SMART Backend Services client, or a
// confidential app holding a private key, sends as client_assertion to
// authenticate at the token endpoint. Each assertion it signs has iss and
// sub set to clientID, aud set to tokenURL, the JOSE headers typ JWT and
// kid, a unique jti, and an exp five minutes after its iat.
//
// It fails with [auth.ErrInvalidConfig], and never panics, when clientID,
// tokenURL, alg or kid is empty, signer is empty (nil, a nil RSA or ECDSA
// private key, or a signer whose Public method panics or reports no usable
// public key), alg is not supported, or the key does not fit alg, as
// [NewClaimsSigner] describes. A signer of the caller's own type that
// reports a usable public key is accepted as it is.
// [NewClaimsSigner] lists the key each algorithm needs.
func NewClientAssertion(clientID, tokenURL string, signer crypto.Signer, alg, kid string) (*ClaimsSigner, error) {
	if clientID == "" {
		return nil, fmt.Errorf("%w: a SMART client assertion needs a clientID", auth.ErrInvalidConfig)
	}
	if tokenURL == "" {
		return nil, fmt.Errorf("%w: a SMART client assertion needs a tokenURL", auth.ErrInvalidConfig)
	}
	if kid == "" {
		return nil, fmt.Errorf("%w: a SMART client assertion needs a kid", auth.ErrInvalidConfig)
	}
	// NewClaimsSigner refuses a nil signer, an empty or unsupported alg, and
	// a key that does not fit alg.
	return NewClaimsSigner(ClaimsTemplate{
		Issuer:   clientID,
		Subject:  clientID,
		Audience: tokenURL,
		Lifetime: clientAssertionLifetime,
	}, signer, WithAlgorithm(alg), WithKeyID(kid))
}

// isNilKey reports whether signer is a nil *rsa.PrivateKey or
// *ecdsa.PrivateKey: a nil key held in a non-nil crypto.Signer. It names the
// two key types the supported algorithms take; a nil value of any other
// type is caught when its Public method panics (see publicKey).
func isNilKey(signer crypto.Signer) bool {
	switch k := signer.(type) {
	case *rsa.PrivateKey:
		return k == nil
	case *ecdsa.PrivateKey:
		return k == nil
	}
	return false
}

// publicKey returns signer.Public(). A Public method that panics, as it
// does for a nil key of most types held in a non-nil crypto.Signer, is
// refused with auth.ErrInvalidConfig instead of crashing the caller.
func publicKey(signer crypto.Signer) (pub crypto.PublicKey, err error) {
	defer func() {
		if recover() != nil {
			pub = nil
			err = fmt.Errorf("%w: the signer's Public method panicked: a nil or unusable %T key", auth.ErrInvalidConfig, signer)
		}
	}()
	return signer.Public(), nil
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
// signer are never identical (RFC 7523 §3 requires jti to be unique
// within the assertion's lifetime).
//
// Signing is performed by go-jose/v4, which handles all JOSE encoding
// including ECDSA r‖s byte padding.
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

// validateKeyAlg checks that the signer's public key fits alg: RS256 and
// RS384 need an *rsa.PublicKey, ES256 an *ecdsa.PublicKey on P-256, and
// ES384 one on P-384. It inspects only Public(), so an opaque crypto.Signer
// (e.g. a KMS or HSM adapter) passes when its Public() returns a usable key
// of the type alg needs, and is refused when it returns any other type, no
// key, or an RSA or ECDSA key without its modulus or curve. At signing time
// a signer that is not a concrete *rsa.PrivateKey or *ecdsa.PrivateKey is
// wrapped with github.com/go-jose/go-jose/v4/cryptosigner, which handles
// both RSA and ECDSA (including ES256/ES384). An alg outside the four is not
// checked here; toJoseAlg refuses it. (REQ-068)
func validateKeyAlg(signer crypto.Signer, alg string) error {
	pub, err := publicKey(signer)
	if err != nil {
		return err
	}
	if err := usablePublicKey(pub); err != nil {
		return err
	}
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

// usablePublicKey refuses a public key the algorithm checks cannot read: no
// key at all, or an RSA or ECDSA key that is nil or lacks its modulus or
// curve. Any other key passes here and is judged against the algorithm.
func usablePublicKey(pub crypto.PublicKey) error {
	switch k := pub.(type) {
	case nil:
		return fmt.Errorf("%w: the signer's Public method reports no public key", auth.ErrInvalidConfig)
	case *rsa.PublicKey:
		if k == nil || k.N == nil {
			return fmt.Errorf("%w: the signer's Public method reports an RSA public key without its modulus", auth.ErrInvalidConfig)
		}
	case *ecdsa.PublicKey:
		if k == nil || k.Curve == nil {
			return fmt.Errorf("%w: the signer's Public method reports an ECDSA public key without its curve", auth.ErrInvalidConfig)
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
