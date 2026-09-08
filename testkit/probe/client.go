package probe

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// NewClient builds a [transport.Client] against baseURL (the openEHR
// REST service entry, including the /openehr/v1 suffix). httpClient
// is required (REQ-021). tokenSrc may be nil (anonymous). baseURL is
// parsed rather than handed to [discovery.MustParseURL], which panics
// on a malformed value — a base URL sourced from an environment
// variable (as Live-mode invocations do) must fail as an error, not
// crash the process.
func NewClient(baseURL string, httpClient *http.Client, tokenSrc auth.TokenSource) (*transport.Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("probe.NewClient: %w: empty base URL", transport.ErrInvalidConfig)
	}
	if httpClient == nil {
		return nil, fmt.Errorf("probe.NewClient: %w: HTTP client is required", transport.ErrInvalidConfig)
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("probe.NewClient: %w: base URL: %w", transport.ErrInvalidConfig, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("probe.NewClient: %w: base URL must be absolute", transport.ErrInvalidConfig)
	}
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: baseURL,
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     u,
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("probe.NewClient: %w", err)
	}
	opts := []transport.Option{transport.WithHTTPClient(httpClient)}
	if tokenSrc != nil {
		opts = append(opts, transport.WithTokenSource(tokenSrc))
	}
	return transport.New(cat, opts...)
}
