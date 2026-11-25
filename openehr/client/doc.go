// Package client holds the types and helpers every openEHR REST 1.1.0
// resource client shares, and its sub-packages hold the clients
// themselves, one per openEHR resource:
//
//   - client/ehr: EHR and sub-resources (Composition, Contribution,
//     Directory, EHR_STATUS, ItemTags)
//   - client/query: AQL executor
//   - client/definition: templates, stored queries
//   - client/demographic: demographic resources
//   - client/system: system info, capabilities (where applicable)
//
// The shared surface names a versioned resource and its versions
// ([VersionedObjectID], [VersionUID]), the target of a versioned read
// ([Ref], built with [LatestOf], [LatestAtTime] or [VersionOf]), the
// metadata a versioned read or write returns ([VersionMetadata]), the
// write options and result handling every versioned write shares ([WriteConfig], [WriteResult], [DoDelete]),
// and the openEHR request headers ([MarshalAuditDetails],
// [FormatLifecycleStateHeader], [FormatItemTagHeader],
// [FormatVersionAtTime]).
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
// a wire error. EHR creation (ehr.Create) follows the same rule for its
// empty or null body (a [*NoRepresentationError]), while an undecodable
// body stays a [*transport.DecodeError].
//
// Clients accept context.Context as the first argument on every I/O
// method.
package client
