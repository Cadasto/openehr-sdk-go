package auth

import "context"

type tokenSourceCtxKey struct{}

// WithTokenSource returns a derived context carrying ts as a per-request
// override. The transport consults TokenSourceFromContext on every
// outgoing request and prefers the per-request TokenSource over the
// client default when present.
//
// Use case: a server acting for many users, such as an MCP server, holds
// one shared transport.Client, and each request's context carries a source
// for a token issued for the openEHR API on behalf of the user that request
// serves, for example by an OAuth 2.0 token exchange. The client itself
// owns no user-level credential. An MCP server never passes on the token
// its MCP client sent it; the MCP specification forbids that.
//
// A nil ts returns ctx unchanged, so the client default applies. A ts that
// is not nil itself but holds a nil pointer, func, map, slice or chan, such
// as a nil *smart.Source, is attached as given: the transport then fails
// the request with an error matching transport.ErrInvalidConfig before
// sending it, and never falls back to the client default, which would send
// the client's own credential in place of the one meant for this request.
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
