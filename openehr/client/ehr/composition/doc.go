// Package composition is the openEHR REST 1.1.0
// Composition sub-resource client. It covers reads and the versioned
// writes (Save / Update / Delete). Update carries If-Match / ETag
// optimistic concurrency; Delete names the version to delete in its
// path and sends no If-Match.
//
// A Composition GET addresses either the versioned-object family
// (returns the latest) or a specific version, discriminated by
// [github.com/cadasto/openehr-sdk-go/openehr/client/ehr.Ref];
// callers construct one via [ehr.LatestOf], [ehr.LatestAtTime], or
// [ehr.VersionOf].
//
// The package also reads a composition's version history: its versioned
// object ([GetVersionedObject]), its revision history
// ([GetRevisionHistory]), and its versions, each with the commit audit
// around the COMPOSITION: the latest ([GetVersion]), the one current at a
// time ([GetVersionAtTime]), or one by its id ([GetVersionByID]).
package composition
