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
