package directory

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
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

// writeConfig is the resolved option set for Save / Update.
type writeConfig struct {
	prefer       transport.Prefer
	auditDetails *rm.AuditDetails
}

// WriteOption mutates the request shape for [Save] and [Update].
type WriteOption func(*writeConfig)

// WithPrefer overrides the response-shape preference (REQ-094).
// Default [transport.PreferMinimal] per the spec.
func WithPrefer(p transport.Prefer) WriteOption {
	return func(c *writeConfig) { c.prefer = p }
}

// WithAuditDetails attaches the commit-time audit envelope via the
// `openehr-audit-details` header (REQ-059). Nil omits the header.
func WithAuditDetails(a *rm.AuditDetails) WriteOption {
	return func(c *writeConfig) { c.auditDetails = a }
}

// deleteConfig is the resolved option set for [Delete].
type deleteConfig struct {
	auditDetails *rm.AuditDetails
}

// DeleteOption mutates [Delete]'s request shape.
type DeleteOption func(*deleteConfig)

// WithDeleteAudit attaches the commit-time audit envelope on a
// delete (REQ-059).
func WithDeleteAudit(a *rm.AuditDetails) DeleteOption {
	return func(c *deleteConfig) { c.auditDetails = a }
}

// Save creates the Directory under ehrID. Each EHR has at most one
// Directory; saving when one already exists is a server-side error
// (typically 409). Use [Update] to modify an existing Directory.
//
// Wire: POST /ehr/{ehr_id}/directory.
func Save(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, folder *rm.Folder, opts ...WriteOption) (*rm.OriginalVersion[*rm.Folder], *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("directory.Save: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if folder == nil {
		return nil, nil, fmt.Errorf("directory.Save: %w: nil Folder", transport.ErrInvalidConfig)
	}
	cfg := writeConfig{prefer: transport.PreferMinimal}
	for _, o := range opts {
		o(&cfg)
	}
	body, err := canjson.Marshal(folder)
	if err != nil {
		return nil, nil, fmt.Errorf("directory.Save: marshal body: %w", err)
	}
	auditHeader, err := marshalAuditDetails(cfg.auditDetails)
	if err != nil {
		return nil, nil, fmt.Errorf("directory.Save: %w", err)
	}
	req := &transport.Request{
		Method:             http.MethodPost,
		Path:               basePath(ehrID),
		Route:              routeTemplate,
		Body:               body,
		Prefer:             cfg.prefer,
		AuditDetailsHeader: auditHeader,
	}
	return doWrite(ctx, c, req, cfg.prefer)
}

// Update modifies the Directory under ehrID, requiring `ifMatch` per
// REQ-054. Errors map per REQ-093.
//
// Wire: PUT /ehr/{ehr_id}/directory with If-Match.
func Update(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, ifMatch string, folder *rm.Folder, opts ...WriteOption) (*rm.OriginalVersion[*rm.Folder], *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("directory.Update: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if ifMatch == "" {
		return nil, nil, fmt.Errorf("directory.Update: %w: empty If-Match (REQ-054)", transport.ErrInvalidConfig)
	}
	if folder == nil {
		return nil, nil, fmt.Errorf("directory.Update: %w: nil Folder", transport.ErrInvalidConfig)
	}
	cfg := writeConfig{prefer: transport.PreferMinimal}
	for _, o := range opts {
		o(&cfg)
	}
	body, err := canjson.Marshal(folder)
	if err != nil {
		return nil, nil, fmt.Errorf("directory.Update: marshal body: %w", err)
	}
	auditHeader, err := marshalAuditDetails(cfg.auditDetails)
	if err != nil {
		return nil, nil, fmt.Errorf("directory.Update: %w", err)
	}
	req := &transport.Request{
		Method:             http.MethodPut,
		Path:               basePath(ehrID),
		Route:              routeTemplate,
		Body:               body,
		IfMatch:            ifMatch,
		Prefer:             cfg.prefer,
		AuditDetailsHeader: auditHeader,
	}
	return doWrite(ctx, c, req, cfg.prefer)
}

// Delete logically deletes the Directory addressed by versionUID,
// requiring `ifMatch` per REQ-054.
//
// Wire: DELETE /ehr/{ehr_id}/directory with If-Match. Some deployments
// require the version UID in the path — the openEHR REST spec leaves
// the canonical path slightly under-specified; this binding follows
// the base-path form. If a deployment requires `/directory/{vuid}`,
// use [transport.Client.Do] with a custom request to override.
func Delete(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, ifMatch string, opts ...DeleteOption) (*openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, fmt.Errorf("directory.Delete: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if ifMatch == "" {
		return nil, fmt.Errorf("directory.Delete: %w: empty If-Match (REQ-054)", transport.ErrInvalidConfig)
	}
	cfg := deleteConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	auditHeader, err := marshalAuditDetails(cfg.auditDetails)
	if err != nil {
		return nil, fmt.Errorf("directory.Delete: %w", err)
	}
	req := &transport.Request{
		Method:             http.MethodDelete,
		Path:               basePath(ehrID),
		Route:              routeTemplate,
		IfMatch:            ifMatch,
		AuditDetailsHeader: auditHeader,
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return openehrclient.NewVersionMetadata(resp.Metadata), err
		}
		return nil, err
	}
	return openehrclient.NewVersionMetadata(resp.Metadata), nil
}

func doWrite(ctx context.Context, c *transport.Client, req *transport.Request, prefer transport.Prefer) (*rm.OriginalVersion[*rm.Folder], *openehrclient.VersionMetadata, error) {
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return nil, openehrclient.NewVersionMetadata(resp.Metadata), err
		}
		return nil, nil, err
	}
	meta := openehrclient.NewVersionMetadata(resp.Metadata)
	if prefer != transport.PreferRepresentation || len(resp.Body) == 0 {
		return nil, meta, nil
	}
	var version rm.OriginalVersion[*rm.Folder]
	if err := canjson.Unmarshal(resp.Body, &version); err != nil {
		return nil, meta, fmt.Errorf("directory: decode ORIGINAL_VERSION: %w", err)
	}
	return &version, meta, nil
}

func marshalAuditDetails(a *rm.AuditDetails) (string, error) {
	if a == nil {
		return "", nil
	}
	b, err := canjson.Marshal(a)
	if err != nil {
		return "", fmt.Errorf("marshal audit details: %w", err)
	}
	return string(b), nil
}

// Repository mirrors the package-level Directory functions.
type Repository interface {
	Get(ctx context.Context, ehrID openehrclient.EHRID) (*rm.Folder, *openehrclient.VersionMetadata, error)
	GetAtTime(ctx context.Context, ehrID openehrclient.EHRID, t time.Time) (*rm.Folder, *openehrclient.VersionMetadata, error)
	GetVersioned(ctx context.Context, ehrID openehrclient.EHRID, versionUID openehrclient.VersionUID) (*rm.Folder, *openehrclient.VersionMetadata, error)
	Save(ctx context.Context, ehrID openehrclient.EHRID, folder *rm.Folder, opts ...WriteOption) (*rm.OriginalVersion[*rm.Folder], *openehrclient.VersionMetadata, error)
	Update(ctx context.Context, ehrID openehrclient.EHRID, ifMatch string, folder *rm.Folder, opts ...WriteOption) (*rm.OriginalVersion[*rm.Folder], *openehrclient.VersionMetadata, error)
	Delete(ctx context.Context, ehrID openehrclient.EHRID, ifMatch string, opts ...DeleteOption) (*openehrclient.VersionMetadata, error)
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

func (r *repository) Save(ctx context.Context, id openehrclient.EHRID, folder *rm.Folder, opts ...WriteOption) (*rm.OriginalVersion[*rm.Folder], *openehrclient.VersionMetadata, error) {
	return Save(ctx, r.c, id, folder, opts...)
}

func (r *repository) Update(ctx context.Context, id openehrclient.EHRID, ifMatch string, folder *rm.Folder, opts ...WriteOption) (*rm.OriginalVersion[*rm.Folder], *openehrclient.VersionMetadata, error) {
	return Update(ctx, r.c, id, ifMatch, folder, opts...)
}

func (r *repository) Delete(ctx context.Context, id openehrclient.EHRID, ifMatch string, opts ...DeleteOption) (*openehrclient.VersionMetadata, error) {
	return Delete(ctx, r.c, id, ifMatch, opts...)
}
