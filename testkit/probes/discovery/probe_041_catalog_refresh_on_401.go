package discoveryprobes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// Probe041CatalogRefreshOn401 implements PROBE-041 in its
// discovery-layer scope: a `Refresh` triggered by a stale catalog
// MUST produce exactly one fetch and surface a typed error when the
// upstream rejects with `401 Unauthorized`. The full transport-
// driven retry-on-401 leg (REQ-071 bullet 3 — transport sees 401,
// asks discovery to refresh, retries once, second 401 → typed
// `transport.ErrUnauthorized`) is asserted at the discovery boundary
// here; the transport-level half lives under
// `testkit/probes/auth/` once that probe range is wired.
//
// The probe primes the cache via a successful Resolve, then flips
// the upstream to 401 and calls Refresh. The Refresh MUST:
//
//   - issue exactly one fetch (the refresh itself);
//   - NOT retry beyond that — DiscoveryError carries the failure;
//   - return a typed `*discovery.DiscoveryError` whose Reason is
//     `ReasonFetchFailed`, so `errors.As` callers can act on it.
func Probe041CatalogRefreshOn401(ctx context.Context, cassetteBody []byte) (Result, error) {
	r := Result{Probe: "PROBE-041"}
	if len(cassetteBody) == 0 {
		return r, fmt.Errorf("PROBE-041: cassetteBody is empty")
	}
	var (
		hits      int32
		mode      atomic.Int32 // 0 = serve cassette; 1 = 401 reject
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		if mode.Load() == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"unauthorized","code":"UNAUTHORIZED"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = w.Write(cassetteBody)
	}))
	defer srv.Close()

	res, err := discovery.NewResolver(
		discovery.NewMemoryCache(),
		discovery.WithHTTPClient(srv.Client()),
		discovery.WithAllowInsecure(),
	)
	if err != nil {
		return r, fmt.Errorf("PROBE-041: build resolver: %w", err)
	}
	if _, err := res.Resolve(ctx, srv.URL); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("priming Resolve failed: %v", err)
		return r, nil
	}
	primingHits := atomic.LoadInt32(&hits)
	if primingHits != 1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("priming Resolve produced %d fetches; want exactly 1", primingHits)
		return r, nil
	}

	// Rotate the upstream — 401 on every subsequent request.
	mode.Store(1)

	_, refreshErr := res.Refresh(ctx, srv.URL)
	if refreshErr == nil {
		r.Status = "fail"
		r.Detail = "Refresh against a 401 backend MUST surface an error, got nil"
		return r, nil
	}
	var derr *discovery.DiscoveryError
	if !errors.As(refreshErr, &derr) {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Refresh error = %v; want *discovery.DiscoveryError", refreshErr)
		return r, nil
	}
	if derr.Reason != discovery.ReasonFetchFailed {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("DiscoveryError.Reason = %q; want %q", derr.Reason, discovery.ReasonFetchFailed)
		return r, nil
	}
	refreshHits := atomic.LoadInt32(&hits) - primingHits
	if refreshHits != 1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Refresh issued %d fetches after the priming Resolve; want exactly 1", refreshHits)
		return r, nil
	}
	r.Status = "pass"
	r.Detail = "Refresh against 401 issued exactly 1 fetch and returned a typed DiscoveryError(fetch_failed)"
	return r, nil
}
