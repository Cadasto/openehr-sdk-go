package discovery

import (
	"context"
	"sync"
)

// Cache is the discovery catalog cache abstraction. The default cache
// is in-process; consumers may inject a file-backed or distributed
// implementation for use cases that share catalogs across processes
// (REQ-071).
type Cache interface {
	Get(ctx context.Context, issuer string) (*ServiceCatalog, bool)
	Put(ctx context.Context, issuer string, c *ServiceCatalog) error
	Invalidate(ctx context.Context, issuer string) error
}

// MemoryCache is the default in-process Cache implementation. Safe for
// concurrent use.
type MemoryCache struct {
	mu sync.RWMutex
	m  map[string]*ServiceCatalog
}

// NewMemoryCache returns an empty MemoryCache.
func NewMemoryCache() *MemoryCache { return &MemoryCache{m: map[string]*ServiceCatalog{}} }

// Get returns the cached catalog for issuer.
func (c *MemoryCache) Get(ctx context.Context, issuer string) (*ServiceCatalog, bool) {
	if err := ctx.Err(); err != nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	cat, ok := c.m[issuer]
	return cat, ok
}

// Put stores cat under issuer.
func (c *MemoryCache) Put(ctx context.Context, issuer string, cat *ServiceCatalog) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.m[issuer] = cat
	c.mu.Unlock()
	return nil
}

// Invalidate removes any cached catalog for issuer.
func (c *MemoryCache) Invalidate(ctx context.Context, issuer string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	delete(c.m, issuer)
	c.mu.Unlock()
	return nil
}
