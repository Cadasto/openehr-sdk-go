// Package admin is the client for tenant, environment, system info,
// and healthcheck endpoints on a Cadasto deployment.
//
// # Health probes (SDK-GAP-07)
//
// Live and Ready probe deployment-level liveness and readiness. The
// SDK ships default paths (DefaultLivePath = "/health/live",
// DefaultReadyPath = "/health/ready") and a per-call override via
// WithLivePath / WithReadyPath for deployments that diverge (e.g.
// "/healthz"). Probes derive their URL from the openEHR REST service
// entry's origin (scheme + host) — the openEHR REST API path prefix
// is NOT inherited. Returns nil on 2xx; transport.ErrNotFound on 404;
// other transport sentinels (ErrUnauthorized, ErrForbidden,
// ErrServerError) per REQ-093.
package admin
