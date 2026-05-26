// Package contribution is the openEHR REST 1.1.0-development
// Contribution sub-resource client — multi-version atomic commits
// against an EHR.
//
// The submission body is the ITS-REST `Contribution_create` schema
// [Submission] (audit + inline ORIGINAL_VERSION / IMPORTED_VERSION
// elements), distinct from the persisted [github.com/cadasto/openehr-sdk-go/openehr/rm.Contribution]
// shape returned in responses (audit + OBJECT_REF stubs). This split
// closes SDK-GAP-10 and is symmetric to the SDK-GAP-09 fix on
// `composition.Save` / `Update`: request and response are different
// shapes, both spec-conformant.
//
// The audit envelope is carried inside the body (REQ-059); unlike
// per-resource writes there is no separate `openehr-audit-details`
// header. The server applies the entire batch atomically: either
// every version commits or none do. Errors map per REQ-093;
// optimistic-concurrency failures within the batch return
// [transport.ErrVersionConflict].
package contribution
