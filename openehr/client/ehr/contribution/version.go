package contribution

import (
	"encoding/json"
	"errors"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// updateAuditFromLike builds the write DTO from any AuditDetailsLike,
// dropping the server-assigned time_committed. Uses the interface's
// exported accessors (the interface itself is sealed).
func updateAuditFromLike(a rm.AuditDetailsLike) UpdateAudit {
	ua := UpdateAudit{
		ChangeType: a.GetChangeType(),
		Committer:  a.GetCommitter(),
		SystemID:   a.GetSystemID(),
	}
	if d, ok := a.GetDescription(); ok {
		ua.Description = d
	}
	return ua
}

// OriginalVersion is the write-side ORIGINAL_VERSION element for a
// [Submission]. It marshals like rm.OriginalVersion[T] but emits
// commit_audit as the [UpdateAudit] write DTO (no server-assigned
// time_committed). Construct via [WrapOriginalVersion]. T is one of the
// four versionable RM types (rm.Composition, rm.EHRStatus, rm.Folder,
// rm.EHRAccess).
type OriginalVersion[T any] struct {
	// Version is the underlying RM version. Its CommitAudit is ignored on
	// marshal in favour of the CommitAudit field below.
	Version *rm.OriginalVersion[T]
	// CommitAudit is the write-side audit emitted as commit_audit.
	CommitAudit UpdateAudit
}

// WrapOriginalVersion adapts an rm.OriginalVersion for the contribution
// write path, converting its commit_audit to the UpdateAudit DTO (drops
// time_committed). Only the returned wrapper's CommitAudit is emitted on
// marshal — later mutations to the wrapped Version.CommitAudit are ignored.
func WrapOriginalVersion[T any](v *rm.OriginalVersion[T]) *OriginalVersion[T] {
	return &OriginalVersion[T]{Version: v, CommitAudit: updateAuditFromLike(v.CommitAudit)}
}

// BMMName implements CommitVersion.
func (v *OriginalVersion[T]) BMMName() string { return "ORIGINAL_VERSION" }

// originalVersionJSON and importedVersionJSON (below) shadow the generated
// rm.OriginalVersionJSONMarshaller[T] / rm.ImportedVersionJSONMarshaller[T]
// in openehr/rm/common_change_control_jsonmar_gen.go, replacing the
// commit_audit field's type (AuditDetailsLike) with the UpdateAudit write
// DTO. BMM-BUMP: if bmmgen adds or reorders fields on those generated
// structs, update these copies in lockstep — `go test ./openehr/client/ehr/contribution/...`.
type originalVersionJSON[T any] struct {
	Type                  string               `json:"_type"`
	Contribution          rm.ObjectRefLike     `json:"contribution"`
	Signature             *string              `json:"signature,omitempty"`
	CommitAudit           UpdateAudit          `json:"commit_audit"`
	UID                   rm.ObjectVersionID   `json:"uid"`
	PrecedingVersionUID   *rm.ObjectVersionID  `json:"preceding_version_uid,omitempty"`
	OtherInputVersionUids []rm.ObjectVersionID `json:"other_input_version_uids,omitempty"`
	LifecycleState        rm.DVCodedText       `json:"lifecycle_state"`
	Attestations          []rm.Attestation     `json:"attestations,omitempty"`
	Data                  *T                   `json:"data,omitempty"`
}

// MarshalJSON emits the canonical ORIGINAL_VERSION wire shape, replacing
// commit_audit with the [UpdateAudit] write DTO. Marshals by pointer so
// fields with pointer-receiver MarshalJSON (e.g. rm.DVCodedText,
// *rm.Composition) emit their `_type` discriminators correctly.
func (v *OriginalVersion[T]) MarshalJSON() ([]byte, error) {
	o := v.Version
	if o == nil {
		return nil, errors.New("contribution.OriginalVersion: Version is nil")
	}
	return json.Marshal(&originalVersionJSON[T]{
		Type:                  "ORIGINAL_VERSION",
		Contribution:          o.Contribution,
		Signature:             o.Signature,
		CommitAudit:           v.CommitAudit,
		UID:                   o.UID,
		PrecedingVersionUID:   o.PrecedingVersionUID,
		OtherInputVersionUids: o.OtherInputVersionUids,
		LifecycleState:        o.LifecycleState,
		Attestations:          o.Attestations,
		Data:                  o.Data,
	})
}

// ImportedVersion is the write-side IMPORTED_VERSION element for a
// [Submission]; commit_audit is the [UpdateAudit] write DTO. Construct via
// [WrapImportedVersion]. The nested item (the imported ORIGINAL_VERSION) is
// historical and keeps its own rm shape.
type ImportedVersion[T any] struct {
	// Version is the underlying RM imported version. Its CommitAudit is
	// ignored on marshal in favour of the CommitAudit field below.
	Version *rm.ImportedVersion[T]
	// CommitAudit is the write-side audit emitted as commit_audit.
	CommitAudit UpdateAudit
}

// WrapImportedVersion adapts an rm.ImportedVersion for the contribution
// write path, converting its commit_audit to the UpdateAudit DTO (drops
// time_committed). Only the returned wrapper's CommitAudit is emitted on
// marshal — later mutations to the wrapped Version.CommitAudit are ignored.
func WrapImportedVersion[T any](v *rm.ImportedVersion[T]) *ImportedVersion[T] {
	return &ImportedVersion[T]{Version: v, CommitAudit: updateAuditFromLike(v.CommitAudit)}
}

// BMMName implements CommitVersion.
func (v *ImportedVersion[T]) BMMName() string { return "IMPORTED_VERSION" }

type importedVersionJSON[T any] struct {
	Type         string                  `json:"_type"`
	Contribution rm.ObjectRefLike        `json:"contribution"`
	Signature    *string                 `json:"signature,omitempty"`
	CommitAudit  UpdateAudit             `json:"commit_audit"`
	Item         rm.OriginalVersion[any] `json:"item"`
}

// MarshalJSON emits the canonical IMPORTED_VERSION wire shape, replacing
// commit_audit with the [UpdateAudit] write DTO. Marshals by pointer so
// fields with pointer-receiver MarshalJSON emit their `_type` discriminators.
func (v *ImportedVersion[T]) MarshalJSON() ([]byte, error) {
	i := v.Version
	if i == nil {
		return nil, errors.New("contribution.ImportedVersion: Version is nil")
	}
	return json.Marshal(&importedVersionJSON[T]{
		Type:         "IMPORTED_VERSION",
		Contribution: i.Contribution,
		Signature:    i.Signature,
		CommitAudit:  v.CommitAudit,
		Item:         i.Item,
	})
}
