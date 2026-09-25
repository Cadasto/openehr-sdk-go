// Package admin is the ITS-REST `/admin/*` client. It covers
// the operational housekeeping endpoints used by integration test suites
// and emergency-reset workflows:
//
//   - DeleteEHR: admin-mode delete of a single EHR (DELETE /admin/ehr/{ehr_id}).
//   - DeleteAllEHRs: wholesale reset (DELETE /admin/ehr/all, + optional
//     ehr_id subset); allowed only where deployment policy permits it, and
//     tenants with it disabled return 405.
//   - PurgeTemplates: clears the template registry. It is not part of the
//     ITS-REST admin contract but an EHRbase extension (DELETE
//     /admin/template/all); see the godoc on PurgeTemplates.
//
// The upstream Admin API has x-status: DEVELOPMENT, so this client is a
// draft and may change between minor versions.
//
// The cadasto/admin package holds the Cadasto-platform admin extras. It
// targets a different surface and is unrelated to this one.
package admin
