package ehrstatus

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// versionedBase returns the path of the EHR's VERSIONED_EHR_STATUS. An EHR
// has one EHR_STATUS, so the ehr_id alone addresses its versioned object.
// All four endpoints under it are GET-only.
func versionedBase(ehrID openehrclient.EHRID) string {
	return "/ehr/" + string(ehrID) + "/versioned_ehr_status"
}

const (
	routeVersionedObject = "/ehr/{ehr_id}/versioned_ehr_status"
	routeRevisionHistory = "/ehr/{ehr_id}/versioned_ehr_status/revision_history"
	routeVersion         = "/ehr/{ehr_id}/versioned_ehr_status/version"
	routeVersionByID     = "/ehr/{ehr_id}/versioned_ehr_status/version/{version_uid}"
)

// Version is one decoded VERSION<EHR_STATUS>: the ORIGINAL_VERSION envelope
// plus the EHR_STATUS it carries.
//
// It is a curated projection of ORIGINAL_VERSION: the envelope's
// attestations, other_input_version_uids and signature are not surfaced.
// Only an ORIGINAL_VERSION decodes: a body naming any other version type,
// such as IMPORTED_VERSION, fails with a [*transport.DecodeError].
type Version struct {
	// UID is this version's OBJECT_VERSION_ID. Its Value is the wire string
	// you pass back as an [openehrclient.VersionUID]; see [Version.VersionUID].
	UID rm.ObjectVersionID
	// PrecedingVersionUID is the version this one follows, or nil for the
	// first.
	PrecedingVersionUID *rm.ObjectVersionID
	// LifecycleState is the openEHR version lifecycle code.
	LifecycleState rm.DVCodedText
	// CommitAudit is the audit recorded when the version was committed; nil
	// if the envelope omitted commit_audit.
	CommitAudit rm.AuditDetailsLike
	// Contribution is the contribution this version was committed in; nil if
	// the envelope omitted contribution.
	Contribution rm.ObjectRefLike
	// Status is the EHR_STATUS this version holds, or nil when the version
	// carries no data.
	Status *rm.EHRStatus
}

// VersionUID returns this version's id as an [openehrclient.VersionUID], the
// string type [GetVersionByID] takes, so a read result can be passed straight
// back to a targeted fetch.
func (v *Version) VersionUID() openehrclient.VersionUID {
	return openehrclient.VersionUID(v.UID.Value)
}

// GetVersionedObject returns the VERSIONED_EHR_STATUS of ehrID: the version
// container's header (its uid, owner and creation time), not the EHR_STATUS
// at one version that [GetVersioned] returns. Use [GetRevisionHistory] for
// the change log and [GetVersion] for a version's content.
//
// Wire: GET /ehr/{ehr_id}/versioned_ehr_status.
func GetVersionedObject(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID) (*rm.VersionedEHRStatus, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("ehrstatus.GetVersionedObject: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(ehrID),
		Route:  routeVersionedObject,
	}
	out, meta, err := transport.Decode[rm.VersionedEHRStatus](ctx, c, req)
	return out, openehrclient.NewVersionMetadata(meta), err
}

// GetRevisionHistory returns the REVISION_HISTORY of ehrID's EHR_STATUS: one
// item per version, with the audits recorded for it.
//
// Wire: GET /ehr/{ehr_id}/versioned_ehr_status/revision_history.
func GetRevisionHistory(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID) (*rm.RevisionHistory, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("ehrstatus.GetRevisionHistory: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(ehrID) + "/revision_history",
		Route:  routeRevisionHistory,
	}
	out, meta, err := transport.Decode[rm.RevisionHistory](ctx, c, req)
	return out, openehrclient.NewVersionMetadata(meta), err
}

// GetVersion returns the latest version of ehrID's EHR_STATUS.
//
// Wire: GET /ehr/{ehr_id}/versioned_ehr_status/version, with no
// version_at_time.
func GetVersion(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID) (*Version, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("ehrstatus.GetVersion: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(ehrID) + "/version",
		Route:  routeVersion,
	}
	return getVersion(ctx, c, req)
}

// GetVersionAtTime returns the version of ehrID's EHR_STATUS that was current
// at t.
//
// Wire: GET /ehr/{ehr_id}/versioned_ehr_status/version?version_at_time={t},
// with t sent in UTC with its fraction of a second.
// Zero t is rejected, so pass a real time; use [GetVersion] for the latest.
func GetVersionAtTime(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, t time.Time) (*Version, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("ehrstatus.GetVersionAtTime: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if t.IsZero() {
		return nil, nil, fmt.Errorf("ehrstatus.GetVersionAtTime: %w: zero time — use GetVersion for the latest", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(ehrID) + "/version",
		Route:  routeVersion,
		Query:  url.Values{"version_at_time": []string{openehrclient.FormatVersionAtTime(t)}},
	}
	return getVersion(ctx, c, req)
}

// GetVersionByID returns the version of ehrID's EHR_STATUS identified by
// versionUID, with the version envelope around the EHR_STATUS (its id,
// preceding id, lifecycle state, commit audit and contribution), where
// [GetVersioned] returns the EHR_STATUS alone.
//
// Wire: GET /ehr/{ehr_id}/versioned_ehr_status/version/{version_uid}.
func GetVersionByID(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, versionUID openehrclient.VersionUID) (*Version, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("ehrstatus.GetVersionByID: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if versionUID == "" {
		return nil, nil, fmt.Errorf("ehrstatus.GetVersionByID: %w: empty VersionUID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(ehrID) + "/version/" + string(versionUID),
		Route:  routeVersionByID,
	}
	return getVersion(ctx, c, req)
}

// getVersion issues a version read and projects the ORIGINAL_VERSION
// envelope into a [Version]. The canonical JSON decode refuses a body whose
// _type is not ORIGINAL_VERSION.
func getVersion(ctx context.Context, c *transport.Client, req *transport.Request) (*Version, *openehrclient.VersionMetadata, error) {
	env, meta, err := transport.Decode[rm.OriginalVersion[rm.EHRStatus]](ctx, c, req)
	if err != nil {
		return nil, openehrclient.NewVersionMetadata(meta), err
	}
	return &Version{
		UID:                 env.UID,
		PrecedingVersionUID: env.PrecedingVersionUID,
		LifecycleState:      env.LifecycleState,
		CommitAudit:         env.CommitAudit,
		Contribution:        env.Contribution,
		Status:              env.Data,
	}, openehrclient.NewVersionMetadata(meta), nil
}

func (r *repository) GetVersionedObject(ctx context.Context, ehrID openehrclient.EHRID) (*rm.VersionedEHRStatus, *openehrclient.VersionMetadata, error) {
	return GetVersionedObject(ctx, r.c, ehrID)
}

func (r *repository) GetRevisionHistory(ctx context.Context, ehrID openehrclient.EHRID) (*rm.RevisionHistory, *openehrclient.VersionMetadata, error) {
	return GetRevisionHistory(ctx, r.c, ehrID)
}

func (r *repository) GetVersion(ctx context.Context, ehrID openehrclient.EHRID) (*Version, *openehrclient.VersionMetadata, error) {
	return GetVersion(ctx, r.c, ehrID)
}

func (r *repository) GetVersionAtTime(ctx context.Context, ehrID openehrclient.EHRID, t time.Time) (*Version, *openehrclient.VersionMetadata, error) {
	return GetVersionAtTime(ctx, r.c, ehrID, t)
}

func (r *repository) GetVersionByID(ctx context.Context, ehrID openehrclient.EHRID, versionUID openehrclient.VersionUID) (*Version, *openehrclient.VersionMetadata, error) {
	return GetVersionByID(ctx, r.c, ehrID, versionUID)
}
