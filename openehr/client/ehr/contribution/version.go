package contribution

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// updateAuditFromLike builds the write DTO from any AuditDetailsLike,
// dropping the server-assigned time_committed. Uses the interface's
// exported accessors (the interface itself is sealed). A nil or
// typed-nil a yields the zero UpdateAudit.
func updateAuditFromLike(a rm.AuditDetailsLike) UpdateAudit {
	if a == nil || rm.IsTypedNil(a) {
		return UpdateAudit{}
	}
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
// marshal; later mutations to the wrapped Version.CommitAudit are ignored.
// A nil v yields an empty wrapper; a nil or typed-nil CommitAudit yields
// an empty UpdateAudit. [Submission.Validate] rejects both as errors
// instead of panicking.
func WrapOriginalVersion[T any](v *rm.OriginalVersion[T]) *OriginalVersion[T] {
	if v == nil {
		return &OriginalVersion[T]{}
	}
	return &OriginalVersion[T]{Version: v, CommitAudit: updateAuditFromLike(v.CommitAudit)}
}

// BMMName implements CommitVersion.
func (v *OriginalVersion[T]) BMMName() string { return "ORIGINAL_VERSION" }

// originalVersionJSON and importedVersionJSON (below) shadow the field set of
// the generated rm.OriginalVersion[T] / rm.ImportedVersion[T] streaming
// codec in openehr/rm/common_change_control_jsonmar_gen.go, replacing the
// commit_audit field's type (AuditDetailsLike) with the UpdateAudit write
// DTO and adding the `_type` member the generated form injects through its
// anonymous wrapper (ADR 0022). BMM-BUMP: if bmmgen adds or reorders fields on
// those generated structs, update these copies in lockstep:
// `go test ./openehr/client/ehr/contribution/...`.
//
// The `signature` pointer field carries `omitzero`, not `omitempty`, matching
// the generated RM structs (ADR 0022, Q6): these shadow DTOs marshal through
// the caller's own options, which may carry v1 legacy flags, and under
// `omitempty` a non-nil pointer to an empty string is omitted by a v2 caller
// but emitted by a v1 one. `omitzero` omits only a nil pointer, the same way
// from any entry point.
type originalVersionJSON[T any] struct {
	Type                  string               `json:"_type"`
	Contribution          rm.ObjectRefLike     `json:"contribution,omitempty"`
	Signature             *string              `json:"signature,omitzero"`
	CommitAudit           UpdateAudit          `json:"commit_audit"`
	UID                   *rm.ObjectVersionID  `json:"uid,omitempty"`
	PrecedingVersionUID   *rm.ObjectVersionID  `json:"preceding_version_uid,omitempty"`
	OtherInputVersionUids []rm.ObjectVersionID `json:"other_input_version_uids,omitempty"`
	LifecycleState        rm.DVCodedText       `json:"lifecycle_state"`
	Attestations          []rm.Attestation     `json:"attestations,omitempty"`
	Data                  *T                   `json:"data,omitempty"`
}

// omitIfAbsent reports the contribution reference to emit, or nil when the
// caller supplied none. `contribution` and `uid` are assigned by the server
// at commit and are not declared on the pin's `UpdateVersion` request DTO,
// so a write-side body omits them rather than sending `null` or an empty
// id object (wire.md § REQ-130 § Server-assigned fields). A typed-nil
// interface counts as absent — it would otherwise marshal as `null`.
func omitIfAbsent(ref rm.ObjectRefLike) rm.ObjectRefLike {
	if ref == nil || rm.IsTypedNil(ref) {
		return nil
	}
	return ref
}

// uidOrNil returns a pointer to uid when the caller populated it, so an
// unset uid is omitted rather than emitted as `{"value":""}` (REQ-130).
func uidOrNil(uid rm.ObjectVersionID) *rm.ObjectVersionID {
	if uid.Value == "" {
		return nil
	}
	return &uid
}

// MarshalJSONTo emits the canonical ORIGINAL_VERSION wire shape, replacing
// commit_audit with the [UpdateAudit] write DTO. The streaming pair carries
// the `_type` discriminators on nested rm fields (e.g. rm.DVCodedText,
// *rm.Composition), which marshal through MarshalJSONTo.
func (v *OriginalVersion[T]) MarshalJSONTo(enc *jsontext.Encoder) error {
	o := v.Version
	if o == nil {
		return errors.New("contribution.OriginalVersion: Version is nil")
	}
	return json.MarshalEncode(enc, &originalVersionJSON[T]{
		Type:                  "ORIGINAL_VERSION",
		Contribution:          omitIfAbsent(o.Contribution),
		Signature:             o.Signature,
		CommitAudit:           v.CommitAudit,
		UID:                   uidOrNil(o.UID),
		PrecedingVersionUID:   o.PrecedingVersionUID,
		OtherInputVersionUids: o.OtherInputVersionUids,
		LifecycleState:        o.LifecycleState,
		Attestations:          o.Attestations,
		Data:                  o.Data,
	}, typereg.MarshalOptions(enc))
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
// marshal; later mutations to the wrapped Version.CommitAudit are ignored.
// A nil v yields an empty wrapper; a nil or typed-nil CommitAudit yields
// an empty UpdateAudit. [Submission.Validate] rejects both as errors
// instead of panicking.
func WrapImportedVersion[T any](v *rm.ImportedVersion[T]) *ImportedVersion[T] {
	if v == nil {
		return &ImportedVersion[T]{}
	}
	return &ImportedVersion[T]{Version: v, CommitAudit: updateAuditFromLike(v.CommitAudit)}
}

// BMMName implements CommitVersion.
func (v *ImportedVersion[T]) BMMName() string { return "IMPORTED_VERSION" }

type importedVersionJSON[T any] struct {
	Type         string                  `json:"_type"`
	Contribution rm.ObjectRefLike        `json:"contribution,omitempty"`
	Signature    *string                 `json:"signature,omitzero"`
	CommitAudit  UpdateAudit             `json:"commit_audit"`
	Item         rm.OriginalVersion[any] `json:"item"`
}

// MarshalJSONTo emits the canonical IMPORTED_VERSION wire shape, replacing
// commit_audit with the [UpdateAudit] write DTO. Nested rm fields carry their
// `_type` discriminators through the streaming pair.
func (v *ImportedVersion[T]) MarshalJSONTo(enc *jsontext.Encoder) error {
	i := v.Version
	if i == nil {
		return errors.New("contribution.ImportedVersion: Version is nil")
	}
	return json.MarshalEncode(enc, &importedVersionJSON[T]{
		Type:         "IMPORTED_VERSION",
		Contribution: omitIfAbsent(i.Contribution),
		Signature:    i.Signature,
		CommitAudit:  v.CommitAudit,
		Item:         i.Item,
	}, typereg.MarshalOptions(enc))
}
