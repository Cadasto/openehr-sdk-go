package discovery_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// TestResolveCachesWhenCallerCancels pins that a successful Resolve stores
// the catalog when the caller's context is cancelled after the HTTP response
// is received and before the cache write. The document names no separate
// issuer, so the resolver does not fetch an OpenID configuration.
// MemoryCache.Put refuses a cancelled context, so the write has to outlive
// that cancellation.
func TestResolveCachesWhenCallerCancels(t *testing.T) {
	p := startPlatform(t, false, serve(func(string) string {
		return smartDocument("", "")
	}), notFound)
	ctx, cancel := context.WithCancel(t.Context())
	base := p.srv.Client().Transport
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		resp, err := base.RoundTrip(r)
		if err != nil {
			return nil, err
		}
		cancel()
		return resp, nil
	})}
	cache := discovery.NewMemoryCache()
	res, err := discovery.NewResolver(cache, discovery.WithHTTPClient(client))
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	if _, err := res.Resolve(ctx, p.baseURL()); err != nil {
		t.Fatalf("Resolve(%q) error = %v, want success with the catalog stored", p.baseURL(), err)
	}
	if _, ok := cache.Get(t.Context(), p.baseURL()); !ok {
		t.Errorf("cache.Get(%q) found nothing, want the catalog stored after the caller cancelled", p.baseURL())
	}
}
