// Package authprobes hosts the openEHR conformance probes for authentication
// and token-refresh behaviour. Each probe corresponds to a PROBE-NNN entry in
// docs/specifications/conformance.md.
//
// Probes are plain Go functions returning (Result, error) and are designed to
// be invocable from:
//
//   - the SDK's own test suite (via TestProbeNNN);
//   - the conformance harness in `make conformance`;
//   - third-party consumers checking their integration.
//
// The probes deliberately avoid testing.T so they can run outside go test.
package authprobes

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Result captures the outcome of a probe invocation.  Status is
// "pass" / "fail" / "skip"; Detail is human-readable supplementary text.
// Same shape as the other probe families.
type Result struct {
	Probe  string
	Status string
	Detail string
}

// refreshingTokenSource is a test-double TokenSource + Reauther used by
// PROBE-007. Before Reauth is called it vends oldToken; after Reauth it
// vends newToken.  Reauth calls are counted so the probe can assert
// exactly-once semantics.
type refreshingTokenSource struct {
	reauths  atomic.Int32
	current  atomic.Value // stores string
	oldToken string
	newToken string
}

func newRefreshingTokenSource(old, fresh string) *refreshingTokenSource {
	r := &refreshingTokenSource{oldToken: old, newToken: fresh}
	r.current.Store(old)
	return r
}

func (r *refreshingTokenSource) Token(_ context.Context) (auth.Token, error) {
	return auth.Token{Value: r.current.Load().(string), Type: "Bearer"}, nil
}

func (r *refreshingTokenSource) Reauth(_ context.Context) error {
	r.reauths.Add(1)
	r.current.Store(r.newToken)
	return nil
}

// Probe007TransportTokenRefresh implements PROBE-007 (transport half):
// an expired / rejected access token is refreshed silently before the
// next request when an auth.Reauther is registered via
// transport.WithReauthOn401 (REQ-063, REQ-071 bullet 3).
//
// Scenario:
//   - The test server returns 401 on the first call (simulating token
//     expiry on the wire).
//   - The Reauther swaps the token source to a fresh bearer.
//   - The transport retries exactly once; the second call carries the new
//     bearer and receives 200.
//
// Pass conditions (all must hold):
//  1. The final error is nil (200 succeeds).
//  2. The upstream received exactly 2 requests.
//  3. The first request carried the old bearer.
//  4. The second request carried the new (refreshed) bearer.
//  5. Reauth was called exactly once.
//
// Note: PROBE-007's broader scope (proactive expiry-based refresh via
// TokenSource.Token before the request is issued) is covered by
// auth/smart unit tests.  This probe asserts the transport-layer
// safety-net path only.  The full PROBE-007 probe suite (sandbox +
// cassette + live) lands in Phase 5.
func Probe007TransportTokenRefresh(ctx context.Context) (Result, error) { // PROBE-007
	r := Result{Probe: "PROBE-007"}

	var (
		hits    atomic.Int32
		bearers [2]atomic.Value
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		n := int(hits.Add(1))
		if n <= 2 {
			bearers[n-1].Store(req.Header.Get("Authorization"))
		}
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://probe007.example",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		return r, fmt.Errorf("PROBE-007: build catalog: %w", err)
	}

	ts := newRefreshingTokenSource("old-bearer", "fresh-bearer")
	c, err := transport.New(
		cat,
		transport.WithHTTPClient(srv.Client()),
		transport.WithTokenSource(ts),
		transport.WithReauthOn401(ts),
	)
	if err != nil {
		return r, fmt.Errorf("PROBE-007: build client: %w", err)
	}

	resp, doErr := c.Do(ctx, &transport.Request{Path: "/ehr"})
	if doErr != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Do returned error after reauth: %v", doErr)
		return r, nil
	}
	if resp.StatusCode != http.StatusOK {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("StatusCode = %d, want 200", resp.StatusCode)
		return r, nil
	}

	upstreamCalls := hits.Load()
	if upstreamCalls != 2 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("upstream received %d calls; want exactly 2", upstreamCalls)
		return r, nil
	}

	first, _ := bearers[0].Load().(string)
	second, _ := bearers[1].Load().(string)
	if first != "Bearer old-bearer" {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("first request Authorization = %q; want Bearer old-bearer", first)
		return r, nil
	}
	if second != "Bearer fresh-bearer" {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("second request Authorization = %q; want Bearer fresh-bearer", second)
		return r, nil
	}

	if n := ts.reauths.Load(); n != 1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Reauth called %d times; want exactly 1", n)
		return r, nil
	}

	r.Status = "pass"
	r.Detail = "wire 401 triggered exactly one Reauth; retry carried refreshed bearer; request succeeded"
	return r, nil
}
