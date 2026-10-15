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
// resolved (ServiceCatalog.BaseURL), never by the OpenID Connect issuer. An
// entry whose BaseURL is not the base URL it is found under counts as no
// entry: the Resolver fetches the document with a plain request, as for a
// miss, and the result replaces it.
//
// Resolvers built with different options may share one Cache, and a Cache
// may hold entries that no Resolver in this process fetched, such as one
// read from a file or shared between processes. Before a Resolver returns a
// fresh entry, it runs those of its own checks that send no request, the
// same as on a newly fetched document: every URL in the entry is well
// formed (the base URL, the issuer, the auth endpoints and the service base
// URLs), the base URL, the issuer and the auth endpoints use https unless
// the Resolver allows insecure URLs, and the entry has the required
// services, the accepted spec versions and the authorization-server
// members. An entry that fails them gets the error a new document with the
// same members gets. Two things are not checked on a fresh entry. The
// issuer's OpenID configuration is not fetched, so an entry that a Resolver
// built with WithoutOpenIDConfigurationCheck cached reaches a Resolver that
// wants the issuer confirmed, until the entry expires or is refreshed. Nor
// is the document path compared: entries are keyed by the base URL alone,
// so an entry fetched from one Resolver's WithWellKnownPath reaches a
// Resolver configured with another path.
//
// A fresh entry stays cached whether a Resolver returns it or refuses it: a
// refusal says only that this Resolver's options do not accept it, so the
// entry stays for the Resolvers that do. When a Resolver fetches again,
// because the entry has expired or Refresh was called, a successful fetch
// or 304 renewal replaces the entry, and a failed one, or one whose result
// fails the checks, drops it. Two failures leave the entry as it was: one
// that only the caller's own context ending caused, and a panic in the
// fetch.
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
