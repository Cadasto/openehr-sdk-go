package transport

import (
	"log/slog"
	"net/http"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// config is the unexported configuration struct mutated through With*
// options. REQ-022 — public consumers do not construct this directly.
type config struct {
	httpClient *http.Client
	tokenSrc   auth.TokenSource
	userAgent  string

	specVersion       string
	sendCadastoHeader bool

	retry RetryPolicy

	callerAttribution       CallerAttribution
	callerAttributionHeader string

	logger *slog.Logger

	observer Observer
}

// Option mutates the transport configuration. Apply via transport.New.
type Option func(*config)

// WithHTTPClient injects the *http.Client used for outgoing requests
// per REQ-021. Required — there is no built-in default; transport.New
// returns ErrInvalidConfig when the option is omitted.
func WithHTTPClient(c *http.Client) Option {
	return func(cfg *config) { cfg.httpClient = c }
}

// WithTokenSource sets the client-default auth.TokenSource. Per-request
// overrides via auth.WithTokenSource(ctx, ts) take precedence (REQ-060,
// PROBE-064). Default is auth.AnonymousTokenSource (no Authorization
// header emitted).
func WithTokenSource(ts auth.TokenSource) Option {
	return func(cfg *config) { cfg.tokenSrc = ts }
}

// WithUserAgent sets the User-Agent header for outgoing requests.
// Empty omits any UA override and uses the http.Client's default.
func WithUserAgent(ua string) Option {
	return func(cfg *config) { cfg.userAgent = ua }
}

// WithSpecVersion pins the spec version emitted on the optional
// Cadasto-OpenEhr-Spec-Version header (REQ-051). Effective only when
// WithCadastoSpecVersionHeader(true) is also set.
func WithSpecVersion(v string) Option {
	return func(cfg *config) { cfg.specVersion = v }
}

// WithCadastoSpecVersionHeader toggles emission of the
// Cadasto-OpenEhr-Spec-Version header (REQ-051). Default off; turn on
// only when the catalog or deployment indicates a Cadasto backend.
func WithCadastoSpecVersionHeader(on bool) Option {
	return func(cfg *config) { cfg.sendCadastoHeader = on }
}

// WithRetry installs a retry policy (REQ-091). Default: no retries.
func WithRetry(p RetryPolicy) Option {
	return func(cfg *config) { cfg.retry = p }
}

// WithCallerAttribution attaches a client-default CallerAttribution
// emitted on every outgoing request (REQ-066). Per-request overrides
// via WithCallerAttributionCtx take precedence.
func WithCallerAttribution(a CallerAttribution) Option {
	return func(cfg *config) { cfg.callerAttribution = a }
}

// WithCallerAttributionHeader overrides the header name used to carry
// caller attribution. Default "X-Cadasto-Caller-Attribution" per
// REQ-066.
func WithCallerAttributionHeader(name string) Option {
	return func(cfg *config) { cfg.callerAttributionHeader = name }
}

// WithLogger sets the slog.Logger the transport uses for non-fatal
// diagnostics (TLS warnings, retry attempts). Default slog.Default().
func WithLogger(l *slog.Logger) Option {
	return func(cfg *config) { cfg.logger = l }
}

// WithObserver installs an Observer (REQ-098). The observer fires
// exactly once per logical Client.Do call after retries settle. A nil
// observer is treated as a no-op (safe to pass through configuration
// layers that don't know whether the consumer wants observability).
func WithObserver(o Observer) Option {
	return func(cfg *config) { cfg.observer = o }
}
