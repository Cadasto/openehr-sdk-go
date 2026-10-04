package smart

import (
	"context"
	"fmt"
	"net/url"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// CompleteAuthorization finishes a launch from the redirect back to the app.
// callback is the query the redirect URI received, and req is the
// [AuthorizationRequest] the launch started with.
//
// It checks the redirect in this order and does not contact the token
// endpoint until every check passes:
//
//  1. The state must equal req.State, else the call fails with
//     [ErrLaunchInvalidState].
//  2. An iss parameter (RFC 9207), when present, must equal req.Issuer
//     exactly, else the call fails with [ErrLaunchIssuerMismatch]. When the
//     authorization server advertises
//     authorization_response_iss_parameter_supported, a redirect without iss
//     fails the same way.
//  3. A redirect carrying an error parameter (RFC 6749 §4.1.2.1) fails with
//     [ErrAuthorizationRejected]; errors.As extracts an [*auth.OAuth2Error]
//     holding its error, error_description and error_uri. A redirect with
//     neither an error nor a code fails with [ErrAuthorizationRejected] too.
//
// It then exchanges the code exactly as [Source.ExchangeAuthorizationCode]
// does, including the ID-token check. A req that did not come from
// [Source.BeginAuthorization] fails with [auth.ErrInvalidConfig].
func (s *Source) CompleteAuthorization(ctx context.Context, callback url.Values, req AuthorizationRequest) (auth.Token, TokenResponse, error) {
	if req.State == "" || req.PKCE.Verifier == "" {
		return auth.Token{}, TokenResponse{}, fmt.Errorf("%w: AuthorizationRequest from BeginAuthorization is required", auth.ErrInvalidConfig)
	}
	state := callback.Get("state")
	if state != req.State {
		return auth.Token{}, TokenResponse{}, ErrLaunchInvalidState
	}
	// RFC 9207 §2.4: the issuer named on the response must be the one the
	// request went to; a server that always names itself must have done so.
	if callback.Has("iss") {
		if callback.Get("iss") != req.Issuer {
			return auth.Token{}, TokenResponse{}, ErrLaunchIssuerMismatch
		}
	} else if s.cfg.Auth.AuthorizationResponseIssParameterSupported {
		return auth.Token{}, TokenResponse{}, fmt.Errorf("%w: the redirect carries no iss, which the authorization server always sends", ErrLaunchIssuerMismatch)
	}
	if callback.Has("error") {
		return auth.Token{}, TokenResponse{}, fmt.Errorf("%w: %w", ErrAuthorizationRejected, &auth.OAuth2Error{
			Code:        callback.Get("error"),
			Description: callback.Get("error_description"),
			URI:         callback.Get("error_uri"),
		})
	}
	code := callback.Get("code")
	if code == "" {
		return auth.Token{}, TokenResponse{}, fmt.Errorf("%w: the redirect carries neither a code nor an error", ErrAuthorizationRejected)
	}
	return s.ExchangeAuthorizationCode(ctx, code, state, req)
}
