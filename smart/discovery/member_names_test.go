package discovery_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// appendMember returns doc, a JSON object such as mustJSON writes, with
// member written after its last member, so the new member follows every
// member doc already holds.
func appendMember(doc, member string) string {
	return strings.TrimSuffix(doc, "}") + "," + member + "}"
}

// TestREQ071_DocumentsReadMemberNamesExactly pins that the SMART
// configuration and the issuer's OpenID configuration are read with their
// member names as SMART App Launch, RFC 8414 and OpenID Connect Discovery
// spell them. Each extra member differs from issuer or jwks_uri only in
// letter case and follows the real one, where a reader that folds case would
// take it; each would make the resolution fail if it were read as the real
// member.
func TestREQ071_DocumentsReadMemberNamesExactly(t *testing.T) { // REQ-071
	const jwks = "https://auth.example.com/jwks"
	tests := []struct {
		name   string
		smart  func(origin string) string
		openID func(origin string) string
	}{
		{
			// Read as the issuer, the extra member names an issuer whose
			// OpenID configuration is not found.
			name: "an ISSUER member after the SMART configuration's issuer",
			smart: func(o string) string {
				return appendMember(smartDocument(o+idpPath, jwks), `"ISSUER":"`+o+`/other"`)
			},
			openID: func(o string) string { return openIDDocument(o+idpPath, jwks) },
		},
		{
			name:  "an ISSUER member after the OpenID configuration's issuer",
			smart: func(o string) string { return smartDocument(o+idpPath, jwks) },
			openID: func(o string) string {
				return appendMember(openIDDocument(o+idpPath, jwks), `"ISSUER":"https://idp.example.com"`)
			},
		},
		{
			name:  "a JWKS_URI member after the OpenID configuration's jwks_uri",
			smart: func(o string) string { return smartDocument(o+idpPath, jwks) },
			openID: func(o string) string {
				return appendMember(openIDDocument(o+idpPath, jwks), `"JWKS_URI":"https://other.example.com/jwks"`)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := startPlatform(t, false, serve(tc.smart), serve(tc.openID))
			smartBody, openIDBody := tc.smart(p.srv.URL), tc.openID(p.srv.URL)

			cat, err := p.resolver(t).Resolve(t.Context(), p.baseURL())
			if err != nil {
				t.Fatalf("Resolve(SMART configuration %s, OpenID configuration %s) error = %v, want success", smartBody, openIDBody, err)
			}
			if cat.Issuer != p.issuer() {
				t.Errorf("Resolve(SMART configuration %s) catalog Issuer = %q, want %q", smartBody, cat.Issuer, p.issuer())
			}
			paths, _ := p.requests()
			if want := []string{smartDocPath, openIDDocPath}; !slices.Equal(paths, want) {
				t.Errorf("requested paths %q, want %q", paths, want)
			}
		})
	}
}

// TestREQ071_DocumentWithARepeatedMemberIsRefused pins that a SMART or OpenID
// configuration that repeats a member name is refused as a document that
// does not decode, each through the error its fetch already gives such a
// document. Each repeat carries the same value, so only the repetition
// refuses the document.
func TestREQ071_DocumentWithARepeatedMemberIsRefused(t *testing.T) { // REQ-071
	tests := []struct {
		name       string
		smart      func(origin string) string
		openID     func(origin string) string
		wantReason discovery.DiscoveryErrorReason
	}{
		{
			name: "the SMART configuration repeats token_endpoint",
			smart: func(string) string {
				return appendMember(smartDocument("", ""), `"token_endpoint":"https://auth.example.com/token"`)
			},
			openID:     func(string) string { return `{}` },
			wantReason: discovery.ReasonParseError,
		},
		{
			name:  "the OpenID configuration repeats issuer",
			smart: func(o string) string { return smartDocument(o+idpPath, "") },
			openID: func(o string) string {
				return appendMember(openIDDocument(o+idpPath, ""), `"issuer":"`+o+idpPath+`"`)
			},
			wantReason: discovery.ReasonFetchFailed,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := startPlatform(t, false, serve(tc.smart), serve(tc.openID))
			smartBody, openIDBody := tc.smart(p.srv.URL), tc.openID(p.srv.URL)

			_, err := p.resolver(t).Resolve(t.Context(), p.baseURL())
			derr, ok := errors.AsType[*discovery.DiscoveryError](err)
			if !ok || derr == nil || derr.Reason != tc.wantReason {
				t.Fatalf("Resolve(SMART configuration %s, OpenID configuration %s) error = %v, want a DiscoveryError with Reason %q", smartBody, openIDBody, err, tc.wantReason)
			}
		})
	}
}
