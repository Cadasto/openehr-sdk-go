package smart

import (
	"context"
	"fmt"
	"strings"
	"time"

	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
)

// ValidateConfig controls ID-token validation when building a
// [LaunchContext] (REQ-064).
type ValidateConfig struct {
	JWKS            *authsmart.JWKS
	Issuer          string
	ClientID        string
	Nonce           string
	PrincipalClaims PrincipalClaimNames
	Now             time.Time
}

// ValidateOption mutates [ValidateConfig].
type ValidateOption func(*ValidateConfig)

// WithJWKS sets the JWKS used to validate id_token signatures (REQ-062).
func WithJWKS(jwks *authsmart.JWKS) ValidateOption {
	return func(c *ValidateConfig) { c.JWKS = jwks }
}

// WithIssuer sets the expected iss claim.
func WithIssuer(iss string) ValidateOption {
	return func(c *ValidateConfig) { c.Issuer = iss }
}

// WithClientID sets the expected aud claim (OAuth client_id).
func WithClientID(clientID string) ValidateOption {
	return func(c *ValidateConfig) { c.ClientID = clientID }
}

// WithExpectedNonce sets the nonce claim required on the ID token.
func WithExpectedNonce(nonce string) ValidateOption {
	return func(c *ValidateConfig) { c.Nonce = nonce }
}

// WithPrincipalClaimNames overrides principal_uid / principal_type keys.
func WithPrincipalClaimNames(names PrincipalClaimNames) ValidateOption {
	return func(c *ValidateConfig) { c.PrincipalClaims = names }
}

// WithValidationTime overrides the clock used for exp validation (tests).
func WithValidationTime(t time.Time) ValidateOption {
	return func(c *ValidateConfig) { c.Now = t }
}

// LaunchContextFromTokenResponse maps a SMART token-endpoint payload into
// a typed [LaunchContext] (REQ-064, REQ-067).
func LaunchContextFromTokenResponse(ctx context.Context, tr authsmart.TokenResponse, opts ...ValidateOption) (*LaunchContext, error) {
	cfg := ValidateConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	lc := &LaunchContext{
		Patient:   tr.Patient,
		Encounter: tr.Encounter,
		User:      tr.FHIRUser,
		Issuer:    cfg.Issuer,
		Raw:       tr.Raw,
	}
	if tr.Scope != "" {
		lc.Scopes = strings.Fields(tr.Scope)
	}
	if tr.IDToken != "" {
		if err := requireIDTokenTrustAnchors(cfg.JWKS, cfg.Issuer, cfg.ClientID); err != nil {
			return nil, fmt.Errorf("smart: id_token: %w", err)
		}
		claims, err := ValidateIDToken(ctx, tr.IDToken, cfg.JWKS, cfg.Issuer, cfg.ClientID, cfg.Nonce, cfg.Now)
		if err != nil {
			return nil, fmt.Errorf("smart: id_token: %w", err)
		}
		lc.IDToken = claims
		if lc.User == "" {
			lc.User = claims.FHIRUser
		}
		if lc.User == "" {
			lc.User = claims.Subject
		}
		if lc.Issuer == "" {
			lc.Issuer = claims.Issuer
		}
		allClaims := map[string]any{
			"iss": claims.Issuer,
			"sub": claims.Subject,
			"aud": claims.Audience,
			"exp": claims.ExpiresAt.Unix(),
			"iat": claims.IssuedAt.Unix(),
		}
		for k, v := range claims.Extra {
			allClaims[k] = v
		}
		lc.Principal = principalFromClaims(allClaims, cfg.PrincipalClaims)
	}
	return lc, nil
}
