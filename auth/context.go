package auth

import "context"

type tokenSourceCtxKey struct{}

// WithTokenSource returns a derived context carrying ts as a per-request
// override. transport/ MUST consult TokenSourceFromContext on every
// outgoing request and prefer the per-request TokenSource over the
// client-default when present (docs/specifications/auth.md § Per-request TokenSource;
// PROBE-064).
//
// Use case: an MCP server holds one transport.Client and forwards each
// incoming caller's token through ctx — the client itself does not own
// the user-level credentials.
func WithTokenSource(ctx context.Context, ts TokenSource) context.Context {
	if ts == nil {
		return ctx
	}
	return context.WithValue(ctx, tokenSourceCtxKey{}, ts)
}

// TokenSourceFromContext returns the per-request TokenSource attached
// via WithTokenSource, or (nil, false) if none is attached. transport/
// is the primary caller.
func TokenSourceFromContext(ctx context.Context) (TokenSource, bool) {
	ts, ok := ctx.Value(tokenSourceCtxKey{}).(TokenSource)
	return ts, ok && ts != nil
}
