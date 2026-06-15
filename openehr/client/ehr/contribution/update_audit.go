package contribution

import (
	"encoding/json"
	"errors"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// AuditType selects the `_type` discriminator emitted on the write-side
// commit audit. ITS-REST PR 131 / SPECITS-95 say a client SHOULD send
// `UPDATE_AUDIT` while servers SHOULD accept `AUDIT_DETAILS` (or an omitted
// `_type`). The SDK defaults to `AUDIT_DETAILS` (the form the reference CDRs
// were validated against); callers can switch to `UPDATE_AUDIT` if a
// non-conformant server rejects `AUDIT_DETAILS`.
type AuditType string

const (
	// AuditTypeAuditDetails emits `_type:"AUDIT_DETAILS"`. This is the SDK
	// default — the zero value of [UpdateAudit.Type] resolves to it.
	AuditTypeAuditDetails AuditType = "AUDIT_DETAILS"
	// AuditTypeUpdateAudit emits `_type:"UPDATE_AUDIT"` — the ITS-REST
	// client-SHOULD form; use it as a fallback when a non-conformant server
	// refuses AUDIT_DETAILS on contribution create.
	AuditTypeUpdateAudit AuditType = "UPDATE_AUDIT"
)

// UpdateAudit is the ITS-REST Contribution_create commit-audit DTO — the
// request-side shape, distinct from the persisted [rm.AuditDetails] returned
// on GET. Per ITS-REST PR 131 / SPECITS-95 the commit audit MUST NOT carry
// a server-assigned time_committed; system_id is optional on write. The SDK
// emits _type:"AUDIT_DETAILS" by default (see [AuditType]).
type UpdateAudit struct {
	// ChangeType is the audit change-type coded value (openEHR Terminology
	// "audit change type" group) — DV_CODED_TEXT shaped (defining_code nesting).
	ChangeType rm.DVCodedText
	// Committer is the party that committed the change (required).
	Committer rm.PartyProxy
	// Description is the optional reason-for-committal text; omitted when nil.
	Description rm.DVTextLike
	// SystemID is the optional logical EHR system id; omitted when empty.
	SystemID string
	// Type selects the emitted `_type`. The zero value emits
	// AuditTypeAuditDetails (the SDK default); set AuditTypeUpdateAudit to
	// fall back to the `UPDATE_AUDIT` form for non-conformant servers.
	Type AuditType
}

// updateAuditJSON is the on-wire shape: _type first, then system_id /
// committer / change_type / description, mirroring the cassette field order.
// No time_committed.
type updateAuditJSON struct {
	Type        AuditType      `json:"_type"`
	SystemID    string         `json:"system_id,omitempty"`
	Committer   rm.PartyProxy  `json:"committer"`
	ChangeType  rm.DVCodedText `json:"change_type"`
	Description rm.DVTextLike  `json:"description,omitempty"`
}

// MarshalJSON emits the ITS-REST write-side audit shape. `_type` is the
// resolved [AuditType] (default AuditTypeAuditDetails); time_committed is
// never emitted (server-assigned).
func (a UpdateAudit) MarshalJSON() ([]byte, error) {
	if a.Committer == nil {
		return nil, errors.New("contribution.UpdateAudit: Committer is required")
	}
	t := a.Type
	if t == "" {
		t = AuditTypeAuditDetails
	}
	// Marshal a *pointer* so addressable fields (e.g. ChangeType, whose
	// MarshalJSON has a pointer receiver) emit their `_type` discriminator
	// instead of falling back to reflection — matching the generated
	// rm.AuditDetails marshaller. Note: Committer must hold a pointer
	// (e.g. *rm.PartyIdentified) for its own `_type` to be emitted, since
	// PartyProxy's concrete marshallers also use pointer receivers.
	return json.Marshal(&updateAuditJSON{
		Type:        t,
		SystemID:    a.SystemID,
		Committer:   a.Committer,
		ChangeType:  a.ChangeType,
		Description: a.Description,
	})
}

// UpdateAuditFromAuditDetails adapts a persisted-shaped [rm.AuditDetails]
// into the write DTO, dropping the server-assigned time_committed.
func UpdateAuditFromAuditDetails(ad rm.AuditDetails) UpdateAudit {
	return UpdateAudit{
		ChangeType:  ad.ChangeType,
		Committer:   ad.Committer,
		Description: ad.Description,
		SystemID:    ad.SystemID,
	}
}
