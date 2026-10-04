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

// audienceTestEndpoints returns endpoints that need no live server: the
// tests below only build sources and authorization URLs.
func audienceTestEndpoints() discovery.AuthEndpoints {
	return discovery.AuthEndpoints{
		AuthorizationEndpoint: discovery.MustParseURL("https://idp.example/authorize"),
		TokenEndpoint:         discovery.MustParseURL("https://idp.example/token"),
	}
}

// authorizeQuery begins a launch on src and returns the query of the
// authorization URL it builds.
func authorizeQuery(t *testing.T, src *smart.Source) url.Values {
	t.Helper()
	req, err := src.BeginAuthorization("state-123")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	raw, err := src.AuthorizeURL(req, "")
	if err != nil {
		t.Fatalf("AuthorizeURL: %v", err)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", raw, err)
	}
	return parsed.Query()
}

// TestNewFromCatalogDefaultsAudienceToBaseURL pins that a source built from
// a catalog sends the Platform base URL as aud, not the OpenID Connect
// issuer, when the caller sets no audience. The catalog's two URLs differ,
// as they do when a separate identity provider signs the tokens.
// REQ-061, REQ-065
func TestNewFromCatalogDefaultsAudienceToBaseURL(t *testing.T) { // REQ-061 REQ-065
	const (
		baseURL = "https://platform.example/openehr"
		issuer  = "https://idp.example/realms/clinic"
	)
	catalog, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		BaseURL: baseURL,
		Issuer:  issuer,
		Auth:    audienceTestEndpoints(),
	})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}
	src, err := smart.NewFromCatalog(catalog, "client-id",
		smart.WithHTTPClient(&http.Client{}),
		smart.WithRedirectURI("https://app.example/callback"),
	)
	if err != nil {
		t.Fatalf("NewFromCatalog with BaseURL %q: %v", baseURL, err)
	}
	q := authorizeQuery(t, src)
	if got := q["aud"]; len(got) != 1 || got[0] != baseURL {
		t.Errorf("AuthorizeURL aud = %q, want exactly [%q] (the catalog BaseURL, not the issuer %q)", got, baseURL, issuer)
	}
}

// TestNewFromCatalogCallerAudienceWins pins that a caller's WithAudience
// overrides the catalog's BaseURL default. REQ-061
func TestNewFromCatalogCallerAudienceWins(t *testing.T) { // REQ-061
	const explicit = "urn:example:openehr-api"
	catalog, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		BaseURL: "https://platform.example/openehr",
		Issuer:  "https://idp.example/realms/clinic",
		Auth:    audienceTestEndpoints(),
	})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}
	src, err := smart.NewFromCatalog(catalog, "client-id",
		smart.WithHTTPClient(&http.Client{}),
		smart.WithRedirectURI("https://app.example/callback"),
		smart.WithAudience(explicit),
	)
	if err != nil {
		t.Fatalf("NewFromCatalog with WithAudience(%q): %v", explicit, err)
	}
	q := authorizeQuery(t, src)
	if got := q["aud"]; len(got) != 1 || got[0] != explicit {
		t.Errorf("AuthorizeURL aud = %q, want exactly [%q]", got, explicit)
	}
}

// TestSourceWithoutAudienceRefused pins that every way of building a source
// refuses one that has no audience at all, since SMART requires aud on the
// authorization request. A hand-built catalog with an empty BaseURL gives no
// default. REQ-061
func TestSourceWithoutAudienceRefused(t *testing.T) { // REQ-061
	tests := []struct {
		name  string
		build func() (*smart.Source, error)
	}{
		{name: "New", build: func() (*smart.Source, error) {
			return smart.New("client-id", audienceTestEndpoints(),
				smart.WithHTTPClient(&http.Client{}),
				smart.WithRedirectURI("https://app.example/callback"),
			)
		}},
		{name: "FromConfig", build: func() (*smart.Source, error) {
			return smart.FromConfig(smart.Config{
				HTTPClient:  &http.Client{},
				ClientID:    "client-id",
				RedirectURI: "https://app.example/callback",
				Auth:        audienceTestEndpoints(),
			})
		}},
		{name: "NewFromCatalog with empty BaseURL", build: func() (*smart.Source, error) {
			catalog := &discovery.ServiceCatalog{
				Issuer: "https://idp.example/realms/clinic",
				Auth:   audienceTestEndpoints(),
			}
			return smart.NewFromCatalog(catalog, "client-id",
				smart.WithHTTPClient(&http.Client{}),
				smart.WithRedirectURI("https://app.example/callback"),
			)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.build()
			if !errors.Is(err, auth.ErrInvalidConfig) {
				t.Fatalf("%s without an audience: err = %v, want auth.ErrInvalidConfig", tc.name, err)
			}
			if !strings.Contains(err.Error(), "aud") {
				t.Errorf("%s without an audience: error %q does not name aud", tc.name, err)
			}
		})
	}
}
