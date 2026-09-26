// Package transport is the HTTP client wrapper around an injected
// *http.Client. It hosts request/response interceptors, retry and
// backoff, OpenTelemetry hooks, error mapping, and internal
// spec-version pinning.
//
// The SDK does not allocate its own transport: callers inject the
// *http.Client whose connection pool, timeouts, and TLS config they
// want to control.
//
// The package is named transport, not http, to avoid a collision with
// the standard-library net/http at call sites.
package transport
