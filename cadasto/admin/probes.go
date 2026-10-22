package admin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// ErrInvalidPath is returned by Live, Ready and ReadyBody when a
// [WithLivePath] or [WithReadyPath] override is empty or does not start
// with '/'. Probes target the deployment origin and must be expressed as
// an absolute path on that origin.
var ErrInvalidPath = errors.New("admin: invalid probe path")

// Default probe paths. Override per call via WithLivePath
// or WithReadyPath when a deployment serves health on different paths
// (e.g. "/healthz").
const (
	DefaultLivePath  = "/health/live"
	DefaultReadyPath = "/health/ready"
)

// DefaultMaxReadyBody is the cap on the bytes ReadyBody reads from a
// readiness body when WithMaxReadyBody is not set (or set to 0): 1 MiB.
const DefaultMaxReadyBody int64 = 1 << 20

// LiveOption configures a Live call. Implemented by With* helpers.
type LiveOption func(*liveConfig)

// ReadyOption configures a Ready or ReadyBody call. Implemented by With*
// helpers.
type ReadyOption func(*readyConfig)

type liveConfig struct {
	path string
}

type readyConfig struct {
	path    string
	maxBody int64
}

// WithLivePath overrides the path used by Live. Default DefaultLivePath.
// The path is interpreted at the deployment origin (scheme + host of
// the openEHR REST service entry); it does NOT inherit any API prefix.
func WithLivePath(p string) LiveOption {
	return func(c *liveConfig) { c.path = p }
}

// WithReadyPath overrides the path used by Ready and ReadyBody. Default
// DefaultReadyPath. The path is interpreted at the deployment origin
// (scheme + host of the openEHR REST service entry); it does NOT
// inherit any API prefix.
func WithReadyPath(p string) ReadyOption {
	return func(c *readyConfig) { c.path = p }
}

// WithMaxReadyBody caps the bytes ReadyBody reads from a 2xx readiness
// body, guarding against memory exhaustion from a misbehaving or hostile
// server. It takes the values [transport.WithMaxResponseBody] takes: 0
// selects DefaultMaxReadyBody, a positive value sets the limit and a
// negative value disables it (unbounded read). Ready reads no body it
// returns, so the option does not change it.
func WithMaxReadyBody(n int64) ReadyOption {
	return func(c *readyConfig) { c.maxBody = n }
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
// formatted error with no sentinel, so do not rely on errors.Is
// matching outside the mapped set. Network failures from the
// underlying http.Client (connection refused, TLS errors, context
// cancellation) are wrapped with the probe URL via %w.
//
// Probes reuse [transport]'s sentinel errors for the mapped codes
// above but bypass [transport.Client.Do]: no WireError envelope is
// decoded, no OTel spans are recorded and no retries are applied.
// Use [openehr/client/admin] for retry-aware admin operations.
//
// The request is issued at the deployment origin derived from the
// openEHR REST service catalog entry (scheme + host). The openEHR
// REST API path prefix is stripped so probes target deployment-level
// endpoints rather than API-scoped ones.
//
// No Authorization header is attached by this code, because health
// endpoints are treated as public. (An injected
// http.RoundTripper may still inject headers.) Deployments that gate
// /health behind auth should not use this surface.
//
// Returns [ErrInvalidPath] when the WithLivePath override is empty
// or does not start with '/'.
func Live(ctx context.Context, c *transport.Client, opts ...LiveOption) error {
	cfg := liveConfig{path: DefaultLivePath}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	return probe(ctx, c, cfg.path)
}

// Ready probes the deployment readiness endpoint. It has the same error
// contract as [Live]; see that doc for the status mapping, the
// transport.Do bypass and the [ErrInvalidPath] guard on the
// WithReadyPath override. Ready drains and discards the response body;
// [ReadyBody] returns it.
func Ready(ctx context.Context, c *transport.Client, opts ...ReadyOption) error {
	return probe(ctx, c, newReadyConfig(opts).path)
}

// ReadyBody probes the deployment readiness endpoint as [Ready] does and
// returns the body of a 2xx response, such as a document that names the
// service and its version. It sends the same request, takes the same
// options and has the same error contract as Ready. A status that is not
// 2xx returns nil bytes beside the error Ready returns for it.
//
// The read is capped at [DefaultMaxReadyBody] bytes unless
// [WithMaxReadyBody] sets another limit. A 2xx body over the limit fails
// with an error that matches [transport.ErrResponseTooLarge] and names
// the limit. Any other read failure, such as [io.ErrUnexpectedEOF] for a
// connection cut mid-body, is reached by unwrapping the error. Both
// return nil bytes, because a partial body is not the document. The
// error text names the probe path and the status, but never a body
// byte, a header value or the read failure's own text, which can quote
// bytes the server sent.
func ReadyBody(ctx context.Context, c *transport.Client, opts ...ReadyOption) ([]byte, error) {
	cfg := newReadyConfig(opts)
	resp, err := send(ctx, c, cfg.path)
	if err != nil {
		return nil, err
	}
	// Closed without draining: a body read to its end leaves nothing to
	// drain, and the rest of a body over the limit is not worth reading.
	defer func() { _ = resp.Body.Close() }()
	return readReadyBody(resp.Body, cfg.path, resp.StatusCode, cfg.maxBody)
}

func newReadyConfig(opts []ReadyOption) readyConfig {
	cfg := readyConfig{path: DefaultReadyPath}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	return cfg
}

// probe sends the GET of a Live or Ready call and maps its status. It
// drains and discards the body of a 2xx.
func probe(ctx context.Context, c *transport.Client, path string) error {
	resp, err := send(ctx, c, path)
	if err != nil {
		return err
	}
	drainAndClose(resp)
	return nil
}

// send issues the probe GET for path at the origin of the openEHR REST
// service entry. A 2xx returns the open response, whose body the caller
// reads and closes. Any other status returns the error statusToSentinel
// gives it, with the body already drained and closed.
func send(ctx context.Context, c *transport.Client, path string) (*http.Response, error) {
	if path == "" || !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("%w: %q (must be a non-empty absolute path starting with '/')", ErrInvalidPath, path)
	}
	// REQ-025: this leaf deliberately bypasses transport.Do (see the Live
	// doc), so Do's central nil-Client guard never runs here — the probe
	// carries its own.
	if c == nil {
		return nil, fmt.Errorf("admin: probe: %w: nil Client", transport.ErrInvalidConfig)
	}
	entry, ok := c.Catalog().OpenEHRRest()
	if !ok {
		return nil, fmt.Errorf("admin: probe: %w: %s", transport.ErrServiceUnavailable, discovery.ServiceIDOpenEHRRest)
	}
	if entry.BaseURL == nil {
		return nil, fmt.Errorf("admin: probe: %w: %s has nil BaseURL", transport.ErrInvalidConfig, discovery.ServiceIDOpenEHRRest)
	}
	target := &url.URL{
		Scheme: entry.BaseURL.Scheme,
		Host:   entry.BaseURL.Host,
		Path:   path,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("admin: probe %s: build request: %w", target, err)
	}
	req.Header.Set("Accept", "application/json")

	hc := c.HTTPClient()
	if hc == nil {
		return nil, fmt.Errorf("admin: probe: %w: HTTPClient", transport.ErrInvalidConfig)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("admin: probe GET %s: %w", target, err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	drainAndClose(resp)
	sentinel := statusToSentinel(resp.StatusCode)
	if sentinel == nil {
		return nil, fmt.Errorf("admin: probe %s: unexpected status %d", path, resp.StatusCode)
	}
	return nil, fmt.Errorf("admin: probe %s status=%d: %w", path, resp.StatusCode, sentinel)
}

// drainAndClose reads what is left of the response body and closes it,
// so the underlying connection can be reused: probes run in tight loops
// (k8s liveness polling, CDR benchmarks) where every dropped keep-alive
// becomes a re-handshake cost.
func drainAndClose(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// readReadyBody reads a 2xx readiness body under limit, where 0 selects
// DefaultMaxReadyBody and a negative value reads without a limit. It
// returns no bytes when the read fails or the body is over the limit.
func readReadyBody(body io.Reader, path string, status int, limit int64) ([]byte, error) {
	if limit == 0 {
		limit = DefaultMaxReadyBody
	}
	// The reader stops one byte past the limit, so a body over it is
	// detected rather than cut short. No body can be longer than
	// math.MaxInt64 bytes, and the extra byte would overflow that limit
	// into a negative count that reads nothing, so it gets no reader and
	// no check (as in transport's bounded read).
	bounded := limit > 0 && limit < math.MaxInt64
	r := body
	if bounded {
		r = io.LimitReader(body, limit+1)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, &bodyReadError{path: path, status: status, cause: err}
	}
	if bounded && int64(len(b)) > limit {
		return nil, &bodyReadError{path: path, status: status, limit: limit, cause: transport.ErrResponseTooLarge}
	}
	return b, nil
}

// bodyReadError reports a 2xx readiness body that could not be read. Its
// text names the probe path, the status and, for a body over the limit,
// the limit, which is the caller's own setting. It never carries the
// read failure's own text, which can quote bytes the server sent: net/http
// quotes a malformed chunked trailer line in it. Unwrap returns the read
// failure, so errors.Is still reaches it.
type bodyReadError struct {
	path   string
	status int
	limit  int64 // the limit the body went over; 0 for any other read failure
	cause  error
}

func (e *bodyReadError) Error() string {
	if e.limit > 0 {
		return fmt.Sprintf("admin: probe %s status=%d: read body: body exceeds limit of %d bytes", e.path, e.status, e.limit)
	}
	return fmt.Sprintf("admin: probe %s status=%d: read body: body could not be read", e.path, e.status)
}

func (e *bodyReadError) Unwrap() error { return e.cause }

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
