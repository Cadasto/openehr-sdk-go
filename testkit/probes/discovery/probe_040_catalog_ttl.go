// Package discoveryprobes hosts the cross-SDK conformance probes for
// the openEHR service-discovery layer. Each probe corresponds to a
// PROBE-NNN entry in specs/conformance.md and is implemented in both
// the Go and PHP SDKs against shared cassettes (REQ-080).
//
// Probes are plain Go functions returning (Result, error) and are
// designed to be invocable from:
//
//   - the SDK's own test suite (via TestProbeNNN);
//   - the conformance harness in `make conformance`;
//   - third-party consumers checking their integration.
//
// The probes deliberately avoid `testing.T` so they can run outside
// `go test`.
package discoveryprobes

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// Result captures the outcome of a probe invocation. Status is
// "pass" / "fail" / "skip"; Detail is human-readable supplementary
// text for failures or skips. Same shape as the serialize / versioned
// probe families.
type Result struct {
	Probe  string
	Status string
	Detail string
}

// Probe040CatalogTTL implements PROBE-040: two successive resolves
// of the same issuer within the catalog's declared TTL window MUST
// produce exactly one discovery fetch. Cache hit on the second
// resolve is the load-bearing guarantee — without it every client
// construction pays the discovery RTT.
//
// `cassetteBody` is the SMART configuration JSON the upstream server
// will return on every request — typically the vendored
// `testkit/cassettes/its_rest/discovery/smart-configuration.json`
// content read by the caller. The server replies with
// `Cache-Control: max-age=300` so the SDK's cache honours a real TTL.
//
// The probe spins up a small in-process server and counts inbound
// requests; second resolve hitting the wire is the failure mode.
func Probe040CatalogTTL(ctx context.Context, cassetteBody []byte) (Result, error) {
	r := Result{Probe: "PROBE-040"}
	if len(cassetteBody) == 0 {
		return r, fmt.Errorf("PROBE-040: cassetteBody is empty")
	}
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=300")
		_, _ = w.Write(cassetteBody)
	}))
	defer srv.Close()

	res, err := discovery.NewResolver(
		discovery.NewMemoryCache(),
		discovery.WithHTTPClient(srv.Client()),
		discovery.WithAllowInsecure(),
	)
	if err != nil {
		return r, fmt.Errorf("PROBE-040: build resolver: %w", err)
	}
	if _, err := res.Resolve(ctx, srv.URL); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("first Resolve failed: %v", err)
		return r, nil
	}
	if _, err := res.Resolve(ctx, srv.URL); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("second Resolve failed: %v", err)
		return r, nil
	}
	got := atomic.LoadInt32(&hits)
	if got != 1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("expected 1 discovery fetch within TTL window, got %d", got)
		return r, nil
	}
	r.Status = "pass"
	r.Detail = "cache honoured TTL: 2 resolves → 1 fetch"
	return r, nil
}
