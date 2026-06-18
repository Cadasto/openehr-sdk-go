package auth

import "context"

// Reauther forces a fresh credential after a wire 401, even when the cached
// token is not yet past its proactive-refresh threshold (REQ-063). Implemented
// by token sources that can re-drive a refresh/exchange (e.g. *smart.Source).
type Reauther interface {
	Reauth(ctx context.Context) error
}

// ReautherFunc is a function adapter that implements [Reauther]. It lets a
// closure — for example a discovery-catalog-refresh function (REQ-071 bullet 3)
// — satisfy the Reauther interface without requiring a concrete type.
//
// Example:
//
//	transport.WithReauthOn401(auth.ReautherFunc(func(ctx context.Context) error {
//	    return resolver.Refresh(ctx, issuer)
//	}))
type ReautherFunc func(ctx context.Context) error

// Reauth implements [Reauther] by calling f.
func (f ReautherFunc) Reauth(ctx context.Context) error { return f(ctx) }
