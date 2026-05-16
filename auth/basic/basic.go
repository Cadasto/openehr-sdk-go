// Package basic implements HTTP Basic authentication (RFC 7617) as an
// auth.TokenSource for openEHR REST deployments that accept a static
// username and password on each request.
package basic

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// TokenType is the Authorization scheme emitted on the wire (REQ-069).
const TokenType = "Basic"

// Source returns a fixed Basic credential on every Token() call. There is
// no token exchange or refresh — credentials are supplied at construction.
// Source is safe for concurrent use (REQ-026).
type Source struct {
	token auth.Token
}

// New constructs a Source from username and password. Username MUST be
// non-empty. Password MAY be empty when the deployment allows it.
func New(username, password string) (*Source, error) {
	if username == "" {
		return nil, fmt.Errorf("%w: username is required", auth.ErrInvalidConfig)
	}
	return &Source{token: encodeToken(username, password)}, nil
}

// Token returns the Basic credential. It honours ctx cancellation (REQ-020).
func (s *Source) Token(ctx context.Context) (auth.Token, error) {
	if err := ctx.Err(); err != nil {
		return auth.Token{}, err
	}
	return s.token, nil
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
