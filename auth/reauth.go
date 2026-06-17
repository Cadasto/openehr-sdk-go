package auth

import "context"

// Reauther forces a fresh credential after a wire 401, even when the cached
// token is not yet past its proactive-refresh threshold (REQ-063). Implemented
// by token sources that can re-drive a refresh/exchange (e.g. *smart.Source).
type Reauther interface {
	Reauth(ctx context.Context) error
}
