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
// Save and Update write canonical JSON by default. [WithFormat] writes a
// composition as canonical XML ([FormatXML]) or in the simplified FLAT or
// STRUCTURED format ([FormatFlat], [FormatStructured]), which need the
// template's Web Template; a returned composition is read in the same
// format. [WithFormatQuery] is an opt-in for deployments that select the
// simplified formats with query parameters instead of media types.
package composition
