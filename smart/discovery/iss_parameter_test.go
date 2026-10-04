package discovery_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// TestResolveSurfacesIssParameterSupported pins REQ-070 and REQ-061: the
// resolver reads the RFC 9207 §3 member
// authorization_response_iss_parameter_supported onto
// AuthEndpoints.AuthorizationResponseIssParameterSupported, and an absent
// member leaves it false.
func TestResolveSurfacesIssParameterSupported(t *testing.T) { // REQ-070 REQ-061
	tests := []struct {
		name   string
		member any // nil leaves the member out of the document
		want   bool
	}{
		{name: "advertised true", member: true, want: true},
		{name: "advertised false", member: false, want: false},
		{name: "absent", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := map[string]any{
				"services": map[string]any{
					discovery.ServiceIDOpenEHRRest: map[string]any{"baseUrl": "https://api.example.com/openehr/v1"},
				},
			}
			if tc.member != nil {
				doc["authorization_response_iss_parameter_supported"] = tc.member
			}
			body := mustJSON(doc)
			p := startPlatform(t, false, serve(func(string) string { return body }), notFound)
			cat, err := p.resolver(t).Resolve(t.Context(), p.baseURL())
			if err != nil {
				t.Fatalf("Resolve(%s) error = %v, want success", body, err)
			}
			if got := cat.Auth.AuthorizationResponseIssParameterSupported; got != tc.want {
				t.Errorf("Resolve(%s): AuthorizationResponseIssParameterSupported = %v, want %v", body, got, tc.want)
			}
		})
	}
}
