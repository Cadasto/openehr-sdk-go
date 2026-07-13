package demographic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// versionedBase is the read-only VERSIONED_PARTY resource path. The
// Demographic API mirrors the EHR `versioned_composition` family: a single
// container keyed by the versioned-object uid, with revision-history and
// per-version sub-paths. All four endpoints are GET-only.
func versionedBase(voUID openehrclient.VersionedObjectID) string {
	return "/demographic/versioned_party/" + string(voUID)
}

const (
	routeVersioned       = "/demographic/versioned_party/{versioned_object_uid}"
	routeRevisionHistory = "/demographic/versioned_party/{versioned_object_uid}/revision_history"
	routeVersion         = "/demographic/versioned_party/{versioned_object_uid}/version"
	routeVersionByID     = "/demographic/versioned_party/{versioned_object_uid}/version/{version_uid}"
)

// PartyVersion is a decoded VERSION<PARTY> — the commit envelope
// (ORIGINAL_VERSION) plus the polymorphically-decoded PARTY payload. The
// envelope's `data` is decoded by `_type` into [Party]; the audit / lifecycle
// / version-id fields come straight from the envelope.
//
// It is a curated projection of ORIGINAL_VERSION: the envelope's
// `attestations`, `other_input_version_uids`, and `signature` are
// intentionally not surfaced (the read family is not signature-/merge-aware
// yet). Read the raw VERSION via the CDR if those are needed.
type PartyVersion struct {
	// UID is this version's OBJECT_VERSION_ID. Its Value is the wire string
	// you pass back as an [ehr.VersionUID]; see [PartyVersion.VersionUID].
	UID rm.ObjectVersionID
	// PrecedingVersionUID is the prior version, or nil for the first.
	PrecedingVersionUID *rm.ObjectVersionID
	// LifecycleState is the openEHR version-lifecycle code.
	LifecycleState rm.DVCodedText
	// CommitAudit is the commit-time audit envelope; nil if the VERSION
	// omitted commit_audit.
	CommitAudit rm.AuditDetailsLike
	// Contribution is the contribution this version was committed under; nil
	// if the VERSION omitted contribution.
	Contribution rm.ObjectRefLike
	// Party is the decoded version content (the concrete PARTY type), or nil
	// when the envelope carried no data (e.g. a content-free version).
	Party rm.Party
}

// VersionUID returns this version's id as an [ehr.VersionUID] — the string
// newtype the read paths take (e.g. [GetVersionByID]) — closing the
// round-trip from a read back to a targeted fetch.
func (pv *PartyVersion) VersionUID() openehrclient.VersionUID {
	return openehrclient.VersionUID(pv.UID.Value)
}

// GetVersionedParty retrieves the VERSIONED_PARTY container for voUID — the
// version-control header (owner, time created, uid). Use [GetRevisionHistory]
// for the change log and [GetVersion] for version content.
//
// Wire: GET /demographic/versioned_party/{vo_uid}.
func GetVersionedParty(ctx context.Context, c *transport.Client, voUID openehrclient.VersionedObjectID) (*rm.VersionedParty, *openehrclient.VersionMetadata, error) {
	if voUID == "" {
		return nil, nil, fmt.Errorf("demographic.GetVersionedParty: %w: empty VersionedObjectID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(voUID),
		Route:  routeVersioned,
	}
	out, meta, err := transport.Decode[rm.VersionedParty](ctx, c, req)
	return out, openehrclient.NewVersionMetadata(meta), err
}

// GetRevisionHistory retrieves the REVISION_HISTORY of the PARTY family voUID.
//
// Wire: GET /demographic/versioned_party/{vo_uid}/revision_history.
func GetRevisionHistory(ctx context.Context, c *transport.Client, voUID openehrclient.VersionedObjectID) (*rm.RevisionHistory, *openehrclient.VersionMetadata, error) {
	if voUID == "" {
		return nil, nil, fmt.Errorf("demographic.GetRevisionHistory: %w: empty VersionedObjectID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(voUID) + "/revision_history",
		Route:  routeRevisionHistory,
	}
	out, meta, err := transport.Decode[rm.RevisionHistory](ctx, c, req)
	return out, openehrclient.NewVersionMetadata(meta), err
}

// GetVersion retrieves the latest VERSION of the PARTY family voUID.
//
// Wire: GET /demographic/versioned_party/{vo_uid}/version.
func GetVersion(ctx context.Context, c *transport.Client, voUID openehrclient.VersionedObjectID) (*PartyVersion, *openehrclient.VersionMetadata, error) {
	if voUID == "" {
		return nil, nil, fmt.Errorf("demographic.GetVersion: %w: empty VersionedObjectID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(voUID) + "/version",
		Route:  routeVersion,
	}
	return getVersion(ctx, c, req)
}

// GetVersionAtTime retrieves the VERSION that was current at t.
//
// Wire: GET /demographic/versioned_party/{vo_uid}/version?version_at_time={t}.
func GetVersionAtTime(ctx context.Context, c *transport.Client, voUID openehrclient.VersionedObjectID, t time.Time) (*PartyVersion, *openehrclient.VersionMetadata, error) {
	if voUID == "" {
		return nil, nil, fmt.Errorf("demographic.GetVersionAtTime: %w: empty VersionedObjectID", transport.ErrInvalidConfig)
	}
	if t.IsZero() {
		return nil, nil, fmt.Errorf("demographic.GetVersionAtTime: %w: zero time — use GetVersion for the latest", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(voUID) + "/version",
		Route:  routeVersion,
		Query:  url.Values{"version_at_time": []string{t.UTC().Format(time.RFC3339)}},
	}
	return getVersion(ctx, c, req)
}

// GetVersionByID retrieves a specific VERSION by its version uid.
//
// Wire: GET /demographic/versioned_party/{vo_uid}/version/{version_uid}.
func GetVersionByID(ctx context.Context, c *transport.Client, voUID openehrclient.VersionedObjectID, versionUID openehrclient.VersionUID) (*PartyVersion, *openehrclient.VersionMetadata, error) {
	if voUID == "" {
		return nil, nil, fmt.Errorf("demographic.GetVersionByID: %w: empty VersionedObjectID", transport.ErrInvalidConfig)
	}
	if versionUID == "" {
		return nil, nil, fmt.Errorf("demographic.GetVersionByID: %w: empty VersionUID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(voUID) + "/version/" + string(versionUID),
		Route:  routeVersionByID,
	}
	return getVersion(ctx, c, req)
}

// getVersion issues a version read and decodes the ORIGINAL_VERSION<PARTY>
// envelope. The envelope's polymorphic `data` is captured raw (the generated
// OriginalVersion unmarshaller cannot decode into the abstract [rm.Party]
// interface) and re-decoded by `_type` through the type registry (REQ-040).
func getVersion(ctx context.Context, c *transport.Client, req *transport.Request) (*PartyVersion, *openehrclient.VersionMetadata, error) {
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return nil, openehrclient.NewVersionMetadata(resp.Metadata), err
		}
		return nil, nil, err
	}
	meta := openehrclient.NewVersionMetadata(resp.Metadata)
	if len(resp.Body) == 0 {
		// Mirror getParty: only a 204 legitimately carries no version body
		// (a version_at_time with no match); any other empty-body 2xx is a
		// wire anomaly surfaced as ErrInvalidShape.
		if resp.StatusCode == http.StatusNoContent {
			return nil, meta, nil
		}
		return nil, meta, fmt.Errorf("demographic: %w: %d response with empty body", transport.ErrInvalidShape, resp.StatusCode)
	}
	var env rm.OriginalVersion[json.RawMessage]
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, meta, fmt.Errorf("demographic: decode ORIGINAL_VERSION envelope: %w", err)
	}
	pv := &PartyVersion{
		UID:                 env.UID,
		PrecedingVersionUID: env.PrecedingVersionUID,
		LifecycleState:      env.LifecycleState,
		CommitAudit:         env.CommitAudit,
		Contribution:        env.Contribution,
	}
	if env.Data != nil && len(*env.Data) > 0 {
		party, err := typereg.DecodeAs[rm.Party](*env.Data)
		if err != nil {
			return nil, meta, fmt.Errorf("demographic: decode VERSION data: %w", err)
		}
		pv.Party = party
	}
	return pv, meta, nil
}

func (r *repository) GetVersionedParty(ctx context.Context, voUID openehrclient.VersionedObjectID) (*rm.VersionedParty, *openehrclient.VersionMetadata, error) {
	return GetVersionedParty(ctx, r.c, voUID)
}

func (r *repository) GetRevisionHistory(ctx context.Context, voUID openehrclient.VersionedObjectID) (*rm.RevisionHistory, *openehrclient.VersionMetadata, error) {
	return GetRevisionHistory(ctx, r.c, voUID)
}

func (r *repository) GetVersion(ctx context.Context, voUID openehrclient.VersionedObjectID) (*PartyVersion, *openehrclient.VersionMetadata, error) {
	return GetVersion(ctx, r.c, voUID)
}

func (r *repository) GetVersionAtTime(ctx context.Context, voUID openehrclient.VersionedObjectID, t time.Time) (*PartyVersion, *openehrclient.VersionMetadata, error) {
	return GetVersionAtTime(ctx, r.c, voUID, t)
}

func (r *repository) GetVersionByID(ctx context.Context, voUID openehrclient.VersionedObjectID, versionUID openehrclient.VersionUID) (*PartyVersion, *openehrclient.VersionMetadata, error) {
	return GetVersionByID(ctx, r.c, voUID, versionUID)
}
