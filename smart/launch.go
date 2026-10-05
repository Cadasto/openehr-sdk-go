package smart

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
)

// ValidateConfig controls ID-token validation when building a
// [LaunchContext].
type ValidateConfig struct {
	JWKS            *authsmart.JWKS
	Issuer          string
	ClientID        string
	Nonce           string
	PrincipalClaims PrincipalClaimNames
	Now             time.Time
	// AllowedIDTokenAlgs constrains the accepted id_token signature
	// algorithms. Set it to the authorization server's
	// id_token_signing_alg_values_supported from discovery. When empty the
	// SDK default set (RS256/RS384/ES256/ES384) applies.
	AllowedIDTokenAlgs []string
	// TrustedAudiences names the audiences the ID token's aud claim may list
	// besides ClientID. When empty, a token whose aud lists any other
	// audience is rejected.
	TrustedAudiences []string
}

// ValidateOption mutates [ValidateConfig].
type ValidateOption func(*ValidateConfig)

// WithJWKS sets the JWKS used to validate id_token signatures.
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
// authorization server's advertised id_token_signing_alg_values_supported.
// When unset or empty the SDK default set applies
// (RS256/RS384/ES256/ES384).
func WithIDTokenSigningAlgs(algs []string) ValidateOption {
	return func(c *ValidateConfig) { c.AllowedIDTokenAlgs = algs }
}

// WithTrustedAudiences names the audiences the ID token's aud claim may list
// besides the client ID. Without it, a token whose aud lists any other
// audience is rejected.
func WithTrustedAudiences(aud ...string) ValidateOption {
	trusted := slices.Clone(aud)
	return func(c *ValidateConfig) { c.TrustedAudiences = trusted }
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
// a typed [LaunchContext].
//
// When tr.IDTokenClaims is set, the claims are taken as verified and are
// not checked again. auth/smart sets them only after it has verified the
// ID token, at the code exchange or on a refresh, so pass on unchanged the
// value that [authsmart.Source.ExchangeAuthorizationCode],
// [authsmart.Source.CompleteAuthorization] or
// [authsmart.Source.LastTokenResponse] returned. Claims put
// there by anything else are trusted all the same, so never fill
// IDTokenClaims yourself.
//
// When tr carries an ID token but no verified claims, as a value from
// [authsmart.ParseTokenResponse] does, the token is verified here with the
// options. [WithJWKS], [WithIssuer] and [WithClientID] are then required;
// without one the call fails with auth.ErrInvalidConfig.
//
// User is the verified ID token's fhirUser claim, else its sub, and is empty
// without a verified ID token. A fhirUser member in the token-endpoint body
// does not set it; it stays readable on tr.FHIRUser and on Raw.
func LaunchContextFromTokenResponse(ctx context.Context, tr authsmart.TokenResponse, opts ...ValidateOption) (*LaunchContext, error) {
	cfg := ValidateConfig{}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	lc := &LaunchContext{
		Patient:           tr.Patient,
		Encounter:         tr.Encounter,
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
	// Claims auth/smart verified at the exchange or refresh are used as they
	// are; only an ID token without them is verified here.
	claims := tr.IDTokenClaims
	if claims == nil && tr.IDToken != "" {
		// ValidateIDToken checks the trust anchors (JWKS, issuer, client ID)
		// before the token, so a missing one is a configuration error.
		var err error
		claims, err = authsmart.ValidateIDToken(ctx, tr.IDToken, cfg.JWKS, cfg.Issuer, cfg.ClientID, cfg.Nonce, cfg.Now, cfg.AllowedIDTokenAlgs,
			authsmart.WithTrustedAudiences(cfg.TrustedAudiences...))
		if err != nil {
			return nil, fmt.Errorf("smart: id_token: %w", err)
		}
	}
	if claims != nil {
		lc.IDToken = claims
		// The user is named by the verified ID token only, never by the
		// unsigned token-endpoint body.
		lc.User = cmp.Or(claims.FHIRUser, claims.Subject)
		lc.Issuer = cmp.Or(lc.Issuer, claims.Issuer)
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
