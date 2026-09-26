// Package ehr is the openEHR REST client for the EHR resource and its
// sub-resources: Composition, Contribution, Directory, EHR_STATUS, and
// ItemTags. Aligned with openEHR REST 1.1.0-development.
//
// Write responses follow the Prefer header negotiation. A successful
// minimal or identifier write returns a zero resource. Do not rely on
// `== nil` as a presence test across leaves, because an interface return
// (demographic rm.Party) can legally hold a typed-nil pointer; use
// [HasResource] uniformly (rm.IsTypedNil is the typed-nil absence check
// for callers already holding a registered RM pointer). On the
// versioned-write leaves (composition / directory / ehr_status /
// demographic) and contribution Commit, a 2xx representation with an
// empty, JSON-null, or undecodable body is a [*NoRepresentationError]
// carrying the commit metadata, never a silent success; a non-2xx stays
// a wire error. EHR creation ([Create]) follows the same rule for its
// empty or null body (a [*NoRepresentationError]), while an undecodable
// body stays a [*transport.DecodeError].
//
// ItemTag operations live in the itemtags sub-package.
package ehr
