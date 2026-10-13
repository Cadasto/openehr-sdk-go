package smart_test

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// redactPassword is the password the URLs in these tests carry in their
// userinfo, after the user name svc.
const redactPassword = "s3cret"

// unrequestableEndpoint returns a token or revocation endpoint that
// http.NewRequestWithContext refuses: its port is not a number, so the text
// url.URL.String makes of it does not parse. It carries redactPassword.
func unrequestableEndpoint(path string) *url.URL {
	return &url.URL{
		Scheme: "https",
		User:   url.UserPassword("svc", redactPassword),
		Host:   "x.example:bad",
		Path:   path,
	}
}

// TestREQ060_EndpointRequestErrorMasksPassword checks the credential-bearing
// requests of REQ-060: the code exchange (REQ-061), the refresh (REQ-063)
// and the revocation (REQ-167). When the request to the endpoint cannot be
// built, the error is still an *auth.ExchangeError of the request's own
// class, and it names the endpoint's host but not the password in its
// userinfo.
func TestREQ060_EndpointRequestErrorMasksPassword(t *testing.T) { // REQ-060
	t.Parallel()
	for _, path := range []string{"/token", "/revoke"} {
		if _, err := url.Parse(unrequestableEndpoint(path).String()); err == nil {
			t.Fatalf("url.Parse(%q) error = nil, want an error: the case would not reach the refusal", unrequestableEndpoint(path))
		}
	}
	kinds := []struct {
		name     string
		sentinel error
		run      func(t *testing.T, src *smart.Source) error
	}{
		{"code exchange", auth.ErrTokenExchangeFailed, func(t *testing.T, src *smart.Source) error {
			t.Helper()
			req, err := src.BeginAuthorization("")
			if err != nil {
				t.Fatalf("BeginAuthorization: %v", err)
			}
			_, _, err = src.ExchangeAuthorizationCode(t.Context(), "code", req.State, req)
			return err
		}},
		{"refresh", auth.ErrRefreshFailed, func(t *testing.T, src *smart.Source) error {
			t.Helper()
			src.SetTokens(auth.Token{Value: "at-0"}, "rt-0")
			return src.Reauth(t.Context())
		}},
		{"revocation", auth.ErrRevocationFailed, func(t *testing.T, src *smart.Source) error {
			t.Helper()
			src.SetTokens(auth.Token{Value: "at-0"}, "rt-0")
			return src.Revoke(t.Context())
		}},
	}
	for _, kind := range kinds {
		t.Run(kind.name, func(t *testing.T) {
			t.Parallel()
			src, err := newSource("client-id", discovery.AuthEndpoints{
				AuthorizationEndpoint: discovery.MustParseURL("https://idp.example/authorize"),
				TokenEndpoint:         unrequestableEndpoint("/token"),
				RevocationEndpoint:    unrequestableEndpoint("/revoke"),
			},
				smart.WithHTTPClient(http.DefaultClient),
				smart.WithClientSecret("client-secret"),
				smart.WithRedirectURI("https://app.example/callback"),
			)
			if err != nil {
				t.Fatalf("newSource: %v", err)
			}

			err = kind.run(t, src)

			if ee, ok := errors.AsType[*auth.ExchangeError](err); !ok || ee == nil {
				t.Fatalf("%s error = %v, want an *auth.ExchangeError", kind.name, err)
			}
			if !errors.Is(err, kind.sentinel) {
				t.Errorf("%s error = %v, want it to match %v", kind.name, err, kind.sentinel)
			}
			msg := err.Error()
			if strings.Contains(msg, redactPassword) {
				t.Errorf("%s error = %q, want no password", kind.name, msg)
			}
			if !strings.Contains(msg, "x.example:bad") {
				t.Errorf("%s error = %q, want it to name the host %q", kind.name, msg, "x.example:bad")
			}
		})
	}
}

// TestREQ062_JWKSFetchErrorMasksPassword checks that a JWKS whose URI
// url.Parse refuses fails the lookup with an error that names the URI's
// host but not the password in its userinfo.
func TestREQ062_JWKSFetchErrorMasksPassword(t *testing.T) { // REQ-062
	t.Parallel()
	uri := "https://svc:" + redactPassword + "@[::1"
	jwks, err := smart.NewJWKS(http.DefaultClient, uri)
	if err != nil {
		t.Fatalf("NewJWKS(%q) error = %v", uri, err)
	}
	_, err = jwks.Key(t.Context(), "kid-1")
	if err == nil {
		t.Fatalf("Key with JWKS URI %q error = nil, want an error", uri)
	}
	msg := err.Error()
	if strings.Contains(msg, redactPassword) {
		t.Errorf("Key with JWKS URI %q error = %q, want no password", uri, msg)
	}
	if !strings.Contains(msg, "[::1") {
		t.Errorf("Key with JWKS URI %q error = %q, want it to name the host %q", uri, msg, "[::1")
	}
}
