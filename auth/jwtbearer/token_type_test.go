package jwtbearer_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth/jwtbearer"
)

// TestTokenNormalisesBearerTokenType verifies that a token_type of bearer in
// any letter case becomes the "Bearer" Authorization scheme on auth.Token, an
// absent or empty one defaults to it, and any other scheme passes through
// unchanged (REQ-060). It pins RFC 6749 §5.1: the token_type value is case
// insensitive.
func TestTokenNormalisesBearerTokenType(t *testing.T) { // REQ-060
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "lower-case bearer", body: `{"access_token":"at","token_type":"bearer"}`, want: "Bearer"},
		{name: "upper-case bearer", body: `{"access_token":"at","token_type":"BEARER"}`, want: "Bearer"},
		{name: "canonical Bearer", body: `{"access_token":"at","token_type":"Bearer"}`, want: "Bearer"},
		{name: "absent", body: `{"access_token":"at"}`, want: "Bearer"},
		{name: "empty", body: `{"access_token":"at","token_type":""}`, want: "Bearer"},
		{name: "other scheme unchanged", body: `{"access_token":"at","token_type":"DPoP"}`, want: "DPoP"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			src, err := jwtbearer.New(srv.URL, jwtbearer.StaticAssertion("the-jwt"), jwtbearer.WithHTTPClient(srv.Client()))
			if err != nil {
				t.Fatal(err)
			}
			tok, err := src.Token(t.Context())
			if err != nil {
				t.Fatalf("Token() with body %s: error = %v", tc.body, err)
			}
			if tok.Type != tc.want {
				t.Errorf("Token() with body %s: Type = %q, want %q", tc.body, tok.Type, tc.want)
			}
		})
	}
}
