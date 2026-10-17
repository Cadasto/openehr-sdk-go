package transport

import (
	"context"
	"maps"
	"time"
)

// Observation is the per-request record delivered to a registered
// Observer once the logical call has settled (after any retries).
// It complements the OTel span emitted by Client.Do; use it when
// integrating with non-OTel sinks such as benchmark statistics, custom
// metrics, or test assertions.
type Observation struct {
	// Method is the HTTP method (canonical upper-case, e.g. "POST").
	// Empty when Client.Do was given a nil Request.
	Method string
	// Route is the path template used for OTel span naming (e.g.
	// "/ehr/{ehr_id}/composition"). Falls back to Path when the
	// caller did not supply a Route. Empty when Client.Do was given a
	// nil Request.
	Route string
	// URL is the request URL with any userinfo (user name and password)
	// removed. Its path and query are kept as sent, so they can carry
	// caller-supplied identifiers and any query parameter the service
	// base URL holds. Empty when the call was refused before the URL was
	// built.
	URL string
	// StatusCode is the final HTTP status returned by the server, or 0
	// if the call terminated before a response (network error,
	// ctx-cancellation, token failure, or a refusal before any attempt).
	// It is set when the status and headers arrived but the body could
	// not be read.
	StatusCode int
	// Duration is the total wall-clock time including all retries and
	// backoff sleeps, that is, the latency of the logical call.
	Duration time.Duration
	// Attempts is the number of times Client.Do tried the request: each
	// retry and the replay after a re-authentication count, and so does an
	// attempt that failed before anything reached the server (a request
	// that cannot be built, a token that could not be obtained). It is 1
	// when retries were disabled or the first attempt succeeded, and 0 only
	// when Client.Do refused the call before any attempt: a nil Request, a
	// context already ended, a service missing from the catalog, or a
	// path that fails validation.
	Attempts int
	// Err is nil on a 2xx outcome whose body was read, and is, or wraps, a
	// *BodyReadError on a 2xx whose body could not be read. For non-2xx
	// outcomes it is, or wraps, a *WireError carrying the typed sentinel;
	// when the body could not be read, Err also wraps the read failure
	// beside that *WireError. For pre-response failures it is the wrapped
	// underlying error.
	Err error
	// Tags carries per-call observer tags attached via
	// WithObservationTag. It is read-only; observers must not mutate it.
	Tags map[string]any
}

// Observer receives one Observation per logical request, independent
// of OTel. Implementations must be safe for concurrent invocation: the
// transport calls OnRequest from the goroutine that issued Client.Do,
// but a single Observer may be shared across many concurrent calls.
//
// A panicking Observer does not break the request lifecycle: the
// transport recovers the panic and logs it through the configured
// slog.Logger.
type Observer interface {
	OnRequest(Observation)
}

// observationTagsKey is the unexported context-key type used to attach
// per-call observer tags. The pattern mirrors auth.WithTokenSource.
type observationTagsKey struct{}

// WithObservationTag returns a context carrying observer tag (k,v).
// Multiple calls accumulate; the same key is overwritten by the latest
// value. The returned context is safe to pass through goroutine
// boundaries.
//
// Tags reach the Observer through Observation.Tags as a shallow-cloned
// map. The transport copies them before delivery so observers cannot
// mutate the caller's request context.
func WithObservationTag(ctx context.Context, k string, v any) context.Context {
	if k == "" {
		return ctx
	}
	existing, _ := ctx.Value(observationTagsKey{}).(map[string]any)
	next := make(map[string]any, len(existing)+1)
	maps.Copy(next, existing)
	next[k] = v
	return context.WithValue(ctx, observationTagsKey{}, next)
}

// observationTagsFromContext returns a defensive copy of the tag map
// attached to ctx, or nil when no tags were set. The copy avoids
// observer mutation of the caller's context value.
func observationTagsFromContext(ctx context.Context) map[string]any {
	src, _ := ctx.Value(observationTagsKey{}).(map[string]any)
	if len(src) == 0 {
		return nil
	}
	return maps.Clone(src)
}
