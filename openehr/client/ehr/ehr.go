package ehr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Get retrieves the EHR identified by id.
//
// Wire: GET /ehr/{ehr_id}. Returns the decoded *rm.EHR plus the
// response metadata (ETag/Location captured even though the EHR root
// is not versioned per-write).
func Get(ctx context.Context, c *transport.Client, id EHRID) (*rm.EHR, *VersionMetadata, error) {
	if id == "" {
		return nil, nil, fmt.Errorf("ehr.Get: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   "/ehr/" + url.PathEscape(string(id)),
		Route:  "/ehr/{ehr_id}",
	}
	out, meta, err := transport.Decode[rm.EHR](ctx, c, req)
	return out, NewVersionMetadata(meta), err
}

// Exists reports whether the EHR identified by id is present on the
// deployment.
//
// Wire: HEAD /ehr/{ehr_id}. A 2xx response yields true; a 404 yields
// (false, nil) — absence is not an error. Other wire errors (auth,
// 5xx) surface as the typed error per [transport.WireError].
func Exists(ctx context.Context, c *transport.Client, id EHRID) (bool, error) {
	if id == "" {
		return false, fmt.Errorf("ehr.Exists: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	resp, err := c.Do(ctx, &transport.Request{
		Method: http.MethodHead,
		Path:   "/ehr/" + url.PathEscape(string(id)),
		Route:  "/ehr/{ehr_id}",
	})
	if err != nil {
		if errors.Is(err, transport.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}

// GetBySubject retrieves the EHR associated with an external subject
// identifier — the (namespace, id) pair that the EHR was created with.
//
// Wire: GET /ehr?subject_id=...&subject_namespace=...
// Returns ErrNotFound on a 404 (no EHR matches the subject).
func GetBySubject(ctx context.Context, c *transport.Client, subjectNamespace, subjectID string) (*rm.EHR, *VersionMetadata, error) {
	if subjectNamespace == "" || subjectID == "" {
		return nil, nil, fmt.Errorf("ehr.GetBySubject: %w: namespace and id are required", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   "/ehr",
		Route:  "/ehr",
		Query: url.Values{
			"subject_id":        []string{subjectID},
			"subject_namespace": []string{subjectNamespace},
		},
	}
	out, meta, err := transport.Decode[rm.EHR](ctx, c, req)
	return out, NewVersionMetadata(meta), err
}

// createConfig is the resolved option set for [Create].
type createConfig struct {
	ehrID         EHRID
	initialStatus *rm.EHRStatus
}

// CreateOption mutates [Create]'s behaviour.
type CreateOption func(*createConfig)

// WithEHRID binds the new EHR to a client-supplied identifier. When
// set, [Create] issues PUT /ehr/{ehr_id}. When unset (default), POST
// /ehr is used and the server assigns the id (returned via Location +
// the decoded *rm.EHR).
func WithEHRID(id EHRID) CreateOption {
	return func(c *createConfig) { c.ehrID = id }
}

// WithInitialStatus carries an initial EHR_STATUS in the request body.
// When unset the server creates a default EHR_STATUS (no subject
// linkage, queryable + modifiable per the deployment's policy).
func WithInitialStatus(s *rm.EHRStatus) CreateOption {
	return func(c *createConfig) { c.initialStatus = s }
}

// Create issues an EHR-creation request. POST /ehr (server-assigned
// ehr_id) or PUT /ehr/{ehr_id} (client-supplied via [WithEHRID]).
//
// The optional initial EHR_STATUS body is canjson-encoded; if omitted
// the request body is empty and the server creates a default
// EHR_STATUS. Returns the decoded *rm.EHR (Prefer=representation by
// default — callers almost always need the new ehr_id back even when
// they supplied one).
func Create(ctx context.Context, c *transport.Client, opts ...CreateOption) (*rm.EHR, *VersionMetadata, error) {
	cfg := createConfig{}
	for _, o := range opts {
		o(&cfg)
	}

	var body []byte
	if cfg.initialStatus != nil {
		b, err := canjson.Marshal(cfg.initialStatus)
		if err != nil {
			return nil, nil, fmt.Errorf("ehr.Create: marshal initial status: %w", err)
		}
		body = b
	}

	req := &transport.Request{
		Body:   body,
		Prefer: transport.PreferRepresentation,
	}
	if cfg.ehrID != "" {
		req.Method = http.MethodPut
		req.Path = "/ehr/" + url.PathEscape(string(cfg.ehrID))
		req.Route = "/ehr/{ehr_id}"
	} else {
		req.Method = http.MethodPost
		req.Path = "/ehr"
		req.Route = "/ehr"
	}

	out, meta, err := transport.Decode[rm.EHR](ctx, c, req)
	return out, NewVersionMetadata(meta), err
}

// Repository mirrors the package-level EHR functions as a method set
// bound to a single *transport.Client. Useful for dependency-injection
// seams (REQ-023).
type Repository interface {
	Get(ctx context.Context, id EHRID) (*rm.EHR, *VersionMetadata, error)
	Exists(ctx context.Context, id EHRID) (bool, error)
	GetBySubject(ctx context.Context, subjectNamespace, subjectID string) (*rm.EHR, *VersionMetadata, error)
	Create(ctx context.Context, opts ...CreateOption) (*rm.EHR, *VersionMetadata, error)
}

// NewRepository binds c to a Repository.
func NewRepository(c *transport.Client) Repository { return &repository{c: c} }

type repository struct{ c *transport.Client }

func (r *repository) Get(ctx context.Context, id EHRID) (*rm.EHR, *VersionMetadata, error) {
	return Get(ctx, r.c, id)
}

func (r *repository) Exists(ctx context.Context, id EHRID) (bool, error) {
	return Exists(ctx, r.c, id)
}

func (r *repository) GetBySubject(ctx context.Context, ns, id string) (*rm.EHR, *VersionMetadata, error) {
	return GetBySubject(ctx, r.c, ns, id)
}

func (r *repository) Create(ctx context.Context, opts ...CreateOption) (*rm.EHR, *VersionMetadata, error) {
	return Create(ctx, r.c, opts...)
}
