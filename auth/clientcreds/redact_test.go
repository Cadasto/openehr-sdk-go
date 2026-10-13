package clientcreds_test

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/clientcreds"
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
			_, err := clientcreds.FromConfig(clientcreds.Config{
				HTTPClient:   http.DefaultClient,
				TokenURL:     tc.tokenURL,
				ClientID:     "client-id",
				ClientSecret: "client-secret",
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

// TestREQ068_TokenRequestErrorMasksPassword checks that Token reports a
// TokenURL the constructor accepts but cannot be requested with
// auth.ErrTokenExchangeFailed and url.Parse's *url.Error, and that the error
// names the host but not the password in the URL's userinfo.
func TestREQ068_TokenRequestErrorMasksPassword(t *testing.T) { // REQ-068
	t.Parallel()
	const secret = "s3cret"
	// url.Parse accepts the zone's é, but not the %C3%A9 that the request
	// URL, built from the parsed one, escapes it as.
	tokenURL := "https://svc:" + secret + "@[fe80::1%25é]/token"
	src, err := clientcreds.New("client-id", "client-secret", tokenURL, clientcreds.WithHTTPClient(http.DefaultClient))
	if err != nil {
		t.Fatalf("New(TokenURL %q) error = %v, want nil", tokenURL, err)
	}
	_, err = src.Token(t.Context())
	if !errors.Is(err, auth.ErrTokenExchangeFailed) {
		t.Fatalf("Token() error = %v, want auth.ErrTokenExchangeFailed", err)
	}
	if ue, ok := errors.AsType[*url.Error](err); !ok || ue == nil {
		t.Errorf("Token() error = %v, want url.Parse's *url.Error in its chain", err)
	}
	msg := err.Error()
	if strings.Contains(msg, secret) {
		t.Errorf("Token() error = %q, want no password", msg)
	}
	if !strings.Contains(msg, "fe80::1") {
		t.Errorf("Token() error = %q, want it to name the host %q", msg, "fe80::1")
	}
}
