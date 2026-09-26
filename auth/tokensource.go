package auth

import (
	"context"
	"time"
)

// TokenTypeBearer is the OAuth2 access-token scheme (default when Type is empty).
const TokenTypeBearer = "Bearer"

// TokenTypeBasic is the HTTP Basic scheme; Value must be the base64-encoded
// user-pass payload per RFC 7617.
const TokenTypeBasic = "Basic"

// Token is the credential delivered to the wire. Token is opaque to
// transport/, which emits Authorization: <Type> <Value>.
type Token struct {
	// Value is the scheme-specific credential (bearer token or Basic payload).
	Value string
	// Type is the Authorization scheme ("Bearer", "Basic", …). Empty means Bearer.
	Type string
	// ExpiresAt is the absolute expiry instant. The zero value means
	// "no expiry / unknown". TokenSource implementations that cannot
	// observe an expiry must leave it zero instead of inventing a future
	// time.
	ExpiresAt time.Time
	// Scope carries the space-separated scope grant from the
	// authorization-server response, verbatim. The SDK does not enforce
	// scope as application policy; it passes the string through so
	// callers can audit it.
	Scope string
	// Issuer is the URL of the authorization server that minted the
	// token. Used for audit and disambiguation across multi-issuer
	// federation. Populated by providers that have access to the
	// discovery document; otherwise empty.
	Issuer string
}

// TokenSource produces tokens for outgoing authenticated requests.
//
// Implementations must:
//   - Refresh transparently when ExpiresAt is near or past.
//   - Coalesce concurrent refresh attempts.
//   - Honour ctx for cancellation and deadlines.
//   - Be safe for concurrent use by multiple goroutines.
//
// Outside its owning provider package, a Token comes only from a
// TokenSource; no other SDK package builds Token values directly. transport/ consumes a TokenSource through
// transport.WithTokenSource and per-request through
// auth.WithTokenSource(ctx, ts).
type TokenSource interface {
	Token(ctx context.Context) (Token, error)
}

// StaticTokenSource returns a TokenSource that always yields t.
//
// Useful for tests, for short-lived ad-hoc clients, and for consumers
// who manage token lifecycle externally. The returned TokenSource is
// stateless and safe for concurrent use.
//
// StaticTokenSource does not refresh. If t.ExpiresAt is in the past,
// callers see authentication failures on the wire instead of a typed
// refresh error.
func StaticTokenSource(t Token) TokenSource {
	return staticTokenSource{t: t}
}

type staticTokenSource struct {
	t Token
}

func (s staticTokenSource) Token(ctx context.Context) (Token, error) {
	if err := ctx.Err(); err != nil {
		return Token{}, err
	}
	return s.t, nil
}

// AnonymousTokenSource returns a TokenSource that yields a zero-value
// Token. transport/ treats a zero Token as "do not emit an Authorization
// header", which makes it the standard way to send an unauthenticated
// request to an endpoint that accepts both authenticated and anonymous
// traffic (capabilities, health). The default transport TokenSource is
// AnonymousTokenSource.
func AnonymousTokenSource() TokenSource {
	return staticTokenSource{}
}

// IsZero reports whether t is the zero Token (no value, no type).
// transport/ uses this to decide whether to suppress the Authorization
// header on an outgoing request.
func (t Token) IsZero() bool {
	return t.Value == "" && t.Type == ""
}
