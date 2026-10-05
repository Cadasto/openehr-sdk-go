// Example: service-to-service authentication, for a backend job that calls an
// openEHR server with no user present. The job obtains an access token with the
// OAuth 2.0 client credentials grant and hands the token source to the REST
// transport, which puts the token on every request. One harmless System API
// call proves the wiring, and a second call shows the cached token being
// reused.
//
// It runs offline against an in-process fake (see fake_server.go) that plays
// both the authorization server and the openEHR server, with no network
// listener:
//
//	go run ./cmd/examples/service-auth
//
// To try other credentials, set them in the environment. The fake accepts
// whatever the program reads, so the output does not change:
//
//	OPENEHR_CLIENT_ID=... OPENEHR_CLIENT_SECRET=... go run ./cmd/examples/service-auth
package main

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/clientcreds"
	"github.com/cadasto/openehr-sdk-go/openehr/client/system"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const (
	// issuer is the authorization server the fake pretends to be.
	issuer = "https://sandbox.local"
	// restBaseURL is where the fake pretends to serve the openEHR REST API.
	restBaseURL = issuer + "/openehr/v1"
	// tokenPath and tokenEndpoint locate the token endpoint, outside the REST API.
	tokenPath     = "/oauth/token"
	tokenEndpoint = issuer + tokenPath
)

// Placeholder credentials, used only when the environment sets none. They
// exist so this offline example runs as is; never ship a credential in code.
const (
	placeholderClientID     = "example-client"
	placeholderClientSecret = "example-secret"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// Step 1: read the credentials from the environment, never from flags,
	// which other users on the host can read in the process list. A real
	// deployment fills these variables from its secret store. Nothing below
	// prints the secret, the token or the Authorization header.
	clientID := cmp.Or(os.Getenv("OPENEHR_CLIENT_ID"), placeholderClientID)
	clientSecret := cmp.Or(os.Getenv("OPENEHR_CLIENT_SECRET"), placeholderClientSecret)

	// Step 2: bound the work. The context caps the whole run and the HTTP
	// client's timeout caps each request. The same client serves the token
	// endpoint and the openEHR server. It comes from the fake here; in your
	// app, inject your own, with your TLS settings.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fake := newFakeServer(clientID, clientSecret)
	httpClient := fake.backend().HTTPClient()
	httpClient.Timeout = 10 * time.Second

	// Step 3: describe the deployment in a service catalog.
	catalog, err := newCatalog()
	if err != nil {
		return err
	}

	// Step 4: the token source, which runs the client credentials grant.
	src, err := newTokenSource(catalog, clientID, clientSecret, httpClient)
	if err != nil {
		return err
	}

	// Step 5: the transport asks the token source for a token before each
	// request. WithReauthOn401 adds a safety net: when the server answers 401
	// for a token it no longer accepts, the transport gets a fresh token once
	// and sends the request again.
	client, err := transport.New(catalog,
		transport.WithHTTPClient(httpClient),
		transport.WithTokenSource(src),
		transport.WithReauthOn401(src),
	)
	if err != nil {
		return fmt.Errorf("transport.New: %w", err)
	}

	// Step 6: one harmless call that carries the token. Capabilities sends
	// OPTIONS / and the fake answers it only with a valid bearer token. Do not
	// use system.Health to check the wiring: it never sends a token.
	caps, _, err := system.Capabilities(ctx, client)
	if err != nil {
		return fmt.Errorf("first capabilities call: %w", err)
	}
	fmt.Printf("call 1: %s %s, openEHR REST %s\n", caps.Solution, caps.SolutionVersion, caps.RESTAPISpecsVersion)

	// Step 7: call again. The token source keeps the token until it is within
	// 30 seconds of expiry (the default; change it with
	// clientcreds.WithRefreshThreshold). The fake's token lives 300 seconds,
	// so the second call reuses it and the token endpoint is not asked again.
	caps, _, err = system.Capabilities(ctx, client)
	if err != nil {
		return fmt.Errorf("second capabilities call: %w", err)
	}
	fmt.Printf("call 2: %s %s, openEHR REST %s\n", caps.Solution, caps.SolutionVersion, caps.RESTAPISpecsVersion)
	fmt.Printf("token requests: %d for 2 calls\n", fake.tokenRequests())
	fmt.Println("OK: both calls carried a client credentials token")
	return nil
}

// newCatalog describes the deployment by hand: where the openEHR REST API
// lives, and which token endpoint, grant and client authentication its
// authorization server offers. A deployment that publishes a SMART
// configuration document can be discovered instead: build a resolver with
// discovery.NewResolver and call its Resolve(ctx, baseURL), which returns the
// same catalog type.
func newCatalog() (*discovery.ServiceCatalog, error) {
	catalog, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: issuer,
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(restBaseURL),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
		Auth: discovery.AuthEndpoints{
			TokenEndpoint:                     discovery.MustParseURL(tokenEndpoint),
			GrantTypesSupported:               []string{"client_credentials"},
			TokenEndpointAuthMethodsSupported: []string{"client_secret_basic"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("build static catalog: %w", err)
	}
	return catalog, nil
}

// newTokenSource builds the client credentials token source from the catalog.
// NewFromCatalog takes the token endpoint and the issuer from the same catalog
// the transport uses. It also refuses, at startup rather than on the first
// call, a grant or a client authentication method the catalog says the server
// does not accept. With only a token URL and no catalog, use clientcreds.New.
// The client authenticates with HTTP Basic by default.
func newTokenSource(catalog *discovery.ServiceCatalog, clientID, clientSecret string, httpClient *http.Client) (*clientcreds.Source, error) {
	// A system scope: the client itself, not a user or a patient, may read
	// and search every composition. Token checks the openEHR scope syntax.
	scope, err := auth.OpenEHRScope{
		Compartment: "system",
		Resource:    "composition",
		Pattern:     "*",
		Permissions: "rs",
	}.Token()
	if err != nil {
		return nil, fmt.Errorf("build scope: %w", err)
	}
	src, err := clientcreds.NewFromCatalog(catalog, clientID, clientSecret,
		clientcreds.WithHTTPClient(httpClient),
		clientcreds.WithScope(scope),
	)
	if err != nil {
		return nil, fmt.Errorf("client credentials source: %w", err)
	}
	return src, nil
}
