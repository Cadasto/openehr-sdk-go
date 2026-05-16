// Package transport is the HTTP client wrapper around an injected
// *http.Client. It hosts request/response interceptors, retry and
// backoff, OpenTelemetry hooks, error mapping, and internal
// spec-version pinning.
//
// The SDK does not allocate its own transport — consumers must inject
// the *http.Client whose connection pool, timeouts, and TLS config they
// want to control.
//
// The package is named transport (not http) to avoid collision with
// the standard-library net/http at consumer call sites.
//
// See the SDK Specification proposal — Module layout > transport/.
package transport
