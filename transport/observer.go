package transport

import (
	"context"
	"maps"
	"time"
)

// Observation is the per-request record delivered to a registered
// Observer once the logical call has settled (after any retries).
// It is the additive observability counterpart to the OTel span emitted
// by Client.Do (REQ-090); use it when integrating with non-OTel sinks
// such as benchmark statistics, custom metrics, or test assertions
// (REQ-098).
type Observation struct {
	// Method is the HTTP method (canonical upper-case, e.g. "POST").
	Method string
	// Route is the path template used for OTel span naming (e.g.
	// "/ehr/{ehr_id}/composition"). Falls back to Path when the
	// caller did not supply a Route.
	Route string
	// URL is the sanitised request URL per REQ-090 (query string
	// stripped of secrets; path preserved).
	URL string
	// StatusCode is the final HTTP status returned by the server, or 0
	// if the call terminated before a response (network error,
	// ctx-cancellation, token failure).
	StatusCode int
	// Duration is the total wall-clock time including all retries and
	// backoff sleeps — the "logical call" latency.
	Duration time.Duration
	// Attempts is the number of HTTP attempts actually issued; 1 when
	// retries were disabled or the first attempt succeeded.
	Attempts int
	// Err is nil on 2xx outcomes. For non-2xx outcomes it is a
	// *WireError carrying the typed sentinel; for pre-response
	// failures it is the wrapped underlying error.
	Err error
	// Tags carries per-call observer tags attached via
	// WithObservationTag. Read-only — observers MUST NOT mutate.
	Tags map[string]any
}

// Observer receives one Observation per logical request, independent
// of OTel. Implementations MUST be safe for concurrent invocation; the
// transport calls OnRequest from the goroutine that issued Client.Do
// but a single Observer may be shared across many concurrent calls.
//
// A panicking Observer MUST NOT break the request lifecycle — the
// transport recovers the panic and logs via the configured slog.Logger.
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
// Tags reach the Observer via Observation.Tags as a shallow-cloned map
// — the transport copies before delivery so observers cannot mutate
// the caller's request context.
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
	out := make(map[string]any, len(src))
	maps.Copy(out, src)
	return out
}
