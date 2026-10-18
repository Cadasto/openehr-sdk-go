package jwtbearer_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/jwtbearer"
)

// TestREQ021_SourceWithoutHTTPClient pins the error New and FromConfig
// return when the caller supplies no HTTP client: auth.ErrInvalidConfig,
// whose text names the missing client and carries no internal requirement
// id.
func TestREQ021_SourceWithoutHTTPClient(t *testing.T) { // REQ-021
	const want = "auth: invalid configuration: HTTPClient is required"
	tests := []struct {
		name  string
		build func() (*jwtbearer.Source, error)
	}{
		{name: "New", build: func() (*jwtbearer.Source, error) {
			return jwtbearer.New("https://idp.example/token", jwtbearer.StaticAssertion("a"))
		}},
		{name: "FromConfig", build: func() (*jwtbearer.Source, error) {
			return jwtbearer.FromConfig(jwtbearer.Config{TokenURL: "https://idp.example/token", Assertion: jwtbearer.StaticAssertion("a")})
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src, err := tc.build()
			if src != nil || !errors.Is(err, auth.ErrInvalidConfig) {
				t.Fatalf("%s without an HTTP client = %v, %v; want nil and auth.ErrInvalidConfig", tc.name, src, err)
			}
			if got := err.Error(); got != want || strings.Contains(got, "REQ-") {
				t.Errorf("%s without an HTTP client: error text = %q, want %q", tc.name, got, want)
			}
		})
	}
}
