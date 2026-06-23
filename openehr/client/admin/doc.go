// Package admin is the ITS-REST `/admin/*` client (REQ-099). It covers
// the operational housekeeping endpoints used by integration test suites
// and emergency-reset workflows:
//
//   - DeleteEHR: admin-mode delete of a single EHR (DELETE /admin/ehr/{ehr_id}).
//   - DeleteAllEHRs: wholesale reset (DELETE /admin/ehr/all, + optional
//     ehr_id subset); deployment-policy gated — disabled tenants return 405.
//   - PurgeTemplates: clears the template registry. NOT part of the
//     ITS-REST admin contract — an EHRbase extension (DELETE
//     /admin/template/all); see the godoc on PurgeTemplates.
//
// The Admin API is upstream x-status: DEVELOPMENT, so this client ships as
// Draft and may change between minor versions.
//
// See cadasto/admin/ for the Cadasto-platform admin extras — that
// package targets a different surface and SHOULD NOT be conflated with
// this one.
package admin
