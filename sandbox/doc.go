// Package sandbox is the in-memory openEHR backend that backs
// REQ-082's Sandbox probe mode.
//
// [Backend] implements [http.RoundTripper], so a consumer (or the
// probe runner) injects it as the Transport of an *http.Client and
// hands that client to the SDK transport constructor, transport.New.
// There is no network listener and no credential check — REQ-082 and
// REQ-013: this package imports neither transport/ nor auth/.
//
// # Served scope
//
// Only the EHR resource is built in: POST /ehr, PUT /ehr/{id},
// GET /ehr/{id} and HEAD /ehr/{id}. Every other route — EHR_STATUS,
// composition, directory, contribution, query, definition — answers
// 404 unless a scripted route is registered for it. Those resources
// arrive in later phases; the probe-runnability plan marks its
// sandbox phase partial for exactly this reason. A request path is
// matched after cutting it through the first "/openehr/v1" segment,
// so any deployment base ("/openehr/v1", "/ehrbase/rest/openehr/v1",
// "/api/openehr/v1", or none at all) reaches the same routes.
//
// Isolation is per Backend, not per package or process: the runner or
// a test owns one Backend instance per run, so two probes never
// observe each other's writes unless they are deliberately handed the
// same instance (REQ-082 — sandbox state MUST be per-run and
// isolated). The zero value is ready to use, like [bytes.Buffer];
// [New] is a convenience constructor, not a requirement.
//
// [Backend.Handle] / [Backend.HandleFunc] / [Scripted] register
// planted routes that
// take precedence over the built-in EHR surface, so a probe test can
// serve a hostile or fixture response without httptest.NewServer.
package sandbox
