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
//
// Resolvers built with different options may share one Cache. Before a
// Resolver returns a fresh entry, it runs those of its own checks that send
// no request: the required services, the accepted spec versions, the
// authorization-server members, and the https rule for the base URL, the
// issuer and the auth endpoints. Two things are not checked on a fresh
// entry. The issuer's OpenID configuration is not fetched, so an entry that
// a Resolver built with WithoutOpenIDConfigurationCheck cached reaches a
// Resolver that wants the issuer confirmed, until the entry expires or is
// refreshed. Nor is the document path compared: entries are keyed by the
// base URL alone, so an entry fetched from one Resolver's WithWellKnownPath
// reaches a Resolver configured with another path.
//
// Only a fresh entry that a Resolver refuses stays cached for the others.
// When a Resolver fetches again, because the entry has expired or Refresh
// was called, and the fetch or the checks on its result fail, the entry is
// dropped, unless only the caller's own context ended.
//
// The Resolver logs an error from Put or Invalidate at warning level and
// carries on: it does not reach the caller of Resolve or Refresh. A failed
// Put or Invalidate leaves the cache as it was, so the next Resolve fetches
// again unless the entry the cache still holds is fresh. A panic in Put or
// Invalidate reaches the caller that started the fetch as it was raised, and
// each caller waiting on that fetch whose own context is live fetches it
// again.
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
