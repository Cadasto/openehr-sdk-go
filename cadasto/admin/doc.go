// Package admin is the client for tenant, environment, system info,
// and healthcheck endpoints on a Cadasto deployment.
//
// # Health probes (REQ-083)
//
// Live and Ready probe deployment-level liveness and readiness. The
// SDK ships default paths (DefaultLivePath = "/health/live",
// DefaultReadyPath = "/health/ready") and a per-call override via
// WithLivePath / WithReadyPath for deployments that diverge (e.g.
// "/healthz"). Probes derive their URL from the openEHR REST service
// entry's origin (scheme + host) — the openEHR REST API path prefix
// is NOT inherited.
//
// Status-code mapping (errors.Is-compatible): 2xx → nil; 401 →
// [transport.ErrUnauthorized]; 403 → [transport.ErrForbidden]; 404 →
// [transport.ErrNotFound]; 5xx → [transport.ErrServerError]. Other
// non-2xx codes (400, 405, 408, 429, etc.) surface as a plain
// formatted error with no sentinel.
//
// Probes borrow [transport]'s sentinel taxonomy (REQ-093) for the
// mapped codes but bypass [transport.Client.Do] — no openEHR error
// envelope decoding, no OTel spans, no retries. Use
// [openehr/client/admin] when those concerns matter.
package admin
