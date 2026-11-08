// Package contribution is the openEHR REST 1.1.0
// Contribution sub-resource client: multi-version atomic commits
// against an EHR ([Commit]) and reads of a persisted contribution by
// uid ([Get]), the two operations ITS-REST declares.
//
// The submission body is the ITS-REST `Contribution_create` schema
// [Submission] (audit + inline ORIGINAL_VERSION / IMPORTED_VERSION
// elements), distinct from the persisted [github.com/cadasto/openehr-sdk-go/openehr/rm.Contribution]
// shape returned in responses (audit + OBJECT_REF stubs). As with
// `composition.Save` / `Update`, request and response are different
// shapes, and both conform to ITS-REST.
//
// The audit envelope is carried inside the body; unlike
// per-resource writes there is no separate `openehr-audit-details`
// header. The server applies the entire batch atomically: either
// every version commits or none do. HTTP errors map to the standard
// [transport] sentinels; optimistic-concurrency failures within the batch return
// [transport.ErrVersionConflict].
//
// The commit audit uses the write-side shape [UpdateAudit] (not the
// persisted `rm.AuditDetails`): it omits the server-assigned
// `time_committed` and keeps `change_type` as `DV_CODED_TEXT`
// (SPECITS-95 / ITS-REST PR 131). It marshals as `_type:"AUDIT_DETAILS"`
// by default, and as `_type:"UPDATE_AUDIT"` when its type is
// [AuditTypeUpdateAudit]. Inline versions are submitted through the
// [OriginalVersion] / [ImportedVersion] write-wrappers
// ([WrapOriginalVersion] / [WrapImportedVersion]), which carry the same
// commit-audit shape.
//
// The ITS-REST pin leaves the audit's `system_id` optional on write, asks
// clients to send `UPDATE_AUDIT`, and asks servers to accept `AUDIT_DETAILS`
// or an omitted `_type`. A server that validates the commit audit as the RM
// `AUDIT_DETAILS`, EHRbase among them, refuses a commit without a non-empty
// system id on the batch audit and on every version's `commit_audit`. Set it
// once with [Builder.WithSystemID]: the builder copies it into each version.
// EHRbase accepts any non-empty value and records its own system id on the
// stored audit, so the `system_id` of the EHR being written to
// ([rm.EHR.SystemID], which `ehr.Create` and `ehr.Get` return) is a natural
// value. The SDK cannot pick one itself.
//
// [AuditTypeUpdateAudit] is for a server that accepts only `UPDATE_AUDIT`.
// EHRbase does not resolve that type and refuses it with or without a system
// id. A refusal for a missing system id is fixed by setting one, not by
// switching the type.
//
// [Builder] is the authoring surface over that shape: it assembles a [Submission] from payloads and per-operation
// [Change] values ([Creation], [Amendment], [Modification], [Deletion]),
// setting each version's change-type code and lifecycle state and
// inheriting the batch audit, so a caller never hand-wires a version
// wrapper. It emits nothing a caller could not write by hand: the
// server-assigned `contribution` and `uid` fields, which the ITS-REST
// `UpdateVersion` request DTO does not declare, are omitted from every
// write-side body rather than sent empty.
//
// [UnmarshalSubmission] reads that body back: it decodes a canonical-JSON
// `Contribution_create` document authored elsewhere into a [Submission], each
// version as the wrapper and payload type its `_type` members name and each
// audit keeping the `_type` it was read with, so a batch received as one
// document is committed as the batch it describes.
package contribution
