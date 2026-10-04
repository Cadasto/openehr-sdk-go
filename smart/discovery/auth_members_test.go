package discovery_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// TestResolveConditionalAuthMembers pins REQ-072: when a SMART configuration
// declares any of authorization_endpoint, token_endpoint or jwks_uri, it
// needs token_endpoint; authorization_endpoint when capabilities lists
// launch-ehr or launch-standalone; and jwks_uri when it lists
// sso-openid-connect. A document with none of the three is an anonymous-only
// deployment and passes, and a backend-only document without
// authorization_endpoint passes. A missing member is
// ReasonAuthEndpointsMissing, naming the member and the capability that
// requires it.
func TestResolveConditionalAuthMembers(t *testing.T) { // REQ-072
	const (
		authz = "https://auth.example.com/authorize"
		token = "https://auth.example.com/token"
		jwks  = "https://auth.example.com/jwks"
	)
	tests := []struct {
		name         string
		members      map[string]string // authorization-server members the document declares
		capabilities []string
		wantMissing  string // the member the error names; empty means success
		wantCapName  string // the capability the error names, when one requires the member
	}{
		{
			name: "anonymous-only: no authorization-server members",
		},
		{
			name:         "anonymous-only with a launch capability",
			capabilities: []string{"launch-ehr", "sso-openid-connect"},
		},
		{
			name:    "backend-only: token_endpoint alone",
			members: map[string]string{"token_endpoint": token},
		},
		{
			name:         "backend-only with jwks_uri and no launch capability",
			members:      map[string]string{"token_endpoint": token, "jwks_uri": jwks},
			capabilities: []string{"client-confidential-asymmetric", "permission-v2"},
		},
		{
			name:         "launch capability with authorization_endpoint",
			members:      map[string]string{"authorization_endpoint": authz, "token_endpoint": token},
			capabilities: []string{"launch-ehr", "launch-standalone"},
		},
		{
			name:         "every member for every capability",
			members:      map[string]string{"authorization_endpoint": authz, "token_endpoint": token, "jwks_uri": jwks},
			capabilities: []string{"launch-ehr", "launch-standalone", "sso-openid-connect"},
		},
		{
			name:        "authorization_endpoint without token_endpoint",
			members:     map[string]string{"authorization_endpoint": authz},
			wantMissing: "token_endpoint",
		},
		{
			name:        "jwks_uri without token_endpoint",
			members:     map[string]string{"jwks_uri": jwks},
			wantMissing: "token_endpoint",
		},
		{
			name:         "launch-ehr without authorization_endpoint",
			members:      map[string]string{"token_endpoint": token},
			capabilities: []string{"launch-ehr"},
			wantMissing:  "authorization_endpoint",
			wantCapName:  "launch-ehr",
		},
		{
			name:         "launch-standalone without authorization_endpoint",
			members:      map[string]string{"token_endpoint": token, "jwks_uri": jwks},
			capabilities: []string{"client-public", "launch-standalone"},
			wantMissing:  "authorization_endpoint",
			wantCapName:  "launch-standalone",
		},
		{
			name:         "sso-openid-connect without jwks_uri",
			members:      map[string]string{"authorization_endpoint": authz, "token_endpoint": token},
			capabilities: []string{"launch-standalone", "sso-openid-connect"},
			wantMissing:  "jwks_uri",
			wantCapName:  "sso-openid-connect",
		},
		{
			name:         "sso-openid-connect without jwks_uri on a backend-only document",
			members:      map[string]string{"token_endpoint": token},
			capabilities: []string{"sso-openid-connect"},
			wantMissing:  "jwks_uri",
			wantCapName:  "sso-openid-connect",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := map[string]any{
				"services": map[string]any{
					discovery.ServiceIDOpenEHRRest: map[string]any{"baseUrl": "https://api.example.com/openehr/v1"},
				},
			}
			for k, v := range tc.members {
				doc[k] = v
			}
			if tc.capabilities != nil {
				doc["capabilities"] = tc.capabilities
			}
			body := mustJSON(doc)
			p := startPlatform(t, false, serve(func(string) string { return body }), notFound)
			_, err := p.resolver(t).Resolve(t.Context(), p.baseURL())

			if tc.wantMissing == "" {
				if err != nil {
					t.Fatalf("Resolve(%s) error = %v, want success", body, err)
				}
				return
			}
			derr, ok := errors.AsType[*discovery.DiscoveryError](err)
			if !ok || derr.Reason != discovery.ReasonAuthEndpointsMissing {
				t.Fatalf("Resolve(%s) error = %v, want a DiscoveryError with Reason %q", body, err, discovery.ReasonAuthEndpointsMissing)
			}
			if derr.Issuer != p.baseURL() {
				t.Errorf("DiscoveryError.Issuer = %q, want the base URL %q", derr.Issuer, p.baseURL())
			}
			inner := ""
			if derr.Inner != nil {
				inner = derr.Inner.Error()
			}
			for _, want := range []string{tc.wantMissing, tc.wantCapName} {
				if !strings.Contains(inner, want) {
					t.Errorf("DiscoveryError.Inner = %q, want it to name %q", inner, want)
				}
			}
		})
	}
}
