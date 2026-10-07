package discovery_test

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// testPanic is the value the fakes below panic with. A test compares the
// value a caller recovers with the one the fake raised by identity, so a
// panic that was wrapped or replaced on the way does not match.
type testPanic struct{ source string }

// panickingCache is a MemoryCache whose next Put or Invalidate, once armed,
// panics with value instead of changing the cache.
type panickingCache struct {
	*discovery.MemoryCache
	value           *testPanic
	putArmed        atomic.Bool
	invalidateArmed atomic.Bool
}

func (c *panickingCache) Put(ctx context.Context, baseURL string, cat *discovery.ServiceCatalog) error {
	if c.putArmed.CompareAndSwap(true, false) {
		panic(c.value)
	}
	return c.MemoryCache.Put(ctx, baseURL, cat)
}

func (c *panickingCache) Invalidate(ctx context.Context, baseURL string) error {
	if c.invalidateArmed.CompareAndSwap(true, false) {
		panic(c.value)
	}
	return c.MemoryCache.Invalidate(ctx, baseURL)
}

// panickingTransport wraps an HTTP transport. Once armed, its next round trip
// panics with value after the server has answered, so a request the platform
// holds holds the panic too.
type panickingTransport struct {
	next  http.RoundTripper
	value *testPanic
	armed atomic.Bool
}

func (p *panickingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := p.next.RoundTrip(req)
	if p.armed.CompareAndSwap(true, false) {
		if resp != nil {
			_ = resp.Body.Close()
		}
		panic(p.value)
	}
	return resp, err
}

// panicFixture is a held platform and a Resolver whose Cache and HTTP
// transport can each be armed to panic with value.
type panicFixture struct {
	p     *heldPlatform
	res   *discovery.Resolver
	cache *panickingCache
	tr    *panickingTransport
	value *testPanic
}

// startPanicFixture starts a held platform, as startHeldPlatform does, and
// builds a Resolver over a panickingCache and a panickingTransport.
func startPanicFixture(t *testing.T, holdAt int32, heldStatus int) *panicFixture {
	t.Helper()
	p := startHeldPlatform(t, holdAt, heldStatus)
	value := &testPanic{source: t.Name()}
	tr := &panickingTransport{next: p.client.Transport, value: value}
	p.client.Transport = tr
	cache := &panickingCache{MemoryCache: discovery.NewMemoryCache(), value: value}
	res, err := discovery.NewResolver(cache, discovery.WithHTTPClient(p.client), discovery.WithAllowInsecure())
	if err != nil {
		t.Fatal(err)
	}
	return &panicFixture{p: p, res: res, cache: cache, tr: tr, value: value}
}

// panicSource is one place a panic can start while a fetch runs: the fetch
// itself, or the Cache call that records its result.
type panicSource struct {
	name string
	// heldStatus is the status the platform answers the held request with;
	// zero means 200 with the document.
	heldStatus int
	// arm makes the next fetch panic at this source.
	arm func(f *panicFixture)
}

var panicSources = []panicSource{
	{
		name: "Cache.Put panics",
		arm:  func(f *panicFixture) { f.cache.putArmed.Store(true) },
	},
	{
		name:       "Cache.Invalidate panics",
		heldStatus: http.StatusServiceUnavailable, // a failed fetch drops the entry
		arm:        func(f *panicFixture) { f.cache.invalidateArmed.Store(true) },
	},
	{
		name: "transport panics",
		arm:  func(f *panicFixture) { f.tr.armed.Store(true) },
	},
}

// catchPanic calls f and returns the value it panicked with, or nil.
func catchPanic(f func()) (recovered any) {
	defer func() { recovered = recover() }()
	f()
	return nil
}

// TestPanicInFetchReachesStarterAndStrandsNoCaller pins REQ-071: a panic in
// the fetch, or in the Cache call that records its result, reaches the caller
// that started the fetch as it was raised, and leaves no later caller for that
// base URL waiting on the fetch it ended. After the panic the resolver itself
// neither writes nor drops the cached entry.
func TestPanicInFetchReachesStarterAndStrandsNoCaller(t *testing.T) { // REQ-071
	for _, src := range panicSources {
		for _, expired := range []bool{false, true} {
			name := src.name + ", nothing cached"
			if expired {
				name = src.name + ", expired entry cached"
			}
			t.Run(name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					// The panicking fetch is the first request, or the second
					// after a warm-up; heldStatus applies to it alone.
					holdAt := int32(1)
					if expired {
						holdAt = 2
					}
					f := startPanicFixture(t, holdAt, src.heldStatus)
					f.p.open() // answer every request at once
					baseURL := f.p.baseURL()
					var warm *discovery.ServiceCatalog
					if expired {
						var err error
						if warm, err = f.res.Resolve(t.Context(), baseURL); err != nil {
							t.Fatalf("warm-up Resolve(%q) error = %v", baseURL, err)
						}
						time.Sleep(2 * time.Minute) // past the 60 s the document asked for
					}

					src.arm(f)
					got := catchPanic(func() { _, _ = f.res.Resolve(t.Context(), baseURL) })
					if got != f.value {
						t.Errorf("Resolve(%q) panicked with %#v, want the value %#v the fake raised, unchanged", baseURL, got, f.value)
					}
					if cached, ok := f.cache.Get(t.Context(), baseURL); ok != expired || cached != warm {
						t.Errorf("cache.Get(%q) after the panic = %p, %t, want %p, %t: the resolver leaves the entry as it was", baseURL, cached, ok, warm, expired)
					}

					// Without a deadline a stranded caller would wait forever.
					ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
					defer cancel()
					cat, err := f.res.Resolve(ctx, baseURL)
					if err != nil || cat == nil {
						t.Errorf("Resolve(%q) after the panic = %v, %v, want a catalog: the panicking fetch must not leave the base URL in flight", baseURL, cat, err)
					}
				})
			})
		}
	}
}

// TestCoalescedWaiterFetchesAgainWhenStarterPanics pins REQ-026 and REQ-071: a
// caller that joined a fetch which then panicked is not left waiting, and the
// panic is not its result. While its own context is live it fetches again and
// gets the result of that fetch, while the panic reaches only the caller that
// started the first fetch.
func TestCoalescedWaiterFetchesAgainWhenStarterPanics(t *testing.T) { // REQ-026, REQ-071
	for _, src := range panicSources {
		t.Run(src.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := startPanicFixture(t, 1, src.heldStatus)
				baseURL := f.p.baseURL()
				src.arm(f)

				// The deadline only bounds the test should caller 2 be stranded.
				ctx2, cancel2 := context.WithTimeout(t.Context(), time.Minute)
				defer cancel2()
				var (
					wg        sync.WaitGroup
					recovered any
					cat2      *discovery.ServiceCatalog
					err2      error
				)
				returned2 := make(chan struct{})
				wg.Go(func() {
					recovered = catchPanic(func() { _, _ = f.res.Resolve(t.Context(), baseURL) })
				})
				synctest.Wait() // caller 1's request is held at the server
				wg.Go(func() {
					defer close(returned2)
					cat2, err2 = f.res.Resolve(ctx2, baseURL)
				})
				synctest.Wait() // caller 2 waits for caller 1's fetch
				select {
				case <-returned2:
					t.Error("caller 2 returned while caller 1's fetch was held, want it waiting on that fetch")
				default:
				}
				if got := f.p.hits.Load(); got != 1 {
					t.Errorf("server saw %d requests while caller 1's fetch was held, want 1: caller 2 joins that fetch", got)
				}
				f.p.open() // caller 1's fetch ends in the panic
				wg.Wait()

				if recovered != f.value {
					t.Errorf("caller 1 panicked with %#v, want the value %#v the fake raised, unchanged", recovered, f.value)
				}
				if err2 != nil || cat2 == nil {
					t.Errorf("caller 2 = %v, %v, want a catalog: its own context is live, so it fetches again", cat2, err2)
				}
				if got := f.p.hits.Load(); got != 2 {
					t.Errorf("server answered %d requests, want 2: caller 2 sends a request of its own after the panic", got)
				}
				if cached, ok := f.cache.Get(t.Context(), baseURL); !ok || cached != cat2 {
					t.Errorf("cache.Get(%q) = %p, %t, want the catalog %p caller 2 fetched", baseURL, cached, ok, cat2)
				}
			})
		})
	}
}

// TestCoalescedWaiterWhoseContextEndsAsStarterPanicsDoesNotFetch pins REQ-026
// and REQ-071: a waiter woken because the fetch it joined panicked looks at its
// own context again before it fetches. When that context has ended by then,
// the waiter returns that context's error itself, not the empty result of the
// fetch that panicked, and sends no request.
func TestCoalescedWaiterWhoseContextEndsAsStarterPanicsDoesNotFetch(t *testing.T) { // REQ-026, REQ-071
	for _, src := range panicSources {
		t.Run(src.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := startPanicFixture(t, 1, src.heldStatus)
				baseURL := f.p.baseURL()
				src.arm(f)

				inner2, cancel2 := context.WithCancel(t.Context())
				defer cancel2()
				ctx2 := newLateContext(inner2)
				defer ctx2.open()
				var (
					wg        sync.WaitGroup
					recovered any
					cat2      *discovery.ServiceCatalog
					err2      error
				)
				wg.Go(func() {
					recovered = catchPanic(func() { _, _ = f.res.Resolve(t.Context(), baseURL) })
				})
				synctest.Wait() // caller 1's request is held at the server
				wg.Go(func() { cat2, err2 = f.res.Resolve(ctx2, baseURL) })
				synctest.Wait() // caller 2 waits for caller 1's fetch
				ctx2.hold()
				f.p.open()      // caller 1's fetch ends in the panic
				synctest.Wait() // caller 1 panicked and woke caller 2, whose next look at its context waits
				cancel2()
				ctx2.open()
				wg.Wait()

				if recovered != f.value {
					t.Errorf("caller 1 panicked with %#v, want the value %#v the fake raised, unchanged", recovered, f.value)
				}
				if err2 != context.Canceled || cat2 != nil { //nolint:errorlint // the waiter returns its context's error itself, wrapped in nothing
					t.Errorf("caller 2 = %v, %v, want nil and its own context's error %v itself: its context ended before it looked again", cat2, err2, context.Canceled)
				}
				if got := f.p.hits.Load(); got != 1 {
					t.Errorf("server answered %d requests, want 1: caller 2 sends none once its own context ended", got)
				}
			})
		})
	}
}
