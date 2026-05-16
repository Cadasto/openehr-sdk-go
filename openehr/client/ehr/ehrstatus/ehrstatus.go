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

// basePath returns the per-EHR EHR_STATUS base path. Kept local so the
// route template emitted on OTel spans is stable.
func basePath(ehrID openehrclient.EHRID) string {
	return "/ehr/" + url.PathEscape(string(ehrID)) + "/ehr_status"
}

const routeTemplate = "/ehr/{ehr_id}/ehr_status"
const routeVersioned = "/ehr/{ehr_id}/ehr_status/{version_uid}"

// Get returns the latest EHR_STATUS for ehrID.
//
// Wire: GET /ehr/{ehr_id}/ehr_status.
func Get(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID) (*rm.EHRStatus, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("ehrstatus.Get: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   basePath(ehrID),
		Route:  routeTemplate,
	}
	return decode(ctx, c, req)
}

// GetAtTime returns the EHR_STATUS that was current at t.
//
// Wire: GET /ehr/{ehr_id}/ehr_status?version_at_time={t}.
// Zero t is rejected — callers MUST pass a real time; use [Get] for
// the latest.
func GetAtTime(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, t time.Time) (*rm.EHRStatus, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("ehrstatus.GetAtTime: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if t.IsZero() {
		return nil, nil, fmt.Errorf("ehrstatus.GetAtTime: %w: zero time — use Get for the latest", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   basePath(ehrID),
		Route:  routeTemplate,
		Query: url.Values{
			"version_at_time": []string{t.UTC().Format(time.RFC3339)},
		},
	}
	return decode(ctx, c, req)
}

// GetVersioned returns the EHR_STATUS identified by versionUID.
//
// Wire: GET /ehr/{ehr_id}/ehr_status/{version_uid}.
func GetVersioned(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, versionUID openehrclient.VersionUID) (*rm.EHRStatus, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("ehrstatus.GetVersioned: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if versionUID == "" {
		return nil, nil, fmt.Errorf("ehrstatus.GetVersioned: %w: empty VersionUID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   basePath(ehrID) + "/" + url.PathEscape(string(versionUID)),
		Route:  routeVersioned,
	}
	return decode(ctx, c, req)
}

func decode(ctx context.Context, c *transport.Client, req *transport.Request) (*rm.EHRStatus, *openehrclient.VersionMetadata, error) {
	out, meta, err := transport.Decode[rm.EHRStatus](ctx, c, req)
	return out, newVersionMetadata(meta), err
}

// newVersionMetadata is a thin wrapper around the ehr package helper
// so this file can call it without re-exporting it.
func newVersionMetadata(m *transport.Metadata) *openehrclient.VersionMetadata {
	return openehrclient.NewVersionMetadata(m)
}

// Repository mirrors the package functions for DI seams.
type Repository interface {
	Get(ctx context.Context, ehrID openehrclient.EHRID) (*rm.EHRStatus, *openehrclient.VersionMetadata, error)
	GetAtTime(ctx context.Context, ehrID openehrclient.EHRID, t time.Time) (*rm.EHRStatus, *openehrclient.VersionMetadata, error)
	GetVersioned(ctx context.Context, ehrID openehrclient.EHRID, versionUID openehrclient.VersionUID) (*rm.EHRStatus, *openehrclient.VersionMetadata, error)
}

// NewRepository binds c to a Repository.
func NewRepository(c *transport.Client) Repository { return &repository{c: c} }

type repository struct{ c *transport.Client }

func (r *repository) Get(ctx context.Context, id openehrclient.EHRID) (*rm.EHRStatus, *openehrclient.VersionMetadata, error) {
	return Get(ctx, r.c, id)
}

func (r *repository) GetAtTime(ctx context.Context, id openehrclient.EHRID, t time.Time) (*rm.EHRStatus, *openehrclient.VersionMetadata, error) {
	return GetAtTime(ctx, r.c, id, t)
}

func (r *repository) GetVersioned(ctx context.Context, id openehrclient.EHRID, uid openehrclient.VersionUID) (*rm.EHRStatus, *openehrclient.VersionMetadata, error) {
	return GetVersioned(ctx, r.c, id, uid)
}
