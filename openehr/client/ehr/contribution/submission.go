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
// SDK-GAP-10. Plan: docs/plans/2026-05-26-contribution-submission-shape.md.
type Submission struct {
	// Audit is the AUDIT_DETAILS envelope applied to the whole batch
	// (REQ-059). Carried inside the body — there is no separate
	// `openehr-audit-details` header on this endpoint.
	Audit rm.AuditDetails
	// Versions is the closed type-set of inline-data versions to commit.
	// Each element MUST be a *rm.OriginalVersion[T] or
	// *rm.ImportedVersion[T] for T in {*rm.Composition, *rm.EHRStatus,
	// *rm.Folder, *rm.EHRAccess}. Validate() rejects any other shape.
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

// Validate enforces the closed type-set: every CommitVersion's BMMName
// must be "ORIGINAL_VERSION" or "IMPORTED_VERSION". Called automatically
// by MarshalJSON; callers MAY invoke it earlier to surface a typed error
// without paying for marshalling.
func (s *Submission) Validate() error {
	for i, v := range s.Versions {
		if v == nil {
			return fmt.Errorf("Submission.Versions[%d] is nil", i)
		}
		switch v.BMMName() {
		case "ORIGINAL_VERSION", "IMPORTED_VERSION":
		default:
			return fmt.Errorf("Submission.Versions[%d] BMMName=%q (must be ORIGINAL_VERSION or IMPORTED_VERSION)", i, v.BMMName())
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
