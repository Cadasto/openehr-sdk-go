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

// Error implements error. A nil receiver answers with the zero
// OAuth2Error's text rather than dereferencing (REQ-025 nil-receiver
// axis): a failed errors.As / errors.AsType leaves a typed nil that a
// caller boxes into a non-nil error interface and prints.
func (e *OAuth2Error) Error() string {
	if e == nil {
		return (&OAuth2Error{}).Error()
	}
	// Every producer parses a Code out of the RFC 6749 §5.2 envelope; only a
	// caller-built zero value leaves it empty, and that must not render as a
	// dangling "oauth2: ".
	code := e.Code
	if code == "" {
		code = "unspecified"
	}
	switch {
	case e.Description != "":
		return fmt.Sprintf("oauth2: %s: %s", code, e.Description)
	default:
		return "oauth2: " + code
	}
}

// ExchangeError wraps a token-exchange or refresh failure with the
// parsed OAuth2 error (if any), the HTTP status, and the underlying
// transport error. Detection uses errors.Is against the sentinels above;
// extraction uses errors.AsType[*auth.ExchangeError](err) (or errors.As).
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

// Error implements error. A nil receiver answers with the zero
// ExchangeError's text rather than dereferencing (REQ-025 nil-receiver
// axis): a failed errors.As / errors.AsType leaves a typed nil behind.
func (e *ExchangeError) Error() string {
	if e == nil {
		return (&ExchangeError{}).Error()
	}
	// Every producer sets Sentinel; a caller-built zero value does not, and
	// calling Error on that nil interface would panic just as the nil
	// receiver did. The fallback text deliberately matches no sentinel: this
	// value classifies as none of them, so it must not read as one.
	lead := "auth: unspecified exchange failure"
	if e.Sentinel != nil {
		lead = e.Sentinel.Error()
	}
	parts := []byte(lead)
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
// and errors.AsType[*auth.OAuth2Error](err) both work. A nil receiver
// unwraps to nothing (REQ-025 nil-receiver axis).
func (e *ExchangeError) Unwrap() []error {
	if e == nil {
		return nil
	}
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
// response whose OAuth2 envelope is invalid_grant, invalid_client, or
// invalid_token. Transient failures (5xx, network, context, unparsed) return
// false so callers retain the refresh token and may retry (REQ-063). Reach this
// method via errors.AsType[*auth.ExchangeError](err) rather than a direct
// type assertion; it is
// nil-receiver safe.
func (e *ExchangeError) Terminal() bool {
	if e == nil || e.StatusCode < 400 || e.StatusCode >= 500 || e.OAuth2 == nil {
		return false
	}
	switch e.OAuth2.Code {
	case "invalid_grant", "invalid_client":
		return true
	case "invalid_token":
		// Not an RFC 6749 §5.2 token-endpoint code (it belongs to RFC 6750
		// resource-server responses), but some authorization servers return it
		// on a rejected refresh_token grant. Treat it as terminal so the stored
		// refresh token is cleared rather than re-POSTed in a loop (F-L).
		return true
	}
	return false
}
