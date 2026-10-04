package discovery

import (
	"context"
	"sync"
)

// Cache is the discovery catalog cache abstraction. The default cache
// is in-process; consumers may inject a file-backed or distributed
// implementation for use cases that share catalogs across processes.
//
// The Resolver keys every entry by the Platform base URL the caller
// resolved (ServiceCatalog.BaseURL), never by the OpenID Connect issuer.
type Cache interface {
	Get(ctx context.Context, baseURL string) (*ServiceCatalog, bool)
	Put(ctx context.Context, baseURL string, c *ServiceCatalog) error
	Invalidate(ctx context.Context, baseURL string) error
}

// MemoryCache is the default in-process Cache implementation. Safe for
// concurrent use.
type MemoryCache struct {
	mu sync.RWMutex
	m  map[string]*ServiceCatalog
}

// NewMemoryCache returns an empty MemoryCache.
func NewMemoryCache() *MemoryCache { return &MemoryCache{m: map[string]*ServiceCatalog{}} }

// Get returns the cached catalog for baseURL.
func (c *MemoryCache) Get(ctx context.Context, baseURL string) (*ServiceCatalog, bool) {
	if err := ctx.Err(); err != nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	cat, ok := c.m[baseURL]
	return cat, ok
}

// Put stores cat under baseURL.
func (c *MemoryCache) Put(ctx context.Context, baseURL string, cat *ServiceCatalog) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.m[baseURL] = cat
	c.mu.Unlock()
	return nil
}

// Invalidate removes any cached catalog for baseURL.
func (c *MemoryCache) Invalidate(ctx context.Context, baseURL string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	delete(c.m, baseURL)
	c.mu.Unlock()
	return nil
}
