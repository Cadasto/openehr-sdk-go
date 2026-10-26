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
// Save and Update write canonical JSON by default, and Get reads it.
// [WithFormat] writes a composition as canonical XML ([FormatXML]) or in the
// simplified FLAT or STRUCTURED format ([FormatFlat], [FormatStructured]),
// which need the template's Web Template; a returned composition is read in
// the same format. [WithGetFormat] reads a composition in any of the four.
// Both options also take the simplified codec's decode options and hand
// them to every FLAT or STRUCTURED decode they govern. Without
// [simplified.WithTemplate] a decoded composition lacks the names of the
// nodes below the root and the reference model defaults the compiled
// template completes, so a caller who needs a complete composition passes it.
// Without a Web Template, Get leaves a FLAT or STRUCTURED body undecoded,
// and [WithRawBody] hands back the body as the server sent it, in every
// format. [WithFormatQuery] and [WithGetFormatQuery] are opt-ins for
// deployments that select the simplified formats with query parameters
// instead of media types.
//
// The package also reads a composition's version history: its versioned
// object ([GetVersionedObject]), its revision history
// ([GetRevisionHistory]), and its versions, each with the commit audit
// around the COMPOSITION: the latest ([GetVersion]), the one current at a
// time ([GetVersionAtTime]), or one by its id ([GetVersionByID]). These
// reads take canonical JSON only.
package composition
