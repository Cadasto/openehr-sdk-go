// Package contribution is the openEHR REST 1.1.0-development
// Contribution sub-resource client — multi-version atomic commits
// against an EHR.
//
// The submission body is the ITS-REST `Contribution_create` schema
// [Submission] (audit + inline ORIGINAL_VERSION / IMPORTED_VERSION
// elements), distinct from the persisted [github.com/cadasto/openehr-sdk-go/openehr/rm.Contribution]
// shape returned in responses (audit + OBJECT_REF stubs). This split
// closes REQ-050/095 and is symmetric to the REQ-094 fix on
// `composition.Save` / `Update`: request and response are different
// shapes, both spec-conformant.
//
// The audit envelope is carried inside the body (REQ-059); unlike
// per-resource writes there is no separate `openehr-audit-details`
// header. The server applies the entire batch atomically: either
// every version commits or none do. Errors map per REQ-093;
// optimistic-concurrency failures within the batch return
// [transport.ErrVersionConflict].
//
// The commit audit uses the write-side shape [UpdateAudit] (not the
// persisted `rm.AuditDetails`): it omits the server-assigned
// `time_committed` and keeps `change_type` as `DV_CODED_TEXT`
// (SPECITS-95 / ITS-REST PR 131). It marshals as `_type:"AUDIT_DETAILS"`
// by default — the form reference CDRs validate — with a settable
// [AuditTypeUpdateAudit] fallback for servers that refuse it. Inline
// versions are submitted through the [OriginalVersion] / [ImportedVersion]
// write-wrappers ([WrapOriginalVersion] / [WrapImportedVersion]), which
// carry the same commit-audit shape. PROBE-072; REQ-050 / REQ-095.
package contribution
