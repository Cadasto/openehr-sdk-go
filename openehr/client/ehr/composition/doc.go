// Package composition is the openEHR REST 1.1.0-development
// Composition sub-resource client. Read paths land first; the full
// versioned-write surface (Save / Update / Delete with If-Match /
// ETag plumbing per REQ-054) follows in Phase 4 of the REST API
// client plan.
//
// A Composition GET addresses either the versioned-object family
// (returns the latest) or a specific version, discriminated by
// [github.com/cadasto/openehr-sdk-go/openehr/client/ehr.Ref] —
// callers construct one via [ehr.LatestOf], [ehr.LatestAtTime], or
// [ehr.VersionOf].
package composition
