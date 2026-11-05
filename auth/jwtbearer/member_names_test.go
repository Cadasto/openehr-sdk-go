package jwtbearer_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/jwtbearer"
)

// tokenEndpointServing starts a token endpoint that answers every request
// with 200 and body, and returns a source that uses it.
func tokenEndpointServing(t *testing.T, body string) *jwtbearer.Source {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return newSource(t, srv.URL, srv.Client())
}

// TestREQ063_TokenResponseReadsMemberNamesExactly pins that the token
// response is read with its member names as RFC 6749 §5.1 spells them. Each
// extra member differs from a real one only in letter case and follows it,
// where a reader that folds case would take it.
func TestREQ063_TokenResponseReadsMemberNamesExactly(t *testing.T) { // REQ-063
	tests := []struct {
		name  string
		body  string
		field func(auth.Token) string
		want  string
	}{
		{
			name:  "an ACCESS_TOKEN member after access_token",
			body:  `{"access_token":"good","token_type":"Bearer","ACCESS_TOKEN":"evil"}`,
			field: func(tok auth.Token) string { return tok.Value },
			want:  "good",
		},
		{
			name:  "a TOKEN_TYPE member after token_type",
			body:  `{"access_token":"at","token_type":"Bearer","TOKEN_TYPE":"DPoP"}`,
			field: func(tok auth.Token) string { return tok.Type },
			want:  "Bearer",
		},
		{
			name:  "a SCOPE member after scope",
			body:  `{"access_token":"at","scope":"system/*.read","SCOPE":"system/*.write"}`,
			field: func(tok auth.Token) string { return tok.Scope },
			want:  "system/*.read",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tok, err := tokenEndpointServing(t, tc.body).Token(t.Context())
			if err != nil {
				t.Fatalf("Token() with body %s: error = %v, want nil", tc.body, err)
			}
			if got := tc.field(tok); got != tc.want {
				t.Errorf("Token() with body %s read %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}

// TestREQ063_TokenResponseWithARepeatedMemberIsRefused pins that a token
// response that repeats a member name is refused as a body that does not
// decode, an exchange failure whose error does not quote the token. The
// repeat carries the same value, so only the repetition refuses the
// response.
func TestREQ063_TokenResponseWithARepeatedMemberIsRefused(t *testing.T) { // REQ-063
	const secret = "tok-secret-value"
	body := `{"access_token":"` + secret + `","token_type":"Bearer","access_token":"` + secret + `"}`

	tok, err := tokenEndpointServing(t, body).Token(t.Context())
	if !errors.Is(err, auth.ErrTokenExchangeFailed) || tok.Value != "" {
		t.Fatalf("Token() with body %s = (%q, %v), want no token and an error matching auth.ErrTokenExchangeFailed", body, tok.Value, err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("Token() with body %s: error = %q, quotes the access token", body, err)
	}
}
