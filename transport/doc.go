// Package transport is the HTTP client wrapper around an injected
// *http.Client. It hosts request/response interceptors, retry and
// backoff, OpenTelemetry hooks, error mapping, and internal
// spec-version pinning.
//
// Implements REQ-021 (injected client), REQ-051, REQ-054, REQ-059,
// REQ-066, REQ-090, REQ-091, REQ-093, and REQ-094 per specs/transport.md
// and specs/wire.md.
//
// The SDK does not allocate its own transport — consumers must inject
// the *http.Client whose connection pool, timeouts, and TLS config they
// want to control.
//
// The package is named transport (not http) to avoid collision with
// the standard-library net/http at consumer call sites.
package transport
