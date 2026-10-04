package clientcreds_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/clientcreds"
	"github.com/cadasto/openehr-sdk-go/auth/jwtbearer"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

const (
	catalogIssuer   = "https://issuer.example"
	catalogTokenURL = "https://as.example/token"
	// catalogSecret must never appear in a construction error.
	catalogSecret = "s3cr3t-value"
)

// es256Signer returns a ClaimsSigner that signs with ES256.
func es256Signer(t *testing.T) *jwtbearer.ClaimsSigner {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jwtbearer.NewClaimsSigner(
		jwtbearer.ClaimsTemplate{Issuer: "c", Audience: catalogTokenURL},
		key,
		jwtbearer.WithAlgorithm("ES256"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

// TestNewFromCatalogUsesCatalogEndpointAndIssuer — REQ-068: the Source posts
// to catalog.Auth.TokenEndpoint and records catalog.Issuer on its tokens,
// unless the caller passes its own WithIssuer.
func TestNewFromCatalogUsesCatalogEndpointAndIssuer(t *testing.T) { // REQ-068
	ep := &tokenEndpoint{}
	mux := http.NewServeMux()
	mux.Handle("POST /token", ep)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cat := &discovery.ServiceCatalog{
		BaseURL: srv.URL,
		Issuer:  catalogIssuer,
		Auth: discovery.AuthEndpoints{
			TokenEndpoint:       discovery.MustParseURL(srv.URL + "/token"),
			GrantTypesSupported: []string{"client_credentials"},
		},
	}

	tests := []struct {
		name       string
		opts       []clientcreds.Option
		wantIssuer string
	}{
		{name: "catalog issuer", wantIssuer: catalogIssuer},
		{name: "caller issuer wins", opts: []clientcreds.Option{clientcreds.WithIssuer("https://other.example")}, wantIssuer: "https://other.example"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := append([]clientcreds.Option{clientcreds.WithHTTPClient(srv.Client())}, tc.opts...)
			src, err := clientcreds.NewFromCatalog(cat, "c", catalogSecret, opts...)
			if err != nil {
				t.Fatalf("NewFromCatalog() error = %v", err)
			}
			before := ep.posts.Load()
			tok, err := src.Token(t.Context())
			if err != nil {
				t.Fatalf("Token() error = %v", err)
			}
			if got := ep.posts.Load() - before; got != 1 {
				t.Errorf("POSTs to the catalog token endpoint = %d, want 1", got)
			}
			if tok.Issuer != tc.wantIssuer {
				t.Errorf("Token().Issuer = %q, want %q", tok.Issuer, tc.wantIssuer)
			}
		})
	}
}

// TestNewFromCatalogRefusesMissingTokenEndpoint — REQ-068: a nil catalog, or
// one without a token endpoint, is refused with auth.ErrInvalidConfig.
func TestNewFromCatalogRefusesMissingTokenEndpoint(t *testing.T) { // REQ-068
	tests := []struct {
		name string
		cat  *discovery.ServiceCatalog
	}{
		{name: "nil catalog", cat: nil},
		{name: "no token endpoint", cat: &discovery.ServiceCatalog{Issuer: catalogIssuer}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := clientcreds.NewFromCatalog(tc.cat, "c", catalogSecret, clientcreds.WithHTTPClient(&http.Client{}))
			if !errors.Is(err, auth.ErrInvalidConfig) {
				t.Errorf("NewFromCatalog(%s) error = %v, want auth.ErrInvalidConfig", tc.name, err)
			}
		})
	}
}

// TestNewFromCatalogChecksAdvertisedMetadata — REQ-068: construction fails
// with auth.ErrInvalidConfig when a list the catalog advertises leaves out the
// client_credentials grant, the configured client auth method, or the
// algorithm of the SDK's own ClaimsSigner. An absent or empty list constrains
// nothing, and an assertion source other than a ClaimsSigner is not checked.
// The error names the list and the configured value, never the secret.
func TestNewFromCatalogChecksAdvertisedMetadata(t *testing.T) { // REQ-068
	signer := es256Signer(t)
	static := jwtbearer.StaticAssertion("a.b.c")
	assertion := func(src jwtbearer.AssertionSource) []clientcreds.Option {
		return []clientcreds.Option{clientcreds.WithClientAssertion(src)}
	}
	post := []clientcreds.Option{clientcreds.WithAuthMethod(clientcreds.AuthPost)}

	tests := []struct {
		name    string
		auth    discovery.AuthEndpoints
		secret  string
		opts    []clientcreds.Option
		wantErr []string // substrings of the error; nil means construction succeeds
	}{
		// grant_types_supported
		{
			name:    "grant list leaves out client_credentials",
			auth:    discovery.AuthEndpoints{GrantTypesSupported: []string{"authorization_code"}},
			secret:  catalogSecret,
			wantErr: []string{"grant_types_supported", `"client_credentials"`},
		},
		{
			name:   "grant list names client_credentials",
			auth:   discovery.AuthEndpoints{GrantTypesSupported: []string{"authorization_code", "client_credentials"}},
			secret: catalogSecret,
		},
		{
			name:   "grant list empty",
			auth:   discovery.AuthEndpoints{GrantTypesSupported: []string{}},
			secret: catalogSecret,
		},

		// token_endpoint_auth_methods_supported
		{
			name:    "method list leaves out client_secret_basic",
			auth:    discovery.AuthEndpoints{TokenEndpointAuthMethodsSupported: []string{"client_secret_post", "private_key_jwt"}},
			secret:  catalogSecret,
			wantErr: []string{"token_endpoint_auth_methods_supported", `"client_secret_basic"`},
		},
		{
			name:    "method list leaves out client_secret_post",
			auth:    discovery.AuthEndpoints{TokenEndpointAuthMethodsSupported: []string{"client_secret_basic"}},
			secret:  catalogSecret,
			opts:    post,
			wantErr: []string{"token_endpoint_auth_methods_supported", `"client_secret_post"`},
		},
		{
			name:    "method list leaves out private_key_jwt",
			auth:    discovery.AuthEndpoints{TokenEndpointAuthMethodsSupported: []string{"client_secret_basic", "client_secret_post"}},
			opts:    assertion(static),
			wantErr: []string{"token_endpoint_auth_methods_supported", `"private_key_jwt"`},
		},
		{
			name:   "method list names client_secret_basic",
			auth:   discovery.AuthEndpoints{TokenEndpointAuthMethodsSupported: []string{"client_secret_basic"}},
			secret: catalogSecret,
		},
		{
			name:   "method list names client_secret_post",
			auth:   discovery.AuthEndpoints{TokenEndpointAuthMethodsSupported: []string{"client_secret_post"}},
			secret: catalogSecret,
			opts:   post,
		},
		{
			name: "method list names private_key_jwt",
			auth: discovery.AuthEndpoints{TokenEndpointAuthMethodsSupported: []string{"private_key_jwt"}},
			opts: assertion(static),
		},
		{
			name:   "method list empty",
			auth:   discovery.AuthEndpoints{TokenEndpointAuthMethodsSupported: []string{}},
			secret: catalogSecret,
			opts:   post,
		},

		// token_endpoint_auth_signing_alg_values_supported
		{
			name:    "alg list leaves out the ClaimsSigner algorithm",
			auth:    discovery.AuthEndpoints{TokenEndpointAuthSigningAlgValuesSupported: []string{"RS384", "ES384"}},
			opts:    assertion(signer),
			wantErr: []string{"token_endpoint_auth_signing_alg_values_supported", `"ES256"`},
		},
		{
			name: "alg list names the ClaimsSigner algorithm",
			auth: discovery.AuthEndpoints{TokenEndpointAuthSigningAlgValuesSupported: []string{"RS384", "ES256"}},
			opts: assertion(signer),
		},
		{
			name: "alg list empty",
			auth: discovery.AuthEndpoints{TokenEndpointAuthSigningAlgValuesSupported: []string{}},
			opts: assertion(signer),
		},
		{
			name: "alg list not checked for another assertion source",
			auth: discovery.AuthEndpoints{TokenEndpointAuthSigningAlgValuesSupported: []string{"RS384"}},
			opts: assertion(static),
		},

		// every list advertised and satisfied
		{
			name: "all lists satisfied by a ClaimsSigner",
			auth: discovery.AuthEndpoints{
				GrantTypesSupported:                        []string{"client_credentials"},
				TokenEndpointAuthMethodsSupported:          []string{"private_key_jwt"},
				TokenEndpointAuthSigningAlgValuesSupported: []string{"ES256"},
			},
			opts: assertion(signer),
		},

		// a nil ClaimsSigner can never sign, whatever the catalog lists
		{
			name:    "nil ClaimsSigner with an alg list",
			auth:    discovery.AuthEndpoints{TokenEndpointAuthSigningAlgValuesSupported: []string{"RS384"}},
			opts:    assertion((*jwtbearer.ClaimsSigner)(nil)),
			wantErr: []string{"nil", "ClaimsSigner"},
		},
		{
			name:    "nil ClaimsSigner with no lists",
			opts:    assertion((*jwtbearer.ClaimsSigner)(nil)),
			wantErr: []string{"nil", "ClaimsSigner"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.auth
			a.TokenEndpoint = discovery.MustParseURL(catalogTokenURL)
			cat := &discovery.ServiceCatalog{Issuer: catalogIssuer, Auth: a}
			opts := append([]clientcreds.Option{clientcreds.WithHTTPClient(&http.Client{})}, tc.opts...)

			src, err := clientcreds.NewFromCatalog(cat, "c", tc.secret, opts...)
			if tc.wantErr == nil {
				if err != nil || src == nil {
					t.Fatalf("NewFromCatalog() = (%v, %v), want a Source and no error", src, err)
				}
				return
			}
			if !errors.Is(err, auth.ErrInvalidConfig) {
				t.Fatalf("NewFromCatalog() error = %v, want auth.ErrInvalidConfig", err)
			}
			if src != nil {
				t.Errorf("NewFromCatalog() Source = %v, want nil on error", src)
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("NewFromCatalog() error = %q, want it to name %s", err, want)
				}
			}
			if strings.Contains(err.Error(), catalogSecret) {
				t.Errorf("NewFromCatalog() error = %q, must not carry the client secret", err)
			}
		})
	}
}

// TestConstructorsRefuseNilClaimsSigner — REQ-068: a client assertion that is
// a nil *jwtbearer.ClaimsSigner can never sign, so New and FromConfig refuse
// it with auth.ErrInvalidConfig instead of failing on the first Token call.
func TestConstructorsRefuseNilClaimsSigner(t *testing.T) { // REQ-068
	var nilSigner *jwtbearer.ClaimsSigner
	tests := []struct {
		name  string
		build func() (*clientcreds.Source, error)
	}{
		{
			name: "New",
			build: func() (*clientcreds.Source, error) {
				return clientcreds.New("c", "", catalogTokenURL,
					clientcreds.WithHTTPClient(&http.Client{}),
					clientcreds.WithClientAssertion(nilSigner))
			},
		},
		{
			name: "FromConfig",
			build: func() (*clientcreds.Source, error) {
				return clientcreds.FromConfig(clientcreds.Config{
					HTTPClient:      &http.Client{},
					TokenURL:        catalogTokenURL,
					ClientID:        "c",
					ClientAssertion: nilSigner,
				})
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src, err := tc.build()
			if !errors.Is(err, auth.ErrInvalidConfig) {
				t.Fatalf("%s(nil ClaimsSigner) error = %v, want auth.ErrInvalidConfig", tc.name, err)
			}
			if src != nil {
				t.Errorf("%s(nil ClaimsSigner) Source = %v, want nil on error", tc.name, src)
			}
		})
	}
}
