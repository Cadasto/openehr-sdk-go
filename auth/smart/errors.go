package smart

import "errors"

// ErrLaunchInvalidState indicates the state returned to the redirect
// URI did not match the state issued by BeginAuthorization — a possible
// CSRF attempt. The authorization code MUST NOT be exchanged. (auth.md REQ-061.)
var ErrLaunchInvalidState = errors.New("SMART launch: state mismatch")
