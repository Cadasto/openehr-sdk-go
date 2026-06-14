package smart

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
)

// clockSkew tolerates minor clock drift for exp/nbf/iat checks (OIDC).
const clockSkew = 30 * time.Second

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

// ValidateIDToken verifies a JWT ID token against jwks and returns
// parsed claims (REQ-062, REQ-064).
func ValidateIDToken(ctx context.Context, raw string, jwks *authsmart.JWKS, issuer, clientID, nonce string, now time.Time) (*IDTokenClaims, error) {
	if raw == "" {
		return nil, fmt.Errorf("%w: empty id_token", auth.ErrJWKSValidationFailed)
	}
	if err := requireIDTokenTrustAnchors(jwks, issuer, clientID); err != nil {
		return nil, err
	}
	headerB64, payloadB64, sigB64, err := splitJWT(raw)
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
	if hdr.Alg != "RS256" {
		return nil, fmt.Errorf("%w: unsupported alg %q", auth.ErrJWKSValidationFailed, hdr.Alg)
	}
	jwk, err := jwks.Key(ctx, hdr.Kid)
	if err != nil {
		return nil, err
	}
	pub, err := rsaPublicKeyFromJWK(jwk)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", auth.ErrJWKSValidationFailed, err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return nil, fmt.Errorf("%w: signature decode: %w", auth.ErrJWKSValidationFailed, err)
	}
	signingInput := headerB64 + "." + payloadB64
	sum := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		return nil, fmt.Errorf("%w: signature invalid", auth.ErrJWKSValidationFailed)
	}
	var claims map[string]any
	if err := decodeJWTPart(payloadB64, &claims); err != nil {
		return nil, err
	}
	return claimsFromMap(claims, issuer, clientID, nonce, now)
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

func rsaPublicKeyFromJWK(jwk json.RawMessage) (*rsa.PublicKey, error) {
	var meta struct {
		Kty string `json:"kty"`
		N   string `json:"n"`
		E   string `json:"e"`
	}
	if err := json.Unmarshal(jwk, &meta); err != nil {
		return nil, err
	}
	if meta.Kty != "RSA" || meta.N == "" || meta.E == "" {
		return nil, errors.New("JWK is not RSA")
	}
	nBytes, err := base64.RawURLEncoding.DecodeString(meta.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(meta.E)
	if err != nil {
		return nil, err
	}
	n := new(big.Int).SetBytes(nBytes)
	e := int(new(big.Int).SetBytes(eBytes).Int64())
	if e == 0 {
		return nil, errors.New("invalid RSA exponent")
	}
	return &rsa.PublicKey{N: n, E: e}, nil
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
