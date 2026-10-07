// Package transport is the HTTP client wrapper around an injected
// *http.Client. It hosts request/response interceptors, retry and
// backoff, OpenTelemetry hooks, error mapping, and internal
// spec-version pinning.
//
// The SDK does not allocate its own transport: callers inject the
// *http.Client whose connection pool, timeouts, and TLS config they
// want to control. The one change the transport makes is to the
// redirect policy of a request that carries an Authorization header:
// it refuses a redirect from https to a URL that is not https, with
// [ErrInsecureRedirect], so a redirect cannot carry the credential off
// an encrypted connection. [WithHTTPClient] describes it.
//
// The package is named transport, not http, to avoid a collision with
// the standard-library net/http at call sites.
package transport
