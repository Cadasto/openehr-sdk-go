package contribution

import (
	"encoding/json"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// Submission is the request-side payload for POST /ehr/{ehr_id}/contribution
// — the ITS-REST `Contribution_create` schema. It is distinct from
// [rm.Contribution] (the persisted/response shape) because each versions[]
// element carries the resource payload inline under `data`, not a stub
// [rm.ObjectRef].
//
// At submission time the OBJECT_REFs in the persisted shape would point
// at versions that do not yet exist, so a spec-conformant CDR rejects
// the persisted shape on the write path. This shape is symmetric to the
// SDK-GAP-09 fix on `composition.Save / Update` (response-side bare
// COMPOSITION) — see [docs/specifications/conformance.md] PROBE-071 /
// PROBE-072.
//
// SDK-GAP-10. Plan: docs/plans/archive/2026-05-26-contribution-submission-shape.md.
type Submission struct {
	// Audit is the AUDIT_DETAILS envelope applied to the whole batch
	// (REQ-059). Carried inside the body — there is no separate
	// `openehr-audit-details` header on this endpoint.
	Audit rm.AuditDetails
	// Versions is the closed type-set of inline-data versions to commit.
	// Each element MUST be a *rm.OriginalVersion[T] or
	// *rm.ImportedVersion[T] for T in {rm.Composition, rm.EHRStatus,
	// rm.Folder, rm.EHRAccess}. Validate enforces this via an explicit
	// type-switch over the 8 concrete generic instantiations (no
	// reflection per REQ-024); the slice must also be non-empty.
	Versions []CommitVersion
}

// CommitVersion is the marker interface for Submission.Versions[i].
// Closed type-set: *rm.OriginalVersion[T] and *rm.ImportedVersion[T]
// for the four versionable T's. Other types satisfying this method set
// (json.Marshaler + BMMName() string) are detected at Submission.Validate
// via the BMMName check.
type CommitVersion interface {
	json.Marshaler
	BMMName() string
}

// Validate enforces the documented closed type-set: each
// Submission.Versions[i] must be a *rm.OriginalVersion[T] or
// *rm.ImportedVersion[T] for T ∈ {rm.Composition, rm.EHRStatus,
// rm.Folder, rm.EHRAccess} — the four versionable types in the
// ITS-REST `Contribution_create` schema. A non-empty Versions slice is
// also required (the spec rejects an empty contribution).
//
// Implemented as an explicit type-switch over the 8 concrete generic
// instantiations (no reflection per REQ-024). Other types satisfying
// the CommitVersion method set are rejected with a typed error naming
// the BMMName for caller diagnostics. Called automatically by
// MarshalJSON; callers MAY invoke it earlier to surface a typed error
// without paying for marshalling.
func (s *Submission) Validate() error {
	if len(s.Versions) == 0 {
		return fmt.Errorf("Submission.Versions: empty (Contribution_create requires at least one version)")
	}
	for i, v := range s.Versions {
		if v == nil {
			return fmt.Errorf("Submission.Versions[%d] is nil", i)
		}
		switch v.(type) {
		case *rm.OriginalVersion[rm.Composition],
			*rm.OriginalVersion[rm.EHRStatus],
			*rm.OriginalVersion[rm.Folder],
			*rm.OriginalVersion[rm.EHRAccess],
			*rm.ImportedVersion[rm.Composition],
			*rm.ImportedVersion[rm.EHRStatus],
			*rm.ImportedVersion[rm.Folder],
			*rm.ImportedVersion[rm.EHRAccess]:
		default:
			return fmt.Errorf("Submission.Versions[%d] is %T (BMMName=%q); want *rm.OriginalVersion[T] or *rm.ImportedVersion[T] for T in {Composition, EHRStatus, Folder, EHRAccess}", i, v, v.BMMName())
		}
	}
	return nil
}

// submissionJSON is the on-the-wire shape — no `_type` envelope because
// `Contribution_create` is a request schema, not an RM class. Field order
// mirrors the OpenAPI definition (audit before versions); per-version
// `_type` discrimination is emitted by each element's own MarshalJSON.
type submissionJSON struct {
	Audit    rm.AuditDetails `json:"audit"`
	Versions []CommitVersion `json:"versions"`
}

// MarshalJSON emits the canonical `Contribution_create` wire shape. The
// closed type-set is enforced before encoding via Validate.
func (s *Submission) MarshalJSON() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(&submissionJSON{Audit: s.Audit, Versions: s.Versions})
}
