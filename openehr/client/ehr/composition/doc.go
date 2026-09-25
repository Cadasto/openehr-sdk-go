// Package composition is the openEHR REST 1.1.0-development
// Composition sub-resource client. It covers reads and the versioned
// writes (Save / Update / Delete, with If-Match / ETag optimistic
// concurrency).
//
// A Composition GET addresses either the versioned-object family
// (returns the latest) or a specific version, discriminated by
// [github.com/cadasto/openehr-sdk-go/openehr/client/ehr.Ref];
// callers construct one via [ehr.LatestOf], [ehr.LatestAtTime], or
// [ehr.VersionOf].
package composition
