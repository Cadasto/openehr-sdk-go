// Package admin is the client for tenant, environment, system info,
// and healthcheck endpoints on a Cadasto deployment.
//
// # Health probes
//
// Live and Ready probe deployment-level liveness and readiness.
// ReadyBody sends the request Ready sends and returns the readiness
// body, such as a document that names the service and its version. The
// SDK ships default paths (DefaultLivePath = "/health/live",
// DefaultReadyPath = "/health/ready") and a per-call override via
// WithLivePath / WithReadyPath for deployments that diverge (e.g.
// "/healthz"). Probes derive their URL from the openEHR REST service
// entry's origin (scheme + host); the openEHR REST API path prefix
// is not inherited.
//
// A 2xx returns nil. Any other status returns a [*StatusError] that
// carries the probe path and the status code, reached with
// errors.AsType[*admin.StatusError](err). It unwraps to a sentinel, so
// errors.Is matches (status-code mapping): 401 →
// [transport.ErrUnauthorized]; 403 → [transport.ErrForbidden]; 404 →
// [transport.ErrNotFound]; 5xx → [transport.ErrServerError]; any other
// non-2xx code (400, 405, 408, 429, etc.) → [ErrUnexpectedStatus]. The
// error text never carries a body byte or a header value. ReadyBody
// returns nil bytes beside that error.
//
// ReadyBody reads at most DefaultMaxReadyBody bytes (1 MiB) of a 2xx
// body, or the limit WithMaxReadyBody sets. A longer 2xx body fails with
// an error that matches [transport.ErrResponseTooLarge], and any other
// read failure is reached by unwrapping the error; both return nil bytes.
//
// A probe reads at most 256 KiB of a body it does not return (the 2xx
// body Live and Ready discard, and any non-2xx body), for at most 50 ms,
// before it closes it. So a body that never ends, sent fast or slowly,
// cannot hold a probe until its context ends. When the 50 ms run out, the
// probe ends its request, which can cost the connection.
//
// Probes reuse [transport]'s sentinel errors for the mapped codes but
// bypass [transport.Client.Do]: they do no openEHR error envelope
// decoding, record no OTel spans and apply no retries. Use
// [openehr/client/admin] when those concerns matter.
package admin
