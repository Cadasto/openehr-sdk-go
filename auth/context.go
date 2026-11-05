package auth

import "context"

type tokenSourceCtxKey struct{}

// WithTokenSource returns a derived context carrying ts as a per-request
// override. The transport consults TokenSourceFromContext on every
// outgoing request and prefers the per-request TokenSource over the
// client default when present.
//
// Use case: an MCP server holds one transport.Client and forwards each
// incoming caller's token through ctx, so the client itself does not own
// the user-level credentials.
//
// A nil ts returns ctx unchanged, so the client default applies. A ts that
// is not nil itself but holds a nil pointer, func, map, slice or chan, such
// as a nil *smart.Source, is attached as given: the transport then fails
// the request with an error matching transport.ErrInvalidConfig before
// sending it, and never falls back to the client default, which would send
// the client's own credential in place of the caller's.
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
