package smart

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/auth"
	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
)

// requireIDTokenTrustAnchors enforces OIDC trust binding when validating
// an id_token (REQ-064): JWKS, issuer, and client_id must all be set.
func requireIDTokenTrustAnchors(jwks *authsmart.JWKS, issuer, clientID string) error {
	if jwks == nil {
		return fmt.Errorf("%w: JWKS is required to validate id_token", auth.ErrInvalidConfig)
	}
	if issuer == "" || clientID == "" {
		return fmt.Errorf("%w: Issuer and ClientID are required to validate id_token", auth.ErrInvalidConfig)
	}
	return nil
}
