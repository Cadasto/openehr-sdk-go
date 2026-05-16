package ehrstatus

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

// putConfig is the resolved option set for [Put].
type putConfig struct {
	prefer       transport.Prefer
	auditDetails *rm.AuditDetails
}

// PutOption mutates [Put]'s request shape.
type PutOption func(*putConfig)

// WithPrefer overrides the response-shape preference (REQ-094). The
// default is [transport.PreferMinimal] per the spec's write-path rule.
func WithPrefer(p transport.Prefer) PutOption {
	return func(c *putConfig) { c.prefer = p }
}

// WithAuditDetails attaches the commit-time audit envelope as the
// `openehr-audit-details` header (REQ-059). The struct is canjson-
// encoded; nil omits the header.
func WithAuditDetails(a *rm.AuditDetails) PutOption {
	return func(c *putConfig) { c.auditDetails = a }
}

// Put updates the EHR_STATUS under ehrID. `ifMatch` is the
// preceding version's identifier (typically captured from a previous
// GET's [openehrclient.VersionMetadata.VersionUID] or `ETag`) and is
// REQUIRED per REQ-054 — an empty value returns
// [transport.ErrInvalidConfig] without issuing a request.
//
// Wire: PUT /ehr/{ehr_id}/ehr_status with If-Match. The response
// shape follows the Prefer option: minimal returns no body, the
// returned `*rm.EHRStatus` is nil and only the metadata is populated;
// representation returns the full updated EHR_STATUS in the body.
//
// Errors map per REQ-093: 409 → [transport.ErrVersionConflict], 412 →
// [transport.ErrPreconditionFailed], 428 → [transport.ErrPreconditionRequired].
func Put(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, ifMatch string, status *rm.EHRStatus, opts ...PutOption) (*rm.EHRStatus, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("ehrstatus.Put: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if ifMatch == "" {
		return nil, nil, fmt.Errorf("ehrstatus.Put: %w: empty If-Match (REQ-054)", transport.ErrInvalidConfig)
	}
	if status == nil {
		return nil, nil, fmt.Errorf("ehrstatus.Put: %w: nil status", transport.ErrInvalidConfig)
	}
	cfg := putConfig{prefer: transport.PreferMinimal}
	for _, o := range opts {
		o(&cfg)
	}
	body, err := canjson.Marshal(status)
	if err != nil {
		return nil, nil, fmt.Errorf("ehrstatus.Put: marshal body: %w", err)
	}
	auditHeader, err := marshalAuditDetails(cfg.auditDetails)
	if err != nil {
		return nil, nil, fmt.Errorf("ehrstatus.Put: %w", err)
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
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return nil, newVersionMetadata(resp.Metadata), err
		}
		return nil, nil, err
	}
	meta := newVersionMetadata(resp.Metadata)
	if cfg.prefer != transport.PreferRepresentation || len(resp.Body) == 0 {
		return nil, meta, nil
	}
	var out rm.EHRStatus
	if err := canjson.Unmarshal(resp.Body, &out); err != nil {
		return nil, meta, fmt.Errorf("ehrstatus.Put: decode response: %w", err)
	}
	return &out, meta, nil
}

// marshalAuditDetails canjson-encodes a non-nil AuditDetails for the
// openehr-audit-details header. Returns "" for nil input.
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

// Repository mirrors the package functions for DI seams.
type Repository interface {
	Get(ctx context.Context, ehrID openehrclient.EHRID) (*rm.EHRStatus, *openehrclient.VersionMetadata, error)
	GetAtTime(ctx context.Context, ehrID openehrclient.EHRID, t time.Time) (*rm.EHRStatus, *openehrclient.VersionMetadata, error)
	GetVersioned(ctx context.Context, ehrID openehrclient.EHRID, versionUID openehrclient.VersionUID) (*rm.EHRStatus, *openehrclient.VersionMetadata, error)
	Put(ctx context.Context, ehrID openehrclient.EHRID, ifMatch string, status *rm.EHRStatus, opts ...PutOption) (*rm.EHRStatus, *openehrclient.VersionMetadata, error)
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

func (r *repository) Put(ctx context.Context, id openehrclient.EHRID, ifMatch string, status *rm.EHRStatus, opts ...PutOption) (*rm.EHRStatus, *openehrclient.VersionMetadata, error) {
	return Put(ctx, r.c, id, ifMatch, status, opts...)
}
