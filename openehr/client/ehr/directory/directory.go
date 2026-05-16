package directory

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

const routeTemplate = "/ehr/{ehr_id}/directory"
const routeVersioned = "/ehr/{ehr_id}/directory/{version_uid}"

func basePath(ehrID openehrclient.EHRID) string {
	return "/ehr/" + url.PathEscape(string(ehrID)) + "/directory"
}

// Get returns the latest Directory FOLDER for ehrID.
//
// Wire: GET /ehr/{ehr_id}/directory.
func Get(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID) (*rm.Folder, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("directory.Get: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   basePath(ehrID),
		Route:  routeTemplate,
	}
	return decode(ctx, c, req)
}

// GetAtTime returns the Directory that was current at t.
//
// Wire: GET /ehr/{ehr_id}/directory?version_at_time={t}.
func GetAtTime(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, t time.Time) (*rm.Folder, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("directory.GetAtTime: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if t.IsZero() {
		return nil, nil, fmt.Errorf("directory.GetAtTime: %w: zero time — use Get for the latest", transport.ErrInvalidConfig)
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

// GetVersioned returns the Directory identified by versionUID.
//
// Wire: GET /ehr/{ehr_id}/directory/{version_uid}.
func GetVersioned(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, versionUID openehrclient.VersionUID) (*rm.Folder, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("directory.GetVersioned: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if versionUID == "" {
		return nil, nil, fmt.Errorf("directory.GetVersioned: %w: empty VersionUID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   basePath(ehrID) + "/" + url.PathEscape(string(versionUID)),
		Route:  routeVersioned,
	}
	return decode(ctx, c, req)
}

func decode(ctx context.Context, c *transport.Client, req *transport.Request) (*rm.Folder, *openehrclient.VersionMetadata, error) {
	out, meta, err := transport.Decode[rm.Folder](ctx, c, req)
	return out, openehrclient.NewVersionMetadata(meta), err
}

// Repository mirrors the package-level Directory functions.
type Repository interface {
	Get(ctx context.Context, ehrID openehrclient.EHRID) (*rm.Folder, *openehrclient.VersionMetadata, error)
	GetAtTime(ctx context.Context, ehrID openehrclient.EHRID, t time.Time) (*rm.Folder, *openehrclient.VersionMetadata, error)
	GetVersioned(ctx context.Context, ehrID openehrclient.EHRID, versionUID openehrclient.VersionUID) (*rm.Folder, *openehrclient.VersionMetadata, error)
}

// NewRepository binds c to a Repository.
func NewRepository(c *transport.Client) Repository { return &repository{c: c} }

type repository struct{ c *transport.Client }

func (r *repository) Get(ctx context.Context, id openehrclient.EHRID) (*rm.Folder, *openehrclient.VersionMetadata, error) {
	return Get(ctx, r.c, id)
}

func (r *repository) GetAtTime(ctx context.Context, id openehrclient.EHRID, t time.Time) (*rm.Folder, *openehrclient.VersionMetadata, error) {
	return GetAtTime(ctx, r.c, id, t)
}

func (r *repository) GetVersioned(ctx context.Context, id openehrclient.EHRID, uid openehrclient.VersionUID) (*rm.Folder, *openehrclient.VersionMetadata, error) {
	return GetVersioned(ctx, r.c, id, uid)
}
