package auth_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// TestREQ063_OAuth2ErrorReadsMemberNamesExactly pins that the RFC 6749 §5.2
// error envelope is read with its member names as the RFC spells them. Each
// extra member differs from a real one only in letter case and follows it,
// where a reader that folds case would take it; read as the error code, the
// first would turn a refused refresh token, which ends the session, into a
// failure worth retrying.
func TestREQ063_OAuth2ErrorReadsMemberNamesExactly(t *testing.T) { // REQ-063
	tests := []struct {
		name string
		body string
		want auth.OAuth2Error
	}{
		{
			name: "an ERROR member after error",
			body: `{"error":"invalid_grant","ERROR":"temporarily_unavailable"}`,
			want: auth.OAuth2Error{Code: "invalid_grant"},
		},
		{
			name: "an Error_Description member after error_description",
			body: `{"error":"invalid_grant","error_description":"refresh token expired","Error_Description":"other"}`,
			want: auth.OAuth2Error{Code: "invalid_grant", Description: "refresh token expired"},
		},
		{
			name: "an ERROR_URI member after error_uri",
			body: `{"error":"invalid_grant","error_uri":"https://idp.example.com/errors","ERROR_URI":"https://other.example.com"}`,
			want: auth.OAuth2Error{Code: "invalid_grant", URI: "https://idp.example.com/errors"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := auth.ParseOAuth2Error([]byte(tc.body))
			if got == nil || *got != tc.want {
				t.Errorf("ParseOAuth2Error(%s) = %+v, want %+v", tc.body, got, tc.want)
			}
		})
	}
}

// TestREQ063_OAuth2ErrorWithARepeatedMemberIsNotAnEnvelope pins that an
// error body that repeats a member name gives nil, as a body that does not
// decode does. The repeat carries the same value, so only the repetition
// refuses the body.
func TestREQ063_OAuth2ErrorWithARepeatedMemberIsNotAnEnvelope(t *testing.T) { // REQ-063
	body := `{"error":"invalid_grant","error":"invalid_grant"}`
	if got := auth.ParseOAuth2Error([]byte(body)); got != nil {
		t.Errorf("ParseOAuth2Error(%s) = %+v, want nil", body, got)
	}
}
