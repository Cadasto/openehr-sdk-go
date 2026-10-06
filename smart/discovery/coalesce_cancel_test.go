package discovery_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// heldPlatform serves the SMART configuration document and holds the
// holdAt-th request, until open is called or the client gives up on it, then
// answers it with heldStatus (200 with the document when zero). Every other
// request is answered at once. It runs on the in-memory network, so it is for
// tests inside a synctest bubble. Its URL is plain http, so its resolver
// allows insecure URLs.
type heldPlatform struct {
	srv        *httptest.Server
	client     *http.Client
	hits       atomic.Int32
	holdAt     int32
	heldStatus int
	release    chan struct{}
	once       sync.Once
}

func startHeldPlatform(t *testing.T, holdAt int32, heldStatus int) *heldPlatform {
	t.Helper()
	p := &heldPlatform{holdAt: holdAt, heldStatus: heldStatus, release: make(chan struct{})}
	p.srv = httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.hits.Add(1) == p.holdAt {
			select {
			case <-p.release:
			case <-r.Context().Done():
			}
			if p.heldStatus != 0 {
				w.WriteHeader(p.heldStatus)
				return
			}
		}
		w.Header().Set("Cache-Control", "max-age=60")
		_, _ = io.WriteString(w, smartDocument("", ""))
	}))
	p.client = p.srv.Client() // sets srv.URL
	// The server's own cleanup waits for its handlers; open the held one first.
	t.Cleanup(p.open)
	return p
}

// open answers the held request.
func (p *heldPlatform) open() { p.once.Do(func() { close(p.release) }) }

func (p *heldPlatform) baseURL() string { return p.srv.URL + platformPath }

// resolver builds a Resolver over a fresh MemoryCache, which it also returns.
func (p *heldPlatform) resolver(t *testing.T) (*discovery.Resolver, *discovery.MemoryCache) {
	t.Helper()
	cache := discovery.NewMemoryCache()
	res, err := discovery.NewResolver(cache, discovery.WithHTTPClient(p.client), discovery.WithAllowInsecure())
	if err != nil {
		t.Fatal(err)
	}
	return res, cache
}

// resolverCall is one way to start a resolution: Resolve or Refresh.
type resolverCall func(ctx context.Context, res *discovery.Resolver, baseURL string) (*discovery.ServiceCatalog, error)

func callResolve(ctx context.Context, res *discovery.Resolver, baseURL string) (*discovery.ServiceCatalog, error) {
	return res.Resolve(ctx, baseURL)
}

func callRefresh(ctx context.Context, res *discovery.Resolver, baseURL string) (*discovery.ServiceCatalog, error) {
	return res.Refresh(ctx, baseURL)
}

// TestCoalescedWaiterFetchesAgainWhenStarterCancels pins REQ-071: when the
// caller that is fetching gives up, it gets its own context's error at once,
// and a caller that joined its fetch, whose own context is live, fetches
// again and gets a catalog instead of the starter's context error.
func TestCoalescedWaiterFetchesAgainWhenStarterCancels(t *testing.T) { // REQ-071
	tests := []struct {
		name string
		// warm resolves once before the held fetch, so the held fetch is the
		// second request and starts from a fresh cached entry.
		warm bool
		call resolverCall
	}{
		{name: "Resolve with nothing cached", call: callResolve},
		{name: "Refresh of a fresh entry", warm: true, call: callRefresh},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				holdAt := int32(1)
				if tc.warm {
					holdAt = 2
				}
				p := startHeldPlatform(t, holdAt, 0)
				res, cache := p.resolver(t)
				baseURL := p.baseURL()
				if tc.warm {
					if _, err := res.Resolve(t.Context(), baseURL); err != nil {
						t.Fatalf("warm-up Resolve(%q) error = %v", baseURL, err)
					}
				}

				ctx1, cancel1 := context.WithCancel(t.Context())
				defer cancel1()
				var (
					wg         sync.WaitGroup
					err1, err2 error
					cat2       *discovery.ServiceCatalog
				)
				wg.Go(func() { _, err1 = tc.call(ctx1, res, baseURL) })
				synctest.Wait() // caller 1's request is held at the server
				wg.Go(func() { cat2, err2 = tc.call(t.Context(), res, baseURL) })
				synctest.Wait() // caller 2 waits for caller 1's fetch
				cancel1()
				synctest.Wait() // caller 1 gave up; caller 2 fetched again, or failed
				wg.Wait()       // the held request was never answered

				if !errors.Is(err1, context.Canceled) {
					t.Errorf("caller 1 error = %v, want its own context.Canceled", err1)
				}
				if err2 != nil || cat2 == nil {
					t.Errorf("caller 2 = %v, %v, want a catalog: its own context is live, so it fetches again", cat2, err2)
				}
				if got, want := p.hits.Load(), holdAt+1; got != want {
					t.Errorf("server answered %d requests, want %d: caller 2 sends a second request of its own", got, want)
				}
				if cached, ok := cache.Get(t.Context(), baseURL); cat2 != nil && (!ok || cached != cat2) {
					t.Errorf("cache.Get(%q) = %p, %t, want the catalog %p caller 2 fetched", baseURL, cached, ok, cat2)
				}
			})
		})
	}
}

// TestCancelledFetchKeepsCachedCatalog pins REQ-071: a fetch that failed only
// because its caller's context ended says nothing about the Platform, so it
// leaves the cached entry in place, and a fresh entry keeps being served
// without a request.
func TestCancelledFetchKeepsCachedCatalog(t *testing.T) { // REQ-071
	tests := []struct {
		name string
		call resolverCall
		// expired lets the warmed-up entry expire before the held fetch.
		expired bool
	}{
		{name: "Refresh of a fresh entry", call: callRefresh},
		{name: "Resolve of an expired entry", call: callResolve, expired: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p := startHeldPlatform(t, 2, 0)
				res, cache := p.resolver(t)
				baseURL := p.baseURL()
				warm, err := res.Resolve(t.Context(), baseURL)
				if err != nil {
					t.Fatalf("warm-up Resolve(%q) error = %v", baseURL, err)
				}
				if tc.expired {
					time.Sleep(2 * time.Minute) // past the 60 s the document asked for
				}

				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				var (
					wg   sync.WaitGroup
					err2 error
				)
				wg.Go(func() { _, err2 = tc.call(ctx, res, baseURL) })
				synctest.Wait() // the held fetch is at the server
				cancel()
				wg.Wait()

				if !errors.Is(err2, context.Canceled) {
					t.Fatalf("cancelled call error = %v, want context.Canceled", err2)
				}
				if cached, ok := cache.Get(t.Context(), baseURL); !ok || cached != warm {
					t.Errorf("cache.Get(%q) = %p, %t, want the entry %p kept: the failure was the caller's own cancellation", baseURL, cached, ok, warm)
				}
				if tc.expired {
					return
				}
				got, err := res.Resolve(t.Context(), baseURL)
				if err != nil || got != warm {
					t.Errorf("Resolve(%q) after the cancelled refresh = %p, %v, want the cached catalog %p", baseURL, got, err, warm)
				}
				if hits := p.hits.Load(); hits != 2 {
					t.Errorf("server answered %d requests, want 2: the fresh entry is served without a request", hits)
				}
			})
		})
	}
}

// TestCoalescedFailureNotFromACallersContextIsShared pins REQ-071: a failure
// that no caller's context caused reaches every caller that joined the fetch,
// with no second request, and still drops the cached entry. The HTTP
// client's own timeout reports context.DeadlineExceeded too, but the
// starter's context is still live then, so it is not the starter giving up.
func TestCoalescedFailureNotFromACallersContextIsShared(t *testing.T) { // REQ-071
	tests := []struct {
		name       string
		heldStatus int
		timeout    time.Duration // the HTTP client's Timeout; zero means none
		// settle ends the held fetch.
		settle func(p *heldPlatform)
	}{
		{
			name:       "server error",
			heldStatus: http.StatusServiceUnavailable,
			settle:     func(p *heldPlatform) { p.open() },
		},
		{
			name:    "HTTP client timeout",
			timeout: 30 * time.Second,
			settle:  func(*heldPlatform) { time.Sleep(time.Minute) },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p := startHeldPlatform(t, 2, tc.heldStatus)
				p.client.Timeout = tc.timeout
				res, cache := p.resolver(t)
				baseURL := p.baseURL()
				if _, err := res.Resolve(t.Context(), baseURL); err != nil {
					t.Fatalf("warm-up Resolve(%q) error = %v", baseURL, err)
				}

				var (
					wg         sync.WaitGroup
					err1, err2 error
				)
				wg.Go(func() { _, err1 = res.Refresh(t.Context(), baseURL) })
				synctest.Wait() // the held fetch is at the server
				wg.Go(func() { _, err2 = res.Refresh(t.Context(), baseURL) })
				synctest.Wait() // caller 2 waits for caller 1's fetch
				tc.settle(p)
				wg.Wait()

				var derrs []*discovery.DiscoveryError
				for i, err := range []error{err1, err2} {
					derr, ok := errors.AsType[*discovery.DiscoveryError](err)
					if !ok || derr.Reason != discovery.ReasonFetchFailed {
						t.Errorf("caller %d error = %v, want a DiscoveryError with Reason %q", i+1, err, discovery.ReasonFetchFailed)
					}
					derrs = append(derrs, derr)
				}
				if derrs[0] != derrs[1] {
					t.Errorf("caller 2 error = %v, want the error caller 1 got, %v: both share one fetch", err2, err1)
				}
				if tc.timeout != 0 && !errors.Is(err1, context.DeadlineExceeded) {
					t.Errorf("error = %v, want one that reports context.DeadlineExceeded, as the HTTP client's timeout does", err1)
				}
				if got := p.hits.Load(); got != 2 {
					t.Errorf("server answered %d requests, want 2: the warm-up and one shared fetch", got)
				}
				if _, ok := cache.Get(t.Context(), baseURL); ok {
					t.Errorf("cache still holds a catalog for %q after the failed fetch, want it dropped", baseURL)
				}
			})
		})
	}
}

// TestCoalescedWaiterReturnsItsOwnContextError pins REQ-026 and REQ-071: a
// waiter whose own context ends stops waiting and returns its own context's
// error, while the fetch it joined goes on for the others.
func TestCoalescedWaiterReturnsItsOwnContextError(t *testing.T) { // REQ-026, REQ-071
	synctest.Test(t, func(t *testing.T) {
		p := startHeldPlatform(t, 1, 0)
		res, cache := p.resolver(t)
		baseURL := p.baseURL()

		var (
			wg         sync.WaitGroup
			err1, err2 error
			cat1       *discovery.ServiceCatalog
		)
		wg.Go(func() { cat1, err1 = res.Resolve(t.Context(), baseURL) })
		synctest.Wait() // caller 1's request is held at the server
		waiterCtx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		wg.Go(func() { _, err2 = res.Resolve(waiterCtx, baseURL) })
		time.Sleep(2 * time.Minute) // the waiter's deadline passes, caller 1's fetch is still held
		synctest.Wait()
		p.open()
		wg.Wait()

		if !errors.Is(err2, context.DeadlineExceeded) {
			t.Errorf("waiter error = %v, want its own context.DeadlineExceeded", err2)
		}
		if err1 != nil || cat1 == nil {
			t.Errorf("caller 1 = %v, %v, want its catalog: a waiter that gave up does not fail the fetch", cat1, err1)
		}
		if got := p.hits.Load(); got != 1 {
			t.Errorf("server answered %d requests, want 1", got)
		}
		if cached, ok := cache.Get(t.Context(), baseURL); !ok || cached != cat1 {
			t.Errorf("cache.Get(%q) = %p, %t, want the catalog %p caller 1 fetched", baseURL, cached, ok, cat1)
		}
	})
}
