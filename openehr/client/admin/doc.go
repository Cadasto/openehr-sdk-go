// Package admin is the ITS-REST `/admin/*` client (REQ-099). It covers
// operational housekeeping endpoints that deployments commonly expose
// for test setup/teardown and emergency-reset workflows:
//
//   - DeleteEHR: admin-mode delete of a single EHR (used by integration
//     test suites between scenarios).
//   - DeleteAllEHRs: wholesale reset of every EHR on the deployment
//     (gated by the deployment's policy — typically only available on
//     non-production tenants).
//   - PurgeTemplates: clears the template registry.
//
// All admin operations are idempotent on absent resources: a 404
// surfaces as transport.ErrNotFound, never as a panic.
//
// This package is distinct from cadasto/admin/, which targets the
// Cadasto admin extras and is NOT a sister of the ITS-REST admin
// surface (per the module-layout "single cut line under cadasto/"
// rationale). Mixing the two is a design error.
//
// Wire shape: every operation routes through the standard openEHR REST
// catalog entry (org.openehr.rest) with the path prefix `/admin/`.
// Deployments that mount admin under a separate base URL can override
// per-request via a custom transport.ServiceCatalog entry, but the
// default assumes the unified mount used by ehrbase and Cadasto.
package admin
