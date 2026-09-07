package probe

import (
	"fmt"
	"net/http"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// NewClient builds a [transport.Client] against baseURL (the openEHR
// REST service entry, including the /openehr/v1 suffix). httpClient
// is required (REQ-021). tokenSrc may be nil (anonymous).
func NewClient(baseURL string, httpClient *http.Client, tokenSrc auth.TokenSource) (*transport.Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("probe.NewClient: %w: empty base URL", transport.ErrInvalidConfig)
	}
	if httpClient == nil {
		return nil, fmt.Errorf("probe.NewClient: %w: HTTP client is required", transport.ErrInvalidConfig)
	}
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: baseURL,
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(baseURL),
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
