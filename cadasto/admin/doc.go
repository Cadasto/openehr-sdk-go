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
// Status-code mapping (errors.Is-compatible): 2xx → nil; 401 →
// [transport.ErrUnauthorized]; 403 → [transport.ErrForbidden]; 404 →
// [transport.ErrNotFound]; 5xx → [transport.ErrServerError]. Other
// non-2xx codes (400, 405, 408, 429, etc.) surface as a plain
// formatted error with no sentinel. ReadyBody returns nil bytes beside
// that error.
//
// ReadyBody reads at most DefaultMaxReadyBody bytes (1 MiB), or the
// limit WithMaxReadyBody sets. A longer 2xx body fails with an error
// that matches [transport.ErrResponseTooLarge], and any other read
// failure is reached by unwrapping the error; both return nil bytes.
//
// Probes reuse [transport]'s sentinel errors for the mapped codes but
// bypass [transport.Client.Do]: they do no openEHR error envelope
// decoding, record no OTel spans and apply no retries. Use
// [openehr/client/admin] when those concerns matter.
package admin
