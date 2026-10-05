package smart

import (
	"context"
	"time"

	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
)

// IDTokenClaims holds parsed OpenID ID-token claims. It is the same type as
// [authsmart.IDTokenClaims].
type IDTokenClaims = authsmart.IDTokenClaims

// ValidateIDToken verifies a JWT ID token against jwks and returns parsed
// claims. It calls [authsmart.ValidateIDToken] with the same arguments and no
// options, so a token whose aud lists any audience besides clientID is
// refused. To accept further audiences, call [authsmart.ValidateIDToken]
// with [authsmart.WithTrustedAudiences], or [LaunchContextFromTokenResponse]
// with [WithTrustedAudiences]. [authsmart.ValidateIDToken] documents the
// algorithms, the claim checks and the errors.
func ValidateIDToken(ctx context.Context, raw string, jwks *authsmart.JWKS, issuer, clientID, nonce string, now time.Time, allowedAlgs []string) (*IDTokenClaims, error) {
	return authsmart.ValidateIDToken(ctx, raw, jwks, issuer, clientID, nonce, now, allowedAlgs)
}
