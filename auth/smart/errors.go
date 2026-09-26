package smart

import "errors"

// ErrLaunchInvalidState indicates the state returned to the redirect
// URI did not match the state issued by BeginAuthorization, which may be
// a CSRF attempt. Do not exchange the authorization code.
var ErrLaunchInvalidState = errors.New("SMART launch: state mismatch")
