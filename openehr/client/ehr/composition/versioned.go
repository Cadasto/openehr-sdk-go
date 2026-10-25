package composition

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

// versionedBase is the path of the VERSIONED_COMPOSITION voID in the EHR
// ehrID. Its revision history and its versions are sub-paths of it, and all
// four reads are GET.
func versionedBase(ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID) string {
	return "/ehr/" + string(ehrID) + "/versioned_composition/" + string(voID)
}

const (
	routeVersioned       = "/ehr/{ehr_id}/versioned_composition/{versioned_object_uid}"
	routeRevisionHistory = "/ehr/{ehr_id}/versioned_composition/{versioned_object_uid}/revision_history"
	routeVersion         = "/ehr/{ehr_id}/versioned_composition/{versioned_object_uid}/version"
	routeVersionByID     = "/ehr/{ehr_id}/versioned_composition/{versioned_object_uid}/version/{version_uid}"
)

// Version is one decoded VERSION<COMPOSITION>: the ORIGINAL_VERSION envelope
// a version read returns, with its data decoded as a COMPOSITION. [Get]
// returns the COMPOSITION alone; the version reads return the envelope around
// it, which says when, by whom and in which contribution it was committed.
//
// It is a curated projection of ORIGINAL_VERSION: the envelope's
// attestations, other_input_version_uids and signature are not surfaced. A
// version read reads only an ORIGINAL_VERSION; an IMPORTED_VERSION fails as a
// [*transport.DecodeError].
type Version struct {
	// UID is this version's OBJECT_VERSION_ID. Its Value is the string
	// [GetVersionByID] takes; see [Version.VersionUID].
	UID rm.ObjectVersionID
	// PrecedingVersionUID is the version this one follows, or nil for the
	// first version.
	PrecedingVersionUID *rm.ObjectVersionID
	// LifecycleState is the openEHR version lifecycle state code.
	LifecycleState rm.DVCodedText
	// CommitAudit is the audit of the commit; nil if the envelope omitted
	// commit_audit.
	CommitAudit rm.AuditDetailsLike
	// Contribution is the contribution this version was committed in; nil if
	// the envelope omitted contribution.
	Contribution rm.ObjectRefLike
	// Composition is the version's data, or nil when the version carries
	// none, as a logical deletion may.
	Composition *rm.Composition
}

// VersionUID returns this version's id as an [openehrclient.VersionUID], the
// type [GetVersionByID] takes, so a version read's result can be passed
// straight back to a read of that version.
func (v *Version) VersionUID() openehrclient.VersionUID {
	return openehrclient.VersionUID(v.UID.Value)
}

// GetVersionedObject retrieves the VERSIONED_COMPOSITION voID in the EHR
// ehrID: the version container's uid, owner and creation time. Use
// [GetRevisionHistory] for its change log and [GetVersion] for a version's
// content.
//
// Wire: GET /ehr/{ehr_id}/versioned_composition/{versioned_object_uid}.
func GetVersionedObject(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID) (*rm.VersionedComposition, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("composition.GetVersionedObject: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if voID == "" {
		return nil, nil, fmt.Errorf("composition.GetVersionedObject: %w: empty VersionedObjectID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(ehrID, voID),
		Route:  routeVersioned,
	}
	out, meta, err := transport.Decode[rm.VersionedComposition](ctx, c, req)
	return out, openehrclient.NewVersionMetadata(meta), err
}

// GetRevisionHistory retrieves the REVISION_HISTORY of the composition voID
// in the EHR ehrID: one item per version, with the audits of its commit.
//
// Wire: GET /ehr/{ehr_id}/versioned_composition/{versioned_object_uid}/revision_history.
func GetRevisionHistory(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID) (*rm.RevisionHistory, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("composition.GetRevisionHistory: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if voID == "" {
		return nil, nil, fmt.Errorf("composition.GetRevisionHistory: %w: empty VersionedObjectID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(ehrID, voID) + "/revision_history",
		Route:  routeRevisionHistory,
	}
	out, meta, err := transport.Decode[rm.RevisionHistory](ctx, c, req)
	return out, openehrclient.NewVersionMetadata(meta), err
}

// GetVersion retrieves the latest version of the composition voID in the EHR
// ehrID. Use [GetVersionAtTime] for the version that was current at a given
// time.
//
// Wire: GET /ehr/{ehr_id}/versioned_composition/{versioned_object_uid}/version,
// with no version_at_time.
func GetVersion(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID) (*Version, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("composition.GetVersion: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if voID == "" {
		return nil, nil, fmt.Errorf("composition.GetVersion: %w: empty VersionedObjectID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(ehrID, voID) + "/version",
		Route:  routeVersion,
	}
	return getVersion(ctx, c, req)
}

// GetVersionAtTime retrieves the version of the composition voID in the EHR
// ehrID that was current at t. A zero t fails with
// [transport.ErrInvalidConfig] before any request: [GetVersion] is the read
// for the latest version.
//
// Wire: GET /ehr/{ehr_id}/versioned_composition/{versioned_object_uid}/version?version_at_time={t},
// with t sent in UTC with its fraction of a second.
func GetVersionAtTime(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID, t time.Time) (*Version, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("composition.GetVersionAtTime: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if voID == "" {
		return nil, nil, fmt.Errorf("composition.GetVersionAtTime: %w: empty VersionedObjectID", transport.ErrInvalidConfig)
	}
	if t.IsZero() {
		return nil, nil, fmt.Errorf("composition.GetVersionAtTime: %w: zero time — use GetVersion for the latest", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(ehrID, voID) + "/version",
		Route:  routeVersion,
		Query:  url.Values{"version_at_time": []string{openehrclient.FormatVersionAtTime(t)}},
	}
	return getVersion(ctx, c, req)
}

// GetVersionByID retrieves the version versionUID of the composition voID in
// the EHR ehrID.
//
// Wire: GET /ehr/{ehr_id}/versioned_composition/{versioned_object_uid}/version/{version_uid}.
func GetVersionByID(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID, versionUID openehrclient.VersionUID) (*Version, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("composition.GetVersionByID: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if voID == "" {
		return nil, nil, fmt.Errorf("composition.GetVersionByID: %w: empty VersionedObjectID", transport.ErrInvalidConfig)
	}
	if versionUID == "" {
		return nil, nil, fmt.Errorf("composition.GetVersionByID: %w: empty VersionUID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(ehrID, voID) + "/version/" + string(versionUID),
		Route:  routeVersionByID,
	}
	return getVersion(ctx, c, req)
}

// getVersion issues a version read, decodes the ORIGINAL_VERSION<COMPOSITION>
// envelope, and projects it into a [Version].
func getVersion(ctx context.Context, c *transport.Client, req *transport.Request) (*Version, *openehrclient.VersionMetadata, error) {
	env, meta, err := transport.Decode[rm.OriginalVersion[rm.Composition]](ctx, c, req)
	if err != nil {
		return nil, openehrclient.NewVersionMetadata(meta), err
	}
	return &Version{
		UID:                 env.UID,
		PrecedingVersionUID: env.PrecedingVersionUID,
		LifecycleState:      env.LifecycleState,
		CommitAudit:         env.CommitAudit,
		Contribution:        env.Contribution,
		Composition:         env.Data,
	}, openehrclient.NewVersionMetadata(meta), nil
}

func (r *repository) GetVersionedObject(ctx context.Context, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID) (*rm.VersionedComposition, *openehrclient.VersionMetadata, error) {
	return GetVersionedObject(ctx, r.c, ehrID, voID)
}

func (r *repository) GetRevisionHistory(ctx context.Context, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID) (*rm.RevisionHistory, *openehrclient.VersionMetadata, error) {
	return GetRevisionHistory(ctx, r.c, ehrID, voID)
}

func (r *repository) GetVersion(ctx context.Context, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID) (*Version, *openehrclient.VersionMetadata, error) {
	return GetVersion(ctx, r.c, ehrID, voID)
}

func (r *repository) GetVersionAtTime(ctx context.Context, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID, t time.Time) (*Version, *openehrclient.VersionMetadata, error) {
	return GetVersionAtTime(ctx, r.c, ehrID, voID, t)
}

func (r *repository) GetVersionByID(ctx context.Context, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID, versionUID openehrclient.VersionUID) (*Version, *openehrclient.VersionMetadata, error) {
	return GetVersionByID(ctx, r.c, ehrID, voID, versionUID)
}
