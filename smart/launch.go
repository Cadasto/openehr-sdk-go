package smart

import (
	"context"
	"fmt"
	"maps"
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
	// AllowedIDTokenAlgs constrains the accepted id_token signature
	// algorithms (REQ-062, REQ-064). Set it to the authorization server's
	// id_token_signing_alg_values_supported from discovery. When empty the
	// SDK default set (RS256/RS384/ES256/ES384) applies.
	AllowedIDTokenAlgs []string
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

// WithIDTokenSigningAlgs constrains the id_token signature algorithms to the
// authorization server's advertised id_token_signing_alg_values_supported
// (REQ-062, REQ-064). When unset or empty the SDK default set applies
// (RS256/RS384/ES256/ES384).
func WithIDTokenSigningAlgs(algs []string) ValidateOption {
	return func(c *ValidateConfig) { c.AllowedIDTokenAlgs = algs }
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
		Patient:           tr.Patient,
		Encounter:         tr.Encounter,
		User:              tr.FHIRUser,
		Issuer:            cfg.Issuer,
		EHRID:             tr.EHRID,
		EpisodeID:         tr.EpisodeID,
		Intent:            tr.Intent,
		SMARTStyleURL:     tr.SMARTStyleURL,
		NeedPatientBanner: tr.NeedPatientBanner,
		Tenant:            tr.Tenant,
		Raw:               maps.Clone(tr.Raw),
	}
	if tr.Scope != "" {
		lc.Scopes = strings.Fields(tr.Scope)
	}
	if tr.IDToken != "" {
		if err := requireIDTokenTrustAnchors(cfg.JWKS, cfg.Issuer, cfg.ClientID); err != nil {
			return nil, fmt.Errorf("smart: id_token: %w", err)
		}
		claims, err := ValidateIDToken(ctx, tr.IDToken, cfg.JWKS, cfg.Issuer, cfg.ClientID, cfg.Nonce, cfg.Now, cfg.AllowedIDTokenAlgs)
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
		lc.Principal = principalFromClaims(idTokenClaimMap(claims), cfg.PrincipalClaims)
	}
	if lc.Principal == nil && len(tr.Raw) > 0 {
		lc.Principal = principalFromClaims(tr.Raw, cfg.PrincipalClaims)
	}
	return lc, nil
}

func idTokenClaimMap(claims *IDTokenClaims) map[string]any {
	all := map[string]any{
		"iss": claims.Issuer,
		"sub": claims.Subject,
		"aud": claims.Audience,
		"exp": claims.ExpiresAt.Unix(),
	}
	if !claims.IssuedAt.IsZero() {
		all["iat"] = claims.IssuedAt.Unix()
	}
	if claims.FHIRUser != "" {
		all["fhirUser"] = claims.FHIRUser
	}
	if claims.Nonce != "" {
		all["nonce"] = claims.Nonce
	}
	maps.Copy(all, claims.Extra)
	return all
}
