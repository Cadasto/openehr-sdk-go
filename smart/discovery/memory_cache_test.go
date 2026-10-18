package discovery_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// TestREQ025_ZeroValueMemoryCacheIsReady pins that a MemoryCache literal
// works as NewMemoryCache's does: Get and Invalidate on it find nothing,
// Put stores a catalog Get then returns, and a Resolver built on it caches
// what it resolves. None of them panics.
func TestREQ025_ZeroValueMemoryCacheIsReady(t *testing.T) { // REQ-025
	const key = "https://platform.example/platform"
	cat := &discovery.ServiceCatalog{BaseURL: key}

	var c discovery.MemoryCache
	if p := catchPanic(func() {
		if got, ok := c.Get(t.Context(), key); ok || got != nil {
			t.Errorf("zero MemoryCache Get(%q) = %v, %v, want nil, false", key, got, ok)
		}
		if err := c.Invalidate(t.Context(), key); err != nil {
			t.Errorf("zero MemoryCache Invalidate(%q) = %v, want nil", key, err)
		}
		if err := c.Put(t.Context(), key, cat); err != nil {
			t.Errorf("zero MemoryCache Put(%q) = %v, want nil", key, err)
		}
	}); p != nil {
		t.Fatalf("zero MemoryCache panicked: %v", p)
	}
	if got, ok := c.Get(t.Context(), key); !ok || got != cat {
		t.Errorf("Get(%q) after Put = %v, %v, want the catalog put", key, got, ok)
	}

	p := startPlatform(t, false, serve(func(string) string { return smartDocument("", "") }), notFound)
	cache := &discovery.MemoryCache{}
	res, err := discovery.NewResolver(cache, discovery.WithHTTPClient(p.srv.Client()))
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	if v := catchPanic(func() {
		if _, err := res.Resolve(t.Context(), p.baseURL()); err != nil {
			t.Errorf("Resolve(%q) on a zero MemoryCache: %v", p.baseURL(), err)
		}
	}); v != nil {
		t.Fatalf("Resolve(%q) on a zero MemoryCache panicked: %v", p.baseURL(), v)
	}
	if _, ok := cache.Get(t.Context(), p.baseURL()); !ok {
		t.Errorf("zero MemoryCache Get(%q) after Resolve found nothing, want the resolved catalog", p.baseURL())
	}
}
