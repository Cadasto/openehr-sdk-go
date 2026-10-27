package auth

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"
)

// TokenTypeBearer is the OAuth2 access-token scheme (default when Type is empty).
const TokenTypeBearer = "Bearer"

// TokenTypeBasic is the HTTP Basic scheme; Value must be the base64-encoded
// user-pass payload per RFC 7617.
const TokenTypeBasic = "Basic"

// Token is the credential delivered to the wire. Token is opaque to
// transport/, which emits Authorization: <Type> <Value>.
//
// Printed with fmt or logged with log/slog, a Token shows its Value as
// "[redacted]" and its other fields as they are: see [Token.String] and
// [Token.LogValue]. Its JSON encoding is unchanged and carries the Value, so
// an application can store a token and read it back.
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
//
// Printed with fmt, the source shows t as [Token.String] does. A value that
// holds the source in an unexported field, as a transport client does,
// prints the source's address, so neither shows t's Value.
func StaticTokenSource(t Token) TokenSource {
	return &staticTokenSource{t: t}
}

type staticTokenSource struct {
	t Token
}

func (s *staticTokenSource) Token(ctx context.Context) (Token, error) {
	if err := ctx.Err(); err != nil {
		return Token{}, err
	}
	return s.t, nil
}

// String describes s by the text of [Token.String] for its token. fmt reads
// the unexported field t without calling the token's methods, so without
// String and Format a printed source would show the token's Value. fmt
// calls no method of a value it reaches through an unexported field either,
// so the sources are pointers: there fmt prints an address, not the fields.
func (s *staticTokenSource) String() string {
	return "auth.StaticTokenSource(" + s.t.String() + ")"
}

// GoString returns the text of String.
func (s *staticTokenSource) GoString() string { return s.String() }

// Format writes the text of String for every fmt verb it is called for.
func (s *staticTokenSource) Format(f fmt.State, _ rune) {
	if f == nil {
		return
	}
	_, _ = io.WriteString(f, s.String())
}

// AnonymousTokenSource returns a TokenSource that yields a zero-value
// Token. transport/ treats a zero Token as "do not emit an Authorization
// header", which makes it the standard way to send an unauthenticated
// request to an endpoint that accepts both authenticated and anonymous
// traffic (capabilities, health). The default transport TokenSource is
// AnonymousTokenSource.
func AnonymousTokenSource() TokenSource {
	return &staticTokenSource{}
}

// IsZero reports whether t is the zero Token (no value, no type).
// transport/ uses this to decide whether to suppress the Authorization
// header on an outgoing request.
func (t Token) IsZero() bool {
	return t.Value == "" && t.Type == ""
}

// redacted is the text that stands for a credential when a value is printed
// or logged.
const redacted = "[redacted]"

// redact returns redacted for a credential s that is set and "" for an empty
// one.
func redact(s string) string {
	if s == "" {
		return ""
	}
	return redacted
}

// String describes t with its Value replaced by "[redacted]", or by "" when
// it is empty. Type, ExpiresAt (in RFC 3339 form), Scope and Issuer are shown
// as they are.
func (t Token) String() string {
	return fmt.Sprintf("auth.Token{Value: %q, Type: %q, ExpiresAt: %s, Scope: %q, Issuer: %q}",
		redact(t.Value), t.Type, t.ExpiresAt.Format(time.RFC3339Nano), t.Scope, t.Issuer)
}

// GoString returns the text of [Token.String], so that printing t in Go
// syntax shows no credential either.
func (t Token) GoString() string { return t.String() }

// Format writes the text of [Token.String] for every fmt verb it is called
// for, whatever the flags and width, so no such verb prints the Value. fmt
// answers %T and %p without calling Format: %T prints the type name, %p of a
// *Token prints an address, and %p of a Token value is a fmt error whose text
// shows the fields, Value included, so never print a Token with %p.
func (t Token) Format(f fmt.State, _ rune) {
	if f == nil {
		return
	}
	_, _ = io.WriteString(f, t.String())
}

// LogValue lets log/slog log t as a group of its fields, keyed by field name,
// with the Value replaced as [Token.String] replaces it.
func (t Token) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("Value", redact(t.Value)),
		slog.String("Type", t.Type),
		slog.Time("ExpiresAt", t.ExpiresAt),
		slog.String("Scope", t.Scope),
		slog.String("Issuer", t.Issuer),
	)
}
