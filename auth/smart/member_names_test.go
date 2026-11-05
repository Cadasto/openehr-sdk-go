package smart_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth/smart"
)

// TestREQ063_TokenResponseReadsMemberNamesExactly pins that a token response
// is read with its member names as RFC 6749 §5.1, OpenID Connect and SMART
// App Launch spell them. Each extra member differs from a real one only in
// letter case and follows it, where a reader that folds case would take it;
// Raw still carries it under its own name.
func TestREQ063_TokenResponseReadsMemberNamesExactly(t *testing.T) { // REQ-062, REQ-063
	tests := []struct {
		name  string
		body  string
		field func(smart.TokenResponse) string
		want  string
		extra string // the extra member's name, which Raw carries
	}{
		{
			name:  "an ACCESS_TOKEN member after access_token",
			body:  `{"access_token":"good","token_type":"Bearer","ACCESS_TOKEN":"evil"}`,
			field: func(tr smart.TokenResponse) string { return tr.AccessToken },
			want:  "good",
			extra: "ACCESS_TOKEN",
		},
		{
			name:  "an ID_TOKEN member after id_token",
			body:  `{"access_token":"at","id_token":"good","ID_TOKEN":"evil"}`,
			field: func(tr smart.TokenResponse) string { return tr.IDToken },
			want:  "good",
			extra: "ID_TOKEN",
		},
		{
			name:  "an Expires_In member after expires_in",
			body:  `{"access_token":"at","expires_in":3600,"Expires_In":0}`,
			field: func(tr smart.TokenResponse) string { return strconv.FormatInt(tr.ExpiresIn, 10) },
			want:  "3600",
			extra: "Expires_In",
		},
		{
			name:  "a PATIENT member after patient",
			body:  `{"access_token":"at","patient":"p1","PATIENT":"p2"}`,
			field: func(tr smart.TokenResponse) string { return tr.Patient },
			want:  "p1",
			extra: "PATIENT",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr, err := smart.ParseTokenResponse([]byte(tc.body))
			if err != nil {
				t.Fatalf("ParseTokenResponse(%s) error = %v, want nil", tc.body, err)
			}
			if got := tc.field(tr); got != tc.want {
				t.Errorf("ParseTokenResponse(%s) read %q, want %q", tc.body, got, tc.want)
			}
			if _, ok := tr.Raw[tc.extra]; !ok {
				t.Errorf("ParseTokenResponse(%s).Raw has no %s member, want it kept under its own name", tc.body, tc.extra)
			}
		})
	}
}

// TestREQ063_TokenResponseWithARepeatedMemberIsRefused pins that a token
// response that repeats a member name, at its top level or inside a member,
// is refused as a body that does not decode, and that the error does not
// quote the token. Each repeat carries the same value, so only the
// repetition refuses the response.
func TestREQ063_TokenResponseWithARepeatedMemberIsRefused(t *testing.T) { // REQ-063
	const secret = "tok-secret-value"
	tests := []struct {
		name string
		body string
	}{
		{name: "access_token repeated", body: `{"access_token":"` + secret + `","access_token":"` + secret + `"}`},
		{name: "a member repeated inside fhirContext", body: `{"access_token":"` + secret + `","fhirContext":[{"reference":"Patient/1","reference":"Patient/1"}]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr, err := smart.ParseTokenResponse([]byte(tc.body))
			if err == nil {
				t.Fatalf("ParseTokenResponse(%s) = (AccessToken %q, nil), want an error", tc.body, tr.AccessToken)
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("ParseTokenResponse(%s) error = %q, quotes the access token", tc.body, err)
			}
		})
	}
}
