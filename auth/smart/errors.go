package smart

import "errors"

// ErrLaunchInvalidState indicates the state returned to the redirect
// URI did not match the state issued by BeginAuthorization, which may be
// a CSRF attempt. Do not exchange the authorization code.
var ErrLaunchInvalidState = errors.New("SMART launch: state mismatch")

// ErrLaunchIssuerNotAllowed indicates an embedded launch named an issuer
// the app has not chosen to trust, or that no allowlist was given.
// [ParseEHRLaunch] returns it before any discovery request is made for
// that issuer.
var ErrLaunchIssuerNotAllowed = errors.New("SMART launch: issuer not allowed")

// ErrLaunchInvalidRequest indicates an embedded launch URL that lacks the
// iss or launch parameter, or whose iss is not an absolute URL with a
// host.
var ErrLaunchInvalidRequest = errors.New("SMART launch: invalid launch request")

// ErrLaunchIssuerMismatch indicates the redirect back to the app named an
// issuer (the RFC 9207 iss parameter) other than the one the launch was
// sent to, or carried none although the authorization server says it
// always sends one. The response may come from another authorization
// server (a mix-up attack), so the code is not exchanged.
var ErrLaunchIssuerMismatch = errors.New("SMART launch: authorization response issuer mismatch")

// ErrAuthorizationRejected indicates the redirect back to the app carried
// an error instead of an authorization code, for example because the user
// declined, or carried neither. When the authorization server sent an
// error, errors.As extracts it as an *auth.OAuth2Error.
var ErrAuthorizationRejected = errors.New("SMART launch: authorization rejected")
