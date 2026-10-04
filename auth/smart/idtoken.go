package smart

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
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
)

// clockSkew tolerates minor clock drift for exp/nbf/iat checks (OIDC).
const clockSkew = 30 * time.Second

// defaultIDTokenAlgs is the id_token signature allowlist used when the
// authorization server does not advertise id_token_signing_alg_values_supported.
// It mirrors the SMART asymmetric baseline (RS384/ES384) plus the widely
// deployed RS256/ES256 (REQ-062, REQ-064).
var defaultIDTokenAlgs = []string{"RS256", "RS384", "ES256", "ES384"}

// IDTokenClaims holds parsed OpenID ID-token claims.
type IDTokenClaims struct {
	Subject   string
	Audience  []string
	Issuer    string
	IssuedAt  time.Time
	ExpiresAt time.Time
	// Nonce is the token's own nonce claim, empty when the token has none,
	// whether or not the caller expected a nonce.
	Nonce    string
	FHIRUser string
	// Extra holds the claims not named by the other fields.
	Extra map[string]any
}

// IDTokenOption adjusts the claim checks of [ValidateIDToken].
type IDTokenOption func(*idTokenConfig)

type idTokenConfig struct {
	trustedAudiences []string
}

// WithTrustedAudiences names the audiences an ID token's aud claim may list
// besides the client ID. A token whose aud lists any other audience is
// rejected, so with no trusted audiences only the client ID is accepted. A
// later WithTrustedAudiences replaces an earlier one.
func WithTrustedAudiences(aud ...string) IDTokenOption {
	trusted := slices.Clone(aud)
	return func(c *idTokenConfig) { c.trustedAudiences = trusted }
}

// ValidateIDToken verifies a JWT ID token against jwks and returns parsed
// claims.
//
// Signature verification is delegated to go-oidc/v3 (which uses go-jose),
// supporting RS256, RS384, ES256, and ES384. allowedAlgs constrains the
// accepted signature algorithms: when non-empty (e.g. the authorization
// server's advertised id_token_signing_alg_values_supported) it is
// intersected with the supported set; when empty the full supported set
// is used. The unsecured "none" algorithm is always rejected. The
// signature is always verified before any claim is trusted; the claim
// checks run after that. iss must equal issuer exactly. aud must contain
// clientID, and any other audience it lists must be one named by
// [WithTrustedAudiences]. An azp claim, when present, must equal clientID.
// exp is required, and exp, nbf and iat are checked with a 30-second
// allowance for clock skew. When nonce is not empty the nonce claim must
// equal it.
//
// A token rejected on its own content matches [auth.ErrJWKSValidationFailed].
// A missing jwks, issuer or clientID matches [auth.ErrInvalidConfig], and a
// failed JWKS fetch is returned as the fetch error, so an outage never reads
// as a bad token.
func ValidateIDToken(ctx context.Context, raw string, jwks *JWKS, issuer, clientID, nonce string, now time.Time, allowedAlgs []string, opts ...IDTokenOption) (*IDTokenClaims, error) {
	// The trust anchors are the caller's configuration, so they are checked
	// before the token: a configuration error must not read as a bad token,
	// even when the token is empty too (REQ-064).
	if err := requireIDTokenTrustAnchors(jwks, issuer, clientID); err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, fmt.Errorf("%w: empty id_token", auth.ErrJWKSValidationFailed)
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

	algs := resolveIDTokenAlgs(allowedAlgs)
	if len(algs) == 0 {
		// The caller supplied a non-empty advertised set (e.g. discovery's
		// id_token_signing_alg_values_supported) but none of its algorithms are
		// supported. Fail closed — honour the server's constraint rather than
		// silently falling back to the default RS/ES set (REQ-062). This runs
		// before the key lookup, so no JWKS fetch happens for an allowlist that
		// can never verify, and a JWKS outage cannot replace the sentinel with
		// its fetch error.
		return nil, fmt.Errorf("%w: no supported id_token signing algorithm in the advertised set %v (supported: %v)", auth.ErrJWKSValidationFailed, allowedAlgs, defaultIDTokenAlgs)
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
		SupportedSigningAlgs: algs,
		Now:                  func() time.Time { return now },
		// SkipExpiryCheck delegates expiry enforcement (with 30s clockSkew) to
		// claimsFromMap below, avoiding go-oidc's zero-tolerance expiry check
		// which would reject tokens in the [exp, exp+30s) skew window. Issuer,
		// audience, and signature checks remain active.
		SkipExpiryCheck: true,
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
	var cfg idTokenConfig
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	return claimsFromMap(claims, issuer, clientID, nonce, now, cfg.trustedAudiences)
}

// requireIDTokenTrustAnchors enforces OIDC trust binding when validating
// an id_token (REQ-064): JWKS, issuer, and client_id must all be set.
func requireIDTokenTrustAnchors(jwks *JWKS, issuer, clientID string) error {
	if jwks == nil {
		return fmt.Errorf("%w: JWKS is required to validate id_token", auth.ErrInvalidConfig)
	}
	if issuer == "" || clientID == "" {
		return fmt.Errorf("%w: Issuer and ClientID are required to validate id_token", auth.ErrInvalidConfig)
	}
	return nil
}

// resolveIDTokenAlgs returns the effective id_token signing-alg allowlist.
// With no caller-supplied list it returns the full default set
// (RS256/RS384/ES256/ES384). With a non-empty list it returns the
// case-normalised intersection with the default set ("none" always dropped); a
// non-empty input that intersects nothing returns an EMPTY slice, which
// ValidateIDToken treats as a fail-closed configuration error rather than
// silently widening back to the defaults.
func resolveIDTokenAlgs(allowedAlgs []string) []string {
	if len(allowedAlgs) == 0 {
		return defaultIDTokenAlgs
	}
	out := make([]string, 0, len(allowedAlgs))
	for _, a := range allowedAlgs {
		norm := strings.ToUpper(strings.TrimSpace(a))
		if norm == "NONE" {
			continue
		}
		if slices.Contains(defaultIDTokenAlgs, norm) {
			out = append(out, norm)
		}
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
	switch k.Key.(type) {
	case *rsa.PublicKey, *ecdsa.PublicKey, ed25519.PublicKey:
		// ok — asymmetric public key
	default:
		return nil, fmt.Errorf("%w: JWK key type %T is not a supported asymmetric public key", auth.ErrJWKSValidationFailed, k.Key)
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

func claimsFromMap(claims map[string]any, issuer, clientID, nonce string, now time.Time, trustedAudiences []string) (*IDTokenClaims, error) {
	if now.IsZero() {
		now = time.Now()
	}
	iss, _ := claimString(claims, "iss")
	if iss != issuer {
		return nil, fmt.Errorf("%w: iss mismatch", auth.ErrJWKSValidationFailed)
	}
	aud := audienceStrings(claims["aud"])
	if !slices.Contains(aud, clientID) {
		return nil, fmt.Errorf("%w: aud mismatch", auth.ErrJWKSValidationFailed)
	}
	// OIDC Core 1.0 §3.1.3.7 step 3: any audience besides the client must be
	// one the caller trusts (REQ-062).
	for _, a := range aud {
		if a != clientID && !slices.Contains(trustedAudiences, a) {
			return nil, fmt.Errorf("%w: aud lists an untrusted audience", auth.ErrJWKSValidationFailed)
		}
	}
	// An azp claim, when present, must name the client (REQ-062).
	if v, ok := claims["azp"]; ok {
		if azp, _ := v.(string); azp != clientID {
			return nil, fmt.Errorf("%w: azp is not the client", auth.ErrJWKSValidationFailed)
		}
	}
	exp, err := claimNumericTime(claims, "exp")
	if err != nil {
		return nil, err
	}
	if !exp.After(now.Add(-clockSkew)) {
		return nil, fmt.Errorf("%w: token expired", auth.ErrJWKSValidationFailed)
	}
	nbf, ok, err := optionalClaimNumericTime(claims, "nbf")
	if err != nil {
		return nil, err
	}
	if ok && nbf.After(now.Add(clockSkew)) {
		return nil, fmt.Errorf("%w: nbf in future", auth.ErrJWKSValidationFailed)
	}
	iat, ok, err := optionalClaimNumericTime(claims, "iat")
	if err != nil {
		return nil, err
	}
	if ok && iat.After(now.Add(clockSkew)) {
		return nil, fmt.Errorf("%w: iat in future", auth.ErrJWKSValidationFailed)
	}
	tokenNonce, _ := claimString(claims, "nonce")
	if nonce != "" && tokenNonce != nonce {
		return nil, fmt.Errorf("%w: nonce mismatch", auth.ErrJWKSValidationFailed)
	}
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
		Nonce:     tokenNonce,
		FHIRUser:  fhirUser,
		Extra:     extra,
	}, nil
}

func claimString(claims map[string]any, key string) (string, bool) {
	s, ok := claims[key].(string)
	return s, ok
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
