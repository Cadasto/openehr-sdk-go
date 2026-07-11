package admin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// ErrInvalidPath is returned by Live / Ready when a [WithLivePath] or
// [WithReadyPath] override is empty or does not start with '/'.
// Probes target the deployment origin and must be expressed as an
// absolute path on that origin.
var ErrInvalidPath = errors.New("admin: invalid probe path")

// Default probe paths (REQ-083). Override per call via WithLivePath
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
// response.
//
// Status-code mapping (errors.Is-compatible):
//   - 401 → [transport.ErrUnauthorized]
//   - 403 → [transport.ErrForbidden]
//   - 404 → [transport.ErrNotFound]
//   - 5xx → [transport.ErrServerError]
//
// Other non-2xx codes (e.g. 400, 405, 408, 429) surface as a plain
// formatted error with no sentinel — callers MUST NOT rely on
// errors.Is matching outside the mapped set. Network failures from
// the underlying http.Client (connection refused, TLS errors, context
// cancellation) are wrapped with the probe URL via %w.
//
// Probes borrow [transport]'s sentinel taxonomy (REQ-093) for the
// mapped codes above, but bypass [transport.Client.Do] — no WireError
// envelope is decoded, no OTel spans recorded, no retries applied.
// Use [openehr/client/admin] for retry-aware admin operations.
//
// The request is issued at the deployment origin derived from the
// openEHR REST service catalog entry (scheme + host) — the openEHR
// REST API path prefix is stripped so probes target deployment-level
// endpoints, not API-scoped ones.
//
// No Authorization header is attached by this code: health endpoints
// are public per the REQ-083 contract. (An injected
// http.RoundTripper may still inject headers.) Deployments that gate
// /health behind auth should not use this surface.
//
// Returns [ErrInvalidPath] when the WithLivePath override is empty
// or does not start with '/'.
func Live(ctx context.Context, c *transport.Client, opts ...LiveOption) error {
	cfg := liveConfig{path: DefaultLivePath}
	for _, o := range opts {
		o(&cfg)
	}
	return probe(ctx, c, cfg.path)
}

// Ready probes the deployment readiness endpoint. Same error contract
// as [Live] — see that doc for the status mapping, the deliberate
// transport.Do bypass, and the [ErrInvalidPath] guard on the
// WithReadyPath override.
func Ready(ctx context.Context, c *transport.Client, opts ...ReadyOption) error {
	cfg := readyConfig{path: DefaultReadyPath}
	for _, o := range opts {
		o(&cfg)
	}
	return probe(ctx, c, cfg.path)
}

func probe(ctx context.Context, c *transport.Client, path string) error {
	if path == "" || !strings.HasPrefix(path, "/") {
		return fmt.Errorf("%w: %q (must be a non-empty absolute path starting with '/')", ErrInvalidPath, path)
	}
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
		return fmt.Errorf("admin: probe %s: build request: %w", target, err)
	}
	req.Header.Set("Accept", "application/json")

	hc := c.HTTPClient()
	if hc == nil {
		return fmt.Errorf("admin: probe: %w: HTTPClient", transport.ErrInvalidConfig)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("admin: probe GET %s: %w", target, err)
	}
	defer func() {
		// Drain remaining body so the underlying connection can be
		// reused — probes run in tight loops (k8s liveness polling,
		// CDR benchmarks) where every dropped keep-alive becomes a
		// re-handshake cost.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	sentinel := statusToSentinel(resp.StatusCode)
	if sentinel == nil {
		return fmt.Errorf("admin: probe %s: unexpected status %d", path, resp.StatusCode)
	}
	return fmt.Errorf("admin: probe %s status=%d: %w", path, resp.StatusCode, sentinel)
}

// statusToSentinel maps the subset of status codes this probe
// translates to [transport] sentinels. Returns nil for codes outside
// the mapped set (e.g. 400, 408, 429); callers surface those as a
// plain formatted error without a sentinel.
//
// The set is intentionally narrower than transport.statusToSentinel
// (which also handles 409/412/428 for openEHR REST envelopes) —
// probes are a deployment-level concern and do not see those codes.
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
