// Package ehr is the openEHR REST client for the EHR resource and its
// sub-resources: Composition, Contribution, Directory, EHR_STATUS, and
// ItemTags. Aligned with openEHR REST 1.1.0-development.
//
// Write responses follow REQ-094 Prefer negotiation. A successful
// minimal or identifier write returns a zero resource. Do not build a
// `== nil` presence habit across leaves — an interface return
// (demographic rm.Party) can legally hold a typed-nil pointer — use
// [HasResource] uniformly (rm.IsTypedNil is the typed-nil absence check
// for callers already holding a registered RM pointer). On the
// versioned-write leaves (composition / directory / ehr_status /
// demographic) and contribution Commit, a 2xx representation with an
// empty, JSON-null, or undecodable body is a [*NoRepresentationError]
// carrying the commit metadata, never a silent success; a non-2xx stays
// a wire error. EHR creation ([Create]) follows the same rule for its
// empty/null-body arm — a [*NoRepresentationError] — while its
// decode-failure arm stays a [*transport.DecodeError] per REQ-151.
//
// Implements REQ-023, REQ-050, REQ-054, REQ-059 (ItemTags in
// itemtags/), and the REQ-094 write-result contract per
// docs/specifications/wire.md and docs/specifications/transport.md.
package ehr
