// Package basic implements HTTP Basic authentication (RFC 7617) as an
// auth.TokenSource for openEHR REST deployments that accept a static
// username and password on each request.
package basic

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// TokenType is the Authorization scheme emitted on the wire.
const TokenType = "Basic"

// Source returns a fixed Basic credential on every Token() call. There is
// no token exchange or refresh; credentials are supplied at construction.
// Source is safe for concurrent use.
//
// Printing a Source with any fmt verb shows its credential as "[redacted]":
// see [Source.String].
type Source struct {
	token auth.Token
}

// New constructs a Source from username and password. Username must be
// non-empty. Password can be empty when the deployment allows it.
func New(username, password string) (*Source, error) {
	if username == "" {
		return nil, fmt.Errorf("%w: username is required", auth.ErrInvalidConfig)
	}
	return &Source{token: encodeToken(username, password)}, nil
}

// Token returns the Basic credential. It honours ctx cancellation.
func (s *Source) Token(ctx context.Context) (auth.Token, error) {
	if err := ctx.Err(); err != nil {
		return auth.Token{}, err
	}
	return s.token, nil
}

// String describes s by the text of [auth.Token.String] for its credential,
// which shows the Basic payload as "[redacted]". A nil s gives "<nil>".
func (s *Source) String() string {
	if s == nil {
		return "<nil>"
	}
	return "basic.Source{Token: " + s.token.String() + "}"
}

// GoString returns the text of [Source.String], so that printing s in Go
// syntax shows no credential either.
func (s *Source) GoString() string { return s.String() }

// Format writes the text of [Source.String] for every fmt verb, whatever its
// flags and width. Without it, fmt would print the unexported credential
// field of s without calling its methods, the Basic payload included.
func (s *Source) Format(f fmt.State, _ rune) {
	if f == nil {
		return
	}
	_, _ = io.WriteString(f, s.String())
}

// encodeToken builds the auth.Token transport/ forwards as
// Authorization: Basic <Value> per REQ-069.
func encodeToken(username, password string) auth.Token {
	payload := username + ":" + password
	return auth.Token{
		Value: base64.StdEncoding.EncodeToString([]byte(payload)),
		Type:  TokenType,
	}
}
