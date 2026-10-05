package auth_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/openehr/client/system"
	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func ExampleWithTokenSource() {
	// A fake openEHR server that notes which token each request carried.
	server := newTokenReporter()

	// One client shared by every caller. Its default token source is the one
	// a call uses when its context brings none.
	client, err := transport.New(mustSandboxCatalog(),
		transport.WithHTTPClient(server.backend.HTTPClient()),
		transport.WithTokenSource(auth.StaticTokenSource(auth.Token{Value: serviceToken})),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	ctx := context.Background()

	// A plain context: the request carries the client default.
	if _, _, err := system.Capabilities(ctx, client); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("plain context:", server.last)

	// auth.WithTokenSource attaches a token source to this context. The
	// transport prefers it over the client default, for the calls made with
	// this context only.
	userCtx := auth.WithTokenSource(ctx, auth.StaticTokenSource(auth.Token{Value: userToken}))
	if _, _, err := system.Capabilities(userCtx, client); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("with auth.WithTokenSource:", server.last)

	// Output:
	// plain context: Bearer token from the client default
	// with auth.WithTokenSource: Bearer token from the per-request source
}

// The example tokens. The fake server reports a label for each one, so the
// example prints the label and never the token.
const (
	serviceToken = "example-service-token"
	userToken    = "example-user-token"
)

// capabilitiesBody is the System API's OPTIONS / answer, in the shape the
// openEHR REST API gives for it.
const capabilitiesBody = `{
  "solution": "example-cdr",
  "solution_version": "1.0",
  "vendor": "example",
  "restapi_specs_version": "1.1.0",
  "conformance_profile": "STANDARD",
  "endpoints": ["/ehr", "/query", "/definition"]
}`

// tokenReporter is a fake openEHR server. It answers OPTIONS / and keeps in
// last which Authorization scheme, and which token by its label, the latest
// request carried.
type tokenReporter struct {
	backend *sandbox.Backend
	last    string
}

func newTokenReporter() *tokenReporter {
	r := &tokenReporter{backend: sandbox.New()}
	r.backend.HandleFunc(http.MethodOptions, "/", r.serveOptions)
	return r
}

func (r *tokenReporter) serveOptions(w http.ResponseWriter, req *http.Request) {
	r.last = describeAuthorization(req.Header.Get("Authorization"))
	w.Header().Set("Allow", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Content-Type", "application/json")
	// A failed write shows up as a decode error on the client side.
	_, _ = io.WriteString(w, capabilitiesBody)
}

// describeAuthorization names the scheme of an Authorization header and the
// source of its token, without the token itself.
func describeAuthorization(header string) string {
	if header == "" {
		return "no token"
	}
	scheme, token, _ := strings.Cut(header, " ")
	var source string
	switch token {
	case serviceToken:
		source = "the client default"
	case userToken:
		source = "the per-request source"
	default:
		source = "an unknown source"
	}
	return scheme + " token from " + source
}

// mustSandboxCatalog points the openEHR REST service at the base URL the
// sandbox backend serves. Against a real server, put its base URL here.
func mustSandboxCatalog() *discovery.ServiceCatalog {
	catalog, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://sandbox.local",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL("https://sandbox.local/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return catalog
}
