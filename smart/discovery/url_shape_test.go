package discovery_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// documentWith is a SMART configuration that advertises the openEHR REST
// service at serviceBaseURL and declares the given top-level members.
func documentWith(serviceBaseURL string, members map[string]string) string {
	doc := map[string]any{
		"services": map[string]any{
			discovery.ServiceIDOpenEHRRest: map[string]any{"baseUrl": serviceBaseURL},
		},
	}
	for k, v := range members {
		doc[k] = v
	}
	return mustJSON(doc)
}

// TestResolveRefusesPortWithoutHost pins REQ-072 and REQ-073: a URL whose
// authority has a port but no host name, such as https://:8443/x, has no
// host. It is ReasonMalformedURL wherever the resolver reads a URL: the base
// URL passed to Resolve, the declared issuer (with the OpenID check on and
// off), an auth endpoint and a service baseUrl. Nothing is fetched from it.
func TestResolveRefusesPortWithoutHost(t *testing.T) { // REQ-072, REQ-073
	const (
		hostless   = "https://:8443"
		apiBaseURL = "https://api.example.com/openehr/v1"
	)
	tests := []struct {
		name      string
		opts      []discovery.Option
		baseURL   func(p *stubPlatform) string // the URL passed to Resolve
		doc       string
		wantPaths []string
	}{
		{
			name:      "issuer, OpenID check on",
			baseURL:   (*stubPlatform).baseURL,
			doc:       smartDocument(hostless+idpPath, ""),
			wantPaths: []string{smartDocPath},
		},
		{
			name:      "issuer, OpenID check off",
			opts:      []discovery.Option{discovery.WithoutOpenIDConfigurationCheck()},
			baseURL:   (*stubPlatform).baseURL,
			doc:       smartDocument(hostless+idpPath, ""),
			wantPaths: []string{smartDocPath},
		},
		{
			name:      "auth endpoint",
			baseURL:   (*stubPlatform).baseURL,
			doc:       documentWith(apiBaseURL, map[string]string{"token_endpoint": hostless + "/token"}),
			wantPaths: []string{smartDocPath},
		},
		{
			name:      "service baseUrl",
			baseURL:   (*stubPlatform).baseURL,
			doc:       documentWith(hostless+"/openehr/v1", nil),
			wantPaths: []string{smartDocPath},
		},
		{
			name:      "base URL passed to Resolve",
			baseURL:   func(*stubPlatform) string { return hostless + platformPath },
			doc:       documentWith(apiBaseURL, nil),
			wantPaths: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := startPlatform(t, false, serve(func(string) string { return tc.doc }),
				serve(func(o string) string { return openIDDocument(o+idpPath, "") }))
			baseURL := tc.baseURL(p)
			_, err := p.resolver(t, tc.opts...).Resolve(t.Context(), baseURL)

			derr, ok := errors.AsType[*discovery.DiscoveryError](err)
			if !ok || derr.Reason != discovery.ReasonMalformedURL {
				t.Fatalf("Resolve(%q) error = %v, want a DiscoveryError with Reason %q", baseURL, err, discovery.ReasonMalformedURL)
			}
			if derr.Issuer != baseURL {
				t.Errorf("DiscoveryError.Issuer = %q, want the base URL %q", derr.Issuer, baseURL)
			}
			if paths, _ := p.requests(); !slices.Equal(paths, tc.wantPaths) {
				t.Errorf("requested paths %q, want %q", paths, tc.wantPaths)
			}
		})
	}
}

// TestResolveAcceptsOnlyHTTPSchemes pins REQ-073: an auth endpoint or a
// declared issuer may use https, or http under WithAllowInsecure; any other
// scheme is ReasonMalformedURL, with or without WithAllowInsecure, and http
// without it is ReasonInsecureURL.
func TestResolveAcceptsOnlyHTTPSchemes(t *testing.T) { // REQ-073
	const apiBaseURL = "https://api.example.com/openehr/v1"
	allowInsecure := []discovery.Option{discovery.WithAllowInsecure()}
	tests := []struct {
		name       string
		opts       []discovery.Option
		members    map[string]string
		wantReason discovery.DiscoveryErrorReason // empty means success
	}{
		{
			name:    "https auth endpoint",
			members: map[string]string{"token_endpoint": "https://auth.example.com/token"},
		},
		{
			name:    "upper-case HTTPS auth endpoint",
			members: map[string]string{"token_endpoint": "HTTPS://auth.example.com/token"},
		},
		{
			name:       "http auth endpoint",
			members:    map[string]string{"token_endpoint": "http://auth.example.com/token"},
			wantReason: discovery.ReasonInsecureURL,
		},
		{
			name:    "http auth endpoint with WithAllowInsecure",
			opts:    allowInsecure,
			members: map[string]string{"token_endpoint": "http://auth.example.com/token"},
		},
		{
			name:       "ftp auth endpoint",
			members:    map[string]string{"token_endpoint": "ftp://auth.example.com/token"},
			wantReason: discovery.ReasonMalformedURL,
		},
		{
			name:       "ftp auth endpoint with WithAllowInsecure",
			opts:       allowInsecure,
			members:    map[string]string{"token_endpoint": "ftp://auth.example.com/token"},
			wantReason: discovery.ReasonMalformedURL,
		},
		{
			name:       "ftp optional endpoint with WithAllowInsecure",
			opts:       allowInsecure,
			members:    map[string]string{"token_endpoint": "https://auth.example.com/token", "revocation_endpoint": "ftp://auth.example.com/revoke"},
			wantReason: discovery.ReasonMalformedURL,
		},
		{
			name:       "ftp issuer",
			opts:       []discovery.Option{discovery.WithoutOpenIDConfigurationCheck()},
			members:    map[string]string{"issuer": "ftp://idp.example.com"},
			wantReason: discovery.ReasonMalformedURL,
		},
		{
			name:       "ftp issuer with WithAllowInsecure",
			opts:       []discovery.Option{discovery.WithAllowInsecure(), discovery.WithoutOpenIDConfigurationCheck()},
			members:    map[string]string{"issuer": "ftp://idp.example.com"},
			wantReason: discovery.ReasonMalformedURL,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := documentWith(apiBaseURL, tc.members)
			p := startPlatform(t, false, serve(func(string) string { return body }), notFound)
			_, err := p.resolver(t, tc.opts...).Resolve(t.Context(), p.baseURL())

			if tc.wantReason == "" {
				if err != nil {
					t.Fatalf("Resolve(%s) error = %v, want success", body, err)
				}
				return
			}
			derr, ok := errors.AsType[*discovery.DiscoveryError](err)
			if !ok || derr.Reason != tc.wantReason {
				t.Fatalf("Resolve(%s) error = %v, want a DiscoveryError with Reason %q", body, err, tc.wantReason)
			}
		})
	}
}
