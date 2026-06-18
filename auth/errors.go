package auth

import (
	"errors"
	"fmt"
)

// Sentinel auth errors. Detect classes with errors.Is; the underlying
// wire error is preserved via errors.Unwrap on the wrapping Error type.
var (
	// ErrTokenExchangeFailed indicates the authorization server
	// rejected a token-exchange request (authorization_code,
	// client_credentials, jwt-bearer).
	ErrTokenExchangeFailed = errors.New("auth: token exchange failed")

	// ErrRefreshFailed indicates a refresh_token grant against the
	// token endpoint failed.
	ErrRefreshFailed = errors.New("auth: token refresh failed")

	// ErrReauthRequired indicates the cached token cannot be refreshed
	// without consumer intervention (refresh_token absent or rejected
	// terminally). Consumers MUST restart the launch flow.
	ErrReauthRequired = errors.New("auth: re-authentication required")

	// ErrInvalidConfig indicates a provider was constructed with
	// missing or contradictory required fields (e.g. no token endpoint,
	// no client id).
	ErrInvalidConfig = errors.New("auth: invalid configuration")

	// ErrJWKSValidationFailed indicates a JWT could not be validated
	// against the deployment's JWKS even after one refresh (REQ-062).
	ErrJWKSValidationFailed = errors.New("auth: JWKS validation failed")
)

// OAuth2Error is the parsed error response from an OAuth2 token endpoint.
// The error response shape is defined by RFC 6749 § 5.2.
type OAuth2Error struct {
	Code        string // "invalid_client", "invalid_grant", ...
	Description string // human-readable description
	URI         string // optional URI describing the error
}

// Error implements error.
func (e *OAuth2Error) Error() string {
	switch {
	case e.Description != "":
		return fmt.Sprintf("oauth2: %s: %s", e.Code, e.Description)
	default:
		return "oauth2: " + e.Code
	}
}

// ExchangeError wraps a token-exchange or refresh failure with the
// parsed OAuth2 error (if any), the HTTP status, and the underlying
// transport error. Detection uses errors.Is against the sentinels above;
// extraction uses errors.As(err, &ex *auth.ExchangeError).
type ExchangeError struct {
	// Sentinel is the categorical error class (one of the package
	// sentinels). errors.Is returns true against this value.
	Sentinel error
	// StatusCode is the HTTP status of the token-endpoint response, or
	// 0 when the failure was pre-flight (network, marshal, ctx).
	StatusCode int
	// OAuth2 is the parsed error envelope, if the response shape
	// matched. Nil when the response was not parseable.
	OAuth2 *OAuth2Error
	// Inner is the underlying transport / parse / context error, if any.
	Inner error
}

// Error implements error.
func (e *ExchangeError) Error() string {
	parts := []byte(e.Sentinel.Error())
	if e.StatusCode != 0 {
		parts = fmt.Appendf(parts, " status=%d", e.StatusCode)
	}
	if e.OAuth2 != nil {
		parts = fmt.Appendf(parts, ": %s", e.OAuth2.Error())
	} else if e.Inner != nil {
		parts = fmt.Appendf(parts, ": %v", e.Inner)
	}
	return string(parts)
}

// Unwrap walks the wrapped errors. errors.Is(err, auth.ErrTokenExchangeFailed)
// and errors.As(err, &oauth *auth.OAuth2Error) both work.
func (e *ExchangeError) Unwrap() []error {
	out := make([]error, 0, 3)
	if e.Sentinel != nil {
		out = append(out, e.Sentinel)
	}
	if e.OAuth2 != nil {
		out = append(out, e.OAuth2)
	}
	if e.Inner != nil {
		out = append(out, e.Inner)
	}
	return out
}

// Terminal reports whether the token-endpoint failure is permanent — a 4xx
// response whose OAuth2 envelope is invalid_grant or invalid_client. Transient
// failures (5xx, network, context, unparsed) return false so callers retain
// the refresh token and may retry (REQ-063). Reach this method via
// errors.As(err, &ex) rather than a direct type assertion; it is nil-receiver safe.
func (e *ExchangeError) Terminal() bool {
	if e == nil || e.StatusCode < 400 || e.StatusCode >= 500 || e.OAuth2 == nil {
		return false
	}
	switch e.OAuth2.Code {
	case "invalid_grant", "invalid_client":
		return true
	}
	return false
}
