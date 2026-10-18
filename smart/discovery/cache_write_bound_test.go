package discovery_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// cacheWriteBound is how long the resolver lets a Cache write run.
const cacheWriteBound = 10 * time.Second

// hangingCache is a Cache whose Put and Invalidate honour their context and
// otherwise never return until release is closed, as a network cache that
// cannot reach its server does. It records the deadline of the last write's
// context.
type hangingCache struct {
	*discovery.MemoryCache
	release chan struct{}

	mu       sync.Mutex
	deadline time.Time
	hasDL    bool
}

func (c *hangingCache) hang(ctx context.Context) error {
	c.mu.Lock()
	c.deadline, c.hasDL = ctx.Deadline()
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.release:
		return nil
	}
}

func (c *hangingCache) Put(ctx context.Context, _ string, _ *discovery.ServiceCatalog) error {
	return c.hang(ctx)
}

func (c *hangingCache) Invalidate(ctx context.Context, _ string) error {
	return c.hang(ctx)
}

// TestREQ071_HangingCacheWriteIsBounded pins the bound on the cache write
// that records a fetch's result: a Put after a resolution, or an Invalidate
// after a failed fetch, that waits until its context ends gets a context
// that ends cacheWriteBound after the write starts, not at the starter's
// own, earlier deadline. When it ends, the write's error is logged as a
// cache failure, and the starter and a caller joined to the same fetch are
// both released with the fetch's result.
func TestREQ071_HangingCacheWriteIsBounded(t *testing.T) { // REQ-071, REQ-026
	tests := []struct {
		name    string
		status  int
		wantMsg string
		wantErr bool
	}{
		{name: "Put after a resolution", status: http.StatusOK, wantMsg: "cache put failed"},
		{name: "Invalidate after a failed fetch", status: http.StatusNotFound, wantMsg: "cache invalidate failed", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(tc.status)
					_, _ = io.WriteString(w, smartDocument("", ""))
				}))
				cache := &hangingCache{MemoryCache: discovery.NewMemoryCache(), release: make(chan struct{})}
				var logs bytes.Buffer
				res, err := discovery.NewResolver(cache, discovery.WithHTTPClient(srv.Client()), discovery.WithAllowInsecure(),
					discovery.WithLogger(slog.New(slog.NewTextHandler(&logs, nil))))
				if err != nil {
					t.Fatalf("NewResolver: %v", err)
				}
				base := srv.URL + platformPath

				// The starter's own deadline comes before the bound, and must
				// not end the write.
				starterCtx, cancel := context.WithTimeout(t.Context(), cacheWriteBound/2)
				defer cancel()
				type result struct {
					cat *discovery.ServiceCatalog
					err error
				}
				starter, waiter := make(chan result, 1), make(chan result, 1)
				start := time.Now()
				go func() { cat, err := res.Resolve(starterCtx, base); starter <- result{cat, err} }()
				synctest.Wait() // the starter is inside the cache write
				go func() { cat, err := res.Resolve(t.Context(), base); waiter <- result{cat, err} }()
				synctest.Wait() // the waiter has joined the starter's fetch

				time.Sleep(cacheWriteBound - time.Nanosecond)
				synctest.Wait()
				select {
				case r := <-starter:
					t.Fatalf("starter returned (%v, %v) after %v, before the write's bound of %v", r.cat, r.err, time.Since(start), cacheWriteBound)
				default:
				}
				time.Sleep(time.Nanosecond)
				synctest.Wait()

				var got [2]result
				for i, ch := range []chan result{starter, waiter} {
					select {
					case got[i] = <-ch:
					default:
						close(cache.release)
						t.Fatalf("%s still blocked %v after the cache write began, want it released at the write's bound of %v",
							[]string{"starter", "waiter"}[i], time.Since(start), cacheWriteBound)
					}
				}
				for i, r := range got {
					who := []string{"starter", "waiter"}[i]
					if gotErr := r.err != nil; gotErr != tc.wantErr || (!tc.wantErr && r.cat == nil) {
						t.Errorf("%s Resolve = (%v, %v), want the fetch's result (error: %v)", who, r.cat, r.err, tc.wantErr)
					}
				}
				cache.mu.Lock()
				deadline, hasDL := cache.deadline, cache.hasDL
				cache.mu.Unlock()
				if !hasDL || !deadline.Equal(start.Add(cacheWriteBound)) {
					t.Errorf("cache write context deadline = %v (set %v), want %v, the bound from when the write began", deadline, hasDL, start.Add(cacheWriteBound))
				}
				if out := logs.String(); !strings.Contains(out, tc.wantMsg) || !strings.Contains(out, context.DeadlineExceeded.Error()) {
					t.Errorf("log = %q, want a %q warning naming the expired write", out, tc.wantMsg)
				}
			})
		})
	}
}
