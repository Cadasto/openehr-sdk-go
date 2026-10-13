package jwtbearer_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/jwtbearer"
)

// TestREQ068_InvalidTokenURLErrorMasksPassword checks that FromConfig still
// refuses a TokenURL that is not a valid absolute URL with
// auth.ErrInvalidConfig, and that the error names the URL's host but not
// the password in its userinfo: neither when url.Parse refuses the URL nor
// when it parses without a scheme.
func TestREQ068_InvalidTokenURLErrorMasksPassword(t *testing.T) { // REQ-068
	t.Parallel()
	const secret = "s3cret"
	tests := []struct {
		name     string
		tokenURL string
		host     string // text the error must still carry
	}{
		{name: "bad escape in the host", tokenURL: "https://svc:" + secret + "@x%zz.example/token", host: "x%zz.example"},
		{name: "unclosed IPv6 host", tokenURL: "https://svc:" + secret + "@[::1", host: "[::1"},
		{name: "relative", tokenURL: "//svc:" + secret + "@x.example/token", host: "x.example"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := jwtbearer.FromConfig(jwtbearer.Config{
				HTTPClient: http.DefaultClient,
				TokenURL:   tc.tokenURL,
				Assertion:  jwtbearer.StaticAssertion("header.claims.signature"),
			})
			if !errors.Is(err, auth.ErrInvalidConfig) {
				t.Fatalf("FromConfig(TokenURL %q) error = %v, want auth.ErrInvalidConfig", tc.tokenURL, err)
			}
			msg := err.Error()
			if strings.Contains(msg, secret) {
				t.Errorf("FromConfig(TokenURL %q) error = %q, want no password", tc.tokenURL, msg)
			}
			if !strings.Contains(msg, tc.host) {
				t.Errorf("FromConfig(TokenURL %q) error = %q, want it to name the host %q", tc.tokenURL, msg, tc.host)
			}
		})
	}
}
