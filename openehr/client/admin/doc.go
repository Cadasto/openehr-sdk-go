// Package admin is the ITS-REST `/admin/*` client (REQ-099). It covers
// the operational housekeeping endpoints used by integration test suites
// and emergency-reset workflows:
//
//   - DeleteEHR: admin-mode delete of a single EHR.
//   - DeleteAllEHRs: wholesale reset (deployment-policy gated; many
//     production tenants disable this surface entirely).
//   - PurgeTemplates: clears the template registry.
//
// See cadasto/admin/ for the Cadasto-platform admin extras — that
// package targets a different surface and SHOULD NOT be conflated with
// this one.
package admin
