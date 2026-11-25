// Package ehr is the openEHR REST client for the EHR resource and its
// sub-resources: Composition, Contribution, Directory, EHR_STATUS, and
// ItemTags. Aligned with openEHR REST 1.1.0.
//
// This package reads, checks and creates the EHR root, which [EHRID]
// names. The types every resource client shares, such as
// [client.VersionMetadata], [client.Ref] and [client.NoRepresentationError],
// live in package [client]; the names this package still exports for them
// are deprecated aliases of those.
//
// [Create] asks for the new EHR's representation. A 2xx with an empty or
// null body committed the EHR but is a [*client.NoRepresentationError]
// carrying the commit metadata, never a silent success, while an
// undecodable body is a [*transport.DecodeError].
//
// ItemTag operations live in the itemtags sub-package.
package ehr
