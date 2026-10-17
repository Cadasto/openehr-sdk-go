package discovery_test

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// TestREQ021_NewResolverWithoutHTTPClient pins the error NewResolver returns
// when the caller supplies no HTTP client: a *DiscoveryError with
// ReasonFetchFailed, whose text names the missing client after a single
// "discovery: " prefix.
func TestREQ021_NewResolverWithoutHTTPClient(t *testing.T) { // REQ-021, REQ-025
	const want = "discovery: fetch_failed: HTTPClient is required"
	tests := []struct {
		name string
		opts []discovery.Option
	}{
		{name: "no options"},
		{name: "nil client", opts: []discovery.Option{discovery.WithHTTPClient(nil)}},
		{name: "nil option", opts: []discovery.Option{nil}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := discovery.NewResolver(nil, tc.opts...)
			de, ok := errors.AsType[*discovery.DiscoveryError](err)
			if res != nil || !ok || de == nil || de.Reason != discovery.ReasonFetchFailed {
				t.Fatalf("NewResolver(%s) = %v, %v; want nil and a DiscoveryError with Reason %q", tc.name, res, err, discovery.ReasonFetchFailed)
			}
			if got := err.Error(); got != want {
				t.Errorf("NewResolver(%s) error text = %q, want %q", tc.name, got, want)
			}
		})
	}
}
