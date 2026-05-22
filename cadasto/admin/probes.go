package admin

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Default probe paths (SDK-GAP-07). Override per call via WithLivePath
// or WithReadyPath when a deployment serves health on different paths
// (e.g. "/healthz").
const (
	DefaultLivePath  = "/health/live"
	DefaultReadyPath = "/health/ready"
)

// LiveOption configures a Live call. Implemented by With* helpers.
type LiveOption func(*liveConfig)

// ReadyOption configures a Ready call. Implemented by With* helpers.
type ReadyOption func(*readyConfig)

type liveConfig struct {
	path string
}

type readyConfig struct {
	path string
}

// WithLivePath overrides the path used by Live. Default DefaultLivePath.
// The path is interpreted at the deployment origin (scheme + host of
// the openEHR REST service entry); it does NOT inherit any API prefix.
func WithLivePath(p string) LiveOption {
	return func(c *liveConfig) { c.path = p }
}

// WithReadyPath overrides the path used by Ready. Default
// DefaultReadyPath. The path is interpreted at the deployment origin
// (scheme + host of the openEHR REST service entry); it does NOT
// inherit any API prefix.
func WithReadyPath(p string) ReadyOption {
	return func(c *readyConfig) { c.path = p }
}

// Live probes the deployment liveness endpoint. Returns nil on any 2xx
// response. On 4xx/5xx returns an error matching the appropriate
// transport sentinel via errors.Is (ErrNotFound on 404, ErrUnauthorized
// on 401, ErrForbidden on 403, ErrServerError on 5xx). On
// transport/network failure returns the wrapped underlying error.
//
// The request is issued at the deployment origin derived from the
// openEHR REST service catalog entry (scheme + host) — the openEHR
// REST API path prefix is stripped so probes target deployment-level
// endpoints, not API-scoped ones.
//
// No Authorization header is attached: health endpoints are public
// per the SDK-GAP-07 contract. Deployments that gate /health behind
// auth should not use this surface.
func Live(ctx context.Context, c *transport.Client, opts ...LiveOption) error {
	cfg := liveConfig{path: DefaultLivePath}
	for _, o := range opts {
		o(&cfg)
	}
	return probe(ctx, c, cfg.path)
}

// Ready probes the deployment readiness endpoint. Same error contract
// as Live; see that doc for details.
func Ready(ctx context.Context, c *transport.Client, opts ...ReadyOption) error {
	cfg := readyConfig{path: DefaultReadyPath}
	for _, o := range opts {
		o(&cfg)
	}
	return probe(ctx, c, cfg.path)
}

func probe(ctx context.Context, c *transport.Client, path string) error {
	entry, ok := c.Catalog().OpenEHRRest()
	if !ok {
		return fmt.Errorf("admin: probe: %w: %s", transport.ErrServiceUnavailable, discovery.ServiceIDOpenEHRRest)
	}
	if entry.BaseURL == nil {
		return fmt.Errorf("admin: probe: %w: %s has nil BaseURL", transport.ErrInvalidConfig, discovery.ServiceIDOpenEHRRest)
	}
	target := &url.URL{
		Scheme: entry.BaseURL.Scheme,
		Host:   entry.BaseURL.Host,
		Path:   path,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return fmt.Errorf("admin: probe %s: build request: %w", path, err)
	}
	req.Header.Set("Accept", "application/json")

	hc := c.HTTPClient()
	if hc == nil {
		return fmt.Errorf("admin: probe: %w: HTTPClient", transport.ErrInvalidConfig)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("admin: probe %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	sentinel := statusToSentinel(resp.StatusCode)
	if sentinel == nil {
		return fmt.Errorf("admin: probe %s: unexpected status %d", path, resp.StatusCode)
	}
	return fmt.Errorf("admin: probe %s status=%d: %w", path, resp.StatusCode, sentinel)
}

// statusToSentinel mirrors transport.statusToSentinel for non-2xx
// codes the probe cares about. We re-derive locally (rather than
// reusing the unexported transport function) so cadasto/admin does
// not need a new transport surface beyond HTTPClient(). REQ-093.
func statusToSentinel(s int) error {
	switch s {
	case http.StatusNotFound:
		return transport.ErrNotFound
	case http.StatusUnauthorized:
		return transport.ErrUnauthorized
	case http.StatusForbidden:
		return transport.ErrForbidden
	}
	if s >= 500 {
		return transport.ErrServerError
	}
	return nil
}
