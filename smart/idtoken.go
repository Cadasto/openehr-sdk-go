package smart

import (
	"context"
	"crypto"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	gojose "github.com/go-jose/go-jose/v4"

	"github.com/cadasto/openehr-sdk-go/auth"
	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
)

// clockSkew tolerates minor clock drift for exp/nbf/iat checks (OIDC).
const clockSkew = 30 * time.Second

// defaultIDTokenAlgs is the id_token signature allowlist used when the
// authorization server does not advertise id_token_signing_alg_values_supported.
// It mirrors the SMART asymmetric baseline (RS384/ES384) plus the widely
// deployed RS256/ES256 (REQ-062, REQ-064).
var defaultIDTokenAlgs = []string{"RS256", "RS384", "ES256", "ES384"}

// IDTokenClaims holds parsed OpenID ID-token claims (REQ-064).
type IDTokenClaims struct {
	Subject   string
	Audience  []string
	Issuer    string
	IssuedAt  time.Time
	ExpiresAt time.Time
	Nonce     string
	FHIRUser  string
	Extra     map[string]any
}

// ValidateIDToken verifies a JWT ID token against jwks and returns parsed
// claims (REQ-062, REQ-064).
//
// Signature verification is delegated to go-oidc/v3 (which uses go-jose),
// supporting RS256, RS384, ES256, and ES384. allowedAlgs constrains the
// accepted signature algorithms: when non-empty (e.g. the authorization
// server's advertised id_token_signing_alg_values_supported) it is
// intersected with the supported set; when empty the full supported set
// is used. The unsecured "none" algorithm is always rejected. The
// signature is always verified before any claim is trusted; claim
// semantics (iss/aud/exp/nbf/iat with the SDK's 30s skew, plus nonce) are
// then applied by claimsFromMap.
func ValidateIDToken(ctx context.Context, raw string, jwks *authsmart.JWKS, issuer, clientID, nonce string, now time.Time, allowedAlgs []string) (*IDTokenClaims, error) {
	if raw == "" {
		return nil, fmt.Errorf("%w: empty id_token", auth.ErrJWKSValidationFailed)
	}
	if err := requireIDTokenTrustAnchors(jwks, issuer, clientID); err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now()
	}

	// Read the header to locate the signing key by kid. The signature
	// itself is verified by go-oidc below — this only selects the key.
	headerB64, _, _, err := splitJWT(raw)
	if err != nil {
		return nil, err
	}
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := decodeJWTPart(headerB64, &hdr); err != nil {
		return nil, err
	}
	if strings.EqualFold(hdr.Alg, "none") {
		return nil, fmt.Errorf("%w: unsecured id_token (alg none) rejected", auth.ErrJWKSValidationFailed)
	}

	jwkRaw, err := jwks.Key(ctx, hdr.Kid)
	if err != nil {
		return nil, err
	}
	pub, err := publicKeyFromJWK(jwkRaw)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", auth.ErrJWKSValidationFailed, err)
	}

	keySet := &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{pub}}
	verifier := oidc.NewVerifier(issuer, keySet, &oidc.Config{
		ClientID:             clientID,
		SupportedSigningAlgs: resolveIDTokenAlgs(allowedAlgs),
		Now:                  func() time.Time { return now },
	})
	idt, err := verifier.Verify(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", auth.ErrJWKSValidationFailed, err)
	}

	// Re-extract the raw claim map and re-apply the SDK's stricter claim
	// semantics (skew + nonce). go-jose/go-oidc unmarshal via encoding/json,
	// so numeric times arrive as float64 — matching claimsFromMap's parsing.
	var claims map[string]any
	if err := idt.Claims(&claims); err != nil {
		return nil, fmt.Errorf("%w: claims: %w", auth.ErrJWKSValidationFailed, err)
	}
	return claimsFromMap(claims, issuer, clientID, nonce, now)
}

// resolveIDTokenAlgs computes the effective signature allowlist. The full
// supported set is RS256/RS384/ES256/ES384; "none" is never permitted. When
// allowedAlgs is non-empty it is intersected with the supported set so a
// caller passing the discovery list cannot widen the SDK's support, and an
// empty intersection falls back to the default set rather than the go-oidc
// RS256-only default.
func resolveIDTokenAlgs(allowedAlgs []string) []string {
	if len(allowedAlgs) == 0 {
		return defaultIDTokenAlgs
	}
	out := make([]string, 0, len(allowedAlgs))
	for _, a := range allowedAlgs {
		if strings.EqualFold(a, "none") {
			continue
		}
		if slices.Contains(defaultIDTokenAlgs, a) {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return defaultIDTokenAlgs
	}
	return out
}

// publicKeyFromJWK parses a single JWK document into its crypto.PublicKey
// (RSA or ECDSA) via go-jose, so go-oidc can verify the signature without any
// hand-rolled JWK→key conversion.
func publicKeyFromJWK(jwkRaw json.RawMessage) (crypto.PublicKey, error) {
	var k gojose.JSONWebKey
	if err := k.UnmarshalJSON(jwkRaw); err != nil {
		return nil, fmt.Errorf("parse JWK: %w", err)
	}
	if k.Key == nil {
		return nil, errors.New("JWK has no key material")
	}
	return k.Key, nil
}

func splitJWT(raw string) (header, payload, sig string, err error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("%w: expected 3 JWT segments", auth.ErrJWKSValidationFailed)
	}
	return parts[0], parts[1], parts[2], nil
}

func decodeJWTPart(b64 string, dest any) error {
	b, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return fmt.Errorf("%w: segment decode: %w", auth.ErrJWKSValidationFailed, err)
	}
	if err := json.Unmarshal(b, dest); err != nil {
		return fmt.Errorf("%w: segment json: %w", auth.ErrJWKSValidationFailed, err)
	}
	return nil
}

func claimsFromMap(claims map[string]any, issuer, clientID, nonce string, now time.Time) (*IDTokenClaims, error) {
	if now.IsZero() {
		now = time.Now()
	}
	iss, _ := claimString(claims, "iss")
	if iss != issuer {
		return nil, fmt.Errorf("%w: iss mismatch", auth.ErrJWKSValidationFailed)
	}
	aud := audienceStrings(claims["aud"])
	if !audContains(aud, clientID) {
		return nil, fmt.Errorf("%w: aud mismatch", auth.ErrJWKSValidationFailed)
	}
	exp, err := claimNumericTime(claims, "exp")
	if err != nil {
		return nil, err
	}
	if !exp.After(now.Add(-clockSkew)) {
		return nil, fmt.Errorf("%w: token expired", auth.ErrJWKSValidationFailed)
	}
	if nbf, ok, err := optionalClaimNumericTime(claims, "nbf"); err != nil {
		return nil, err
	} else if ok && nbf.After(now.Add(clockSkew)) {
		return nil, fmt.Errorf("%w: nbf in future", auth.ErrJWKSValidationFailed)
	}
	if iat, ok, err := optionalClaimNumericTime(claims, "iat"); err != nil {
		return nil, err
	} else if ok && iat.After(now.Add(clockSkew)) {
		return nil, fmt.Errorf("%w: iat in future", auth.ErrJWKSValidationFailed)
	}
	if nonce != "" {
		n, _ := claimString(claims, "nonce")
		if n != nonce {
			return nil, fmt.Errorf("%w: nonce mismatch", auth.ErrJWKSValidationFailed)
		}
	}
	iat, _, _ := optionalClaimNumericTime(claims, "iat")
	sub, _ := claimString(claims, "sub")
	fhirUser, _ := claimString(claims, "fhirUser")
	extra := make(map[string]any, len(claims))
	for k, v := range claims {
		switch k {
		case "iss", "sub", "aud", "exp", "iat", "nonce", "fhirUser":
			continue
		default:
			extra[k] = v
		}
	}
	return &IDTokenClaims{
		Subject:   sub,
		Audience:  aud,
		Issuer:    iss,
		IssuedAt:  iat,
		ExpiresAt: exp,
		Nonce:     nonce,
		FHIRUser:  fhirUser,
		Extra:     extra,
	}, nil
}

func audienceStrings(v any) []string {
	switch a := v.(type) {
	case string:
		if a == "" {
			return nil
		}
		return []string{a}
	case []any:
		out := make([]string, 0, len(a))
		for _, item := range a {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func audContains(aud []string, want string) bool {
	return slices.Contains(aud, want)
}

func claimNumericTime(claims map[string]any, key string) (time.Time, error) {
	t, ok, err := optionalClaimNumericTime(claims, key)
	if err != nil {
		return time.Time{}, err
	}
	if !ok {
		return time.Time{}, fmt.Errorf("%w: missing %s", auth.ErrJWKSValidationFailed, key)
	}
	return t, nil
}

func optionalClaimNumericTime(claims map[string]any, key string) (time.Time, bool, error) {
	v, ok := claims[key]
	if !ok {
		return time.Time{}, false, nil
	}
	switch n := v.(type) {
	case float64:
		return time.Unix(int64(n), 0), true, nil
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return time.Time{}, false, fmt.Errorf("%w: invalid %s", auth.ErrJWKSValidationFailed, key)
		}
		return time.Unix(i, 0), true, nil
	default:
		return time.Time{}, false, fmt.Errorf("%w: invalid %s type", auth.ErrJWKSValidationFailed, key)
	}
}
