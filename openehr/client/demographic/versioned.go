package demographic

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
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

// PartyVersion is a decoded VERSION<PARTY>: the commit envelope
// (ORIGINAL_VERSION) plus the polymorphically-decoded PARTY payload. The
// envelope's `data` is decoded by `_type` into [Party]; the audit / lifecycle
// / version-id fields come straight from the envelope.
//
// It is a curated projection of ORIGINAL_VERSION: the envelope's
// `attestations`, `other_input_version_uids`, and `signature` are
// not surfaced (the read family is not signature- or merge-aware). Read the raw VERSION via the CDR if those are needed.
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

// VersionUID returns this version's id as an [ehr.VersionUID], the string
// newtype the read paths take (e.g. [GetVersionByID]), so a read result can
// be passed straight back to a targeted fetch.
func (pv *PartyVersion) VersionUID() openehrclient.VersionUID {
	return openehrclient.VersionUID(pv.UID.Value)
}

// GetVersionedParty retrieves the VERSIONED_PARTY container for voUID: the
// version-control header (owner, time created, uid). Use [GetRevisionHistory]
// for the change log and [GetVersion] for version content.
//
// Like every versioned_party read, it asks for canonical JSON unless
// [WithGetFormat] names canonical XML, and [WithRawBody] hands back the body
// as the server sent it. An empty or null 2xx body, a 204 included, fails
// with [transport.ErrInvalidShape], and a body that does not decode returns
// a [*transport.DecodeError], each beside the response metadata.
//
// Wire: GET /demographic/versioned_party/{vo_uid}.
func GetVersionedParty(ctx context.Context, c *transport.Client, voUID openehrclient.VersionedObjectID, opts ...GetOption) (*rm.VersionedParty, *openehrclient.VersionMetadata, error) {
	if voUID == "" {
		return nil, nil, fmt.Errorf("demographic.GetVersionedParty: %w: empty VersionedObjectID", transport.ErrInvalidConfig)
	}
	cfg, err := newGetConfig("demographic.GetVersionedParty", opts)
	if err != nil {
		return nil, nil, err
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(voUID),
		Route:  routeVersioned,
		Accept: cfg.format.MediaType(),
	}
	return readVersioned[rm.VersionedParty](ctx, c, req, cfg)
}

// GetRevisionHistory retrieves the REVISION_HISTORY of the PARTY family voUID.
// The read options and outcomes are those of [GetVersionedParty].
//
// Wire: GET /demographic/versioned_party/{vo_uid}/revision_history.
func GetRevisionHistory(ctx context.Context, c *transport.Client, voUID openehrclient.VersionedObjectID, opts ...GetOption) (*rm.RevisionHistory, *openehrclient.VersionMetadata, error) {
	if voUID == "" {
		return nil, nil, fmt.Errorf("demographic.GetRevisionHistory: %w: empty VersionedObjectID", transport.ErrInvalidConfig)
	}
	cfg, err := newGetConfig("demographic.GetRevisionHistory", opts)
	if err != nil {
		return nil, nil, err
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(voUID) + "/revision_history",
		Route:  routeRevisionHistory,
		Accept: cfg.format.MediaType(),
	}
	return readVersioned[rm.RevisionHistory](ctx, c, req, cfg)
}

// readVersioned issues a VERSIONED_PARTY or REVISION_HISTORY read and
// decodes the body as T in the format cfg names, as [transport.Decode]
// answers a canonical JSON read: an empty or null 2xx body is
// [transport.ErrInvalidShape], and a body that does not decode a
// [*transport.DecodeError] carrying it, each beside the metadata.
func readVersioned[T any](ctx context.Context, c *transport.Client, req *transport.Request, cfg getConfig) (*T, *openehrclient.VersionMetadata, error) {
	resp, meta, err := cfg.do(ctx, c, req)
	if err != nil {
		return nil, meta, err
	}
	if transport.IsNoRepresentationBody(resp.Body) {
		return nil, meta, fmt.Errorf("demographic: %w: %d response with empty or null body", transport.ErrInvalidShape, resp.StatusCode)
	}
	out := new(T)
	if err := cfg.format.unmarshal(resp.Body, out); err != nil {
		return nil, meta, &transport.DecodeError{
			Method: req.Method, Route: req.Route, Body: resp.Body, Inner: err,
		}
	}
	return out, meta, nil
}

// GetVersion retrieves the latest VERSION of the PARTY family voUID. A 2xx
// response with an empty or null body, a 204 included, fails with
// [transport.ErrInvalidShape] beside the response metadata, as it does on
// [GetVersionAtTime] and [GetVersionByID]. The read options are those of
// [GetVersionedParty]; in canonical XML the PARTY in the version's `data`
// is decoded by the element's `xsi:type`.
//
// Wire: GET /demographic/versioned_party/{vo_uid}/version.
func GetVersion(ctx context.Context, c *transport.Client, voUID openehrclient.VersionedObjectID, opts ...GetOption) (*PartyVersion, *openehrclient.VersionMetadata, error) {
	if voUID == "" {
		return nil, nil, fmt.Errorf("demographic.GetVersion: %w: empty VersionedObjectID", transport.ErrInvalidConfig)
	}
	cfg, err := newGetConfig("demographic.GetVersion", opts)
	if err != nil {
		return nil, nil, err
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(voUID) + "/version",
		Route:  routeVersion,
		Accept: cfg.format.MediaType(),
	}
	return getVersion(ctx, c, req, cfg)
}

// GetVersionAtTime retrieves the VERSION that was current at t. A zero t
// fails with [transport.ErrInvalidConfig] before any request: [GetVersion]
// is the read for the latest. A t whose UTC year falls outside 0000-9999
// fails the same way, since RFC 3339 cannot write it. When the family had no
// version at t, the server answers 404 and the read fails with
// [transport.ErrNotFound]. The read options are those of [GetVersion].
//
// Wire: GET /demographic/versioned_party/{vo_uid}/version?version_at_time={t},
// with t sent in UTC with its fraction of a second.
func GetVersionAtTime(ctx context.Context, c *transport.Client, voUID openehrclient.VersionedObjectID, t time.Time, opts ...GetOption) (*PartyVersion, *openehrclient.VersionMetadata, error) {
	if voUID == "" {
		return nil, nil, fmt.Errorf("demographic.GetVersionAtTime: %w: empty VersionedObjectID", transport.ErrInvalidConfig)
	}
	if t.IsZero() {
		return nil, nil, fmt.Errorf("demographic.GetVersionAtTime: %w: zero time — use GetVersion for the latest", transport.ErrInvalidConfig)
	}
	at, err := openehrclient.FormatVersionAtTime(t)
	if err != nil {
		return nil, nil, fmt.Errorf("demographic.GetVersionAtTime: %w", err)
	}
	cfg, err := newGetConfig("demographic.GetVersionAtTime", opts)
	if err != nil {
		return nil, nil, err
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(voUID) + "/version",
		Route:  routeVersion,
		Query:  url.Values{"version_at_time": []string{at}},
		Accept: cfg.format.MediaType(),
	}
	return getVersion(ctx, c, req, cfg)
}

// GetVersionByID retrieves a specific VERSION by its version uid. The read
// options are those of [GetVersion].
//
// Wire: GET /demographic/versioned_party/{vo_uid}/version/{version_uid}.
func GetVersionByID(ctx context.Context, c *transport.Client, voUID openehrclient.VersionedObjectID, versionUID openehrclient.VersionUID, opts ...GetOption) (*PartyVersion, *openehrclient.VersionMetadata, error) {
	if voUID == "" {
		return nil, nil, fmt.Errorf("demographic.GetVersionByID: %w: empty VersionedObjectID", transport.ErrInvalidConfig)
	}
	if versionUID == "" {
		return nil, nil, fmt.Errorf("demographic.GetVersionByID: %w: empty VersionUID", transport.ErrInvalidConfig)
	}
	cfg, err := newGetConfig("demographic.GetVersionByID", opts)
	if err != nil {
		return nil, nil, err
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   versionedBase(voUID) + "/version/" + string(versionUID),
		Route:  routeVersionByID,
		Accept: cfg.format.MediaType(),
	}
	return getVersion(ctx, c, req, cfg)
}

// getVersion issues a version read and decodes the ORIGINAL_VERSION<PARTY>
// envelope in the format cfg names. In canonical JSON the envelope's
// polymorphic `data` is captured raw (the generated OriginalVersion JSON
// unmarshaller cannot decode into the abstract [rm.Party] interface) and
// re-decoded by `_type` through the type registry (REQ-040). In canonical
// XML the generated unmarshaller decodes `data` by its `xsi:type`.
func getVersion(ctx context.Context, c *transport.Client, req *transport.Request, cfg getConfig) (*PartyVersion, *openehrclient.VersionMetadata, error) {
	resp, meta, err := cfg.do(ctx, c, req)
	if err != nil {
		return nil, meta, err
	}
	if transport.IsNoRepresentationBody(resp.Body) {
		// The pin answers these reads 200 with a VERSION or 404, no version
		// at the requested time included, so every empty or null 2xx body,
		// a 204 too, is a wire anomaly. Unlike getParty there is no 204 to
		// carve out.
		return nil, meta, fmt.Errorf("demographic: %w: %d response with empty or null body", transport.ErrInvalidShape, resp.StatusCode)
	}
	if cfg.format == FormatXML {
		var env rm.OriginalVersion[rm.Party]
		if err := unmarshalXML(resp.Body, &env); err != nil {
			return nil, meta, fmt.Errorf("demographic: ORIGINAL_VERSION envelope: %w", &transport.DecodeError{
				Method: req.Method, Route: req.Route, Body: resp.Body, Inner: err,
			})
		}
		pv := &PartyVersion{
			UID:                 env.UID,
			PrecedingVersionUID: env.PrecedingVersionUID,
			LifecycleState:      env.LifecycleState,
			CommitAudit:         env.CommitAudit,
			Contribution:        env.Contribution,
		}
		if env.Data != nil {
			pv.Party = *env.Data
		}
		return pv, meta, nil
	}
	var env rm.OriginalVersion[jsontext.Value]
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, meta, fmt.Errorf("demographic: ORIGINAL_VERSION envelope: %w", &transport.DecodeError{
			Method: req.Method, Route: req.Route, Body: resp.Body, Inner: err,
		})
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
			// Body carries the full response, not the `data` sub-slice this
			// decode was reading: REQ-151 hands back the bytes the server
			// delivered, so the caller's recovery is the same whichever of
			// getVersion's two decodes tripped.
			return nil, meta, fmt.Errorf("demographic: VERSION data: %w", &transport.DecodeError{
				Method: req.Method, Route: req.Route, Body: resp.Body, Inner: err,
			})
		}
		pv.Party = party
	}
	return pv, meta, nil
}

func (r *repository) GetVersionedParty(ctx context.Context, voUID openehrclient.VersionedObjectID, opts ...GetOption) (*rm.VersionedParty, *openehrclient.VersionMetadata, error) {
	return GetVersionedParty(ctx, r.c, voUID, opts...)
}

func (r *repository) GetRevisionHistory(ctx context.Context, voUID openehrclient.VersionedObjectID, opts ...GetOption) (*rm.RevisionHistory, *openehrclient.VersionMetadata, error) {
	return GetRevisionHistory(ctx, r.c, voUID, opts...)
}

func (r *repository) GetVersion(ctx context.Context, voUID openehrclient.VersionedObjectID, opts ...GetOption) (*PartyVersion, *openehrclient.VersionMetadata, error) {
	return GetVersion(ctx, r.c, voUID, opts...)
}

func (r *repository) GetVersionAtTime(ctx context.Context, voUID openehrclient.VersionedObjectID, t time.Time, opts ...GetOption) (*PartyVersion, *openehrclient.VersionMetadata, error) {
	return GetVersionAtTime(ctx, r.c, voUID, t, opts...)
}

func (r *repository) GetVersionByID(ctx context.Context, voUID openehrclient.VersionedObjectID, versionUID openehrclient.VersionUID, opts ...GetOption) (*PartyVersion, *openehrclient.VersionMetadata, error) {
	return GetVersionByID(ctx, r.c, voUID, versionUID, opts...)
}
