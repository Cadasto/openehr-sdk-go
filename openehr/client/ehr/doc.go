// Package ehr is the openEHR REST client for the EHR resource and its
// sub-resources: Composition, Contribution, Directory, EHR_STATUS, and
// ItemTags. Aligned with openEHR REST 1.1.0-development.
//
// Write responses follow REQ-094 Prefer negotiation. A successful
// minimal or identifier write returns a zero resource — a typed-nil
// pointer, or an interface holding one — so `== nil` is not a reliable
// presence test; use [HasResource] (or rm.IsTypedNil on an RM value).
// A 2xx representation with an empty or undecodable body is a
// [*NoRepresentationError] carrying the commit metadata, never a silent
// success; a non-2xx stays a wire error.
//
// Implements REQ-023, REQ-050, REQ-054, and REQ-059 (ItemTags in
// itemtags/) per docs/specifications/wire.md and docs/specifications/transport.md.
package ehr
